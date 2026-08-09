package autonomy

import (
	"context"
	"fmt"
	"time"
)

// Checkpoint 自治目标检查点快照
type Checkpoint struct {
	// GoalID 目标 ID
	GoalID string `json:"goal_id"`
	// GoalDescription 目标描述（v4.5-1：跨节点续跑时重建目标用）
	GoalDescription string `json:"goal_description,omitempty"`
	// State 目标当前状态
	State GoalState `json:"state"`
	// LastCompletedStep 最后完成的步骤 ID
	LastCompletedStep string `json:"last_completed_step"`
	// PlanSnapshot 计划快照
	PlanSnapshot *GoalPlan `json:"plan_snapshot"`
	// Timestamp 检查点时间
	Timestamp time.Time `json:"timestamp"`
	// Completed 目标是否已完成
	Completed bool `json:"completed"`
}

// CheckpointStore 检查点存储接口（由外部 persist/ 包实现）
type CheckpointStore interface {
	// SaveCheckpoint 保存检查点
	SaveCheckpoint(ctx context.Context, cp *Checkpoint) error
	// LoadCheckpoint 加载指定目标的检查点
	LoadCheckpoint(ctx context.Context, goalID string) (*Checkpoint, error)
	// ListIncomplete 列出所有未完成目标的检查点
	ListIncomplete(ctx context.Context) ([]*Checkpoint, error)
}

// ResumeManager 崩溃恢复管理器
type ResumeManager struct {
	store CheckpointStore
}

// NewResumeManager 创建恢复管理器
func NewResumeManager(store CheckpointStore) *ResumeManager {
	return &ResumeManager{store: store}
}

// SaveCheckpoint 保存当前执行状态为检查点
func (rm *ResumeManager) SaveCheckpoint(ctx context.Context, goalID, description string, plan *GoalPlan, state GoalState) error {
	lastCompleted := ""
	for _, s := range plan.Steps {
		if s.Status == StepCompleted {
			lastCompleted = s.ID
		}
	}

	cp := &Checkpoint{
		GoalID:            goalID,
		GoalDescription:   description,
		State:             state,
		LastCompletedStep: lastCompleted,
		PlanSnapshot:      plan,
		Timestamp:         time.Now(),
		Completed:         state.IsTerminal(),
	}
	return rm.store.SaveCheckpoint(ctx, cp)
}

// LoadCheckpoint 加载目标的检查点
func (rm *ResumeManager) LoadCheckpoint(ctx context.Context, goalID string) (*Checkpoint, error) {
	return rm.store.LoadCheckpoint(ctx, goalID)
}

// ScanIncomplete 扫描所有未完成目标（启动时调用）
func (rm *ResumeManager) ScanIncomplete(ctx context.Context) ([]*Checkpoint, error) {
	return rm.store.ListIncomplete(ctx)
}

// ValidateConsistency 恢复后校验上下文一致性
func (rm *ResumeManager) ValidateConsistency(cp *Checkpoint) error {
	if cp.PlanSnapshot == nil {
		return fmt.Errorf("autonomy: 检查点 %s 缺少计划快照", cp.GoalID)
	}

	// 校验 lastCompletedStep 存在于计划中
	if cp.LastCompletedStep != "" {
		found := false
		for _, s := range cp.PlanSnapshot.Steps {
			if s.ID == cp.LastCompletedStep {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("autonomy: 检查点 %s 的 lastCompletedStep=%s 不在计划中，需要重规划",
				cp.GoalID, cp.LastCompletedStep)
		}
	}

	// 校验计划合法性
	if err := cp.PlanSnapshot.Validate(); err != nil {
		return fmt.Errorf("autonomy: 检查点 %s 计划校验失败: %w", cp.GoalID, err)
	}

	return nil
}
