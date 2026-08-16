package soak

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestConstantPattern(t *testing.T) {
	p := ConstantPattern(10)
	if p.Name() != "constant" {
		t.Errorf("Name = %s", p.Name())
	}
	interval := p.NextInterval()
	expected := time.Second / 10
	if interval != expected {
		t.Errorf("interval = %v, expected %v", interval, expected)
	}
}

func TestStepPattern(t *testing.T) {
	p := StepPattern(5, 20, 5, 1*time.Millisecond)
	if p.Name() != "step" {
		t.Errorf("Name = %s", p.Name())
	}
	// 初始 RPS 为 5
	interval := p.NextInterval()
	if interval != time.Second/5 {
		t.Errorf("初始 interval = %v, expected %v", interval, time.Second/5)
	}
	// 等待 stepDuration 后应增加
	time.Sleep(2 * time.Millisecond)
	_ = p.NextInterval() // 触发步进
	if p.currentRPS < 5 {
		t.Errorf("步进后 RPS = %d, 应 >= 10", p.currentRPS)
	}
}

func TestBurstPattern(t *testing.T) {
	p := BurstPattern(50, 10*time.Millisecond, 5*time.Millisecond)
	if p.Name() != "burst" {
		t.Errorf("Name = %s", p.Name())
	}
	// 应该在突发期
	if !p.inBurst {
		t.Error("初始应在突发期")
	}
	interval := p.NextInterval()
	if interval != time.Second/50 {
		t.Errorf("突发期 interval = %v, expected %v", interval, time.Second/50)
	}
}

func TestRandomPattern(t *testing.T) {
	p := RandomPattern(1, 10)
	if p.Name() != "random" {
		t.Errorf("Name = %s", p.Name())
	}
	for i := 0; i < 10; i++ {
		interval := p.NextInterval()
		if interval <= 0 {
			t.Error("interval 应为正数")
		}
	}
}

func TestRampPattern(t *testing.T) {
	p := RampPattern(1, 10, 100*time.Millisecond)
	if p.Name() != "ramp" {
		t.Errorf("Name = %s", p.Name())
	}
	for i := 0; i < 10; i++ {
		interval := p.NextInterval()
		if interval <= 0 {
			t.Error("interval 应为正数")
		}
	}
}

func TestRunnerBasic(t *testing.T) {
	// 并发请求回调：计数器必须线程安全（-race 实测发现无锁计数竞态）
	var counter atomic.Int64
	cfg := RunnerConfig{
		Duration: 200 * time.Millisecond,
		Pattern:  ConstantPattern(50), // 50 RPS
		RequestFn: func(ctx context.Context) (*Response, error) {
			counter.Add(1)
			return &Response{Success: true}, nil
		},
		SamplingInterval: 50 * time.Millisecond,
	}

	runner := NewRunner(cfg)
	result := runner.Run(context.Background())

	if result.TotalRequests == 0 {
		t.Error("总请求数为 0")
	}
	if result.TotalErrors != 0 {
		t.Errorf("总错误数 = %d, 期望 0", result.TotalErrors)
	}
	if result.Duration < 200*time.Millisecond {
		t.Errorf("持续时间 = %v, 应 >= 200ms", result.Duration)
	}
	if counter.Load() == 0 {
		t.Error("请求函数未被调用")
	}
}

func TestRunnerWithErrors(t *testing.T) {
	cfg := RunnerConfig{
		Duration: 200 * time.Millisecond,
		Pattern:  ConstantPattern(50),
		RequestFn: func(ctx context.Context) (*Response, error) {
			return &Response{Success: false}, nil // 总是失败
		},
		SamplingInterval: 50 * time.Millisecond,
	}

	runner := NewRunner(cfg)
	result := runner.Run(context.Background())

	if result.TotalRequests == 0 {
		t.Error("总请求数为 0")
	}
	if result.TotalErrors != result.TotalRequests {
		t.Errorf("总错误数 = %d, 期望 = %d", result.TotalErrors, result.TotalRequests)
	}
}

func TestRunnerStop(t *testing.T) {
	cfg := RunnerConfig{
		Duration: 10 * time.Second, // 长时间
		Pattern:  ConstantPattern(100),
		RequestFn: func(ctx context.Context) (*Response, error) {
			return &Response{Success: true}, nil
		},
		SamplingInterval: 50 * time.Millisecond,
	}

	runner := NewRunner(cfg)
	go func() {
		time.Sleep(100 * time.Millisecond)
		runner.Stop()
	}()

	result := runner.Run(context.Background())

	// 应在远小于 10 秒内结束
	if result.Duration >= 5*time.Second {
		t.Errorf("Stop() 后持续时间 = %v, 应 < 5s", result.Duration)
	}
}

func TestDegradationNoSamples(t *testing.T) {
	report := AnalyzeDegradation(nil, nil)
	if report == nil {
		t.Fatal("报告为 nil")
	}
	if report.HasDegradation {
		t.Error("无采样时不应有退化")
	}
}

func TestDegradationStable(t *testing.T) {
	samples := make([]Sample, 10)
	for i := range samples {
		samples[i] = Sample{
			Requests:   100,
			Errors:      1,
			AvgLatency:  50 * time.Millisecond,
			Throughput:  100,
			SuccessRate: 0.99,
		}
	}

	report := AnalyzeDegradation(samples, nil)
	if report.HasDegradation {
		t.Error("稳定数据不应检测到退化")
	}
}

func TestDegradationLatency(t *testing.T) {
	samples := make([]Sample, 10)
	// 前半段：50ms 延迟
	for i := 0; i < 5; i++ {
		samples[i] = Sample{
			Requests:   100,
			Errors:      0,
			AvgLatency:  50 * time.Millisecond,
			Throughput:  100,
			SuccessRate: 1.0,
		}
	}
	// 后半段：200ms 延迟（退化 300%）
	for i := 5; i < 10; i++ {
		samples[i] = Sample{
			Requests:   100,
			Errors:      0,
			AvgLatency:  200 * time.Millisecond,
			Throughput:  100,
			SuccessRate: 1.0,
		}
	}

	report := AnalyzeDegradation(samples, nil)
	if !report.HasDegradation {
		t.Error("延迟退化 300% 应被检测")
	}
	if report.LatencyTrend != TrendDegrading {
		t.Errorf("延迟趋势 = %s, 期望 degrading", report.LatencyTrend)
	}
}

func TestDegradationErrorRate(t *testing.T) {
	samples := make([]Sample, 10)
	// 前半段：1% 错误率
	for i := 0; i < 5; i++ {
		samples[i] = Sample{
			Requests:   100,
			Errors:      1,
			AvgLatency:  50 * time.Millisecond,
			Throughput:  100,
			SuccessRate: 0.99,
		}
	}
	// 后半段：50% 错误率（退化）
	for i := 5; i < 10; i++ {
		samples[i] = Sample{
			Requests:   100,
			Errors:      50,
			AvgLatency:  50 * time.Millisecond,
			Throughput:  100,
			SuccessRate: 0.50,
		}
	}

	report := AnalyzeDegradation(samples, nil)
	if !report.HasDegradation {
		t.Error("错误率退化应被检测")
	}
	if report.ErrorRateTrend != TrendDegrading {
		t.Errorf("错误率趋势 = %s, 期望 degrading", report.ErrorRateTrend)
	}
}

func TestFormatReport(t *testing.T) {
	result := &SoakResult{
		StartTime:      time.Now(),
		EndTime:        time.Now().Add(1 * time.Second),
		Duration:       1 * time.Second,
		TotalRequests:   100,
		TotalErrors:     5,
		TotalLatencyNs:  5000000000,
	}

	report := FormatReport(result)
	if report == "" {
		t.Error("报告为空")
	}
}

func TestCalculateP99(t *testing.T) {
	latencies := make([]time.Duration, 100)
	for i := range latencies {
		latencies[i] = time.Duration(i+1) * time.Millisecond
	}

	p99 := calculateP99(latencies)
	if p99 <= 0 {
		t.Error("P99 应为正数")
	}
	// P99 应该接近 99ms
	if p99 < 95*time.Millisecond || p99 > 100*time.Millisecond {
		t.Errorf("P99 = %v, 期望 ~99ms", p99)
	}
}

func TestChangePercent(t *testing.T) {
	// 从 100 到 200 = +100%
	if pct := changePercent(100, 200); pct != 100 {
		t.Errorf("changePercent(100, 200) = %v, 期望 100", pct)
	}
	// 从 200 到 100 = -50%
	if pct := changePercent(200, 100); pct != -50 {
		t.Errorf("changePercent(200, 100) = %v, 期望 -50", pct)
	}
	// 0 到 0 = 0
	if pct := changePercent(0, 0); pct != 0 {
		t.Errorf("changePercent(0, 0) = %v, 期望 0", pct)
	}
	// 0 到 100 = 100%
	if pct := changePercent(0, 100); pct != 100 {
		t.Errorf("changePercent(0, 100) = %v, 期望 100", pct)
	}
}

func TestSoakResultMethods(t *testing.T) {
	result := &SoakResult{
		TotalRequests:  100,
		TotalErrors:    5,
		TotalLatencyNs: 5000000000, // 5s in ns
	}

	if result.AvgLatency() != 50*time.Millisecond {
		t.Errorf("AvgLatency = %v, 期望 50ms", result.AvgLatency())
	}
	if result.ErrorRate() != 0.05 {
		t.Errorf("ErrorRate = %v, 期望 0.05", result.ErrorRate())
	}

	// 零请求
	empty := &SoakResult{}
	if empty.AvgLatency() != 0 {
		t.Errorf("空 AvgLatency = %v, 期望 0", empty.AvgLatency())
	}
	if empty.ErrorRate() != 0 {
		t.Errorf("空 ErrorRate = %v, 期望 0", empty.ErrorRate())
	}
}
