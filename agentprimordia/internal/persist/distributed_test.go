package persist

import (
	"context"
	"errors"
	"testing"
	"time"
)

func sampleState(agentID, sessionID string) *AgentState {
	return &AgentState{
		AgentID:   agentID,
		SessionID: sessionID,
		Status:    "running",
		Messages:  []CheckpointMessage{{Role: "user", Content: "hello"}},
		TurnCount: 1,
		SavedAt:   time.Now(),
	}
}

func TestMemoryCoordinator_AcquireRelease(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCoordinator("node-a", time.Minute)

	lease, err := c.Acquire(ctx, "agent1")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if lease.Holder() != "node-a" {
		t.Fatalf("holder = %q, want node-a", lease.Holder())
	}

	owner, err := c.Owner(ctx, "agent1")
	if err != nil {
		t.Fatalf("Owner: %v", err)
	}
	if owner != "node-a" {
		t.Fatalf("owner = %q, want node-a", owner)
	}

	if _, err := c.Acquire(ctx, "agent1"); err != nil { // 同节点续约应成功
		t.Fatalf("re-Acquire: %v", err)
	}
	if err := c.Release(ctx, lease); err != nil {
		t.Fatalf("Release: %v", err)
	}
	owner, err = c.Owner(ctx, "agent1")
	if err != nil {
		t.Fatalf("Owner: %v", err)
	}
	if owner != "" {
		t.Fatalf("owner after release = %q, want empty", owner)
	}
}

func TestMemoryCoordinator_ExpiredReclaim(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCoordinator("node-a", time.Nanosecond) // 立即过期
	if _, err := c.Acquire(ctx, "eph"); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	owner, err := c.Owner(ctx, "eph")
	if err != nil {
		t.Fatalf("Owner: %v", err)
	}
	if owner != "" {
		t.Fatalf("过期后 owner = %q, want empty", owner)
	}
}

func TestFSCoordinator_CrossProcess(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cA, err := NewFSCoordinator(dir, "node-a", time.Minute)
	if err != nil {
		t.Fatalf("NewFSCoordinator A: %v", err)
	}
	cB, err := NewFSCoordinator(dir, "node-b", time.Minute)
	if err != nil {
		t.Fatalf("NewFSCoordinator B: %v", err)
	}

	if _, err := cA.Acquire(ctx, "agentFS"); err != nil {
		t.Fatalf("A Acquire: %v", err)
	}
	owner, err := cB.Owner(ctx, "agentFS")
	if err != nil {
		t.Fatalf("B Owner: %v", err)
	}
	if owner != "node-a" {
		t.Fatalf("owner = %q, want node-a（跨进程可见）", owner)
	}
	_, err = cB.Acquire(ctx, "agentFS")
	if err == nil {
		t.Fatal("B Acquire 应失败")
	}
	var held *ErrLockHeld
	if !errors.As(err, &held) {
		t.Fatalf("err = %v, want *ErrLockHeld", err)
	}
	if held.Holder != "node-a" {
		t.Fatalf("held.Holder = %q, want node-a", held.Holder)
	}
}

func TestFSCoordinator_ExpiredReclaim(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c, err := NewFSCoordinator(dir, "node-a", time.Nanosecond) // 极短 TTL，立即过期
	if err != nil {
		t.Fatalf("NewFSCoordinator: %v", err)
	}
	if _, err := c.Acquire(ctx, "agentE"); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	owner, err := c.Owner(ctx, "agentE")
	if err != nil {
		t.Fatalf("Owner: %v", err)
	}
	if owner != "" {
		t.Fatalf("过期后 owner = %q, want empty", owner)
	}
}

func TestDistributedCheckpointStore_SaveLoad(t *testing.T) {
	ctx := context.Background()
	base, err := InMemoryCheckpointStore()
	if err != nil {
		t.Fatalf("InMemoryCheckpointStore: %v", err)
	}
	coord := NewMemoryCoordinator("node-a", time.Minute)
	d := NewDistributedCheckpointStore(base, coord, "node-a")

	st := sampleState("agent1", "sess1")
	if err := d.Save(ctx, st); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := d.Load(ctx, "agent1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Messages[0].Content != "hello" || got.TurnCount != 1 {
		t.Fatalf("loaded state mismatch: %+v", got)
	}
}

func TestDistributedCheckpointStore_CrossNodeRecovery(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	baseA, err := InMemoryCheckpointStore()
	if err != nil {
		t.Fatalf("InMemoryCheckpointStore: %v", err)
	}
	coordA, err := NewFSCoordinator(dir, "node-a", time.Minute)
	if err != nil {
		t.Fatalf("coordA: %v", err)
	}
	dA := NewDistributedCheckpointStore(baseA, coordA, "node-a")

	st := sampleState("roamer", "sessR")
	if err := dA.Save(ctx, st); err != nil {
		t.Fatalf("dA.Save: %v", err)
	}

	coordB, err := NewFSCoordinator(dir, "node-b", time.Minute)
	if err != nil {
		t.Fatalf("coordB: %v", err)
	}
	dB := NewDistributedCheckpointStore(baseA, coordB, "node-b")

	owner, err := dB.Owner(ctx, "roamer")
	if err != nil {
		t.Fatalf("dB.Owner: %v", err)
	}
	if owner != "node-a" {
		t.Fatalf("owner = %q, want node-a", owner)
	}
	if err := dB.ForceTakeover(ctx, "roamer"); err == nil {
		t.Fatal("A 持锁未过期，B 不应能接管")
	}

	// A 释放（Release 内部释放锁）后，B 可接管
	if err := dA.Release(ctx, "roamer"); err != nil {
		t.Fatalf("dA.Release: %v", err)
	}
	if err := dB.ForceTakeover(ctx, "roamer"); err != nil {
		t.Fatalf("B ForceTakeover after release: %v", err)
	}
	if _, err := dB.Load(ctx, "roamer"); err != nil {
		t.Fatalf("dB.Load: %v", err)
	}
}
