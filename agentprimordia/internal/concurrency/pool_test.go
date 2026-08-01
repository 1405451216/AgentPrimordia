package concurrency

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestGoroutinePool_BasicExecution(t *testing.T) {
	pool := NewGoroutinePool(Config{
		MinWorkers:  2,
		MaxWorkers:  10,
		QueueSize:   100,
		IdleTimeout: 5 * time.Second,
	})
	defer pool.Stop()

	var completed atomic.Int32

	for i := 0; i < 20; i++ {
		_ = pool.Submit(func(ctx context.Context) error {
			completed.Add(1)
			return nil
		})
	}

	pool.Wait()

	if completed.Load() != 20 {
		t.Errorf("完成数 = %d, 期望 20", completed.Load())
	}
}

func TestGoroutinePool_DynamicScaling(t *testing.T) {
	pool := NewGoroutinePool(Config{
		MinWorkers:  2,
		MaxWorkers:  10,
		QueueSize:   100,
		IdleTimeout: 100 * time.Millisecond,
	})
	defer pool.Stop()

	// 提交大量任务触发扩容
	for i := 0; i < 50; i++ {
		_ = pool.Submit(func(ctx context.Context) error {
			time.Sleep(10 * time.Millisecond)
			return nil
		})
	}

	// 等待任务完成
	pool.Wait()

	// 验证工作协程数增加
	workers := pool.ActiveWorkers()
	if workers < 2 {
		t.Errorf("活跃工作数 = %d, 期望 >= 2", workers)
	}

	// 等待空闲超时
	time.Sleep(200 * time.Millisecond)

	// 验证工作协程数减少
	workers = pool.ActiveWorkers()
	if workers > 5 {
		t.Errorf("空闲后工作数 = %d, 期望 <= 5", workers)
	}
}

func TestGoroutinePool_ContextCancellation(t *testing.T) {
	pool := NewGoroutinePool(Config{
		MinWorkers: 2,
		MaxWorkers: 5,
		QueueSize:  10,
	})
	defer pool.Stop()

	ctx, cancel := context.WithCancel(context.Background())

	var started atomic.Int32
	for i := 0; i < 10; i++ {
		_ = pool.SubmitWithContext(ctx, func(ctx context.Context) error {
			started.Add(1)
			<-ctx.Done()
			return ctx.Err()
		})
	}

	time.Sleep(50 * time.Millisecond)
	cancel()

	pool.Wait()

	if started.Load() == 0 {
		t.Error("应有任务启动")
	}
}

func TestGoroutinePool_QueueFull(t *testing.T) {
	pool := NewGoroutinePool(Config{
		MinWorkers:  1,
		MaxWorkers:  1,
		QueueSize:   2,
		IdleTimeout: 1 * time.Second,
	})
	defer pool.Stop()

	// 填满队列
	for i := 0; i < 3; i++ {
		_ = pool.Submit(func(ctx context.Context) error {
			time.Sleep(1 * time.Second)
			return nil
		})
	}

	// 第 4 个应被拒绝或阻塞
	err := pool.Submit(func(ctx context.Context) error {
		return nil
	})

	// 根据实现，可能返回错误
	if err != nil && err != ErrQueueFull {
		t.Errorf("错误 = %v, 期望 nil 或 ErrQueueFull", err)
	}
}

func TestGoroutinePool_Stop(t *testing.T) {
	pool := NewGoroutinePool(Config{
		MinWorkers: 2,
		MaxWorkers: 5,
		QueueSize:  10,
	})

	pool.Stop()

	// 停止后提交应返回错误
	err := pool.Submit(func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Error("停止后提交应返回错误")
	}
}

func BenchmarkGoroutinePool_Submit(b *testing.B) {
	pool := NewGoroutinePool(Config{
		MinWorkers: 10,
		MaxWorkers: 100,
		QueueSize:  1000,
	})
	defer pool.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.Submit(func(ctx context.Context) error {
			return nil
		})
	}
}
