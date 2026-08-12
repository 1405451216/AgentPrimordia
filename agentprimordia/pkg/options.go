// Stability: Stable — 函数式选项（WithXxx / ApplyOptions）。
package ap

import (
	"time"
)

// AppOption 是 Agent 行为配置的函数选项（pkg/ap 级别）。
//
// v6.x 修复（评估报告 §五.1 "api-surface 爆炸"）：旧命名 Option 与
// pkg/agent.AgentOption 同名但语义不同（前者操作 *options，后者操作
// agent.AgentConfig），易被误用。新命名 AppOption 让两种 option 类型
// 在 godoc 自动补全时区分清晰。
//
// 兼容：原 Option 类型作为 deprecated alias 保留至少 1 个 minor 版本
// （计划在 v7.0 移除），调用方可在不改代码情况下迁移到 AppOption。
type AppOption func(*options)

// Option 是 AppOption 的历史命名（DEPRECATED: 请使用 AppOption）。
//
// Deprecated: 自 v6.x 起，pkg/ap 级别的 Option 类型重命名为 AppOption，
// 避免与 pkg/agent.AgentOption 混淆。保留此类型作为过渡别名，
// 将在 v7.0 移除。
type Option = AppOption

// options 是 AppOption 的内部配置结构
type options struct {
	timeout       time.Duration
	maxTurns      int
	temperature   float64
	checkpointDir string
	streamingFn   StreamingFunc
	metadata      Metadata
}

// StreamingFunc 是流式输出时每个 chunk 的回调函数
type StreamingFunc func(chunk string)

// WithTimeout 设置execution timeout时间
func WithTimeout(d time.Duration) AppOption {
	return func(o *options) { o.timeout = d }
}

// WithMaxIterations 设置 ReAct 循环的最大迭代次数（区别于 agent.WithMaxTurns 的 AgentOption 类型）
func WithMaxIterations(n int) AppOption {
	return func(o *options) { o.maxTurns = n }
}

// WithTemp 设置 LLM 温度参数（区别于 agent.WithTemperature 的 AgentOption 类型）
func WithTemp(t float64) AppOption {
	return func(o *options) { o.temperature = t }
}

// WithCheckpoint 启用状态检查点，将状态持久化到指定目录
func WithCheckpoint(dir string) AppOption {
	return func(o *options) { o.checkpointDir = dir }
}

// WithStreaming 启用流式输出模式，每个 chunk 调用回调函数
func WithStreaming(fn StreamingFunc) AppOption {
	return func(o *options) { o.streamingFn = fn }
}

// WithMetadata 添加自定义元数据到会话
func WithMetadata(m Metadata) AppOption {
	return func(o *options) { o.metadata = m }
}

// ApplyOptions 将选项列表应用到目标对象。
// 目标对象必须实现 OptionSetter 接口（有 SetOption(string, any) error 方法）。
func ApplyOptions(opts ...AppOption) options {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
