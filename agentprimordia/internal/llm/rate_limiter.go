// RateLimiter 基于令牌桶算法的速率限制器
// 用于控制 LLM API 调用频率，防止触发上游限流
package llm

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrRateLimited 表示请求被速率限制拒绝
var ErrRateLimited = errors.New("rate limit exceeded")

// RateLimitConfig 速率限制配置
type RateLimitConfig struct {
	// RequestsPerSecond 每秒允许的请求数
	RequestsPerSecond float64
	// BurstSize 突发容量（令牌桶最大令牌数）
	BurstSize int
	// MaxWait 等待令牌的最大时间，0 表示不等待
	MaxWait time.Duration
}

// DefaultRateLimitConfig 默认速率限制配置
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         20,
		MaxWait:           5 * time.Second,
	}
}

// tokenBucket 令牌桶实现
type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // 每秒补充的令牌数
	lastRefill time.Time
}

func newTokenBucket(rate float64, burst int) *tokenBucket {
	return &tokenBucket{
		tokens:     float64(burst),
		maxTokens:  float64(burst),
		refillRate: rate,
		lastRefill: time.Now(),
	}
}

// allow 尝试获取一个令牌，返回是否成功
func (b *tokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refill()
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// waitToken 等待获取令牌，支持上下文取消
// perf-v4 Task 9：合并 deadline 检查与 waitDuration 比较，减少 time.Now() 系统调用
func (b *tokenBucket) waitToken(ctx context.Context, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	for {
		b.mu.Lock()
		b.refill()
		if b.tokens >= 1 {
			b.tokens--
			b.mu.Unlock()
			return nil
		}
		// 计算下一个令牌到达时间
		deficit := 1 - b.tokens
		waitDuration := time.Duration(float64(time.Second) * deficit / b.refillRate)
		b.mu.Unlock()

		// 一次性检查：合并 deadline 过期与 waitDuration 超限逻辑（perf-v4 Task 9）
		if waitDuration <= 0 {
			return ErrRateLimited
		}
		// 限制等待时间不超过 deadline（同时检测 deadline 过期）
		remaining := time.Until(deadline)
		if waitDuration > remaining {
			waitDuration = remaining
		}
		if waitDuration <= 0 {
			return ErrRateLimited
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
			// 继续循环尝试获取令牌
		}
	}
}

// refill 补充令牌（调用者必须持有锁）
func (b *tokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	b.lastRefill = now
}

// Tokens 返回当前可用令牌数（用于测试和监控）
func (b *tokenBucket) Tokens() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refill()
	return b.tokens
}

// RateLimitedProvider 包装 Provider 并施加速率限制
type RateLimitedProvider struct {
	provider Provider
	bucket   *tokenBucket
	config   RateLimitConfig
}

// NewRateLimitedProvider 创建带速率限制的 Provider 包装器
func NewRateLimitedProvider(provider Provider, cfg RateLimitConfig) (*RateLimitedProvider, error) {
	if provider == nil {
		return nil, errors.New("provider must not be nil")
	}
	if cfg.RequestsPerSecond <= 0 {
		cfg.RequestsPerSecond = DefaultRateLimitConfig().RequestsPerSecond
	}
	if cfg.BurstSize <= 0 {
		cfg.BurstSize = DefaultRateLimitConfig().BurstSize
	}
	if cfg.MaxWait <= 0 {
		cfg.MaxWait = DefaultRateLimitConfig().MaxWait
	}
	return &RateLimitedProvider{
		provider: provider,
		bucket:   newTokenBucket(cfg.RequestsPerSecond, cfg.BurstSize),
		config:   cfg,
	}, nil
}

// acquire 获取令牌，支持上下文和等待
func (r *RateLimitedProvider) acquire(ctx context.Context) error {
	if r.bucket.allow() {
		return nil
	}
	return r.bucket.waitToken(ctx, r.config.MaxWait)
}

func (r *RateLimitedProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if err := r.acquire(ctx); err != nil {
		return nil, err
	}
	return r.provider.Complete(ctx, req)
}

func (r *RateLimitedProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	if err := r.acquire(ctx); err != nil {
		return nil, err
	}
	return r.provider.Stream(ctx, req)
}

func (r *RateLimitedProvider) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	if err := r.acquire(ctx); err != nil {
		return nil, err
	}
	return r.provider.CallTools(ctx, req)
}

func (r *RateLimitedProvider) Info() ModelInfo {
	return r.provider.Info()
}

// AvailableTokens 返回当前可用令牌数（用于监控）
func (r *RateLimitedProvider) AvailableTokens() float64 {
	return r.bucket.Tokens()
}
