package llm

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// batchMockProvider 批量测试专用的 mock Provider
// 记录调用次数和并发信息，用于验证批量行为
type batchMockProvider struct {
	mu         sync.Mutex
	callCount  int
	responses  []*CompletionResponse
	err        error
	// 记录最大并发调用数
	maxConcurrent atomic.Int32
	currentConcurrent atomic.Int32
}

func newBatchMockProvider() *batchMockProvider {
	return &batchMockProvider{}
}

func (m *batchMockProvider) WithResponse(content string) *batchMockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, &CompletionResponse{
		ID:      "batch-mock-id",
		Content: content,
		Role:    "assistant",
		Usage:   Usage{PromptTokens: 5, CompletionTokens: len(content) / 4, TotalTokens: 5 + len(content)/4},
	})
	return m
}

func (m *batchMockProvider) WithError(err error) *batchMockProvider {
	m.err = err
	return m
}

func (m *batchMockProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	// 跟踪并发数
	cur := m.currentConcurrent.Add(1)
	defer m.currentConcurrent.Add(-1)

	// 更新最大并发
	for {
		old := m.maxConcurrent.Load()
		if cur <= old || m.maxConcurrent.CompareAndSwap(old, cur) {
			break
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++

	if m.err != nil {
		return nil, m.err
	}

	if len(m.responses) == 0 {
		return &CompletionResponse{
			ID:      "batch-default",
			Content: "default batch response",
			Role:    "assistant",
			Usage:   Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
		}, nil
	}

	resp := m.responses[0]
	m.responses = m.responses[1:]
	return resp, nil
}

func (m *batchMockProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	ch := make(chan Chunk, 1)
	ch <- Chunk{Content: "batch-stream", Done: true}
	close(ch)
	return ch, nil
}

func (m *batchMockProvider) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	return &ToolCallResponse{
		Content:   "batch-tools",
		ToolCalls: []FunctionCall{},
		Usage:     Usage{},
	}, nil
}

func (m *batchMockProvider) Info() ModelInfo {
	return ModelInfo{
		Name:              "batch-mock-model",
		Provider:          "batch-mock",
		MaxContext:        4096,
		SupportsTools:     true,
		SupportsStreaming: true,
	}
}

func (m *batchMockProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// TestBatchProcessor_SingleRequest 单个请求通过批量处理器并返回响应
func TestBatchProcessor_SingleRequest(t *testing.T) {
	mock := newBatchMockProvider().WithResponse("hello from batch")

	cfg := BatchConfig{
		MaxBatchSize: 5,
		FlushTimeout: 100 * time.Millisecond,
	}
	bp := NewBatchProcessor(mock, cfg)
	defer bp.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := bp.Complete(ctx, &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
		Model:    "batch-mock-model",
	})
	if err != nil {
		t.Fatalf("Complete 返回错误: %v", err)
	}

	if resp.Content != "hello from batch" {
		t.Errorf("期望内容 'hello from batch'，实际 '%s'", resp.Content)
	}
	if resp.Role != "assistant" {
		t.Errorf("期望角色 'assistant'，实际 '%s'", resp.Role)
	}
}

// TestBatchProcessor_FlushTimeout 请求在超时后即使批次未满也会被刷新执行
func TestBatchProcessor_FlushTimeout(t *testing.T) {
	mock := newBatchMockProvider().WithResponse("flushed")

	cfg := BatchConfig{
		MaxBatchSize: 100, // 很大的批次，不会因为满了而触发
		FlushTimeout: 50 * time.Millisecond,
	}
	bp := NewBatchProcessor(mock, cfg)
	defer bp.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := bp.Complete(ctx, &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "test flush"}},
		Model:    "batch-mock-model",
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Complete 返回错误: %v", err)
	}

	if resp.Content != "flushed" {
		t.Errorf("期望内容 'flushed'，实际 '%s'", resp.Content)
	}

	// 请求应该在 FlushTimeout 左右完成，不应等待太久
	if elapsed > 2*time.Second {
		t.Errorf("请求耗时 %v，超过预期（应在 FlushTimeout 附近完成）", elapsed)
	}

	// 底层 Provider 应该被调用了
	if mock.CallCount() != 1 {
		t.Errorf("期望底层 Provider 被调用 1 次，实际 %d 次", mock.CallCount())
	}
}

// TestBatchProcessor_Close 关闭处理器应该是安全的
func TestBatchProcessor_Close(t *testing.T) {
	mock := newBatchMockProvider().WithResponse("before-close")

	cfg := BatchConfig{
		MaxBatchSize: 5,
		FlushTimeout: 100 * time.Millisecond,
	}
	bp := NewBatchProcessor(mock, cfg)

	// 先发一个请求确保处理器正常工作
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := bp.Complete(ctx, &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "before close"}},
		Model:    "batch-mock-model",
	})
	if err != nil {
		t.Fatalf("关闭前的请求返回错误: %v", err)
	}
	if resp.Content != "before-close" {
		t.Errorf("期望内容 'before-close'，实际 '%s'", resp.Content)
	}

	// 关闭处理器，不应 panic
	bp.Close()

	// 重复关闭也不应 panic
	bp.Close()
}

// TestBatchProcessor_MultipleRequests 多个请求被批量处理
func TestBatchProcessor_MultipleRequests(t *testing.T) {
	mock := newBatchMockProvider()
	for i := 0; i < 3; i++ {
		mock.WithResponse("response-" + string(rune('A'+i)))
	}

	cfg := BatchConfig{
		MaxBatchSize: 3, // 刚好容纳 3 个请求
		FlushTimeout: 500 * time.Millisecond,
	}
	bp := NewBatchProcessor(mock, cfg)
	defer bp.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	results := make([]string, 3)
	errors := make([]error, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp, err := bp.Complete(ctx, &CompletionRequest{
				Messages: []ChatMessage{{Role: "user", Content: "request"}},
				Model:    "batch-mock-model",
			})
			if err != nil {
				errors[idx] = err
				return
			}
			results[idx] = resp.Content
		}(i)
	}

	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Errorf("请求 %d 返回错误: %v", i, err)
		}
	}

	// 底层 Provider 应被调用 3 次
	if mock.CallCount() != 3 {
		t.Errorf("期望底层 Provider 被调用 3 次，实际 %d 次", mock.CallCount())
	}
}

// TestBatchProcessor_ContextCancel 上下文取消时请求应返回错误
func TestBatchProcessor_ContextCancel(t *testing.T) {
	mock := newBatchMockProvider().WithResponse("should-not-see")

	cfg := BatchConfig{
		MaxBatchSize: 5,
		FlushTimeout: 2 * time.Second, // 长超时，确保不会先执行
	}
	bp := NewBatchProcessor(mock, cfg)
	defer bp.Close()

	ctx, cancel := context.WithCancel(context.Background())
	// 立即取消
	cancel()

	_, err := bp.Complete(ctx, &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "cancelled"}},
		Model:    "batch-mock-model",
	})

	if err == nil {
		t.Error("期望上下文取消返回错误，但得到了 nil")
	}
}
