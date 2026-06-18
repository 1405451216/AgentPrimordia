package llm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
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

// ResilientProvider 弹性 Provider（带重试 + Fallback + 熔断）
// perf-v4 Task 10：state/failures/lastFail/halfOpenProbe 改为原子操作，
// checkCircuit() 快速路径（closed 状态）零锁获取
type ResilientProvider struct {
	primary       Provider
	fallbacks     []Provider
	config        ResilientConfig
	state         atomic.Int32 // circuitState
	failures      atomic.Int64
	mu            sync.RWMutex // 仅保护 fallbacks slice
	lastFail      atomic.Int64 // UnixNano 时间戳
	halfOpenProbe atomic.Bool
}

func NewResilientProvider(primary Provider, cfg ResilientConfig) (*ResilientProvider, error) {
	if primary == nil {
		return nil, fmt.Errorf("primary provider must not be nil")
	}
	r := &ResilientProvider{
		primary: primary,
		config:  cfg,
	}
	r.state.Store(int32(circuitClosed))
	return r, nil
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

	// 尝试主 Provider（带一次重试）
	for attempt := 0; attempt <= 1; attempt++ {
		if attempt > 0 {
			backoff := r.calculateBackoff(attempt)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		ch, err := r.primary.Stream(ctx, req)
		if err == nil {
			r.recordSuccess()
			return ch, nil
		}
	}

	// 主 Provider 失败，尝试 Fallback
	var lastErr error
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
		resp, err := embedder.Embeddings(ctx, texts)
		if err != nil {
			r.recordFailure()
			return nil, err
		}
		r.recordSuccess()
		return resp, nil
	}
	return nil, ErrNotSupported
}

func (r *ResilientProvider) Info() ModelInfo {
	return r.primary.Info()
}

// checkCircuit 检查熔断器状态（perf-v4 Task 10：快速路径零锁）
func (r *ResilientProvider) checkCircuit() error {
	state := circuitState(r.state.Load())
	switch state {
	case circuitClosed:
		// 快速路径：closed 状态零锁直接返回（perf-v4 Task 10）
		return nil
	case circuitOpen:
		// open 状态：检查是否已过恢复时间（无锁读取 lastFail）
		lastFailNanos := r.lastFail.Load()
		if lastFailNanos == 0 {
			return ErrCircuitOpen
		}
		if time.Since(time.Unix(0, lastFailNanos)) > r.config.CircuitRecoverAfter {
			// 升级到 half-open：使用 CAS 避免并发升级
			if r.state.CompareAndSwap(int32(circuitOpen), int32(circuitHalfOpen)) {
				r.halfOpenProbe.Store(false)
			}
			return nil
		}
		return ErrCircuitOpen
	case circuitHalfOpen:
		// 半开状态只允许一个试探请求
		if r.halfOpenProbe.Load() {
			return ErrCircuitOpen
		}
		r.halfOpenProbe.Store(true)
		return nil
	default:
		return nil
	}
}

func (r *ResilientProvider) recordSuccess() {
	// 成功直接重置为 closed（perf-v4 Task 10：无锁）
	r.failures.Store(0)
	r.state.Store(int32(circuitClosed))
	r.halfOpenProbe.Store(false)
}

func (r *ResilientProvider) recordFailure() {
	// 失败累加计数并记录时间戳（perf-v4 Task 10：无锁）
	r.failures.Add(1)
	r.lastFail.Store(time.Now().UnixNano())
	r.halfOpenProbe.Store(false)

	if r.failures.Load() >= int64(r.config.CircuitThreshold) {
		r.state.Store(int32(circuitOpen))
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
	// 添加随机抖动 (jitter) 防止惊群效应，但不超过 MaxBackoff
	jitter := rand.Float64() * float64(r.config.RetryBackoff) * 0.5
	result := time.Duration(backoff + jitter)
	if result > r.config.MaxBackoff {
		result = r.config.MaxBackoff
	}
	return result
}
