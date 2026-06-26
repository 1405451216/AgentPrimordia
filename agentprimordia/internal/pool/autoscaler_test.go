package pool

import (
	"testing"
	"time"
)

func TestAutoScaler_ScaleUp(t *testing.T) {
	cfg := AutoScalerConfig{
		MinConcurrency:     2,
		MaxConcurrency:     20,  // 提高最大值，允许扩容
		ScaleUpThreshold:   0.8, // 80% 利用率时扩容
		ScaleDownThreshold: 0.2, // 20% 利用率时缩容
		CoolDownPeriod:     100 * time.Millisecond,
	}

	scaler := NewAutoScaler(cfg)

	// 模拟高负载：running=8, queued=2, current=10
	// 利用率 = (8 + 2) / 10 = 1.0 > 0.8，应该扩容
	newConcurrency := scaler.Calculate(8, 2, 10)

	if newConcurrency <= 10 {
		t.Errorf("高负载下应该扩容，当前值: %d, 期望 > 10", newConcurrency)
	}
}

func TestAutoScaler_ScaleDown(t *testing.T) {
	cfg := AutoScalerConfig{
		MinConcurrency:     2,
		MaxConcurrency:     10,
		ScaleUpThreshold:   0.8,
		ScaleDownThreshold: 0.2,
		CoolDownPeriod:     100 * time.Millisecond,
	}

	scaler := NewAutoScaler(cfg)

	// 模拟低负载：running=1, queued=0, max=10
	// 利用率 = 1 / 10 = 0.1 < 0.2，应该缩容
	newConcurrency := scaler.Calculate(1, 0, 10)

	if newConcurrency >= 10 {
		t.Errorf("低负载下应该缩容，当前值: %d", newConcurrency)
	}
}

func TestAutoScaler_NoChange(t *testing.T) {
	cfg := AutoScalerConfig{
		MinConcurrency:     2,
		MaxConcurrency:     10,
		ScaleUpThreshold:   0.8,
		ScaleDownThreshold: 0.2,
		CoolDownPeriod:     100 * time.Millisecond,
	}

	scaler := NewAutoScaler(cfg)

	// 模拟中等负载：running=5, queued=0, max=10
	// 利用率 = 5 / 10 = 0.5，在 0.2-0.8 之间，不应该变化
	newConcurrency := scaler.Calculate(5, 0, 10)

	if newConcurrency != 10 {
		t.Errorf("中等负载下不应该变化，当前值: %d, 期望: 10", newConcurrency)
	}
}

func TestAutoScaler_RespectMinMax(t *testing.T) {
	cfg := AutoScalerConfig{
		MinConcurrency:     2,
		MaxConcurrency:     10,
		ScaleUpThreshold:   0.8,
		ScaleDownThreshold: 0.2,
		CoolDownPeriod:     100 * time.Millisecond,
	}

	scaler := NewAutoScaler(cfg)

	// 即使负载极高，也不应该超过 MaxConcurrency
	newConcurrency := scaler.Calculate(100, 100, 10)
	if newConcurrency > 10 {
		t.Errorf("不应该超过 MaxConcurrency，当前值: %d", newConcurrency)
	}

	// 即使负载极低，也不应该低于 MinConcurrency
	newConcurrency = scaler.Calculate(0, 0, 2)
	if newConcurrency < 2 {
		t.Errorf("不应该低于 MinConcurrency，当前值: %d", newConcurrency)
	}
}

func TestAutoScaler_CoolDownPeriod(t *testing.T) {
	cfg := AutoScalerConfig{
		MinConcurrency:     2,
		MaxConcurrency:     10,
		ScaleUpThreshold:   0.8,
		ScaleDownThreshold: 0.2,
		CoolDownPeriod:     200 * time.Millisecond,
	}

	scaler := NewAutoScaler(cfg)

	// 第一次扩容
	current := 5
	newConcurrency := scaler.Calculate(8, 2, current)
	if newConcurrency <= current {
		t.Errorf("第一次应该扩容")
	}

	// 立即再次计算，应该被冷却期阻止
	current = newConcurrency
	newConcurrency2 := scaler.Calculate(8, 2, current)
	if newConcurrency2 != current {
		t.Errorf("冷却期内不应该再次扩容，当前值: %d, 期望: %d", newConcurrency2, current)
	}

	// 等待冷却期后再计算
	time.Sleep(250 * time.Millisecond)
	newConcurrency3 := scaler.Calculate(8, 2, current)
	if newConcurrency3 <= current {
		t.Errorf("冷却期后应该可以再次扩容")
	}
}

func TestPool_WithAutoScaler(t *testing.T) {
	cfg := PoolConfig{
		MaxConcurrency: 5,
		Timeout:        5 * time.Second,
		AutoScaler: &AutoScalerConfig{
			MinConcurrency:     2,
			MaxConcurrency:     10,
			ScaleUpThreshold:   0.8,
			ScaleDownThreshold: 0.2,
			CoolDownPeriod:     100 * time.Millisecond,
		},
	}

	p := NewPool(cfg)
	defer p.Close()

	// 验证 AutoScaler 已启用
	if p.autoScaler == nil {
		t.Error("AutoScaler 应该被启用")
	}

	// 验证初始并发度
	if p.config.MaxConcurrency != 5 {
		t.Errorf("初始并发度应为 5，当前值: %d", p.config.MaxConcurrency)
	}
}

func TestPool_AutoScalerIntegration(t *testing.T) {
	cfg := PoolConfig{
		MaxConcurrency: 2,
		Timeout:        5 * time.Second,
		AutoScaler: &AutoScalerConfig{
			MinConcurrency:     2,
			MaxConcurrency:     5,
			ScaleUpThreshold:   0.5,
			ScaleDownThreshold: 0.1,
			CoolDownPeriod:     50 * time.Millisecond,
			CheckInterval:      50 * time.Millisecond,
		},
	}

	p := NewPool(cfg)
	defer p.Close()

	// 启动 AutoScaler
	p.StartAutoScaler()

	// 等待 AutoScaler 运行
	time.Sleep(100 * time.Millisecond)

	// 验证 AutoScaler 正在运行
	if !p.autoScalerRunning.Load() {
		t.Error("AutoScaler 应该正在运行")
	}

	// 停止 AutoScaler
	p.StopAutoScaler()

	// 验证 AutoScaler 已停止
	time.Sleep(100 * time.Millisecond)
	if p.autoScalerRunning.Load() {
		t.Error("AutoScaler 应该已停止")
	}
}

// TestAutoScaler_CalculateWithZeroCurrent 验证 current=0 时不会除零 panic
func TestAutoScaler_CalculateWithZeroCurrent(t *testing.T) {
	cfg := AutoScalerConfig{
		MinConcurrency:     2,
		MaxConcurrency:     10,
		ScaleUpThreshold:   0.8,
		ScaleDownThreshold: 0.2,
		CoolDownPeriod:     100 * time.Millisecond,
	}

	scaler := NewAutoScaler(cfg)

	// current=0 不应 panic，应使用 MinConcurrency 作为基准
	newConcurrency := scaler.Calculate(5, 2, 0)
	if newConcurrency < cfg.MinConcurrency {
		t.Errorf("current=0 时结果不应低于 MinConcurrency，当前值: %d", newConcurrency)
	}
}

// TestAutoScaler_GetConfig 验证 GetConfig 返回正确的配置
func TestAutoScaler_GetConfig(t *testing.T) {
	cfg := AutoScalerConfig{
		MinConcurrency:     3,
		MaxConcurrency:     20,
		ScaleUpThreshold:   0.7,
		ScaleDownThreshold: 0.3,
		CoolDownPeriod:     200 * time.Millisecond,
		CheckInterval:      500 * time.Millisecond,
	}

	scaler := NewAutoScaler(cfg)
	got := scaler.GetConfig()

	if got.MinConcurrency != cfg.MinConcurrency {
		t.Errorf("MinConcurrency = %d, 期望 %d", got.MinConcurrency, cfg.MinConcurrency)
	}
	if got.MaxConcurrency != cfg.MaxConcurrency {
		t.Errorf("MaxConcurrency = %d, 期望 %d", got.MaxConcurrency, cfg.MaxConcurrency)
	}
	if got.ScaleUpThreshold != cfg.ScaleUpThreshold {
		t.Errorf("ScaleUpThreshold = %f, 期望 %f", got.ScaleUpThreshold, cfg.ScaleUpThreshold)
	}
	if got.ScaleDownThreshold != cfg.ScaleDownThreshold {
		t.Errorf("ScaleDownThreshold = %f, 期望 %f", got.ScaleDownThreshold, cfg.ScaleDownThreshold)
	}
	if got.CoolDownPeriod != cfg.CoolDownPeriod {
		t.Errorf("CoolDownPeriod = %v, 期望 %v", got.CoolDownPeriod, cfg.CoolDownPeriod)
	}
	if got.CheckInterval != cfg.CheckInterval {
		t.Errorf("CheckInterval = %v, 期望 %v", got.CheckInterval, cfg.CheckInterval)
	}
}
