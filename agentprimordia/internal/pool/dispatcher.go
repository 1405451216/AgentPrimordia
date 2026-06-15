package pool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"agentprimordia/internal/agent"
	"agentprimordia/internal/llm"
	"agentprimordia/internal/tools"
)

const (
	defaultMaxConcurrency = 10
	defaultPoolTimeout    = 5 * time.Minute
	poolEventBufferSize   = 100
	defaultMaxTurns       = 50
)

var (
	ErrTimeout      = errors.New("operation timed out")
	ErrTaskNotFound = errors.New("task not found")
)

type Pool struct {
	mu           sync.RWMutex
	config       PoolConfig
	semaphore    chan struct{}
	tasks        map[string]*poolTask
	eventCh      chan PoolEvent
	agents       map[string]agent.Agent
	stats        PoolStats
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	startTime    time.Time
	model        llm.Provider
	toolkit      *tools.Registry
	agentFactory AgentFactory
	closeOnce    sync.Once
}

type poolTask struct {
	config     TaskConfig
	sessionID  string
	status     PoolTaskStatus
	result     *TaskResult
	startTime  time.Time
	retryCount int
	cancelCtx  context.Context
	cancelFunc context.CancelFunc
}

func NewPool(cfg PoolConfig) *Pool {
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = defaultMaxConcurrency
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultPoolTimeout
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Pool{
		config:    cfg,
		semaphore: make(chan struct{}, cfg.MaxConcurrency),
		tasks:     make(map[string]*poolTask),
		agents:    make(map[string]agent.Agent),
		eventCh:   make(chan PoolEvent, poolEventBufferSize),
		stats: PoolStats{
			MaxConcurrency: cfg.MaxConcurrency,
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

func (p *Pool) Dispatch(ctx context.Context, tasks []TaskConfig) ([]*TaskResult, error) {
	if len(tasks) == 0 {
		return []*TaskResult{}, nil
	}

	p.mu.Lock()
	p.startTime = time.Now()
	p.stats.TotalTasks += len(tasks)
	p.stats.QueuedTasks += len(tasks)
	p.mu.Unlock()

	// M8 修复：派发前检查是否需要清理终态任务，避免 task map 无界增长
	p.cleanupIfNeeded()
	results := make([]*TaskResult, len(tasks))
	errCh := make(chan error, len(tasks))

	for i, task := range tasks {
		if task.ID == "" {
			task.ID = generateTaskID(i)
		}

		taskCtx, taskCancel := context.WithCancel(ctx)

		pt := &poolTask{
			config:     task,
			sessionID:  task.SessionID,
			status:     PoolTaskQueued,
			startTime:  time.Now(),
			cancelCtx:  taskCtx,
			cancelFunc: taskCancel,
		}

		p.mu.Lock()
		if _, exists := p.tasks[task.ID]; exists {
			p.mu.Unlock()
			return nil, fmt.Errorf("duplicate task ID: %s", task.ID)
		}
		p.tasks[task.ID] = pt
		p.mu.Unlock()

		p.wg.Add(1)
		go func(idx int, t TaskConfig) {
			defer p.wg.Done()
			defer taskCancel()

			result, err := p.executeTask(taskCtx, t)
			if result == nil {
				result = &TaskResult{
					TaskID: t.ID,
					Task:   t,
					Error:  err,
					Status: PoolTaskFailed,
				}
			}
			results[idx] = result

			if err != nil {
				errCh <- err
			} else {
				errCh <- nil
			}
		}(i, task)
	}

	p.wg.Wait()
	close(errCh)

	var firstErr error
	for err := range errCh {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return results, firstErr
}

func (p *Pool) executeTask(ctx context.Context, task TaskConfig) (*TaskResult, error) {
	startTime := time.Now()

	pt := p.getTask(task.ID)
	if pt == nil {
		return &TaskResult{
			TaskID: task.ID,
			Task:   task,
			Error:  ErrTaskNotFound,
			Status: PoolTaskFailed,
		}, fmt.Errorf("%w: %s", ErrTaskNotFound, task.ID)
	}

	// 获取 semaphore（仅获取一次，重试期间持有不释放）
	select {
	case p.semaphore <- struct{}{}:
		defer func() { <-p.semaphore }()
	case <-ctx.Done():
		p.mu.Lock()
		pt.status = PoolTaskCancelled
		p.mu.Unlock()
		p.emitEvent(PoolEvent{Type: "task_cancelled", TaskID: task.ID})
		return &TaskResult{
			TaskID: task.ID,
			Task:   task,
			Error:  ctx.Err(),
			Status: PoolTaskCancelled,
		}, ctx.Err()
	case <-time.After(p.config.Timeout):
		p.mu.Lock()
		pt.status = PoolTaskFailed
		p.mu.Unlock()
		p.emitEvent(PoolEvent{Type: "task_failed", TaskID: task.ID, Data: "timeout"})
		return &TaskResult{
			TaskID: task.ID,
			Task:   task,
			Error:  ErrTimeout,
			Status: PoolTaskFailed,
		}, ErrTimeout
	}

	// 重试循环（semaphore 已持有，不再重复获取）
	for {
		p.mu.Lock()
		pt.status = PoolTaskRunning
		p.mu.Unlock()
		p.emitEvent(PoolEvent{Type: "task_started", TaskID: task.ID, Data: task.Title})

		taskCtx, cancel := context.WithTimeout(ctx, p.config.Timeout)

		agt := p.createAgentForTask(task)
		result, err := agt.Run(taskCtx, agent.UserMessage(task.Prompt))
		taskCtxErr := taskCtx.Err()
		cancel()

		duration := time.Since(startTime)

		if err != nil {
			if taskCtxErr == context.DeadlineExceeded {
				p.mu.Lock()
				pt.status = PoolTaskFailed
				p.mu.Unlock()
				p.emitEvent(PoolEvent{Type: "task_failed", TaskID: task.ID, Data: "timeout"})
			} else if taskCtxErr == context.Canceled || ctx.Err() != nil {
				p.mu.Lock()
				pt.status = PoolTaskCancelled
				p.mu.Unlock()
				p.emitEvent(PoolEvent{Type: "task_cancelled", TaskID: task.ID})
			} else {
				p.mu.Lock()
				pt.status = PoolTaskFailed
				p.mu.Unlock()

				if p.isRetryable(err) && pt.retryCount < p.config.RetryPolicy.MaxRetries {
					pt.retryCount++
					p.mu.Lock()
					p.mu.Unlock()
					time.Sleep(p.config.RetryPolicy.Backoff)
					continue
				}

				p.emitEvent(PoolEvent{Type: "task_failed", TaskID: task.ID, Data: err.Error()})
			}
		} else {
			p.mu.Lock()
			pt.status = PoolTaskCompleted
			p.mu.Unlock()
			p.emitEvent(PoolEvent{Type: "task_completed", TaskID: task.ID})
		}

		pt.result = &TaskResult{
			TaskID:   task.ID,
			Task:     task,
			Response: result,
			Error:    err,
			Duration: duration,
			Status:   pt.status,
		}

		p.mu.Lock()
		delete(p.agents, task.ID)
		p.mu.Unlock()

		p.updateStats()

		return pt.result, err
	}
}

func (p *Pool) createAgentForTask(task TaskConfig) agent.Agent {
	p.mu.RLock()
	factory := p.agentFactory
	p.mu.RUnlock()

	if factory != nil {
		factoryCfg := AgentFactoryConfig{
			Name:         task.Title,
			SystemPrompt: p.config.DefaultAgent.SystemPrompt,
			MaxTurns:     task.MaxTurns,
			Temperature:  p.config.DefaultAgent.Temperature,
			FilesScope:   task.FilesScope,
			SessionID:    task.SessionID,
			Metadata:     task.Metadata,
		}

		if factoryCfg.MaxTurns == 0 {
			factoryCfg.MaxTurns = p.config.DefaultAgent.MaxTurns
		}
		if factoryCfg.MaxTurns == 0 {
			factoryCfg.MaxTurns = defaultMaxTurns
		}

		agt := factory(factoryCfg)
		p.mu.Lock()
		p.agents[task.ID] = agt
		p.mu.Unlock()
		return agt
	}

	cfg := agent.ReActConfig{
		Name:     task.Title,
		MaxTurns: task.MaxTurns,
	}

	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = p.config.DefaultAgent.MaxTurns
	}
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = defaultMaxTurns
	}

	if p.config.DefaultAgent.SystemPrompt != "" {
		cfg.SystemPrompt = p.config.DefaultAgent.SystemPrompt
	}

	if p.model != nil {
		cfg.Model = p.model
	}

	if p.toolkit != nil {
		cfg.Toolkit = p.toolkit
	}

	agt := agent.NewReActAgent(cfg)
	p.mu.Lock()
	p.agents[task.ID] = agt
	p.mu.Unlock()
	return agt
}

// SetAgentFactory 设置 Agent 工厂函数，替代默认的 ReActAgent 创建逻辑
func (p *Pool) SetAgentFactory(factory AgentFactory) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.agentFactory = factory
}

func (p *Pool) getTask(id string) *poolTask {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.tasks[id]
}

func (p *Pool) updateStats() {
	p.mu.Lock()
	defer p.mu.Unlock()

	running := 0
	queued := 0
	completed := 0
	failed := 0

	for _, t := range p.tasks {
		switch t.status {
		case PoolTaskQueued:
			queued++
		case PoolTaskRunning:
			running++
		case PoolTaskCompleted:
			completed++
		case PoolTaskFailed:
			failed++
		}
	}

	p.stats.RunningTasks = running
	p.stats.QueuedTasks = queued
	p.stats.CompletedTasks = completed
	p.stats.FailedTasks = failed
	p.stats.ActiveConcurrency = running
}

func (p *Pool) emitEvent(event PoolEvent) {
	event.Timestamp = time.Now()
	select {
	case p.eventCh <- event:
	default:
	}
}

func (p *Pool) isRetryable(err error) bool {
	if p.config.RetryPolicy.MaxRetries == 0 {
		return false
	}

	errMsg := err.Error()
	for _, pattern := range p.config.RetryPolicy.RetryableErrors {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

func (p *Pool) Cancel(taskID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	task, exists := p.tasks[taskID]
	if !exists {
		return ErrTaskNotFound
	}

	if task.status == PoolTaskRunning {
		task.cancelFunc()
		task.status = PoolTaskCancelled
	}

	p.emitEvent(PoolEvent{Type: "task_cancelled", TaskID: taskID})
	return nil
}

func (p *Pool) CancelAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, task := range p.tasks {
		if task.status == PoolTaskQueued || task.status == PoolTaskRunning {
			task.cancelFunc()
			task.status = PoolTaskCancelled
			p.emitEvent(PoolEvent{Type: "task_cancelled", TaskID: id})
		}
	}
}

func (p *Pool) GetTasksBySession(sessionID string) []TaskResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var results []TaskResult
	for _, pt := range p.tasks {
		if pt.sessionID == sessionID {
			if pt.result != nil {
				results = append(results, *pt.result)
			} else {
				results = append(results, TaskResult{
					TaskID: pt.config.ID,
					Task:   pt.config,
					Status: pt.status,
				})
			}
		}
	}
	return results
}

func (p *Pool) CancelBySession(sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, pt := range p.tasks {
		if pt.sessionID == sessionID && (pt.status == PoolTaskQueued || pt.status == PoolTaskRunning) {
			pt.cancelFunc()
			pt.status = PoolTaskCancelled
			p.emitEvent(PoolEvent{Type: "task_cancelled", TaskID: id})
		}
	}
	return nil
}

func (p *Pool) GetTask(id string) (TaskResult, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	pt, exists := p.tasks[id]
	if !exists || pt.result == nil {
		return TaskResult{}, false
	}
	return *pt.result, true
}

// ListTasks 返回所有任务的结果列表
func (p *Pool) ListTasks() []TaskResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	results := make([]TaskResult, 0, len(p.tasks))
	for _, pt := range p.tasks {
		if pt.result != nil {
			results = append(results, *pt.result)
		} else {
			results = append(results, TaskResult{
				TaskID: pt.config.ID,
				Task:   pt.config,
				Status: pt.status,
			})
		}
	}
	return results
}

// ListAgents 返回所有 Agent 的统计信息，按任务 ID 索引
func (p *Pool) ListAgents() map[string]agent.AgentStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[string]agent.AgentStats, len(p.agents))
	for id, agt := range p.agents {
		result[id] = agt.Stats()
	}
	return result
}

func (p *Pool) Stats() PoolStats {
	p.updateStats()
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats
}

func (p *Pool) EventChannel() <-chan PoolEvent {
	return p.eventCh
}

func (p *Pool) SetModel(model llm.Provider) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.model = model
}

func (p *Pool) SetToolkit(registry *tools.Registry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.toolkit = registry
}

// Cleanup 清理已完成、失败或取消的任务记录，释放 tasks map 中的内存
func (p *Pool) Cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, t := range p.tasks {
		if t.status == PoolTaskCompleted || t.status == PoolTaskFailed || t.status == PoolTaskCancelled {
			delete(p.tasks, id)
		}
	}
}

// cleanupIfNeeded 当 task map 超过 MaxRetainedTasks 阈值时自动清理终态任务（M8 修复）。
// 仅在配置了 MaxRetainedTasks（>0）且当前任务数超过阈值时触发，保留活跃任务。
// 零值（0）时不做任何事，保持向后兼容。
func (p *Pool) cleanupIfNeeded() {
	if p.config.MaxRetainedTasks <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.tasks) < p.config.MaxRetainedTasks {
		return
	}
	for id, t := range p.tasks {
		if t.status == PoolTaskCompleted || t.status == PoolTaskFailed || t.status == PoolTaskCancelled {
			delete(p.tasks, id)
		}
	}
}

func (p *Pool) Close() {
	p.closeOnce.Do(func() {
		p.cancel()
		p.wg.Wait()
		close(p.eventCh)
	})
}

func generateTaskID(index int) string {
	return fmt.Sprintf("task_%d_%d", time.Now().UnixNano(), index)
}
