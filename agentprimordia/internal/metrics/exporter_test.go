package metrics

import (
	"testing"
	"time"
)

func TestPrometheusHandler_MetricsOutput(t *testing.T) {
	m := NewMetrics()
	m.RecordLLMCall(100*time.Millisecond, nil)
	m.RecordToolCall(50*time.Millisecond, nil)
	m.RecordTurn(1 * time.Second)

	output := m.String()
	if output == "" {
		t.Error("metrics string should not be empty")
	}
	// 验证 Prometheus 格式
	if !containsStr(output, "ap_llm_total_calls") {
		t.Error("expected ap_llm_total_calls in output")
	}
	if !containsStr(output, "ap_tool_total_calls") {
		t.Error("expected ap_tool_total_calls in output")
	}
	if !containsStr(output, "# TYPE") {
		t.Error("expected # TYPE in Prometheus output")
	}
	if !containsStr(output, "# HELP") {
		t.Error("expected # HELP in Prometheus output")
	}
}

// perf-v6 round 4 Task 3：Prometheus 文本格式严格验证
func TestPrometheusHandler_FormatStrict(t *testing.T) {
	m := NewMetrics()
	m.RecordLLMCall(100*time.Millisecond, nil)
	m.RecordLLMCall(50*time.Millisecond, nil)
	m.RecordLLMCallWithLabels(200*time.Millisecond, nil, "openai", "gpt-4o")
	m.RecordToolCallWithLabels(30*time.Millisecond, nil, "web_search")
	m.RecordTurn(2 * time.Second)

	output := m.String()

	// 验证 # HELP 注释格式（每条 metric 都应有 HELP）
	requiredHelps := []string{
		"# HELP ap_llm_total_calls",
		"# HELP ap_llm_total_errors",
		"# HELP ap_tool_total_calls",
		"# HELP ap_total_turns",
	}
	for _, h := range requiredHelps {
		if !containsStr(output, h) {
			t.Errorf("missing required HELP: %s", h)
		}
	}

	// 验证 # TYPE 注释（counter/gauge）
	requiredTypes := []string{
		"# TYPE ap_llm_total_calls counter",
		"# TYPE ap_llm_total_errors counter",
		"# TYPE ap_tool_total_calls counter",
		"# TYPE ap_total_turns counter",
	}
	for _, t1 := range requiredTypes {
		if !containsStr(output, t1) {
			t.Errorf("missing required TYPE: %s", t1)
		}
	}

	// 验证标签维度（{provider="openai",model="gpt-4o"}）
	if !containsStr(output, `provider="openai"`) {
		t.Error("missing provider label dimension")
	}
	if !containsStr(output, `model="gpt-4o"`) {
		t.Error("missing model label dimension")
	}
	if !containsStr(output, `tool_name="web_search"`) {
		t.Error("missing tool_name label dimension")
	}

	// 验证数值样本：3 次 LLM call = ap_llm_total_calls 3
	if !containsStr(output, "ap_llm_total_calls 3") {
		t.Error("expected ap_llm_total_calls = 3")
	}
}

func TestLogExporter(t *testing.T) {
	exporter := NewLogExporter()
	snap := MetricsSnapshot{
		LLMTotalCalls:  10,
		LLMTotalErrors: 1,
		ToolTotalCalls: 5,
		ActiveAgents:   2,
	}
	exporter.ExportMetrics(snap)
	exporter.ExportEvent("test.event", "test-source", nil)
	if err := exporter.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestJSONExporter(t *testing.T) {
	exporter := NewJSONExporter(64)
	defer exporter.Close()

	snap := MetricsSnapshot{
		LLMTotalCalls: 3,
		ActiveAgents:  1,
	}
	exporter.ExportMetrics(snap)
	exporter.ExportEvent("test", "src", nil)

	// 验证 channel 有数据
	select {
	case line := <-exporter.Channel():
		if line == "" {
			t.Error("expected non-empty JSON line")
		}
	default:
		t.Error("expected data in channel")
	}
}

func TestMultiExporter(t *testing.T) {
	log1 := NewLogExporter()
	log2 := NewLogExporter()
	multi := NewMultiExporter(log1, log2)
	defer multi.Close()

	snap := MetricsSnapshot{LLMTotalCalls: 5}
	multi.ExportMetrics(snap)
	multi.ExportEvent("evt", "src", nil)
}

func TestMetricsExporter_StartStop(t *testing.T) {
	m := NewMetrics()
	log := NewLogExporter()
	defer log.Close()

	exporter := NewMetricsExporter(m, log, 100*time.Millisecond)
	exporter.Start()

	// 记录一些指标
	m.RecordLLMCall(50*time.Millisecond, nil)

	// 等待至少一次导出
	time.Sleep(200 * time.Millisecond)

	exporter.Stop()
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
