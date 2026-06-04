package agent

import (
	"agentprimordia/internal/llm"
	"agentprimordia/internal/memory"
	"agentprimordia/internal/persist"
	"agentprimordia/internal/tools"
)

// 此文件提供 ReActAgent 的链式 API 入口方法。
// 每个方法创建一个 CapabilityAgent 包装器并注入对应能力，
// 后续可通过 CapabilityAgent 的链式方法继续注入其他能力。
//
// 使用方式：
//
//	agent := NewReActAgent(ReActConfig{
//	    Name: "my-agent", Model: provider, MaxTurns: 10,
//	}).WithMemory(mem).WithRAG(RAGConfig{...}).WithHooks(hooks)

// wrapSelf 创建 CapabilityAgent 并更新 ReActAgent 的自引用
func (a *ReActAgent) wrapSelf(cap *CapabilityAgent) *CapabilityAgent {
	a.self = cap // 更新自引用，使引擎通过接口发现能力
	return cap
}

// WithMemory 注入记忆存储能力，返回可链式调用的 CapabilityAgent
func (a *ReActAgent) WithMemory(m MemoryStore) *CapabilityAgent {
	a.config.Memory = m
	return a.wrapSelf(&CapabilityAgent{inner: a, memory: m})
}

// WithToolkit 注入工具注册表，返回可链式调用的 CapabilityAgent
func (a *ReActAgent) WithToolkit(t *tools.Registry) *CapabilityAgent {
	a.config.Toolkit = t
	return a.wrapSelf(&CapabilityAgent{inner: a, toolkit: t})
}

// WithHooks 注入 Hook 管理器，返回可链式调用的 CapabilityAgent
func (a *ReActAgent) WithHooks(h Hooks) *CapabilityAgent {
	a.hooks = h
	a.config.Hooks = h
	return a.wrapSelf(&CapabilityAgent{inner: a, hooks: h})
}

// WithRAG 注入 RAG 检索能力，返回可链式调用的 CapabilityAgent
func (a *ReActAgent) WithRAG(cfg RAGConfig) *CapabilityAgent {
	a.config.RAG = &cfg
	return a.wrapSelf(&CapabilityAgent{inner: a, rag: &cfg})
}

// WithHITL 注入人机协作能力，返回可链式调用的 CapabilityAgent
func (a *ReActAgent) WithHITL(cfg HITLConfig) *CapabilityAgent {
	a.config.HITL = &cfg
	a.hitlMgr = NewHITLManager(cfg)
	return a.wrapSelf(&CapabilityAgent{inner: a, hitl: &cfg})
}

// WithTracer 注入分布式追踪器，返回可链式调用的 CapabilityAgent
func (a *ReActAgent) WithTracer(t Tracer) *CapabilityAgent {
	a.config.Tracer = t
	return a.wrapSelf(&CapabilityAgent{inner: a, tracer: t})
}

// WithCostTracker 注入成本追踪器，返回可链式调用的 CapabilityAgent
func (a *ReActAgent) WithCostTracker(ct *CostTracker) *CapabilityAgent {
	a.config.CostTracker = ct
	return a.wrapSelf(&CapabilityAgent{inner: a, costTracker: ct})
}

// WithContextWindow 注入上下文窗口裁剪策略，返回可链式调用的 CapabilityAgent
func (a *ReActAgent) WithContextWindow(cw ContextWindowStrategy) *CapabilityAgent {
	a.config.ContextWindow = cw
	return a.wrapSelf(&CapabilityAgent{inner: a, ctxWindow: cw})
}

// WithEvents 注入事件发布器，返回可链式调用的 CapabilityAgent
func (a *ReActAgent) WithEvents(ep EventPublisher) *CapabilityAgent {
	a.config.EventPublisher = ep
	return a.wrapSelf(&CapabilityAgent{inner: a, eventPub: ep})
}

// WithMetrics 注入指标记录器，返回可链式调用的 CapabilityAgent
func (a *ReActAgent) WithMetrics(m MetricsRecorder) *CapabilityAgent {
	a.config.Metrics = m
	return a.wrapSelf(&CapabilityAgent{inner: a, metrics: m})
}

// WithCheckpointStore 注入检查点存储，返回可链式调用的 CapabilityAgent
func (a *ReActAgent) WithCheckpointStore(cs persist.CheckpointStore) *CapabilityAgent {
	a.config.CheckpointStore = cs
	return a.wrapSelf(&CapabilityAgent{inner: a, checkpoint: cs})
}

// WithSummarizer 注入摘要提取器，返回可链式调用的 CapabilityAgent
func (a *ReActAgent) WithSummarizer(s memory.SummaryExtractor) *CapabilityAgent {
	a.config.Summarizer = s
	return a.wrapSelf(&CapabilityAgent{inner: a, summarizer: s})
}

// WithFileScope 注入文件作用域限制，返回可链式调用的 CapabilityAgent
func (a *ReActAgent) WithFileScope(scopes []string) *CapabilityAgent {
	a.config.FileScope = scopes
	return a.wrapSelf(&CapabilityAgent{inner: a, fileScope: scopes})
}

// WithCache 注入 LLM 缓存，返回可链式调用的 CapabilityAgent
// 注意：缓存会在 LLM Provider 层生效，设置后自动包装 Model 为 CachedProvider
func (a *ReActAgent) WithCache(cache llm.LLMCache) *CapabilityAgent {
	a.config.Cache = cache
	// 包装 Model 为 CachedProvider
	if cache != nil {
		cached, err := llm.NewCachedProvider(a.config.Model, cache, 0.8)
		if err == nil {
			a.config.Model = cached
		}
	}
	return a.wrapSelf(&CapabilityAgent{inner: a, cache: cache})
}
