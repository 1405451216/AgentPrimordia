package observability

import (
	"testing"
	"time"
)

// TestAlertEngine_FireAboveThreshold 验证指标超过阈值时触发告警。
func TestAlertEngine_FireAboveThreshold(t *testing.T) {
	store := NewCorrelationStore()
	// 模拟 10 个请求，产生较高的错误率
	for i := 0; i < 10; i++ {
		traceID := "trace-" + string(rune('a'+i))
		store.Start(traceID, "agent-a", "sess-1")
		store.RecordTurn(traceID)
		store.End(traceID)
	}

	engine := NewAlertEngine(store)
	// 注册一条规则：当请求总数 > 5 时触发告警
	engine.RegisterRule(NewThresholdAlertRule(ThresholdAlertConfig{
		Name:      "high_request_count",
		Threshold: 5,
		Severity:  SeverityWarning,
		MetricFn: func(store *CorrelationStore) float64 {
			return float64(store.Len())
		},
	}))

	alerts := engine.Evaluate()
	if len(alerts) == 0 {
		t.Fatal("预期触发告警，但未产生任何告警事件")
	}
	if alerts[0].Rule != "high_request_count" {
		t.Errorf("告警规则名 = %q, want high_request_count", alerts[0].Rule)
	}
	if alerts[0].Severity != SeverityWarning {
		t.Errorf("告警严重度 = %q, want warning", alerts[0].Severity)
	}
	if alerts[0].Actual <= 5 {
		t.Errorf("告警实际值 = %f, 应 > 5", alerts[0].Actual)
	}
}

// TestAlertEngine_NoAlertBelowThreshold 验证指标未超过阈值时不触发告警。
func TestAlertEngine_NoAlertBelowThreshold(t *testing.T) {
	store := NewCorrelationStore()
	store.Start("trace-1", "agent-a", "sess-1")
	store.End("trace-1")

	engine := NewAlertEngine(store)
	engine.RegisterRule(NewThresholdAlertRule(ThresholdAlertConfig{
		Name:      "high_request_count",
		Threshold: 5,
		Severity:  SeverityWarning,
		MetricFn: func(store *CorrelationStore) float64 {
			return float64(store.Len())
		},
	}))

	alerts := engine.Evaluate()
	if len(alerts) != 0 {
		t.Errorf("不应触发告警，但产生了 %d 条", len(alerts))
	}
}

// TestAlertEngine_MultipleRules 验证多条规则独立评估。
func TestAlertEngine_MultipleRules(t *testing.T) {
	store := NewCorrelationStore()
	// 创建 3 个请求，每个有 span
	for i := 0; i < 3; i++ {
		traceID := "trace-m-" + string(rune('0'+i))
		store.Start(traceID, "agent-a", "sess-1")
		store.AddSpan(traceID, SpanRecord{Name: "agent.run", SpanID: "s" + string(rune('0'+i))})
		store.End(traceID)
	}

	engine := NewAlertEngine(store)

	// 规则 1：请求数 > 2 时触发
	engine.RegisterRule(NewThresholdAlertRule(ThresholdAlertConfig{
		Name:      "request_count_alert",
		Threshold: 2,
		Severity:  SeverityInfo,
		MetricFn: func(store *CorrelationStore) float64 {
			return float64(store.Len())
		},
	}))

	// 规则 2：span 总数 > 10 时触发（不应触发）
	engine.RegisterRule(NewThresholdAlertRule(ThresholdAlertConfig{
		Name:      "span_count_alert",
		Threshold: 10,
		Severity:  SeverityCritical,
		MetricFn: func(store *CorrelationStore) float64 {
			total := 0
			for _, rt := range store.List(0) {
				total += len(rt.Spans)
			}
			return float64(total)
		},
	}))

	// 规则 3：始终触发（阈值为 -1）
	engine.RegisterRule(NewThresholdAlertRule(ThresholdAlertConfig{
		Name:      "always_fire",
		Threshold: -1,
		Severity:  SeverityInfo,
		MetricFn: func(store *CorrelationStore) float64 {
			return 0
		},
	}))

	alerts := engine.Evaluate()
	// 规则 1 触发 + 规则 3 触发 = 2 条
	if len(alerts) != 2 {
		t.Fatalf("预期 2 条告警，实际 %d 条: %+v", len(alerts), alerts)
	}

	// 验证规则名
	names := map[string]bool{}
	for _, a := range alerts {
		names[a.Rule] = true
	}
	if !names["request_count_alert"] {
		t.Error("缺少 request_count_alert 告警")
	}
	if !names["always_fire"] {
		t.Error("缺少 always_fire 告警")
	}
}

// TestAlertEngine_ConcurrentEvaluate 验证并发评估的安全性。
func TestAlertEngine_ConcurrentEvaluate(t *testing.T) {
	store := NewCorrelationStore()
	store.Start("trace-c", "agent-a", "sess-1")
	store.End("trace-c")

	engine := NewAlertEngine(store)
	engine.RegisterRule(NewThresholdAlertRule(ThresholdAlertConfig{
		Name:      "concurrent_rule",
		Threshold: 100,
		Severity:  SeverityInfo,
		MetricFn: func(store *CorrelationStore) float64 {
			return float64(store.Len())
		},
	}))

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_ = engine.Evaluate()
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestAlertEngine_RuleError 验证规则评估返回错误时不产生告警。
func TestAlertEngine_RuleError(t *testing.T) {
	store := NewCorrelationStore()
	engine := NewAlertEngine(store)

	// 注册一条始终返回错误的规则
	engine.RegisterRule(&errorRule{})

	alerts := engine.Evaluate()
	if len(alerts) != 0 {
		t.Errorf("错误规则不应产生告警，实际 %d 条", len(alerts))
	}
}

// errorRule 始终返回错误的测试用规则。
type errorRule struct{}

func (r *errorRule) Name() string { return "error_rule" }
func (r *errorRule) Evaluate(_ *CorrelationStore) ([]AlertEvent, error) {
	return nil, errTestRule
}

// TestNewAlertEngine 验证构造函数。
func TestNewAlertEngine(t *testing.T) {
	store := NewCorrelationStore()
	engine := NewAlertEngine(store)
	if engine == nil {
		t.Fatal("NewAlertEngine 不应返回 nil")
	}
	// 未注册规则时评估应返回空
	alerts := engine.Evaluate()
	if len(alerts) != 0 {
		t.Errorf("无规则时应返回空告警列表，实际 %d 条", len(alerts))
	}
}

// TestNewThresholdAlertRule 验证阈值规则构造。
func TestNewThresholdAlertRule(t *testing.T) {
	rule := NewThresholdAlertRule(ThresholdAlertConfig{
		Name:      "test_rule",
		Threshold: 10.0,
		Severity:  SeverityCritical,
		MetricFn:  func(_ *CorrelationStore) float64 { return 0 },
	})
	if rule.Name() != "test_rule" {
		t.Errorf("规则名 = %q, want test_rule", rule.Name())
	}
}

// TestThresholdAlertRule_LatencyMetric 验证延迟指标阈值告警。
func TestThresholdAlertRule_LatencyMetric(t *testing.T) {
	store := NewCorrelationStore()
	store.Start("trace-lat", "agent-a", "sess-1")
	store.RecordLLM("trace-lat", 5*time.Second, 100, 200, 0.01)
	store.End("trace-lat")

	rule := NewThresholdAlertRule(ThresholdAlertConfig{
		Name:      "high_latency",
		Threshold: 3000, // 3 秒
		Severity:  SeverityCritical,
		MetricFn: func(store *CorrelationStore) float64 {
			traces := store.List(0)
			if len(traces) == 0 {
				return 0
			}
			return float64(traces[0].Metrics.LLMLatencyMs)
		},
	})

	alerts, err := rule.Evaluate(store)
	if err != nil {
		t.Fatalf("评估失败: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("预期 1 条告警，实际 %d 条", len(alerts))
	}
	if alerts[0].Severity != SeverityCritical {
		t.Errorf("严重度 = %q, want critical", alerts[0].Severity)
	}
}

// errTestRule 测试用错误。
var errTestRule = &testError{}

type testError struct{}

func (e *testError) Error() string { return "test rule error" }
