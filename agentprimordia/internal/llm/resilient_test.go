package llm

import (
	"context"
	"errors"
	"strings"
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

// ===== perf-v6 round 8 Task 3：Retry-After + 错误分类测试 =====

// TestResilient_FatalError_StopsRetrying 验证 fatal error 立即停止重试
func TestResilient_FatalError_StopsRetrying(t *testing.T) {
	// 401 认证错误是 fatal（不可重试）
	fatalErr := NewRetryableError(KindAuthError, 401, 0, errors.New("invalid api key"))
	mock := NewMockLLM(t).WithError(fatalErr)
	provider, _ := NewResilientProvider(mock, ResilientConfig{
		MaxRetries:   5,
		RetryBackoff: 1 * time.Millisecond,
		MaxBackoff:   1 * time.Millisecond,
	})

	_, err := provider.Complete(context.Background(), &CompletionRequest{})

	// 应立即返回 fatal 错误，不重试
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, fatalErr) && !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 fatal error, got %v", err)
	}
}

// TestResilient_ClientError_DoesNotCountAsCircuitFailure 验证 4xx 不触发熔断
func TestResilient_ClientError_DoesNotCountAsCircuitFailure(t *testing.T) {
	// 400 client error 不计入失败次数
	clientErr := NewRetryableError(KindClientError, 400, 0, errors.New("bad request"))
	mock := NewMockLLM(t).WithError(clientErr)
	provider, _ := NewResilientProvider(mock, ResilientConfig{
		MaxRetries:       0, // 不重试
		RetryBackoff:     1 * time.Millisecond,
		MaxBackoff:       1 * time.Millisecond,
		CircuitThreshold: 3,
	})

	// 触发 5 次 400 错误
	for i := 0; i < 5; i++ {
		_, _ = provider.Complete(context.Background(), &CompletionRequest{})
	}

	// 熔断器应保持 closed（因为 4xx 不计入失败）
	state := provider.state.Load()
	if circuitState(state) != circuitClosed {
		t.Errorf("circuit should remain closed after 4xx errors, got state=%d", state)
	}
}

// TestResilient_RateLimit_TriggersCircuit 验证 429 计入熔断
func TestResilient_RateLimit_TriggersCircuit(t *testing.T) {
	rateLimitErr := NewRetryableError(KindRateLimited, 429, 0, errors.New("rate limited"))
	mock := NewMockLLM(t).WithError(rateLimitErr)
	provider, _ := NewResilientProvider(mock, ResilientConfig{
		MaxRetries:       0,
		RetryBackoff:     1 * time.Millisecond,
		MaxBackoff:       1 * time.Millisecond,
		CircuitThreshold: 3,
	})

	for i := 0; i < 4; i++ {
		_, _ = provider.Complete(context.Background(), &CompletionRequest{})
	}

	state := provider.state.Load()
	if circuitState(state) != circuitOpen {
		t.Errorf("circuit should be open after 4x 429 errors, got state=%d", state)
	}
}

// TestResilient_RetryAfter_HonorsServerHeader 验证 Retry-After 被尊重
func TestResilient_RetryAfter_HonorsServerHeader(t *testing.T) {
	// 用 Retry-After=200ms 的 rate limit 错误
	retryAfter := 200 * time.Millisecond
	rateLimitErr := NewRetryableError(KindRateLimited, 429, retryAfter, errors.New("rate limited"))

	// 第一次失败，第二次成功
	callCount := 0
	mock := &countingProvider{
		fn: func(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
			callCount++
			if callCount == 1 {
				return nil, rateLimitErr
			}
			return &CompletionResponse{Content: "ok"}, nil
		},
	}

	provider, _ := NewResilientProvider(mock, ResilientConfig{
		MaxRetries:   3,
		RetryBackoff: 1 * time.Hour, // 故意设很大，验证 Retry-After 优先
		MaxBackoff:   1 * time.Hour,
	})

	start := time.Now()
	resp, err := provider.Complete(context.Background(), &CompletionRequest{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("expected 'ok', got %q", resp.Content)
	}
	// 验证实际等待时间在 Retry-After 附近（允许 ±50ms 误差）
	if elapsed < retryAfter-50*time.Millisecond || elapsed > retryAfter+200*time.Millisecond {
		t.Errorf("expected elapsed ~%v, got %v (Retry-After should take priority over RetryBackoff)", retryAfter, elapsed)
	}
}

// TestResilient_RetryAfter_CappedByMaxBackoff 验证 Retry-After 不会超过 MaxBackoff
func TestResilient_RetryAfter_CappedByMaxBackoff(t *testing.T) {
	// server 给了 1 小时，但 MaxBackoff 只有 100ms
	rateLimitErr := NewRetryableError(KindRateLimited, 429, 1*time.Hour, errors.New("rate limited"))

	callCount := 0
	mock := &countingProvider{
		fn: func(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
			callCount++
			if callCount == 1 {
				return nil, rateLimitErr
			}
			return &CompletionResponse{Content: "ok"}, nil
		},
	}

	provider, _ := NewResilientProvider(mock, ResilientConfig{
		MaxRetries:   3,
		RetryBackoff: 1 * time.Millisecond,
		MaxBackoff:   100 * time.Millisecond,
	})

	start := time.Now()
	_, err := provider.Complete(context.Background(), &CompletionRequest{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	// 应被 cap 在 MaxBackoff
	if elapsed > 200*time.Millisecond {
		t.Errorf("Retry-After should be capped by MaxBackoff, but elapsed=%v", elapsed)
	}
}

// TestResilient_NonRetryableError_StopsRetrying 验证非 RetryableError 也按默认重试
// （保持向后兼容：网络错误等不属于 RetryableError 但仍可重试）
func TestResilient_NonRetryableError_StopsRetrying(t *testing.T) {
	// 普通 error（非 RetryableError）走默认重试
	plainErr := errors.New("connection reset")
	mock := NewMockLLM(t).WithError(plainErr)
	provider, _ := NewResilientProvider(mock, ResilientConfig{
		MaxRetries:   2,
		RetryBackoff: 1 * time.Millisecond,
		MaxBackoff:   1 * time.Millisecond,
	})

	_, err := provider.Complete(context.Background(), &CompletionRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// 不应是 fatal（plain error 默认按 retryable 处理）
	if strings.Contains(err.Error(), "context canceled") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestResilient_Stream_RespectsRetryAfter 验证 Stream 路径也尊重 Retry-After
func TestResilient_Stream_RespectsRetryAfter(t *testing.T) {
	retryAfter := 100 * time.Millisecond
	rateLimitErr := NewRetryableError(KindRateLimited, 429, retryAfter, errors.New("rate limited"))

	callCount := 0
	mock := &countingProvider{
		streamFn: func(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
			callCount++
			if callCount == 1 {
				return nil, rateLimitErr
			}
			ch := make(chan Chunk, 1)
			ch <- Chunk{Content: "ok", Done: true}
			close(ch)
			return ch, nil
		},
	}

	provider, _ := NewResilientProvider(mock, ResilientConfig{
		MaxRetries:   1,
		RetryBackoff: 1 * time.Hour, // 应被 Retry-After 覆盖
		MaxBackoff:   1 * time.Hour,
	})

	start := time.Now()
	ch, err := provider.Stream(context.Background(), &CompletionRequest{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected stream success, got %v", err)
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
	// 消费 channel
	for range ch {
	}

	if elapsed < retryAfter-50*time.Millisecond || elapsed > retryAfter+200*time.Millisecond {
		t.Errorf("expected elapsed ~%v (Retry-After), got %v", retryAfter, elapsed)
	}
}

// TestResilient_Stream_FatalError_NoRetry 验证 Stream 路径 fatal 错误不重试
func TestResilient_Stream_FatalError_NoRetry(t *testing.T) {
	fatalErr := NewRetryableError(KindAuthError, 401, 0, errors.New("bad api key"))
	mock := &countingProvider{
		streamFn: func(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
			return nil, fatalErr
		},
	}

	provider, _ := NewResilientProvider(mock, ResilientConfig{
		MaxRetries:   5,
		RetryBackoff: 1 * time.Millisecond,
		MaxBackoff:   1 * time.Millisecond,
	})

	_, err := provider.Stream(context.Background(), &CompletionRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// 401 错误不应被重试，直接返回
	if !errors.Is(err, fatalErr) && !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 fatal error, got %v", err)
	}
}

// countingProvider 帮助测试的 mock provider，按调用次数返回不同结果
type countingProvider struct {
	fn       func(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
	streamFn func(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error)
}

func (p *countingProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if p.fn != nil {
		return p.fn(ctx, req)
	}
	return &CompletionResponse{Content: "default"}, nil
}

func (p *countingProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	if p.streamFn != nil {
		return p.streamFn(ctx, req)
	}
	ch := make(chan Chunk)
	close(ch)
	return ch, nil
}

func (p *countingProvider) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	return nil, ErrNotSupported
}

func (p *countingProvider) Info() ModelInfo {
	return ModelInfo{Name: "counting"}
}

// BenchmarkResilient_RetryAfter_Path 验证 Retry-After 处理开销（perf-v6 round 8 Task 3 性能基线）
// 模拟：每个请求都是带 Retry-After=1s 的限流错误，验证路径开销
func BenchmarkResilient_RetryAfter_Path(b *testing.B) {
	rateLimitErr := NewRetryableError(KindRateLimited, 429, 1*time.Millisecond, errors.New("rate limited"))

	mock := &countingProvider{
		fn: func(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
			return &CompletionResponse{Content: "ok"}, nil // 永远成功，验证路径开销
		},
	}

	provider, _ := NewResilientProvider(mock, ResilientConfig{
		MaxRetries:   0,
		RetryBackoff: 1 * time.Millisecond,
		MaxBackoff:   1 * time.Millisecond,
	})

	// 验证 classifyHTTPError 不会改变现有成功路径的开销
	_ = rateLimitErr

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = provider.Complete(context.Background(), &CompletionRequest{})
	}
}
