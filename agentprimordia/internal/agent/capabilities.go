package agent

import (
	"agentprimordia/internal/agent/planning"
	"agentprimordia/internal/agent/reflection"
	"agentprimordia/internal/agent/tool_learning"
	"agentprimordia/internal/llm"
	"agentprimordia/internal/memory"
	"agentprimordia/internal/persist"
	"agentprimordia/internal/tools"
)

// 此文件定义 Capable 接口协议，实现协议式微内核架构。
//
// 设计理念：借鉴 Go 标准库的接口发现模式（io.Reader、http.Handler），
// 让能力成为接口而非配置字段。ReAct 引擎通过类型断言自动发现 Agent
// 实现了哪些 Capable 接口，从而启用对应能力。
//
// 使用方式：
//
//	agent := NewReActAgent(ReActConfig{...}).WithMemory(mem).WithRAG(ragCfg)
//	// 引擎自动发现 agent 实现了 MemoryCapable 和 RAGCapable
//	// 无需在 config 中声明"我要用 RAG"

// MemoryCapable 标识 Agent 具备记忆存储能力。
// ReAct 引擎在每轮结束后自动保存对话到 MemoryStore。
type MemoryCapable interface {
	GetMemoryStore() MemoryStore
}

// RAGCapable 标识 Agent 具备 RAG 检索能力。
// ReAct 引擎在每轮推理前自动查询知识库并注入上下文。
type RAGCapable interface {
	GetRAGConfig() *RAGConfig
}

// HITLCapable 标识 Agent 具备人机协作能力。
// 工具执行前自动检查是否需要人类确认。
type HITLCapable interface {
	GetHITLConfig() *HITLConfig
}

// HookCapable 标识 Agent 具备 Hook 能力。
// 引擎在关键点自动触发注册的 Hook 函数。
type HookCapable interface {
	GetHooks() Hooks
}

// TraceCapable 标识 Agent 具备分布式追踪能力。
// 引擎在 ReAct Loop 关键点自动创建 Span。
type TraceCapable interface {
	GetTracer() Tracer
}

// CostCapable 标识 Agent 具备成本追踪能力。
// 引擎自动记录每轮 LLM Usage 并追踪成本。
type CostCapable interface {
	GetCostTracker() *CostTracker
}

// ContextWindowCapable 标识 Agent 具备上下文窗口裁剪能力。
// 引擎在 LLM 调用前自动裁剪历史消息。
type ContextWindowCapable interface {
	GetContextWindowStrategy() ContextWindowStrategy
}

// EventCapable 标识 Agent 具备事件发布能力。
// 引擎在生命周期关键点自动发布事件。
type EventCapable interface {
	GetEventPublisher() EventPublisher
}

// MetricsCapable 标识 Agent 具备指标记录能力。
// 引擎自动收集 LLM 调用、工具调用等指标。
type MetricsCapable interface {
	GetMetricsRecorder() MetricsRecorder
}

// CheckpointCapable 标识 Agent 具备检查点持久化能力。
// 引擎在每轮结束后自动保存 Agent 状态。
type CheckpointCapable interface {
	GetCheckpointStore() persist.CheckpointStore
}

// SummarizerCapable 标识 Agent 具备摘要提取能力。
// 引擎在保存记忆后异步提取摘要和标签。
type SummarizerCapable interface {
	GetSummarizer() memory.SummaryExtractor
}

// FileScopeCapable 标识 Agent 具备文件作用域限制能力。
// 引擎在构建系统提示词时自动注入文件权限规则。
type FileScopeCapable interface {
	GetFileScope() []string
}

// CacheCapable 标识 Agent 具备 LLM 缓存能力。
// 引擎自动缓存 Complete 调用结果以减少重复请求。
type CacheCapable interface {
	GetCache() llm.LLMCache
}

// ToolkitCapable 标识 Agent 具备工具注册表能力。
// 引擎通过此接口发现可用的工具定义。
type ToolkitCapable interface {
	GetToolkit() *tools.Registry
}

// PlanningCapable 标识 Agent 具备任务规划能力。
// Agent 可以将复杂任务分解为子任务并生成执行计划。
type PlanningCapable interface {
	GetPlanner() planning.Planner
}

// ReflectionCapable 标识 Agent 具备自我反思能力。
// Agent 可以对输出进行反思、批评和改进。
type ReflectionCapable interface {
	GetReflector() reflection.Reflector
}

// ToolLearningCapable 标识 Agent 具备工具学习能力。
// Agent 可以记录工具使用经验，获取最佳实践，并基于历史经验建议改进。
type ToolLearningCapable interface {
	GetToolLearner() tool_learning.ToolLearner
}
