package autonomy

import (
	"context"
	"time"
)

// MemoryType 记忆条目类型
type MemoryType string

const (
	// MemoryTypeContext 目标执行上下文
	MemoryTypeContext MemoryType = "context"
	// MemoryTypeLesson 失败教训/经验
	MemoryTypeLesson MemoryType = "lesson"
	// MemoryTypeFailure 失败记录
	MemoryTypeFailure MemoryType = "failure"
)

// MemoryEntry 记忆条目
type MemoryEntry struct {
	// GoalID 关联目标
	GoalID string `json:"goal_id"`
	// Type 条目类型
	Type MemoryType `json:"type"`
	// Content 内容
	Content string `json:"content"`
	// StepID 关联步骤（可选）
	StepID string `json:"step_id,omitempty"`
	// Error 错误信息（失败记录专用）
	Error string `json:"error,omitempty"`
	// Resolution 解决方案/教训
	Resolution string `json:"resolution,omitempty"`
	// Timestamp 时间戳
	Timestamp time.Time `json:"timestamp"`
}

// MemoryStore 记忆存储接口（由外部 memory/ 包实现）
type MemoryStore interface {
	// Save 保存记忆条目
	Save(ctx context.Context, entry MemoryEntry) error
	// Query 查询指定目标的记忆条目
	Query(ctx context.Context, goalID string) ([]MemoryEntry, error)
}

// GoalMemory 目标记忆闭环：跨会话记忆 + 失败经验反馈
type GoalMemory struct {
	store MemoryStore
}

// NewGoalMemory 创建目标记忆管理器
func NewGoalMemory(store MemoryStore) *GoalMemory {
	return &GoalMemory{store: store}
}

// SaveContext 保存目标执行上下文
func (gm *GoalMemory) SaveContext(ctx context.Context, goalID string, content string) error {
	return gm.store.Save(ctx, MemoryEntry{
		GoalID:    goalID,
		Type:      MemoryTypeContext,
		Content:   content,
		Timestamp: time.Now(),
	})
}

// SaveLesson 保存失败教训（由 reflection 提炼）
func (gm *GoalMemory) SaveLesson(ctx context.Context, goalID string, lesson string) error {
	return gm.store.Save(ctx, MemoryEntry{
		GoalID:    goalID,
		Type:      MemoryTypeLesson,
		Content:   lesson,
		Timestamp: time.Now(),
	})
}

// SaveFailure 保存失败尝试记录
func (gm *GoalMemory) SaveFailure(ctx context.Context, goalID string, stepID string, errMsg string, resolution string) error {
	return gm.store.Save(ctx, MemoryEntry{
		GoalID:     goalID,
		Type:       MemoryTypeFailure,
		Content:    "步骤 " + stepID + " 执行失败",
		StepID:     stepID,
		Error:      errMsg,
		Resolution: resolution,
		Timestamp:  time.Now(),
	})
}

// Query 查询目标的所有记忆
func (gm *GoalMemory) Query(ctx context.Context, goalID string) ([]MemoryEntry, error) {
	return gm.store.Query(ctx, goalID)
}

// QueryFailures 查询目标的失败记录
func (gm *GoalMemory) QueryFailures(ctx context.Context, goalID string) ([]MemoryEntry, error) {
	all, err := gm.store.Query(ctx, goalID)
	if err != nil {
		return nil, err
	}
	var failures []MemoryEntry
	for _, e := range all {
		if e.Type == MemoryTypeFailure {
			failures = append(failures, e)
		}
	}
	return failures, nil
}
