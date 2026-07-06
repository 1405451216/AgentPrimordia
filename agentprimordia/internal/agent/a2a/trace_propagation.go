package a2a

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
)

// TraceContext W3C Trace Context 表示
//
// 参考 W3C Trace Context Level 2 规范：https://www.w3.org/TR/trace-context/
// 一个 traceparent 形如：00-<trace-id 32 hex>-<parent-id 16 hex>-<flags 2 hex>
// 当前仅支持 version=00 与 default flags。
type TraceContext struct {
	// Version 2 hex chars；当前固定为 "00"
	Version string
	// TraceID 32 hex chars（128-bit）
	TraceID string
	// SpanID 16 hex chars（64-bit）
	SpanID string
	// Flags 2 hex chars；"01" 表示 sampled
	Flags string
}

// String 序列化为 W3C traceparent 字符串
func (tc TraceContext) String() string {
	return fmt.Sprintf("%s-%s-%s-%s", tc.Version, tc.TraceID, tc.SpanID, tc.Flags)
}

// IsZero 判断 TraceContext 是否为空
func (tc TraceContext) IsZero() bool {
	return tc.TraceID == "" && tc.SpanID == ""
}

// Sampled 返回是否被采样
func (tc TraceContext) Sampled() bool {
	return len(tc.Flags) >= 2 && tc.Flags[1] == '1'
}

// traceparentHeader W3C traceparent header 名称（HTTP/gRPC metadata key）
const traceparentHeader = "traceparent"

// tracestateHeader W3C tracestate header 名称（保留用于未来扩展）
const tracestateHeader = "tracestate"

// traceContextKey context.Value 私有键，避免与其他包冲突
type traceContextKey struct{}

// spanIDCounter 用于生成 trace context 时提供单调递增的 span ID 后缀
// 仅用于本进程生成的 trace；跨进程时由对端覆盖。
var spanIDCounter uint64

// GenerateTraceContext 生成一个新的 TraceContext（无父）
func GenerateTraceContext() TraceContext {
	return TraceContext{
		Version: "00",
		TraceID: generateTraceID(),
		SpanID:  generateSpanID(),
		Flags:   "01", // 默认采样
	}
}

// ChildTraceContext 基于父上下文生成子 TraceContext（span ID 更新，trace ID 保持）
func ChildTraceContext(parent TraceContext) TraceContext {
	if parent.IsZero() {
		return GenerateTraceContext()
	}
	return TraceContext{
		Version: parent.Version,
		TraceID: parent.TraceID,
		SpanID:  generateSpanID(),
		Flags:   parent.Flags,
	}
}

// GenerateTraceContextInCtx 生成新 TraceContext 并注入 ctx
//
// 便捷封装：等价于 tc := GenerateTraceContext(); WithTraceContext(ctx, tc)
func GenerateTraceContextInCtx(ctx context.Context) (context.Context, TraceContext) {
	tc := GenerateTraceContext()
	return WithTraceContext(ctx, tc), tc
}

// ContinueTraceInCtx 基于父 TraceContext 创建子 TraceContext 并注入 ctx
func ContinueTraceInCtx(ctx context.Context, parent TraceContext) (context.Context, TraceContext) {
	tc := ChildTraceContext(parent)
	return WithTraceContext(ctx, tc), tc
}

// ParseTraceParent 解析 W3C traceparent 字符串
//
// 接受标准格式：version-traceid-spanid-flags
// 错误情况下返回 zero value；调用方应通过 IsZero 检查。
func ParseTraceParent(header string) (TraceContext, error) {
	if len(header) == 0 {
		return TraceContext{}, fmt.Errorf("empty traceparent")
	}

	// 快速校验长度：2+1+32+1+16+1+2 = 55
	if len(header) < 55 {
		return TraceContext{}, fmt.Errorf("traceparent too short: %d", len(header))
	}

	var tc TraceContext
	n, err := fmt.Sscanf(header, "%2s-%32s-%16s-%2s", &tc.Version, &tc.TraceID, &tc.SpanID, &tc.Flags)
	if err != nil || n != 4 {
		return TraceContext{}, fmt.Errorf("invalid traceparent format: %q", header)
	}

	if tc.Version != "00" {
		// 未来版本暂不支持
		return TraceContext{}, fmt.Errorf("unsupported traceparent version: %q", tc.Version)
	}

	if tc.TraceID == "00000000000000000000000000000000" {
		return TraceContext{}, fmt.Errorf("invalid all-zero trace_id")
	}
	if tc.SpanID == "0000000000000000" {
		return TraceContext{}, fmt.Errorf("invalid all-zero span_id")
	}

	return tc, nil
}

// WithTraceContext 将 TraceContext 注入 context
func WithTraceContext(ctx context.Context, tc TraceContext) context.Context {
	if tc.IsZero() {
		return ctx
	}
	return context.WithValue(ctx, traceContextKey{}, tc)
}

// TraceContextFromContext 从 context 提取 TraceContext（不存在时返回 zero value）
func TraceContextFromContext(ctx context.Context) (TraceContext, bool) {
	if ctx == nil {
		return TraceContext{}, false
	}
	tc, ok := ctx.Value(traceContextKey{}).(TraceContext)
	if !ok || tc.IsZero() {
		return TraceContext{}, false
	}
	return tc, true
}

// InjectTraceParent 将 TraceContext 注入 outgoing metadata
//
// 这是 client-side 入口：调用方需要已经将 metadata 写入 ctx。
// 当前实现：从 ctx 提取 TraceContext，若存在则附加 traceparent header。
//
// 入参 ctx 应已包含 metadata（通过 metadata.NewOutgoingContext 注入）。
// 返回的新 ctx 是同一 metadata 的拷贝，避免并发修改。
func InjectTraceParent(ctx context.Context) context.Context {
	// 从 ctx 提取 trace context
	tc, ok := TraceContextFromContext(ctx)
	if !ok {
		// 无 trace context 时也尝试从 metadata 已有 traceparent 中保持透传（不应发生）
		return ctx
	}

	md, ok := OutgoingMetadataFromContext(ctx)
	if !ok {
		md = make(Metadata)
	} else {
		md = md.Clone()
	}
	md.Set(traceparentHeader, tc.String())

	// tracestate 可选：保留已有值
	return WithOutgoingMetadata(ctx, md)
}

// ExtractTraceParent 从 incoming metadata 提取 TraceContext 并注入 ctx
//
// 这是 server-side 入口：从 ctx 的 incoming metadata 读取 traceparent，
// 解析后存入 context.Value，供后续 span 创建使用。
func ExtractTraceParent(ctx context.Context) context.Context {
	md, ok := IncomingMetadataFromContext(ctx)
	if !ok {
		return ctx
	}

	vals := md.Get(traceparentHeader)
	if len(vals) == 0 {
		return ctx
	}

	tc, err := ParseTraceParent(vals[0])
	if err != nil {
		return ctx
	}
	return WithTraceContext(ctx, tc)
}

// generateTraceID 生成 32 字符 16 字节 hex trace ID
func generateTraceID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 退化为单调计数（永不失败）
		atomic.AddUint64(&spanIDCounter, 1)
		return fmt.Sprintf("ffffffff%016x", spanIDCounter)
	}
	return hex.EncodeToString(b[:])
}

// generateSpanID 生成 16 字符 8 字节 hex span ID
func generateSpanID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		atomic.AddUint64(&spanIDCounter, 1)
		return fmt.Sprintf("%016x", spanIDCounter)
	}
	return hex.EncodeToString(b[:])
}
