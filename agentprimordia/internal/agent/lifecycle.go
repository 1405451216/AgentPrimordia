package agent

import (
	"agentprimordia/internal/agent/lifecycle"
)

// Lifecycle 是生命周期管理器的类型别名
type Lifecycle = lifecycle.Lifecycle

// StateTransition 是状态转换记录的类型别名
type StateTransition = lifecycle.StateTransition

// StateHook 是状态钩子函数的类型别名
type StateHook = lifecycle.StateHook

// Status 是状态类型的类型别名
type Status = lifecycle.Status

// AgentStatus 是 Status 的类型别名，保持向后兼容
type AgentStatus = Status

// 状态常量
const (
	StatusIdle            = lifecycle.StatusIdle
	StatusRunning         = lifecycle.StatusRunning
	StatusPaused          = lifecycle.StatusPaused
	StatusWaitingForInput = lifecycle.StatusWaitingForInput
	StatusCompleted       = lifecycle.StatusCompleted
	StatusFailed          = lifecycle.StatusFailed
	StatusCancelled       = lifecycle.StatusCancelled
)

// NewLifecycle 创建新的生命周期管理器
func NewLifecycle() *Lifecycle {
	return lifecycle.New()
}

// TransitionGuard 是状态转换守卫的类型别名
type TransitionGuard func(from, to Status) bool

// ErrInvalidTransition 是无效状态转换错误
var ErrInvalidTransition = lifecycle.ErrInvalidTransition

// ErrNotResettable 是不可重置状态错误
var ErrNotResettable = lifecycle.ErrNotResettable

// ErrAgentStopped 是 Agent 已停止错误
var ErrAgentStopped = lifecycle.ErrAgentStopped

// validTransitions 定义合法的状态转换
var validTransitions = map[Status][]Status{
	StatusIdle:            {StatusRunning},
	StatusRunning:         {StatusPaused, StatusWaitingForInput, StatusCompleted, StatusFailed, StatusCancelled},
	StatusPaused:          {StatusRunning, StatusCancelled},
	StatusWaitingForInput: {StatusRunning, StatusCancelled, StatusFailed},
	StatusCompleted:       {StatusIdle},
	StatusFailed:          {StatusIdle},
	StatusCancelled:       {StatusIdle},
}

// isValidTransition 检查状态转换是否合法
func isValidTransition(from, to Status) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}
