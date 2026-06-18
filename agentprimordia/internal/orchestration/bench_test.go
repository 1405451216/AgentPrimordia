// perf-v4 Task 12.6：Supervisor Select 策略性能基线
package orchestration

import (
	"context"
	"testing"
)

// benchWorker 用于基准测试的最小化 Worker 实现
type benchWorker struct {
	id string
}

func (w *benchWorker) ID() string { return w.id }
func (w *benchWorker) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
	return &TaskResult{WorkerID: w.id, TaskID: task.ID, Status: TaskStatusCompleted}, nil
}

// makeBenchWorkers 构造 n 个可用 worker
func makeBenchWorkers(n int) []*WorkerState {
	workers := make([]*WorkerState, n)
	for i := 0; i < n; i++ {
		workers[i] = &WorkerState{
			Worker: &benchWorker{id: "worker_" + string(rune('a'+i%26))},
			ID:     "w" + string(rune('a'+i%26)),
			Skills: []string{"general"},
		}
		workers[i].available = true
	}
	return workers
}

// BenchmarkSupervisor_Select_RoundRobin 轮询策略（10 worker）
func BenchmarkSupervisor_Select_RoundRobin(b *testing.B) {
	strategy := NewRoundRobinStrategy()
	workers := makeBenchWorkers(10)
	task := &Task{ID: "t1", Type: "test"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = strategy.Select(task, workers)
	}
}

// BenchmarkSupervisor_Select_LoadBalanced 负载均衡策略（10 worker）
func BenchmarkSupervisor_Select_LoadBalanced(b *testing.B) {
	strategy := NewLoadBalancedStrategy()
	workers := makeBenchWorkers(10)
	task := &Task{ID: "t1", Type: "test"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = strategy.Select(task, workers)
	}
}

// BenchmarkSupervisor_Select_SkillBased 技能匹配策略（10 worker）
func BenchmarkSupervisor_Select_SkillBased(b *testing.B) {
	strategy := NewSkillBasedStrategy()
	workers := makeBenchWorkers(10)
	task := &Task{ID: "t1", Type: "test", RequiredSkills: []string{"general"}}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = strategy.Select(task, workers)
	}
}

// BenchmarkSupervisor_Select_RoundRobin_50Workers 50 worker 轮询
func BenchmarkSupervisor_Select_RoundRobin_50Workers(b *testing.B) {
	strategy := NewRoundRobinStrategy()
	workers := makeBenchWorkers(50)
	task := &Task{ID: "t1", Type: "test"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = strategy.Select(task, workers)
	}
}
