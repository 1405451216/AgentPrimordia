package agent

import (
	"bytes"
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

// BenchmarkBuffer_Direct 对比直接分配 bytes.Buffer。
func BenchmarkBuffer_Direct(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := bytes.NewBuffer(make([]byte, 0, 1024))
		buf.WriteString("hello")
		_ = buf
	}
}

// BenchmarkBuffer_Pool 对比池化分配 bytes.Buffer。
func BenchmarkBuffer_Pool(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := AcquireBuffer()
		buf.WriteString("hello")
		ReleaseBuffer(buf)
	}
}
