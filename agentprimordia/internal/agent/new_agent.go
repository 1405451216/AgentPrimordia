package agent

import (
	"fmt"

	"agentprimordia/internal/llm"
)

// AgentOption 是 NewAgent 的函数式选项类型。
//
// 注意：pkg/options.go 中另有一个 Option 类型（func(*options)）用于 ApplyOptions，
// 与此处的 AgentOption 不同。NewAgent 接受的是 AgentOption。
type AgentOption = Option

// NewAgent 是创建 Agent 的推荐入口（v0.7.0 起）。
//
// 只暴露核心字段（名称、系统提示词、模型），能力通过 Functional Options 注入。
// 构造后核心能力不可变，消除了 ReActConfig 中 14 个已废弃字段的直接使用。
//
// 参数：
//   - name: Agent 名称（不能为空）
//   - systemPrompt: 系统提示词（可为空）
//   - model: LLM Provider（不能为 nil）
//
// 可选能力通过 Option 函数注入。配置校验失败时返回 error。
//
// 使用方式：
//
//	agent, err := NewAgent("my-bot", "you are helpful", provider,
//	    WithMaxTurns(10),
//	    WithTemperature(0.7),
//	    WithMemory(memStore),
//	    WithToolkit(registry),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
func NewAgent(name, systemPrompt string, model llm.Provider, opts ...Option) (*CapabilityAgent, error) {
	cfg := defaultConfig()
	cfg.Name = name
	cfg.SystemPrompt = systemPrompt
	cfg.Model = model
	for _, opt := range opts {
		opt(&cfg)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid agent config: %w", err)
	}
	return buildAgent(cfg)
}

// buildAgent 从 AgentConfig 构建 Agent，通过链式 API 注入所有能力。
// 这是 NewAgent 的内部实现，将分组配置转换为链式调用。
// 当前实现不会返回 error，保留 error 返回值供未来扩展（如能力组合冲突校验）。
func buildAgent(cfg AgentConfig) (*CapabilityAgent, error) {
	// 构造 ReActAgent（复用现有逻辑，只传递核心标量字段）
	reactCfg := ReActConfig{
		Name:           cfg.Name,
		SystemPrompt:   cfg.SystemPrompt,
		PromptTemplate: cfg.PromptTemplate,
		Model:          cfg.Model,
		MaxTurns:       cfg.MaxTurns,
		Temperature:    cfg.Temperature,
		SessionID:      cfg.SessionID,
		Lifecycle:      cfg.Lifecycle,
		Logger:         cfg.Logger,
		// 认知能力：Reflection 改进的严重度阈值
		ReflectionSeverityThreshold: cfg.Cognition.ReflectionSeverityThreshold,
	}
	a := newReActAgent(reactCfg)

	// 包装为 CapabilityAgent 以暴露链式 API
	cap := a.AsCapability()

	// 注入认知能力（Planning / Reflection）
	if cfg.Cognition.Planner != nil {
		cap = cap.WithPlanner(cfg.Cognition.Planner)
	}
	if cfg.Cognition.Reflector != nil {
		cap = cap.WithReflector(cfg.Cognition.Reflector)
	}

	// 注入记忆能力
	if cfg.Memory.Store != nil {
		cap = cap.WithMemory(cfg.Memory.Store)
	}
	if cfg.Memory.Summarizer != nil {
		cap = cap.WithSummarizer(cfg.Memory.Summarizer)
	}
	if len(cfg.Memory.FileScope) > 0 {
		cap = cap.WithFileScope(cfg.Memory.FileScope)
	}

	// 注入tool能力
	if cfg.Tools.Registry != nil {
		cap = cap.WithToolkit(cfg.Tools.Registry)
	}

	// 注入可观测性能力
	if cfg.Observability.Hooks != nil {
		cap = cap.WithHooks(cfg.Observability.Hooks)
	}
	if cfg.Observability.Tracer != nil {
		cap = cap.WithTracer(cfg.Observability.Tracer)
	}
	if cfg.Observability.InputGuard != nil {
		cap = cap.WithInputGuard(cfg.Observability.InputGuard)
	}
	if cfg.Observability.Metrics != nil {
		cap = cap.WithMetrics(cfg.Observability.Metrics)
	}
	if cfg.Observability.Events != nil {
		cap = cap.WithEvents(cfg.Observability.Events)
	}
	if cfg.Observability.CostTracker != nil {
		cap = cap.WithCostTracker(cfg.Observability.CostTracker)
	}

	// 注入韧性能力
	if cfg.Resilience.CheckpointStore != nil {
		cap = cap.WithCheckpointStore(cfg.Resilience.CheckpointStore)
	}
	if cfg.Resilience.HITL != nil {
		cap = cap.WithHITL(*cfg.Resilience.HITL)
	}
	if cfg.Resilience.Cache != nil {
		cap = cap.WithCache(cfg.Resilience.Cache)
	}
	if cfg.Resilience.ContextWindow != nil {
		cap = cap.WithContextWindow(cfg.Resilience.ContextWindow)
	}

	// 注入 RAG 能力（通过 Provider 是否设置判断启用）
	if cfg.RAG.Provider != nil {
		cap = cap.WithRAG(cfg.RAG)
	}

	// 注入自适应学习能力（v3.0）
	if cfg.Learning.Distiller != nil || cfg.Learning.Evolver != nil || cfg.Learning.FeedbackLearner != nil {
		cap = cap.WithLearning(cfg.Learning)
	}

	return cap, nil
}
