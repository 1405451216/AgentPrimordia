// Package planning 提供计划状态机——管理计划生命周期转换
package planning

import (
	"fmt"
	"time"
)

// validTransitions 定义合法的状态转换路径
var validTransitions = map[PlanState][]PlanState{
	PlanStatePending: {PlanStateActive, PlanStateFailed},
	PlanStateActive:  {PlanStateBlocked, PlanStateCompleted, PlanStateFailed},
	PlanStateBlocked: {PlanStateActive, PlanStateFailed},
}

// Transition 执行状态转换，验证合法性并记录历史
func (mp *ManagedPlan) Transition(to PlanState, reason string) error {
	if mp.State == PlanStateCompleted || mp.State == PlanStateFailed {
		return fmt.Errorf("plan in terminal state %s, cannot transition to %s", mp.State, to)
	}

	allowed := false
	for _, s := range validTransitions[mp.State] {
		if s == to {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("invalid transition %s → %s", mp.State, to)
	}

	mp.History = append(mp.History, PlanTransition{
		From: mp.State,
		To:   to,
		At:   time.Now(),
		Why:  reason,
	})
	mp.State = to
	mp.UpdatedAt = time.Now()
	return nil
}

// NewManagedPlan 创建带状态机的计划实例
func NewManagedPlan(plan *Plan) *ManagedPlan {
	now := time.Now()
	return &ManagedPlan{
		Plan:      plan,
		State:     PlanStatePending,
		History:   nil,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// IsTerminal 判断当前状态是否为终态
func (mp *ManagedPlan) IsTerminal() bool {
	return mp.State == PlanStateCompleted || mp.State == PlanStateFailed
}
