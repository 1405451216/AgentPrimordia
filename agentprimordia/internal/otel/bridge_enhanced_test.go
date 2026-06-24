package otel

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewEnhancedBridge(t *testing.T) {
	cfg := BridgeConfig{
		ServiceName: "test-service",
	}
	b := NewEnhancedBridge(cfg)
	if b == nil {
		t.Fatal("NewEnhancedBridge should not return nil")
	}
	if b.tracer == nil {
		t.Error("tracer should be initialized")
	}
	if b.metrics == nil {
		t.Error("metrics should be initialized")
	}
}

func TestEnhancedBridge_StartSpanWithMetrics(t *testing.T) {
	b := NewEnhancedBridge(BridgeConfig{ServiceName: "test"})

	ctx, span := b.StartSpanWithMetrics(context.Background(), "test-op", map[string]string{
		"service": "test-service",
	})
	if span == nil {
		t.Fatal("span should not be nil")
	}
	if ctx == nil {
		t.Fatal("context should not be nil")
	}

	// 验证 span 属性
	otelSpan, ok := span.(*otelSpan)
	if !ok {
		t.Fatal("span should be *otelSpan")
	}
	attrs := otelSpan.Attributes()
	if attrs["service"] != "test-service" {
		t.Errorf("expected service=test-service, got %v", attrs["service"])
	}

	span.End()
}

func TestEnhancedBridge_FinishSpanWithMetrics(t *testing.T) {
	b := NewEnhancedBridge(BridgeConfig{ServiceName: "test"})

	_, span := b.StartSpanWithMetrics(context.Background(), "test-op", nil)

	// 正常结束
	b.FinishSpanWithMetrics(span, nil)

	otelSpan := span.(*otelSpan)
	if !otelSpan.IsEnded() {
		t.Error("span should be ended")
	}

	// 验证指标被记录
	b.metrics.mu.Lock()
	if len(b.metrics.counters) == 0 {
		t.Error("should have recorded span count metric")
	}
	b.metrics.mu.Unlock()
}

func TestEnhancedBridge_FinishSpanWithMetrics_WithError(t *testing.T) {
	b := NewEnhancedBridge(BridgeConfig{ServiceName: "test"})

	_, span := b.StartSpanWithMetrics(context.Background(), "failing-op", nil)

	// 带错误结束
	b.FinishSpanWithMetrics(span, errors.New("something went wrong"))

	otelSpan := span.(*otelSpan)
	if !otelSpan.IsEnded() {
		t.Error("span should be ended")
	}

	errs := otelSpan.Errors()
	if len(errs) == 0 {
		t.Error("should have recorded error")
	}
}

func TestEnhancedBridge_RecordLLMMetrics(t *testing.T) {
	b := NewEnhancedBridge(BridgeConfig{ServiceName: "test"})

	// 记录成功的 LLM 调用
	b.RecordLLMMetrics("openai", "gpt-4", 500*time.Millisecond, 1500, nil)

	b.metrics.mu.Lock()
	// 应该有 LLM 调用计数器
	foundCall := false
	foundDuration := false
	for _, c := range b.metrics.counters {
		if c.Name == "ap_llm_calls" {
			foundCall = true
			if c.Value != 1 {
				t.Errorf("expected 1 call, got %f", c.Value)
			}
		}
		if c.Name == "ap_llm_tokens" {
			foundCall = true
		}
	}
	for _, h := range b.metrics.histograms {
		if h.Name == "ap_llm_duration_ms" {
			foundDuration = true
		}
	}
	b.metrics.mu.Unlock()

	if !foundCall {
		t.Error("should have recorded LLM call counter")
	}
	if !foundDuration {
		t.Error("should have recorded LLM duration histogram")
	}
}

func TestEnhancedBridge_RecordLLMMetrics_WithError(t *testing.T) {
	b := NewEnhancedBridge(BridgeConfig{ServiceName: "test"})

	// 记录失败的 LLM 调用
	b.RecordLLMMetrics("openai", "gpt-4", 100*time.Millisecond, 0, errors.New("timeout"))

	b.metrics.mu.Lock()
	foundError := false
	for _, c := range b.metrics.counters {
		if c.Name == "ap_llm_errors" {
			foundError = true
			if c.Value != 1 {
				t.Errorf("expected 1 error, got %f", c.Value)
			}
		}
	}
	b.metrics.mu.Unlock()

	if !foundError {
		t.Error("should have recorded LLM error counter")
	}
}

func TestEnhancedBridge_RecordToolMetrics(t *testing.T) {
	b := NewEnhancedBridge(BridgeConfig{ServiceName: "test"})

	// 记录成功的工具调用
	b.RecordToolMetrics("filesystem", 200*time.Millisecond, nil)

	b.metrics.mu.Lock()
	foundCall := false
	foundDuration := false
	for _, c := range b.metrics.counters {
		if c.Name == "ap_tool_calls" {
			foundCall = true
			if c.Value != 1 {
				t.Errorf("expected 1 call, got %f", c.Value)
			}
		}
	}
	for _, h := range b.metrics.histograms {
		if h.Name == "ap_tool_duration_ms" {
			foundDuration = true
		}
	}
	b.metrics.mu.Unlock()

	if !foundCall {
		t.Error("should have recorded tool call counter")
	}
	if !foundDuration {
		t.Error("should have recorded tool duration histogram")
	}
}

func TestEnhancedBridge_RecordToolMetrics_WithError(t *testing.T) {
	b := NewEnhancedBridge(BridgeConfig{ServiceName: "test"})

	// 记录失败的工具调用
	b.RecordToolMetrics("shell", 50*time.Millisecond, errors.New("permission denied"))

	b.metrics.mu.Lock()
	foundError := false
	for _, c := range b.metrics.counters {
		if c.Name == "ap_tool_errors" {
			foundError = true
		}
	}
	b.metrics.mu.Unlock()

	if !foundError {
		t.Error("should have recorded tool error counter")
	}
}

func TestEnhancedBridge_Integration(t *testing.T) {
	b := NewEnhancedBridge(BridgeConfig{ServiceName: "integration-test"})

	// 模拟一次完整的 LLM + 工具调用流程
	ctx := context.Background()
	ctx, llmSpan := b.StartSpanWithMetrics(ctx, "llm.invoke", map[string]string{
		"provider": "openai",
		"model":    "gpt-4",
	})

	b.RecordLLMMetrics("openai", "gpt-4", 800*time.Millisecond, 2000, nil)

	_, toolSpan := b.StartSpanWithMetrics(ctx, "tool.execute", map[string]string{
		"tool": "filesystem",
	})

	b.RecordToolMetrics("filesystem", 100*time.Millisecond, nil)
	b.FinishSpanWithMetrics(toolSpan, nil)
	b.FinishSpanWithMetrics(llmSpan, nil)

	// 验证 Prometheus 导出
	output := b.metrics.ExportPrometheus()
	if len(output) == 0 {
		t.Error("should have Prometheus output after integration flow")
	}
}
