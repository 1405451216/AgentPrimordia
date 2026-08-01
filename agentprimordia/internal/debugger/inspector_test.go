package debugger

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInspector_StartSpan(t *testing.T) {
	inspector := NewInspector(100)
	ctx := context.Background()

	span, _ := inspector.StartSpan(ctx, "test-span", "agent", "session-1")

	if span == nil {
		t.Fatal("expected span to be created")
	}

	if span.Name != "test-span" {
		t.Errorf("expected span name 'test-span', got '%s'", span.Name)
	}

	if span.Kind != "agent" {
		t.Errorf("expected span kind 'agent', got '%s'", span.Kind)
	}

	if span.SessionID != "session-1" {
		t.Errorf("expected session ID 'session-1', got '%s'", span.SessionID)
	}

	if span.Status != "started" {
		t.Errorf("expected status 'started', got '%s'", span.Status)
	}

	// 验证span被存储
	traces := inspector.GetTraces()
	if len(traces) != 1 {
		t.Errorf("expected 1 trace, got %d", len(traces))
	}

	// 验证session被创建
	sessions := inspector.GetAllSessions()
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}
}

func TestInspector_EndSpan(t *testing.T) {
	inspector := NewInspector(100)
	ctx := context.Background()

	span, _ := inspector.StartSpan(ctx, "test-span", "agent", "session-1")
	time.Sleep(10 * time.Millisecond)
	inspector.EndSpan(span, nil)

	if span.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", span.Status)
	}

	if span.EndTime.IsZero() {
		t.Error("expected end time to be set")
	}

	if span.Duration <= 0 {
		t.Error("expected duration to be positive")
	}
}

func TestInspector_EndSpanWithError(t *testing.T) {
	inspector := NewInspector(100)
	ctx := context.Background()

	span, _ := inspector.StartSpan(ctx, "test-span", "agent", "session-1")
	testErr := errors.New("test error")
	inspector.EndSpan(span, testErr)

	if span.Status != "failed" {
		t.Errorf("expected status 'failed', got '%s'", span.Status)
	}

	if span.Error != "test error" {
		t.Errorf("expected error 'test error', got '%s'", span.Error)
	}
}

func TestInspector_AddEvent(t *testing.T) {
	inspector := NewInspector(100)
	ctx := context.Background()

	span, _ := inspector.StartSpan(ctx, "test-span", "llm", "session-1")

	attrs := map[string]interface{}{
		"model":  "gpt-4",
		"tokens": 100,
	}
	inspector.AddEvent(span, "llm-call", attrs)

	if len(span.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(span.Events))
	}

	event := span.Events[0]
	if event.Name != "llm-call" {
		t.Errorf("expected event name 'llm-call', got '%s'", event.Name)
	}

	if event.Attributes["model"] != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got '%v'", event.Attributes["model"])
	}
}

func TestInspector_SetAttribute(t *testing.T) {
	inspector := NewInspector(100)
	ctx := context.Background()

	span, _ := inspector.StartSpan(ctx, "test-span", "tool", "session-1")
	inspector.SetAttribute(span, "tool-name", "search")
	inspector.SetAttribute(span, "duration_ms", 150)

	if span.Attributes["tool-name"] != "search" {
		t.Errorf("expected tool-name 'search', got '%v'", span.Attributes["tool-name"])
	}

	if span.Attributes["duration_ms"] != 150 {
		t.Errorf("expected duration_ms 150, got '%v'", span.Attributes["duration_ms"])
	}
}

func TestInspector_ParentChildSpan(t *testing.T) {
	inspector := NewInspector(100)
	ctx := context.Background()

	// 创建父span
	parentSpan, ctx := inspector.StartSpan(ctx, "parent", "agent", "session-1")

	// 创建子span（应该自动继承parent ID和trace ID）
	childSpan, _ := inspector.StartSpan(ctx, "child", "llm", "session-1")

	if childSpan.ParentID != parentSpan.ID {
		t.Errorf("expected parent ID '%s', got '%s'", parentSpan.ID, childSpan.ParentID)
	}

	if childSpan.TraceID != parentSpan.TraceID {
		t.Errorf("expected trace ID '%s', got '%s'", parentSpan.TraceID, childSpan.TraceID)
	}
}

func TestInspector_GetSessionTrace(t *testing.T) {
	inspector := NewInspector(100)
	ctx := context.Background()

	// 创建多个span
	span1, ctx := inspector.StartSpan(ctx, "span1", "agent", "session-1")
	inspector.EndSpan(span1, nil)

	span2, _ := inspector.StartSpan(ctx, "span2", "llm", "session-1")
	inspector.EndSpan(span2, nil)

	// 获取session trace
	session := inspector.GetSessionTrace("session-1")
	if session == nil {
		t.Fatal("expected session trace to exist")
	}

	if session.SessionID != "session-1" {
		t.Errorf("expected session ID 'session-1', got '%s'", session.SessionID)
	}

	if len(session.Spans) != 2 {
		t.Errorf("expected 2 spans, got %d", len(session.Spans))
	}
}

func TestInspector_GetStats(t *testing.T) {
	inspector := NewInspector(100)
	ctx := context.Background()

	// 创建多个不同类型的span
	span1, _ := inspector.StartSpan(ctx, "agent-span", "agent", "session-1")
	span1.TotalTokens = 100
	inspector.EndSpan(span1, nil)

	span2, _ := inspector.StartSpan(ctx, "llm-span", "llm", "session-1")
	span2.TotalTokens = 200
	inspector.EndSpan(span2, nil)

	span3, _ := inspector.StartSpan(ctx, "tool-span", "tool", "session-2")
	span3.TotalTokens = 50
	inspector.EndSpan(span3, errors.New("error"))

	stats := inspector.GetStats()

	if stats.TotalSpans != 3 {
		t.Errorf("expected 3 total spans, got %d", stats.TotalSpans)
	}

	if stats.TotalSessions != 2 {
		t.Errorf("expected 2 total sessions, got %d", stats.TotalSessions)
	}

	if stats.TotalTokens != 350 {
		t.Errorf("expected 350 total tokens, got %d", stats.TotalTokens)
	}

	if stats.SpanByKind["agent"] != 1 {
		t.Errorf("expected 1 agent span, got %d", stats.SpanByKind["agent"])
	}

	if stats.SpanByKind["llm"] != 1 {
		t.Errorf("expected 1 llm span, got %d", stats.SpanByKind["llm"])
	}

	if stats.SpanByStatus["completed"] != 2 {
		t.Errorf("expected 2 completed spans, got %d", stats.SpanByStatus["completed"])
	}

	if stats.SpanByStatus["failed"] != 1 {
		t.Errorf("expected 1 failed span, got %d", stats.SpanByStatus["failed"])
	}
}

func TestInspector_MaxSpansLimit(t *testing.T) {
	inspector := NewInspector(5) // 限制为5个span
	ctx := context.Background()

	// 创建10个span
	for i := 0; i < 10; i++ {
		span, _ := inspector.StartSpan(ctx, "span", "agent", "session-1")
		inspector.EndSpan(span, nil)
	}

	traces := inspector.GetTraces()
	if len(traces) != 5 {
		t.Errorf("expected 5 traces (max limit), got %d", len(traces))
	}
}

func TestSpanFromContext(t *testing.T) {
	inspector := NewInspector(100)
	ctx := context.Background()

	// 初始context应该没有span
	span := SpanFromContext(ctx)
	if span != nil {
		t.Error("expected nil span from empty context")
	}

	// 创建span后应该能从context中获取
	_, ctx = inspector.StartSpan(ctx, "test", "agent", "session-1")
	span = SpanFromContext(ctx)
	if span == nil {
		t.Fatal("expected span from context")
	}

	if span.Name != "test" {
		t.Errorf("expected span name 'test', got '%s'", span.Name)
	}
}

func TestInspector_NilSpanSafety(t *testing.T) {
	inspector := NewInspector(100)

	// 这些操作不应该panic
	inspector.EndSpan(nil, nil)
	inspector.AddEvent(nil, "test", nil)
	inspector.SetAttribute(nil, "key", "value")
}
