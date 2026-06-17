// react_persist.go — 记忆保存 + 检查点 + 上下文裁剪
// 负责 Agent 运行过程中的持久化操作：记忆存储、检查点保存、上下文窗口裁剪
package agent

import (
	"context"
	"time"

	"agentprimordia/internal/memory"
	"agentprimordia/internal/persist"
)

// saveMemoryChBuffer 是异步 saveMemory 队列容量。
// 高吞吐场景下消息数远大于此值时会触发丢弃，但持久化路径不应阻塞主循环。
const saveMemoryChBuffer = 256

// saveMemory 将消息保存到 Memory。
// 优化（Task 1）：将 mem.Add() 写入异步化到独立的 goroutine 中，
// 避免 SQLite 同步写入阻塞 ReAct 主循环。
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

	// 非阻塞提交到当前 Run 的异步写入 channel。
	// 优化（Task 1）：channel 满时丢弃并记录告警，避免阻塞主循环。
	// 注意：不要在 a 上保存 WaitGroup，因为 flushMemoryWriter 与 saveMemory 的并发
	// 访问同一字段会引发 TOCTOU 竞态。WaitGroup 由 consume goroutine 内部维护，
	// flushMemoryWriter 通过 doneCh 等待。
	submitCh := a.getMemoryWriterCh(mem)

	select {
	case submitCh <- ep:
	default:
		a.logger.Warn("异步记忆队列已满，丢弃写入", "role", msg.Role)
		return
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

// getMemoryWriterCh 返回当前 Run() 期间的异步写入 channel。
// 优化（Task 1）：每个 Run 都有自己的 channel 和 goroutine，关闭后随 Run 退出，
// 下次 Run 重新创建。这样可以安全支持同一个 agent 的多次连续 Run() 调用。
//
// 闭包内的 wg 和 doneCh 不与 agent 字段共享，避免与 flushMemoryWriter 的并发 race。
func (a *ReActAgent) getMemoryWriterCh(mem MemoryStore) chan *memory.Episode {
	a.memorySetupMu.Lock()
	defer a.memorySetupMu.Unlock()
	if a.memoryCh == nil {
		ch := make(chan *memory.Episode, saveMemoryChBuffer)
		a.memoryCh = ch
		// 启动 consume goroutine
		doneCh := make(chan struct{})
		a.memoryDoneCh = doneCh
		go func() {
			defer close(doneCh)
			for ep := range ch {
				// 使用独立的 context，避免 ctx 取消导致最后一次写入丢失
				writeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := mem.Add(writeCtx, ep); err != nil {
					a.logger.Warn("保存记忆失败", "error", err, "role", ep.Role)
				}
				cancel()
			}
		}()
	}
	return a.memoryCh
}

// flushMemoryWriter 关闭异步写入队列并等待所有待写入完成。
// 应在 agent 运行结束（reactLoopEngine 的 defer）时调用。
// 关闭后下次 saveMemory 会重新创建 channel 和 goroutine。
func (a *ReActAgent) flushMemoryWriter() {
	a.memorySetupMu.Lock()
	ch := a.memoryCh
	doneCh := a.memoryDoneCh
	// 清空字段：下次 saveMemory 触发重新创建
	a.memoryCh = nil
	a.memoryDoneCh = nil
	a.memorySetupMu.Unlock()

	if ch == nil {
		return
	}
	// 关闭 channel，使 consume goroutine 退出 range 循环
	close(ch)
	// 等待 consume goroutine 退出（确保所有 mem.Add() 完成）
	if doneCh != nil {
		<-doneCh
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
