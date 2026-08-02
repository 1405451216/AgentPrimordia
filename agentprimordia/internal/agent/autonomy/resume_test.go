package autonomy

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockCheckpointStore 模拟检查点存储
type mockCheckpointStore struct {
	mu         sync.Mutex
	checkpoints map[string]*Checkpoint
}

func newMockCheckpointStore() *mockCheckpointStore {
	return &mockCheckpointStore{
		checkpoints: make(map[string]*Checkpoint),
	}
}

func (m *mockCheckpointStore) SaveCheckpoint(ctx context.Context, cp *Checkpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkpoints[cp.GoalID] = cp
	return nil
}

func (m *mockCheckpointStore) LoadCheckpoint(ctx context.Context, goalID string) (*Checkpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp, ok := m.checkpoints[goalID]
	if !ok {
		return nil, fmt.Errorf("checkpoint not found for goal %s", goalID)
	}
	return cp, nil
}

func (m *mockCheckpointStore) ListIncomplete(ctx context.Context) ([]*Checkpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*Checkpoint
	for _, cp := range m.checkpoints {
		if !cp.Completed {
			result = append(result, cp)
		}
	}
	return result, nil
}

// TestCheckpointSaveLoad 验证检查点存取
func TestCheckpointSaveLoad(t *testing.T) {
	store := newMockCheckpointStore()
	rm := NewResumeManager(store)
	ctx := context.Background()

	plan := NewGoalPlan("goal-1", []PlanStep{
		{ID: "s1", Description: "步骤1"},
		{ID: "s2", Description: "步骤2"},
	})
	plan.MarkStepCompleted("s1")

	err := rm.SaveCheckpoint(ctx, "goal-1", plan, GoalExecuting)
	if err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	cp, err := rm.LoadCheckpoint(ctx, "goal-1")
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if cp.GoalID != "goal-1" {
		t.Errorf("goal id = %q, want %q", cp.GoalID, "goal-1")
	}
	if cp.State != GoalExecuting {
		t.Errorf("state = %s, want executing", cp.State)
	}
	if cp.LastCompletedStep != "s1" {
		t.Errorf("last completed = %q, want %q", cp.LastCompletedStep, "s1")
	}
}

// TestCrashRecovery 验证崩溃恢复
func TestCrashRecovery(t *testing.T) {
	store := newMockCheckpointStore()
	rm := NewResumeManager(store)
	ctx := context.Background()

	// 模拟两个未完成目标
	plan1 := NewGoalPlan("goal-1", []PlanStep{
		{ID: "s1", Description: "a"},
		{ID: "s2", Description: "b"},
	})
	plan1.MarkStepCompleted("s1")
	_ = rm.SaveCheckpoint(ctx, "goal-1", plan1, GoalExecuting)

	plan2 := NewGoalPlan("goal-2", []PlanStep{
		{ID: "s1", Description: "x"},
	})
	_ = rm.SaveCheckpoint(ctx, "goal-2", plan2, GoalPlanned)

	// 扫描未完成目标
	incomplete, err := rm.ScanIncomplete(ctx)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(incomplete) != 2 {
		t.Fatalf("incomplete = %d, want 2", len(incomplete))
	}
}

// TestResumeValidation 验证恢复一致性
func TestResumeValidation(t *testing.T) {
	store := newMockCheckpointStore()
	rm := NewResumeManager(store)
	ctx := context.Background()

	// 正常检查点
	plan := NewGoalPlan("goal-1", []PlanStep{
		{ID: "s1", Description: "a"},
		{ID: "s2", Description: "b", DependsOn: []string{"s1"}},
	})
	plan.MarkStepCompleted("s1")
	_ = rm.SaveCheckpoint(ctx, "goal-1", plan, GoalExecuting)

	cp, _ := rm.LoadCheckpoint(ctx, "goal-1")
	err := rm.ValidateConsistency(cp)
	if err != nil {
		t.Fatalf("consistency check: %v", err)
	}

	// 不一致：lastCompletedStep 不在计划中
	badCp := &Checkpoint{
		GoalID:            "goal-bad",
		LastCompletedStep: "nonexist",
		PlanSnapshot:      plan,
	}
	err = rm.ValidateConsistency(badCp)
	if err == nil {
		t.Error("expected error for inconsistent checkpoint")
	}
}

// TestCheckpointTimestamp 验证检查点时间戳
func TestCheckpointTimestamp(t *testing.T) {
	store := newMockCheckpointStore()
	rm := NewResumeManager(store)
	ctx := context.Background()

	plan := NewGoalPlan("goal-1", []PlanStep{{ID: "s1", Description: "a"}})
	_ = rm.SaveCheckpoint(ctx, "goal-1", plan, GoalExecuting)

	cp, _ := rm.LoadCheckpoint(ctx, "goal-1")
	if cp.Timestamp.IsZero() {
		t.Error("checkpoint timestamp should not be zero")
	}
	if time.Since(cp.Timestamp) > time.Second {
		t.Error("checkpoint timestamp should be recent")
	}
}
