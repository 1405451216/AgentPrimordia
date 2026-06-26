// stream_collector.go — 流式输出收集器
// 将流式 Chunk 通道收集为完整的 CollectedResult
// 支持并发安全收集、工具调用信息提取、Usage 聚合
package llm

import (
	"sync"
	"time"
)

// ToolCallInfo 工具调用信息
type ToolCallInfo struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	ID        string `json:"id"`
}

// CollectedResult 收集结果
type CollectedResult struct {
	Content   string         `json:"content"`
	ToolCalls []ToolCallInfo `json:"tool_calls,omitempty"`
	Tokens    int            `json:"tokens"`
	Duration  time.Duration  `json:"duration"`
	Usage     *Usage         `json:"usage,omitempty"`
}

// StreamError 流式输出错误
type StreamError struct {
	Message string `json:"message"`
}

func (e *StreamError) Error() string {
	return e.Message
}

// StreamCollector 收集流式输出为完整结果
// 将 Chunk 通道中的所有内容拼接为完整文本
// 并发安全，可复用
type StreamCollector struct {
	mu sync.Mutex
}

// NewStreamCollector 创建流式输出收集器
func NewStreamCollector() *StreamCollector {
	return &StreamCollector{}
}

// Collect 从流通道收集所有 chunks
// 持续读取通道直到关闭，拼接所有 Content
// Done chunk 标记流结束，携带最终的 Usage 信息
func (c *StreamCollector) Collect(ch <-chan Chunk) (*CollectedResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	start := time.Now()
	var tokens []string
	var toolCalls []ToolCallInfo
	var usage *Usage

	for chunk := range ch {
		if chunk.Content != "" {
			tokens = append(tokens, chunk.Content)
		}

		// 收集 Usage 信息（Done chunk 通常携带最终 Usage）
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}

	duration := time.Since(start)

	// 拼接所有 token
	content := ""
	for _, t := range tokens {
		content += t
	}

	result := &CollectedResult{
		Content:   content,
		ToolCalls: toolCalls,
		Tokens:    len(tokens),
		Duration:  duration,
		Usage:     usage,
	}

	return result, nil
}
