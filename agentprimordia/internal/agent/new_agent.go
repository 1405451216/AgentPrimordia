package agent

import "agentprimordia/internal/llm"

// AgentOption 是 NewAgent 的函数式选项。
type AgentOption func(*agentOptions)

type agentOptions struct {
	maxTurns    int
	temperature float64
	sessionID   string
}

// WithMaxTurns 设置 ReAct 循环的最大迭代次数（默认 50）。
func WithMaxTurns(n int) AgentOption {
	return func(o *agentOptions) { o.maxTurns = n }
}

// WithTemperature 设置 LLM 温度参数（默认 0，由 Model 决定）。
func WithTemperature(t float64) AgentOption {
	return func(o *agentOptions) { o.temperature = t }
}

// WithSessionID 设置会话 ID，用于跨轮记忆关联。
func WithSessionID(id string) AgentOption {
	return func(o *agentOptions) { o.sessionID = id }
}

// NewAgent 是创建 Agent 的推荐入口。
//
// 只暴露核心必填字段（名称、系统提示词、模型），能力通过链式 API 注入。
// 等价于 NewReActAgent(ReActConfig{...})，但不会暴露 ReActConfig 中
// 14 个已废弃的配置字段。
//
// 使用方式：
//
//	agent := NewAgent("my-bot", "you are helpful", provider,
//	    WithMaxTurns(10),
//	    WithTemperature(0.7),
//	).WithMemory(mem).WithRAG(ragCfg).WithHooks(hooks)
func NewAgent(name, systemPrompt string, model llm.Provider, opts ...AgentOption) *CapabilityAgent {
	o := &agentOptions{}
	for _, opt := range opts {
		opt(o)
	}

	cfg := ReActConfig{
		Name:         name,
		SystemPrompt: systemPrompt,
		Model:        model,
		MaxTurns:     o.maxTurns,
		Temperature:  o.temperature,
		SessionID:    o.sessionID,
	}

	a := NewReActAgent(cfg)

	// 触发 CapabilityAgent 包装以支持链式注入
	// WithMemory(nil) 将 ReActAgent 包装为 CapabilityAgent，内存能力留空
	return a.WithMemory(nil)
}
