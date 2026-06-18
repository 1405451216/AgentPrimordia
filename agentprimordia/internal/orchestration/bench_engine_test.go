package orchestration

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func makeBenchSteps(n int) []*AgentStep {
	steps := make([]*AgentStep, n)
	for i := 0; i < n; i++ {
		steps[i] = &AgentStep{ID: fmt.Sprintf("s%d", i), Name: fmt.Sprintf("step%d", i)}
	}
	return steps
}

func makeBenchChainEdges(n int) []DAGEdge {
	edges := make([]DAGEdge, 0, n-1)
	for i := 1; i < n; i++ {
		edges = append(edges, DAGEdge{From: fmt.Sprintf("s%d", i-1), To: fmt.Sprintf("s%d", i)})
	}
	return edges
}

func BenchmarkEngine_Sequential_10Steps(b *testing.B) {
	steps := makeBenchSteps(10)
	edges := makeBenchChainEdges(10)
	exec := StepExecutorFunc(func(ctx context.Context, step *AgentStep, input map[string]any) *StepResult {
		time.Sleep(time.Microsecond * 100)
		return &StepResult{StepID: step.ID, Status: StepCompleted}
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool := NewWorkerPool(1, exec)
		plan, _ := BuildExecutionPlan(SequentialMode, steps, edges)
		scheduler := NewScheduler(plan, pool, SchedulerConfig{MaxConcurrency: 1})
		_, _ = scheduler.Run(context.Background(), nil)
		pool.Stop()
	}
}

func BenchmarkEngine_Parallel_10Steps(b *testing.B) {
	steps := makeBenchSteps(10)
	exec := StepExecutorFunc(func(ctx context.Context, step *AgentStep, input map[string]any) *StepResult {
		time.Sleep(time.Microsecond * 100)
		return &StepResult{StepID: step.ID, Status: StepCompleted}
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool := NewWorkerPool(10, exec)
		plan, _ := BuildExecutionPlan(ParallelMode, steps, nil)
		scheduler := NewScheduler(plan, pool, SchedulerConfig{MaxConcurrency: 10})
		_, _ = scheduler.Run(context.Background(), nil)
		pool.Stop()
	}
}

func BenchmarkEngine_DAG_FanIn_7Steps(b *testing.B) {
	steps := makeBenchSteps(7)
	edges := []DAGEdge{
		{From: "s0", To: "s3"}, {From: "s1", To: "s3"}, {From: "s2", To: "s3"},
		{From: "s3", To: "s4"},
		{From: "s4", To: "s5"}, {From: "s4", To: "s6"},
	}
	exec := StepExecutorFunc(func(ctx context.Context, step *AgentStep, input map[string]any) *StepResult {
		time.Sleep(time.Microsecond * 100)
		return &StepResult{StepID: step.ID, Status: StepCompleted}
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool := NewWorkerPool(4, exec)
		plan, _ := BuildExecutionPlan(DAGMode, steps, edges)
		scheduler := NewScheduler(plan, pool, SchedulerConfig{MaxConcurrency: 4})
		_, _ = scheduler.Run(context.Background(), nil)
		pool.Stop()
	}
}
