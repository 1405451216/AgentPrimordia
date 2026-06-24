package resilience

import (
	"context"
	"errors"
	"sync"
	"time"
)

// State 断路器状态
type State int

const (
	StateClosed   State = iota // 正常
	StateOpen                  // 断路
	StateHalfOpen              // 半开（试探）
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ErrCircuitOpen 断路器打开时返回的错误
var ErrCircuitOpen = errors.New("circuit breaker is open")

// Config 断路器配置
type Config struct {
	FailureThreshold int           // 触发断路的连续失败次数
	Timeout          time.Duration // 从 Open 到 HalfOpen 的等待时间
}

// CircuitBreaker 断路器实现
type CircuitBreaker struct {
	cfg      Config
	mu       sync.RWMutex
	state    State
	failures int
	lastFail time.Time
}

// NewCircuitBreaker 创建断路器
func NewCircuitBreaker(cfg Config) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &CircuitBreaker{
		cfg:   cfg,
		state: StateClosed,
	}
}

// State 返回当前状态（只读观察，不做状态转换）
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// currentState 获取当前状态，并在需要时执行 Open→HalfOpen 转换（需持有写锁）
func (cb *CircuitBreaker) currentState() State {
	if cb.state == StateOpen && time.Since(cb.lastFail) > cb.cfg.Timeout {
		cb.state = StateHalfOpen
		cb.failures = 0 // 进入 HalfOpen 时重置失败计数
	}
	return cb.state
}

// Execute 通过断路器执行函数
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	cb.mu.Lock()
	state := cb.currentState()
	if state == StateOpen {
		cb.mu.Unlock()
		return ErrCircuitOpen
	}
	cb.mu.Unlock()

	err := fn(ctx)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		cb.lastFail = time.Now()
		if state == StateHalfOpen {
			// HalfOpen 下一次失败直接回到 Open
			cb.state = StateOpen
		} else if cb.failures >= cb.cfg.FailureThreshold {
			cb.state = StateOpen
		}
		return err
	}

	// 成功：重置计数
	cb.failures = 0
	if state == StateHalfOpen {
		cb.state = StateClosed
	}
	return nil
}

// ExecuteWithFallback 带降级回调的执行
func (cb *CircuitBreaker) ExecuteWithFallback(
	ctx context.Context,
	fn func(ctx context.Context) error,
	fallback func(ctx context.Context, err error) error,
) error {
	err := cb.Execute(ctx, fn)
	if err != nil {
		return fallback(ctx, err)
	}
	return nil
}

// Reset 手动重置断路器
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	cb.state = StateClosed
	cb.failures = 0
	cb.mu.Unlock()
}
