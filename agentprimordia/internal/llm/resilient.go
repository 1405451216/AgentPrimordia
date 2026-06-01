package llm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

var (
	ErrCircuitOpen      = errors.New("circuit breaker is open")
	ErrRetriesExhausted = errors.New("all retries exhausted")
	ErrFallbackFailed   = errors.New("all fallback providers failed")
)

type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

type ResilientConfig struct {
	MaxRetries          int
	RetryBackoff        time.Duration
	MaxBackoff          time.Duration
	CircuitThreshold    int
	CircuitRecoverAfter time.Duration
}

func DefaultResilientConfig() ResilientConfig {
	return ResilientConfig{
		MaxRetries:          3,
		RetryBackoff:        500 * time.Millisecond,
		MaxBackoff:          10 * time.Second,
		CircuitThreshold:    5,
		CircuitRecoverAfter: 30 * time.Second,
	}
}

type ResilientProvider struct {
	primary       Provider
	fallbacks     []Provider
	config        ResilientConfig
	state         circuitState
	failures      int
	mu            sync.RWMutex
	lastFail      time.Time
	halfOpenProbe bool
}

func NewResilientProvider(primary Provider, cfg ResilientConfig) (*ResilientProvider, error) {
	if primary == nil {
		return nil, fmt.Errorf("primary provider must not be nil")
	}
	return &ResilientProvider{
		primary: primary,
		config:  cfg,
		state:   circuitClosed,
	}, nil
}

func (r *ResilientProvider) AddFallback(provider Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallbacks = append(r.fallbacks, provider)
}

// getFallbacks returns a snapshot of current fallback providers under read lock.
func (r *ResilientProvider) getFallbacks() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	snapshot := make([]Provider, len(r.fallbacks))
	copy(snapshot, r.fallbacks)
	return snapshot
}

func (r *ResilientProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if err := r.checkCircuit(); err != nil {
		return nil, err
	}

	resp, err := executeWithRetry(ctx, r, func(p Provider) (*CompletionResponse, error) {
		return p.Complete(ctx, req)
	})
	if err != nil {
		r.recordFailure()
		return nil, err
	}

	r.recordSuccess()
	return resp, nil
}

func (r *ResilientProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	if err := r.checkCircuit(); err != nil {
		return nil, err
	}

	// 尝试主 Provider
	ch, err := r.primary.Stream(ctx, req)
	if err == nil {
		r.recordSuccess()
		return ch, nil
	}

	// 主 Provider 失败，尝试 Fallback
	lastErr := err
	fallbacks := r.getFallbacks()
	for _, fb := range fallbacks {
		fbCh, fbErr := fb.Stream(ctx, req)
		if fbErr == nil {
			r.recordSuccess()
			return fbCh, nil
		}
		lastErr = fbErr
	}

	r.recordFailure()
	return nil, fmt.Errorf("%w: stream failed on all providers: %v", ErrFallbackFailed, lastErr)
}

func (r *ResilientProvider) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	if err := r.checkCircuit(); err != nil {
		return nil, err
	}

	resp, err := executeWithRetry(ctx, r, func(p Provider) (*ToolCallResponse, error) {
		return p.CallTools(ctx, req)
	})
	if err != nil {
		r.recordFailure()
		return nil, err
	}

	r.recordSuccess()
	return resp, nil
}

func (r *ResilientProvider) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if embedder, ok := r.primary.(Embedder); ok {
		return embedder.Embeddings(ctx, texts)
	}
	return nil, ErrNotSupported
}

func (r *ResilientProvider) Info() ModelInfo {
	return r.primary.Info()
}

func (r *ResilientProvider) checkCircuit() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch r.state {
	case circuitOpen:
		if time.Since(r.lastFail) > r.config.CircuitRecoverAfter {
			r.state = circuitHalfOpen
			r.halfOpenProbe = true
			return nil
		}
		return ErrCircuitOpen
	case circuitHalfOpen:
		// 半开状态只允许一个试探请求
		if r.halfOpenProbe {
			return ErrCircuitOpen
		}
		r.halfOpenProbe = true
		return nil
	default:
		return nil
	}
}

func (r *ResilientProvider) recordSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures = 0
	r.state = circuitClosed
	r.halfOpenProbe = false
}

func (r *ResilientProvider) recordFailure() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.failures++
	r.lastFail = time.Now()
	r.halfOpenProbe = false

	if r.failures >= r.config.CircuitThreshold {
		r.state = circuitOpen
	}
}

// executeWithRetry 泛型重试方法，统一 Complete 和 CallTools 的重试逻辑
func executeWithRetry[T any](ctx context.Context, r *ResilientProvider, fn func(Provider) (T, error)) (T, error) {
	var lastErr error
	var zero T

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := r.calculateBackoff(attempt)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return zero, ctx.Err()
			}
		}

		resp, err := fn(r.primary)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}

	fallbacks := r.getFallbacks()
	if len(fallbacks) == 0 {
		return zero, fmt.Errorf("%w: %v", ErrRetriesExhausted, lastErr)
	}

	for _, fallback := range fallbacks {
		// Fallback 也使用重试，默认重试 1 次
		for attempt := 0; attempt <= 1; attempt++ {
			if attempt > 0 {
				backoff := r.calculateBackoff(attempt)
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return zero, ctx.Err()
				}
			}
			resp, err := fn(fallback)
			if err == nil {
				return resp, nil
			}
			lastErr = err
		}
	}

	return zero, fmt.Errorf("%w: %v", ErrFallbackFailed, lastErr)
}

func (r *ResilientProvider) calculateBackoff(attempt int) time.Duration {
	backoff := float64(r.config.RetryBackoff) * math.Pow(2, float64(attempt-1))
	if backoff > float64(r.config.MaxBackoff) {
		backoff = float64(r.config.MaxBackoff)
	}
	return time.Duration(backoff)
}
