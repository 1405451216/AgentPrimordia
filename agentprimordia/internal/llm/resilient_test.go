package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestResilient_PrimarySuccess(t *testing.T) {
	mock := NewMockLLM(t).WithResponse("success")
	provider, _ := NewResilientProvider(mock, DefaultResilientConfig())

	resp, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "success" {
		t.Errorf("expected 'success', got '%s'", resp.Content)
	}
}

func TestResilient_PrimaryRetryThenSuccess(t *testing.T) {
	callCount := 0
	mock := &retryMockLLM{
		MockLLM: NewMockLLM(t),
		fn: func() (*CompletionResponse, error) {
			callCount++
			if callCount == 1 {
				return nil, errors.New("temporary error")
			}
			return &CompletionResponse{
				ID:      "retry-success",
				Content: "success",
				Role:    "assistant",
			}, nil
		},
	}

	config := DefaultResilientConfig()
	config.MaxRetries = 2
	config.RetryBackoff = 10 * time.Millisecond

	provider, _ := NewResilientProvider(mock, config)

	resp, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "success" {
		t.Errorf("expected 'success', got '%s'", resp.Content)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

type retryMockLLM struct {
	*MockLLM
	fn func() (*CompletionResponse, error)
}

func (m *retryMockLLM) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	return m.fn()
}

type streamErrorMock struct {
	*MockLLM
	streamErr error
}

func (m *streamErrorMock) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	return nil, m.streamErr
}

func TestResilient_PrimaryFails_FallbackSuccess(t *testing.T) {
	primary := NewMockLLM(t).WithError(errors.New("primary failed"))
	fallback := NewMockLLM(t).WithResponse("fallback success")

	config := DefaultResilientConfig()
	config.MaxRetries = 1
	config.RetryBackoff = 10 * time.Millisecond

	provider, _ := NewResilientProvider(primary, config)
	provider.AddFallback(fallback)

	resp, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "fallback success" {
		t.Errorf("expected 'fallback success', got '%s'", resp.Content)
	}
}

func TestResilient_AllFail_ReturnError(t *testing.T) {
	primary := NewMockLLM(t).WithError(errors.New("primary failed"))
	fallback := NewMockLLM(t).WithError(errors.New("fallback failed"))

	config := DefaultResilientConfig()
	config.MaxRetries = 1
	config.RetryBackoff = 10 * time.Millisecond

	provider, _ := NewResilientProvider(primary, config)
	provider.AddFallback(fallback)

	_, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResilient_CircuitBreakerOpens(t *testing.T) {
	mock := NewMockLLM(t).WithError(errors.New("always fails"))

	config := DefaultResilientConfig()
	config.MaxRetries = 0
	config.CircuitThreshold = 3

	provider, _ := NewResilientProvider(mock, config)

	for i := 0; i < 3; i++ {
		_, _ = provider.Complete(context.Background(), &CompletionRequest{})
	}

	_, err := provider.Complete(context.Background(), &CompletionRequest{})
	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestResilient_CircuitHalfOpenRecovers(t *testing.T) {
	mock := NewMockLLM(t).WithError(errors.New("fails"))

	config := DefaultResilientConfig()
	config.MaxRetries = 0
	config.CircuitThreshold = 2
	config.CircuitRecoverAfter = 50 * time.Millisecond

	provider, _ := NewResilientProvider(mock, config)

	for i := 0; i < 2; i++ {
		_, _ = provider.Complete(context.Background(), &CompletionRequest{})
	}

	_, err := provider.Complete(context.Background(), &CompletionRequest{})
	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	mock2 := NewMockLLM(t).WithResponse("recovered")
	provider.primary = mock2

	resp, err := provider.Complete(context.Background(), &CompletionRequest{})
	if err != nil {
		t.Fatalf("expected recovery, got error: %v", err)
	}
	if resp.Content != "recovered" {
		t.Errorf("expected 'recovered', got '%s'", resp.Content)
	}
}

func TestResilient_CircuitRejectsWhenOpen(t *testing.T) {
	mock := NewMockLLM(t).WithError(errors.New("fails"))

	config := DefaultResilientConfig()
	config.MaxRetries = 0
	config.CircuitThreshold = 1

	provider, _ := NewResilientProvider(mock, config)

	_, _ = provider.Complete(context.Background(), &CompletionRequest{})

	for i := 0; i < 5; i++ {
		_, err := provider.Complete(context.Background(), &CompletionRequest{})
		if err != ErrCircuitOpen {
			t.Errorf("iteration %d: expected ErrCircuitOpen, got %v", i, err)
		}
	}
}

func TestResilient_ExponentialBackoff(t *testing.T) {
	config := DefaultResilientConfig()
	config.RetryBackoff = 100 * time.Millisecond
	config.MaxBackoff = 1 * time.Second

	mock := NewMockLLM(t)
	provider, _ := NewResilientProvider(mock, config)

	b1 := provider.calculateBackoff(1)
	b2 := provider.calculateBackoff(2)
	b3 := provider.calculateBackoff(3)

	if b2 <= b1 {
		t.Errorf("backoff should increase: b1=%v, b2=%v", b1, b2)
	}
	if b3 <= b2 {
		t.Errorf("backoff should increase: b2=%v, b3=%v", b2, b3)
	}

	b10 := provider.calculateBackoff(10)
	if b10 > config.MaxBackoff {
		t.Errorf("backoff should not exceed max: got %v, max %v", b10, config.MaxBackoff)
	}
}

func TestResilient_NilPrimary(t *testing.T) {
	config := DefaultResilientConfig()
	_, err := NewResilientProvider(nil, config)
	if err == nil {
		t.Fatal("expected error for nil primary, got nil")
	}
}

func TestResilient_ContextCancel(t *testing.T) {
	mock := NewMockLLM(t).
		WithError(errors.New("slow")).
		WithDelay(100 * time.Millisecond)

	config := DefaultResilientConfig()
	config.MaxRetries = 5
	config.RetryBackoff = 1 * time.Second

	provider, _ := NewResilientProvider(mock, config)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := provider.Complete(ctx, &CompletionRequest{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("should have been canceled quickly, took %v", elapsed)
	}
}

func TestResilient_NoFallbacks(t *testing.T) {
	mock := NewMockLLM(t).WithError(errors.New("only primary"))

	config := DefaultResilientConfig()
	config.MaxRetries = 1
	config.RetryBackoff = 10 * time.Millisecond

	provider, _ := NewResilientProvider(mock, config)

	_, err := provider.Complete(context.Background(), &CompletionRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResilient_CallTools(t *testing.T) {
	mock := NewMockLLM(t).WithToolResponse([]FunctionCall{
		{ID: "call_1", Name: "test", Arguments: "{}"},
	})

	provider, _ := NewResilientProvider(mock, DefaultResilientConfig())

	resp, err := provider.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
}

func TestResilient_Info(t *testing.T) {
	mock := NewMockLLM(t)
	provider, _ := NewResilientProvider(mock, DefaultResilientConfig())

	info := provider.Info()
	if info.Name != "mock-model" {
		t.Errorf("expected 'mock-model', got '%s'", info.Name)
	}
}

func TestResilient_Embeddings(t *testing.T) {
	mock := NewMockLLM(t)
	provider, _ := NewResilientProvider(mock, DefaultResilientConfig())

	embeddings, err := provider.Embeddings(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(embeddings) != 1 {
		t.Errorf("expected 1 embedding, got %d", len(embeddings))
	}
}

func TestDefaultResilientConfig(t *testing.T) {
	config := DefaultResilientConfig()

	if config.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", config.MaxRetries)
	}
	if config.CircuitThreshold != 5 {
		t.Errorf("expected CircuitThreshold=5, got %d", config.CircuitThreshold)
	}
}

func TestResilientProvider_Stream_Success(t *testing.T) {
	mock := NewMockLLM(t).WithResponse("streamed")
	provider, _ := NewResilientProvider(mock, DefaultResilientConfig())

	ch, err := provider.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var chunks []Chunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	if chunks[0].Content != "streamed" {
		t.Errorf("expected 'streamed', got '%s'", chunks[0].Content)
	}
}

func TestResilientProvider_Stream_Fallback(t *testing.T) {
	primary := &streamErrorMock{MockLLM: NewMockLLM(t), streamErr: errors.New("primary stream failed")}
	fallback := NewMockLLM(t).WithResponse("fallback stream")

	provider, _ := NewResilientProvider(primary, DefaultResilientConfig())
	provider.AddFallback(fallback)

	ch, err := provider.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var chunks []Chunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	if chunks[0].Content != "fallback stream" {
		t.Errorf("expected 'fallback stream', got '%s'", chunks[0].Content)
	}
}

func TestResilientProvider_Stream_AllFail(t *testing.T) {
	primary := &streamErrorMock{MockLLM: NewMockLLM(t), streamErr: errors.New("primary failed")}
	fallback := &streamErrorMock{MockLLM: NewMockLLM(t), streamErr: errors.New("fallback failed")}

	provider, _ := NewResilientProvider(primary, DefaultResilientConfig())
	provider.AddFallback(fallback)

	_, err := provider.Stream(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrFallbackFailed) {
		t.Errorf("expected ErrFallbackFailed, got %v", err)
	}
}

func TestResilientProvider_CallTools_Fallback(t *testing.T) {
	primary := NewMockLLM(t).WithError(errors.New("primary tool failed"))
	fallback := NewMockLLM(t).WithToolResponse([]FunctionCall{
		{ID: "fb_1", Name: "fallback_tool", Arguments: "{}"},
	})

	config := DefaultResilientConfig()
	config.MaxRetries = 1
	config.RetryBackoff = 10 * time.Millisecond

	provider, _ := NewResilientProvider(primary, config)
	provider.AddFallback(fallback)

	resp, err := provider.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "fallback_tool" {
		t.Errorf("expected 'fallback_tool', got '%s'", resp.ToolCalls[0].Name)
	}
}

func TestResilientProvider_CallTools_AllFail(t *testing.T) {
	primary := NewMockLLM(t).WithError(errors.New("primary failed"))
	fallback := NewMockLLM(t).WithError(errors.New("fallback failed"))

	config := DefaultResilientConfig()
	config.MaxRetries = 1
	config.RetryBackoff = 10 * time.Millisecond

	provider, _ := NewResilientProvider(primary, config)
	provider.AddFallback(fallback)

	_, err := provider.CallTools(context.Background(), &ToolCallRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResilientProvider_Stream_CircuitOpen(t *testing.T) {
	mock := &streamErrorMock{MockLLM: NewMockLLM(t), streamErr: errors.New("fails")}

	config := DefaultResilientConfig()
	config.MaxRetries = 0
	config.CircuitThreshold = 1

	provider, _ := NewResilientProvider(mock, config)

	_, _ = provider.Stream(context.Background(), &CompletionRequest{})

	_, err := provider.Stream(context.Background(), &CompletionRequest{})
	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestResilientProvider_CallTools_CircuitOpen(t *testing.T) {
	mock := NewMockLLM(t).WithError(errors.New("fails"))

	config := DefaultResilientConfig()
	config.MaxRetries = 0
	config.CircuitThreshold = 1

	provider, _ := NewResilientProvider(mock, config)

	_, _ = provider.CallTools(context.Background(), &ToolCallRequest{})

	_, err := provider.CallTools(context.Background(), &ToolCallRequest{})
	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestResilientProvider_AddFallback_Multiple(t *testing.T) {
	primary := NewMockLLM(t).WithError(errors.New("primary failed"))
	fb1 := NewMockLLM(t).WithError(errors.New("fb1 failed"))
	fb2 := NewMockLLM(t).WithResponse("fb2 success")

	config := DefaultResilientConfig()
	config.MaxRetries = 1
	config.RetryBackoff = 10 * time.Millisecond

	provider, _ := NewResilientProvider(primary, config)
	provider.AddFallback(fb1)
	provider.AddFallback(fb2)

	resp, err := provider.Complete(context.Background(), &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "fb2 success" {
		t.Errorf("expected 'fb2 success', got '%s'", resp.Content)
	}
}

func TestResilientProvider_Complete_RetryExhausted_NoFallback(t *testing.T) {
	mock := NewMockLLM(t).WithError(errors.New("always fails"))

	config := DefaultResilientConfig()
	config.MaxRetries = 2
	config.RetryBackoff = 10 * time.Millisecond

	provider, _ := NewResilientProvider(mock, config)

	_, err := provider.Complete(context.Background(), &CompletionRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrRetriesExhausted) {
		t.Errorf("expected ErrRetriesExhausted, got %v", err)
	}
}

func TestResilient_CircuitHalfOpen_AllowsOnlyOneProbe(t *testing.T) {
	// 使用延迟 mock 确保第一个请求在执行期间第二个请求到来
	slowMock := NewMockLLM(t).WithError(errors.New("slow")).WithDelay(500 * time.Millisecond)

	config := DefaultResilientConfig()
	config.MaxRetries = 0
	config.CircuitThreshold = 2
	config.CircuitRecoverAfter = 50 * time.Millisecond

	provider, _ := NewResilientProvider(slowMock, config)

	// 触发熔断
	for i := 0; i < 2; i++ {
		_, _ = provider.Complete(context.Background(), &CompletionRequest{})
	}

	// 确认熔断器打开
	_, err := provider.Complete(context.Background(), &CompletionRequest{})
	if err != ErrCircuitOpen {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}

	// 等待恢复时间
	time.Sleep(100 * time.Millisecond)

	// 第一个请求应该通过（半开试探），使用延迟 mock 保持在执行中
	started := make(chan struct{})
	err1 := make(chan error, 1)
	go func() {
		close(started)
		_, err := provider.Complete(context.Background(), &CompletionRequest{})
		err1 <- err
	}()

	// 等待第一个请求开始执行并进入 checkCircuit
	<-started
	// 轮询直到第二个请求被拒绝（确保第一个请求的 checkCircuit 已执行）
	var err2 error
	for i := 0; i < 50; i++ {
		_, err2 = provider.Complete(context.Background(), &CompletionRequest{})
		if err2 == ErrCircuitOpen {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err2 != ErrCircuitOpen {
		t.Errorf("expected second request in half-open to be rejected with ErrCircuitOpen, got %v", err2)
	}

	// 等待第一个请求完成
	<-err1
}
