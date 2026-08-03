// engine.go — ReAct 循环状态机（B-3 包拆分核心）
//
// ⚠️ 实验性骨架：本引擎仅覆盖基础 turn 迭代，无 checkpoint/成本预算/
// guardrail/RAG/planning/metrics。生产主路径为 internal/agent 的
// reactLoopEngine，不经过本引擎。
//
// Engine 封装了 turn 迭代骨架：
//
//	for turn := 0; turn < maxTurns; turn++ {
//	    1. 中断检测
//	    2. OnTurnStart 回调
//	    3. 构建 LLM 请求
//	    4. 调用 LLM
//	    5. 判断：tool调用 or 最终答案
//	    6. 执行tool（若有）
//	    7. OnTurnEnd 回调
//	}
//
// 所有副作用（hooks、events、memory、metrics）通过 Delegate 回调注入，
// Engine 本身是纯状态机，无外部依赖。
package react

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"agentprimordia/internal/agent/core"
)

// Engine 是接口驱动的 ReAct 循环引擎
type Engine struct {
	cfg    Config
	logger *slog.Logger
}

// NewEngine 创建循环引擎
func NewEngine(cfg Config) *Engine {
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 50
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Engine{cfg: cfg, logger: cfg.Logger}
}

// Run 执行 ReAct 循环
//
// 接受 Delegate 接口作为宿主 Agent 的抽象，执行 turn 迭代直到：
//   - LLM 返回最终答案（无tool调用）
//   - 达到 MaxTurns 上限
//   - ctx 取消或 Delegate.IsCancelled 返回 true
//   - 发生不可恢复错误
func (e *Engine) Run(ctx context.Context, delegate Delegate, history []core.Message) (*LoopResult, error) {
	startTime := time.Now()
	toolCallCount := 0

	for turn := 0; turn < e.cfg.MaxTurns; turn++ {
		// 步骤 1：中断检测
		if delegate.IsCancelled(ctx) {
			result := &LoopResult{
				TotalTurns:    turn,
				TotalDuration: time.Since(startTime),
				ToolCallCount: toolCallCount,
			}
			delegate.OnComplete(ctx, result)
			return result, context.Canceled
		}

		if ctx.Err() != nil {
			result := &LoopResult{
				TotalTurns:    turn,
				TotalDuration: time.Since(startTime),
				ToolCallCount: toolCallCount,
			}
			delegate.OnError(ctx, ctx.Err())
			return result, ctx.Err()
		}

		// 步骤 2：OnTurnStart 回调
		if err := delegate.OnTurnStart(ctx, turn); err != nil {
			delegate.OnError(ctx, err)
			return nil, fmt.Errorf("turn %d start callback failed: %w", turn, err)
		}

		turnStart := time.Now()

		// 步骤 3+4：调用 LLM
		content, toolCalls, err := delegate.CallLLM(ctx, turn, history)
		if err != nil {
			delegate.OnError(ctx, err)
			return nil, fmt.Errorf("turn %d LLM call failed: %w", turn, err)
		}

		// 步骤 5：判断响应类型
		turnResult := &TurnResult{
			Turn:      turn,
			Content:   content,
			ToolCalls: toolCalls,
			Duration:  time.Since(turnStart),
		}

		if len(toolCalls) == 0 {
			// 最终答案：无tool调用，循环终止
			turnResult.Finished = true

			// 流式输出最终内容
			if delegate.IsStream() {
				delegate.EmitStream(core.StreamEvent{
					Type:    core.StreamEventComplete,
					Content: content,
				})
			}

			delegate.OnTurnEnd(ctx, turnResult)

			result := &LoopResult{
				Content:       content,
				TotalTurns:    turn + 1,
				TotalDuration: time.Since(startTime),
				ToolCallCount: toolCallCount,
			}
			delegate.OnComplete(ctx, result)
			return result, nil
		}

		// 步骤 6：执行tool调用
		toolCallCount += len(toolCalls)
		toolResults := delegate.ExecuteTools(ctx, toolCalls)
		turnResult.ToolResults = toolResults

		// 将tool结果追加到历史（供下一轮 LLM 调用使用）
		history = append(history, core.Message{
			Role:      core.RoleAssistant,
			Content:   content,
			ToolCalls: toolCalls,
		})
		for _, tr := range toolResults {
			history = append(history, core.Message{
				Role:    core.RoleTool,
				Content: tr.Output,
			})
		}

		// 流式输出tool执行事件
		if delegate.IsStream() {
			for _, tr := range toolResults {
				delegate.EmitStream(core.StreamEvent{
					Type:    core.StreamEventToolResult,
					Content: fmt.Sprintf("[%s] %s", tr.ToolName, truncate(tr.Output, 200)),
				})
			}
		}

		// 步骤 7：OnTurnEnd 回调
		turnResult.Duration = time.Since(turnStart)
		delegate.OnTurnEnd(ctx, turnResult)
	}

	// 达到 MaxTurns 上限
	result := &LoopResult{
		Content:       fmt.Sprintf("达到最大轮次限制（%d）", e.cfg.MaxTurns),
		TotalTurns:    e.cfg.MaxTurns,
		TotalDuration: time.Since(startTime),
		ToolCallCount: toolCallCount,
	}
	delegate.OnComplete(ctx, result)
	return result, nil
}

// truncate 截断字符串到指定长度
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
