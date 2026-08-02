package autonomy

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockStepExecutor 模拟步骤执行器
type mockStepExecutor struct {
	mu        sync.Mutex
	results   map[string]string
	errors    map[string]string
	callCount map[string]int
	delay     time.Duration
}

func newMockStepExecutor() *mockStepExecutor {
	return &mockStepExecutor{
		results:   make(map[string]string),
		errors:    make(map[string]string),
		callCount: make(map[string]int),
	}
}

func (m *mockStepExecutor) ExecuteStep(ctx context.Context, step PlanStep) (string, error) {
	m.mu.Lock()
	m.callCount[step.ID]++
	m.mu.Unlock()

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err, ok := m.errors[step.ID]; ok {
		return "", fmt.Errorf("%s", err)
	}
	return m.results[step.ID], nil
}

// TestExecutorSequential 验证顺序执行
func TestExecutorSequential(t *testing.T) {
	mock := newMockStepExecutor()
	mock.results["s1"] = "result-1"
	mock.results["s2"] = "result-2"

	steps := []PlanStep{
		{ID: "s1", Description: "步骤1"},
		{ID: "s2", Description: "步骤2", DependsOn: []string{"s1"}},
	}
	plan := NewGoalPlan("goal-1", steps)

	exec := NewGoalExecutor(GoalExecutorConfig{
		StepExecutor: mock,
		MaxRetries:   1,
	})

	err := exec.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !plan.IsComplete() {
		t.Error("plan should be complete")
	}
	s1 := plan.GetStep("s1")
	if s1.Result != "result-1" {
		t.Errorf("s1 result = %q, want %q", s1.Result, "result-1")
	}
}

// TestExecutorParallel 验证并行步骤执行
func TestExecutorParallel(t *testing.T) {
	mock := newMockStepExecutor()
	mock.results["s1"] = "r1"
	mock.results["s2"] = "r2"
	mock.results["s3"] = "r3"
	mock.delay = 50 * time.Millisecond

	steps := []PlanStep{
		{ID: "s1", Description: "独立步骤1", Strategy: StepStrategyParallel},
		{ID: "s2", Description: "独立步骤2", Strategy: StepStrategyParallel},
		{ID: "s3", Description: "汇总步骤", DependsOn: []string{"s1", "s2"}},
	}
	plan := NewGoalPlan("goal-1", steps)

	exec := NewGoalExecutor(GoalExecutorConfig{
		StepExecutor: mock,
		MaxRetries:   1,
	})

	start := time.Now()
	err := exec.Execute(context.Background(), plan)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !plan.IsComplete() {
		t.Error("plan should be complete")
	}
	// 并行执行应比顺序快（2*50ms 并行 < 3*50ms 顺序）
	if elapsed > 200*time.Millisecond {
		t.Errorf("parallel execution took %v, expected < 200ms", elapsed)
	}
}

// TestExecutorRetry 验证步骤失败重试
func TestExecutorRetry(t *testing.T) {
	mock2 := &failOnceExecutor{
		failIDs: map[string]bool{"s1": true},
	}

	steps := []PlanStep{
		{ID: "s1", Description: "会失败的步骤"},
	}
	plan := NewGoalPlan("goal-1", steps)

	exec := NewGoalExecutor(GoalExecutorConfig{
		StepExecutor: mock2,
		MaxRetries:   3,
	})

	err := exec.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("execute with retry: %v", err)
	}
	if !plan.IsComplete() {
		t.Error("plan should complete after retry")
	}
}

// TestExecutorContextCancel 验证上下文取消
func TestExecutorContextCancel(t *testing.T) {
	mock := newMockStepExecutor()
	mock.delay = 5 * time.Second

	steps := []PlanStep{
		{ID: "s1", Description: "慢步骤"},
	}
	plan := NewGoalPlan("goal-1", steps)

	exec := NewGoalExecutor(GoalExecutorConfig{
		StepExecutor: mock,
		MaxRetries:   1,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := exec.Execute(ctx, plan)
	if err == nil {
		t.Fatal("expected error on context cancel")
	}
}

// TestExecutorStepFailure 验证步骤最终失败（重试耗尽）
func TestExecutorStepFailure(t *testing.T) {
	mock := &alwaysFailExecutor{}

	steps := []PlanStep{
		{ID: "s1", Description: "永远失败"},
	}
	plan := NewGoalPlan("goal-1", steps)

	exec := NewGoalExecutor(GoalExecutorConfig{
		StepExecutor: mock,
		MaxRetries:   2,
	})

	err := exec.Execute(context.Background(), plan)
	if err == nil {
		t.Fatal("expected error when retries exhausted")
	}
	step := plan.GetStep("s1")
	if step.Status != StepFailed {
		t.Errorf("step status = %s, want failed", step.Status)
	}
}

// --- 辅助 mock ---

// failOnceExecutor 第一次调用失败，后续成功
type failOnceExecutor struct {
	failIDs  map[string]bool
	failed   map[string]bool
	mu       sync.Mutex
}

func (f *failOnceExecutor) ExecuteStep(ctx context.Context, step PlanStep) (string, error) {
	f.mu.Lock()
	if f.failed == nil {
		f.failed = make(map[string]bool)
	}
	shouldFail := f.failIDs[step.ID] && !f.failed[step.ID]
	if shouldFail {
		f.failed[step.ID] = true
	}
	f.mu.Unlock()

	if shouldFail {
		return "", fmt.Errorf("模拟临时失败")
	}
	return "retry-ok", nil
}

// alwaysFailExecutor 总是失败
type alwaysFailExecutor struct{}

func (a *alwaysFailExecutor) ExecuteStep(ctx context.Context, step PlanStep) (string, error) {
	return "", fmt.Errorf("永久失败")
}
