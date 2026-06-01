package agent

import (
	"testing"
)

func TestTracerInterface_LoggingTracer(t *testing.T) {
	var tr Tracer = NewLoggingTracer()
	span := tr.Start("test", SpanKindInternal)
	if span == nil {
		t.Fatal("Start should return non-nil span")
	}
	span.End()
}

func TestTracerInterface_NoopTracer(t *testing.T) {
	var tr Tracer = NewNoopTracer()
	span := tr.Start("test", SpanKindInternal)
	if span == nil {
		t.Fatal("Start should return non-nil span")
	}
	span.End()
}

func TestTracerDebug_LoggingTracer(t *testing.T) {
	tr := NewLoggingTracer()
	span := tr.Start("test", SpanKindInternal)
	span.End()

	debug, ok := any(tr).(TracerDebug)
	if !ok {
		t.Fatal("LoggingTracer should implement TracerDebug")
	}
	output := debug.String()
	if output == "" {
		t.Error("TracerDebug.String should return non-empty after span end")
	}
	debug.Reset()
	if debug.String() != "" {
		t.Error("TracerDebug.Reset should clear spans")
	}
}

func TestNoopTracer_DoesNotPanic(t *testing.T) {
	tr := NewNoopTracer()
	span := tr.Start("op", SpanKindClient, WithParent(SpanContext{TraceID: "t", SpanID: "s"}))
	span.SetName("renamed")
	span.SetAttribute("k", "v")
	span.SetStatus(SpanStatusOK, "")
	_ = span.SpanContext()
	_ = span.IsEnded()
	span.End()
}
