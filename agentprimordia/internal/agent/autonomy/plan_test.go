package autonomy

import (
	"testing"
)

// TestNewGoalPlan 验证计划创建
func TestNewGoalPlan(t *testing.T) {
	steps := []PlanStep{
		{ID: "s1", Description: "采集数据", Strategy: StepStrategySequential},
		{ID: "s2", Description: "分析异常", DependsOn: []string{"s1"}, Strategy: StepStrategySequential},
		{ID: "s3", Description: "修复数据", DependsOn: []string{"s2"}, Strategy: StepStrategySequential},
	}
	plan := NewGoalPlan("goal-1", steps)

	if plan.GoalID != "goal-1" {
		t.Errorf("goal id = %q, want %q", plan.GoalID, "goal-1")
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("steps len = %d, want 3", len(plan.Steps))
	}
	if plan.Version != 1 {
		t.Errorf("version = %d, want 1", plan.Version)
	}
	if plan.CreatedAt.IsZero() {
		t.Error("created at should not be zero")
	}
}

// TestPlanStepStatus 验证步骤状态流转
func TestPlanStepStatus(t *testing.T) {
	// 通过 NewGoalPlan 创建以确保状态初始化
	plan := NewGoalPlan("g", []PlanStep{{ID: "s1", Description: "测试步骤"}})
	step := plan.GetStep("s1")
	if step.Status != StepPending {
		t.Errorf("initial status = %s, want pending", step.Status)
	}

	plan.MarkStepRunning("s1")
	step = plan.GetStep("s1")
	if step.Status != StepRunning {
		t.Errorf("status = %s, want running", step.Status)
	}

	plan.MarkStepCompleted("s1")
	step = plan.GetStep("s1")
	if step.Status != StepCompleted {
		t.Errorf("status = %s, want completed", step.Status)
	}
	if step.Duration() < 0 {
		t.Error("duration should not be negative")
	}
}

// TestPlanReadySteps 验证就绪步骤计算
func TestPlanReadySteps(t *testing.T) {
	steps := []PlanStep{
		{ID: "s1", Description: "无依赖"},
		{ID: "s2", Description: "依赖s1", DependsOn: []string{"s1"}},
		{ID: "s3", Description: "依赖s1", DependsOn: []string{"s1"}},
		{ID: "s4", Description: "依赖s2和s3", DependsOn: []string{"s2", "s3"}},
	}
	plan := NewGoalPlan("goal-1", steps)

	// 初始只有 s1 就绪
	ready := plan.ReadySteps()
	if len(ready) != 1 || ready[0].ID != "s1" {
		t.Fatalf("initial ready = %v, want [s1]", stepIDs(ready))
	}

	// 完成 s1 后，s2 和 s3 就绪
	plan.MarkStepCompleted("s1")
	ready = plan.ReadySteps()
	if len(ready) != 2 {
		t.Fatalf("after s1 done, ready = %v, want [s2, s3]", stepIDs(ready))
	}

	// 完成 s2 和 s3 后，s4 就绪
	plan.MarkStepCompleted("s2")
	plan.MarkStepCompleted("s3")
	ready = plan.ReadySteps()
	if len(ready) != 1 || ready[0].ID != "s4" {
		t.Fatalf("after s2,s3 done, ready = %v, want [s4]", stepIDs(ready))
	}
}

// TestPlanProgress 验证进度计算
func TestPlanProgress(t *testing.T) {
	steps := []PlanStep{
		{ID: "s1", Description: "步骤1"},
		{ID: "s2", Description: "步骤2"},
		{ID: "s3", Description: "步骤3"},
		{ID: "s4", Description: "步骤4"},
	}
	plan := NewGoalPlan("goal-1", steps)

	if plan.Progress() != 0 {
		t.Errorf("initial progress = %f, want 0", plan.Progress())
	}

	plan.MarkStepCompleted("s1")
	plan.MarkStepCompleted("s2")
	if p := plan.Progress(); p != 0.5 {
		t.Errorf("progress = %f, want 0.5", p)
	}

	plan.MarkStepCompleted("s3")
	plan.MarkStepCompleted("s4")
	if p := plan.Progress(); p != 1.0 {
		t.Errorf("progress = %f, want 1.0", p)
	}
}

// TestPlanIsComplete 验证完成判断
func TestPlanIsComplete(t *testing.T) {
	steps := []PlanStep{
		{ID: "s1", Description: "步骤1"},
		{ID: "s2", Description: "步骤2"},
	}
	plan := NewGoalPlan("goal-1", steps)

	if plan.IsComplete() {
		t.Error("plan should not be complete initially")
	}

	plan.MarkStepCompleted("s1")
	if plan.IsComplete() {
		t.Error("plan should not be complete with 1/2 steps done")
	}

	plan.MarkStepCompleted("s2")
	if !plan.IsComplete() {
		t.Error("plan should be complete with all steps done")
	}
}

// TestPlanMarkStepFailed 验证步骤失败标记
func TestPlanMarkStepFailed(t *testing.T) {
	steps := []PlanStep{
		{ID: "s1", Description: "步骤1"},
	}
	plan := NewGoalPlan("goal-1", steps)

	plan.MarkStepFailed("s1", "连接超时")
	step := plan.GetStep("s1")
	if step.Status != StepFailed {
		t.Errorf("step status = %s, want failed", step.Status)
	}
	if step.Error != "连接超时" {
		t.Errorf("step error = %q, want %q", step.Error, "连接超时")
	}
}

// TestPlanRemainingSteps 验证剩余步骤
func TestPlanRemainingSteps(t *testing.T) {
	steps := []PlanStep{
		{ID: "s1", Description: "步骤1"},
		{ID: "s2", Description: "步骤2"},
		{ID: "s3", Description: "步骤3"},
	}
	plan := NewGoalPlan("goal-1", steps)

	plan.MarkStepCompleted("s1")
	remaining := plan.RemainingSteps()
	if len(remaining) != 2 {
		t.Fatalf("remaining = %v, want [s2, s3]", stepIDs(remaining))
	}
}

// TestPlanValidation 验证计划校验（循环依赖检测）
func TestPlanValidation(t *testing.T) {
	// 正常计划
	valid := NewGoalPlan("g1", []PlanStep{
		{ID: "s1", Description: "a"},
		{ID: "s2", Description: "b", DependsOn: []string{"s1"}},
	})
	if err := valid.Validate(); err != nil {
		t.Errorf("valid plan should pass: %v", err)
	}

	// 循环依赖
	cyclic := NewGoalPlan("g2", []PlanStep{
		{ID: "s1", Description: "a", DependsOn: []string{"s2"}},
		{ID: "s2", Description: "b", DependsOn: []string{"s1"}},
	})
	if err := cyclic.Validate(); err == nil {
		t.Error("cyclic plan should fail validation")
	}

	// 空计划
	empty := NewGoalPlan("g3", nil)
	if err := empty.Validate(); err == nil {
		t.Error("empty plan should fail validation")
	}
}

func stepIDs(steps []PlanStep) []string {
	ids := make([]string, len(steps))
	for i, s := range steps {
		ids[i] = s.ID
	}
	return ids
}
