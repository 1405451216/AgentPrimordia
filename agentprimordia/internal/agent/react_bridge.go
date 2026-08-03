// react_bridge.go — ReActAgent 与 react.Engine 的桥接层（B-3 包拆分）
//
// 本文件实现 react.Delegate 接口，将 ReActAgent 的内部能力
// 暴露给 react.Engine 循环状态机。
//
// 依赖方向：agent/ → react/（正向，无循环）
package agent

import (
	"agentprimordia/internal/agent/react"
)

// ReactEngine 返回与此 Agent 关联的 react.Engine 实例。
//
// Deprecated: react.Engine 为 B-3 包拆分的实验性重构骨架，非生产主路径。
// ReAct 循环实际由 reactLoopEngine（react_loop_engine.go）驱动，具备 checkpoint、
// 成本预算、guardrail、RAG、planning、metrics 等完整能力。本方法仅作为实验性
// 执行策略探索入口保留，不建议在生产依赖其行为。
// Removed in v4.0.
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
