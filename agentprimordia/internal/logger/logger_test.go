package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNewLogger_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  LevelInfo,
		Format: FormatJSON,
		Output: &buf,
	})

	l.Info("test message", "key", "value")

	output := buf.String()
	var m map[string]any
	if err := json.Unmarshal([]byte(output), &m); err != nil {
		t.Fatalf("输出不是有效 JSON: %v\n输出: %s", err, output)
	}

	if m["msg"] != "test message" {
		t.Errorf("msg = %v, 期望 test message", m["msg"])
	}
	if m["key"] != "value" {
		t.Errorf("key = %v, 期望 value", m["key"])
	}
}

func TestNewLogger_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  LevelInfo,
		Format: FormatText,
		Output: &buf,
	})

	l.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("输出 = %q, 期望包含 test message", output)
	}
}

func TestNewLogger_LevelFilter(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  LevelWarn,
		Format: FormatJSON,
		Output: &buf,
	})

	l.Info("should be filtered")
	l.Warn("should appear")

	output := buf.String()
	if strings.Contains(output, "should be filtered") {
		t.Error("Info 日志未被过滤")
	}
	if !strings.Contains(output, "should appear") {
		t.Error("Warn 日志被错误过滤")
	}
}

func TestNewLogger_WithAgentContext(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  LevelInfo,
		Format: FormatJSON,
		Output: &buf,
	})

	agentL := l.WithAgent("my-agent", "session-123")
	agentL.Info("agent action")

	var m map[string]any
	_ = json.Unmarshal(buf.Bytes(), &m)

	if m["agent_name"] != "my-agent" {
		t.Errorf("agent_name = %v, 期望 my-agent", m["agent_name"])
	}
	if m["session_id"] != "session-123" {
		t.Errorf("session_id = %v, 期望 session-123", m["session_id"])
	}
}

func TestNewLogger_DefaultConfig(t *testing.T) {
	// 测试默认配置
	l := New(nil)
	if l == nil {
		t.Fatal("New(nil) 返回 nil")
	}
}

func TestNewLogger_WithComponent(t *testing.T) {
	var buf bytes.Buffer
	l := New(&Config{
		Level:  LevelInfo,
		Format: FormatJSON,
		Output: &buf,
	})

	compL := l.WithComponent("database")
	compL.Info("connection established")

	var m map[string]any
	_ = json.Unmarshal(buf.Bytes(), &m)

	if m["component"] != "database" {
		t.Errorf("component = %v, 期望 database", m["component"])
	}
}

func TestLogLevel_Constants(t *testing.T) {
	if LevelDebug != slog.LevelDebug {
		t.Error("LevelDebug 不匹配")
	}
	if LevelInfo != slog.LevelInfo {
		t.Error("LevelInfo 不匹配")
	}
	if LevelWarn != slog.LevelWarn {
		t.Error("LevelWarn 不匹配")
	}
	if LevelError != slog.LevelError {
		t.Error("LevelError 不匹配")
	}
}
