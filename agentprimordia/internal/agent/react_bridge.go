// react_bridge.go — ReActAgent 与 react.Engine 的桥接层（B-3 包拆分）
//
// 本文件实现 react.Delegate 接口，将 ReActAgent 的内部能力
// 暴露给 react.Engine 循环状态机。
//
// 依赖方向：agent/ → react/（正向，无循环）
package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"agentprimordia/internal/agent/core"
	"agentprimordia/internal/agent/react"
	"agentprimordia/internal/llm"
)

// reactDelegate 桥接 ReActAgent 到 react.Engine
type reactDelegate struct {
	agent  *ReActAgent
	cfg    loopConfig
	stream bool
}

// newReactDelegate 创建桥接实例
func newReactDelegate(a *ReActAgent, cfg loopConfig) *reactDelegate {
	return &reactDelegate{
		agent:  a,
		cfg:    cfg,
		stream: cfg.stream,
	}
}

// CallLLM 执行单轮 LLM 调用
// 将 ReActAgent 内部的 LLM 调用逻辑（含tool定义注入、RAG 上下文）封装为统一接口
func (d *reactDelegate) CallLLM(ctx context.Context, turn int, history []core.Message) (string, []core.ToolCall, error) {
	a := d.agent

	// 构建 LLM 请求消息（从 core.Message 转换为 llm.ChatMessage）
	messages := make([]llm.ChatMessage, len(history))
	for i, m := range history {
		messages[i] = llm.ChatMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}

	// 获取tool定义
	var toolDefs []llm.ToolDefinition
	if a.capCache != nil && len(a.capCache.toolDefinitions) > 0 {
		toolDefs = a.capCache.toolDefinitions
	}

	// 调用 LLM（使用 CallTools 接口获取tool调用能力）
	if len(toolDefs) > 0 {
		resp, err := a.config.Model.CallTools(ctx, &llm.ToolCallRequest{
			Messages: messages,
			Tools:    toolDefs,
		})
		if err != nil {
			return "", nil, err
		}

		// 转换tool调用
		var toolCalls []core.ToolCall
		if len(resp.ToolCalls) > 0 {
			toolCalls = make([]core.ToolCall, len(resp.ToolCalls))
			for i, tc := range resp.ToolCalls {
				toolCalls[i] = core.ToolCall{
					ID:   tc.ID,
					Name: tc.Name,
					Args: tc.Arguments,
				}
			}
		}
		return resp.Content, toolCalls, nil
	}

	// 无tool时走 Complete 接口
	resp, err := a.config.Model.Complete(ctx, &llm.CompletionRequest{
		Messages: messages,
	})
	if err != nil {
		return "", nil, err
	}
	return resp.Content, nil, nil
}

// ExecuteTools 执行tool调用
func (d *reactDelegate) ExecuteTools(ctx context.Context, calls []core.ToolCall) []react.ToolResult {
	a := d.agent
	results := make([]react.ToolResult, len(calls))

	for i, call := range calls {
		output, err := a.executeToolForDelegate(ctx, call)
		if err != nil {
			results[i] = react.ToolResult{
				ToolName: call.Name,
				Output:   "error: " + err.Error(),
				IsError:  true,
			}
		} else {
			results[i] = react.ToolResult{
				ToolName: call.Name,
				Output:   output,
			}
		}
	}
	return results
}

// executeToolForDelegate 通过tool注册表执行单个tool（桥接层专用）
func (a *ReActAgent) executeToolForDelegate(ctx context.Context, call core.ToolCall) (string, error) {
	if a.capCache == nil || a.capCache.toolkit == nil {
		return "", fmt.Errorf("tool registry not configured")
	}
	tool, ok := a.capCache.toolkit.Get(call.Name)
	if !ok {
		return "", fmt.Errorf("tool %q not found", call.Name)
	}
	result, err := tool.Execute(ctx, json.RawMessage(call.Args))
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// IsCancelled 检测循环中断
func (d *reactDelegate) IsCancelled(ctx context.Context) bool {
	return d.agent.lifecycle.IsStopped() || ctx.Err() != nil
}

// OnTurnStart 轮次开始回调
func (d *reactDelegate) OnTurnStart(ctx context.Context, turn int) error {
	a := d.agent
	_ = a.fireHookWithPool(HookBeforeTurn, turn)
	if a.hasEventSubscriber() {
		a.publishEvent(EventTurnStart, map[string]int{"turn": turn})
	}
	return nil
}

// OnTurnEnd 轮次结束回调
func (d *reactDelegate) OnTurnEnd(_ context.Context, _ *react.TurnResult) {
	// 指标记录和 memory 保存由 ReActAgent 内部处理
}

// OnComplete 循环完成回调
func (d *reactDelegate) OnComplete(_ context.Context, _ *react.LoopResult) {
	d.agent.publishEvent(EventAgentStop, map[string]string{
		"name":   d.agent.config.Name,
		"status": string(d.agent.lifecycle.Status()),
	})
}

// OnError 循环错误回调
func (d *reactDelegate) OnError(_ context.Context, _ error) {
	_ = d.agent.lifecycle.SetStatus(StatusFailed)
}

// EmitStream 发送流式事件
func (d *reactDelegate) EmitStream(event core.StreamEvent) {
	d.agent.emitStream(d.cfg, StreamEvent(event))
}

// IsStream 是否流式模式
func (d *reactDelegate) IsStream() bool {
	return d.stream
}

// ReactEngine 返回与此 Agent 关联的 react.Engine 实例。
// 外部调用方可通过此方法获取引擎并自定义执行策略。
func (a *ReActAgent) ReactEngine() *react.Engine {
	return react.NewEngine(react.Config{
		AgentName:             a.config.Name,
		MaxTurns:              a.config.MaxTurns,
		SessionID:             a.config.SessionID,
		ParallelToolExecution: a.config.ParallelToolExecution,
		MaxParallelTools:      a.config.MaxParallelTools,
		Logger:                a.logger,
	})
}
