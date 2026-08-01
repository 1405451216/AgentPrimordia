package logger

import (
	"context"
	"testing"
)

func TestWithTraceID(t *testing.T) {
	ctx := WithTraceID(context.Background(), "trace-123")
	got := TraceIDFromContext(ctx)
	if got != "trace-123" {
		t.Errorf("TraceIDFromContext = %q, want %q", got, "trace-123")
	}
}

func TestWithSpanID(t *testing.T) {
	ctx := WithSpanID(context.Background(), "span-456")
	got := SpanIDFromContext(ctx)
	if got != "span-456" {
		t.Errorf("SpanIDFromContext = %q, want %q", got, "span-456")
	}
}

func TestTraceIDFromContext_Empty(t *testing.T) {
	got := TraceIDFromContext(context.Background())
	if got != "" {
		t.Errorf("TraceIDFromContext(empty) = %q, want empty", got)
	}
}

func TestSpanIDFromContext_Empty(t *testing.T) {
	got := SpanIDFromContext(context.Background())
	if got != "" {
		t.Errorf("SpanIDFromContext(empty) = %q, want empty", got)
	}
}
