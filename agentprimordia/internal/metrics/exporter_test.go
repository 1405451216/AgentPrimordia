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
