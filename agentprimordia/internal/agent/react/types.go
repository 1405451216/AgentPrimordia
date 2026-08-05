// Package react 提供接口驱动的 ReAct 循环引擎（B-3 包拆分）。
//
// ⚠️ 实验性骨架：本包是 ReAct 循环接口化拆分的重构探索，仅覆盖基础 turn
// 迭代状态机（无 checkpoint、成本预算、guardrail、RAG、planning、metrics 等能力）。
// Agent 生产主路径为 internal/agent 的 reactLoopEngine，不经过本引擎。
// 本包保留供外部自定义执行策略探索，不作为 Agent 默认运行路径。
//
// 设计目标：
//   - 将 ReAct 循环骨架（turn 迭代状态机）从 internal/agent 包提取为独立子包
//   - 通过 Delegate 接口解耦循环逻辑与 Agent 内部实现
//   - 依赖方向：react/ → core/ + llm/ + tools/（无循环依赖）
//
// 使用方式：
//
//	engine := react.NewEngine(react.Config{...})
//	result, err := engine.Run(ctx, delegate)
//
// agent.ReActAgent 实现 react.Delegate 接口并委托执行。
package react

import (
	"context"
	"log/slog"
	"time"

	"agentprimordia/internal/agent/core"
)

// Config 循环引擎配置
type Config struct {
	// AgentName Agent 名称（用于日志和事件）
	AgentName string
	// MaxTurns 最大循环轮次
	MaxTurns int
	// SessionID 会话标识
	SessionID string
	// ParallelToolExecution 是否启用tool并行执行
	ParallelToolExecution bool
	// MaxParallelTools 单批并行tool数上限（0=无限制）
	MaxParallelTools int
	// Logger 结构化日志
	Logger *slog.Logger
}

// TurnResult 单轮执行结果
type TurnResult struct {
	// Turn 当前轮次（0-based）
	Turn int
	// Content LLM 输出内容
	Content string
	// ToolCalls 本轮tool调用（若有）
	ToolCalls []core.ToolCall
	// ToolResults tool执行结果
	ToolResults []ToolResult
	// Finished 是否产生最终答案（循环应终止）
	Finished bool
	// Duration 本轮耗时
	Duration time.Duration
}

// ToolResult tool执行结果
type ToolResult struct {
	ToolName string
	Output   string
	IsError  bool
	Duration time.Duration
}

// LoopResult 循环最终结果
type LoopResult struct {
	// Content 最终输出内容
	Content string
	// TotalTurns 实际执行轮次
	TotalTurns int
	// TotalDuration 总耗时
	TotalDuration time.Duration
	// ToolCallCount tool调用总次数
	ToolCallCount int
	// RequestID 请求标识
	RequestID string
}

// Delegate 是循环引擎对宿主 Agent 的抽象接口。
// agent.ReActAgent 实现此接口，将内部能力暴露给引擎。
//
// 引擎通过 Delegate 完成：
//   - LLM 调用（CallLLM）
//   - tool执行（ExecuteTools）
//   - 生命周期通知（OnTurnStart / OnTurnEnd / OnComplete / OnError）
//   - 流式输出（EmitStream）
//   - 中断检测（IsCancelled）
type Delegate interface {
	// CallLLM 执行单轮 LLM 调用，返回内容 + tool调用（若有）
	// history 包含完整的对话历史（系统提示词 + 用户输入 + tool结果）
	CallLLM(ctx context.Context, turn int, history []core.Message) (content string, toolCalls []core.ToolCall, err error)

	// ExecuteTools 执行tool调用并返回结果
	ExecuteTools(ctx context.Context, calls []core.ToolCall) []ToolResult

	// IsCancelled 检测循环是否应中断（Agent 停止或 ctx 取消）
	IsCancelled(ctx context.Context) bool

	// OnTurnStart 每轮开始前的回调（hooks、事件、span 创建）
	OnTurnStart(ctx context.Context, turn int) error

	// OnTurnEnd 每轮结束后的回调（memory 保存、指标记录）
	OnTurnEnd(ctx context.Context, result *TurnResult)

	// OnComplete 循环正常完成时的回调
	OnComplete(ctx context.Context, result *LoopResult)

	// OnError 循环异常终止时的回调
	OnError(ctx context.Context, err error)

	// EmitStream 发送流式事件（非流式模式下可忽略）
	EmitStream(event core.StreamEvent)

	// IsStream 是否处于流式模式
	IsStream() bool
}
