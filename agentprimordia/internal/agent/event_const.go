// event_const.go — Agent 生命周期事件类型常量
//
// 这些常量用于 publishEvent 调用，字符串值与 internal/events 包中的
// EventType 常量保持一致。agent 包不直接 import events 包（依赖方向规则），
// 而是通过 EventPublisher 接口解耦。消费者通过 events.Bus 订阅时，
// 应使用 events 包中对应的 EventType 常量。
package agent

const (
	EventAgentStart  = "agent.start"
	EventAgentStop   = "agent.stop"
	EventAgentPanic  = "agent.panic"
	EventAgentError  = "agent.error"
	EventAgentResume = "agent.resume"
	EventTurnStart   = "turn.start"
	EventTurnEnd     = "turn.end"
	EventLLMCall     = "llm.call"
	EventLLMResponse = "llm.response"
	EventToolCall    = "tool.call"
	EventToolResult  = "tool.result"
)
