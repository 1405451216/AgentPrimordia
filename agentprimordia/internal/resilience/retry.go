package resilience

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxAttempts  int           // 最大尝试次数（包含首次）
	InitialDelay time.Duration // 初始延迟
	MaxDelay     time.Duration // 最大延迟
	Multiplier   float64       // 延迟乘数（指数退避因子）
}

// Retrier 重试器
type Retrier struct {
	cfg RetryConfig
}

// NewRetrier 创建重试器
func NewRetrier(cfg RetryConfig) *Retrier {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = 100 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 10 * time.Second
	}
	if cfg.Multiplier <= 0 {
		cfg.Multiplier = 2.0
	}
	return &Retrier{cfg: cfg}
}

// Do 执行函数，失败时重试
func (r *Retrier) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	var lastErr error

	for attempt := 0; attempt < r.cfg.MaxAttempts; attempt++ {
		// 检查上下文是否已取消
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// 执行函数
		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}

		// 检查是否为不可重试错误
		var nre *NonRetryableError
		if errors.As(lastErr, &nre) {
			return lastErr
		}

		// 如果还有重试机会，等待后重试
		if attempt < r.cfg.MaxAttempts-1 {
			delay := r.calculateDelay(attempt)
			select {
			case <-time.After(delay):
				// 继续重试
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return lastErr
}

// calculateDelay 计算第 n 次重试的延迟（指数退避 + 抖动）
func (r *Retrier) calculateDelay(attempt int) time.Duration {
	delay := float64(r.cfg.InitialDelay)

	// 指数退避
	for i := 0; i < attempt; i++ {
		delay *= r.cfg.Multiplier
	}

	// 限制最大延迟
	if delay > float64(r.cfg.MaxDelay) {
		delay = float64(r.cfg.MaxDelay)
	}

	// 添加抖动（±25%）
	jitter := delay * 0.25
	delay = delay + (rand.Float64()*2-1)*jitter

	return time.Duration(delay)
}

// NonRetryableError 不可重试错误
type NonRetryableError struct {
	err error
}

func (e *NonRetryableError) Error() string {
	return e.err.Error()
}

func (e *NonRetryableError) Unwrap() error {
	return e.err
}

// NewNonRetryableError 创建不可重试错误
func NewNonRetryableError(err error) error {
	if err == nil {
		err = errors.New("non-retryable error")
	}
	return &NonRetryableError{err: err}
}

// IsNonRetryable 检查是否为不可重试错误
func IsNonRetryable(err error) bool {
	var nre *NonRetryableError
	return errors.As(err, &nre)
}
