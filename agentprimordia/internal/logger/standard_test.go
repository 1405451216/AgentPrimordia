package logger

import (
	"bytes"
	"encoding/json"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestStandardLogger_DefaultJSON(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	l.Info("hello", "key", "value")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if m["msg"] != "hello" {
		t.Errorf("msg = %v, want hello", m["msg"])
	}
}

func TestStandardLogger_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	l.Debug("text-msg")
	if !strings.Contains(buf.String(), "text-msg") {
		t.Errorf("text output = %q, want containing text-msg", buf.String())
	}
}

func TestStandardLogger_LevelFilter(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	l.Info("should be filtered")
	l.Warn("should appear")

	if strings.Contains(buf.String(), "should be filtered") {
		t.Error("Info log not filtered")
	}
	if !strings.Contains(buf.String(), "should appear") {
		t.Error("Warn log incorrectly filtered")
	}
}

func TestStandardLogger_SetLevel(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))
	l.Debug("debug-msg")
	l.Info("info-msg")
	l.Error("error-msg")

	output := buf.String()
	if strings.Contains(output, "debug-msg") {
		t.Error("Debug log not filtered at Error level")
	}
	if strings.Contains(output, "info-msg") {
		t.Error("Info log not filtered at Error level")
	}
	if !strings.Contains(output, "error-msg") {
		t.Error("Error log missing")
	}
	_ = l
}

func TestStandardLogger_WithAgent(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	agentL := l.With(FieldAgentID, "agent-001")
	agentL.Info("agent action")

	var m map[string]any
	json.Unmarshal(buf.Bytes(), &m)
	if m["agent_id"] != "agent-001" {
		t.Errorf("agent_id = %v, want agent-001", m["agent_id"])
	}
}

func TestStandardLogger_WithSession(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	sessionL := l.With(FieldSessionID, "sess-abc")
	sessionL.Info("session action")

	var m map[string]any
	json.Unmarshal(buf.Bytes(), &m)
	if m["session_id"] != "sess-abc" {
		t.Errorf("session_id = %v, want sess-abc", m["session_id"])
	}
}

func TestStandardLogger_DebugCtx(t *testing.T) {
	ctx := context.Background()
	ctx = WithTraceID(ctx, "trace-123")
	ctx = WithSpanID(ctx, "span-456")

	var buf bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctxL := FromContext(ctx, l)
	ctxL.Debug("ctx msg")

	var m map[string]any
	json.Unmarshal(buf.Bytes(), &m)
	if m["trace_id"] != "trace-123" {
		t.Errorf("trace_id = %v, want trace-123", m["trace_id"])
	}
	if m["span_id"] != "span-456" {
		t.Errorf("span_id = %v, want span-456", m["span_id"])
	}
}

func TestStandardLogLevel_Constants(t *testing.T) {
	if LogLevelDebug != 0 {
		t.Errorf("LogLevelDebug = %d, want 0", LogLevelDebug)
	}
	if LogLevelInfo != 1 {
		t.Errorf("LogLevelInfo = %d, want 1", LogLevelInfo)
	}
	if LogLevelWarn != 2 {
		t.Errorf("LogLevelWarn = %d, want 2", LogLevelWarn)
	}
	if LogLevelError != 3 {
		t.Errorf("LogLevelError = %d, want 3", LogLevelError)
	}
}

func TestStandardToLogLevel_Mapping(t *testing.T) {
	tests := []struct {
		in       LogLevel
		expected slog.Level
	}{
		{LogLevelDebug, slog.LevelDebug},
		{LogLevelInfo, slog.LevelInfo},
		{LogLevelWarn, slog.LevelWarn},
		{LogLevelError, slog.LevelError},
		{LogLevel(99), slog.LevelInfo},
	}
	for _, tt := range tests {
		got := toSlogLevel(tt.in)
		if got != tt.expected {
			t.Errorf("toSlogLevel(%d) = %v, want %v", tt.in, got, tt.expected)
		}
	}
}
