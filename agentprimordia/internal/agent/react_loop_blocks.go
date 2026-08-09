// react_loop_blocks.go — runLoop 按块拆分的命名函数（v4.1 真实接线：runLoop 拆分）
//
// 从 react_loop_core.go 的 runLoop 主体按块抽出：
//   - checkBudgetExceeded        成本检查
//   - injectMemoryContextAndFastPath 记忆注入（长期记忆回读 + 已解任务 fast-path）
//   - ragRetrieveAndInject       RAG 检索与注入
//   - guardrailSanitizeOutput    输出端护栏（脱敏/拦截）
//
// 纯行为保持重构：既有测试为门，任何一处语义偏差都会在
// go test ./internal/agent/... 中暴露。
package agent

import (
	"context"
	"time"
)

// checkBudgetExceeded 成本检查：超预算时返回中止响应与 ErrBudgetExceeded；
// 未超预算返回 (nil, nil)。
func (a *ReActAgent) checkBudgetExceeded(cfg loopConfig, costTracker *CostTracker) (*Response, error) {
	if costTracker == nil || !costTracker.CheckBudget() {
		return nil, nil
	}
	a.logger.Warn("Agent 超出预算", "name", a.config.Name)
	_ = a.lifecycle.SetStatus(StatusFailed)
	return &Response{RequestID: cfg.requestID, Error: ErrBudgetExceeded}, ErrBudgetExceeded
}

// injectMemoryContextAndFastPath 首轮记忆注入：
//  1. 长期记忆回读（跨 session 记忆召回注入 system）；
//  2. 跨任务已解记忆 fast-path——命中时直接复用答案（0 轮 LLM 推理）。
//
// 返回注入后的 history、fast-path 命中时的提前响应、以及是否提前返回。
func (a *ReActAgent) injectMemoryContextAndFastPath(ctx context.Context, history []Message, turn, startTurn int, cfg loopConfig) ([]Message, *Response, bool) {
	if turn != startTurn {
		return history, nil, false
	}

	if memCtx := a.searchMemoryContext(ctx, history); memCtx != "" {
		history = injectMemoryContext(history, memCtx)
		a.logger.Debug("长期记忆已注入", "turn", turn)
		a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: "[Memory] 长期记忆上下文已注入"})
	}

	if answer, ok := a.tryMemorySolution(ctx, history); ok {
		a.logger.Info("跨任务记忆命中，直接复用已解答案", "name", a.config.Name)
		a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: "[Memory] 命中已解任务记忆，跳过推理"})
		a.statsMu.Lock()
		a.stats.MemoryHits++
		a.statsMu.Unlock()
		return history, &Response{
			RequestID: cfg.requestID,
			Content:   answer,
			Metrics: Metrics{
				TotalTurns: 0, // fast-path：0 轮 LLM 推理
				Duration:   0,
				MemoryHit:  true,
			},
		}, true
	}
	return history, nil, false
}

// extractRAGQuery 提取本轮 RAG 查询文本（首轮取最后一条 user 消息，其余轮取对应历史）。
func extractRAGQuery(history []Message, turn, startTurn int) string {
	if turn == startTurn && len(history) > 0 {
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Role == RoleUser {
				return history[i].Content
			}
		}
	} else if turn < len(history) {
		return history[turn].Content
	}
	return ""
}

// ragRetrieveAndInject RAG 检索与注入：命中时把知识上下文注入 history，
// 返回更新后的 history（未命中时原样返回）。
func (a *ReActAgent) ragRetrieveAndInject(ctx context.Context, history []Message, turn, startTurn int, cfg loopConfig, tracer Tracer, turnSpan Span) []Message {
	ragQuery := extractRAGQuery(history, turn, startTurn)
	if !a.shouldRAG(turn) || ragQuery == "" {
		return history
	}

	var ragSpan Span = &NoopSpan{}
	if tracer != nil {
		ragSpan = tracer.Start(
			"rag.search",
			SpanKindInternal,
			WithParent(turnSpan.SpanContext()),
			WithAttributes(map[string]any{"agent": a.config.Name, "turn": turn}),
		)
	}
	ragContext, ragDocs := a.searchRAG(ctx, ragQuery)
	ragSpan.SetAttribute("docs_found", len(ragDocs))
	ragSpan.End()
	if ragContext != "" {
		history = a.injectRAGContext(history, ragContext)
		a.logger.Debug("RAG 上下文已注入", "turn", turn, "query_len", len(ragQuery), "docs", len(ragDocs))
		a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: "[RAG] 知识库上下文已注入"})
	}
	return history
}

// guardrailSanitizeOutput 输出端护栏（PII 脱敏、注入拦截）：
//   - 命中拦截 → 写入 GuardrailBlock 审计并返回 ErrOutputBlocked 响应；
//   - 命中脱敏 → 就地修改 thought.Content 并写入 GuardrailSanitize 审计；
//   - 检查失败/未配置/空内容 → 原样放行。
//
// 正常放行返回 (nil, nil)。
func (a *ReActAgent) guardrailSanitizeOutput(ctx context.Context, cfg loopConfig, thought *Thought, turn int) (*Response, error) {
	if a.capCache == nil || a.capCache.outputGuard == nil || thought.Content == "" {
		return nil, nil
	}

	sanitized, blocked, gerr := a.capCache.outputGuard(thought.Content)
	if gerr != nil {
		a.logger.Warn("Guardrail output 检查失败", "err", gerr)
		return nil, nil
	}
	if blocked {
		a.writeAudit(ctx, AuditEvent{
			Actor:    a.config.Name,
			Action:   auditActionGuardrailBlock,
			Resource: cfg.requestID,
			Result:   auditResultBlocked,
			Details:  map[string]any{"turn": turn, "rule": "output_guard"},
		})
		return &Response{RequestID: cfg.requestID, Error: ErrOutputBlocked}, ErrOutputBlocked
	}
	if sanitized != "" {
		thought.Content = sanitized
		a.logger.Debug("Guardrail 已脱敏输出")
		a.writeAudit(ctx, AuditEvent{
			Actor:    a.config.Name,
			Action:   auditActionGuardrailSanitize,
			Resource: cfg.requestID,
			Result:   auditResultSuccess,
			Details:  map[string]any{"turn": turn, "rule": "output_guard"},
		})
	}
	return nil, nil
}

// 确保 time 被引用（与 runLoop 计时语义一致）。
var _ = time.Now
