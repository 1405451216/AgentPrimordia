package llm

import (
	"context"
	"testing"
	"time"
)

// mockProvider 用于测试的简单 Provider
type mockProvider struct {
	completeResp *CompletionResponse
	completeErr  error
	callCount    int
}

func (m *mockProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	m.callCount++
	return m.completeResp, m.completeErr
}

func (m *mockProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	return nil, nil
}

func (m *mockProvider) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	return nil, nil
}

func (m *mockProvider) Info() ModelInfo {
	return ModelInfo{Name: "test-model"}
}

func TestRateLimitedProvider_Basic(t *testing.T) {
	mock := &mockProvider{
		completeResp: &CompletionResponse{Content: "test"},
	}

	cfg := RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         5,
		MaxWait:           100 * time.Millisecond,
	}

	rl, err := NewRateLimitedProvider(mock, cfg)
	if err != nil {
		t.Fatalf("创建速率限制器失败: %v", err)
	}

	// 突发 5 个请求应该立即成功
	for i := 0; i < 5; i++ {
		resp, err := rl.Complete(context.Background(), &CompletionRequest{})
		if err != nil {
			t.Errorf("请求 %d 失败: %v", i, err)
		}
		if resp.Content != "test" {
			t.Errorf("响应内容错误: %v", resp.Content)
		}
	}

	// 第 6 个请求应该等待或被拒绝
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err = rl.Complete(ctx, &CompletionRequest{})
	if err == nil {
		t.Error("应该被速率限制")
	}
}

func TestRateLimitedProvider_WaitForToken(t *testing.T) {
	mock := &mockProvider{
		completeResp: &CompletionResponse{Content: "test"},
	}

	cfg := RateLimitConfig{
		RequestsPerSecond: 100, // 每秒 100 个，即每 10ms 一个
		BurstSize:         1,
		MaxWait:           50 * time.Millisecond,
	}

	rl, err := NewRateLimitedProvider(mock, cfg)
	if err != nil {
		t.Fatalf("创建速率限制器失败: %v", err)
	}

	// 第一个请求立即成功
	_, err = rl.Complete(context.Background(), &CompletionRequest{})
	if err != nil {
		t.Errorf("第一个请求失败: %v", err)
	}

	// 第二个请求应该等待约 10ms 后成功
	start := time.Now()
	_, err = rl.Complete(context.Background(), &CompletionRequest{})
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("第二个请求失败: %v", err)
	}
	if elapsed < 5*time.Millisecond {
		t.Errorf("等待时间过短: %v", elapsed)
	}
	if elapsed > 30*time.Millisecond {
		t.Errorf("等待时间过长: %v", elapsed)
	}
}

func TestRateLimitedProvider_ContextCancel(t *testing.T) {
	mock := &mockProvider{
		completeResp: &CompletionResponse{Content: "test"},
	}

	cfg := RateLimitConfig{
		RequestsPerSecond: 1, // 每秒 1 个
		BurstSize:         1,
		MaxWait:           1 * time.Second,
	}

	rl, err := NewRateLimitedProvider(mock, cfg)
	if err != nil {
		t.Fatalf("创建速率限制器失败: %v", err)
	}

	// 消耗令牌
	_, _ = rl.Complete(context.Background(), &CompletionRequest{})

	// 使用已取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = rl.Complete(ctx, &CompletionRequest{})
	if err == nil {
		t.Error("应该返回上下文取消错误")
	}
}

func TestRateLimitedProvider_Concurrent(t *testing.T) {
	mock := &mockProvider{
		completeResp: &CompletionResponse{Content: "test"},
	}

	cfg := RateLimitConfig{
		RequestsPerSecond: 1000,
		BurstSize:         100,
		MaxWait:           100 * time.Millisecond,
	}

	rl, err := NewRateLimitedProvider(mock, cfg)
	if err != nil {
		t.Fatalf("创建速率限制器失败: %v", err)
	}

	done := make(chan error, 50)
	for i := 0; i < 50; i++ {
		go func() {
			_, err := rl.Complete(context.Background(), &CompletionRequest{})
			done <- err
		}()
	}

	successCount := 0
	for i := 0; i < 50; i++ {
		if err := <-done; err == nil {
			successCount++
		}
	}

	// 至少应该有部分请求成功
	if successCount == 0 {
		t.Error("所有请求都失败了")
	}
	t.Logf("成功请求数: %d/50", successCount)
}

func TestRateLimitedProvider_Info(t *testing.T) {
	mock := &mockProvider{}

	cfg := DefaultRateLimitConfig()
	rl, err := NewRateLimitedProvider(mock, cfg)
	if err != nil {
		t.Fatalf("创建速率限制器失败: %v", err)
	}

	info := rl.Info()
	if info.Name != "test-model" {
		t.Errorf("模型名称错误: %v", info.Name)
	}
}

func TestRateLimitedProvider_InvalidConfig(t *testing.T) {
	mock := &mockProvider{}

	// 测试无效配置使用默认值
	cfg := RateLimitConfig{
		RequestsPerSecond: 0,
		BurstSize:         0,
		MaxWait:           0,
	}

	rl, err := NewRateLimitedProvider(mock, cfg)
	if err != nil {
		t.Fatalf("创建速率限制器失败: %v", err)
	}

	if rl.config.RequestsPerSecond != DefaultRateLimitConfig().RequestsPerSecond {
		t.Error("应该使用默认的 RequestsPerSecond")
	}
	if rl.config.BurstSize != DefaultRateLimitConfig().BurstSize {
		t.Error("应该使用默认的 BurstSize")
	}
	if rl.config.MaxWait != DefaultRateLimitConfig().MaxWait {
		t.Error("应该使用默认的 MaxWait")
	}
}

func TestRateLimitedProvider_NilProvider(t *testing.T) {
	cfg := DefaultRateLimitConfig()
	_, err := NewRateLimitedProvider(nil, cfg)
	if err == nil {
		t.Error("应该返回错误")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	bucket := newTokenBucket(100, 10) // 每秒 100 个，突发 10

	// 消耗所有令牌
	for i := 0; i < 10; i++ {
		if !bucket.allow() {
			t.Errorf("第 %d 个令牌应该可用", i)
		}
	}

	// 应该没有令牌了
	if bucket.allow() {
		t.Error("不应该有令牌")
	}

	// 等待 50ms，应该补充约 5 个令牌
	time.Sleep(50 * time.Millisecond)

	tokens := bucket.Tokens()
	if tokens < 3 || tokens > 7 {
		t.Errorf("补充的令牌数不合理: %v", tokens)
	}
}

func TestRateLimitedProvider_AvailableTokens(t *testing.T) {
	mock := &mockProvider{
		completeResp: &CompletionResponse{Content: "test"},
	}

	cfg := RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         5,
		MaxWait:           100 * time.Millisecond,
	}

	rl, err := NewRateLimitedProvider(mock, cfg)
	if err != nil {
		t.Fatalf("创建速率限制器失败: %v", err)
	}

	// 初始应该有 5 个令牌
	tokens := rl.AvailableTokens()
	if tokens < 4.9 || tokens > 5.1 {
		t.Errorf("初始令牌数错误: %v", tokens)
	}

	// 消耗一个令牌
	_, _ = rl.Complete(context.Background(), &CompletionRequest{})

	tokens = rl.AvailableTokens()
	if tokens < 3.9 || tokens > 4.1 {
		t.Errorf("消耗后令牌数错误: %v", tokens)
	}
}
