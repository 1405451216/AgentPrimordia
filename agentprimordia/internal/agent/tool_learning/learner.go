// Package tool_learning 提供工具使用经验学习能力
package tool_learning

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// MemoryStore 定义工具学习所需的最小记忆存储接口
type MemoryStore interface {
	Add(ctx context.Context, episode *Episode) error
	// Query 按 sessionID + metadata 过滤查询 episodes
	// metadata 为 nil 或空时忽略 metadata 过滤；sessionID 为空时不过滤 session
	// 返回的 episode 顺序由实现决定（建议按时间倒序）
	Query(ctx context.Context, sessionID string, metadata map[string]string) ([]*Episode, error)
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
		ID:        "tool_usage_" + toolName + "_" + strconv.FormatInt(time.Now().UnixNano(), 10),
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
		ID:        "tool_usage_" + toolName + "_" + strconv.FormatInt(time.Now().UnixNano(), 10),
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
// 从记忆存储中查询指定工具的使用记录，按成功率聚合为 BestPractice。
// 当无历史数据时返回空切片（非 nil）。
//
// 实现说明：
//   - 调用 l.memory.Query(ctx, "tool_learning", {"tool_name": toolName}) 获取相关记录
//   - 反序列化 Content 字段中的 JSON 记录
//   - 统计 success/total ratio，收集最近的成功示例（最多 5 个）
//   - 返回包含聚合信息的单条 BestPractice
func (l *MemoryToolLearner) GetBestPractices(ctx context.Context, toolName string) ([]BestPractice, error) {
	if l.memory == nil {
		return []BestPractice{}, nil
	}
	episodes, err := l.memory.Query(ctx, "tool_learning", map[string]string{
		"tool_name": toolName,
	})
	if err != nil {
		return nil, fmt.Errorf("query tool usage history: %w", err)
	}

	var successCount, totalCount int
	examples := make([]string, 0, 5)
	for _, ep := range episodes {
		if ep == nil {
			continue
		}
		var record ToolUsageRecord
		if err := json.Unmarshal([]byte(ep.Content), &record); err != nil {
			// 跳过损坏的记录，不影响整体聚合
			continue
		}
		totalCount++
		if record.Success {
			successCount++
			if len(examples) < 5 {
				examples = append(examples, record.Args)
			}
		}
	}

	if totalCount == 0 {
		return []BestPractice{}, nil
	}

	successRate := float64(successCount) / float64(totalCount)
	return []BestPractice{{
		ToolName:    toolName,
		Pattern:     "most_common_success_pattern",
		Description: fmt.Sprintf("成功率 %.1f%% (%d/%d)", successRate*100, successCount, totalCount),
		SuccessRate: successRate,
		Examples:    examples,
		CreatedAt:   time.Now(),
	}}, nil
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
		Reason:       "基于历史成功记录（成功率 " + strconv.FormatFloat(practices[0].SuccessRate, 'f', 2, 64) + "）",
		Confidence:   practices[0].SuccessRate,
	}, nil
}
