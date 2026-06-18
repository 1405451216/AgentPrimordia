package orchestration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestScheduler_RunsAllSteps(t *testing.T) {
	steps := []*AgentStep{
		{ID: "a", Name: "a"},
		{ID: "b", Name: "b"},
		{ID: "c", Name: "c"},
	}
	plan, _ := BuildExecutionPlan(ParallelMode, steps, nil)

	exec := StepExecutorFunc(func(ctx context.Context, step *AgentStep, input map[string]any) *StepResult {
		return &StepResult{StepID: step.ID, Status: StepCompleted, Output: map[string]any{"k": step.ID}}
	})
	pool := NewWorkerPool(2, exec)
	defer pool.Stop()

	scheduler := NewScheduler(plan, pool, SchedulerConfig{MaxConcurrency: 2})
	results, err := scheduler.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	if results["a"].Status != StepCompleted {
		t.Errorf("a not completed")
	}
}

func TestScheduler_RespectsDependencies(t *testing.T) {
	steps := []*AgentStep{
		{ID: "a"},
		{ID: "b"},
		{ID: "c"},
	}
	edges := []DAGEdge{{From: "a", To: "c"}, {From: "b", To: "c"}}
	plan, _ := BuildExecutionPlan(DAGMode, steps, edges)

	exec := StepExecutorFunc(func(ctx context.Context, step *AgentStep, input map[string]any) *StepResult {
		return &StepResult{StepID: step.ID, Status: StepCompleted}
	})
	pool := NewWorkerPool(2, exec)
	defer pool.Stop()

	scheduler := NewScheduler(plan, pool, SchedulerConfig{MaxConcurrency: 2})
	results, err := scheduler.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results["c"].Status != StepCompleted {
		t.Errorf("c should be completed after a and b")
	}
}

func TestScheduler_RetryThenSuccess(t *testing.T) {
	steps := []*AgentStep{{ID: "r1"}}
	plan, _ := BuildExecutionPlan(ParallelMode, steps, nil)

	attempts := 0
	exec := StepExecutorFunc(func(ctx context.Context, step *AgentStep, input map[string]any) *StepResult {
		attempts++
		if attempts < 3 {
			return &StepResult{StepID: step.ID, Status: StepFailed, Error: fmt.Errorf("fail")}
		}
		return &StepResult{StepID: step.ID, Status: StepCompleted}
	})
	pool := NewWorkerPool(1, exec)
	defer pool.Stop()

	scheduler := NewScheduler(plan, pool, SchedulerConfig{
		MaxConcurrency: 1,
		RetryPolicy:    RetryPolicy{MaxRetries: 3, Backoff: 10 * time.Millisecond},
	})
	results, err := scheduler.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results["r1"].Status != StepCompleted {
		t.Errorf("expected success after retry, got %s", results["r1"].Status)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}
