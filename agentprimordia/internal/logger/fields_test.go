package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// newTestLogger 返回把 JSON 输出到 buf 的 *slog.Logger。
func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// parseJSONLines 把 buf 中每行 JSON 解析为 map，方便单行断言。
func parseJSONLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("无法解析日志行 %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestFieldConstants_Values(t *testing.T) {
	// 字段名稳定是公共契约，修改会破坏日志聚合查询，必须显式锁定。
	cases := map[string]string{
		FieldAgentID:   "agent_id",
		FieldSessionID: "session_id",
		FieldTurn:      "turn",
		FieldProvider:  "provider",
		FieldModel:     "model",
		FieldTool:      "tool",
		FieldDuration:  "duration_ms",
		FieldError:     "error",
		FieldTraceID:   "trace_id",
		FieldSpanID:    "span_id",
		FieldComponent: "component",
		FieldArgsLen:   "args_len",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("字段常量值 = %q, 期望 %q", got, want)
		}
	}
}

func TestWithTraceID_AndFromContext(t *testing.T) {
	ctx := WithTraceID(context.Background(), "abc123def456")

	got := TraceIDFromContext(ctx)
	if got != "abc123def456" {
		t.Errorf("TraceIDFromContext = %q, 期望 %q", got, "abc123def456")
	}
}

func TestWithSpanID_AndFromContext(t *testing.T) {
	ctx := WithSpanID(context.Background(), "span-xyz")

	got := SpanIDFromContext(ctx)
	if got != "span-xyz" {
		t.Errorf("SpanIDFromContext = %q, 期望 %q", got, "span-xyz")
	}
}

func TestWithTraceID_EmptyStringIsNoop(t *testing.T) {
	// 空字符串不应写入 ctx，避免生成带空 trace_id 的日志。
	ctx := WithTraceID(context.Background(), "")
	if v := TraceIDFromContext(ctx); v != "" {
		t.Errorf("空字符串被错误写入 ctx: %q", v)
	}
}

func TestWithSpanID_EmptyStringIsNoop(t *testing.T) {
	ctx := WithSpanID(context.Background(), "")
	if v := SpanIDFromContext(ctx); v != "" {
		t.Errorf("空字符串被错误写入 ctx: %q", v)
	}
}

func TestTraceIDFromContext_NilContext(t *testing.T) {
	if v := TraceIDFromContext(nil); v != "" { //nolint:staticcheck // 故意测试 nil ctx
		t.Errorf("nil ctx 应返回空串, 实际 = %q", v)
	}
}

func TestSpanIDFromContext_NilContext(t *testing.T) {
	if v := SpanIDFromContext(nil); v != "" { //nolint:staticcheck // 故意测试 nil ctx
		t.Errorf("nil ctx 应返回空串, 实际 = %q", v)
	}
}

func TestTraceIDFromContext_EmptyContext(t *testing.T) {
	if v := TraceIDFromContext(context.Background()); v != "" {
		t.Errorf("空 ctx 应返回空串, 实际 = %q", v)
	}
}

func TestFromContext_WithTraceAndSpan(t *testing.T) {
	var buf bytes.Buffer
	base := newTestLogger(&buf)

	ctx := WithTraceID(WithSpanID(context.Background(), "span-1"), "trace-1")
	l := FromContext(ctx, base)
	l.Info("hello")

	rows := parseJSONLines(t, &buf)
	if len(rows) != 1 {
		t.Fatalf("期望 1 行日志, 实际 %d", len(rows))
	}
	if rows[0][FieldTraceID] != "trace-1" {
		t.Errorf("trace_id = %v, 期望 trace-1", rows[0][FieldTraceID])
	}
	if rows[0][FieldSpanID] != "span-1" {
		t.Errorf("span_id = %v, 期望 span-1", rows[0][FieldSpanID])
	}
}

func TestFromContext_WithTraceOnly(t *testing.T) {
	var buf bytes.Buffer
	base := newTestLogger(&buf)

	ctx := WithTraceID(context.Background(), "trace-only")
	l := FromContext(ctx, base)
	l.Info("hello")

	rows := parseJSONLines(t, &buf)
	if len(rows) != 1 {
		t.Fatalf("期望 1 行日志, 实际 %d", len(rows))
	}
	if rows[0][FieldTraceID] != "trace-only" {
		t.Errorf("trace_id = %v, 期望 trace-only", rows[0][FieldTraceID])
	}
	if _, ok := rows[0][FieldSpanID]; ok {
		t.Errorf("不应有 span_id 字段, 实际 = %v", rows[0][FieldSpanID])
	}
}

func TestFromContext_NoTraceNoSpan(t *testing.T) {
	var buf bytes.Buffer
	base := newTestLogger(&buf)

	l := FromContext(context.Background(), base)
	l.Info("hello")

	rows := parseJSONLines(t, &buf)
	if len(rows) != 1 {
		t.Fatalf("期望 1 行日志, 实际 %d", len(rows))
	}
	if _, ok := rows[0][FieldTraceID]; ok {
		t.Errorf("不应有 trace_id 字段, 实际 = %v", rows[0][FieldTraceID])
	}
	if _, ok := rows[0][FieldSpanID]; ok {
		t.Errorf("不应有 span_id 字段, 实际 = %v", rows[0][FieldSpanID])
	}
}

func TestFromContext_NilLoggerUsesDefault(t *testing.T) {
	// nil logger 时使用 slog.Default()，不应 panic。
	ctx := WithTraceID(context.Background(), "trace-x")
	l := FromContext(ctx, nil)
	if l == nil {
		t.Fatal("FromContext 不应返回 nil logger")
	}
}

func TestFromContext_NilContextReturnsBase(t *testing.T) {
	var buf bytes.Buffer
	base := newTestLogger(&buf)

	l := FromContext(nil, base) //nolint:staticcheck // 故意测试 nil ctx
	l.Info("hello")

	rows := parseJSONLines(t, &buf)
	if len(rows) != 1 {
		t.Fatalf("期望 1 行日志, 实际 %d", len(rows))
	}
	if _, ok := rows[0][FieldTraceID]; ok {
		t.Errorf("nil ctx 不应附加 trace_id, 实际 = %v", rows[0][FieldTraceID])
	}
}

func TestFromContext_PreservesBaseAttributes(t *testing.T) {
	// 在 ctx 之上不应丢失 base logger 已有的 With 属性（如 agent_id）。
	var buf bytes.Buffer
	base := newTestLogger(&buf).With(FieldAgentID, "agent-007")

	ctx := WithTraceID(context.Background(), "trace-007")
	l := FromContext(ctx, base)
	l.Info("hello")

	rows := parseJSONLines(t, &buf)
	if len(rows) != 1 {
		t.Fatalf("期望 1 行日志, 实际 %d", len(rows))
	}
	if rows[0][FieldAgentID] != "agent-007" {
		t.Errorf("agent_id = %v, 期望 agent-007", rows[0][FieldAgentID])
	}
	if rows[0][FieldTraceID] != "trace-007" {
		t.Errorf("trace_id = %v, 期望 trace-007", rows[0][FieldTraceID])
	}
}

func TestFromContextDefault(t *testing.T) {
	// FromContextDefault 等价于 FromContext(ctx, slog.Default())。
	ctx := WithTraceID(context.Background(), "trace-default")
	l := FromContextDefault(ctx)
	if l == nil {
		t.Fatal("FromContextDefault 不应返回 nil logger")
	}
}

func TestFromContext_TraceIDNotConflictingWithOtherKeys(t *testing.T) {
	// 业务方可能使用 string 类型的 key 存自己的 trace-id，这里验证
	// logger 包的私有 struct{} key 与之互不干扰。
	type otherKey struct{}
	ctx := context.WithValue(context.Background(), otherKey{}, "other-trace")
	ctx = WithTraceID(ctx, "logger-trace")

	if v := TraceIDFromContext(ctx); v != "logger-trace" {
		t.Errorf("logger TraceIDFromContext = %q, 期望 logger-trace", v)
	}
}

func TestLogger_WithAgent_EmitsUnifiedFields(t *testing.T) {
	// 端到端：WithAgent 应输出 agent_name + session_id 字段。
	var buf bytes.Buffer
	l := New(&Config{Level: LevelInfo, Format: FormatJSON, Output: &buf})
	l.WithAgent("a", "s").Info("hi")

	rows := parseJSONLines(t, &buf)
	if len(rows) != 1 {
		t.Fatalf("期望 1 行日志, 实际 %d", len(rows))
	}
	if rows[0]["agent_name"] != "a" {
		t.Errorf("agent_name = %v, 期望 a", rows[0]["agent_name"])
	}
	if rows[0]["session_id"] != "s" {
		t.Errorf("session_id = %v, 期望 s", rows[0]["session_id"])
	}
}
