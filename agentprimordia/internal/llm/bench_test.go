// perf-v4 Task 12.4：LLM Cache 性能基线
package llm

import (
	"context"
	"testing"
)

// BenchmarkCache_Get_FingerprintHit 精确指纹命中快路径
func BenchmarkCache_Get_FingerprintHit(b *testing.B) {
	cache := NewInMemoryCache(nil, 100, 0.9)
	ctx := context.Background()
	mockResp := &CompletionResponse{ID: "test", Model: "m", Content: "ok"}
	_ = cache.Set(ctx, "test query", mockResp)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = cache.Get(ctx, "test query", 0.9)
	}
}

// BenchmarkCache_Get_Miss 缓存未命中场景
func BenchmarkCache_Get_Miss(b *testing.B) {
	cache := NewInMemoryCache(nil, 100, 0.9)
	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = cache.Get(ctx, "test query that is not in cache", 0.9)
	}
}

// BenchmarkCache_Set 写入场景
func BenchmarkCache_Set(b *testing.B) {
	cache := NewInMemoryCache(nil, 1000, 0.9)
	ctx := context.Background()
	mockResp := &CompletionResponse{ID: "test", Model: "m", Content: "ok"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = cache.Set(ctx, "test query", mockResp)
	}
}

// BenchmarkCache_FullCycle Set + Get 完整周期
func BenchmarkCache_FullCycle(b *testing.B) {
	cache := NewInMemoryCache(nil, 1000, 0.9)
	ctx := context.Background()
	mockResp := &CompletionResponse{ID: "test", Model: "m", Content: "ok"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = cache.Set(ctx, "test query", mockResp)
		_, _ = cache.Get(ctx, "test query", 0.9)
	}
}
