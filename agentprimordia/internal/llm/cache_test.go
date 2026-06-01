package llm

import (
	"context"
	"sync"
	"testing"
)

func TestInMemoryCache_SetGet(t *testing.T) {
	cache := NewInMemoryCache(dummyEmbedder, 100, 0.8)

	resp := &CompletionResponse{
		ID:      "resp-1",
		Content: "cached answer",
		Usage:   Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
	}

	err := cache.Set(context.Background(), "hello world", resp)
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	got, ok := cache.Get(context.Background(), "hello world", 0.8)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Content != "cached answer" {
		t.Errorf("Content = %q, want %q", got.Content, "cached answer")
	}
}

func TestInMemoryCache_Miss(t *testing.T) {
	cache := NewInMemoryCache(dummyEmbedder, 100, 0.8)

	_, ok := cache.Get(context.Background(), "nonexistent query", 0.8)
	if ok {
		t.Error("expected cache miss")
	}
}

func TestInMemoryCache_SemanticMatch(t *testing.T) {
	cache := NewInMemoryCache(dummyEmbedder, 100, 0.7)

	resp := &CompletionResponse{
		ID:      "resp-1",
		Content: "Go is a statically typed language",
		Usage:   Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
	}

	err := cache.Set(context.Background(), "what is Go programming language", resp)
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	got, ok := cache.Get(context.Background(), "tell me about Go language", 0.7)
	if !ok {
		t.Fatal("expected semantic cache hit for similar query")
	}
	if got.Content != "Go is a statically typed language" {
		t.Errorf("Content = %q, want %q", got.Content, "Go is a statically typed language")
	}
}

func TestInMemoryCache_LowSimilarity(t *testing.T) {
	cache := NewInMemoryCache(dummyEmbedder, 100, 0.95)

	resp := &CompletionResponse{
		ID:      "resp-1",
		Content: "Paris is the capital of France",
		Usage:   Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
	}

	err := cache.Set(context.Background(), "what is the capital of France", resp)
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}

	_, ok := cache.Get(context.Background(), "how to cook pasta", 0.95)
	if ok {
		t.Error("expected cache miss for dissimilar query with high threshold")
	}
}

func TestInMemoryCache_Eviction(t *testing.T) {
	cache := NewInMemoryCache(dummyEmbedder, 3, 0.8)

	for i := 0; i < 5; i++ {
		resp := &CompletionResponse{ID: "resp", Content: "answer"}
		err := cache.Set(context.Background(), "query "+string(rune('a'+i)), resp)
		if err != nil {
			t.Fatalf("Set error: %v", err)
		}
	}

	stats := cache.Stats(context.Background())
	if stats.EntryCount > 3 {
		t.Errorf("EntryCount = %d, want <= 3 after eviction", stats.EntryCount)
	}
}

func TestInMemoryCache_Clear(t *testing.T) {
	cache := NewInMemoryCache(dummyEmbedder, 100, 0.8)

	resp := &CompletionResponse{ID: "resp", Content: "answer"}
	_ = cache.Set(context.Background(), "query", resp)

	cache.Clear(context.Background())

	stats := cache.Stats(context.Background())
	if stats.EntryCount != 0 {
		t.Errorf("EntryCount = %d after Clear, want 0", stats.EntryCount)
	}
}

func TestInMemoryCache_Stats(t *testing.T) {
	cache := NewInMemoryCache(dummyEmbedder, 100, 0.8)

	resp := &CompletionResponse{ID: "resp", Content: "answer", Usage: Usage{TotalTokens: 50}}
	_ = cache.Set(context.Background(), "query", resp)

	cache.Get(context.Background(), "query", 0.8)
	cache.Get(context.Background(), "miss", 0.8)

	stats := cache.Stats(context.Background())
	if stats.TotalQueries != 2 {
		t.Errorf("TotalQueries = %d, want 2", stats.TotalQueries)
	}
	if stats.CacheHits != 1 {
		t.Errorf("CacheHits = %d, want 1", stats.CacheHits)
	}
	if stats.CacheMisses != 1 {
		t.Errorf("CacheMisses = %d, want 1", stats.CacheMisses)
	}
	if stats.TokensSaved != 50 {
		t.Errorf("TokensSaved = %d, want 50", stats.TokensSaved)
	}
}

func TestInMemoryCache_Concurrent(t *testing.T) {
	cache := NewInMemoryCache(dummyEmbedder, 100, 0.8)
	resp := &CompletionResponse{ID: "resp", Content: "answer"}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = cache.Set(context.Background(), "query", resp)
			cache.Get(context.Background(), "query", 0.8)
		}(i)
	}
	wg.Wait()
}

func TestCachedProvider_CacheHit(t *testing.T) {
	inner := &mockProviderForCache{
		response: &CompletionResponse{ID: "resp", Content: "real answer", Usage: Usage{TotalTokens: 100}},
	}
	cache := NewInMemoryCache(dummyEmbedder, 100, 0.8)

	cached, err := NewCachedProvider(inner, cache, 0.8)
	if err != nil {
		t.Fatalf("NewCachedProvider error: %v", err)
	}

	req := &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	}

	resp1, err := cached.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if resp1.Content != "real answer" {
		t.Errorf("Content = %q, want %q", resp1.Content, "real answer")
	}
	if inner.callCount != 1 {
		t.Errorf("inner callCount = %d after first call, want 1", inner.callCount)
	}

	resp2, err := cached.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	if resp2.Content != "real answer" {
		t.Errorf("Content = %q, want %q", resp2.Content, "real answer")
	}
	if inner.callCount != 1 {
		t.Errorf("inner callCount = %d after cache hit, want 1 (should not call inner again)", inner.callCount)
	}
}

func TestCachedProvider_CacheMiss(t *testing.T) {
	inner := &mockProviderForCache{
		response: &CompletionResponse{ID: "resp", Content: "answer1"},
	}
	cache := NewInMemoryCache(dummyEmbedder, 100, 0.95)

	cached, err := NewCachedProvider(inner, cache, 0.95)
	if err != nil {
		t.Fatalf("NewCachedProvider error: %v", err)
	}

	req1 := &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "query about Go"}},
	}
	req2 := &CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "query about cooking pasta"}},
	}

	_, _ = cached.Complete(context.Background(), req1)
	_, _ = cached.Complete(context.Background(), req2)

	if inner.callCount != 2 {
		t.Errorf("inner callCount = %d, want 2 (both should be cache misses)", inner.callCount)
	}
}

func TestCachedProvider_Info(t *testing.T) {
	inner := &mockProviderForCache{
		info: ModelInfo{Name: "test-model", Provider: "test"},
	}
	cache := NewInMemoryCache(dummyEmbedder, 100, 0.8)
	cached, err := NewCachedProvider(inner, cache, 0.8)
	if err != nil {
		t.Fatalf("NewCachedProvider error: %v", err)
	}

	info := cached.Info()
	if info.Name != "test-model" {
		t.Errorf("Info().Name = %q, want %q", info.Name, "test-model")
	}
}

func dummyEmbedder(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, 16)
	for i, r := range text {
		idx := i % 16
		vec[idx] += float32(r) * float32(i+1)
	}
	norm := float32(0)
	for _, v := range vec {
		norm += v * v
	}
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec, nil
}

type mockProviderForCache struct {
	response  *CompletionResponse
	info      ModelInfo
	callCount int
	mu        sync.Mutex
}

func (m *mockProviderForCache) Complete(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	return m.response, nil
}

func (m *mockProviderForCache) Stream(_ context.Context, _ *CompletionRequest) (<-chan Chunk, error) {
	ch := make(chan Chunk, 1)
	ch <- Chunk{Content: m.response.Content, Done: true}
	close(ch)
	return ch, nil
}

func (m *mockProviderForCache) CallTools(_ context.Context, _ *ToolCallRequest) (*ToolCallResponse, error) {
	return &ToolCallResponse{Content: m.response.Content}, nil
}

func (m *mockProviderForCache) Embeddings(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = make([]float32, 8)
	}
	return result, nil
}

func (m *mockProviderForCache) Info() ModelInfo {
	return m.info
}
