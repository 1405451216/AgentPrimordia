// react_lifecycle.go — Agent 生命周期方法
// 包含 Agent 的启停、暂停/恢复、检查点恢复、统计信息等生命周期管理
package agent

import (
	"context"
	"fmt"
	"maps"
	"time"

	"agentprimordia/internal/llm"
)

// Stats returns current agent statistics
func (a *ReActAgent) Stats() AgentStats {
	a.statsMu.RLock()
	stats := a.stats
	a.statsMu.RUnlock()

	// 在锁外设置 status，避免嵌套锁定（lifecycle.Status 有自己的锁）
	stats.Status = a.lifecycle.Status()

	// Deep copy the ToolsCalled map to prevent caller from mutating internal state
	toolsCopy := make(map[string]int, len(stats.ToolsCalled))
	maps.Copy(toolsCopy, stats.ToolsCalled)
	stats.ToolsCalled = toolsCopy
	return stats
}

// recordUsage 记录 LLM Usage 到 CostTracker 和 Metrics
func (a *ReActAgent) recordUsage(usage llm.Usage) {
	if usage.TotalTokens == 0 {
		return
	}

	if ct := a.getCostTracker(); ct != nil {
		modelName := ""
		if info := a.config.Model.Info(); info.Name != "" {
			modelName = info.Name
		}
		_ = ct.Record(modelName, a.config.SessionID, a.config.Name, usage)
	}

	if m := a.getMetricsRecorder(); m != nil {
		modelName := ""
		if info := a.config.Model.Info(); info.Name != "" {
			modelName = info.Name
		}
		m.RecordTokenUsage(modelName, usage.PromptTokens, usage.CompletionTokens)
	}
}

// Stop 优雅停止 Agent，发送停止信号
func (a *ReActAgent) Stop() {
	a.lifecycle.Stop()
	a.logger.Info("Agent 收到停止信号", "name", a.config.Name)
}

// GracefulShutdown 优雅关闭 Agent
// 请求在当前 turn 完成后停止，而不是立即中断
// 如果 ctx 超时仍未完成，则回退到强制停止
func (a *ReActAgent) GracefulShutdown(ctx context.Context) error {
	a.logger.Info("Agent 请求优雅关闭", "name", a.config.Name)
	return a.lifecycle.GracefulShutdown(ctx)
}

// Name 返回 Agent 名称
func (a *ReActAgent) Name() string {
	return a.config.Name
}

// Pause 暂停 Agent
func (a *ReActAgent) Pause() {
	a.lifecycle.Pause()
	a.logger.Info("Agent 已暂停", "name", a.config.Name)
}

// Resume 恢复暂停的 Agent
func (a *ReActAgent) Resume() {
	a.lifecycle.Resume()
	a.logger.Info("Agent 已恢复", "name", a.config.Name)
}

// ResumeFromCheckpoint 从检查点恢复 Agent 执行
// 加载 CheckpointStore 中的状态，从上次中断的位置继续运行
func (a *ReActAgent) ResumeFromCheckpoint(ctx context.Context) (*Response, error) {
	a.runMu.Lock()
	defer a.runMu.Unlock()

	cs := a.getCheckpointStore()
	if cs == nil {
		return nil, fmt.Errorf("checkpoint store not configured")
	}

	state, err := cs.Load(ctx, a.config.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	if state.Status != "paused" && state.Status != "failed" && state.Status != "cancelled" {
		return nil, fmt.Errorf("cannot resume from status %q, expected paused/failed/cancelled", state.Status)
	}

	a.logger.Info("Agent 从检查点恢复", "name", a.config.Name, "turn", state.TurnCount, "saved_at", state.SavedAt)

	history := make([]Message, 0, len(state.Messages))
	for _, m := range state.Messages {
		history = append(history, Message{
			Role:    Role(m.Role),
			Content: m.Content,
		})
	}

	// 恢复 startTime 时减去已运行的时长，保持 Duration 累计一致
	if prevDur, err := time.ParseDuration(state.Metrics.Duration); err == nil && prevDur > 0 {
		a.startTime = time.Now().Add(-prevDur)
	} else {
		a.startTime = time.Now()
	}
	// 注入请求 ID
	reqID := RequestIDFromCtx(ctx)
	if reqID == "" {
		reqID = NewRequestID()
		ctx = WithRequestID(ctx, reqID)
	}
	a.mu.Lock()
	a.currentRequestID = reqID
	a.mu.Unlock()

	a.statsMu.Lock()
	a.stats.StartTime = a.startTime
	a.stats.CurrentTurn = state.TurnCount
	a.stats.TotalMessages = len(history)
	a.stats.Status = StatusRunning
	a.stats.RequestID = reqID
	a.statsMu.Unlock()
	_ = a.lifecycle.SetStatus(StatusRunning)

	if m := a.getMetricsRecorder(); m != nil {
		m.IncActiveAgents()
		defer m.DecActiveAgents()
	}

	defer func() {
		if a.lifecycle.Status() != StatusCompleted &&
			a.lifecycle.Status() != StatusFailed &&
			a.lifecycle.Status() != StatusCancelled {
			_ = a.lifecycle.SetStatus(StatusCompleted)
		}
	}()

	_ = a.fireHook(HookBeforeRun, &HookContext{})
	a.publishEvent(EventAgentResume, map[string]string{"name": a.config.Name, "from_turn": fmt.Sprintf("%d", state.TurnCount)})

	prevMetrics := state.Metrics
	var totalLLMLatency time.Duration
	var totalToolLatency time.Duration
	if d, parseErr := time.ParseDuration(prevMetrics.LLMLatency); parseErr == nil {
		totalLLMLatency = d
	}
	if d, parseErr := time.ParseDuration(prevMetrics.ToolLatency); parseErr == nil {
		totalToolLatency = d
	}
	toolCount := prevMetrics.TotalTools

	return a.runLoop(ctx, history, state.TurnCount, loopConfig{requestID: reqID}, totalLLMLatency, totalToolLatency, toolCount)
}
