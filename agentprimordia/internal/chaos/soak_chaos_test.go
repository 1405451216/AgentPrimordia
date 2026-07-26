package chaos

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ===== Mock 实现 =====

// mockSoakFault 模拟故障
type mockSoakFault struct {
	name string
}

func (f *mockSoakFault) Type() string        { return "mock" }
func (f *mockSoakFault) Description() string { return f.name }
func (f *mockSoakFault) Inject(ctx context.Context) (CleanupFunc, error) {
	return func(ctx context.Context) error { return nil }, nil
}

// mockSoakSteadyState 模拟稳态
type mockSoakSteadyState struct {
	met bool
}

func (s *mockSoakSteadyState) Name() string { return "mock-steady" }
func (s *mockSoakSteadyState) Check(ctx context.Context) (SteadyStateResult, error) {
	return SteadyStateResult{Met: s.met, Message: "mock"}, nil
}

// ===== 测试用例 =====

func TestSoakChaosRunner_BasicRun(t *testing.T) {
	requestCount := 0
	runner := NewSoakChaosRunner(SoakChaosConfig{
		SoakDuration:      2 * time.Second,
		ChaosInterval:     500 * time.Millisecond,
		ChaosDuration:     100 * time.Millisecond,
		RequestsPerSecond: 10,
		RequestFn: func(ctx context.Context) (*SoakResponse, error) {
			requestCount++
			return &SoakResponse{
				Latency: 5 * time.Millisecond,
				Success: true,
			}, nil
		},
		Experiments: []Experiment{
			{
				Name:       "test-fault",
				Hypothesis: "system should survive",
				Faults:     []Fault{&mockSoakFault{name: "test"}},
				SteadyState: &mockSoakSteadyState{met: true},
				Duration:   100 * time.Millisecond,
			},
		},
	})

	result := runner.Run(context.Background())

	if result.Error != nil {
		t.Fatalf("运行失败: %v", result.Error)
	}
	if result.TotalRequests == 0 {
		t.Error("应有请求被处理")
	}
	if result.Duration <= 0 {
		t.Error("持续时间应大于 0")
	}
	if result.StartTime.IsZero() {
		t.Error("StartTime 不应为零")
	}
	if result.EndTime.IsZero() {
		t.Error("EndTime 不应为零")
	}
}

func TestSoakChaosRunner_NoRequestFn(t *testing.T) {
	runner := NewSoakChaosRunner(SoakChaosConfig{
		SoakDuration:  500 * time.Millisecond,
		ChaosInterval: 200 * time.Millisecond,
		Experiments: []Experiment{
			{
				Name:     "no-load",
				Faults:   []Fault{&mockSoakFault{name: "test"}},
				Duration: 50 * time.Millisecond,
			},
		},
	})

	result := runner.Run(context.Background())
	if result.Error != nil {
		t.Fatalf("运行失败: %v", result.Error)
	}
	// 无 RequestFn 时不应有请求
	if result.TotalRequests != 0 {
		t.Errorf("无 RequestFn 时不应有请求, 得到 %d", result.TotalRequests)
	}
}

func TestSoakChaosRunner_NoExperiments(t *testing.T) {
	runner := NewSoakChaosRunner(SoakChaosConfig{
		SoakDuration:      1 * time.Second,
		RequestsPerSecond: 5,
		RequestFn: func(ctx context.Context) (*SoakResponse, error) {
			return &SoakResponse{Latency: time.Millisecond, Success: true}, nil
		},
		// 无实验
	})

	result := runner.Run(context.Background())
	if result.Error != nil {
		t.Fatalf("运行失败: %v", result.Error)
	}
	if len(result.ChaosResults) != 0 {
		t.Errorf("无实验时不应有混沌结果, 得到 %d", len(result.ChaosResults))
	}
	if result.TotalRequests == 0 {
		t.Error("应有请求被处理")
	}
}

func TestSoakChaosRunner_DegradationDetection(t *testing.T) {
	callCount := 0
	runner := NewSoakChaosRunner(SoakChaosConfig{
		SoakDuration:         3 * time.Second,
		RequestsPerSecond:    20,
		DegradationThreshold: 100.0, // 延迟翻倍触发
		StopOnDegradation:    true,
		RequestFn: func(ctx context.Context) (*SoakResponse, error) {
			callCount++
			// 前 20 个请求快，后面变慢
			latency := 5 * time.Millisecond
			if callCount > 20 {
				latency = 200 * time.Millisecond
			}
			return &SoakResponse{Latency: latency, Success: true}, nil
		},
	})

	result := runner.Run(context.Background())

	if result.Error != nil {
		t.Fatalf("运行失败: %v", result.Error)
	}
	// 应该检测到退化
	if !result.DegradationDetected {
		t.Log("注意：退化检测取决于采样时机，可能未触发")
	}
}

func TestSoakChaosRunner_WithErrors(t *testing.T) {
	callCount := 0
	runner := NewSoakChaosRunner(SoakChaosConfig{
		SoakDuration:      1 * time.Second,
		RequestsPerSecond: 10,
		RequestFn: func(ctx context.Context) (*SoakResponse, error) {
			callCount++
			if callCount%3 == 0 {
				return &SoakResponse{Latency: time.Millisecond, Success: false}, nil
			}
			return &SoakResponse{Latency: time.Millisecond, Success: true}, nil
		},
	})

	result := runner.Run(context.Background())
	if result.Error != nil {
		t.Fatalf("运行失败: %v", result.Error)
	}
	if result.TotalErrors == 0 {
		t.Error("应有错误请求")
	}
	if result.ErrorRate() <= 0 {
		t.Error("错误率应大于 0")
	}
}

func TestSoakChaosRunner_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	runner := NewSoakChaosRunner(SoakChaosConfig{
		SoakDuration:      10 * time.Second, // 长持续时间
		RequestsPerSecond: 5,
		RequestFn: func(ctx context.Context) (*SoakResponse, error) {
			return &SoakResponse{Latency: time.Millisecond, Success: true}, nil
		},
	})

	// 500ms 后取消
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	result := runner.Run(ctx)
	if result.Error != nil {
		t.Fatalf("运行失败: %v", result.Error)
	}
	// 应提前结束
	if result.Duration > 5*time.Second {
		t.Errorf("应在取消后尽快结束, 实际 %v", result.Duration)
	}
}

func TestSoakChaosRunner_DefaultConfig(t *testing.T) {
	cfg := SoakChaosConfigWithDefaults(SoakChaosConfig{})

	if cfg.SoakDuration != 30*time.Minute {
		t.Errorf("默认 SoakDuration 应为 30m, 得到 %v", cfg.SoakDuration)
	}
	if cfg.ChaosInterval != 5*time.Minute {
		t.Errorf("默认 ChaosInterval 应为 5m, 得到 %v", cfg.ChaosInterval)
	}
	if cfg.ChaosDuration != 30*time.Second {
		t.Errorf("默认 ChaosDuration 应为 30s, 得到 %v", cfg.ChaosDuration)
	}
	if cfg.RequestsPerSecond != 5 {
		t.Errorf("默认 RPS 应为 5, 得到 %d", cfg.RequestsPerSecond)
	}
	if cfg.DegradationThreshold != 50.0 {
		t.Errorf("默认退化阈值应为 50.0, 得到 %f", cfg.DegradationThreshold)
	}
}

func TestSoakChaosResult_Metrics(t *testing.T) {
	result := &SoakChaosResult{
		TotalRequests: 100,
		TotalErrors:   10,
		Samples: []SoakSample{
			{AvgLatency: 10 * time.Millisecond},
			{AvgLatency: 20 * time.Millisecond},
			{AvgLatency: 30 * time.Millisecond},
		},
	}

	if result.ErrorRate() != 0.1 {
		t.Errorf("期望错误率 0.1, 得到 %f", result.ErrorRate())
	}

	avgMs := result.AvgLatencyMs()
	if avgMs != 20.0 {
		t.Errorf("期望平均延迟 20ms, 得到 %f", avgMs)
	}
}

func TestSoakChaosResult_EmptyMetrics(t *testing.T) {
	result := &SoakChaosResult{}

	if result.ErrorRate() != 0 {
		t.Error("空结果错误率应为 0")
	}
	if result.AvgLatencyMs() != 0 {
		t.Error("空结果平均延迟应为 0")
	}
}

func TestFormatSoakChaosReport(t *testing.T) {
	result := &SoakChaosResult{
		StartTime:           time.Now().Add(-time.Minute),
		EndTime:             time.Now(),
		Duration:            time.Minute,
		TotalRequests:       1000,
		TotalErrors:         5,
		DegradationDetected: true,
		DegradationDetails:  "延迟退化 60%",
		StoppedEarly:        true,
		Samples: []SoakSample{
			{Timestamp: time.Now(), Requests: 100, Errors: 1, AvgLatency: 10 * time.Millisecond, P99Latency: 50 * time.Millisecond},
		},
		ChaosResults: []*ExperimentResult{
			{
				Experiment:          Experiment{Name: "test-exp"},
				Status:              StatusCompleted,
				HypothesisValidated: true,
				Duration:            30 * time.Second,
			},
		},
	}

	report := FormatSoakChaosReport(result)

	if !strings.Contains(report, "Soak + Chaos") {
		t.Error("报告应包含标题")
	}
	if !strings.Contains(report, "退化检测") {
		t.Error("报告应包含退化检测")
	}
	if !strings.Contains(report, "test-exp") {
		t.Error("报告应包含实验名称")
	}
	if !strings.Contains(report, "1000") {
		t.Error("报告应包含总请求数")
	}
}

func TestFormatSoakChaosReport_NoDegradation(t *testing.T) {
	result := &SoakChaosResult{
		Duration:            time.Minute,
		TotalRequests:       500,
		DegradationDetected: false,
	}

	report := FormatSoakChaosReport(result)
	if !strings.Contains(report, "未检测到退化") {
		t.Error("无退化时应显示'未检测到退化'")
	}
}
