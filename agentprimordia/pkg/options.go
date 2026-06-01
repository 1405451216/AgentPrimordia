package ap

import (
	"time"
)

// Option 是 Agent 行为配置的函数选项
type Option func(*options)

// options 是 Option 的内部配置结构
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

// WithTimeout 设置执行超时时间
func WithTimeout(d time.Duration) Option {
	return func(o *options) { o.timeout = d }
}

// WithMaxTurns 设置 ReAct 循环的最大迭代次数
func WithMaxTurns(n int) Option {
	return func(o *options) { o.maxTurns = n }
}

// WithTemperature 设置 LLM 温度参数
func WithTemperature(t float64) Option {
	return func(o *options) { o.temperature = t }
}

// WithCheckpoint 启用状态检查点，将状态持久化到指定目录
func WithCheckpoint(dir string) Option {
	return func(o *options) { o.checkpointDir = dir }
}

// WithStreaming 启用流式输出模式，每个 chunk 调用回调函数
func WithStreaming(fn StreamingFunc) Option {
	return func(o *options) { o.streamingFn = fn }
}

// WithMetadata 添加自定义元数据到会话
func WithMetadata(m Metadata) Option {
	return func(o *options) { o.metadata = m }
}

// ApplyOptions 将选项列表应用到目标对象。
// 目标对象必须实现 OptionSetter 接口（有 SetOption(string, any) error 方法）。
func ApplyOptions(opts ...Option) options {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
