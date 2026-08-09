package autonomy

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// RuntimeConfig 自治运行时配置
type RuntimeConfig struct {
	// StepExecutor 步骤执行器（必须）
	StepExecutor StepExecutor
	// MemoryStore 记忆存储（可选）
	MemoryStore MemoryStore
	// CheckpointStore 检查点存储（可选）
	CheckpointStore CheckpointStore
	// ReplanPlanner 重规划器（可选）
	ReplanPlanner ReplanPlanner
	// SchedulerConfig 调度器配置
	SchedulerConfig SchedulerConfig
	// MonitorConfig 监控器配置
	MonitorConfig MonitorConfig
	// MaxRetries 步骤级最大重试次数
	MaxRetries int
	// MaxReplans 最大重规划次数
	MaxReplans int
}

// AutonomyRuntime 自治运行时：装配目标分解→计划→执行→校验→再计划循环
type AutonomyRuntime struct {
	mu        sync.RWMutex
	cfg       RuntimeConfig
	executor  *GoalExecutor
	scheduler *Scheduler
	monitor   *Monitor
	memory    *GoalMemory
	resume    *ResumeManager
	replanner *Replanner
	idemp     *IdempotencyGuard
	goals     map[string]*AgentGoal
	plans     map[string]*GoalPlan
}

// NewAutonomyRuntime 创建自治运行时
func NewAutonomyRuntime(cfg RuntimeConfig) *AutonomyRuntime {
	rt := &AutonomyRuntime{
		cfg:       cfg,
		executor:  NewGoalExecutor(GoalExecutorConfig{StepExecutor: cfg.StepExecutor, MaxRetries: cfg.MaxRetries}),
		scheduler: NewScheduler(cfg.SchedulerConfig),
		monitor:   NewMonitor(cfg.MonitorConfig),
		idemp:     NewIdempotencyGuard(),
		goals:     make(map[string]*AgentGoal),
		plans:     make(map[string]*GoalPlan),
	}
	if cfg.MemoryStore != nil {
		rt.memory = NewGoalMemory(cfg.MemoryStore)
	}
	if cfg.CheckpointStore != nil {
		rt.resume = NewResumeManager(cfg.CheckpointStore)
	}
	if cfg.ReplanPlanner != nil {
		rt.replanner = NewReplanner(ReplannerConfig{Planner: cfg.ReplanPlanner, MaxReplans: cfg.MaxReplans})
	}
	return rt
}

// SubmitGoal 提交新目标
func (rt *AutonomyRuntime) SubmitGoal(description string, cfg GoalConfig) *AgentGoal {
	goal := NewAgentGoal(description, cfg)
	rt.mu.Lock()
	rt.goals[goal.ID] = goal
	rt.mu.Unlock()
	return goal
}

// SetPlan 为目标设置执行计划
func (rt *AutonomyRuntime) SetPlan(goalID string, plan *GoalPlan) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	goal, ok := rt.goals[goalID]
	if !ok {
		return fmt.Errorf("autonomy: 目标 %s 不存在", goalID)
	}
	if err := goal.TransitionTo(GoalPlanned); err != nil {
		return fmt.Errorf("autonomy: 目标 %s 状态转换失败: %w", goalID, err)
	}
	rt.plans[goalID] = plan
	return nil
}

// ExecuteGoal 执行目标（阻塞直到完成或失败）
func (rt *AutonomyRuntime) ExecuteGoal(ctx context.Context, goalID string) error {
	rt.mu.RLock()
	goal, ok := rt.goals[goalID]
	plan, hasPlan := rt.plans[goalID]
	rt.mu.RUnlock()

	if !ok {
		return fmt.Errorf("autonomy: 目标 %s 不存在", goalID)
	}
	if !hasPlan {
		return fmt.Errorf("autonomy: 目标 %s 无执行计划", goalID)
	}

	// 状态转换：planned → executing
	if err := goal.TransitionTo(GoalExecuting); err != nil {
		return fmt.Errorf("autonomy: 目标 %s 无法进入执行状态: %w", goalID, err)
	}

	// 执行计划
	err := rt.executor.Execute(ctx, plan)

	// 保存检查点
	if rt.resume != nil {
		if plan.IsComplete() {
			_ = rt.resume.SaveCheckpoint(ctx, goalID, goal.Description, plan, GoalValidated)
		} else {
			_ = rt.resume.SaveCheckpoint(ctx, goalID, goal.Description, plan, GoalExecuting)
		}
	}

	if err != nil {
		// 执行失败
		_ = goal.TransitionTo(GoalFailed)
		if rt.memory != nil {
			_ = rt.memory.SaveFailure(ctx, goalID, "", err.Error(), "执行中断")
		}
		return fmt.Errorf("autonomy: 目标 %s 执行失败: %w", goalID, err)
	}

	// 执行成功 → validated
	if err := goal.TransitionTo(GoalValidated); err != nil {
		return err
	}

	// 上报进度
	rt.monitor.ReportHeartbeat(goalID, 1.0)
	return nil
}

// CompleteGoal 标记目标完成（校验通过后调用）
func (rt *AutonomyRuntime) CompleteGoal(goalID string) error {
	rt.mu.RLock()
	goal, ok := rt.goals[goalID]
	rt.mu.RUnlock()
	if !ok {
		return fmt.Errorf("autonomy: 目标 %s 不存在", goalID)
	}
	return goal.TransitionTo(GoalDone)
}

// GetGoal 获取目标
func (rt *AutonomyRuntime) GetGoal(goalID string) (*AgentGoal, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	g, ok := rt.goals[goalID]
	return g, ok
}

// GetPlan 获取计划
func (rt *AutonomyRuntime) GetPlan(goalID string) (*GoalPlan, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	p, ok := rt.plans[goalID]
	return p, ok
}

// ListGoals 列出全部目标（指针快照，按创建时间新→旧）。
// 供 Studio 面板等轮询消费；读取目标字段请用 AgentGoal.Snapshot 保证并发安全。
func (rt *AutonomyRuntime) ListGoals() []*AgentGoal {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	goals := make([]*AgentGoal, 0, len(rt.goals))
	for _, g := range rt.goals {
		goals = append(goals, g)
	}
	sort.Slice(goals, func(i, j int) bool {
		return goals[i].CreatedAt.After(goals[j].CreatedAt)
	})
	return goals
}

// GetMonitor 获取监控器
func (rt *AutonomyRuntime) GetMonitor() *Monitor {
	return rt.monitor
}

// GetScheduler 获取调度器
func (rt *AutonomyRuntime) GetScheduler() *Scheduler {
	return rt.scheduler
}

// GetIdempotencyGuard 获取幂等保护器
func (rt *AutonomyRuntime) GetIdempotencyGuard() *IdempotencyGuard {
	return rt.idemp
}

// ResumeIncomplete 恢复所有未完成目标（启动时调用）
func (rt *AutonomyRuntime) ResumeIncomplete(ctx context.Context) ([]string, error) {
	if rt.resume == nil {
		return nil, nil
	}

	checkpoints, err := rt.resume.ScanIncomplete(ctx)
	if err != nil {
		return nil, fmt.Errorf("autonomy: 扫描未完成目标失败: %w", err)
	}

	var resumed []string
	for _, cp := range checkpoints {
		if err := rt.resume.ValidateConsistency(cp); err != nil {
			// 一致性校验失败，跳过（需重规划）
			continue
		}
		rt.mu.Lock()
		rt.plans[cp.GoalID] = cp.PlanSnapshot
		// v4.5-1 跨节点续跑：重建目标（ID 与检查点一致，状态推进到 planned 续跑）
		if _, exists := rt.goals[cp.GoalID]; !exists {
			goal := NewAgentGoal(cp.GoalDescription, GoalConfig{})
			goal.ID = cp.GoalID
			_ = goal.TransitionTo(GoalPlanned)
			rt.goals[cp.GoalID] = goal
		}
		rt.mu.Unlock()
		resumed = append(resumed, cp.GoalID)
	}
	return resumed, nil
}
