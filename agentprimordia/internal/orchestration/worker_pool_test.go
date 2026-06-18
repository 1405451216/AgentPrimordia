package orchestration

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPool_FixedConcurrency(t *testing.T) {
	var running int64
	var maxRunning int64
	exec := StepExecutorFunc(func(ctx context.Context, step *AgentStep, input map[string]any) *StepResult {
		cur := atomic.AddInt64(&running, 1)
		if cur > atomic.LoadInt64(&maxRunning) {
			atomic.StoreInt64(&maxRunning, cur)
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt64(&running, -1)
		return &StepResult{StepID: step.ID, Status: StepCompleted}
	})

	pool := NewWorkerPool(2, exec)
	defer pool.Stop()

	resultCh := make(chan *StepResult, 5)
	for i := 0; i < 5; i++ {
		pool.Submit(context.Background(), &StepNode{Step: &AgentStep{ID: fmt.Sprintf("s%d", i)}}, nil, resultCh)
	}

	for i := 0; i < 5; i++ {
		<-resultCh
	}

	if atomic.LoadInt64(&maxRunning) > 2 {
		t.Errorf("max concurrent should be 2, got %d", maxRunning)
	}
}
