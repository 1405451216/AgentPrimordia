package agent

import "agentprimordia/internal/llm"

// AgentOption 是 NewAgent 的函数式选项（已废弃，使用 Option 代替）。
//
// Deprecated: 使用 Option 代替。保留为类型别名以维持向后兼容，
// v0.7.0 起 NewAgent 直接接受 Option 类型。
type AgentOption = Option

// NewAgent 是创建 Agent 的推荐入口。
//
// 只暴露核心必填字段（名称、系统提示词、模型），能力通过 Functional Options 注入。
// 等价于 NewReActAgent(ReActConfig{...})，但不会暴露 ReActConfig 中
// 14 个已废弃的配置字段。
//
// 使用方式：
//
//	agent := NewAgent("my-bot", "you are helpful", provider,
//	    WithMaxTurns(10),
//	    WithTemperature(0.7),
//	    WithMemory(mem),
//	    WithRAG(ragCfg),
//	)
func NewAgent(name, systemPrompt string, model llm.Provider, opts ...Option) *CapabilityAgent {
	cfg := defaultConfig()
	cfg.Name = name
	cfg.SystemPrompt = systemPrompt
	cfg.Model = model
	for _, opt := range opts {
		opt(&cfg)
	}

	// 转换 AgentConfig 到 ReActConfig（NewReActAgent 仍使用 ReActConfig）
	reactCfg := ReActConfig{
		Name:           cfg.Name,
		SystemPrompt:   cfg.SystemPrompt,
		Model:          cfg.Model,
		PromptTemplate: cfg.PromptTemplate,
		MaxTurns:       cfg.MaxTurns,
		Temperature:    cfg.Temperature,
		SessionID:      cfg.SessionID,
	}

	a := NewReActAgent(reactCfg)

	// 包装为 CapabilityAgent 以暴露链式 API
	return a.AsCapability()
}
