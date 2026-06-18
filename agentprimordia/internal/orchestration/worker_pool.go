package orchestration

import (
	"context"
	"sync"
)

// WorkerPool 使用固定 worker goroutine 执行 step。
// 优化（perf-v2）：任务通道使用缓冲区（容量=worker数），
// 避免 Submit 在所有 worker 繁忙时阻塞调度器事件循环。
type WorkerPool struct {
	workers  int
	tasks    chan workerTask
	executor StepExecutor
	wg       sync.WaitGroup
}

type workerTask struct {
	ctx      context.Context
	node     *StepNode
	input    map[string]any
	resultCh chan<- *StepResult
}

// NewWorkerPool 创建 worker 池。
// 优化（perf-v2）：任务通道缓冲大小 = workers，允许调度器批量派发而不阻塞。
func NewWorkerPool(workers int, executor StepExecutor) *WorkerPool {
	if workers <= 0 {
		workers = 1
	}
	p := &WorkerPool{
		workers:  workers,
		tasks:    make(chan workerTask, workers),
		executor: executor,
	}
	p.start()
	return p
}

func (p *WorkerPool) start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for task := range p.tasks {
				result := p.executor.Execute(task.ctx, task.node.Step, task.input)
				task.resultCh <- result
			}
		}()
	}
}

// Submit 提交任务到 worker 池。
func (p *WorkerPool) Submit(ctx context.Context, node *StepNode, input map[string]any, resultCh chan<- *StepResult) {
	p.tasks <- workerTask{ctx: ctx, node: node, input: input, resultCh: resultCh}
}

// Stop 关闭任务通道并等待所有 worker 退出。
func (p *WorkerPool) Stop() {
	close(p.tasks)
	p.wg.Wait()
}

// StepExecutorFunc 允许用函数实现 StepExecutor。
type StepExecutorFunc func(ctx context.Context, step *AgentStep, input map[string]any) *StepResult

func (f StepExecutorFunc) Execute(ctx context.Context, step *AgentStep, input map[string]any) *StepResult {
	return f(ctx, step, input)
}
