package logger

import (
	"context"
	"log/slog"
)

// 统一日志字段名常量（Phase 4 Task 10：结构化日志标准化）
//
// 所有模块应使用这些常量作为 slog 的 key，确保不同模块的日志字段命名一致，
// 便于日志聚合系统（Loki / ELK / Datadog 等）做过滤与统计。
const (
	// FieldAgentID Agent 实例标识
	FieldAgentID = "agent_id"
	// FieldSessionID 会话标识
	FieldSessionID = "session_id"
	// FieldTurn ReAct 循环当前轮次
	FieldTurn = "turn"
	// FieldProvider LLM 提供方（如 openai / anthropic / ollama）
	FieldProvider = "provider"
	// FieldModel 模型名称（如 gpt-4o / claude-sonnet-4）
	FieldModel = "model"
	// FieldTool 工具名
	FieldTool = "tool"
	// FieldDuration 耗时（毫秒）。统一以毫秒输出，避免秒/毫秒混用。
	FieldDuration = "duration_ms"
	// FieldError 错误信息
	FieldError = "error"
	// FieldTraceID W3C Trace Context 的 trace-id
	FieldTraceID = "trace_id"
	// FieldSpanID W3C Trace Context 的 parent-id / span-id
	FieldSpanID = "span_id"
	// FieldComponent 组件名（admin / pool / tools.executor 等）
	FieldComponent = "component"
	// FieldArgsLen 工具调用参数长度（避免记录完整参数可能泄漏敏感数据）
	FieldArgsLen = "args_len"
)

// traceIDContextKey 是 logger 包在 ctx 中保存 trace-id 的私有键。
// 使用 struct{} 类型以避免与其他包冲突；对外仅暴露 WithTraceID / TraceIDFromContext。
type traceIDContextKey struct{}

// spanIDContextKey 是 logger 包在 ctx 中保存 span-id 的私有键。
type spanIDContextKey struct{}

// WithTraceID 把 trace-id 注入 ctx，便于跨调用链传递。
// 通常由最外层入口（如 HTTP handler / A2A 接收端）调用。
func WithTraceID(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDContextKey{}, traceID)
}

// WithSpanID 把 span-id 注入 ctx。
func WithSpanID(ctx context.Context, spanID string) context.Context {
	if spanID == "" {
		return ctx
	}
	return context.WithValue(ctx, spanIDContextKey{}, spanID)
}

// TraceIDFromContext 从 ctx 取出 trace-id；不存在时返回空串。
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, ok := ctx.Value(traceIDContextKey{}).(string)
	if !ok {
		return ""
	}
	return v
}

// SpanIDFromContext 从 ctx 取出 span-id；不存在时返回空串。
func SpanIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, ok := ctx.Value(spanIDContextKey{}).(string)
	if !ok {
		return ""
	}
	return v
}

// FromContext 返回一个会自动附带 trace_id / span_id 字段的 *slog.Logger。
//
// 当 ctx 中没有 trace-id 时，返回的 logger 与 l 等价（无附加字段）。
// 推荐在请求入口处调用一次，下游所有日志都会自动带上 trace_id，便于排障。
//
// 用法：
//
//	logger := logger.FromContext(ctx, slog.Default())
//	logger.Info("processing request", FieldAgentID, agentID)
func FromContext(ctx context.Context, l *slog.Logger) *slog.Logger {
	if l == nil {
		l = slog.Default()
	}
	if ctx == nil {
		return l
	}
	traceID := TraceIDFromContext(ctx)
	spanID := SpanIDFromContext(ctx)
	switch {
	case traceID != "" && spanID != "":
		return l.With(FieldTraceID, traceID, FieldSpanID, spanID)
	case traceID != "":
		return l.With(FieldTraceID, traceID)
	case spanID != "":
		return l.With(FieldSpanID, spanID)
	}
	return l
}

// FromContextDefault 等价于 FromContext(ctx, slog.Default())，便捷封装。
func FromContextDefault(ctx context.Context) *slog.Logger {
	return FromContext(ctx, slog.Default())
}
