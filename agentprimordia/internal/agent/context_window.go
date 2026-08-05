// context_window.go — context 子包的类型别名，保持向后兼容
package agent

import (
	"agentprimordia/internal/agent/context"
)

// ContextWindowStrategy 上下文窗口裁剪策略接口
// 类型别名保持向后兼容
type ContextWindowStrategy = context.Strategy

// DefaultStrategy 默认滑动窗口策略
// 类型别名保持向后兼容
type DefaultStrategy = context.DefaultStrategy

// NewDefaultStrategy 创建默认策略
// 委托到 context 子包，保持向后兼容
func NewDefaultStrategy(keepLast int) *DefaultStrategy {
	return context.NewDefaultStrategy(keepLast)
}

// SummarizingStrategy 摘要压缩策略：超出窗口时保留系统消息与目标，折叠中间历史。
// 相比纯滑动窗口，长任务不丢失原始目标（v3.4-2）。
type SummarizingStrategy = context.SummarizingStrategy

// NewSummarizingStrategy 创建摘要压缩策略
func NewSummarizingStrategy(keepLast int) *SummarizingStrategy {
	return context.NewSummarizingStrategy(keepLast)
}
