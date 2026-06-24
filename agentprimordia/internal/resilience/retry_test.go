package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetrier_SuccessOnFirstAttempt(t *testing.T) {
	r := NewRetrier(RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	attempts := 0
	err := r.Do(context.Background(), func(ctx context.Context) error {
		attempts++
		return nil
	})

	if err != nil {
		t.Errorf("期望成功，得到错误: %v", err)
	}
	if attempts != 1 {
		t.Errorf("期望尝试 1 次，实际 %d 次", attempts)
	}
}

func TestRetrier_SuccessAfterRetries(t *testing.T) {
	r := NewRetrier(RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
	})

	attempts := 0
	err := r.Do(context.Background(), func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("期望成功，得到错误: %v", err)
	}
	if attempts != 3 {
		t.Errorf("期望尝试 3 次，实际 %d 次", attempts)
	}
}

func TestRetrier_AllAttemptsFail(t *testing.T) {
	r := NewRetrier(RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
	})

	testErr := errors.New("persistent error")
	attempts := 0
	err := r.Do(context.Background(), func(ctx context.Context) error {
		attempts++
		return testErr
	})

	if !errors.Is(err, testErr) {
		t.Errorf("期望返回原始错误，得到: %v", err)
	}
	if attempts != 3 {
		t.Errorf("期望尝试 3 次，实际 %d 次", attempts)
	}
}

func TestRetrier_NonRetryableError(t *testing.T) {
	r := NewRetrier(RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
	})

	authErr := NewNonRetryableError(errors.New("auth failed"))
	attempts := 0
	err := r.Do(context.Background(), func(ctx context.Context) error {
		attempts++
		return authErr
	})

	if attempts != 1 {
		t.Errorf("不可重试错误应只尝试 1 次，实际 %d 次", attempts)
	}
	if !errors.Is(err, authErr) {
		t.Errorf("期望返回原始错误，得到: %v", err)
	}
}

func TestRetrier_ContextCancelled(t *testing.T) {
	r := NewRetrier(RetryConfig{
		MaxAttempts:  10,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	attempts := 0
	err := r.Do(ctx, func(ctx context.Context) error {
		attempts++
		return errors.New("always fail")
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("期望返回 DeadlineExceeded，得到: %v", err)
	}
	if attempts >= 10 {
		t.Errorf("上下文取消后应停止重试，实际尝试 %d 次", attempts)
	}
}

func TestRetrier_ExponentialBackoff(t *testing.T) {
	r := NewRetrier(RetryConfig{
		MaxAttempts:  4,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	})

	delays := make([]time.Duration, 0, 3)
	for attempt := 0; attempt < 3; attempt++ {
		delay := r.calculateDelay(attempt)
		delays = append(delays, delay)
	}

	// 验证延迟递增（考虑抖动，允许一定误差）
	if delays[1] <= delays[0] {
		t.Errorf("延迟应递增: %v <= %v", delays[1], delays[0])
	}
	if delays[2] <= delays[1] {
		t.Errorf("延迟应递增: %v <= %v", delays[2], delays[1])
	}
}

func TestRetrier_MaxDelayLimit(t *testing.T) {
	r := NewRetrier(RetryConfig{
		MaxAttempts:  10,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     200 * time.Millisecond,
		Multiplier:   10.0, // 很大的乘数
	})

	// 第 5 次重试的延迟应该被限制在 MaxDelay
	delay := r.calculateDelay(5)
	if delay > 250*time.Millisecond { // 允许 25% 抖动
		t.Errorf("延迟应限制在 MaxDelay 附近，实际: %v", delay)
	}
}

func TestRetrier_DefaultConfig(t *testing.T) {
	r := NewRetrier(RetryConfig{})

	if r.cfg.MaxAttempts != 3 {
		t.Errorf("默认 MaxAttempts = %d, 期望 3", r.cfg.MaxAttempts)
	}
	if r.cfg.InitialDelay != 100*time.Millisecond {
		t.Errorf("默认 InitialDelay = %v, 期望 100ms", r.cfg.InitialDelay)
	}
	if r.cfg.MaxDelay != 10*time.Second {
		t.Errorf("默认 MaxDelay = %v, 期望 10s", r.cfg.MaxDelay)
	}
	if r.cfg.Multiplier != 2.0 {
		t.Errorf("默认 Multiplier = %f, 期望 2.0", r.cfg.Multiplier)
	}
}

func TestNonRetryableError(t *testing.T) {
	originalErr := errors.New("original error")
	wrappedErr := NewNonRetryableError(originalErr)

	if wrappedErr.Error() != "original error" {
		t.Errorf("错误消息不匹配: %v", wrappedErr.Error())
	}

	if !errors.Is(wrappedErr, originalErr) {
		t.Error("Unwrap 应返回原始错误")
	}

	if !IsNonRetryable(wrappedErr) {
		t.Error("应识别为不可重试错误")
	}

	regularErr := errors.New("regular error")
	if IsNonRetryable(regularErr) {
		t.Error("普通错误不应识别为不可重试")
	}
}
