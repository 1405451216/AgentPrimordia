// config.go — AgentConfig 分组配置 struct
// v0.7.0 API 稳定化：引入分组式 Functional Options 模式（类似 gRPC KeepaliveParams），
// 替代 ReActConfig 中扁平的 14 个已废弃字段。后续 NewAgent 重写将基于此结构。
package agent

import (
	"errors"
	"fmt"
	"log/slog"

	"agentprimordia/internal/agent/learning"
	"agentprimordia/internal/llm"
	"agentprimordia/internal/memory"
	"agentprimordia/internal/persist"
	"agentprimordia/internal/tools"
)

// perf-v6 round 4 Task 2：config 静态错误
var (
	ErrAgentNameRequired  = errors.New("agent name is required")
	ErrAgentModelRequired = errors.New("agent model (LLM Provider) is required")
)

// MemoryConfig 记忆能力分组配置
type MemoryConfig struct {
	// Store 记忆存储，存储对话和记忆片段
	Store MemoryStore

	// Summarizer 记忆摘要生成器
	Summarizer memory.SummaryExtractor

	// FileScope 文件范围限制，指定 Agent 可操作的文件或目录列表
	FileScope []string
}

// ObservabilityConfig 可观测性分组配置
type ObservabilityConfig struct {
	// Hooks Hook 管理器
	Hooks Hooks

	// Tracer 分布式追踪器
	Tracer Tracer

	// Metrics 指标收集器
	Metrics MetricsRecorder

	// Events 事件发布器
	Events EventPublisher

	// CostTracker 成本追踪器
	CostTracker *CostTracker
}

// ResilienceConfig 韧性分组配置
type ResilienceConfig struct {
	// CheckpointStore 状态持久化存储
	CheckpointStore persist.CheckpointStore

	// HITL 人机协作配置
	HITL *HITLConfig

	// Cache LLM 响应缓存
	Cache llm.LLMCache

	// ContextWindow 上下文窗口裁剪策略
	ContextWindow ContextWindowStrategy
}

// ToolsConfig 工具系统分组配置
type ToolsConfig struct {
	// Registry 工具注册表
	Registry *tools.Registry
}

// LearningConfig 自适应学习能力分组配置（v3.0）。
//
// 注入后引擎在 Agent 完成推理后自动从交互中蒸馏知识，
// 并可评估能力弱项、记录人类反馈。
type LearningConfig struct {
	// Distiller 知识蒸馏器：从交互中提取事实/模式/偏好
	Distiller *learning.KnowledgeDistiller

	// Evolver 能力进化器：评估 Agent 能力弱项
	Evolver *learning.CapabilityEvolver

	// FeedbackLearner 反馈学习器：记录人类反馈并调整行为偏好
	FeedbackLearner *learning.FeedbackLearner
}

// AgentConfig 是 Agent 的分组式配置结构。
//
// v0.7.0 引入，替代 ReActConfig 的扁平字段布局。字段按职责分组：
//   - 核心必填：Name / SystemPrompt / Model / PromptTemplate
//   - 标量参数：MaxTurns / Temperature / SessionID
//   - 能力分组：Memory / RAG / Observability / Resilience / Tools
//   - 运行时辅助：Lifecycle / Logger
//
// 推荐通过 Functional Options 构造（见后续 Task），直接构造时请调用
// defaultConfig() 获取合理默认值后再覆盖。
type AgentConfig struct {
	// ===== 核心配置（必填） =====
	Name           string
	SystemPrompt   string
	Model          llm.Provider
	PromptTemplate *PromptTemplate

	// ===== 标量配置 =====
	MaxTurns    int
	Temperature float64
	SessionID   string

	// ===== 能力分组 =====
	Memory        MemoryConfig
	RAG           RAGConfig
	Observability ObservabilityConfig
	Resilience    ResilienceConfig
	Tools         ToolsConfig
	Learning      LearningConfig

	// ===== 运行时辅助 =====
	Lifecycle *Lifecycle
	Logger    *slog.Logger
}

// defaultConfig 返回带有合理默认值的 AgentConfig。
// 默认值：MaxTurns=50, Temperature=0.0, Lifecycle=NewLifecycle(), Logger=slog.Default()
func defaultConfig() AgentConfig {
	return AgentConfig{
		MaxTurns:    50,
		Temperature: 0.0,
		Lifecycle:   NewLifecycle(),
		Logger:      slog.Default(),
	}
}

// Validate 校验 AgentConfig 的必填字段与约束。
// 返回的错误信息面向用户，可直接展示。
//
// 校验规则：
//   - Name 不能为空
//   - Model (LLM Provider) 不能为 nil
//   - MaxTurns 必须为正数
func (c *AgentConfig) Validate() error {
	if c.Name == "" {
		return ErrAgentNameRequired
	}
	if c.Model == nil {
		return ErrAgentModelRequired
	}
	if c.MaxTurns <= 0 {
		return fmt.Errorf("MaxTurns must be positive, got %d", c.MaxTurns)
	}
	return nil
}
