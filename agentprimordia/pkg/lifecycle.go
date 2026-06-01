package ap

import (
	"agentprimordia/internal/agent"
)

// Lifecycle 管理 Agent 的生命周期状态转换（idle / running / paused / stopped / completed / failed / cancelled）
type Lifecycle = agent.Lifecycle

// ContextWindowStrategy 是上下文窗口裁剪策略接口，定义如何裁剪过长的历史消息
type ContextWindowStrategy = agent.ContextWindowStrategy

// DefaultStrategy 是默认的上下文窗口裁剪策略，保留系统消息和最近的对话历史
type DefaultStrategy = agent.DefaultStrategy

var (
	// NewLifecycle 创建 Agent 生命周期管理器实例
	NewLifecycle = agent.NewLifecycle
	// NewDefaultStrategy 创建默认上下文窗口裁剪策略
	NewDefaultStrategy = agent.NewDefaultStrategy
)
