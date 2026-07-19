// stream_middleware.go — 流式输出中间件管道
// 提供可组合的中间件链，用于对流式 Chunk 进行过滤、变换、缓冲、限流等操作
//
// 性能优化（perf-v2 #2）：
//
//	Use() 时预编译中间件链，Process() 直接执行预编译链，
//	避免每次 Process() 都从最后一个中间件到第一个逐层包装闭包。
package llm

import (
	"log/slog"
	"strings"
	"sync"
	"time"
)

// StreamHandler 流式输出处理器
type StreamHandler func(chunk Chunk) error

// StreamMiddleware 流式输出中间件
// chunk 为当前数据块，next 为链中下一个处理器
// 中间件可在调用 next 前后执行逻辑，也可修改 chunk 或短路不调用 next
type StreamMiddleware func(chunk Chunk, next StreamHandler) error

// StreamPipeline 流式输出管道
// 将多个中间件按顺序组合，最终调用 handler 处理 chunk
type StreamPipeline struct {
	middlewares []StreamMiddleware
	handler     StreamHandler
	mu          sync.RWMutex
	compiled    StreamHandler // 预编译后的中间件链（热路径）
}

// NewStreamPipeline 创建流式输出管道
func NewStreamPipeline(handler StreamHandler) *StreamPipeline {
	p := &StreamPipeline{
		handler: handler,
	}
	p.rebuildChain()
	return p
}

// Use 添加中间件，返回自身以支持链式调用。
// 添加后立即预编译中间件链，使 Process() 热路径无闭包分配。
func (p *StreamPipeline) Use(mw StreamMiddleware) *StreamPipeline {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.middlewares = append(p.middlewares, mw)
	p.rebuildChain()
	return p
}

// rebuildChain 预编译中间件链。
// 调用方必须持有写锁（mu.Lock）。
func (p *StreamPipeline) rebuildChain() {
	handler := p.handler
	for i := len(p.middlewares) - 1; i >= 0; i-- {
		mw := p.middlewares[i]
		current := handler
		handler = func(c Chunk) error { return mw(c, current) }
	}
	p.compiled = handler
}

// Process 处理一个 chunk。
// 直接执行预编译后的中间件链，无闭包分配开销。
func (p *StreamPipeline) Process(chunk Chunk) error {
	p.mu.RLock()
	compiled := p.compiled
	p.mu.RUnlock()
	return compiled(chunk)
}

// ===== 内置中间件 =====

// FilterMiddleware 过滤空 chunk 中间件
// 移除 Content 为空且 Done 为 false 的 chunk
// Done 为 true 的 chunk 即使 Content 为空也会保留（标记流结束）
func FilterMiddleware() StreamMiddleware {
	return func(chunk Chunk, next StreamHandler) error {
		if chunk.Content == "" && !chunk.Done {
			return nil
		}
		return next(chunk)
	}
}

// TransformMiddleware 内容变换中间件
// 对 chunk 的 Content 应用变换函数
func TransformMiddleware(transform func(string) string) StreamMiddleware {
	return func(chunk Chunk, next StreamHandler) error {
		if chunk.Content != "" {
			chunk.Content = transform(chunk.Content)
		}
		return next(chunk)
	}
}

// LoggingMiddleware 日志记录中间件
// 记录每个 chunk 的内容长度和时间戳
func LoggingMiddleware() StreamMiddleware {
	logger := slog.Default()
	return func(chunk Chunk, next StreamHandler) error {
		logger.Info("流式 chunk",
			"content_len", len(chunk.Content),
			"done", chunk.Done,
			"timestamp", time.Now().Format(time.RFC3339Nano),
		)
		return next(chunk)
	}
}

// RateLimitMiddleware 速率限制中间件
// 限制每秒处理的 chunk 数量，使用令牌桶算法
// chunksPerSecond 为每秒允许的最大 chunk 数
func RateLimitMiddleware(chunksPerSecond float64) StreamMiddleware {
	// 令牌桶：初始满桶，允许突发
	var mu sync.Mutex
	tokens := chunksPerSecond * 2 // 突发容量为 2 倍速率
	maxTokens := tokens
	lastRefill := time.Now()

	tryAcquire := func() bool {
		mu.Lock()
		defer mu.Unlock()

		now := time.Now()
		elapsed := now.Sub(lastRefill).Seconds()
		tokens += elapsed * chunksPerSecond
		if tokens > maxTokens {
			tokens = maxTokens
		}
		lastRefill = now

		if tokens >= 1 {
			tokens--
			return true
		}
		return false
	}

	return func(chunk Chunk, next StreamHandler) error {
		// 简单自旋等待获取令牌
		for !tryAcquire() {
			time.Sleep(time.Millisecond)
		}
		return next(chunk)
	}
}

// BufferMiddleware 缓冲中间件
// 缓冲 chunk 内容，在句子边界（. ! ? \n）处刷新
// Done 为 true 时强制刷新所有缓冲内容
func BufferMiddleware() StreamMiddleware {
	var buf strings.Builder
	var mu sync.Mutex

	// isSentenceEnd 判断是否为句子结束边界
	isSentenceEnd := func(s string) bool {
		if len(s) == 0 {
			return false
		}
		last := s[len(s)-1]
		return last == '.' || last == '!' || last == '?' || last == '\n'
	}

	return func(chunk Chunk, next StreamHandler) error {
		mu.Lock()
		defer mu.Unlock()

		if chunk.Done {
			// 流结束，强制刷新
			if buf.Len() > 0 {
				flushChunk := Chunk{Content: buf.String(), Done: true}
				if chunk.Usage != nil {
					flushChunk.Usage = chunk.Usage
				}
				buf.Reset()
				return next(flushChunk)
			}
			return next(chunk)
		}

		buf.WriteString(chunk.Content)

		if isSentenceEnd(buf.String()) {
			// 到达句子边界，刷新缓冲
			flushChunk := Chunk{Content: buf.String()}
			buf.Reset()
			return next(flushChunk)
		}

		return nil
	}
}
