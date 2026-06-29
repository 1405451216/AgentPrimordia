// pool.go — 动态调优协程池
package concurrency

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrQueueFull 队列已满错误
var ErrQueueFull = errors.New("task queue is full")

// ErrPoolStopped 协程池已停止
var ErrPoolStopped = errors.New("pool is stopped")

// Config 协程池配置
type Config struct {
	MinWorkers  int           // 最小工作协程数
	MaxWorkers  int           // 最大工作协程数
	QueueSize   int           // 任务队列大小
	IdleTimeout time.Duration // 空闲协程超时时间
}

// Task 任务函数类型
type Task func(ctx context.Context) error

// GoroutinePool 动态调优协程池
type GoroutinePool struct {
	cfg       Config
	taskQueue chan taskItem
	workers   atomic.Int32
	active    atomic.Int32
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	stopOnce  sync.Once
	stopped   atomic.Bool
	// waitMu/cond 替代忙等待：当任务完成时通过 Signal 唤醒 Wait()
	waitMu   sync.Mutex
	waitCond *sync.Cond
}

type taskItem struct {
	task Task
	ctx  context.Context
}

// NewGoroutinePool 创建协程池
func NewGoroutinePool(cfg Config) *GoroutinePool {
	if cfg.MinWorkers <= 0 {
		cfg.MinWorkers = 2
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 100
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 100
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())
	pool := &GoroutinePool{
		cfg:       cfg,
		taskQueue: make(chan taskItem, cfg.QueueSize),
		ctx:       ctx,
		cancel:    cancel,
	}
	pool.waitCond = sync.NewCond(&pool.waitMu)

	// 启动最小工作协程数
	for i := 0; i < cfg.MinWorkers; i++ {
		pool.startWorker()
	}

	return pool
}

// Submit 提交任务
func (p *GoroutinePool) Submit(task Task) error {
	return p.SubmitWithContext(context.Background(), task)
}

// SubmitWithContext 提交带 context 的任务
func (p *GoroutinePool) SubmitWithContext(ctx context.Context, task Task) error {
	if p.stopped.Load() {
		return ErrPoolStopped
	}

	item := taskItem{task: task, ctx: ctx}

	select {
	case p.taskQueue <- item:
		// 如果队列繁忙，动态扩容
		if len(p.taskQueue) > cap(p.taskQueue)/2 {
			p.tryScaleUp()
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// 队列已满，尝试扩容
		if p.tryScaleUp() {
			select {
			case p.taskQueue <- item:
				return nil
			default:
				return ErrQueueFull
			}
		}
		return ErrQueueFull
	}
}

// Wait 等待所有已提交任务完成（使用 sync.Cond 替代忙等待）
func (p *GoroutinePool) Wait() {
	p.waitMu.Lock()
	defer p.waitMu.Unlock()
	for len(p.taskQueue) > 0 || p.active.Load() > 0 {
		p.waitCond.Wait()
	}
}

// Stop 停止协程池
func (p *GoroutinePool) Stop() {
	p.stopOnce.Do(func() {
		p.stopped.Store(true)
		p.cancel()
		p.wg.Wait()
		p.waitCond.Broadcast() // 唤醒所有 Wait() 等待者
	})
}

// ActiveWorkers 返回活跃工作协程数
func (p *GoroutinePool) ActiveWorkers() int {
	return int(p.workers.Load())
}

func (p *GoroutinePool) startWorker() {
	p.workers.Add(1)
	p.wg.Add(1)

	go func() {
		defer p.wg.Done()
		defer p.workers.Add(-1)

		idleTimer := time.NewTimer(p.cfg.IdleTimeout)
		defer idleTimer.Stop()

		for {
			select {
			case item, ok := <-p.taskQueue:
				if !ok {
					return
				}
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				idleTimer.Reset(p.cfg.IdleTimeout)

				p.active.Add(1)
				_ = item.task(item.ctx)
				p.active.Add(-1)
				// 唤醒 Wait() 等待者
				p.waitCond.Broadcast()

			case <-idleTimer.C:
				// 空闲超时，如果当前工作数 > 最小值，退出
				if p.workers.Load() > int32(p.cfg.MinWorkers) {
					return
				}
				idleTimer.Reset(p.cfg.IdleTimeout)

			case <-p.ctx.Done():
				return
			}
		}
	}()
}

// tryScaleUp 尝试扩容一个工作协程。
// 使用 CAS 原子操作避免多个调用者同时扩容导致 worker 数超过 MaxWorkers。
// 注意：极端并发下仍可能短暂超过 MaxWorkers（两个 goroutine 同时看到 current < max 并 CAS 成功），
// 但 worker 协程的空闲超时退出会自然回落，因此不影响正确性。
func (p *GoroutinePool) tryScaleUp() bool {
	current := p.workers.Load()
	if current >= int32(p.cfg.MaxWorkers) {
		return false
	}

	// 原子增加工作协程数
	if p.workers.CompareAndSwap(current, current+1) {
		p.startWorker()
		return true
	}
	return false
}
