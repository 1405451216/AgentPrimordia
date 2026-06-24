package pool

import (
	"context"
	"testing"
	"time"
)

func TestGracefulShutdown_NoRunningTasks(t *testing.T) {
	p := NewPool(PoolConfig{MaxConcurrency: 2})
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := p.GracefulShutdown(ctx)
	if err != nil {
		t.Fatalf("无运行任务时优雅关闭应成功: %v", err)
	}
}

func TestGracefulShutdown_WaitsForRunningTasks(t *testing.T) {
	p := NewPool(PoolConfig{MaxConcurrency: 2})

	// 模拟有运行中的任务
	p.runningCount.Add(2)

	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		done <- p.GracefulShutdown(ctx)
	}()

	// 验证关闭仍在等待
	time.Sleep(100 * time.Millisecond)
	select {
	case <-done:
		t.Error("GracefulShutdown 不应在有运行任务时立即返回")
	default:
	}

	// 模拟任务完成
	p.runningCount.Add(-2)

	// 等待关闭完成
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("优雅关闭失败: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("GracefulShutdown 超时")
	}
}

func TestGracefulShutdown_Timeout(t *testing.T) {
	p := NewPool(PoolConfig{MaxConcurrency: 1})
	defer p.Close()

	// 模拟有运行中的任务
	p.runningCount.Add(1)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := p.GracefulShutdown(ctx)
	if err == nil {
		t.Error("超时应返回错误")
	}
}

func TestGracefulShutdown_RejectsNewDispatch(t *testing.T) {
	p := NewPool(PoolConfig{MaxConcurrency: 2})
	defer p.Close()

	// 标记为关闭状态
	p.shutdown.Store(true)

	// 尝试派发应被拒绝
	_, err := p.Dispatch(context.Background(), []TaskConfig{
		{ID: "test-1", Title: "test", Prompt: "test"},
	})
	if err == nil {
		t.Error("关闭状态下 Dispatch 应返回错误")
	}
}
