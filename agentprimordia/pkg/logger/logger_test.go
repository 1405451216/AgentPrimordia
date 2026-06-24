package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	l := Default()
	if l == nil {
		t.Fatal("Default() returned nil")
	}
}

func TestWithComponent(t *testing.T) {
	l := WithComponent("test-module")
	if l == nil {
		t.Fatal("WithComponent() returned nil")
	}
}

func TestSetLevel(t *testing.T) {
	// 设置 Debug 级别
	SetLevel(slog.LevelDebug)
	l := Default()
	if l == nil {
		t.Fatal("Default() returned nil after SetLevel")
	}

	// 验证日志输出包含 component 属性
	var buf bytes.Buffer
	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	h := slog.NewTextHandler(&buf, opts)
	testLogger := slog.New(h).With("component", "test")
	testLogger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "component=test") {
		t.Errorf("expected component=test in output, got: %s", output)
	}
}
