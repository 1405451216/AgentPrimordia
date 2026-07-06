package a2a

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/metadata"
)

// TestTraceContext_String 验证 W3C traceparent 序列化格式
func TestTraceContext_String(t *testing.T) {
	tc := TraceContext{
		Version: "00",
		TraceID: "0af7651916cd43dd8448eb211c80319c",
		SpanID:  "b7ad6b7169203331",
		Flags:   "01",
	}
	got := tc.String()
	want := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestTraceContext_IsZero 验证零值检测
func TestTraceContext_IsZero(t *testing.T) {
	var zero TraceContext
	if !zero.IsZero() {
		t.Errorf("zero value should report IsZero=true")
	}

	tc := GenerateTraceContext()
	if tc.IsZero() {
		t.Errorf("generated TraceContext should not be zero")
	}
}

// TestTraceContext_Sampled 验证采样位
func TestTraceContext_Sampled(t *testing.T) {
	tests := []struct {
		name  string
		flags string
		want  bool
	}{
		{"sampled", "01", true},
		{"not_sampled", "00", false},
		{"empty_flags", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TraceContext{Flags: tt.flags}
			if got := tc.Sampled(); got != tt.want {
				t.Errorf("Sampled() with flags %q = %v, want %v", tt.flags, got, tt.want)
			}
		})
	}
}

// TestGenerateTraceContext 验证新生成的 TraceContext 格式有效
func TestGenerateTraceContext(t *testing.T) {
	tc := GenerateTraceContext()

	if tc.Version != "00" {
		t.Errorf("Version = %q, want %q", tc.Version, "00")
	}
	if len(tc.TraceID) != 32 {
		t.Errorf("TraceID length = %d, want 32", len(tc.TraceID))
	}
	if len(tc.SpanID) != 16 {
		t.Errorf("SpanID length = %d, want 16", len(tc.SpanID))
	}
	if tc.Flags != "01" {
		t.Errorf("Flags = %q, want %q", tc.Flags, "01")
	}
	if tc.TraceID == "00000000000000000000000000000000" {
		t.Errorf("TraceID must not be all zeros")
	}
	if tc.SpanID == "0000000000000000" {
		t.Errorf("SpanID must not be all zeros")
	}

	// 两次生成应得到不同 trace ID
	tc2 := GenerateTraceContext()
	if tc.TraceID == tc2.TraceID {
		t.Errorf("two generated TraceContexts should have different trace IDs (got %s twice)", tc.TraceID)
	}
}

// TestChildTraceContext 验证子 span 共享 trace ID 但 span ID 不同
func TestChildTraceContext(t *testing.T) {
	parent := TraceContext{
		Version: "00",
		TraceID: "0af7651916cd43dd8448eb211c80319c",
		SpanID:  "b7ad6b7169203331",
		Flags:   "01",
	}

	child := ChildTraceContext(parent)
	if child.TraceID != parent.TraceID {
		t.Errorf("child.TraceID = %q, want %q", child.TraceID, parent.TraceID)
	}
	if child.SpanID == parent.SpanID {
		t.Errorf("child.SpanID should differ from parent")
	}
	if child.Version != parent.Version {
		t.Errorf("child.Version = %q, want %q", child.Version, parent.Version)
	}
	if child.Flags != parent.Flags {
		t.Errorf("child.Flags = %q, want %q", child.Flags, parent.Flags)
	}

	// 当父为零值时应自动生成新 trace
	child2 := ChildTraceContext(TraceContext{})
	if child2.TraceID == "" {
		t.Errorf("zero-parent should auto-generate a new trace ID")
	}
}

// TestParseTraceParent_Valid 验证合法 traceparent 解析
func TestParseTraceParent_Valid(t *testing.T) {
	header := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	tc, err := ParseTraceParent(header)
	if err != nil {
		t.Fatalf("ParseTraceParent returned error: %v", err)
	}
	if tc.Version != "00" {
		t.Errorf("Version = %q, want %q", tc.Version, "00")
	}
	if tc.TraceID != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("TraceID = %q", tc.TraceID)
	}
	if tc.SpanID != "b7ad6b7169203331" {
		t.Errorf("SpanID = %q", tc.SpanID)
	}
	if tc.Flags != "01" {
		t.Errorf("Flags = %q", tc.Flags)
	}
}

// TestParseTraceParent_Errors 验证各种异常输入
func TestParseTraceParent_Errors(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{"empty", ""},
		{"too_short", "00-aa"},
		{"invalid_format", "garbage"},
		{"zero_trace_id", "00-00000000000000000000000000000000-b7ad6b7169203331-01"},
		{"zero_span_id", "00-0af7651916cd43dd8448eb211c80319c-0000000000000000-01"},
		{"unsupported_version", "ff-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseTraceParent(tt.header)
			if err == nil {
				t.Errorf("ParseTraceParent(%q) expected error", tt.header)
			}
		})
	}
}

// TestWithTraceContext 验证 context 注入与提取
func TestWithTraceContext(t *testing.T) {
	ctx := context.Background()

	// 无 trace 时应返回 false
	if _, ok := TraceContextFromContext(ctx); ok {
		t.Errorf("empty ctx should not yield a TraceContext")
	}

	tc := GenerateTraceContext()
	ctx2 := WithTraceContext(ctx, tc)

	got, ok := TraceContextFromContext(ctx2)
	if !ok {
		t.Fatalf("WithTraceContext should be retrievable")
	}
	if got.TraceID != tc.TraceID {
		t.Errorf("retrieved TraceID = %q, want %q", got.TraceID, tc.TraceID)
	}

	// 注入零值不应改变 ctx
	ctx3 := WithTraceContext(ctx, TraceContext{})
	if _, ok := TraceContextFromContext(ctx3); ok {
		t.Errorf("zero TraceContext injection should be a no-op")
	}
}

// TestInjectTraceParent_NoTraceContext 验证无 trace 时注入是 no-op
func TestInjectTraceParent_NoTraceContext(t *testing.T) {
	ctx := context.Background()
	got := InjectTraceParent(ctx)
	if got != ctx {
		t.Errorf("InjectTraceParent on ctx without TraceContext should return same ctx")
	}
}

// TestInjectTraceParent_WithMetadata 验证 trace parent 写入 outgoing metadata
func TestInjectTraceParent_WithMetadata(t *testing.T) {
	tc := TraceContext{
		Version: "00",
		TraceID: "0af7651916cd43dd8448eb211c80319c",
		SpanID:  "b7ad6b7169203331",
		Flags:   "01",
	}

	ctx := WithTraceContext(context.Background(), tc)
	ctx = InjectTraceParent(ctx)

	md, ok := OutgoingMetadataFromContext(ctx)
	if !ok {
		t.Fatalf("expected outgoing metadata to be set after InjectTraceParent")
	}
	got := md.Get(traceparentHeader)
	if len(got) == 0 {
		t.Fatalf("expected traceparent header in metadata")
	}
	if got[0] != tc.String() {
		t.Errorf("traceparent = %q, want %q", got[0], tc.String())
	}
}

// TestExtractTraceParent_WithMetadata 验证从 incoming metadata 还原 trace
func TestExtractTraceParent_WithMetadata(t *testing.T) {
	header := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	md := Metadata{traceparentHeader: []string{header}}
	ctx := WithIncomingMetadata(context.Background(), md)

	ctx2 := ExtractTraceParent(ctx)
	got, ok := TraceContextFromContext(ctx2)
	if !ok {
		t.Fatalf("ExtractTraceParent failed to restore TraceContext")
	}
	if got.TraceID != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("TraceID = %q", got.TraceID)
	}
	if got.SpanID != "b7ad6b7169203331" {
		t.Errorf("SpanID = %q", got.SpanID)
	}
}

// TestExtractTraceParent_NoMetadata 验证无 metadata 时不修改 ctx
func TestExtractTraceParent_NoMetadata(t *testing.T) {
	ctx := context.Background()
	ctx2 := ExtractTraceParent(ctx)
	if _, ok := TraceContextFromContext(ctx2); ok {
		t.Errorf("ExtractTraceParent on ctx without metadata should not produce TraceContext")
	}
}

// TestExtractTraceParent_InvalidTrace 验证非法 trace header 不影响 ctx
func TestExtractTraceParent_InvalidTrace(t *testing.T) {
	md := Metadata{traceparentHeader: []string{"invalid"}}
	ctx := WithIncomingMetadata(context.Background(), md)
	ctx2 := ExtractTraceParent(ctx)
	if _, ok := TraceContextFromContext(ctx2); ok {
		t.Errorf("invalid traceparent should not produce TraceContext")
	}
}

// TestTracePropagation_A2ACall 模拟 Agent A → A2A RPC → Agent B 的 trace 传播
func TestTracePropagation_A2ACall(t *testing.T) {
	// Agent A 端：生成 trace context
	tc := GenerateTraceContext()
	if tc.TraceID == "" {
		t.Fatalf("generate failed")
	}

	// 包装到 outgoing ctx
	outCtx := WithTraceContext(context.Background(), tc)
	outCtx = InjectTraceParent(outCtx)

	md, ok := OutgoingMetadataFromContext(outCtx)
	if !ok {
		t.Fatalf("expected outgoing metadata")
	}
	hdrs := md.Get(traceparentHeader)
	if len(hdrs) == 0 {
		t.Fatalf("expected traceparent header")
	}

	// 模拟 wire：Agent B 端从 incoming metadata 提取
	inMd := Metadata{traceparentHeader: hdrs}
	inCtx := WithIncomingMetadata(context.Background(), inMd)
	serverCtx := ExtractTraceParent(inCtx)

	restored, ok := TraceContextFromContext(serverCtx)
	if !ok {
		t.Fatalf("server failed to extract trace")
	}
	if restored.TraceID != tc.TraceID {
		t.Errorf("trace_id mismatch: agent_a=%q agent_b=%q", tc.TraceID, restored.TraceID)
	}
}

// TestTracePropagation_ChildSpan 验证子 span 继承父 trace ID
func TestTracePropagation_ChildSpan(t *testing.T) {
	parent := GenerateTraceContext()
	child := ChildTraceContext(parent)

	if child.TraceID != parent.TraceID {
		t.Errorf("child.TraceID should inherit from parent")
	}
	if child.SpanID == parent.SpanID {
		t.Errorf("child.SpanID should differ from parent")
	}
}

// TestMetadata_Clone 验证 metadata 克隆独立性
func TestMetadata_Clone(t *testing.T) {
	md := Metadata{
		"traceparent": []string{"00-aaa-bbb-01"},
		"x-custom":    []string{"v1", "v2"},
	}
	cp := md.Clone()

	cp.Set("traceparent", "00-ccc-ddd-01")
	cp.Append("x-custom", "v3")

	if md.Get("traceparent")[0] != "00-aaa-bbb-01" {
		t.Errorf("original traceparent mutated: %v", md.Get("traceparent"))
	}
	if len(md.Get("x-custom")) != 2 {
		t.Errorf("original x-custom mutated: %v", md.Get("x-custom"))
	}

	if cp.Get("traceparent")[0] != "00-ccc-ddd-01" {
		t.Errorf("clone traceparent not updated: %v", cp.Get("traceparent"))
	}
	if len(cp.Get("x-custom")) != 3 {
		t.Errorf("clone x-custom length = %d, want 3", len(cp.Get("x-custom")))
	}
}

// TestMetadata_NilSafe 验证 nil metadata 的安全性
func TestMetadata_NilSafe(t *testing.T) {
	var md Metadata
	if md.Get("foo") != nil {
		t.Errorf("nil metadata Get should return nil")
	}
	if cp := md.Clone(); cp != nil {
		t.Errorf("nil metadata Clone should return nil")
	}
	// Set/Append 在 nil 上应静默忽略（不 panic）
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil metadata Set/Append should not panic, got: %v", r)
		}
	}()
	md.Set("foo", "bar")
	md.Append("baz", "qux")
}

// TestOutgoingMetadata_RoundTrip 验证 outgoing metadata 经过 ToGRPCOutgoingContext
// 后能被 FromGRPCIncomingContext 完整还原（语义往返）
//
// 注意：gRPC outgoing context 与 incoming context 是不同的 context value 类型。
// 要完整测试 round-trip 需要实际走 gRPC wire；这里我们通过 metadata.FromOutgoingContext
// 验证 outgoing context 正确填充，并模拟 server 端的 incoming 处理路径。
func TestOutgoingMetadata_RoundTrip(t *testing.T) {
	md := Metadata{
		"x-api-key":   []string{"secret"},
		"traceparent": []string{"00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"},
		"x-custom":    []string{"a", "b"},
	}

	ctx := WithOutgoingMetadata(context.Background(), md)
	grpcCtx := ToGRPCOutgoingContext(ctx)

	// 验证 outgoing context 已正确写入 gRPC metadata
	grpcMD, ok := metadata.FromOutgoingContext(grpcCtx)
	if !ok {
		t.Fatalf("ToGRPCOutgoingContext should populate gRPC outgoing metadata")
	}
	if got := grpcMD.Get("x-api-key"); len(got) == 0 || got[0] != "secret" {
		t.Errorf("x-api-key not preserved in outgoing: %v", got)
	}
	if got := grpcMD.Get("traceparent"); len(got) == 0 || got[0] != md["traceparent"][0] {
		t.Errorf("traceparent not preserved in outgoing: %v", got)
	}
	if got := grpcMD.Get("x-custom"); len(got) != 2 {
		t.Errorf("x-custom length in outgoing = %d, want 2", len(got))
	}

	// 模拟 server 端：从 gRPC metadata（已经是 incoming 视角）解析
	// 这里我们把 outgoing metadata 当作 incoming 处理（模拟经过 wire）
	serverCtx := WithIncomingMetadata(context.Background(), Metadata(grpcMD))
	restored, ok := IncomingMetadataFromContext(serverCtx)
	if !ok {
		t.Fatalf("incoming metadata should be retrievable")
	}
	if got := restored.Get("traceparent"); len(got) == 0 || got[0] != md["traceparent"][0] {
		t.Errorf("traceparent not preserved: %v", got)
	}

	// server 提取 trace 应得到原始 TraceContext
	traceCtx := ExtractTraceParent(serverCtx)
	tc, ok := TraceContextFromContext(traceCtx)
	if !ok {
		t.Fatalf("ExtractTraceParent failed in round-trip")
	}
	if tc.TraceID != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("round-trip TraceID mismatch: %q", tc.TraceID)
	}
}

// TestToGRPCOutgoingContext_NoMetadata 验证无 metadata 时不修改 ctx
func TestToGRPCOutgoingContext_NoMetadata(t *testing.T) {
	ctx := context.Background()
	got := ToGRPCOutgoingContext(ctx)
	_, hasMD := metadata.FromOutgoingContext(got)
	if hasMD {
		t.Errorf("ToGRPCOutgoingContext should not add metadata when none present")
	}
}

// TestFromGRPCIncomingContext_NoMetadata 验证无 gRPC metadata 时返回原 ctx
func TestFromGRPCIncomingContext_NoMetadata(t *testing.T) {
	ctx := context.Background()
	got, md := FromGRPCIncomingContext(ctx)
	if got != ctx {
		t.Errorf("FromGRPCIncomingContext should return same ctx when no metadata")
	}
	if md != nil {
		t.Errorf("FromGRPCIncomingContext should return nil md when no metadata")
	}
}

// TestA2AGRPCClient_StartTrace 验证 client 的 StartTrace/ContinueTrace 辅助方法
func TestA2AGRPCClient_StartTrace(t *testing.T) {
	c := &A2AGRPCClient{}

	ctx, tc1 := c.StartTrace(context.Background())
	if tc1.TraceID == "" {
		t.Fatalf("StartTrace should yield a non-empty trace")
	}
	if _, ok := TraceContextFromContext(ctx); !ok {
		t.Errorf("ctx should carry TraceContext after StartTrace")
	}

	parent := GenerateTraceContext()
	ctx2, tc2 := c.ContinueTrace(ctx, parent)
	if tc2.TraceID != parent.TraceID {
		t.Errorf("ContinueTrace should inherit parent trace ID")
	}
	if tc2.SpanID == parent.SpanID {
		t.Errorf("ContinueTrace should produce a new span ID")
	}
	if _, ok := TraceContextFromContext(ctx2); !ok {
		t.Errorf("ctx2 should carry TraceContext after ContinueTrace")
	}
}

// TestA2AGRPCClient_WithTraceContext 验证 client 的 WithTraceContext 辅助方法
func TestA2AGRPCClient_WithTraceContext(t *testing.T) {
	c := &A2AGRPCClient{}

	tc := TraceContext{
		Version: "00",
		TraceID: "0af7651916cd43dd8448eb211c80319c",
		SpanID:  "b7ad6b7169203331",
		Flags:   "01",
	}
	ctx := c.WithTraceContext(context.Background(), tc)
	got, ok := TraceContextFromContext(ctx)
	if !ok {
		t.Fatalf("WithTraceContext should put TraceContext into ctx")
	}
	if got.TraceID != tc.TraceID {
		t.Errorf("TraceID = %q, want %q", got.TraceID, tc.TraceID)
	}
}

// TestTraceContext_String_Format 验证标准 W3C 样例格式
func TestTraceContext_String_Format(t *testing.T) {
	// W3C Trace Context spec 中的标准示例
	tc := TraceContext{
		Version: "00",
		TraceID: "0af7651916cd43dd8448eb211c80319c",
		SpanID:  "b7ad6b7169203331",
		Flags:   "01",
	}
	got := tc.String()
	if !strings.HasPrefix(got, "00-") {
		t.Errorf("traceparent must start with version 00-")
	}
	parts := strings.Split(got, "-")
	if len(parts) != 4 {
		t.Fatalf("traceparent should have 4 dash-separated parts, got %d", len(parts))
	}
	if len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		t.Errorf("traceparent format invalid: %s", got)
	}
}
