package autonomy

import (
	"context"
	"fmt"
	"testing"
)

// mockPlanner 模拟规划器
type mockPlanner struct {
	plans map[string][]PlanStep
}

func (m *mockPlanner) Replan(ctx context.Context, goal *AgentGoal, failedSteps []PlanStep, reason string) ([]PlanStep, error) {
	steps, ok := m.plans[goal.ID]
	if !ok {
		return nil, fmt.Errorf("no plan for goal %s", goal.ID)
	}
	return steps, nil
}

// TestReplanTrigger 验证重规划触发
func TestReplanTrigger(t *testing.T) {
	planner := &mockPlanner{
		plans: map[string][]PlanStep{
			"goal-1": {
				{ID: "s2-new", Description: "替代方案"},
			},
		},
	}

	rp := NewReplanner(ReplannerConfig{
		Planner:      planner,
		MaxReplans:   3,
	})

	goal := NewAgentGoal("测试重规划", GoalConfig{})
	goal.ID = "goal-1"
	_ = goal.TransitionTo(GoalPlanned)
	_ = goal.TransitionTo(GoalExecuting)
	_ = goal.TransitionTo(GoalValidated)

	failedSteps := []PlanStep{
		{ID: "s1", Description: "失败步骤", Status: StepFailed, Error: "超时"},
	}

	newPlan, err := rp.Trigger(context.Background(), goal, failedSteps, "校验不通过")
	if err != nil {
		t.Fatalf("trigger replan: %v", err)
	}
	if newPlan == nil {
		t.Fatal("new plan should not be nil")
	}
	if len(newPlan.Steps) != 1 || newPlan.Steps[0].ID != "s2-new" {
		t.Errorf("new plan steps = %v, want [s2-new]", newPlan.Steps)
	}
	if newPlan.Version != 2 {
		t.Errorf("new plan version = %d, want 2", newPlan.Version)
	}
	if newPlan.ReplanReason != "校验不通过" {
		t.Errorf("replan reason = %q, want %q", newPlan.ReplanReason, "校验不通过")
	}
}

// TestReplanLimit 验证重规划次数限制
func TestReplanLimit(t *testing.T) {
	planner := &mockPlanner{
		plans: map[string][]PlanStep{
			"goal-1": {{ID: "s1", Description: "x"}},
		},
	}

	rp := NewReplanner(ReplannerConfig{
		Planner:    planner,
		MaxReplans: 2,
	})

	goal := NewAgentGoal("限制测试", GoalConfig{})
	goal.ID = "goal-1"

	failed := []PlanStep{{ID: "s1", Status: StepFailed}}

	// 前两次成功
	_, err := rp.Trigger(context.Background(), goal, failed, "r1")
	if err != nil {
		t.Fatalf("replan 1: %v", err)
	}
	_, err = rp.Trigger(context.Background(), goal, failed, "r2")
	if err != nil {
		t.Fatalf("replan 2: %v", err)
	}

	// 第三次超限
	_, err = rp.Trigger(context.Background(), goal, failed, "r3")
	if err == nil {
		t.Fatal("expected error on exceeding max replans")
	}
}

// TestReplanHistory 验证重规划历史记录
func TestReplanHistory(t *testing.T) {
	planner := &mockPlanner{
		plans: map[string][]PlanStep{
			"goal-1": {{ID: "s1", Description: "x"}},
		},
	}

	rp := NewReplanner(ReplannerConfig{
		Planner:    planner,
		MaxReplans: 5,
	})

	goal := NewAgentGoal("历史测试", GoalConfig{})
	goal.ID = "goal-1"
	failed := []PlanStep{{ID: "s1", Status: StepFailed}}

	_, _ = rp.Trigger(context.Background(), goal, failed, "原因A")
	_, _ = rp.Trigger(context.Background(), goal, failed, "原因B")

	history := rp.History()
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].Reason != "原因A" {
		t.Errorf("history[0].Reason = %q, want %q", history[0].Reason, "原因A")
	}
	if history[1].Reason != "原因B" {
		t.Errorf("history[1].Reason = %q, want %q", history[1].Reason, "原因B")
	}
}

// TestValidator 验证结果校验器
func TestValidator(t *testing.T) {
	v := NewValidator()

	// 全部通过
	result1 := v.Validate([]string{"标准1", "标准2"}, map[string]bool{
		"标准1": true,
		"标准2": true,
	})
	if !result1.Passed {
		t.Error("should pass when all criteria met")
	}

	// 部分不通过
	result2 := v.Validate([]string{"标准1", "标准2"}, map[string]bool{
		"标准1": true,
		"标准2": false,
	})
	if result2.Passed {
		t.Error("should fail when criteria not met")
	}
	if len(result2.FailedCriteria) != 1 || result2.FailedCriteria[0] != "标准2" {
		t.Errorf("failed criteria = %v, want [标准2]", result2.FailedCriteria)
	}
}
