// Package autonomy 实现长期自治执行模型（v3.3 核心）。
// 提供目标驱动的自主规划、执行、校验、再计划循环，
// 使 Agent 从"被动会话"跃迁为"给定目标→自主执行数小时/数天"。
package autonomy

import (
	"fmt"
	"sync"
	"time"
)

// GoalState 目标状态
type GoalState int

const (
	// GoalCreated 目标已创建，尚未规划
	GoalCreated GoalState = iota
	// GoalPlanned 目标已分解为执行计划
	GoalPlanned
	// GoalExecuting 正在执行计划步骤
	GoalExecuting
	// GoalValidated 执行完成，正在校验结果
	GoalValidated
	// GoalDone 目标已完成（终态）
	GoalDone
	// GoalFailed 目标失败（终态，可重试转回 planned）
	GoalFailed
)

// String 返回状态的字符串表示
func (s GoalState) String() string {
	switch s {
	case GoalCreated:
		return "created"
	case GoalPlanned:
		return "planned"
	case GoalExecuting:
		return "executing"
	case GoalValidated:
		return "validated"
	case GoalDone:
		return "done"
	case GoalFailed:
		return "failed"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// IsTerminal 判断是否为终态
func (s GoalState) IsTerminal() bool {
	return s == GoalDone || s == GoalFailed
}

// StateChangeEvent 状态变更事件
type StateChangeEvent struct {
	GoalID    string
	From      GoalState
	To        GoalState
	Timestamp time.Time
	Reason    string
}

// StateMachine 目标状态机，管理合法转换与事件发布
type StateMachine struct {
	mu          sync.RWMutex
	transitions map[GoalState][]GoalState
	listeners   []func(StateChangeEvent)
}

// NewStateMachine 创建状态机并注册默认合法转换表
func NewStateMachine() *StateMachine {
	sm := &StateMachine{
		transitions: make(map[GoalState][]GoalState),
	}
	// 合法转换定义
	sm.transitions[GoalCreated] = []GoalState{GoalPlanned, GoalFailed}
	sm.transitions[GoalPlanned] = []GoalState{GoalExecuting, GoalFailed}
	sm.transitions[GoalExecuting] = []GoalState{GoalValidated, GoalFailed}
	sm.transitions[GoalValidated] = []GoalState{GoalDone, GoalExecuting} // executing: 校验不通过→重规划
	sm.transitions[GoalFailed] = []GoalState{GoalPlanned}                // 重试
	// GoalDone 无出边（终态）
	return sm
}

// ValidateTransition 校验状态转换是否合法
func (sm *StateMachine) ValidateTransition(from, to GoalState) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	allowed, ok := sm.transitions[from]
	if !ok {
		return fmt.Errorf("autonomy: 状态 %s 无合法出边", from)
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("autonomy: 非法状态转换 %s → %s", from, to)
}

// Apply 执行状态转换，非法转换返回错误且不改变状态
func (sm *StateMachine) Apply(current GoalState, next GoalState) (GoalState, error) {
	if err := sm.ValidateTransition(current, next); err != nil {
		return current, err
	}

	sm.mu.RLock()
	listeners := make([]func(StateChangeEvent), len(sm.listeners))
	copy(listeners, sm.listeners)
	sm.mu.RUnlock()

	event := StateChangeEvent{
		From:      current,
		To:        next,
		Timestamp: time.Now(),
	}
	for _, fn := range listeners {
		fn(event)
	}
	return next, nil
}

// OnTransition 注册状态变更监听器
func (sm *StateMachine) OnTransition(fn func(StateChangeEvent)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.listeners = append(sm.listeners, fn)
}
