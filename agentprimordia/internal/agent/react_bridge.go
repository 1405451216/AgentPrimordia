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
