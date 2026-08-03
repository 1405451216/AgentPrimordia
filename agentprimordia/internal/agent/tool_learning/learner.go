// Package tool_learning 提供tool使用经验学习能力
package tool_learning

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// MemoryStore 定义tool学习所需的最小记忆存储接口
type MemoryStore interface {
	Add(ctx context.Context, episode *Episode) error
	// Query 按 sessionID + metadata 过滤查询 episodes
	// metadata 为 nil 或空时忽略 metadata 过滤；sessionID 为空时不过滤 session
	// 返回的 episode 顺序由实现决定（建议按时间倒序）
	Query(ctx context.Context, sessionID string, metadata map[string]string) ([]*Episode, error)
}

// Episode 是tool学习使用的记忆条目
type Episode struct {
	ID        string            `json:"id"`
	SessionID string            `json:"session_id"`
	Role      string            `json:"role"`
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt string            `json:"created_at"`
}

// ToolLearner 定义tool学习能力接口
type ToolLearner interface {
	// RecordSuccess 记录tool成功使用经验
	RecordSuccess(ctx context.Context, toolName string, args, result string) error
	// RecordFailure 记录tool失败经验
	RecordFailure(ctx context.Context, toolName string, args, errorMsg string) error
	// GetBestPractices 获取tool最佳实践
	GetBestPractices(ctx context.Context, toolName string) ([]BestPractice, error)
	// SuggestImprovement 基于历史经验建议改进
	SuggestImprovement(ctx context.Context, toolName string, args string) (*Suggestion, error)
	// SuggestProcessCorrection 基于历史失败模式建议流程修正（v3.6-2）：
	// 当本次调用命中高频失败模式时返回 Avoid=true，使引擎自动规避已知失败调用。
	SuggestProcessCorrection(ctx context.Context, toolName, args string) (*ProcessCorrection, error)
}

// FailurePattern 高频失败模式（流程修正的基础）。
type FailurePattern struct {
	ToolName   string    `json:"tool_name"`
	ArgsMarker string    `json:"args_marker,omitempty"` // 命中失败记录的参数（规范化）
	Error      string    `json:"error"`
	Frequency  int       `json:"frequency"` // 该参数组合失败次数
	LastSeen   time.Time `json:"last_seen"`
}

// ProcessCorrection 流程修正建议（v3.6-2）。
// 相比 SuggestImprovement（参数建议），它直接建议"规避已知失败调用"，
// 或在可换参数时给出规避失败的替代参数——即从流程层面修正。
type ProcessCorrection struct {
	ToolName        string  `json:"tool_name"`
	Avoid           bool    `json:"avoid"`                      // 是否应规避本次调用
	Reason          string  `json:"reason"`                     // 规避理由
	Confidence      float64 `json:"confidence"`                 // 规避置信度
	AlternativeArgs string  `json:"alternative_args,omitempty"` // 可规避失败的替代参数
	ErrorPattern    string  `json:"error_pattern,omitempty"`    // 命中的失败错误（归一化）
	Frequency       int     `json:"frequency,omitempty"`        // 该失败模式出现次数
}

// minFailureRepeat 失败模式判定：同一参数组合失败达到该次数即视为高频失败模式。
const minFailureRepeat = 2

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

// ToolUsageRecord tool使用记录
type ToolUsageRecord struct {
	ToolName  string    `json:"tool_name"`
	Args      string    `json:"args"`
	Result    string    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
	Success   bool      `json:"success"`
	Timestamp time.Time `json:"timestamp"`
}

// MemoryToolLearner 基于 MemoryStore 的tool学习器
type MemoryToolLearner struct {
	memory MemoryStore
}

// NewMemoryToolLearner 创建 MemoryToolLearner 实例
func NewMemoryToolLearner(mem MemoryStore) *MemoryToolLearner {
	return &MemoryToolLearner{memory: mem}
}

// RecordSuccess 记录tool成功使用
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

// RecordFailure 记录tool失败使用
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

// GetBestPractices 获取tool最佳实践
// 从记忆存储中查询指定tool的使用记录，按成功率聚合为 BestPractice。
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

// normalizeArgs 规范化参数用于失败模式匹配。
// JSON 参数中空白无意义，直接去除全部空白得到规范形。
func normalizeArgs(args string) string {
	var b strings.Builder
	b.Grow(len(args))
	for _, r := range args {
		switch r {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SuggestProcessCorrection 基于历史失败模式建议流程修正（v3.6-2）。
//
// 判定逻辑：
//   - 查询该 tool 的历史失败记录；
//   - 若当前参数（规范化后）在失败记录中出现 ≥ minFailureRepeat 次，
//     判定为高频失败模式 → Avoid=true；
//   - 提供替代参数：若存在该 tool 的成功记录且参数不同，则给出；
//   - 未命中时返回 Avoid=false，不阻断执行。
func (l *MemoryToolLearner) SuggestProcessCorrection(ctx context.Context, toolName, args string) (*ProcessCorrection, error) {
	if l.memory == nil {
		return &ProcessCorrection{ToolName: toolName}, nil
	}
	episodes, err := l.memory.Query(ctx, "tool_learning", map[string]string{
		"tool_name": toolName,
	})
	if err != nil {
		return nil, fmt.Errorf("query tool usage history: %w", err)
	}

	current := normalizeArgs(args)
	var failCount int
	var failError string
	var lastSeen time.Time
	successArgs := ""

	for _, ep := range episodes {
		if ep == nil {
			continue
		}
		var record ToolUsageRecord
		if err := json.Unmarshal([]byte(ep.Content), &record); err != nil {
			continue
		}
		if record.ToolName != toolName {
			continue
		}
		if record.Success {
			if successArgs == "" {
				successArgs = record.Args
			}
			continue
		}
		if current != "" && normalizeArgs(record.Args) == current {
			failCount++
			failError = record.Error
			if t, perr := time.Parse(time.RFC3339, ep.CreatedAt); perr == nil && t.After(lastSeen) {
				lastSeen = t
			}
		}
	}

	if failCount < minFailureRepeat {
		return &ProcessCorrection{ToolName: toolName}, nil
	}

	correction := &ProcessCorrection{
		ToolName:     toolName,
		Avoid:        true,
		Reason:       fmt.Sprintf("参数组合已失败 %d 次（%s），应规避", failCount, truncate(failError, 80)),
		Confidence:   minFloat(0.9, 0.5+0.1*float64(failCount)),
		ErrorPattern: failError,
		Frequency:    failCount,
	}
	if successArgs != "" && normalizeArgs(successArgs) != current {
		correction.AlternativeArgs = successArgs
	}
	return correction, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
