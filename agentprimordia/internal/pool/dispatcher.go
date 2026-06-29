package pool

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"agentprimordia/internal/agent"
	"agentprimordia/internal/llm"
	"agentprimordia/internal/tools"
)

const (
	// perf-v6 Task H：默认 maxConcurrency 提升到 2 * NumCPU（最少 16）
	// 老值 10 在多核机器上明显不足
	defaultMaxConcurrency   = 16
	defaultPoolTimeout      = 5 * time.Minute
	poolEventBufferSize     = 100
	defaultMaxTurns         = 50
	defaultMaxRetainedTasks = 1000
)

var (
	ErrTimeout      = errors.New("operation timed out")
	ErrTaskNotFound = errors.New("task not found")
)

// TaskError 单个任务错误信息
type TaskError struct {
	TaskID string
	Error  error
}

// AggregatedError 聚合多个任务的错误信息
// 支持 errors.Is/errors.As 解包，errors.Join 风格的语义
type AggregatedError struct {
	TaskErrors []TaskError
}

// Error 返回聚合的错误信息
func (e *AggregatedError) Error() string {
	if len(e.TaskErrors) == 0 {
		return "no errors"
	}
	if len(e.TaskErrors) == 1 {
		return fmt.Sprintf("task %s failed: %v", e.TaskErrors[0].TaskID, e.TaskErrors[0].Error)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d tasks failed: ", len(e.TaskErrors))
	for i, te := range e.TaskErrors {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s: %v", te.TaskID, te.Error)
	}
	return b.String()
}

// Unwrap 返回所有子错误，支持 errors.Is/errors.As 遍历
func (e *AggregatedError) Unwrap() []error {
	errs := make([]error, len(e.TaskErrors))
	for i, te := range e.TaskErrors {
		errs[i] = te.Error
	}
	return errs
}

// Is 检查 AggregatedError 中是否包含目标错误
func (e *AggregatedError) Is(target error) bool {
	for _, te := range e.TaskErrors {
		if errors.Is(te.Error, target) {
			return true
		}
	}
	return false
}

// As 尝试将 AggregatedError 中的任意错误转换为目标类型
func (e *AggregatedError) As(target interface{}) bool {
	for _, te := range e.TaskErrors {
		if errors.As(te.Error, target) {
			return true
		}
	}
	return false
}

type Pool struct {
	mu           sync.RWMutex
	config       PoolConfig
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
	shutdown     atomic.Bool // 优雅关闭标志，置位后拒绝新任务

	// 动态并发度信号量（替代固定容量的 channel）
	// 使用 sync.Cond 实现可动态调整上限的计数信号量
	semaMu    sync.Mutex
	semaCond  *sync.Cond
	semaCount int64 // 当前已获取令牌的 goroutine 数

	// 优化（perf-v2）：会话索引，将 GetTasksBySession/CancelBySession 从 O(n) 优化为 O(k)
	sessionIndex map[string]map[string]struct{} // sessionID -> set of taskIDs

	// 优化（Task 8）：使用 atomic 增量维护 stats，Stats() 无需遍历 tasks map
	runningCount   atomic.Int64
	queuedCount    atomic.Int64
	completedCount atomic.Int64
	failedCount    atomic.Int64

	// Task 9：动态 Agent 池（自动扩缩容）
	autoScaler         *AutoScaler
	autoScalerRunning  atomic.Bool
	dynamicConcurrency atomic.Int64 // 动态并发度限制，由 AutoScaler 更新
}

type poolTask struct {
	config     TaskConfig
	sessionID  string
	status     atomic.Value // PoolTaskStatus string (perf-v6 Task A：atomic.Value 无锁读)
	result     *TaskResult
	startTime  time.Time
	retryCount int
	cancelCtx  context.Context
	cancelFunc context.CancelFunc
}

// loadStatus 从 atomic.Value 安全读取 PoolTaskStatus（perf-v6 Task A）
func (pt *poolTask) loadStatus() PoolTaskStatus {
	v := pt.status.Load()
	if v == nil {
		return PoolTaskQueued
	}
	return v.(PoolTaskStatus)
}

// storeStatus 安全写 PoolTaskStatus 到 atomic.Value（perf-v6 Task A）
func (pt *poolTask) storeStatus(s PoolTaskStatus) {
	pt.status.Store(s)
}

func NewPool(cfg PoolConfig) *Pool {
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = defaultMaxConcurrency
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultPoolTimeout
	}
	if cfg.MaxRetainedTasks <= 0 {
		cfg.MaxRetainedTasks = defaultMaxRetainedTasks
	}

	ctx, cancel := context.WithCancel(context.Background())

	p := &Pool{
		config:       cfg,
		tasks:        make(map[string]*poolTask),
		agents:       make(map[string]agent.Agent),
		eventCh:      make(chan PoolEvent, poolEventBufferSize),
		sessionIndex: make(map[string]map[string]struct{}),
		stats: PoolStats{
			MaxConcurrency: cfg.MaxConcurrency,
		},
		ctx:    ctx,
		cancel: cancel,
	}
	p.semaCond = sync.NewCond(&p.semaMu)

	// Task 9：初始化 AutoScaler 和动态并发度
	p.dynamicConcurrency.Store(int64(cfg.MaxConcurrency))
	if cfg.AutoScaler != nil {
		p.autoScaler = NewAutoScaler(*cfg.AutoScaler)
	}

	return p
}

type dispatchItem struct {
	idx        int
	task       TaskConfig
	taskCtx    context.Context
	taskCancel context.CancelFunc
}

func (p *Pool) Dispatch(ctx context.Context, tasks []TaskConfig) ([]*TaskResult, error) {
	if len(tasks) == 0 {
		return []*TaskResult{}, nil
	}

	// 优雅关闭期间拒绝新任务
	if p.shutdown.Load() {
		return nil, errors.New("pool is shutting down")
	}

	p.mu.Lock()
	p.startTime = time.Now()
	p.stats.TotalTasks += len(tasks)
	p.mu.Unlock()
	// 优化（Task 8）：使用原子计数
	p.queuedCount.Add(int64(len(tasks)))

	// M8 修复：派发前检查是否需要清理终态任务，避免 task map 无界增长
	p.cleanupIfNeeded()
	results := make([]*TaskResult, len(tasks))
	errCh := make(chan error, len(tasks))

	taskCh := make(chan dispatchItem, len(tasks))

	for i, task := range tasks {
		if task.ID == "" {
			task.ID = generateTaskID(i)
		}

		taskCtx, taskCancel := context.WithCancel(ctx)

		pt := &poolTask{
			config:     task,
			sessionID:  task.SessionID,
			startTime:  time.Now(),
			cancelCtx:  taskCtx,
			cancelFunc: taskCancel,
		}
		pt.storeStatus(PoolTaskQueued) // perf-v6 Task A

		p.mu.Lock()
		if _, exists := p.tasks[task.ID]; exists {
			p.mu.Unlock()
			close(taskCh)
			return nil, fmt.Errorf("duplicate task ID: %s", task.ID)
		}
		p.tasks[task.ID] = pt
		// 优化（perf-v2）：维护会话索引
		if task.SessionID != "" {
			if p.sessionIndex[task.SessionID] == nil {
				p.sessionIndex[task.SessionID] = make(map[string]struct{})
			}
			p.sessionIndex[task.SessionID][task.ID] = struct{}{}
		}
		p.mu.Unlock()

		taskCh <- dispatchItem{idx: i, task: task, taskCtx: taskCtx, taskCancel: taskCancel}
	}
	close(taskCh)

	// 使用固定 worker 池，避免任务数过大时创建大量 goroutine
	workerCount := p.config.MaxConcurrency
	if workerCount > len(tasks) {
		workerCount = len(tasks)
	}
	p.wg.Add(workerCount)
	for w := 0; w < workerCount; w++ {
		go func() {
			defer p.wg.Done()
			for item := range taskCh {
				func() {
					defer item.taskCancel()

					result, err := p.executeTask(item.taskCtx, item.task)
					if result == nil {
						result = &TaskResult{
							TaskID: item.task.ID,
							Task:   item.task,
							Error:  err,
							Status: PoolTaskFailed,
						}
					}
					results[item.idx] = result

					if err != nil {
						errCh <- err
					} else {
						errCh <- nil
					}
				}()
			}
		}()
	}

	p.wg.Wait()
	close(errCh)

	// 收集所有错误，构建聚合错误信息
	var taskErrors []TaskError
	for _, r := range results {
		if r != nil && r.Error != nil {
			taskErrors = append(taskErrors, TaskError{TaskID: r.TaskID, Error: r.Error})
		}
	}

	if len(taskErrors) == 0 {
		return results, nil
	}
	if len(taskErrors) == 1 {
		return results, taskErrors[0].Error
	}
	return results, &AggregatedError{TaskErrors: taskErrors}
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

	// 获取动态信号量（仅获取一次，重试期间持有不释放）
	// 使用 sync.Cond 实现可动态调整上限的计数信号量
	if err := p.acquireSlot(ctx); err != nil {
		if err == ctx.Err() {
			p.mu.Lock()
			pt.storeStatus(PoolTaskCancelled) // perf-v6 Task A
			p.mu.Unlock()
			// 优化（Task 8）：原子计数 - queued 转 cancelled
			p.queuedCount.Add(-1)
			p.emitEvent(PoolEvent{Type: "task_cancelled", TaskID: task.ID})
			return &TaskResult{
				TaskID: task.ID,
				Task:   task,
				Error:  ctx.Err(),
				Status: PoolTaskCancelled,
			}, ctx.Err()
		}
		// 超时（acquireSlot 内部已经处理了超时）
		p.mu.Lock()
		pt.storeStatus(PoolTaskFailed) // perf-v6 Task A
		p.mu.Unlock()
		// 优化（Task 8）：原子计数 - queued 转 failed
		p.queuedCount.Add(-1)
		p.failedCount.Add(1)
		p.emitEvent(PoolEvent{Type: "task_failed", TaskID: task.ID, Data: "timeout"})
		return &TaskResult{
			TaskID: task.ID,
			Task:   task,
			Error:  ErrTimeout,
			Status: PoolTaskFailed,
		}, ErrTimeout
	}
	defer p.releaseSlot()

	// 重试循环（semaphore 已持有，不再重复获取）
	// 优化（Task 8）：从 queued 转为 running（仅在首次进入循环时）
	p.queuedCount.Add(-1)
	p.runningCount.Add(1)
	defer p.runningCount.Add(-1)
	for {
		p.mu.Lock()
		pt.storeStatus(PoolTaskRunning) // perf-v6 Task A
		p.mu.Unlock()
		p.emitEvent(PoolEvent{Type: "task_started", TaskID: task.ID, Data: task.Title})

		taskCtx, cancel := context.WithTimeout(ctx, p.config.Timeout)

		agt, err := p.createAgentForTask(task)
		if err != nil {
			cancel()
			p.mu.Lock()
			pt.storeStatus(PoolTaskFailed)
			p.mu.Unlock()
			p.failedCount.Add(1)
			p.emitEvent(PoolEvent{Type: "task_failed", TaskID: task.ID, Data: err.Error()})
			return &TaskResult{
				TaskID: task.ID,
				Task:   task,
				Error:  err,
				Status: PoolTaskFailed,
			}, err
		}
		result, runErr := agt.Run(taskCtx, agent.UserMessage(task.Prompt))
		err = runErr
		taskCtxErr := taskCtx.Err()
		cancel()

		duration := time.Since(startTime)

		if err != nil {
			if taskCtxErr == context.DeadlineExceeded {
				p.mu.Lock()
				pt.storeStatus(PoolTaskFailed) // perf-v6 Task A
				p.mu.Unlock()
				// 优化（Task 8）：running 转 failed
				p.failedCount.Add(1)
				p.emitEvent(PoolEvent{Type: "task_failed", TaskID: task.ID, Data: "timeout"})
			} else if taskCtxErr == context.Canceled || ctx.Err() != nil {
				p.mu.Lock()
				pt.storeStatus(PoolTaskCancelled) // perf-v6 Task A
				p.mu.Unlock()
				// cancelled 不计入 failed
				p.emitEvent(PoolEvent{Type: "task_cancelled", TaskID: task.ID})
			} else {
				p.mu.Lock()
				pt.storeStatus(PoolTaskFailed) // perf-v6 Task A
				p.mu.Unlock()

				if p.isRetryable(err) && pt.retryCount < p.config.RetryPolicy.MaxRetries {
					pt.retryCount++
					p.mu.Lock()
					p.mu.Unlock()
					time.Sleep(p.config.RetryPolicy.Backoff)
					continue
				}

				// 优化（Task 8）：running 转 failed
				p.failedCount.Add(1)
				p.emitEvent(PoolEvent{Type: "task_failed", TaskID: task.ID, Data: err.Error()})
			}
		} else {
			p.mu.Lock()
			pt.storeStatus(PoolTaskCompleted) // perf-v6 Task A
			p.mu.Unlock()
			// 优化（Task 8）：running 转 completed
			p.completedCount.Add(1)
			p.emitEvent(PoolEvent{Type: "task_completed", TaskID: task.ID})
		}

		pt.result = &TaskResult{
			TaskID:   task.ID,
			Task:     task,
			Response: result,
			Error:    err,
			Duration: duration,
			Status:   pt.loadStatus(),
		}

		p.mu.Lock()
		delete(p.agents, task.ID)
		p.mu.Unlock()

		// 保留 updateStats() 调用以同步 PoolStats 整体结构；Stats() 现在直接读原子变量

		return pt.result, err
	}
}

func (p *Pool) createAgentForTask(task TaskConfig) (agent.Agent, error) {
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
		return agt, nil
	}

	// 解析配置参数
	name := task.Title
	systemPrompt := p.config.DefaultAgent.SystemPrompt
	model := p.model

	maxTurns := task.MaxTurns
	if maxTurns == 0 {
		maxTurns = p.config.DefaultAgent.MaxTurns
	}
	if maxTurns == 0 {
		maxTurns = defaultMaxTurns
	}

	temperature := p.config.DefaultAgent.Temperature

	// 使用 NewAgent 创建 Agent，通过 Option 模式注入配置
	reactAgt, err := agent.NewAgent(name, systemPrompt, model,
		agent.WithMaxTurns(maxTurns),
		agent.WithTemperature(temperature),
	)
	if err != nil {
		return nil, err
	}

	// v0.7.0: Toolkit 字段已废弃，通过链式 API 注入工具能力
	var agt agent.Agent = reactAgt
	if p.toolkit != nil {
		agt = reactAgt.WithToolkit(p.toolkit)
	}

	p.mu.Lock()
	p.agents[task.ID] = agt
	p.mu.Unlock()
	return agt, nil
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

// updateStats 同步原子计数器到 PoolStats 结构体，供向后兼容的 Stats() 调用使用。
// 优化（Task 8）：O(1) 原子读取替代 O(n) 全量遍历 tasks map。
func (p *Pool) updateStats() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stats.RunningTasks = int(p.runningCount.Load())
	p.stats.QueuedTasks = int(p.queuedCount.Load())
	p.stats.CompletedTasks = int(p.completedCount.Load())
	p.stats.FailedTasks = int(p.failedCount.Load())
	p.stats.ActiveConcurrency = p.stats.RunningTasks
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

	if task.loadStatus() == PoolTaskRunning {
		task.cancelFunc()
		task.storeStatus(PoolTaskCancelled) // perf-v6 Task A
	}

	p.emitEvent(PoolEvent{Type: "task_cancelled", TaskID: taskID})
	return nil
}

func (p *Pool) CancelAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, task := range p.tasks {
		if task.loadStatus() == PoolTaskQueued || task.loadStatus() == PoolTaskRunning {
			task.cancelFunc()
			task.storeStatus(PoolTaskCancelled) // perf-v6 Task A
			p.emitEvent(PoolEvent{Type: "task_cancelled", TaskID: id})
		}
	}
}

// GetTasksBySession 返回指定会话的任务结果。
// 优化（perf-v2）：使用会话索引 O(k) 查找替代 O(n) 全量遍历。
func (p *Pool) GetTasksBySession(sessionID string) []TaskResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	taskIDs, ok := p.sessionIndex[sessionID]
	if !ok {
		return nil
	}

	results := make([]TaskResult, 0, len(taskIDs))
	for taskID := range taskIDs {
		pt, exists := p.tasks[taskID]
		if !exists {
			continue
		}
		if pt.result != nil {
			results = append(results, *pt.result)
		} else {
			results = append(results, TaskResult{
				TaskID: pt.config.ID,
				Task:   pt.config,
				Status: pt.loadStatus(),
			})
		}
	}
	return results
}

// CancelBySession 取消指定会话的所有任务。
// 优化（perf-v2）：使用会话索引 O(k) 查找替代 O(n) 全量遍历。
func (p *Pool) CancelBySession(sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	taskIDs, ok := p.sessionIndex[sessionID]
	if !ok {
		return nil
	}

	for taskID := range taskIDs {
		pt, exists := p.tasks[taskID]
		if !exists {
			continue
		}
		if pt.loadStatus() == PoolTaskQueued || pt.loadStatus() == PoolTaskRunning {
			pt.cancelFunc()
			pt.storeStatus(PoolTaskCancelled) // perf-v6 Task A
			p.emitEvent(PoolEvent{Type: "task_cancelled", TaskID: taskID})
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
				Status: pt.loadStatus(),
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

// Stats 直接读取原子计数器 + 缓存的 stats 字段，避免双重锁。
// 优化（Task 8）：Stats() 不再调用 updateStats() 走写锁后再读 RLock，
// 改为仅一次读锁 + 直接读原子变量。
func (p *Pool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	// 原子读取叠加到返回的快照上，避免修改共享的 p.stats
	stats := p.stats
	stats.RunningTasks = int(p.runningCount.Load())
	stats.QueuedTasks = int(p.queuedCount.Load())
	stats.CompletedTasks = int(p.completedCount.Load())
	stats.FailedTasks = int(p.failedCount.Load())
	stats.ActiveConcurrency = stats.RunningTasks
	return stats
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
// 优化（perf-v2）：同步清理会话索引
func (p *Pool) Cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, t := range p.tasks {
		if t.loadStatus() == PoolTaskCompleted || t.loadStatus() == PoolTaskFailed || t.loadStatus() == PoolTaskCancelled {
			// 清理会话索引
			if t.sessionID != "" {
				if ids, ok := p.sessionIndex[t.sessionID]; ok {
					delete(ids, id)
					if len(ids) == 0 {
						delete(p.sessionIndex, t.sessionID)
					}
				}
			}
			delete(p.tasks, id)
		}
	}
}

// cleanupIfNeeded 当 task map 超过 MaxRetainedTasks 阈值时自动清理终态任务（M8 修复）。
// 优化（perf-v2）：同步清理会话索引，避免索引泄漏。
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
		if t.loadStatus() == PoolTaskCompleted || t.loadStatus() == PoolTaskFailed || t.loadStatus() == PoolTaskCancelled {
			// 清理会话索引
			if t.sessionID != "" {
				if ids, ok := p.sessionIndex[t.sessionID]; ok {
					delete(ids, id)
					if len(ids) == 0 {
						delete(p.sessionIndex, t.sessionID)
					}
				}
			}
			delete(p.tasks, id)
		}
	}
}

// GracefulShutdown 优雅关闭：停止接受新任务，等待正在执行的任务完成
func (p *Pool) GracefulShutdown(ctx context.Context) error {
	// 标记为关闭状态，拒绝新任务
	p.shutdown.Store(true)

	// 如果 Pool 已关闭（cancel 已调用），直接返回
	select {
	case <-p.ctx.Done():
		return nil
	default:
	}

	// 等待正在执行的任务完成
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.ctx.Done():
			// Pool 已被 Close()，直接返回
			return nil
		case <-ticker.C:
			if p.runningCount.Load() == 0 {
				p.Close()
				return nil
			}
		}
	}
}

func (p *Pool) Close() {
	p.closeOnce.Do(func() {
		// 广播唤醒所有等待信号量的 goroutine
		p.semaCond.Broadcast()
		p.cancel()
		p.wg.Wait()
		close(p.eventCh)
	})
}

// acquireSlot 获取动态并发度信号量令牌。
// 使用短间隔轮询检查令牌可用性，支持 ctx 取消和超时。
// 返回 error 仅在 ctx 被取消或超时时非 nil。
//
// 设计说明：相比 sync.Cond + goroutine 方案，轮询方式更简洁，
// 避免了每个阻塞调用者创建额外 goroutine（~2KB 栈）。
// 在 dispatcher 场景下并发等待数通常较小，10ms 轮询开销可忽略。
func (p *Pool) acquireSlot(ctx context.Context) error {
	// 快速路径：检查是否可以立即获取
	p.semaMu.Lock()
	if p.semaCount < p.dynamicConcurrency.Load() {
		p.semaCount++
		p.semaMu.Unlock()
		return nil
	}
	p.semaMu.Unlock()

	// 慢路径：轮询检查令牌可用性
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var timerC <-chan time.Time
	if p.config.Timeout > 0 {
		timer := time.NewTimer(p.config.Timeout)
		defer timer.Stop()
		timerC = timer.C
	}

	for {
		p.semaMu.Lock()
		if p.semaCount < p.dynamicConcurrency.Load() {
			p.semaCount++
			p.semaMu.Unlock()
			return nil
		}
		p.semaMu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timerC:
			return ErrTimeout
		case <-ticker.C:
			// 继续轮询
		}
	}
}

// releaseSlot 释放动态并发度信号量令牌，并唤醒一个等待者。
func (p *Pool) releaseSlot() {
	p.semaMu.Lock()
	p.semaCount--
	p.semaCond.Signal()
	p.semaMu.Unlock()
}

// generateTaskID 生成唯一任务 ID。优化（Task 8）：使用 strconv 避免 fmt.Sprintf 的反射分配。
var taskIDCounter atomic.Int64

func generateTaskID(index int) string {
	seq := taskIDCounter.Add(1)
	return "task_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + strconv.FormatInt(int64(index), 10) + "_" + strconv.FormatInt(seq, 10)
}
