package llm

import (
	"testing"
)

// BenchmarkStreamPipeline_Process 测试预编译中间件链的热路径性能。
// 对比：预编译后 Process() 应该比每次重建链有显著的 allocs/op 降低。
func BenchmarkStreamPipeline_Process(b *testing.B) {
	handler := func(c Chunk) error { return nil }

	p := NewStreamPipeline(handler)
	p.Use(FilterMiddleware())
	p.Use(TransformMiddleware(func(s string) string { return s }))
	p.Use(BufferMiddleware())

	chunk := Chunk{Content: "hello world"}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = p.Process(chunk)
	}
}

// BenchmarkStreamPipeline_Process_Parallel 测试并发 Process 性能。
func BenchmarkStreamPipeline_Process_Parallel(b *testing.B) {
	handler := func(c Chunk) error { return nil }

	p := NewStreamPipeline(handler)
	p.Use(FilterMiddleware())
	p.Use(TransformMiddleware(func(s string) string { return s }))

	chunk := Chunk{Content: "hello world"}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = p.Process(chunk)
		}
	})
}

// BenchmarkStreamPipeline_RebuildChain 测试中间件添加（含重建）性能。
func BenchmarkStreamPipeline_RebuildChain(b *testing.B) {
	handler := func(c Chunk) error { return nil }

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p := NewStreamPipeline(handler)
		p.Use(FilterMiddleware())
		p.Use(TransformMiddleware(func(s string) string { return s }))
		p.Use(BufferMiddleware())
		_ = p
	}
}

