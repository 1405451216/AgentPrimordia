package autonomy

import (
	"context"
	"testing"
)

// TestRuntimeEndToEnd 验证运行时端到端流程
func TestRuntimeEndToEnd(t *testing.T) {
	mock := newMockStepExecutor()
	mock.results["s1"] = "data-collected"
	mock.results["s2"] = "analysis-done"

	rt := NewAutonomyRuntime(RuntimeConfig{
		StepExecutor: mock,
		MaxRetries:   2,
	})

	// 提交目标
	goal := rt.SubmitGoal("监控数据异常", GoalConfig{
		AcceptanceCriteria: []string{"异常归零"},
		Priority:           PriorityHigh,
	})

	// 设置计划
	plan := NewGoalPlan(goal.ID, []PlanStep{
		{ID: "s1", Description: "采集数据"},
		{ID: "s2", Description: "分析异常", DependsOn: []string{"s1"}},
	})
	if err := rt.SetPlan(goal.ID, plan); err != nil {
		t.Fatalf("set plan: %v", err)
	}

	// 执行
	if err := rt.ExecuteGoal(context.Background(), goal.ID); err != nil {
		t.Fatalf("execute: %v", err)
	}

	// 验证状态
	g, _ := rt.GetGoal(goal.ID)
	if g.State != GoalValidated {
		t.Errorf("goal state = %s, want validated", g.State)
	}

	// 完成目标
	if err := rt.CompleteGoal(goal.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if g.State != GoalDone {
		t.Errorf("goal state = %s, want done", g.State)
	}
}

// TestRuntimeWithCheckpoint 验证带检查点的运行时
func TestRuntimeWithCheckpoint(t *testing.T) {
	mock := newMockStepExecutor()
	mock.results["s1"] = "ok"

	store := newMockCheckpointStore()
	rt := NewAutonomyRuntime(RuntimeConfig{
		StepExecutor:    mock,
		CheckpointStore: store,
	})

	goal := rt.SubmitGoal("检查点测试", GoalConfig{})
	plan := NewGoalPlan(goal.ID, []PlanStep{
		{ID: "s1", Description: "唯一步骤"},
	})
	_ = rt.SetPlan(goal.ID, plan)
	_ = rt.ExecuteGoal(context.Background(), goal.ID)

	// 验证检查点已保存
	cp, err := rt.resume.LoadCheckpoint(context.Background(), goal.ID)
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if cp.LastCompletedStep != "s1" {
		t.Errorf("last completed = %q, want %q", cp.LastCompletedStep, "s1")
	}
}

// TestRuntimeResume 验证崩溃恢复流程
func TestRuntimeResume(t *testing.T) {
	store := newMockCheckpointStore()
	ctx := context.Background()

	// 模拟之前的执行留下了检查点
	plan := NewGoalPlan("goal-old", []PlanStep{
		{ID: "s1", Description: "已完成"},
		{ID: "s2", Description: "待执行"},
	})
	plan.MarkStepCompleted("s1")
	cp := &Checkpoint{
		GoalID:            "goal-old",
		State:             GoalExecuting,
		LastCompletedStep: "s1",
		PlanSnapshot:      plan,
		Completed:         false,
	}
	_ = store.SaveCheckpoint(ctx, cp)

	// 新运行时恢复
	mock := newMockStepExecutor()
	rt := NewAutonomyRuntime(RuntimeConfig{
		StepExecutor:    mock,
		CheckpointStore: store,
	})

	resumed, err := rt.ResumeIncomplete(ctx)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if len(resumed) != 1 || resumed[0] != "goal-old" {
		t.Errorf("resumed = %v, want [goal-old]", resumed)
	}

	// 验证计划已恢复
	p, ok := rt.GetPlan("goal-old")
	if !ok {
		t.Fatal("plan should be restored")
	}
	if p.GetStep("s1").Status != StepCompleted {
		t.Error("s1 should still be completed after resume")
	}
}
