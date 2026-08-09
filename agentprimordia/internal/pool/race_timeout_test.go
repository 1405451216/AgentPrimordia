package pool

// race_timeout_test.go — 并发安全回归测试（评估报告 2026-08-09 §三.1）
//
// 覆盖两类问题：
//   1. acquireSlot 超时绕过：等待槽位的 goroutine 在 Timeout 到期后不被唤醒，
//      必须依赖其他事件（releaseSlot/ctx 取消）才能返回——本文件 TestPool_AcquireSlotTimeout
//      验证等待者应在 Timeout 内自行失败。
//   2. 数据竞争回归（CI -race 下生效）：并发 Dispatch/GetTask/ListTasks/SetModel
//      不应有 unsynchronized 读写（dispatcher.go pt.result / p.model / p.toolkit）。
//      本机（Windows 无 gcc）无 race 检测器时仅作压力冒烟，语义验证依赖 CI。

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"agentprimordia/internal/agent"
	"agentprimordia/internal/llm"
)

// fakeProvider 无 *testing.T 依赖的假 LLM Provider，供并发测试在 goroutine 中使用
// （MockLLM 内部引用 testing.T，并发调用不安全，是并发测试偶发 flake 的来源）。
type fakeProvider struct{}

func (fakeProvider) Complete(_ context.Context, _ *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	return &llm.CompletionResponse{Content: "ok"}, nil
}

func (fakeProvider) Stream(_ context.Context, _ *llm.CompletionRequest) (<-chan llm.Chunk, error) {
	ch := make(chan llm.Chunk, 1)
	ch <- llm.Chunk{Content: "ok", Done: true}
	close(ch)
	return ch, nil
}

func (fakeProvider) CallTools(_ context.Context, _ *llm.ToolCallRequest) (*llm.ToolCallResponse, error) {
	return &llm.ToolCallResponse{Content: ""}, nil
}

func (fakeProvider) Info() llm.ModelInfo {
	return llm.ModelInfo{Name: "fake", Provider: "fake", MaxContext: 1024}
}

// blockingAgent 阻塞在 release channel 上，用于占据 pool 槽位。
type blockingAgent struct {
	release chan struct{}
}

func (a *blockingAgent) Run(ctx context.Context, _ agent.Message) (*agent.Response, error) {
	select {
	case <-a.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &agent.Response{Content: "ok"}, nil
}

func (a *blockingAgent) StreamRun(ctx context.Context, _ agent.Message) (<-chan agent.StreamEvent, error) {
	ch := make(chan agent.StreamEvent, 1)
	select {
	case <-a.release:
		ch <- agent.StreamEvent{Type: agent.StreamEventComplete}
	case <-ctx.Done():
		close(ch)
		return ch, ctx.Err()
	}
	close(ch)
	return ch, nil
}

func (a *blockingAgent) Stop()                   {}
func (a *blockingAgent) Stats() agent.AgentStats { return agent.AgentStats{} }
func (a *blockingAgent) Name() string            { return "blocking" }

// TestPool_CanceledTaskErrMatchesSentinel 验证 ctx 取消的任务错误匹配
// pool.ErrContextCanceled（修复评估报告 §四.1-④：此前 pkg.ErrContextCanceled
// 无任何抛出点），同时 errors.Is 仍匹配 context.Canceled。
func TestPool_CanceledTaskErrMatchesSentinel(t *testing.T) {
	pool := NewPool(PoolConfig{MaxConcurrency: 1, Timeout: 30 * time.Second})
	blocking := &blockingAgent{release: make(chan struct{})}
	pool.SetAgentFactory(func(AgentFactoryConfig) agent.Agent { return blocking })
	defer pool.Close()

	// 第一个任务长期占据唯一槽位
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_, _ = pool.Dispatch(context.Background(), []TaskConfig{{ID: "long", Title: "t", Prompt: "x"}})
	}()
	time.Sleep(50 * time.Millisecond)

	// 第二个任务在已取消的 ctx 下等待槽位 → acquireSlot 应返回 ctx 取消
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results, err := pool.Dispatch(ctx, []TaskConfig{{ID: "c1", Title: "t", Prompt: "x"}})

	// 先释放第一个任务，避免阻塞 pool.Close
	close(blocking.release)
	<-firstDone

	if err == nil {
		t.Fatal("取消任务应返回错误")
	}
	if !errors.Is(err, ErrContextCanceled) {
		t.Fatalf("取消任务 err = %v, want 匹配 pool.ErrContextCanceled", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("取消任务 err = %v, want 仍匹配 context.Canceled", err)
	}
	if results[0].Status != PoolTaskCancelled {
		t.Fatalf("取消任务状态 = %v, want PoolTaskCancelled", results[0].Status)
	}
}

// TestPool_AcquireSlotTimeout 验证：槽位占满时，等待者应在 Timeout 到期后自行以
// ErrTimeout 失败，而不是无限阻塞直到其他事件唤醒（回归：dispatcher.go acquireSlot）。
func TestPool_AcquireSlotTimeout(t *testing.T) {
	pool := NewPool(PoolConfig{
		MaxConcurrency: 1,
		Timeout:        100 * time.Millisecond,
	})
	defer pool.Close()

	// 占满唯一槽位
	if err := pool.acquireSlot(context.Background()); err != nil {
		t.Fatalf("首个 acquireSlot 失败: %v", err)
	}

	start := time.Now()
	got := make(chan error, 1)
	go func() {
		got <- pool.acquireSlot(context.Background())
	}()

	select {
	case err := <-got:
		if !errors.Is(err, ErrTimeout) {
			t.Fatalf("acquireSlot = %v, want ErrTimeout", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("acquireSlot 未在 Timeout 内返回（超时绕过：等待者未被定时器唤醒）")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("acquireSlot 超时返回过慢: %v", elapsed)
	}
}

// TestPool_AcquireSlotContextCancel 验证 ctx 取消能立即唤醒等待者（既有行为回归守卫）。
func TestPool_AcquireSlotContextCancel(t *testing.T) {
	pool := NewPool(PoolConfig{MaxConcurrency: 1, Timeout: 30 * time.Second})
	defer pool.Close()

	if err := pool.acquireSlot(context.Background()); err != nil {
		t.Fatalf("首个 acquireSlot 失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan error, 1)
	go func() {
		got <- pool.acquireSlot(ctx)
	}()
	cancel()

	select {
	case err := <-got:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("acquireSlot = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("acquireSlot 未响应 ctx 取消")
	}
}

// TestPool_ConcurrentDispatchReadResult 并发 Dispatch + GetTask/ListTasks，
// 在 CI -race 下验证 pt.result 无锁写与 RLock 读之间无数据竞争。
func TestPool_ConcurrentDispatchReadResult(t *testing.T) {
	pool := NewPool(PoolConfig{
		MaxConcurrency: 4,
		Timeout:        5 * time.Second,
	})
	pool.SetModel(fakeProvider{})
	defer pool.Close()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := fmt.Sprintf("race-task-%d", n)
			_, _ = pool.Dispatch(context.Background(), []TaskConfig{
				{ID: id, Title: "t", Prompt: "hello"},
			})
			if _, ok := pool.GetTask(id); !ok {
				// 任务可能尚未写入 result，不视为失败
				_ = pool.ListTasks()
			}
			_ = pool.GetTasksBySession("")
		}(i)
	}
	wg.Wait()

	// 全部任务应已完成且可读
	all := pool.ListTasks()
	if len(all) != 8 {
		t.Fatalf("ListTasks = %d, want 8", len(all))
	}
}

// TestPool_ConcurrentSetModelDuringDispatch 并发 SetModel + Dispatch，
// 在 CI -race 下验证 createAgentForTask 无锁读 p.model/p.toolkit 与
// SetModel/SetToolkit 持锁写之间无数据竞争。
func TestPool_ConcurrentSetModelDuringDispatch(t *testing.T) {
	pool := NewPool(PoolConfig{
		MaxConcurrency: 2,
		Timeout:        5 * time.Second,
	})
	pool.SetModel(fakeProvider{})
	defer pool.Close()

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = pool.Dispatch(context.Background(), []TaskConfig{
				{ID: fmt.Sprintf("model-task-%d", n), Title: "t", Prompt: "hello"},
			})
		}(i)
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.SetModel(fakeProvider{})
		}()
	}
	wg.Wait()
}
