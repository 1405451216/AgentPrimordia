// batch.go 实现 LLM 请求批量处理器
// 将多个并发请求收集到批次中，达到 MaxBatchSize 或 FlushTimeout 后统一执行
package llm

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrBatchClosed 批量处理器已关闭
	ErrBatchClosed = errors.New("batch processor is closed")
)

// BatchConfig 批量处理器配置
type BatchConfig struct {
	// MaxBatchSize 单个批次最大请求数，达到此数量立即刷新
	MaxBatchSize int
	// FlushTimeout 刷新超时，即使批次未满也会在超时后执行
	FlushTimeout time.Duration
}

// DefaultBatchConfig 返回默认批量配置
func DefaultBatchConfig() BatchConfig {
	return BatchConfig{
		MaxBatchSize: 10,
		FlushTimeout: 100 * time.Millisecond,
	}
}

// batchEntry 批次中的单个请求条目
type batchEntry struct {
	ctx  context.Context
	req  *CompletionRequest
	resp chan *CompletionResponse
	err  chan error
}

// BatchProcessor 批量处理器，包装 Provider 实现请求批量执行
// 收集并发请求到批次中，达到 MaxBatchSize 或 FlushTimeout 后统一执行
type BatchProcessor struct {
	provider Provider
	config   BatchConfig

	mu      sync.Mutex
	entries []*batchEntry
	closed  bool

	// flush 信号通道
	flushCh chan struct{}
	// 关闭信号
	done chan struct{}
	// 关闭完成信号
	closedCh chan struct{}
}

// NewBatchProcessor 创建批量处理器
func NewBatchProcessor(provider Provider, config BatchConfig) *BatchProcessor {
	bp := &BatchProcessor{
		provider: provider,
		config:   config,
		entries:  make([]*batchEntry, 0, config.MaxBatchSize),
		flushCh:  make(chan struct{}, 1),
		done:     make(chan struct{}),
		closedCh: make(chan struct{}),
	}

	// 启动后台刷新循环
	go bp.flushLoop()

	return bp
}

// QueueDepth 返回当前等待 flush 的批次条目数（Phase 3 Task 6 导出指标）。
//
// 主要用于：
//   - Prometheus 导出 llm_batch_queue_depth
//   - Pool 调度器决策（背压控制）
func (bp *BatchProcessor) QueueDepth() int {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	return len(bp.entries)
}

// Complete 提交请求到批量处理器，等待执行结果
func (bp *BatchProcessor) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entry := &batchEntry{
		ctx:  ctx,
		req:  req,
		resp: make(chan *CompletionResponse, 1),
		err:  make(chan error, 1),
	}

	// 将请求加入批次
	bp.mu.Lock()
	if bp.closed {
		bp.mu.Unlock()
		return nil, ErrBatchClosed
	}
	bp.entries = append(bp.entries, entry)
	shouldFlush := len(bp.entries) >= bp.config.MaxBatchSize
	bp.mu.Unlock()

	// 如果批次已满，立即触发刷新
	if shouldFlush {
		bp.triggerFlush()
	}

	// 等待结果或上下文取消
	select {
	case resp := <-entry.resp:
		return resp, nil
	case err := <-entry.err:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Stream 批量处理器不支持流式，直接委托给底层 Provider
func (bp *BatchProcessor) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	return bp.provider.Stream(ctx, req)
}

// CallTools 批量处理器不支持tool调用的批量，直接委托给底层 Provider
func (bp *BatchProcessor) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	return bp.provider.CallTools(ctx, req)
}

// Info 返回底层 Provider 的模型信息
func (bp *BatchProcessor) Info() ModelInfo {
	return bp.provider.Info()
}

// Close 关闭批量处理器，等待所有待处理请求完成
func (bp *BatchProcessor) Close() {
	bp.mu.Lock()
	if bp.closed {
		bp.mu.Unlock()
		return
	}
	bp.closed = true
	bp.mu.Unlock()

	// 通知刷新循环退出
	close(bp.done)

	// 等待刷新循环结束
	<-bp.closedCh
}

// triggerFlush 非阻塞地触发一次刷新
func (bp *BatchProcessor) triggerFlush() {
	select {
	case bp.flushCh <- struct{}{}:
	default:
		// 已经有待处理的刷新信号，无需重复触发
	}
}

// flushLoop 后台刷新循环
func (bp *BatchProcessor) flushLoop() {
	defer close(bp.closedCh)

	// 启动时的定时器
	timer := time.NewTimer(bp.config.FlushTimeout)
	defer timer.Stop()

	for {
		select {
		case <-bp.done:
			// 关闭信号：刷新剩余请求后退出
			bp.flush()
			return

		case <-bp.flushCh:
			// 批次满触发刷新
			timer.Stop()
			bp.flush()
			timer.Reset(bp.config.FlushTimeout)

		case <-timer.C:
			// 超时触发刷新
			bp.flush()
			timer.Reset(bp.config.FlushTimeout)
		}
	}
}

// flush 执行一次批量刷新，将当前收集到的所有请求提交给底层 Provider
func (bp *BatchProcessor) flush() {
	bp.mu.Lock()
	if len(bp.entries) == 0 {
		bp.mu.Unlock()
		return
	}
	// 取出当前批次
	entries := bp.entries
	bp.entries = make([]*batchEntry, 0, bp.config.MaxBatchSize)
	bp.mu.Unlock()

	// 并发执行批次中的所有请求
	var wg sync.WaitGroup
	for _, entry := range entries {
		wg.Add(1)
		go func(e *batchEntry) {
			defer wg.Done()
			resp, err := bp.provider.Complete(e.ctx, e.req)
			if err != nil {
				e.err <- err
				return
			}
			e.resp <- resp
		}(entry)
	}
	wg.Wait()
}
