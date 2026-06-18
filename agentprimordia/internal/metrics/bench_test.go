// perf-v4 Task 12.3：Metrics 模块性能基线
package metrics

import (
	"testing"
)

// BenchmarkHistogram_Record 标准场景：直方图记录
func BenchmarkHistogram_Record(b *testing.B) {
	h := NewHistogram(defaultLatencyBuckets)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h.Record(int64(i % 5000))
	}
}

// BenchmarkHistogram_Record_HighConcurrency 模拟 8 goroutine 并发写入
func BenchmarkHistogram_Record_HighConcurrency(b *testing.B) {
	h := NewHistogram(defaultLatencyBuckets)
	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := int64(0)
		for pb.Next() {
			h.Record(i % 5000)
			i++
		}
	})
}

// BenchmarkHistogram_Snapshot 读取快照
func BenchmarkHistogram_Snapshot(b *testing.B) {
	h := NewHistogram(defaultLatencyBuckets)
	for i := 0; i < 1000; i++ {
		h.Record(int64(i % 5000))
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = h.Snapshot()
	}
}
