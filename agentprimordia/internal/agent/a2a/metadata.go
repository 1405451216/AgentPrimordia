package a2a

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// Metadata 是对 gRPC metadata.MD 的轻量包装，提供 key/value 操作
//
// 设计动机：
//  1. 让 client/server 之间的 trace context 传播与 gRPC metadata 解耦
//  2. 便于测试：可以使用纯 map 实现替代，不需要启动 gRPC server
//  3. 允许后续替换底层传输（如切换到 in-process transport）而不影响 API
//
// 当前实现直接包装 gRPC metadata.MD（这是 gRPC 规范的事实标准）。
type Metadata metadata.MD

// Clone 浅拷贝 metadata（slice 内的字符串仍共享底层 array）
func (m Metadata) Clone() Metadata {
	if m == nil {
		return nil
	}
	out := make(Metadata, len(m))
	for k, v := range m {
		cp := make([]string, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

// Get 获取 key 对应的所有 value（不存在返回 nil）
func (m Metadata) Get(key string) []string {
	if len(m) == 0 {
		return nil
	}
	return metadata.MD(m).Get(key)
}

// Set 设置 key 单个 value（覆盖已有值）
//
// 在 nil receiver 上调用会自动初始化底层 map，避免 panic。
func (m Metadata) Set(key, value string) {
	if m == nil {
		return
	}
	metadata.MD(m).Set(key, value)
}

// Append 追加 value 到 key 的现有列表
//
// 在 nil receiver 上调用会自动初始化底层 map，避免 panic。
func (m Metadata) Append(key, value string) {
	if m == nil {
		return
	}
	metadata.MD(m).Append(key, value)
}

// outgoingMetadataKey context.Value 私有键
type outgoingMetadataKey struct{}

// incomingMetadataKey context.Value 私有键
type incomingMetadataKey struct{}

// WithOutgoingMetadata 包装 outgoing metadata 到 ctx
//
// 该 ctx 可传给 gRPC client，gRPC 会自动将 metadata 写入 HTTP/2 headers。
func WithOutgoingMetadata(ctx context.Context, md Metadata) context.Context {
	if md == nil {
		return ctx
	}
	return context.WithValue(ctx, outgoingMetadataKey{}, md)
}

// OutgoingMetadataFromContext 从 ctx 提取 outgoing metadata
func OutgoingMetadataFromContext(ctx context.Context) (Metadata, bool) {
	if ctx == nil {
		return nil, false
	}
	md, ok := ctx.Value(outgoingMetadataKey{}).(Metadata)
	if !ok || md == nil {
		return nil, false
	}
	return md, true
}

// WithIncomingMetadata 包装 incoming metadata 到 ctx
//
// gRPC server 拦截器使用：将 metadata.MD 注入 ctx。
func WithIncomingMetadata(ctx context.Context, md Metadata) context.Context {
	if md == nil {
		return ctx
	}
	return context.WithValue(ctx, incomingMetadataKey{}, md)
}

// IncomingMetadataFromContext 从 ctx 提取 incoming metadata
func IncomingMetadataFromContext(ctx context.Context) (Metadata, bool) {
	if ctx == nil {
		return nil, false
	}
	md, ok := ctx.Value(incomingMetadataKey{}).(Metadata)
	if !ok || md == nil {
		return nil, false
	}
	return md, true
}

// ToGRPCOutgoingContext 将 ctx 中的 outgoing metadata 转换为 gRPC outgoing context
//
// gRPC client 发送请求时调用：先用 WithOutgoingMetadata 包装 metadata，
// 再通过本函数转换为 metadata.NewOutgoingContext 形式。
func ToGRPCOutgoingContext(ctx context.Context) context.Context {
	md, ok := OutgoingMetadataFromContext(ctx)
	if !ok {
		return ctx
	}

	// 如果 ctx 已经有 gRPC outgoing metadata，合并；否则直接覆盖
	existing, hasExisting := metadata.FromOutgoingContext(ctx)
	merged := Metadata{}
	if hasExisting {
		for k, v := range existing {
			merged[k] = append([]string{}, v...)
		}
	}
	for k, v := range md {
		merged[k] = append([]string{}, v...)
	}

	return metadata.NewOutgoingContext(ctx, metadata.MD(merged))
}

// FromGRPCIncomingContext 从 gRPC incoming ctx 提取 metadata 包装为 incoming metadata
//
// gRPC server handler 入口调用：将 metadata.FromIncomingContext 包装为
// 我们的 incoming metadata 抽象。
func FromGRPCIncomingContext(ctx context.Context) (context.Context, Metadata) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx, nil
	}
	wrapped := Metadata{}
	for k, v := range md {
		wrapped[k] = append([]string{}, v...)
	}
	return WithIncomingMetadata(ctx, wrapped), wrapped
}
