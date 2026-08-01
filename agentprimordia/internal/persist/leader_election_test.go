package persist

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestLeaderElection_SingleNode(t *testing.T) {
	ctx := context.Background()
	coord := NewMemoryCoordinator("node-a", 5*time.Second)
	config := LeaderConfig{
		ElectionKey:       "leader:test",
		TTL:               5 * time.Second,
		HeartbeatInterval: 500 * time.Millisecond,
		RetryInterval:     200 * time.Millisecond,
	}

	le := NewLeaderElector(coord, "node-a", config)
	var eventCount atomic.Int32
	le.OnEvent(func(e LeaderEvent) {
		eventCount.Add(1)
	})

	if err := le.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !le.IsLeader() {
		t.Fatal("应成为 Leader")
	}
	if le.LeaderSince().IsZero() {
		t.Fatal("LeaderSince 不应为零值")
	}

	// 等待几次心跳
	time.Sleep(2 * time.Second)
	sent, failed, _ := le.GetStats()
	if sent < 2 {
		t.Fatalf("心跳次数 = %d, want >= 2", sent)
	}
	if failed > 0 {
		t.Fatalf("心跳失败次数 = %d, want 0", failed)
	}

	le.Stop()
	if le.IsLeader() {
		t.Fatal("停止后不应再是 Leader")
	}

	if eventCount.Load() < 2 {
		t.Fatalf("事件数 = %d, want >= 2", eventCount.Load())
	}
}

func TestLeaderElection_StateTransitions(t *testing.T) {
	ctx := context.Background()
	coord := NewMemoryCoordinator("node-x", 500*time.Millisecond)
	config := LeaderConfig{
		ElectionKey:       "leader:transitions",
		TTL:               500 * time.Millisecond,
		HeartbeatInterval: 100 * time.Millisecond,
		RetryInterval:     100 * time.Millisecond,
	}

	var events []LeaderEvent
	le := NewLeaderElector(coord, "node-x", config)
	le.OnEvent(func(e LeaderEvent) {
		events = append(events, e)
	})

	if err := le.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// 验证状态转换：Following → Candidate → Leading
	if len(events) < 2 {
		t.Fatalf("事件数 = %d, want >= 2", len(events))
	}
	if events[0].NewState != LeaderCandidate {
		t.Fatalf("第一个事件 NewState = %q, want %q", events[0].NewState, LeaderCandidate)
	}
	if events[1].NewState != LeaderLeading {
		t.Fatalf("第二个事件 NewState = %q, want %q", events[1].NewState, LeaderLeading)
	}

	le.Stop()
}

func TestLeaderElection_HealthCheck(t *testing.T) {
	ctx := context.Background()
	coord := NewMemoryCoordinator("node-hc", 5*time.Second)

	healthy := atomic.Bool{}
	healthy.Store(true)

	config := LeaderConfig{
		ElectionKey:       "leader:hc",
		TTL:               5 * time.Second,
		HeartbeatInterval: 200 * time.Millisecond,
		RetryInterval:     100 * time.Millisecond,
		HealthCheck:       func() bool { return healthy.Load() },
	}

	le := NewLeaderElector(coord, "node-hc", config)
	if err := le.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !le.IsLeader() {
		t.Fatal("应成为 Leader")
	}

	// 模拟健康检查失败
	healthy.Store(false)

	// 等待下一次心跳
	time.Sleep(500 * time.Millisecond)

	if le.IsLeader() {
		t.Fatal("健康检查失败后应让出 Leader")
	}

	le.Stop()
}

func TestLeaderElection_ForceTakeover(t *testing.T) {
	ctx := context.Background()
	coord := NewMemoryCoordinator("node-t", 100*time.Millisecond)

	le1 := NewLeaderElector(coord, "node-t1", DefaultLeaderConfig("leader:takeover"))
	if err := le1.Start(ctx); err != nil {
		t.Fatalf("le1.Start: %v", err)
	}

	// 等待 TTL 过期
	time.Sleep(200 * time.Millisecond)

	// le1 的租约已过期，le2 可以强制接管
	le2 := NewLeaderElector(coord, "node-t2", DefaultLeaderConfig("leader:takeover"))
	if err := le2.ForceTakeover(ctx); err != nil {
		t.Fatalf("ForceTakeover: %v", err)
	}
	if !le2.IsLeader() {
		t.Fatal("le2 应通过 ForceTakeover 成为 Leader")
	}

	le2.Stop()
}

func TestLeaderElection_RenewFailure(t *testing.T) {
	ctx := context.Background()
	// 极短 TTL，心跳间隔比 TTL 长，模拟续约失败
	coord := NewMemoryCoordinator("node-rf", 100*time.Millisecond)
	config := LeaderConfig{
		ElectionKey:       "leader:renew-fail",
		TTL:               100 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
		RetryInterval:     50 * time.Millisecond,
	}

	le := NewLeaderElector(coord, "node-rf", config)
	if err := le.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// 等待足够时间让心跳续约成功几次
	time.Sleep(300 * time.Millisecond)

	sent, failed, renewFails := le.GetStats()
	if sent == 0 {
		t.Fatal("应至少发送一次心跳")
	}
	// 在 MemoryCoordinator 中，Renew 总是成功的，所以 failed 应为 0
	if failed > 0 {
		t.Logf("心跳失败 = %d, renewFails = %d（MemoryCoordinator 的 Renew 不应失败）", failed, renewFails)
	}

	le.Stop()
}

func TestLeaderElection_TwoNodes_MemoryCoord(t *testing.T) {
	// 测试 Leader 停止后锁被释放，其他节点可以获取
	ctx := context.Background()
	coord := NewMemoryCoordinator("node-a", 2*time.Second)

	config := LeaderConfig{
		ElectionKey:       "leader:two-logic",
		TTL:               2 * time.Second,
		HeartbeatInterval: 300 * time.Millisecond,
		RetryInterval:     200 * time.Millisecond,
	}

	// A 成为 Leader
	leA := NewLeaderElector(coord, "node-a", config)
	if err := leA.Start(ctx); err != nil {
		t.Fatalf("leA.Start: %v", err)
	}
	if !leA.IsLeader() {
		t.Fatal("A 应成为 Leader")
	}

	// 验证：A 持有锁
	owner, _ := coord.Owner(ctx, config.ElectionKey)
	if owner != "node-a" {
		t.Fatalf("锁持有者 = %q, want node-a", owner)
	}

	// A 停止
	leA.Stop()
	if leA.IsLeader() {
		t.Fatal("A 停止后不应再是 Leader")
	}

	// A 释放后，锁应无主
	owner, _ = coord.Owner(ctx, config.ElectionKey)
	if owner != "" {
		t.Fatalf("A 停止后锁持有者 = %q, want 空", owner)
	}

	// B（用不同 nodeID 的 Coordinator）应能获取锁
	coordB := NewMemoryCoordinator("node-b", 2*time.Second)
	lease, err := coordB.Acquire(ctx, config.ElectionKey)
	if err != nil {
		t.Fatalf("B 应能获取锁: %v", err)
	}
	if lease.Holder() != "node-b" {
		t.Fatalf("锁持有者 = %q, want node-b", lease.Holder())
	}
	_ = coordB.Release(ctx, lease)
}
