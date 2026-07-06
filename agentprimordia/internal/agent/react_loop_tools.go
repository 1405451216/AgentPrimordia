// react_loop_tools.go — 工具执行与 HITL 处理
// 包含 executeToolCalls 和 handleHITL 函数
package agent

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// executeToolCalls 执行一组工具调用，处理 HITL 确认、追踪、Hook 等。
// 返回更新后的 history、累计工具延迟和工具调用计数。
func (a *ReActAgent) executeToolCalls(ctx context.Context, history []Message, toolCalls []ToolCall, turn int, cfg loopConfig, tracer Tracer, turnSpan Span, totalToolLatency time.Duration, toolCount int) ([]Message, time.Duration, int) {
	for _, tc := range toolCalls {
		a.emitStream(cfg, StreamEvent{Type: StreamEventToolCall, Content: tc.Name, Data: tc})
		_ = a.fireHook(HookBeforeTool, &HookContext{ToolCall: &tc, Turn: turn})
		if a.hasEventSubscriber() {
			a.publishEvent(EventToolCall, map[string]string{"tool": tc.Name, "turn": strconv.Itoa(turn)})
		}

		// HITL 处理
		tc, skip := a.handleHITL(ctx, &tc, turn, cfg)
		if skip {
			result := &ToolResult{ToolCallID: tc.ID, Content: "人类拒绝执行此操作", IsError: true}
			history = append(history, result.ToMessage())
			a.saveMemory(ctx, result.ToMessage())
			continue
		}

		toolStart := time.Now()

		var toolSpan Span = &NoopSpan{}
		if tracer != nil {
			toolSpan = tracer.Start(
				"tool."+tc.Name,
				SpanKindClient,
				WithParent(turnSpan.SpanContext()),
				WithAttributes(map[string]any{"tool": tc.Name, "agent": a.config.Name}),
			)
		}

		result, err := a.executeTool(ctx, tc)
		toolLatency := time.Since(toolStart)
		totalToolLatency += toolLatency

		toolSpan.SetAttribute("latency_ms", toolLatency.Milliseconds())
		if err != nil {
			toolSpan.SetStatus(SpanStatusError, err.Error())
		}
		toolSpan.End()
		a.recordTool(toolLatency, err, tc.Name)

		if err != nil {
			a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: fmt.Sprintf("tool %s error: %v", tc.Name, err)})
			_ = a.fireHook(HookOnError, &HookContext{Error: err, Turn: turn})
			if a.hasEventSubscriber() {
				a.publishEvent(EventAgentError, map[string]string{"tool": tc.Name, "error": err.Error()})
			}
		} else {
			a.emitStream(cfg, StreamEvent{Type: StreamEventToolResult, Content: result.Content, Data: result})
		}

		if cfg.stream {
			if err == nil {
				_ = a.fireHook(HookAfterTool, &HookContext{ToolResult: result, Turn: turn})
			}
		} else {
			_ = a.fireHook(HookAfterTool, &HookContext{ToolResult: result, Turn: turn})
			if a.hasEventSubscriber() {
				a.publishEvent(EventToolResult, map[string]string{"tool": tc.Name})
			}
		}

		toolCount++
		a.statsMu.Lock()
		a.stats.ToolsCalled[tc.Name]++
		a.statsMu.Unlock()

		history = append(history, result.ToMessage())
		a.saveMemory(ctx, result.ToMessage())
	}
	return history, totalToolLatency, toolCount
}

// handleHITL 处理 Human-in-the-Loop 确认流程。
// 返回可能被修改的 ToolCall 和是否跳过执行的标志。
func (a *ReActAgent) handleHITL(ctx context.Context, tc *ToolCall, turn int, cfg loopConfig) (_ ToolCall, skip bool) {
	if a.hitlMgr == nil || !a.hitlMgr.ShouldInterrupt(tc.Name, InterruptToolConfirm) {
		return *tc, false
	}

	a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: fmt.Sprintf("[HITL] 工具 %s 需要人类确认", tc.Name)})
	_ = a.lifecycle.WaitForInput(fmt.Sprintf("工具 %s 需确认", tc.Name))

	humanResp, hitlErr := a.hitlMgr.RequestInterrupt(ctx, &InterruptRequest{
		Reason:  InterruptToolConfirm,
		Message: fmt.Sprintf("Agent 请求执行工具 %s，参数: %s", tc.Name, tc.Args),
		Data:    map[string]any{"tool": tc.Name, "args": tc.Args},
		Turn:    turn,
	})

	_ = a.lifecycle.SetStatus(StatusRunning)

	if hitlErr != nil {
		a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: fmt.Sprintf("[HITL] 等待人类确认失败: %v", hitlErr)})
		*tc = ToolCall{ID: tc.ID, Name: tc.Name, Args: tc.Args}
		return *tc, true
	}

	if !humanResp.Approved {
		a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: fmt.Sprintf("[HITL] 人类拒绝执行工具 %s", tc.Name)})
		return *tc, true
	}

	if humanResp.Modified != nil {
		if modifiedArgs, ok := humanResp.Modified["args"].(string); ok {
			tc.Args = modifiedArgs
		}
	}

	a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: fmt.Sprintf("[HITL] 人类已确认执行工具 %s", tc.Name)})
	return *tc, false
}
