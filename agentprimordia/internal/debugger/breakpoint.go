package debugger

import (
	"fmt"
	"sync"
)

// BreakpointAction 定义断点触发时的行为
type BreakpointAction int

const (
	// ActionPause 暂停执行
	ActionPause BreakpointAction = iota
	// ActionLog 仅记录日志，不暂停
	ActionLog
	// ActionContinue 继续执行，不做任何事（仅注册用）
	ActionContinue
)

// AgentState 表示 Agent 在某个时间点的状态，用于断点条件判断和快照
type AgentState struct {
	Turn   int
	Status string
	Memory *DebugMemorySnapshot
	// 扩展字段
	Attributes map[string]interface{}
}

// DebugMemorySnapshot 表示 Agent 记忆的当前状态
type DebugMemorySnapshot struct {
	Latest   string
	Summary  string
	Episodes int
}

// Breakpoint 表示一个条件断点
type Breakpoint struct {
	StepName  string                      // 断点名称
	Condition func(state *AgentState) bool // 条件断点函数，nil 表示总是匹配
	Action    BreakpointAction            // 触发后的行为
}

// Match 检查断点是否匹配给定状态
// 如果 Condition 为 nil，则总是返回 true（无条件断点）
func (bp *Breakpoint) Match(state *AgentState) bool {
	if bp == nil {
		return false
	}
	if bp.Condition == nil {
		return true
	}
	return bp.Condition(state)
}

// BreakpointManager 管理一组断点，支持添加、移除、清空和检查
type BreakpointManager struct {
	mu         sync.RWMutex
	breakpoints []*Breakpoint
}

// NewBreakpointManager 创建一个新的断点管理器
func NewBreakpointManager() *BreakpointManager {
	return &BreakpointManager{
		breakpoints: make([]*Breakpoint, 0),
	}
}

// Add 注册一个断点
func (m *BreakpointManager) Add(bp *Breakpoint) {
	if bp == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.breakpoints = append(m.breakpoints, bp)
}

// Remove 按名称移除一个断点
func (m *BreakpointManager) Remove(stepName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, bp := range m.breakpoints {
		if bp.StepName == stepName {
			m.breakpoints = append(m.breakpoints[:i], m.breakpoints[i+1:]...)
			return
		}
	}
}

// Clear 移除所有断点
func (m *BreakpointManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.breakpoints = m.breakpoints[:0]
}

// Check 检查是否有任意断点匹配给定状态
// 返回 true 表示至少有一个断点匹配
func (m *BreakpointManager) Check(state *AgentState) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, bp := range m.breakpoints {
		if bp.Match(state) {
			return true
		}
	}
	return false
}

// GetBreakpoints 返回所有已注册的断点（拷贝）
func (m *BreakpointManager) GetBreakpoints() []*Breakpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Breakpoint, len(m.breakpoints))
	copy(result, m.breakpoints)
	return result
}

// String 返回 BreakpointAction 的可读名称
func (a BreakpointAction) String() string {
	switch a {
	case ActionPause:
		return "pause"
	case ActionLog:
		return "log"
	case ActionContinue:
		return "continue"
	default:
		return fmt.Sprintf("unknown(%d)", int(a))
	}
}
