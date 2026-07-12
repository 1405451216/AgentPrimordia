package protocol

import (
	"context"
	"testing"
)

func TestNewTraceContext(t *testing.T) {
	tc := NewTraceContext()
	if tc.TraceID == "" || len(tc.TraceID) != 16 {
		t.Errorf("invalid TraceID length: %d", len(tc.TraceID))
	}
	if tc.SpanID == "" || len(tc.SpanID) != 8 {
		t.Errorf("invalid SpanID length: %d", len(tc.SpanID))
	}
	if !tc.Sampled {
		t.Errorf("expected Sampled=true")
	}
}

func TestInjectAndExtractTraceContext(t *testing.T) {
	original := NewTraceContext()
	headers := InjectTraceContext(original)

	extracted := ExtractTraceContext(headers)
	if extracted.TraceID != original.TraceID {
		t.Errorf("TraceID mismatch: %s vs %s", extracted.TraceID, original.TraceID)
	}
	if extracted.SpanID != original.SpanID {
		t.Errorf("SpanID mismatch")
	}
	if extracted.Sampled != original.Sampled {
		t.Errorf("Sampled mismatch")
	}
}

func TestWithTraceFromContext(t *testing.T) {
	tc := NewTraceContext()
	ctx := WithTrace(context.Background(), tc)

	extracted, ok := FromTrace(ctx)
	if !ok {
		t.Fatal("expected trace context in context")
	}
	if extracted.TraceID != tc.TraceID {
		t.Errorf("TraceID mismatch")
	}
}

func TestFromContextNoTrace(t *testing.T) {
	_, ok := FromTrace(context.Background())
	if ok {
		t.Error("expected no trace context")
	}
}

