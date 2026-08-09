package autonomy

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Priority 目标优先级
type Priority int

const (
	// PriorityLow 低优先级
	PriorityLow Priority = 0
	// PriorityNormal 普通优先级（默认）
	PriorityNormal Priority = 1
	// PriorityHigh 高优先级
	PriorityHigh Priority = 2
	// PriorityCritical 紧急优先级
	PriorityCritical Priority = 3
)

// GoalConfig 目标创建配置
type GoalConfig struct {
	// AcceptanceCriteria 验收标准列表
	AcceptanceCriteria []string
	// Priority 优先级（默认 PriorityNormal）
	Priority Priority
	// MaxRetries 最大重试次数（默认 3）
	MaxRetries int
	// Deadline 截止时间（零值表示无限制）
	Deadline time.Time
	// Metadata 附加元数据
	Metadata map[string]string
	// BudgetUSD 目标级成本预算（v4.9-4：0 表示不限；重规划消耗 ReplanCostUSD）
	BudgetUSD float64
	// ReplanCostUSD 每次重规划的 LLM 成本估计（默认 0.01 USD）
	ReplanCostUSD float64
}

// AgentGoal 持久化目标：自治执行的核心载体
type AgentGoal struct {
	mu sync.RWMutex

	// ID 全局唯一标识
	ID string `json:"id"`
	// Description 目标描述
	Description string `json:"description"`
	// AcceptanceCriteria 验收标准
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	// Priority 优先级
	Priority Priority `json:"priority"`
	// State 当前状态
	State GoalState `json:"state"`
	// MaxRetries 最大重试次数
	MaxRetries int `json:"max_retries"`
	// RetryCount 已重试次数
	RetryCount int `json:"retry_count"`
	// Deadline 截止时间
	Deadline time.Time `json:"deadline,omitempty"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后更新时间
	UpdatedAt time.Time `json:"updated_at"`
	// CostSpentUSD 已消耗成本（v4.9-4 目标级预算；原子访问）
	CostSpentUSD float64 `json:"cost_spent_usd"`
	// budgetUSD 目标预算（内部字段，经 Budget 方法读取）
	budgetUSD    float64
	// replanCostUSD 每次重规划成本估计
	replanCostUSD float64

	// metadata 附加元数据
	metadata map[string]string
	// history 状态变更历史
	history []StateChangeEvent
	// sm 状态机
	sm *StateMachine
}

// AgentGoalView 目标的并发安全只读快照（供 Studio 面板/监控等外部消费）。
type AgentGoalView struct {
	// ID 全局唯一标识
	ID string
	// Description 目标描述
	Description string
	// Priority 优先级
	Priority Priority
	// State 当前状态
	State GoalState
	// MaxRetries 最大重试次数
	MaxRetries int
	// RetryCount 已重试次数
	RetryCount int
	// Deadline 截止时间（零值表示无限制）
	Deadline time.Time
	// CreatedAt 创建时间
	CreatedAt time.Time
	// UpdatedAt 最后更新时间
	UpdatedAt time.Time
}

// Budget 返回目标级成本预算（0 表示不限）。
func (g *AgentGoal) Budget() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.budgetUSD
}

// CostSpent 返回已消耗成本。
func (g *AgentGoal) CostSpent() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.CostSpentUSD
}

// ReplanCost 返回每次重规划的成本估计（默认 0.01 USD）。
func (g *AgentGoal) ReplanCost() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.replanCostUSD <= 0 {
		return 0.01
	}
	return g.replanCostUSD
}

// Charge 记账一次成本消耗（预算已满时返回 ErrGoalBudgetExceeded）。
func (g *AgentGoal) Charge(cost float64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.budgetUSD > 0 && g.CostSpentUSD+cost > g.budgetUSD {
		return ErrGoalBudgetExceeded
	}
	g.CostSpentUSD += cost
	return nil
}

// Snapshot 返回目标的只读快照（并发安全，RWMutex 保护）。
func (g *AgentGoal) Snapshot() AgentGoalView {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return AgentGoalView{
		ID:          g.ID,
		Description: g.Description,
		Priority:    g.Priority,
		State:       g.State,
		MaxRetries:  g.MaxRetries,
		RetryCount:  g.RetryCount,
		Deadline:    g.Deadline,
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
	}
}

// NewAgentGoal 创建新的自治目标
func NewAgentGoal(description string, cfg GoalConfig) *AgentGoal {
	priority := cfg.Priority
	if priority == 0 {
		priority = PriorityNormal
	}
	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	g := &AgentGoal{
		ID:              generateGoalID(),
		Description:     description,
		AcceptanceCriteria: cfg.AcceptanceCriteria,
		Priority:        priority,
		State:           GoalCreated,
		MaxRetries:      maxRetries,
		Deadline:        cfg.Deadline,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		budgetUSD:       cfg.BudgetUSD,
		replanCostUSD:   cfg.ReplanCostUSD,
		metadata:        cfg.Metadata,
		sm:              NewStateMachine(),
	}
	if g.metadata == nil {
		g.metadata = make(map[string]string)
	}

	// 记录状态变更历史
	g.sm.OnTransition(func(e StateChangeEvent) {
		e.GoalID = g.ID
		g.history = append(g.history, e)
	})
	return g
}

// TransitionTo 执行状态转换（非法转换返回错误且不改变状态）
func (g *AgentGoal) TransitionTo(next GoalState) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	newState, err := g.sm.Apply(g.State, next)
	if err != nil {
		return err
	}
	g.State = newState
	g.UpdatedAt = time.Now()
	return nil
}

// IncrementRetry 增加重试计数
func (g *AgentGoal) IncrementRetry() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.RetryCount++
	g.UpdatedAt = time.Now()
}

// CanRetry 判断是否还能重试
func (g *AgentGoal) CanRetry() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.RetryCount < g.MaxRetries
}

// SetMetadata 设置元数据
func (g *AgentGoal) SetMetadata(key, value string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.metadata[key] = value
}

// GetMetadata 获取元数据
func (g *AgentGoal) GetMetadata(key string) string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.metadata[key]
}

// History 返回状态变更历史（副本）
func (g *AgentGoal) History() []StateChangeEvent {
	g.mu.RLock()
	defer g.mu.RUnlock()
	h := make([]StateChangeEvent, len(g.history))
	copy(h, g.history)
	return h
}

// IsExpired 判断目标是否已超过截止时间
func (g *AgentGoal) IsExpired() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.Deadline.IsZero() {
		return false
	}
	return time.Now().After(g.Deadline)
}

// generateGoalID 生成唯一目标 ID
func generateGoalID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "goal-" + hex.EncodeToString(b)
}
