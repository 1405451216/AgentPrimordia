package persist

import (
	"context"
	"testing"
	"time"
)

// ===== LeaderMetrics 测试 =====

func TestNewLeaderMetrics(t *testing.T) {
	m := NewLeaderMetrics()
	if m == nil {
		t.Fatal("NewLeaderMetrics 返回 nil")
	}
	snap := m.Snapshot()
	if snap.HeartbeatsSent != 0 || snap.HeartbeatsFailed != 0 {
		t.Error("新实例的计数器应为 0")
	}
}

func TestLeaderMetrics_NilSafe(t *testing.T) {
	var m *LeaderMetrics
	// 所有方法对 nil 接收者安全
	m.RecordHeartbeatSent()
	m.RecordHeartbeatFailed()
	m.RecordRenewFailure()
	m.RecordAcquireAttempt()
	m.RecordAcquireSuccess()
	m.RecordAcquireFailure()
	m.SetState(LeaderLeading)
	snap := m.Snapshot()
	if snap.HeartbeatsSent != 0 {
		t.Error("nil 接收者 Snapshot 应返回零值")
	}
}

func TestLeaderMetrics_Counters(t *testing.T) {
	m := NewLeaderMetrics()

	m.RecordHeartbeatSent()
	m.RecordHeartbeatSent()
	m.RecordHeartbeatSent()
	m.RecordHeartbeatFailed()
	m.RecordRenewFailure()
	m.RecordRenewFailure()
	m.RecordAcquireAttempt()
	m.RecordAcquireAttempt()
	m.RecordAcquireAttempt()
	m.RecordAcquireSuccess()
	m.RecordAcquireFailure()

	snap := m.Snapshot()
	if snap.HeartbeatsSent != 3 {
		t.Errorf("HeartbeatsSent = %d, want 3", snap.HeartbeatsSent)
	}
	if snap.HeartbeatsFailed != 1 {
		t.Errorf("HeartbeatsFailed = %d, want 1", snap.HeartbeatsFailed)
	}
	if snap.RenewFailures != 2 {
		t.Errorf("RenewFailures = %d, want 2", snap.RenewFailures)
	}
	if snap.AcquireAttempts != 3 {
		t.Errorf("AcquireAttempts = %d, want 3", snap.AcquireAttempts)
	}
	if snap.AcquireSuccesses != 1 {
		t.Errorf("AcquireSuccesses = %d, want 1", snap.AcquireSuccesses)
	}
	if snap.AcquireFailures != 1 {
		t.Errorf("AcquireFailures = %d, want 1", snap.AcquireFailures)
	}
}

func TestLeaderMetrics_SetState(t *testing.T) {
	m := NewLeaderMetrics()

	m.SetState(LeaderFollowing)
	snap := m.Snapshot()
	if snap.CurrentState != metricStateFollowing {
		t.Errorf("CurrentState = %d, want %d", snap.CurrentState, metricStateFollowing)
	}
	if snap.LeaderDurationSec != 0 {
		t.Error("Following 状态下 LeaderDurationSec 应为 0")
	}

	m.SetState(LeaderLeading)
	time.Sleep(2 * time.Millisecond) // 确保时间差可测量
	snap = m.Snapshot()
	if snap.CurrentState != metricStateLeading {
		t.Errorf("CurrentState = %d, want %d", snap.CurrentState, metricStateLeading)
	}
	if snap.StateChanges != 1 {
		t.Errorf("StateChanges = %d, want 1", snap.StateChanges)
	}
	if snap.LeaderDurationSec <= 0 {
		t.Error("Leading 状态下 LeaderDurationSec 应 > 0")
	}

	// 重复设置相同状态不增加 StateChanges
	m.SetState(LeaderLeading)
	snap = m.Snapshot()
	if snap.StateChanges != 1 {
		t.Errorf("StateChanges = %d, want 1（重复设置不应增加）", snap.StateChanges)
	}

	m.SetState(LeaderDegraded)
	snap = m.Snapshot()
	if snap.CurrentState != metricStateDegraded {
		t.Errorf("CurrentState = %d, want %d", snap.CurrentState, metricStateDegraded)
	}
	if snap.StateChanges != 2 {
		t.Errorf("StateChanges = %d, want 2", snap.StateChanges)
	}

	m.SetState(LeaderCandidate)
	snap = m.Snapshot()
	if snap.CurrentState != metricStateCandidate {
		t.Errorf("CurrentState = %d, want %d", snap.CurrentState, metricStateCandidate)
	}
}

func TestLeaderMetrics_WithMetrics(t *testing.T) {
	coord := NewMemoryCoordinator("node-a", 5*time.Second)
	config := DefaultLeaderConfig("test:metrics")
	le := NewLeaderElector(coord, "node-a", config)

	m := NewLeaderMetrics()
	returned := le.WithMetrics(m)
	if returned != le {
		t.Error("WithMetrics 应返回自身以支持链式调用")
	}
	if le.metrics != m {
		t.Error("WithMetrics 未正确注入 metrics")
	}
}

func TestLeaderStateToMetric(t *testing.T) {
	cases := []struct {
		state LeaderState
		want  int64
	}{
		{LeaderFollowing, metricStateFollowing},
		{LeaderLeading, metricStateLeading},
		{LeaderDegraded, metricStateDegraded},
		{LeaderCandidate, metricStateCandidate},
		{LeaderState("unknown"), metricStateFollowing},
	}
	for _, tc := range cases {
		got := leaderStateToMetric(tc.state)
		if got != tc.want {
			t.Errorf("leaderStateToMetric(%q) = %d, want %d", tc.state, got, tc.want)
		}
	}
}

// ===== FSCoordinator 测试 =====

func TestFSCoordinator_AcquireRelease(t *testing.T) {
	dir := t.TempDir()
	coord, err := NewFSCoordinator(dir, "node-1", 5*time.Second)
	if err != nil {
		t.Fatalf("NewFSCoordinator: %v", err)
	}

	ctx := context.Background()
	lease, err := coord.Acquire(ctx, "test-key")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// 验证 Lease 接口方法
	if lease.Key() != "test-key" {
		t.Errorf("Key() = %q, want %q", lease.Key(), "test-key")
	}
	if lease.Holder() != "node-1" {
		t.Errorf("Holder() = %q, want %q", lease.Holder(), "node-1")
	}
	if lease.ExpiresAt().Before(time.Now()) {
		t.Error("ExpiresAt() 应在当前时间之后")
	}

	// 验证 Owner
	owner, err := coord.Owner(ctx, "test-key")
	if err != nil {
		t.Fatalf("Owner: %v", err)
	}
	if owner != "node-1" {
		t.Errorf("Owner = %q, want %q", owner, "node-1")
	}

	// 续约
	if err := lease.Renew(ctx); err != nil {
		t.Fatalf("Renew: %v", err)
	}

	// 释放
	if err := coord.Release(ctx, lease); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// 释放后 Owner 应为空
	owner, err = coord.Owner(ctx, "test-key")
	if err != nil {
		t.Fatalf("Owner after release: %v", err)
	}
	if owner != "" {
		t.Errorf("Owner after release = %q, want empty", owner)
	}
}

func TestFSCoordinator_Reentrant(t *testing.T) {
	dir := t.TempDir()
	coord, err := NewFSCoordinator(dir, "node-1", 5*time.Second)
	if err != nil {
		t.Fatalf("NewFSCoordinator: %v", err)
	}

	ctx := context.Background()
	lease1, err := coord.Acquire(ctx, "reentrant-key")
	if err != nil {
		t.Fatalf("First Acquire: %v", err)
	}
	_ = lease1

	// 同节点重入应成功
	lease2, err := coord.Acquire(ctx, "reentrant-key")
	if err != nil {
		t.Fatalf("Reentrant Acquire: %v", err)
	}
	if lease2.Holder() != "node-1" {
		t.Errorf("Holder() = %q, want %q", lease2.Holder(), "node-1")
	}
}

func TestFSCoordinator_LockHeld(t *testing.T) {
	dir := t.TempDir()
	coord1, err := NewFSCoordinator(dir, "node-1", 5*time.Second)
	if err != nil {
		t.Fatalf("NewFSCoordinator node-1: %v", err)
	}
	coord2, err := NewFSCoordinator(dir, "node-2", 5*time.Second)
	if err != nil {
		t.Fatalf("NewFSCoordinator node-2: %v", err)
	}

	ctx := context.Background()
	_, err = coord1.Acquire(ctx, "contested-key")
	if err != nil {
		t.Fatalf("node-1 Acquire: %v", err)
	}

	// node-2 应获取失败
	_, err = coord2.Acquire(ctx, "contested-key")
	if err == nil {
		t.Fatal("node-2 Acquire 应失败（锁被持有）")
	}
	lockErr, ok := err.(*ErrLockHeld)
	if !ok {
		t.Fatalf("错误类型 = %T, want *ErrLockHeld", err)
	}
	if lockErr.Key != "contested-key" {
		t.Errorf("Key = %q, want %q", lockErr.Key, "contested-key")
	}
	if lockErr.Holder != "node-1" {
		t.Errorf("Holder = %q, want %q", lockErr.Holder, "node-1")
	}
}

func TestFSCoordinator_ExpiredLock(t *testing.T) {
	dir := t.TempDir()
	// 使用极短 TTL
	coord1, err := NewFSCoordinator(dir, "node-1", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("NewFSCoordinator node-1: %v", err)
	}

	ctx := context.Background()
	_, err = coord1.Acquire(ctx, "expire-key")
	if err != nil {
		t.Fatalf("node-1 Acquire: %v", err)
	}

	// 等待锁过期
	time.Sleep(100 * time.Millisecond)

	// node-2 应能获取已过期的锁
	coord2, err := NewFSCoordinator(dir, "node-2", 5*time.Second)
	if err != nil {
		t.Fatalf("NewFSCoordinator node-2: %v", err)
	}
	lease, err := coord2.Acquire(ctx, "expire-key")
	if err != nil {
		t.Fatalf("node-2 Acquire expired lock: %v", err)
	}
	if lease.Holder() != "node-2" {
		t.Errorf("Holder() = %q, want %q", lease.Holder(), "node-2")
	}
}

func TestFSCoordinator_DefaultTTL(t *testing.T) {
	dir := t.TempDir()
	// ttl <= 0 应使用默认 30s
	coord, err := NewFSCoordinator(dir, "node-1", 0)
	if err != nil {
		t.Fatalf("NewFSCoordinator: %v", err)
	}
	if coord == nil {
		t.Fatal("coord 不应为 nil")
	}
}

func TestFSCoordinator_ReleaseInvalidLease(t *testing.T) {
	dir := t.TempDir()
	coord, err := NewFSCoordinator(dir, "node-1", 5*time.Second)
	if err != nil {
		t.Fatalf("NewFSCoordinator: %v", err)
	}

	ctx := context.Background()
	// 传入非法 lease 类型
	err = coord.Release(ctx, &memoryLease{key: "x", holder: "y"})
	if err == nil {
		t.Error("Release 非法 lease 类型应返回错误")
	}
}

// ===== DistributedCheckpointStore 补充测试 =====

func TestDistributedCheckpointStore_ListAndDelete(t *testing.T) {
	ctx := context.Background()
	store, err := InMemoryCheckpointStore()
	if err != nil {
		t.Fatalf("InMemoryCheckpointStore: %v", err)
	}
	coord := NewMemoryCoordinator("node-a", 5*time.Second)

	d := NewDistributedCheckpointStore(store, coord, "node-a")

	// 保存多个检查点
	states := []*AgentState{
		{AgentID: "agent-1", SessionID: "sess-1", Status: "running"},
		{AgentID: "agent-2", SessionID: "sess-1", Status: "running"},
		{AgentID: "agent-3", SessionID: "sess-2", Status: "running"},
	}
	for _, s := range states {
		if err := d.Save(ctx, s); err != nil {
			t.Fatalf("Save %s: %v", s.AgentID, err)
		}
	}

	// List sess-1 应返回 2 个
	list, err := d.List(ctx, "sess-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List(sess-1) 长度 = %d, want 2", len(list))
	}

	// Delete agent-1
	if err := d.Delete(ctx, "agent-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// 验证已删除
	_, err = d.Load(ctx, "agent-1")
	if err == nil {
		t.Error("Load 已删除的 agent-1 应返回错误")
	}

	// List sess-1 应只剩 1 个
	list, err = d.List(ctx, "sess-1")
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List(sess-1) after delete 长度 = %d, want 1", len(list))
	}
}

func TestDistributedCheckpointStore_ReleaseNotHeld(t *testing.T) {
	ctx := context.Background()
	store, err := InMemoryCheckpointStore()
	if err != nil {
		t.Fatalf("InMemoryCheckpointStore: %v", err)
	}
	coord := NewMemoryCoordinator("node-a", 5*time.Second)

	d := NewDistributedCheckpointStore(store, coord, "node-a")

	// 释放未持有的锁不应报错
	if err := d.Release(ctx, "nonexistent"); err != nil {
		t.Errorf("Release 未持有的锁: %v", err)
	}
}
