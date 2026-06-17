// Package tool_learning 提供工具使用经验学习能力
package tool_learning

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// MemoryStore 定义工具学习所需的最小记忆存储接口
type MemoryStore interface {
	Add(ctx context.Context, episode *Episode) error
}

// Episode 是工具学习使用的记忆条目
type Episode struct {
	ID        string            `json:"id"`
	SessionID string            `json:"session_id"`
	Role      string            `json:"role"`
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt string            `json:"created_at"`
}

// ToolLearner 定义工具学习能力接口
type ToolLearner interface {
	// RecordSuccess 记录工具成功使用经验
	RecordSuccess(ctx context.Context, toolName string, args, result string) error
	// RecordFailure 记录工具失败经验
	RecordFailure(ctx context.Context, toolName string, args, errorMsg string) error
	// GetBestPractices 获取工具最佳实践
	GetBestPractices(ctx context.Context, toolName string) ([]BestPractice, error)
	// SuggestImprovement 基于历史经验建议改进
	SuggestImprovement(ctx context.Context, toolName string, args string) (*Suggestion, error)
}

// BestPractice 最佳实践
type BestPractice struct {
	ToolName    string    `json:"tool_name"`
	Pattern     string    `json:"pattern"`
	Description string    `json:"description"`
	SuccessRate float64   `json:"success_rate"`
	Examples    []string  `json:"examples"`
	CreatedAt   time.Time `json:"created_at"`
}

// Suggestion 改进建议
type Suggestion struct {
	OriginalArgs string  `json:"original_args"`
	ImprovedArgs string  `json:"improved_args"`
	Reason       string  `json:"reason"`
	Confidence   float64 `json:"confidence"`
}

// ToolUsageRecord 工具使用记录
type ToolUsageRecord struct {
	ToolName  string    `json:"tool_name"`
	Args      string    `json:"args"`
	Result    string    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
	Success   bool      `json:"success"`
	Timestamp time.Time `json:"timestamp"`
}

// MemoryToolLearner 基于 MemoryStore 的工具学习器
type MemoryToolLearner struct {
	memory MemoryStore
}

// NewMemoryToolLearner 创建 MemoryToolLearner 实例
func NewMemoryToolLearner(mem MemoryStore) *MemoryToolLearner {
	return &MemoryToolLearner{memory: mem}
}

// RecordSuccess 记录工具成功使用
func (l *MemoryToolLearner) RecordSuccess(ctx context.Context, toolName string, args, result string) error {
	record := ToolUsageRecord{
		ToolName:  toolName,
		Args:      args,
		Result:    result,
		Success:   true,
		Timestamp: time.Now(),
	}

	recordJSON, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal record failed: %w", err)
	}

	episode := &Episode{
		ID:        fmt.Sprintf("tool_usage_%s_%d", toolName, time.Now().UnixNano()),
		SessionID: "tool_learning",
		Role:      "tool_usage",
		Content:   string(recordJSON),
		CreatedAt: time.Now().Format(time.RFC3339),
		Metadata: map[string]string{
			"tool_name": toolName,
			"success":   "true",
		},
	}

	return l.memory.Add(ctx, episode)
}

// RecordFailure 记录工具失败使用
func (l *MemoryToolLearner) RecordFailure(ctx context.Context, toolName string, args, errorMsg string) error {
	record := ToolUsageRecord{
		ToolName:  toolName,
		Args:      args,
		Error:     errorMsg,
		Success:   false,
		Timestamp: time.Now(),
	}

	recordJSON, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal record failed: %w", err)
	}

	episode := &Episode{
		ID:        fmt.Sprintf("tool_usage_%s_%d", toolName, time.Now().UnixNano()),
		SessionID: "tool_learning",
		Role:      "tool_usage",
		Content:   string(recordJSON),
		CreatedAt: time.Now().Format(time.RFC3339),
		Metadata: map[string]string{
			"tool_name": toolName,
			"success":   "false",
			"error":     errorMsg,
		},
	}

	return l.memory.Add(ctx, episode)
}

// GetBestPractices 获取工具最佳实践
func (l *MemoryToolLearner) GetBestPractices(ctx context.Context, toolName string) ([]BestPractice, error) {
	return []BestPractice{}, nil
}

// SuggestImprovement 基于历史经验建议改进
func (l *MemoryToolLearner) SuggestImprovement(ctx context.Context, toolName string, args string) (*Suggestion, error) {
	practices, err := l.GetBestPractices(ctx, toolName)
	if err != nil {
		return nil, err
	}

	if len(practices) == 0 || len(practices[0].Examples) == 0 {
		return &Suggestion{
			OriginalArgs: args,
			ImprovedArgs: args,
			Reason:       "没有足够的历史数据提供改进建议",
			Confidence:   0.0,
		}, nil
	}

	bestExample := practices[0].Examples[0]

	return &Suggestion{
		OriginalArgs: args,
		ImprovedArgs: bestExample,
		Reason:       fmt.Sprintf("基于历史成功记录（成功率 %.2f）", practices[0].SuccessRate),
		Confidence:   practices[0].SuccessRate,
	}, nil
}
