// react_persist.go — 记忆保存 + 检查点 + 上下文裁剪
// 负责 Agent 运行过程中的持久化操作：记忆存储、检查点保存、上下文窗口裁剪
package agent

import (
	"context"
	"time"

	"agentprimordia/internal/memory"
	"agentprimordia/internal/persist"
)

// saveMemory 将消息保存到 Memory
func (a *ReActAgent) saveMemory(ctx context.Context, msg Message) {
	mem := a.getMemoryStore()
	if mem == nil {
		return
	}
	ep := &memory.Episode{
		ID:        nextMemoryID(),
		SessionID: a.config.SessionID,
		Role:      string(msg.Role),
		Content:   msg.Content,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if ep.SessionID == "" {
		ep.SessionID = a.config.Name
	}
	if err := mem.Add(ctx, ep); err != nil {
		a.logger.Warn("保存记忆失败", "error", err, "role", msg.Role)
	}

	// 异步提取摘要（绑定到 agent 的 hookCtx 防止泄漏）
	summarizer := a.getSummarizer()
	if summarizer != nil && ep.ID != "" {
		epID := ep.ID
		epContent := ep.Content
		parentCtx := a.hookCtx
		if parentCtx == nil {
			parentCtx = context.Background()
		}
		go func() {
			defer func() {
				if r := recover(); r != nil {
					a.logger.Warn("异步摘要提取 panic", "error", r)
				}
			}()
			// 使用父级 context + 超时，防止泄漏
			sumCtx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
			defer cancel()
			result, err := summarizer.ExtractSummary(sumCtx, epContent)
			if err != nil {
				a.logger.Warn("异步摘要提取失败", "id", epID, "error", err)
				return
			}
			a.logger.Info("异步摘要提取成功", "id", epID, "summary_len", len(result.Summary), "topics", result.Topics)
			// M2 修复：存储摘要结果，不再只记录日志丢弃
			if err := mem.UpdateSummary(sumCtx, epID, result.Summary, result.Topics); err != nil {
				a.logger.Warn("异步摘要存储失败", "id", epID, "error", err)
			}
		}()
	}
}

// saveCheckpoint 保存 Agent 状态
func (a *ReActAgent) saveCheckpoint(ctx context.Context, history []Message, turnCount int, m Metrics) {
	cs := a.getCheckpointStore()
	if cs == nil {
		return
	}

	// 转换消息格式
	msgs := make([]persist.CheckpointMessage, len(history))
	for i, m := range history {
		msgs[i] = persist.CheckpointMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}

	state := &persist.AgentState{
		AgentID:   a.config.Name,
		SessionID: a.config.SessionID,
		Status:    string(a.lifecycle.Status()),
		Messages:  msgs,
		TurnCount: turnCount,
		Metrics: persist.CheckpointMetrics{
			TotalTurns:  m.TotalTurns,
			TotalTools:  m.TotalTools,
			Duration:    m.Duration.String(),
			LLMLatency:  m.LLMLatency.String(),
			ToolLatency: m.ToolLatency.String(),
		},
		SavedAt: time.Now().UTC(),
	}

	if err := cs.Save(ctx, state); err != nil {
		a.logger.Warn("保存检查点失败", "error", err)
	}
}

// trimContext 应用上下文窗口策略裁剪历史
func (a *ReActAgent) trimContext(history []Message, maxMessages int) []Message {
	if cw := a.getContextWindowStrategy(); cw != nil {
		return cw.Trim(history, maxMessages)
	}
	return history
}
