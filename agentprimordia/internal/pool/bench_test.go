package pool

import (
	"context"
	"fmt"
	"testing"

	"agentprimordia/internal/llm"
)

// makeBenchTasks 生成指定数量的 TaskConfig
func makeBenchTasks(n int) []TaskConfig {
	tasks := make([]TaskConfig, n)
	for i := 0; i < n; i++ {
		tasks[i] = TaskConfig{
			ID:     fmt.Sprintf("bench-task-%d", i),
			Title:  fmt.Sprintf("Bench Task %d", i),
			Prompt: "benchmark prompt",
		}
	}
	return tasks
}

// BenchmarkPool_Dispatch_10Agents 测试 10 个并发任务的调度开销
func BenchmarkPool_Dispatch_10Agents(b *testing.B) {
	b.ReportAllocs()

	tasks := makeBenchTasks(10)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		mockLLM := llm.NewMockLLM(nil)
		for j := 0; j < 10; j++ {
			mockLLM.WithResponse("done")
		}

		pool := NewPool(PoolConfig{
			MaxConcurrency: 10,
		})
		pool.SetModel(mockLLM)
		b.StartTimer()

		_, err := pool.Dispatch(context.Background(), tasks)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		pool.Close()
	}
}

// BenchmarkPool_Dispatch_100Agents 测试 100 个并发任务的调度开销
func BenchmarkPool_Dispatch_100Agents(b *testing.B) {
	b.ReportAllocs()

	tasks := makeBenchTasks(100)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		mockLLM := llm.NewMockLLM(nil)
		for j := 0; j < 100; j++ {
			mockLLM.WithResponse("done")
		}

		pool := NewPool(PoolConfig{
			MaxConcurrency: 100,
		})
		pool.SetModel(mockLLM)
		b.StartTimer()

		_, err := pool.Dispatch(context.Background(), tasks)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		pool.Close()
	}
}
