package trace

import (
	"strings"
	"testing"
)

// ===== SpanContext tests =====

func TestSpanContext_IsValid(t *testing.T) {
	sc := SpanContext{TraceID: "abc", SpanID: "def"}
	if !sc.IsValid() {
		t.Error("SpanContext with TraceID and SpanID should be valid")
	}

	sc = SpanContext{TraceID: "", SpanID: "def"}
	if sc.IsValid() {
		t.Error("SpanContext without TraceID should be invalid")
	}

	sc = SpanContext{TraceID: "abc", SpanID: ""}
	if sc.IsValid() {
		t.Error("SpanContext without SpanID should be invalid")
	}
}

func TestSpanContext_ToW3CTraceParent(t *testing.T) {
	sc := SpanContext{TraceID: "trace123", SpanID: "span123", TraceFlags: 0}
	result := sc.ToW3CTraceParent()
	expected := "00-trace123-span123-00"
	if result != expected {
		t.Errorf("ToW3CTraceParent() = %q, want %q", result, expected)
	}

	sc.TraceFlags = 1
	result = sc.ToW3CTraceParent()
	expected = "00-trace123-span123-01"
	if result != expected {
		t.Errorf("ToW3CTraceParent() with flags=1 = %q, want %q", result, expected)
	}
}

func TestSpanContext_FromW3CTraceParent(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		traceID string
		spanID  string
		flags   byte
	}{
		{
			name:    "valid with flags 0",
			input:   "00-abcdef0123456789abcdef0123456789-1234567890abcdef-00",
			wantErr: false,
			traceID: "abcdef0123456789abcdef0123456789",
			spanID:  "1234567890abcdef",
			flags:   0,
		},
		{
			name:    "valid with flags 1",
			input:   "00-abcdef0123456789abcdef0123456789-1234567890abcdef-01",
			wantErr: false,
			traceID: "abcdef0123456789abcdef0123456789",
			spanID:  "1234567890abcdef",
			flags:   1,
		},
		{
			name:    "invalid format - wrong prefix",
			input:   "01-abcdef0123456789abcdef0123456789-1234567890abcdef-00",
			wantErr: true,
		},
		{
			name:    "invalid - too few parts",
			input:   "00-abcdef-1234567890abcdef",
			wantErr: true,
		},
		{
			name:    "invalid - trace ID too short",
			input:   "00-short-1234567890abcdef-00",
			wantErr: true,
		},
		{
			name:    "invalid - span ID too short",
			input:   "00-abcdef0123456789abcdef0123456789-short-00",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc, err := FromW3CTraceParent(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sc.TraceID != tt.traceID {
				t.Errorf("TraceID = %q, want %q", sc.TraceID, tt.traceID)
			}
			if sc.SpanID != tt.spanID {
				t.Errorf("SpanID = %q, want %q", sc.SpanID, tt.spanID)
			}
			if sc.TraceFlags != tt.flags {
				t.Errorf("TraceFlags = %d, want %d", sc.TraceFlags, tt.flags)
			}
		})
	}
}

func TestSpanContext_WithTraceState(t *testing.T) {
	sc := SpanContext{TraceID: "trace", SpanID: "span"}
	sc = sc.WithTraceState("key1", "value1")

	if sc.TraceState["key1"] != "value1" {
		t.Errorf("TraceState[key1] = %q, want value1", sc.TraceState["key1"])
	}

	// Ensure it doesn't mutate the original map
	sc2 := sc.WithTraceState("key2", "value2")
	if _, ok := sc.TraceState["key2"]; ok {
		t.Error("WithTraceState should not mutate the original TraceState")
	}
	if sc2.TraceState["key2"] != "value2" {
		t.Errorf("TraceState[key2] = %q, want value2", sc2.TraceState["key2"])
	}
}

// ===== NoopSpan tests =====

func TestNoopSpan(t *testing.T) {
	s := &NoopSpan{}
	s.SetName("test")
	s.SetAttribute("key", "value")
	s.SetStatus(SpanStatusOK, "ok")

	if s.SpanContext().IsValid() {
		t.Error("NoopSpan SpanContext should be invalid (empty)")
	}
	if s.IsEnded() {
		t.Error("NoopSpan should never be ended")
	}
	s.End() // should not panic
}

// ===== LoggingSpan tests =====

func TestLoggingSpan_Basic(t *testing.T) {
	tracer := NewLoggingTracer()
	span := tracer.Start("test-span", SpanKindInternal)

	span.SetName("renamed-span")
	span.SetAttribute("attr1", "val1")
	span.SetAttribute("attr2", 42)
	span.SetStatus(SpanStatusOK, "success")

	if span.IsEnded() {
		t.Error("span should not be ended before End()")
	}

	span.End()

	if !span.IsEnded() {
		t.Error("span should be ended after End()")
	}

	// Double End should be safe
	span.End()
}

func TestLoggingSpan_SpanContext(t *testing.T) {
	tracer := NewLoggingTracer()
	span := tracer.Start("test-span", SpanKindClient)

	sc := span.SpanContext()
	if !sc.IsValid() {
		t.Error("span context should be valid")
	}
	if sc.TraceID == "" {
		t.Error("TraceID should not be empty")
	}
	if sc.SpanID == "" {
		t.Error("SpanID should not be empty")
	}

	span.End()
}

func TestLoggingSpan_WithParent(t *testing.T) {
	tracer := NewLoggingTracer()
	parent := tracer.Start("parent", SpanKindServer)
	parentCtx := parent.SpanContext()

	child := tracer.Start("child", SpanKindInternal, WithParent(parentCtx))

	childCtx := child.SpanContext()
	if childCtx.TraceID != parentCtx.TraceID {
		t.Errorf("child TraceID = %q, want %q (parent)", childCtx.TraceID, parentCtx.TraceID)
	}
	if childCtx.SpanID == parentCtx.SpanID {
		t.Error("child SpanID should differ from parent")
	}

	parent.End()
	child.End()
}

func TestLoggingSpan_WithAttributes(t *testing.T) {
	tracer := NewLoggingTracer()
	attrs := map[string]any{
		"service": "test",
		"version": "1.0",
	}
	span := tracer.Start("test-span", SpanKindInternal, WithAttributes(attrs))

	// Add more attributes
	span.SetAttribute("custom", "value")

	span.End()
}

// ===== LoggingTracer tests =====

func TestLoggingTracer_Start(t *testing.T) {
	tracer := NewLoggingTracer()

	s1 := tracer.Start("span1", SpanKindInternal)
	s2 := tracer.Start("span2", SpanKindClient)

	s1.End()
	s2.End()

	// Both spans should have different trace IDs (no parent)
	if s1.SpanContext().TraceID == s2.SpanContext().TraceID {
		t.Error("spans without parent should have different trace IDs")
	}
}

func TestLoggingTracer_Reset(t *testing.T) {
	tracer := NewLoggingTracer()

	s := tracer.Start("span1", SpanKindInternal)
	s.End()

	tracer.Reset()

	// After reset, String() should be empty
	output := tracer.String()
	if output != "" {
		t.Errorf("String() after Reset = %q, want empty", output)
	}
}

func TestLoggingTracer_String(t *testing.T) {
	tracer := NewLoggingTracer()

	s1 := tracer.Start("span1", SpanKindInternal)
	s1.SetAttribute("key", "val")
	s1.SetStatus(SpanStatusOK, "ok")
	s1.End()

	s2 := tracer.Start("span2", SpanKindClient)
	s2.End()

	output := tracer.String()
	if !strings.Contains(output, "span1") {
		t.Errorf("String() should contain 'span1': %q", output)
	}
	if !strings.Contains(output, "span2") {
		t.Errorf("String() should contain 'span2': %q", output)
	}
	if !strings.Contains(output, "key=val") && !strings.Contains(output, "key:val") {
		// attrs format may vary
		t.Errorf("String() should contain attribute key=val or key:val: %q", output)
	}
}

func TestLoggingTracer_String_UnendedSpan(t *testing.T) {
	tracer := NewLoggingTracer()

	s := tracer.Start("unended", SpanKindInternal)
	// Don't end it

	output := tracer.String()
	if strings.Contains(output, "unended") {
		t.Errorf("String() should not contain unended spans: %q", output)
	}

	s.End()
}

// ===== SpanOption tests =====

func TestWithParent(t *testing.T) {
	cfg := &SpanConfig{}
	opt := WithParent(SpanContext{TraceID: "parent-trace", SpanID: "parent-span"})
	opt(cfg)

	if cfg.ParentContext.TraceID != "parent-trace" {
		t.Errorf("ParentContext.TraceID = %q, want parent-trace", cfg.ParentContext.TraceID)
	}
}

func TestWithAttributes(t *testing.T) {
	cfg := &SpanConfig{}
	attrs := map[string]any{"a": 1, "b": "two"}
	opt := WithAttributes(attrs)
	opt(cfg)

	if cfg.Attributes["a"] != 1 {
		t.Errorf("Attributes[a] = %v, want 1", cfg.Attributes["a"])
	}
	if cfg.Attributes["b"] != "two" {
		t.Errorf("Attributes[b] = %v, want two", cfg.Attributes["b"])
	}
}

func TestWithAttributes_NilExisting(t *testing.T) {
	cfg := &SpanConfig{}
	// Attributes starts as nil, WithAttributes should initialize it
	opt := WithAttributes(map[string]any{"key": "val"})
	opt(cfg)

	if cfg.Attributes == nil {
		t.Fatal("Attributes should not be nil after WithAttributes")
	}
	if cfg.Attributes["key"] != "val" {
		t.Errorf("Attributes[key] = %v, want val", cfg.Attributes["key"])
	}
}

func TestWithAttributes_MergeIntoExisting(t *testing.T) {
	cfg := &SpanConfig{
		Attributes: map[string]any{"existing": "yes"},
	}
	opt := WithAttributes(map[string]any{"new": "added"})
	opt(cfg)

	if cfg.Attributes["existing"] != "yes" {
		t.Error("existing attribute should be preserved")
	}
	if cfg.Attributes["new"] != "added" {
		t.Error("new attribute should be added")
	}
}

// ===== Integration tests =====

func TestLoggingTracer_ParentChildChain(t *testing.T) {
	tracer := NewLoggingTracer()

	// Root span
	root := tracer.Start("root", SpanKindServer)
	rootCtx := root.SpanContext()

	// Child span
	child := tracer.Start("child", SpanKindInternal, WithParent(rootCtx))
	childCtx := child.SpanContext()

	if childCtx.TraceID != rootCtx.TraceID {
		t.Errorf("child trace ID %q should match root %q", childCtx.TraceID, rootCtx.TraceID)
	}

	// Grandchild span
	grandchild := tracer.Start("grandchild", SpanKindClient, WithParent(childCtx))
	grandchildCtx := grandchild.SpanContext()

	if grandchildCtx.TraceID != rootCtx.TraceID {
		t.Errorf("grandchild trace ID %q should match root %q", grandchildCtx.TraceID, rootCtx.TraceID)
	}

	root.End()
	child.End()
	grandchild.End()

	// All three should appear in String()
	output := tracer.String()
	if !strings.Contains(output, "root") || !strings.Contains(output, "child") || !strings.Contains(output, "grandchild") {
		t.Errorf("String() should contain all span names: %q", output)
	}
}

func TestLoggingSpan_SetStatus_Error(t *testing.T) {
	tracer := NewLoggingTracer()
	span := tracer.Start("error-span", SpanKindInternal)
	span.SetStatus(SpanStatusError, "something went wrong")
	span.End()

	output := tracer.String()
	if !strings.Contains(output, "error") {
		t.Errorf("String() should contain error status: %q", output)
	}
}

func TestSpanStatus_Constants(t *testing.T) {
	if SpanStatusOK != "ok" {
		t.Errorf("SpanStatusOK = %q, want 'ok'", SpanStatusOK)
	}
	if SpanStatusError != "error" {
		t.Errorf("SpanStatusError = %q, want 'error'", SpanStatusError)
	}
}

func TestSpanKind_Constants(t *testing.T) {
	if SpanKindInternal != "internal" {
		t.Errorf("SpanKindInternal = %q, want 'internal'", SpanKindInternal)
	}
	if SpanKindClient != "client" {
		t.Errorf("SpanKindClient = %q, want 'client'", SpanKindClient)
	}
	if SpanKindServer != "server" {
		t.Errorf("SpanKindServer = %q, want 'server'", SpanKindServer)
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID(16)
	id2 := generateID(16)

	if len(id1) != 32 {
		t.Errorf("generateID(16) length = %d, want 32 (hex encoded)", len(id1))
	}
	if id1 == id2 {
		t.Error("generateID should produce unique IDs")
	}
}
