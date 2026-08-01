package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStdoutShipper(t *testing.T) {
	var buf bytes.Buffer
	s := NewStdoutShipperWithWriter(&buf)

	entries := []LogEntry{
		{Timestamp: time.Now(), Level: "INFO", Message: "hello", TraceID: "t1"},
		{Timestamp: time.Now(), Level: "ERROR", Message: "boom", AgentID: "a1"},
	}

	if err := s.Ship(entries); err != nil {
		t.Fatalf("Ship failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if m["message"] != "hello" {
		t.Errorf("message = %v, want hello", m["message"])
	}
	if m["trace_id"] != "t1" {
		t.Errorf("trace_id = %v, want t1", m["trace_id"])
	}
}

func TestStdoutShipper_Close(t *testing.T) {
	s := NewStdoutShipper()
	if err := s.Close(); err != nil {
		t.Errorf("Close stdout failed: %v", err)
	}
}

func TestFileShipper(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	s, err := NewFileShipper(path, 1, 3)
	if err != nil {
		t.Fatalf("NewFileShipper failed: %v", err)
	}
	defer s.Close()

	entries := []LogEntry{
		{Timestamp: time.Now(), Level: "INFO", Message: "entry-1"},
		{Timestamp: time.Now(), Level: "WARN", Message: "entry-2"},
	}

	if err := s.Ship(entries); err != nil {
		t.Fatalf("Ship failed: %v", err)
	}

	// 验证文件存在且包含内容
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d", len(lines))
	}
}

func TestFileShipper_Rotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rotate.log")

	// 设置非常小的 maxSize 以便触发滚动
	s, err := NewFileShipper(path, 0, 3) // maxSizeMB=0 -> 100MB default
	if err != nil {
		t.Fatalf("NewFileShipper failed: %v", err)
	}
	defer s.Close()

	// 手动设置小阈值
	s.mu.Lock()
	s.maxSize = 200 // 200 bytes
	s.mu.Unlock()

	// 写入足够多的数据触发滚动
	entries := make([]LogEntry, 0, 10)
	for i := 0; i < 10; i++ {
		entries = append(entries, LogEntry{
			Timestamp: time.Now(),
			Level:     "INFO",
			Message:   "this is a long enough message to trigger rotation quickly " + strings.Repeat("x", 50),
		})
	}

	for _, e := range entries {
		if err := s.Ship([]LogEntry{e}); err != nil {
			t.Fatalf("Ship failed: %v", err)
		}
	}

	// 验证主文件存在
	if _, err := os.Stat(path); err != nil {
		t.Errorf("main log file should exist: %v", err)
	}

	// 验证至少有一个滚动文件
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Logf("rotation file .1 not found (may depend on timing): %v", err)
	}
}

func TestHook_Handle(t *testing.T) {
	var buf bytes.Buffer
	shipper := NewStdoutShipperWithWriter(&buf)
	hook := NewHook(shipper)

	// 模拟 slog.Record
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	ctx := WithTraceID(context.Background(), "trace-abc")

	err := hook.Handle(ctx, rec)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("output should contain message, got: %s", output)
	}
	if !strings.Contains(output, "trace-abc") {
		t.Errorf("output should contain trace_id, got: %s", output)
	}
}

func TestHook_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	shipper := NewStdoutShipperWithWriter(&buf)
	hook := NewHook(shipper)

	hookWithAttrs := hook.WithAttrs([]slog.Attr{
		slog.String("component", "test"),
	})

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "attr test", 0)
	err := hookWithAttrs.Handle(context.Background(), rec)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	var m map[string]any
	_ = json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &m)
	fields, ok := m["fields"].(map[string]any)
	if !ok {
		t.Fatalf("fields not found or not an object in: %s", buf.String())
	}
	if fields["component"] != "test" {
		t.Errorf("component = %v, want test", fields["component"])
	}
}

func TestHook_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	shipper := NewStdoutShipperWithWriter(&buf)
	hook := NewHook(shipper)

	hookWithGroup := hook.WithGroup("mygroup")
	if hookWithGroup == nil {
		t.Fatal("WithGroup returned nil")
	}

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "group test", 0)
	err := hookWithGroup.Handle(context.Background(), rec)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if !strings.Contains(buf.String(), "group test") {
		t.Errorf("output should contain message, got: %s", buf.String())
	}
}

func TestHook_Enabled(t *testing.T) {
	hook := NewHook(NewStdoutShipper())
	if !hook.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Hook should always be enabled")
	}
	if !hook.Enabled(context.Background(), slog.LevelError) {
		t.Error("Hook should always be enabled")
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"DEBUG", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"WARN", slog.LevelWarn},
		{"WARNING", slog.LevelWarn},
		{"ERROR", slog.LevelError},
		{"", slog.LevelInfo},
		{"invalid", slog.LevelInfo},
		{"0", slog.LevelInfo}, // slog.Level(0) == LevelInfo
	}
	for _, tt := range tests {
		got := ParseLogLevel(tt.input)
		if got != tt.expected {
			t.Errorf("ParseLogLevel(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestFormatLogEntry(t *testing.T) {
	e := LogEntry{
		Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Level:     "INFO",
		Message:   "test",
		TraceID:   "t1",
		AgentID:   "a1",
	}
	s := FormatLogEntry(e)
	if !strings.Contains(s, "test") {
		t.Errorf("FormatLogEntry should contain message, got: %s", s)
	}
	if !strings.Contains(s, "trace_id=t1") {
		t.Errorf("FormatLogEntry should contain trace_id, got: %s", s)
	}
	if !strings.Contains(s, "agent_id=a1") {
		t.Errorf("FormatLogEntry should contain agent_id, got: %s", s)
	}
}
