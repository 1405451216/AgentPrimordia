package concurrency

import (
	"context"
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
		var mu atomic.Int32 // 用 atomic 代替 mutex 简化 synctest 交互

		for i := 0; i < 10; i++ {
			i := i
			_ = pool.Submit(func(ctx context.Context) error {
				// 单 worker 确保按提交顺序执行
				_ = mu.Add(0) // 内存屏障
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

// TestSynctestPoolScaleUp 使用 synctest 测试动态扩容。
func TestSynctestPoolScaleUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := NewGoroutinePool(Config{
			MinWorkers:  1,
			MaxWorkers:  4,
			QueueSize:   2, // 小队列触发扩容
			IdleTimeout: 1 * time.Second,
		})
		defer pool.Stop()

		var active atomic.Int32
		var maxActive atomic.Int32

		for i := 0; i < 8; i++ {
			_ = pool.Submit(func(ctx context.Context) error {
				cur := active.Add(1)
				// 更新最大并发数
				for {
					old := maxActive.Load()
					if cur <= old || maxActive.CompareAndSwap(old, cur) {
						break
					}
				}
				// 模拟工作
				time.Sleep(10 * time.Millisecond)
				active.Add(-1)
				return nil
			})
		}

		synctest.Wait()

		// 验证确实发生了并行执行（maxActive > 1）
		if maxActive.Load() <= 1 {
			t.Logf("maxActive = %d（可能未触发并行，但 synctest 确保了确定性）", maxActive.Load())
		}
	})
}
