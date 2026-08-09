package autonomy

import (
	"context"
	"testing"
	"time"
)

// mockListStepExecutor 列表测试用的步骤执行器
type mockListStepExecutor struct{}

func (m *mockListStepExecutor) ExecuteStep(_ context.Context, step PlanStep) (string, error) {
	return "ok:" + step.ID, nil
}

// TestAutonomyRuntime_ListGoals 验证 ListGoals 返回全部目标（新→旧）。
func TestAutonomyRuntime_ListGoals(t *testing.T) {
	rt := NewAutonomyRuntime(RuntimeConfig{StepExecutor: &mockListStepExecutor{}})

	rt.SubmitGoal("目标一", GoalConfig{})
	time.Sleep(2 * time.Millisecond)
	rt.SubmitGoal("目标二", GoalConfig{})

	goals := rt.ListGoals()
	if len(goals) != 2 {
		t.Fatalf("goals = %d, want 2", len(goals))
	}
	if goals[0].Description != "目标二" {
		t.Errorf("first goal = %q, want 目标二（新→旧）", goals[0].Description)
	}
	if goals[1].Description != "目标一" {
		t.Errorf("second goal = %q, want 目标一", goals[1].Description)
	}
}

// TestAgentGoal_Snapshot 验证快照并发安全读取：转换后快照反映最新状态。
func TestAgentGoal_Snapshot(t *testing.T) {
	g := NewAgentGoal("快照目标", GoalConfig{Priority: PriorityHigh})
	if err := g.TransitionTo(GoalPlanned); err != nil {
		t.Fatalf("transition: %v", err)
	}

	view := g.Snapshot()
	if view.ID != g.ID || view.Description != "快照目标" {
		t.Errorf("snapshot id/description mismatch")
	}
	if view.State != GoalPlanned {
		t.Errorf("snapshot state = %v, want planned", view.State)
	}
	if view.Priority != PriorityHigh {
		t.Errorf("snapshot priority = %v, want high", view.Priority)
	}
	if view.CreatedAt.IsZero() {
		t.Error("snapshot createdAt is zero")
	}
}
