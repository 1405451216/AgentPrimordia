// react_loop_core.go — ReAct 循环核心体
// 包含 runLoop 主循环逻辑，被 reactLoopEngine 和 ResumeFromCheckpoint 共享
package agent

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"agentprimordia/internal/llm"
	"agentprimordia/internal/tools"
)

// runLoop ReAct 循环核心体，被 reactLoopEngine 和 ResumeFromCheckpoint 共享
// 封装从 startTurn 开始的主循环逻辑，包括 LLM 调用、工具执行、checkpoint 保存等
func (a *ReActAgent) runLoop(ctx context.Context, history []Message, startTurn int, cfg loopConfig, totalLLMLatency time.Duration, totalToolLatency time.Duration, toolCount int) (*Response, error) {
	// 优化（Task 2）：从 capCache 一次性取所有能力引用；capCache 由 reactLoopEngine 在 Run 入口处填充
	tracer := Tracer(nil)
	costTracker := (*CostTracker)(nil)
	if a.capCache != nil {
		tracer = a.capCache.tracer
		costTracker = a.capCache.costTracker
	}
	for turn := startTurn; turn < a.config.MaxTurns; turn++ {
		turnStart := time.Now()

		if a.lifecycle.IsStopped() {
			_ = a.lifecycle.SetStatus(StatusCancelled)
			a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: ErrAgentStopped.Error()})
			return &Response{RequestID: cfg.requestID, Error: ErrAgentStopped}, ErrAgentStopped
		}

		if ctx.Err() != nil {
			_ = a.lifecycle.SetStatus(StatusCancelled)
			a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: ctx.Err().Error()})
			return &Response{RequestID: cfg.requestID, Error: ctx.Err()}, ctx.Err()
		}

		a.statsMu.Lock()
		a.stats.CurrentTurn = turn + 1
		a.stats.TotalMessages = len(history)
		a.statsMu.Unlock()

		_ = a.fireHookWithPool(HookBeforeTurn, turn)
		// 优化（Task 3）：仅在存在订阅者时构造 payload map，避免热点路径上的堆分配
		if a.hasEventSubscriber() {
			a.publishEvent("turn.start", map[string]int{"turn": turn})
		}

		var turnSpan Span = &NoopSpan{}
		if tracer != nil {
			turnSpan = tracer.Start(
				"turn."+strconv.Itoa(turn),
				SpanKindInternal,
				WithAttributes(map[string]any{"agent": a.config.Name, "turn": turn}),
			)
		}

		if costTracker != nil && costTracker.CheckBudget() {
			a.logger.Warn("Agent 超出预算", "name", a.config.Name)
			_ = a.lifecycle.SetStatus(StatusFailed)
			return &Response{RequestID: cfg.requestID, Error: ErrBudgetExceeded}, ErrBudgetExceeded
		}

		var ragQuery string
		if turn == startTurn && len(history) > 0 {
			for i := len(history) - 1; i >= 0; i-- {
				if history[i].Role == RoleUser {
					ragQuery = history[i].Content
					break
				}
			}
		} else if turn < len(history) {
			ragQuery = history[turn].Content
		}

		if a.shouldRAG(turn) && ragQuery != "" {
			ragContext, ragDocs := a.searchRAG(ctx, ragQuery)
			if ragContext != "" {
				history = a.injectRAGContext(history, ragContext)
				a.logger.Debug("RAG 上下文已注入", "turn", turn, "query_len", len(ragQuery), "docs", len(ragDocs))
				a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: "[RAG] 知识库上下文已注入"})
			}
		}

		trimmedHistory := a.trimContext(history, 0)
		llmMessages := convertToLLMMessages(trimmedHistory)

		// 优化（Task 2 / Task 2.5 / perf-v2）：使用 capCache.toolkit 和预转换的 toolDefinitions
		var toolDefinitions []llm.ToolDefinition
		if a.capCache != nil && a.capCache.toolDefinitions != nil {
			toolDefinitions = a.capCache.toolDefinitions
		} else {
			var toolDefs []map[string]any
			var toolkit *tools.Registry
			if a.capCache != nil {
				toolkit = a.capCache.toolkit
			} else {
				toolkit = a.getToolkit()
			}
			if toolkit != nil {
				toolDefs = toolkit.Definitions()
			}
			toolDefinitions = convertToolDefsToLLMDefinitions(toolDefs)
		}

		llmStart := time.Now()
		if a.hasEventSubscriber() {
			a.publishEvent("llm.call", map[string]int{"turn": turn})
		}

		var llmSpan Span = &NoopSpan{}
		if tracer != nil {
			llmSpan = tracer.Start(
				"llm.call",
				SpanKindClient,
				WithParent(turnSpan.SpanContext()),
				WithAttributes(map[string]any{"agent": a.config.Name, "turn": turn}),
			)
		}

		var thought Thought

		if cfg.stream {
			thought = a.streamReasoning(ctx, cfg, llmMessages, toolDefinitions, llmStart)
			if thought.Content == "" && len(thought.ToolCalls) == 0 {
				return &Response{RequestID: cfg.requestID, Error: fmt.Errorf("stream reasoning failed")}, fmt.Errorf("stream reasoning failed")
			}
		} else {
			var err error
			thought, err = a.syncReasoning(ctx, llmMessages, toolDefinitions, llmStart)
			if err != nil {
				a.handleOnError(ctx, err)
				_ = a.lifecycle.SetStatus(StatusFailed)
				return &Response{RequestID: cfg.requestID, Error: err}, err
			}
		}

		llmLatency := time.Since(llmStart)
		totalLLMLatency += llmLatency

		llmSpan.SetAttribute("latency_ms", llmLatency.Milliseconds())
		llmSpan.End()

		a.recordUsage(thought.Usage)

		assistantMsg := Message{
			Role:      RoleAssistant,
			Content:   thought.Content,
			ToolCalls: thought.ToolCalls,
		}
		a.saveMemory(ctx, assistantMsg)

		_ = a.fireHookWithPool(HookAfterLLM, turn)
		if a.hasEventSubscriber() {
			a.publishEvent("llm.response", map[string]int{"turn": turn})
		}

		// 无工具调用 → Agent 完成
		if len(thought.ToolCalls) == 0 {
			duration := time.Since(a.startTime)
			response := &Response{
				RequestID: cfg.requestID,
				Content:   thought.Content,
				Metrics: Metrics{
					TotalTurns:  turn + 1,
					TotalTools:  toolCount,
					Duration:    duration,
					LLMLatency:  totalLLMLatency,
					ToolLatency: totalToolLatency,
				},
			}
			_ = a.lifecycle.SetStatus(StatusCompleted)
			a.saveCheckpoint(ctx, history, turn+1, response.Metrics)
			_ = a.fireHookWithPoolResp(HookOnComplete, response)
			turnSpan.End()
			_ = a.fireHookWithPool(HookAfterTurn, turn)
			a.recordTurn(time.Since(turnStart))
			if a.hasEventSubscriber() {
				a.publishEvent("turn.end", map[string]int{"turn": turn})
			}
			a.emitStream(cfg, StreamEvent{Type: StreamEventComplete, Content: thought.Content, Data: response})
			if cfg.stream {
				a.logger.Info("Agent 流式完成", "name", a.config.Name, "turns", turn+1, "duration", duration)
			} else {
				a.logger.Info("Agent 完成", "name", a.config.Name, "turns", turn+1, "duration", duration)
			}
			return response, nil
		}

		history = append(history, assistantMsg)

		// 执行所有工具调用
		history, totalToolLatency, toolCount = a.executeToolCalls(ctx, history, thought.ToolCalls, turn, cfg, tracer, turnSpan, totalToolLatency, toolCount)

		turnSpan.End()
		_ = a.fireHookWithPool(HookAfterTurn, turn)
		a.recordTurn(time.Since(turnStart))
		if a.hasEventSubscriber() {
			a.publishEvent("turn.end", map[string]int{"turn": turn})
		}

		if a.lifecycle.IsGracefulShutdown() {
			a.logger.Info("Agent 优雅关闭：当前 turn 已完成，退出循环", "name", a.config.Name, "turn", turn+1)
			_ = a.lifecycle.SetStatusWithReason(StatusCancelled, "graceful shutdown")
			duration := time.Since(a.startTime)
			response := &Response{
				RequestID: cfg.requestID,
				Content:   thought.Content,
				Error:     ErrAgentStopped,
				Metrics: Metrics{
					TotalTurns:  turn + 1,
					TotalTools:  toolCount,
					Duration:    duration,
					LLMLatency:  totalLLMLatency,
					ToolLatency: totalToolLatency,
				},
			}
			a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: "graceful shutdown: agent stopped after turn completion"})
			return response, ErrAgentStopped
		}

		a.saveCheckpoint(ctx, history, turn+1, Metrics{
			TotalTurns:  turn + 1,
			TotalTools:  toolCount,
			Duration:    time.Since(a.startTime),
			LLMLatency:  totalLLMLatency,
			ToolLatency: totalToolLatency,
		})
	}

	_ = a.lifecycle.SetStatus(StatusFailed)
	a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: ErrMaxTurnsExceeded.Error()})
	_ = a.fireHookWithPoolErr(HookOnError, ErrMaxTurnsExceeded)
	a.logger.Warn("Agent 超出最大轮次", "name", a.config.Name, "max_turns", a.config.MaxTurns)

	response := &Response{
		RequestID: cfg.requestID,
		Error:     ErrMaxTurnsExceeded,
		Metrics: Metrics{
			TotalTurns:  a.config.MaxTurns,
			TotalTools:  toolCount,
			Duration:    time.Since(a.startTime),
			LLMLatency:  totalLLMLatency,
			ToolLatency: totalToolLatency,
		},
	}

	return response, ErrMaxTurnsExceeded
}
