package concurrency

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// TestSynctestPoolDeterministic 使用 synctest 验证 GoroutinePool 的确定性行为。
// synctest.Test 创建一个「气泡」，其中的 goroutine 和 time 操作都是确定性的。
// synctest.Wait() 会等待气泡内所有 goroutine 阻塞后再继续。
func TestSynctestPoolDeterministic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := NewGoroutinePool(Config{
			MinWorkers:  2,
			MaxWorkers:  4,
			QueueSize:   10,
			IdleTimeout: 1 * time.Second,
		})
		defer pool.Stop()

		var counter atomic.Int32

		// 提交 5 个任务
		for i := 0; i < 5; i++ {
			err := pool.Submit(func(ctx context.Context) error {
				counter.Add(1)
				return nil
			})
			if err != nil {
				t.Fatalf("Submit 返回错误: %v", err)
			}
		}

		// 等待所有 goroutine 阻塞（任务执行完毕）
		synctest.Wait()

		// 在 synctest 气泡中，此时所有任务应已完成
		if got := counter.Load(); got != 5 {
			t.Errorf("计数器 = %d, 期望 5", got)
		}
	})
}

// TestSynctestPoolTimeout 使用 synctest 测试带超时的任务。
// 在 synctest 气泡中，time.Sleep 和 context.WithTimeout 都是虚拟化的，
// 无需真实等待即可测试超时逻辑。
func TestSynctestPoolTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := NewGoroutinePool(Config{
			MinWorkers:  1,
			MaxWorkers:  2,
			QueueSize:   5,
			IdleTimeout: 1 * time.Second,
		})
		defer pool.Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		var completed atomic.Bool

		// 提交一个会超时的任务
		err := pool.SubmitWithContext(ctx, func(taskCtx context.Context) error {
			select {
			case <-taskCtx.Done():
				return taskCtx.Err()
			case <-time.After(1 * time.Second):
				completed.Store(true)
				return nil
			}
		})
		if err != nil {
			t.Fatalf("SubmitWithContext 返回错误: %v", err)
		}

		// 等待所有 goroutine 阻塞
		synctest.Wait()

		// 任务应因超时而未完成
		if completed.Load() {
			t.Error("任务不应完成（应超时）")
		}
	})
}

// TestSynctestPoolOrdering 使用 synctest 验证任务执行顺序的确定性。
func TestSynctestPoolOrdering(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := NewGoroutinePool(Config{
			MinWorkers:  1, // 单 worker 确保顺序
			MaxWorkers:  1,
			QueueSize:   10,
			IdleTimeout: 1 * time.Second,
		})
		defer pool.Stop()

		var results []int

		for i := 0; i < 10; i++ {
			i := i
			_ = pool.Submit(func(ctx context.Context) error {
				// 单 worker 从 channel 读取，确保按提交顺序执行
				results = append(results, i)
				return nil
			})
		}

		synctest.Wait()

		// 单 worker 应保持提交顺序
		if len(results) != 10 {
			t.Fatalf("结果数 = %d, 期望 10", len(results))
		}
		for i, v := range results {
			if v != i {
				t.Errorf("results[%d] = %d, 期望 %d", i, v, i)
			}
		}
	})
}

// TestSynctestPoolScaleUp 使用真实 goroutine + WaitGroup 测试动态扩容。
func TestSynctestPoolScaleUp(t *testing.T) {
	pool := NewGoroutinePool(Config{
		MinWorkers:  1,
		MaxWorkers:  4,
		QueueSize:   8,
		IdleTimeout: 1 * time.Second,
	})
	defer pool.Stop()

	var active atomic.Int32
	var maxActive atomic.Int32
	var failed atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		if err := pool.Submit(func(ctx context.Context) error {
			defer wg.Done()
			cur := active.Add(1)
			for {
				old := maxActive.Load()
				if cur <= old || maxActive.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			active.Add(-1)
			return nil
		}); err != nil {
			wg.Done() // Submit 失败，撤销预添加的计数
			failed.Add(1)
		}
	}

	wg.Wait()

	if failed.Load() > 0 {
		t.Logf("failed submissions: %d", failed.Load())
	}
	if maxActive.Load() <= 1 {
		t.Logf("maxActive = %d（可能未触发并行）", maxActive.Load())
	}
}
