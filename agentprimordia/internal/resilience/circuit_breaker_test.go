package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedState(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 3,
		Timeout:          1 * time.Second,
	})

	// 成功调用不应触发断路
	for i := 0; i < 5; i++ {
		err := cb.Execute(context.Background(), func(ctx context.Context) error {
			return nil
		})
		if err != nil {
			t.Fatalf("成功调用返回错误: %v", err)
		}
	}

	if cb.State() != StateClosed {
		t.Errorf("状态 = %v, 期望 Closed", cb.State())
	}
}

func TestCircuitBreaker_OpenAfterFailures(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 3,
		Timeout:          1 * time.Second,
	})

	testErr := errors.New("test error")

	// 连续失败 3 次
	for i := 0; i < 3; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return testErr
		})
	}

	if cb.State() != StateOpen {
		t.Errorf("状态 = %v, 期望 Open", cb.State())
	}

	// 断路后调用应立即返回错误
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("错误 = %v, 期望 ErrCircuitOpen", err)
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 2,
		Timeout:          50 * time.Millisecond,
	})

	// 触发断路
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return errors.New("fail")
		})
	}

	if cb.State() != StateOpen {
		t.Fatalf("状态应为 Open")
	}

	// 等待超时
	time.Sleep(60 * time.Millisecond)

	// 通过 Execute 触发 Open→HalfOpen 转换，半开状态成功调用应关闭断路器
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("半开成功调用返回错误: %v", err)
	}

	if cb.State() != StateClosed {
		t.Errorf("状态 = %v, 期望 Closed", cb.State())
	}
}

func TestCircuitBreaker_Fallback(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 1,
		Timeout:          1 * time.Second,
	})

	// 触发断路
	cb.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("fail")
	})

	// 使用 fallback
	fallbackCalled := false
	err := cb.ExecuteWithFallback(context.Background(),
		func(ctx context.Context) error {
			return errors.New("primary fail")
		},
		func(ctx context.Context, err error) error {
			fallbackCalled = true
			return nil
		},
	)

	if err != nil {
		t.Fatalf("fallback 后仍返回错误: %v", err)
	}
	if !fallbackCalled {
		t.Error("fallback 未被调用")
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 2,
		Timeout:          1 * time.Second,
	})

	// 触发断路
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return errors.New("fail")
		})
	}

	if cb.State() != StateOpen {
		t.Fatalf("状态应为 Open")
	}

	// 手动重置
	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("重置后状态 = %v, 期望 Closed", cb.State())
	}

	// 重置后应能正常执行
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("重置后执行失败: %v", err)
	}
}

func TestCircuitBreaker_DefaultConfig(t *testing.T) {
	cb := NewCircuitBreaker(Config{})

	if cb.cfg.FailureThreshold != 5 {
		t.Errorf("默认 FailureThreshold = %d, 期望 5", cb.cfg.FailureThreshold)
	}
	if cb.cfg.Timeout != 30*time.Second {
		t.Errorf("默认 Timeout = %v, 期望 30s", cb.cfg.Timeout)
	}
}

func TestCircuitBreaker_StateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, 期望 %q", tt.state, got, tt.want)
		}
	}
}

// TestCircuitBreaker_ConcurrentExecute 验证并发 Execute 不会产生数据竞争
func TestCircuitBreaker_ConcurrentExecute(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 10,
		Timeout:          1 * time.Second,
	})

	const goroutines = 20
	done := make(chan struct{}, goroutines)

	// 并发执行成功和失败调用
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				if id%2 == 0 {
					cb.Execute(context.Background(), func(ctx context.Context) error {
						return nil
					})
				} else {
					cb.Execute(context.Background(), func(ctx context.Context) error {
						return errors.New("fail")
					})
				}
			}
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < goroutines; i++ {
		<-done
	}
}

// TestCircuitBreaker_HalfOpenFailureBackToOpen 验证 HalfOpen 状态下失败会回到 Open
func TestCircuitBreaker_HalfOpenFailureBackToOpen(t *testing.T) {
	cb := NewCircuitBreaker(Config{
		FailureThreshold: 2,
		Timeout:          50 * time.Millisecond,
	})

	// 触发断路
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return errors.New("fail")
		})
	}

	if cb.State() != StateOpen {
		t.Fatalf("状态应为 Open")
	}

	// 等待超时，通过 Execute 触发 Open→HalfOpen 转换
	time.Sleep(60 * time.Millisecond)

	// HalfOpen 状态下再次失败，应回到 Open
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("still failing")
	})
	if err == nil {
		t.Error("期望返回错误")
	}

	if cb.State() != StateOpen {
		t.Errorf("HalfOpen 失败后状态 = %v, 期望 Open", cb.State())
	}
}
