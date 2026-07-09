// leader_election_bench_test.go — Leader 选举性能基准测试（生产集成深度）
//
// 基准测试覆盖：
//   - LeaderElector 竞选吞吐量（MemoryCoordinator）
//   - 心跳续约性能
//   - 状态转换性能
//   - LeaderMetrics 记录性能
//   - 并发竞选下的吞吐量
package persist

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// BenchmarkLeaderElection_AcquireRelease 竞选 + 释放的吞吐量。
func BenchmarkLeaderElection_AcquireRelease(b *testing.B) {
	coord := NewMemoryCoordinator("bench-node", 30*time.Second)
	config := LeaderConfig{
		ElectionKey:       "bench-leader",
		TTL:               30 * time.Second,
		HeartbeatInterval: 10 * time.Second,
		RetryInterval:     1 * time.Millisecond,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		le := NewLeaderElector(coord, fmt.Sprintf("node-%d", i), config)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = le.Start(ctx)
		le.Stop()
		cancel()
	}
}

// BenchmarkLeaderElection_Heartbeat 单次心跳续约性能。
func BenchmarkLeaderElection_Heartbeat(b *testing.B) {
	coord := NewMemoryCoordinator("bench-node", 30*time.Second)
	config := LeaderConfig{
		ElectionKey:       "bench-heartbeat",
		TTL:               30 * time.Second,
		HeartbeatInterval: 1 * time.Hour, // 不自动触发
	}

	le := NewLeaderElector(coord, "bench-node", config)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_ = le.Start(ctx)
	defer le.Stop()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		le.heartbeat(ctx)
	}
}

// BenchmarkLeaderMetrics_Record 指标记录性能。
func BenchmarkLeaderMetrics_Record(b *testing.B) {
	m := NewLeaderMetrics()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.RecordHeartbeatSent()
		m.RecordAcquireAttempt()
		m.SetState(LeaderLeading)
	}
}

// BenchmarkLeaderMetrics_Snapshot 指标快照性能。
func BenchmarkLeaderMetrics_Snapshot(b *testing.B) {
	m := NewLeaderMetrics()
	for i := 0; i < 10000; i++ {
		m.RecordHeartbeatSent()
		m.RecordAcquireAttempt()
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.Snapshot()
	}
}

// BenchmarkLeaderElection_ConcurrentContention 并发竞争下的吞吐量。
// 多个节点同时尝试竞选同一 key，测量竞争开销。
func BenchmarkLeaderElection_ConcurrentContention(b *testing.B) {
	for _, n := range []int{2, 4, 8, 16} {
		b.Run(fmt.Sprintf("nodes=%d", n), func(b *testing.B) {
			coord := NewMemoryCoordinator("bench-coord", 30*time.Second)
			config := LeaderConfig{
				ElectionKey:       "bench-contention",
				TTL:               30 * time.Second,
				HeartbeatInterval: 10 * time.Second,
				RetryInterval:     1 * time.Millisecond,
			}

			b.ResetTimer()
			b.ReportAllocs()

			var wg sync.WaitGroup
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func(nodeID int) {
					defer wg.Done()
					le := NewLeaderElector(coord, fmt.Sprintf("node-%d", nodeID), config)
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					_ = le.Start(ctx)
					le.Stop()
					cancel()
				}(i)
			}
			wg.Wait()
		})
	}
}
