package agent

import (
	"bytes"
	"io"
	"testing"
)

// BenchmarkHookContext_Direct 对比直接分配 HookContext。
// 用于验证 sync.Pool 的实际收益。
func BenchmarkHookContext_Direct(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		hctx := &HookContext{AgentID: "test", Turn: i}
		_ = hctx
	}
}

// BenchmarkHookContext_Pool 对比池化分配 HookContext。
func BenchmarkHookContext_Pool(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		hctx := AcquireHookContext()
		hctx.AgentID = "test"
		hctx.Turn = i
		ReleaseHookContext(hctx)
	}
}

// BenchmarkHookContext_Direct_Escaped 修正 BenchmarkHookContext_Direct 的假象：
// `_ = hctx` 让编译器完全消除字面量分配。
// 真实场景下 hctx 通过 Fire(ctx, hctx) 逃逸到堆，这里用 sink slice 模拟。
func BenchmarkHookContext_Direct_Escaped(b *testing.B) {
	b.ReportAllocs()
	sink := make([]*HookContext, 0, b.N)
	for i := 0; i < b.N; i++ {
		hctx := &HookContext{AgentID: "test", Turn: i, Point: HookAfterLLM}
		sink = append(sink, hctx)
	}
	_ = sink
}

// BenchmarkHookContext_Pool_Escaped 池化版本对应逃逸场景（同步 Release）。
func BenchmarkHookContext_Pool_Escaped(b *testing.B) {
	b.ReportAllocs()
	sink := make([]*HookContext, 0, b.N)
	for i := 0; i < b.N; i++ {
		hctx := AcquireHookContext()
		hctx.AgentID = "test"
		hctx.Turn = i
		hctx.Point = HookAfterLLM
		sink = append(sink, hctx)
		ReleaseHookContext(hctx)
	}
	_ = sink
}

// BenchmarkSSEWriter_Token 验证 perf-v12 BufferPool 改动的真实收益。
func BenchmarkSSEWriter_Token(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	flusher := &fakeFlusher{w: &buf}
	w := NewSSEWriter(flusher, flusher)
	for i := 0; i < b.N; i++ {
		_ = w.Token("hello world this is a sample token")
	}
}

// fakeFlusher 实现 io.Writer + http.Flusher，仅用于 bench。
type fakeFlusher struct {
	w io.Writer
}

func (f *fakeFlusher) Write(p []byte) (int, error) { return f.w.Write(p) }
func (f *fakeFlusher) Flush()                      {}
