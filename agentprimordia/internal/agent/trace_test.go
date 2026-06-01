package agent

import (
	"strings"
	"testing"
	"time"
)

func TestSpanContext(t *testing.T) {
	sc := SpanContext{
		TraceID: "trace-123",
		SpanID:  "span-456",
	}
	if sc.TraceID != "trace-123" {
		t.Error("TraceID mismatch")
	}
	if sc.SpanID != "span-456" {
		t.Error("SpanID mismatch")
	}
}

func TestSpanContext_IsValid(t *testing.T) {
	valid := SpanContext{TraceID: "t1", SpanID: "s1"}
	if !valid.IsValid() {
		t.Error("non-empty TraceID and SpanID should be valid")
	}

	emptyTrace := SpanContext{TraceID: "", SpanID: "s1"}
	if emptyTrace.IsValid() {
		t.Error("empty TraceID should be invalid")
	}

	emptySpan := SpanContext{TraceID: "t1", SpanID: ""}
	if emptySpan.IsValid() {
		t.Error("empty SpanID should be invalid")
	}
}

func TestNoopSpan(t *testing.T) {
	span := NoopSpan{}

	span.SetName("test")
	span.SetAttribute("key", "value")
	span.SetStatus(SpanStatusOK, "")
	span.End()

	if span.IsEnded() {
		t.Error("NoopSpan.IsEnded should always return false")
	}
	if span.SpanContext().TraceID != "" {
		t.Error("NoopSpan should have empty SpanContext")
	}
}

func TestLoggingSpan_Basic(t *testing.T) {
	tracer := NewLoggingTracer()
	span := tracer.Start("test-operation", SpanKindInternal)

	if span.SpanContext().TraceID == "" {
		t.Error("TraceID should not be empty")
	}
	if span.SpanContext().SpanID == "" {
		t.Error("SpanID should not be empty")
	}
	if span.IsEnded() {
		t.Error("span should not be ended yet")
	}

	span.SetName("renamed")
	span.SetAttribute("key1", "value1")
	span.SetAttribute("key2", 42)
	span.SetStatus(SpanStatusOK, "")
	span.End()

	if !span.IsEnded() {
		t.Error("span should be ended after End()")
	}
}

func TestLoggingSpan_ChildSpan(t *testing.T) {
	tracer := NewLoggingTracer()
	parent := tracer.Start("parent", SpanKindServer)

	childCtx := parent.SpanContext()
	child := tracer.Start("child", SpanKindClient, WithParent(childCtx))

	if child.SpanContext().TraceID != childCtx.TraceID {
		t.Error("child TraceID should match parent")
	}
	if child.SpanContext().SpanID == childCtx.SpanID {
		t.Error("child SpanID should differ from parent")
	}

	child.End()
	parent.End()
}

func TestLoggingTracer(t *testing.T) {
	tracer := NewLoggingTracer()

	span := tracer.Start("operation", SpanKindInternal)
	if span == nil {
		t.Fatal("Start should return non-nil span")
	}

	span.End()

	output := tracer.String()
	if !strings.Contains(output, "operation") {
		t.Errorf("tracer output should contain operation name, got: %s", output)
	}
}

func TestLoggingTracer_MultipleSpans(t *testing.T) {
	tracer := NewLoggingTracer()

	s1 := tracer.Start("op1", SpanKindInternal)
	s2 := tracer.Start("op2", SpanKindClient)

	s1.End()
	s2.End()

	output := tracer.String()
	if !strings.Contains(output, "op1") {
		t.Error("should contain op1")
	}
	if !strings.Contains(output, "op2") {
		t.Error("should contain op2")
	}
}

func TestSpanKind_Constants(t *testing.T) {
	if SpanKindInternal != "internal" {
		t.Errorf("SpanKindInternal = %q, want internal", SpanKindInternal)
	}
	if SpanKindClient != "client" {
		t.Errorf("SpanKindClient = %q, want client", SpanKindClient)
	}
	if SpanKindServer != "server" {
		t.Errorf("SpanKindServer = %q, want server", SpanKindServer)
	}
}

func TestSpanStatus_Constants(t *testing.T) {
	if SpanStatusOK != "ok" {
		t.Errorf("SpanStatusOK = %q, want ok", SpanStatusOK)
	}
	if SpanStatusError != "error" {
		t.Errorf("SpanStatusError = %q, want error", SpanStatusError)
	}
}

func TestLoggingSpan_Duration(t *testing.T) {
	tracer := NewLoggingTracer()
	span := tracer.Start("timed-op", SpanKindInternal)

	time.Sleep(10 * time.Millisecond)
	span.End()

	ls, ok := span.(*LoggingSpan)
	if !ok {
		t.Fatal("expected LoggingSpan")
	}
	if ls.Duration < 10*time.Millisecond {
		t.Errorf("Duration = %v, want at least 10ms", ls.Duration)
	}
}

func TestLoggingSpan_StatusError(t *testing.T) {
	tracer := NewLoggingTracer()
	span := tracer.Start("error-op", SpanKindInternal)
	span.SetStatus(SpanStatusError, "something went wrong")
	span.End()

	ls, ok := span.(*LoggingSpan)
	if !ok {
		t.Fatal("expected LoggingSpan")
	}
	if ls.Status != SpanStatusError {
		t.Errorf("Status = %q, want error", ls.Status)
	}
}

func TestWithParent_Option(t *testing.T) {
	parentCtx := SpanContext{TraceID: "parent-trace", SpanID: "parent-span"}
	opt := WithParent(parentCtx)

	cfg := &SpanConfig{}
	opt(cfg)

	if cfg.ParentContext.TraceID != "parent-trace" {
		t.Error("WithParent should set ParentContext")
	}
}

func TestWithAttributes_Option(t *testing.T) {
	opt := WithAttributes(map[string]any{"key": "value", "num": 123})

	cfg := &SpanConfig{}
	opt(cfg)

	if cfg.Attributes["key"] != "value" {
		t.Error("WithAttributes should set attributes")
	}
}

func TestLoggingTracer_Reset(t *testing.T) {
	tracer := NewLoggingTracer()
	span := tracer.Start("op", SpanKindInternal)
	span.End()

	tracer.Reset()

	output := tracer.String()
	if strings.Contains(output, "op") {
		t.Error("after Reset, tracer should not contain previous spans")
	}
}

func TestSpanContext_W3CTraceParent(t *testing.T) {
	sc := SpanContext{
		TraceID:    "0af7651916cd43dd8448eb211c80319c",
		SpanID:     "b7ad6b7169203331",
		TraceFlags: 1,
	}
	parent := sc.ToW3CTraceParent()
	expected := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	if parent != expected {
		t.Errorf("ToW3CTraceParent = %q, want %q", parent, expected)
	}
}

func TestSpanContext_ToW3CTraceParent_Unsampled(t *testing.T) {
	sc := SpanContext{
		TraceID:    "0af7651916cd43dd8448eb211c80319c",
		SpanID:     "b7ad6b7169203331",
		TraceFlags: 0,
	}
	parent := sc.ToW3CTraceParent()
	expected := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00"
	if parent != expected {
		t.Errorf("unsampled = %q, want %q", parent, expected)
	}
}

func TestSpanContext_FromW3CTraceParent(t *testing.T) {
	input := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	sc, err := FromW3CTraceParent(input)
	if err != nil {
		t.Fatalf("FromW3CTraceParent error: %v", err)
	}
	if sc.TraceID != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("TraceID = %q, want 0af7651916cd43dd8448eb211c80319c", sc.TraceID)
	}
	if sc.SpanID != "b7ad6b7169203331" {
		t.Errorf("SpanID = %q, want b7ad6b7169203331", sc.SpanID)
	}
	if sc.TraceFlags != 1 {
		t.Errorf("TraceFlags = %d, want 1", sc.TraceFlags)
	}
	if sc.Remote {
		t.Error("Remote should be false for parsed traceparent")
	}
}

func TestSpanContext_FromW3CTraceParent_Invalid(t *testing.T) {
	cases := []string{
		"invalid",
		"01-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
		"00-short-b7ad6b7169203331-01",
		"00-0af7651916cd43dd8448eb211c80319c-short-01",
	}
	for _, tc := range cases {
		_, err := FromW3CTraceParent(tc)
		if err == nil {
			t.Errorf("should return error for %q", tc)
		}
	}
}

func TestSpanContext_WithTraceState(t *testing.T) {
	sc := SpanContext{TraceID: "abc", SpanID: "123"}
	sc2 := sc.WithTraceState("vendor", "value")
	if sc2.TraceState["vendor"] != "value" {
		t.Error("WithTraceState should add key-value pair")
	}
	if len(sc.TraceState) != 0 {
		t.Error("original SpanContext should not be modified")
	}
}
