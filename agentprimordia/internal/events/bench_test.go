// perf-v4 Task 12.1：Event Bus 性能基线
// 建立性能基线，追踪 perf-v4 优化前后的回归
package events

import (
	"testing"
)

// BenchmarkBus_PublishAsync_10Subscribers 模拟典型场景：1 个发布者、10 个订阅者
func BenchmarkBus_PublishAsync_10Subscribers(b *testing.B) {
	bus := NewBus(64)
	for i := 0; i < 10; i++ {
		bus.Subscribe("test.event")
	}
	event := Event{Type: "test.event", Source: "bench"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = bus.PublishAsync(event)
	}
}

// BenchmarkBus_PublishAsync_Wildcard 模拟通配符订阅场景
func BenchmarkBus_PublishAsync_Wildcard(b *testing.B) {
	bus := NewBus(64)
	bus.SubscribeAll()
	event := Event{Type: "test.event", Source: "bench"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = bus.PublishAsync(event)
	}
}

// BenchmarkBus_PublishAsync_NoSubscribers 无订阅者场景（典型 fallback）
func BenchmarkBus_PublishAsync_NoSubscribers(b *testing.B) {
	bus := NewBus(64)
	event := Event{Type: "test.event", Source: "bench"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = bus.PublishAsync(event)
	}
}

// BenchmarkBus_Subscribe 订阅路径性能
func BenchmarkBus_Subscribe(b *testing.B) {
	bus := NewBus(64)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = bus.Subscribe("test.event")
	}
}
