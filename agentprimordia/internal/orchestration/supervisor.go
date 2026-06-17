package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultSupervisorTimeout    = 5 * time.Minute
	supervisorEventBufferSize   = 100
	defaultWorkerMaxConcurrency = 10
)

// ===== Worker 接口 =====

// Worker 工作者最小接口：仅要求 Execute 与 ID
type Worker interface {
	// Execute 执行任务，返回任务结果
	Execute(ctx context.Context, task *Task) (*TaskResult, error)
	// ID 返回工作者唯一标识
	ID() string
}

// ===== Task / TaskResult =====

// Task 任务定义
type Task struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Type           string         `json:"type"`                      // 任务类型，用于技能匹配
	Payload        map[string]any `json:"payload"`                   // 任务载荷
	RequiredSkills []string       `json:"required_skills,omitempty"` // 所需技能标签
	Priority       int            `json:"priority"`                  // 优先级（0-10）
	Timeout        time.Duration  `json:"timeout,omitempty"`         // 单任务超时
}

// TaskStatus 任务状态
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// TaskResult 任务执行结果
type TaskResult struct {
	WorkerID string         `json:"worker_id"`
	TaskID   string         `json:"task_id"`
	Status   TaskStatus     `json:"status"`
	Output   map[string]any `json:"output,omitempty"`
	Error    error          `json:"error,omitempty"`
	Duration time.Duration  `json:"duration"`
}

// ===== WorkerState =====

// WorkerState worker 在 supervisor 中的运行时状态
type WorkerState struct {
	Worker         Worker   `json:"-"`
	ID             string   `json:"id"`
	Skills         []string `json:"skills,omitempty"`
	MaxConcurrency int      `json:"max_concurrency"`

	// 运行时指标（原子操作保护）
	activeTasks    int64
	totalCompleted int64
	totalFailed    int64
	available      bool
}

// ActiveTasks 返回当前活跃任务数
func (w *WorkerState) ActiveTasks() int {
	return int(atomic.LoadInt64(&w.activeTasks))
}

// TotalCompleted 返回累计完成任务数
func (w *WorkerState) TotalCompleted() int {
	return int(atomic.LoadInt64(&w.totalCompleted))
}

// TotalFailed 返回累计失败任务数
func (w *WorkerState) TotalFailed() int {
	return int(atomic.LoadInt64(&w.totalFailed))
}

// Available 返回是否可用
func (w *WorkerState) Available() bool {
	return w.available && w.ActiveTasks() < w.MaxConcurrency
}

// ===== AssignmentStrategy 接口 =====

// AssignmentStrategy 任务分配策略
type AssignmentStrategy interface {
	// Name 策略名称
	Name() string
	// Select 从候选 workers 中为 task 选择一个
	Select(task *Task, workers []*WorkerState) (*WorkerState, error)
}

// ===== 内置策略 =====

// RoundRobinStrategy 轮询分配策略
type RoundRobinStrategy struct {
	mu    sync.Mutex
	index int
}

// NewRoundRobinStrategy 创建轮询策略
func NewRoundRobinStrategy() *RoundRobinStrategy {
	return &RoundRobinStrategy{}
}

func (s *RoundRobinStrategy) Name() string { return "round_robin" }

func (s *RoundRobinStrategy) Select(_ *Task, workers []*WorkerState) (*WorkerState, error) {
	available := filterAvailable(workers)
	if len(available) == 0 {
		return nil, fmt.Errorf("no available workers")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 轮询索引对可用 worker 取模
	idx := s.index % len(available)
	s.index++
	return available[idx], nil
}

// LoadBalancedStrategy 基于当前负载分配（选择活跃任务最少的 worker）
type LoadBalancedStrategy struct{}

func NewLoadBalancedStrategy() *LoadBalancedStrategy { return &LoadBalancedStrategy{} }

func (s *LoadBalancedStrategy) Name() string { return "load_balanced" }

func (s *LoadBalancedStrategy) Select(_ *Task, workers []*WorkerState) (*WorkerState, error) {
	available := filterAvailable(workers)
	if len(available) == 0 {
		return nil, fmt.Errorf("no available workers")
	}

	var best *WorkerState
	bestLoad := int(^uint(0) >> 1) // max int
	for _, w := range available {
		load := w.ActiveTasks()
		if load < bestLoad {
			best = w
			bestLoad = load
		}
	}
	return best, nil
}

// SkillBasedStrategy 基于技能标签匹配分配
type SkillBasedStrategy struct {
	// Fallback 当无技能匹配时的回退策略；为 nil 时使用轮询
	Fallback AssignmentStrategy
}

func NewSkillBasedStrategy() *SkillBasedStrategy {
	return &SkillBasedStrategy{}
}

func (s *SkillBasedStrategy) Name() string { return "skill_based" }

func (s *SkillBasedStrategy) Select(task *Task, workers []*WorkerState) (*WorkerState, error) {
	available := filterAvailable(workers)
	if len(available) == 0 {
		return nil, fmt.Errorf("no available workers")
	}

	// 无技能要求时，回退到 Fallback 或轮询
	if len(task.RequiredSkills) == 0 {
		if s.Fallback != nil {
			return s.Fallback.Select(task, available)
		}
		return available[0], nil
	}

	// 计算技能匹配度（命中技能数）
	type candidate struct {
		worker *WorkerState
		score  int
	}
	var candidates []candidate
	for _, w := range available {
		score := skillMatchScore(w.Skills, task.RequiredSkills)
		if score > 0 {
			candidates = append(candidates, candidate{worker: w, score: score})
		}
	}

	if len(candidates) == 0 {
		// 无匹配时回退
		if s.Fallback != nil {
			return s.Fallback.Select(task, available)
		}
		return nil, fmt.Errorf("no worker matches required skills: %v", task.RequiredSkills)
	}

	// 选择得分最高的；得分相同时选负载较低的
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		} else if c.score == best.score && c.worker.ActiveTasks() < best.worker.ActiveTasks() {
			best = c
		}
	}
	return best.worker, nil
}

// skillMatchScore 计算 worker 技能与所需技能的命中数
func skillMatchScore(workerSkills, required []string) int {
	set := make(map[string]struct{}, len(workerSkills))
	for _, s := range workerSkills {
		set[strings.ToLower(s)] = struct{}{}
	}
	score := 0
	for _, r := range required {
		if _, ok := set[strings.ToLower(r)]; ok {
			score++
		}
	}
	return score
}

// filterAvailable 过滤出可用的 worker
func filterAvailable(workers []*WorkerState) []*WorkerState {
	out := make([]*WorkerState, 0, len(workers))
	for _, w := range workers {
		if w.Available() {
			out = append(out, w)
		}
	}
	return out
}

// ===== Supervisor =====

// SupervisorConfig supervisor 配置
type SupervisorConfig struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Timeout     time.Duration `json:"timeout"`     // 全局超时
	MaxRetries  int           `json:"max_retries"` // 失败重试次数
}

// SupervisorEvent supervisor 事件
type SupervisorEvent struct {
	Type      string    `json:"type"` // worker_added, worker_removed, task_assigned, task_completed, task_failed
	Timestamp time.Time `json:"timestamp"`
	WorkerID  string    `json:"worker_id,omitempty"`
	TaskID    string    `json:"task_id,omitempty"`
	Data      any       `json:"data,omitempty"`
}

// Supervisor 监督者：管理多个 Worker 并根据策略分配任务
type Supervisor struct {
	mu       sync.RWMutex
	config   SupervisorConfig
	workers  map[string]*WorkerState
	strategy AssignmentStrategy
	eventCh  chan *SupervisorEvent

	// 运行时统计
	totalAssigned  int64
	totalCompleted int64
	totalFailed    int64
}

// NewSupervisor 创建 supervisor
func NewSupervisor(config SupervisorConfig, strategy AssignmentStrategy) (*Supervisor, error) {
	if strategy == nil {
		return nil, fmt.Errorf("strategy is required")
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultSupervisorTimeout
	}
	if config.Name == "" {
		config.Name = "supervisor"
	}

	return &Supervisor{
		config:   config,
		workers:  make(map[string]*WorkerState),
		strategy: strategy,
		eventCh:  make(chan *SupervisorEvent, supervisorEventBufferSize),
	}, nil
}

// AddWorker 动态添加 worker
func (s *Supervisor) AddWorker(w Worker, skills []string, maxConcurrency int) error {
	if w == nil || w.ID() == "" {
		return fmt.Errorf("invalid worker: nil or empty ID")
	}
	if maxConcurrency <= 0 {
		maxConcurrency = defaultWorkerMaxConcurrency
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.workers[w.ID()]; exists {
		return fmt.Errorf("worker already exists: %s", w.ID())
	}

	state := &WorkerState{
		Worker:         w,
		ID:             w.ID(),
		Skills:         append([]string(nil), skills...),
		MaxConcurrency: maxConcurrency,
		available:      true,
	}
	s.workers[w.ID()] = state

	s.emitEvent(&SupervisorEvent{
		Type:     "worker_added",
		WorkerID: w.ID(),
		Data:     map[string]any{"skills": skills, "max_concurrency": maxConcurrency},
	})
	return nil
}

// RemoveWorker 动态移除 worker（标记为不可用并从池中移除；若有活跃任务则等待完成）
func (s *Supervisor) RemoveWorker(workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.workers[workerID]
	if !exists {
		return fmt.Errorf("worker not found: %s", workerID)
	}
	if state.ActiveTasks() > 0 {
		// 先标记为不可用，不再分配新任务；不立即删除避免影响正在执行的任务
		state.available = false
		return fmt.Errorf("worker %s has %d active tasks, marked unavailable (will be removed when idle)", workerID, state.ActiveTasks())
	}

	delete(s.workers, workerID)
	s.emitEvent(&SupervisorEvent{
		Type:     "worker_removed",
		WorkerID: workerID,
	})
	return nil
}

// SetStrategy 运行时切换策略
func (s *Supervisor) SetStrategy(strategy AssignmentStrategy) error {
	if strategy == nil {
		return fmt.Errorf("strategy is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.strategy = strategy
	return nil
}

// Execute 根据策略选择 worker 并执行任务
func (s *Supervisor) Execute(ctx context.Context, task *Task) (*TaskResult, error) {
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}

	// 全局超时
	ctx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()

	// 单任务超时覆盖
	if task.Timeout > 0 {
		var cancelTask context.CancelFunc
		ctx, cancelTask = context.WithTimeout(ctx, task.Timeout)
		defer cancelTask()
	}

	// 选择 worker
	state, err := s.selectWorker(task)
	if err != nil {
		return nil, fmt.Errorf("select worker failed: %w", err)
	}

	// 增加活跃计数
	atomic.AddInt64(&state.activeTasks, 1)
	defer atomic.AddInt64(&state.activeTasks, -1)

	atomic.AddInt64(&s.totalAssigned, 1)
	s.emitEvent(&SupervisorEvent{
		Type:     "task_assigned",
		WorkerID: state.ID,
		TaskID:   task.ID,
		Data:     map[string]any{"task_name": task.Name, "strategy": s.strategy.Name()},
	})

	// 执行（带重试）
	start := time.Now()
	retries := s.config.MaxRetries
	var result *TaskResult
	var execErr error

	for attempt := 0; attempt <= retries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		result, execErr = state.Worker.Execute(ctx, task)
		if execErr == nil && result != nil && result.Status != TaskStatusFailed {
			break
		}
		// 最后一次重试仍失败则退出
		if attempt == retries {
			break
		}
		// 简单线性退避
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
	}

	if result == nil {
		result = &TaskResult{
			WorkerID: state.ID,
			TaskID:   task.ID,
			Status:   TaskStatusFailed,
			Error:    execErr,
		}
	}
	result.Duration = time.Since(start)

	if result.Status == TaskStatusFailed || execErr != nil {
		atomic.AddInt64(&s.totalFailed, 1)
		atomic.AddInt64(&state.totalFailed, 1)
		if execErr != nil {
			result.Error = execErr
		}
		s.emitEvent(&SupervisorEvent{
			Type:     "task_failed",
			WorkerID: state.ID,
			TaskID:   task.ID,
			Data:     map[string]any{"error": result.Error, "attempts": retries + 1},
		})
		return result, result.Error
	}

	atomic.AddInt64(&s.totalCompleted, 1)
	atomic.AddInt64(&state.totalCompleted, 1)
	s.emitEvent(&SupervisorEvent{
		Type:     "task_completed",
		WorkerID: state.ID,
		TaskID:   task.ID,
		Data:     map[string]any{"duration": result.Duration},
	})

	return result, nil
}

// selectWorker 通过策略选择 worker（读锁内收集列表，按 ID 排序确保确定性）
func (s *Supervisor) selectWorker(task *Task) (*WorkerState, error) {
	s.mu.RLock()
	strategy := s.strategy
	workers := make([]*WorkerState, 0, len(s.workers))
	for _, w := range s.workers {
		workers = append(workers, w)
	}
	s.mu.RUnlock()

	if len(workers) == 0 {
		return nil, fmt.Errorf("no workers registered")
	}

	// 按 ID 排序，确保 map 随机迭代不影响策略的确定性
	sort.Slice(workers, func(i, j int) bool {
		return workers[i].ID < workers[j].ID
	})

	return strategy.Select(task, workers)
}

// Workers 返回当前所有 worker 状态快照
func (s *Supervisor) Workers() []*WorkerState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*WorkerState, 0, len(s.workers))
	for _, w := range s.workers {
		out = append(out, w)
	}
	return out
}

// WorkerCount 返回 worker 数量
func (s *Supervisor) WorkerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.workers)
}

// Strategy 返回当前策略名称
func (s *Supervisor) Strategy() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.strategy.Name()
}

// Stats 返回 supervisor 统计
func (s *Supervisor) Stats() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]any{
		"name":            s.config.Name,
		"strategy":        s.strategy.Name(),
		"worker_count":    len(s.workers),
		"total_assigned":  atomic.LoadInt64(&s.totalAssigned),
		"total_completed": atomic.LoadInt64(&s.totalCompleted),
		"total_failed":    atomic.LoadInt64(&s.totalFailed),
	}
}

// Events 返回事件通道
func (s *Supervisor) Events() <-chan *SupervisorEvent {
	return s.eventCh
}

// Export 导出为 JSON
func (s *Supervisor) Export() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data := map[string]any{
		"config":   s.config,
		"strategy": s.strategy.Name(),
		"stats":    s.Stats(),
		"workers":  s.workerSnapshotLocked(),
	}
	return json.MarshalIndent(data, "", "  ")
}

func (s *Supervisor) workerSnapshotLocked() []map[string]any {
	out := make([]map[string]any, 0, len(s.workers))
	for _, w := range s.workers {
		out = append(out, map[string]any{
			"id":              w.ID,
			"skills":          w.Skills,
			"max_concurrency": w.MaxConcurrency,
			"active_tasks":    w.ActiveTasks(),
			"total_completed": w.TotalCompleted(),
			"total_failed":    w.TotalFailed(),
			"available":       w.Available(),
		})
	}
	return out
}

func (s *Supervisor) emitEvent(ev *SupervisorEvent) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	select {
	case s.eventCh <- ev:
	default:
	}
}
