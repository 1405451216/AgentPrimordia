package autonomy

import (
	"context"
	"errors"
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

// TestReplanBudgetEnforced v4.9-4：目标级预算耗尽 → 重规划被拒（ErrGoalBudgetExceeded）。
func TestReplanBudgetEnforced(t *testing.T) {
	goal := NewAgentGoal("预算目标", GoalConfig{
		BudgetUSD:     0.02,
		ReplanCostUSD: 0.01, // 每次重规划 0.01 → 2 次后耗尽
	})
	planner := &mockPlanner{plans: map[string][]PlanStep{
		goal.ID: {{ID: "s1", Description: "x"}},
	}}
	replanner := NewReplanner(ReplannerConfig{Planner: planner, MaxReplans: 5})
	failed := []PlanStep{{ID: "s1", Description: "失败"}}

	if _, err := replanner.Trigger(context.Background(), goal, failed, "第一次"); err != nil {
		t.Fatalf("第一次重规划应成功: %v", err)
	}
	if _, err := replanner.Trigger(context.Background(), goal, failed, "第二次"); err != nil {
		t.Fatalf("第二次重规划应成功: %v", err)
	}
	if goal.CostSpent() != 0.02 {
		t.Errorf("CostSpent = %v, want 0.02", goal.CostSpent())
	}
	if _, err := replanner.Trigger(context.Background(), goal, failed, "第三次"); !errors.Is(err, ErrGoalBudgetExceeded) {
		t.Fatalf("第三次重规划 err = %v, want ErrGoalBudgetExceeded", err)
	}
}

// TestReplanBudgetUnlimited 无预算 → 不限制。
func TestReplanBudgetUnlimited(t *testing.T) {
	goal := NewAgentGoal("免费目标", GoalConfig{}) // BudgetUSD=0 → 不限
	planner := &mockPlanner{plans: map[string][]PlanStep{
		goal.ID: {{ID: "s1", Description: "x"}},
	}}
	replanner := NewReplanner(ReplannerConfig{Planner: planner, MaxReplans: 3})
	failed := []PlanStep{{ID: "s1", Description: "失败"}}
	for i := 0; i < 3; i++ {
		if _, err := replanner.Trigger(context.Background(), goal, failed, "重试"); err != nil {
			t.Fatalf("第 %d 次重规划: %v", i+1, err)
		}
	}
}
