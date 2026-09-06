package otel

import (
	"fmt"
	"testing"
)

func TestBridge_IsEnabled(t *testing.T) {
	if !BridgeEnabled {
		t.Error("BridgeEnabled should be true (unified implementation)")
	}
}

func TestBridge_StartSpanAndEnd(t *testing.T) {
	b := NewOTelBridge()
	span := b.StartSpan("test-op")
	span.SetAttribute("http.method", "POST")
	span.SetAttribute("http.status_code", 200)
	span.SetStatus("ok", "")
	span.AddEvent("request_sent", map[string]any{"url": "https://api.example.com"})
	span.End()

	if !span.(*otelSpan).IsEnded() {
		t.Error("span should be ended")
	}

	ctx := span.SpanContext()
	if ctx["trace_id"] == "" || ctx["span_id"] == "" || ctx["span_id"] == "noop" {
		t.Errorf("span context should have real IDs, got: %v", ctx)
	}

	if err := b.Shutdown(); err != nil {
		t.Errorf("Shutdown error: %v", err)
	}
}

func TestBridge_ParentChildSpan(t *testing.T) {
	b := NewOTelBridge()
	parent := b.StartSpan("parent-op")
	child := b.StartSpanWithParent("child-op", parent)

	parentCtx := parent.SpanContext()
	childCtx := child.SpanContext()

	if childCtx["parent_span_id"] != parentCtx["span_id"] {
		t.Errorf("child parent_span_id = %q, want %q", childCtx["parent_span_id"], parentCtx["span_id"])
	}
	if childCtx["trace_id"] != parentCtx["trace_id"] {
		t.Errorf("child trace_id = %q, want %q", childCtx["trace_id"], parentCtx["trace_id"])
	}

	child.End()
	parent.End()
	_ = b.Shutdown()
}

func TestBridge_RecordError(t *testing.T) {
	b := NewOTelBridge()
	span := b.StartSpan("error-op")
	span.RecordError(fmt.Errorf("test error"))
	span.RecordError(fmt.Errorf("another error"))
	span.End()

	s := span.(*otelSpan)
	if len(s.Errors()) != 2 {
		t.Errorf("expected 2 errors, got %d", len(s.Errors()))
	}
	_ = b.Shutdown()
}

func TestBridge_FlushSpans(t *testing.T) {
	b := NewOTelBridge()
	b.StartSpan("op1").End()
	b.StartSpan("op2").End()

	if b.SpanCount() != 2 {
		t.Errorf("expected 2 spans, got %d", b.SpanCount())
	}

	spans := b.FlushSpans()
	if len(spans) != 2 {
		t.Errorf("expected 2 flushed spans, got %d", len(spans))
	}
	if b.SpanCount() != 0 {
		t.Errorf("expected 0 after flush, got %d", b.SpanCount())
	}
	_ = b.Shutdown()
}

func TestBridge_W3CTraceParent(t *testing.T) {
	b := NewOTelBridge()
	span := b.StartSpan("w3c-op")
	tp := span.(*otelSpan).W3CTraceParent()

	if len(tp) < 35 { // "00-{32}-{16}-00" = 55 chars
		t.Errorf("W3C traceparent too short: %q", tp)
	}
	span.End()
	_ = b.Shutdown()
}
