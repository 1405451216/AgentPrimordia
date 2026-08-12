package llm

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestInMemoryCache_ExactMatch(t *testing.T) {
	cache := NewInMemoryCacheWithFullConfig(InMemoryCacheFullConfig{
		MaxSize:  100,
		TTL:      time.Hour,
		MinScore: 0.9,
	})

	resp := &CompletionResponse{ID: "r1", Content: "hello", Usage: Usage{TotalTokens: 10}}
	_ = cache.Set(context.Background(), "what is Go", resp)

	got, ok := cache.Get(context.Background(), "what is Go", 0.9)
	if !ok {
		t.Fatal("exact match should hit")
	}
	if got.Content != "hello" {
		t.Errorf("Content = %q", got.Content)
	}
}

func TestInMemoryCache_TTL(t *testing.T) {
	cache := NewInMemoryCacheWithFullConfig(InMemoryCacheFullConfig{
		MaxSize: 100,
		TTL:     10 * time.Millisecond,
	})

	resp := &CompletionResponse{ID: "r1", Content: "ttl test"}
	_ = cache.Set(context.Background(), "query", resp)

	time.Sleep(20 * time.Millisecond)

	_, ok := cache.Get(context.Background(), "query", 0)
	if ok {
		t.Error("expired entry should miss")
	}
}

func TestInMemoryCache_ZeroTTL(t *testing.T) {
	cache := NewInMemoryCacheWithFullConfig(InMemoryCacheFullConfig{MaxSize: 100})
	resp := &CompletionResponse{ID: "r1", Content: "no ttl"}
	_ = cache.Set(context.Background(), "query", resp)

	time.Sleep(10 * time.Millisecond)

	_, ok := cache.Get(context.Background(), "query", 0)
	if !ok {
		t.Error("zero TTL means no expiration")
	}
}

func TestInMemoryCache_Invalidate(t *testing.T) {
	cache := NewInMemoryCacheWithFullConfig(InMemoryCacheFullConfig{MaxSize: 100, MinScore: 0.95})

	resp1 := &CompletionResponse{ID: "r1", Content: "keep"}
	resp2 := &CompletionResponse{ID: "r2", Content: "remove"}
	_ = cache.Set(context.Background(), "keep this message please", resp1)
	_ = cache.Set(context.Background(), "remove that item now", resp2)

	_ = cache.Invalidate(context.Background(), "remove")

	got, ok := cache.Get(context.Background(), "remove that item now", 0.95)
	if ok {
		t.Errorf("invalidated entry should miss, got Content=%q", got.Content)
	}
	got, ok = cache.Get(context.Background(), "keep this message please", 0.95)
	if !ok {
		t.Error("non-invalidated entry should remain")
	}
	if got.Content != "keep" {
		t.Errorf("Content = %q, want keep", got.Content)
	}
}

func TestInMemoryCache_InvalidateAll(t *testing.T) {
	cache := NewInMemoryCacheWithFullConfig(InMemoryCacheFullConfig{MaxSize: 100})
	_ = cache.Set(context.Background(), "a", &CompletionResponse{})
	_ = cache.Set(context.Background(), "b", &CompletionResponse{})

	_ = cache.InvalidateAll(context.Background())

	stats := cache.Stats(context.Background())
	if stats.EntryCount != 0 {
		t.Errorf("EntryCount after InvalidateAll = %d", stats.EntryCount)
	}
}

func TestFingerprintCache_Hit(t *testing.T) {
	cache := NewFingerprintCache(100, time.Hour)

	resp := &CompletionResponse{ID: "r1", Content: "fp answer"}
	_ = cache.Set(context.Background(), "what is the weather today", resp)

	got, ok := cache.Get(context.Background(), "WHAT IS THE WEATHER TODAY", 0)
	if !ok {
		t.Fatal("normalized fingerprint should hit")
	}
	if got.Content != "fp answer" {
		t.Errorf("Content = %q", got.Content)
	}
}

func TestFingerprintCache_Miss(t *testing.T) {
	cache := NewFingerprintCache(100, time.Hour)
	_, ok := cache.Get(context.Background(), "completely different query", 0)
	if ok {
		t.Error("should miss on different content")
	}
}

func TestFingerprintCache_Eviction(t *testing.T) {
	cache := NewFingerprintCache(3, time.Hour)
	for i := 0; i < 5; i++ {
		_ = cache.Set(context.Background(), string(rune('a'+i)), &CompletionResponse{})
	}
	stats := cache.Stats(context.Background())
	if stats.EntryCount > 3 {
		t.Errorf("eviction failed: %d entries", stats.EntryCount)
	}
}

func TestNoopCache_AlwaysMiss(t *testing.T) {
	cache := NoopCache{}
	_, ok := cache.Get(context.Background(), "anything", 0)
	if ok {
		t.Error("noop should always miss")
	}
	_ = cache.Set(context.Background(), "x", &CompletionResponse{})
	stats := cache.Stats(context.Background())
	if stats.TotalQueries != 0 || stats.EntryCount != 0 {
		t.Error("noop should have empty stats")
	}
}

func TestPromptFingerprint_Deterministic(t *testing.T) {
	fp1 := PromptFingerprint("Hello World")
	fp2 := PromptFingerprint("Hello World")
	if fp1 != fp2 {
		t.Error("same input must produce same fingerprint")
	}
}

func TestPromptFingerprint_CaseInsensitive(t *testing.T) {
	fp1 := PromptFingerprint("Hello World")
	fp2 := PromptFingerprint("hello world")
	if fp1 != fp2 {
		t.Error("case-insensitive normalization should produce same fp")
	}
}

func TestPromptFingerprint_DifferentInputs(t *testing.T) {
	fp1 := PromptFingerprint("What is Go?")
	fp2 := PromptFingerprint("How to cook pasta?")
	if fp1 == fp2 {
		t.Error("different inputs should have different fingerprints")
	}
}

// TestRequestFingerprint_DistinguishesSystemPrompt 验证 v6.x 评估 Issue #2 修复：
// 仅基于最后一条 user 消息的旧 PromptFingerprint 会让不同 system prompt
// 的相同 query 命中同一缓存。RequestFingerprint 必须按 system prompt
// 区分不同请求。
func TestRequestFingerprint_DistinguishesSystemPrompt(t *testing.T) {
	q := "What's the weather?"
	r1 := &CompletionRequest{
		Model: "gpt-4",
		Messages: []ChatMessage{
			{Role: "system", Content: "You are a weather expert."},
			{Role: "user", Content: q},
		},
	}
	r2 := &CompletionRequest{
		Model: "gpt-4",
		Messages: []ChatMessage{
			{Role: "system", Content: "You are a travel agent."},
			{Role: "user", Content: q},
		},
	}
	fp1 := RequestFingerprint(r1)
	fp2 := RequestFingerprint(r2)
	if fp1 == fp2 {
		t.Fatalf("system prompt must affect fingerprint: fp1=%s fp2=%s", fp1, fp2)
	}
}

// TestRequestFingerprint_DistinguishesModel 验证 model 字段参与指纹。
func TestRequestFingerprint_DistinguishesModel(t *testing.T) {
	r1 := &CompletionRequest{Model: "gpt-4", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}
	r2 := &CompletionRequest{Model: "claude-3", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}
	fp1 := RequestFingerprint(r1)
	fp2 := RequestFingerprint(r2)
	if fp1 == fp2 {
		t.Fatalf("model must affect fingerprint: fp1=%s fp2=%s", fp1, fp2)
	}
}

// TestRequestFingerprint_Stable 验证相同输入产出相同指纹（缓存命中必要条件）。
func TestRequestFingerprint_Stable(t *testing.T) {
	r := &CompletionRequest{
		Model: "gpt-4",
		Messages: []ChatMessage{
			{Role: "system", Content: "  Hello World!  "},
			{Role: "user", Content: "Question?"},
		},
	}
	fp1 := RequestFingerprint(r)
	fp2 := RequestFingerprint(r)
	if fp1 != fp2 {
		t.Fatalf("identical requests must produce identical fingerprints: %s vs %s", fp1, fp2)
	}
}

// TestFingerprintCache_RequestKey 验证 GetRequest/SetRequest 用 RequestFingerprint
// 而非 PromptFingerprint 作为 key（修复 v6.x Issue #2）。
func TestFingerprintCache_RequestKey(t *testing.T) {
	c := NewFingerprintCache(100, time.Hour)
	ctx := context.Background()

	resp1 := &CompletionResponse{ID: "r1", Content: "weather answer", Usage: Usage{TotalTokens: 5}}
	resp2 := &CompletionResponse{ID: "r2", Content: "travel answer", Usage: Usage{TotalTokens: 5}}

	r1 := &CompletionRequest{
		Model: "gpt-4",
		Messages: []ChatMessage{
			{Role: "system", Content: "weather expert"},
			{Role: "user", Content: "hello"},
		},
	}
	r2 := &CompletionRequest{
		Model: "gpt-4",
		Messages: []ChatMessage{
			{Role: "system", Content: "travel agent"},
			{Role: "user", Content: "hello"}, // 相同 user query
		},
	}

	if err := c.SetRequest(ctx, r1, resp1); err != nil {
		t.Fatalf("SetRequest r1: %v", err)
	}
	if err := c.SetRequest(ctx, r2, resp2); err != nil {
		t.Fatalf("SetRequest r2: %v", err)
	}

	// 新 request-aware API：必须分别返回 resp1 / resp2
	got1, ok1 := c.GetRequest(ctx, r1)
	if !ok1 || got1.Content != "weather answer" {
		t.Fatalf("GetRequest(r1) failed: ok=%v content=%q", ok1, got1.Content)
	}
	got2, ok2 := c.GetRequest(ctx, r2)
	if !ok2 || got2.Content != "travel answer" {
		t.Fatalf("GetRequest(r2) failed: ok=%v content=%q", ok2, got2.Content)
	}
}

func TestHybridCache_ExactHitFirst(t *testing.T) {
	fc := NewFingerprintCache(100, time.Hour)
	sc := NewInMemoryCacheWithFullConfig(InMemoryCacheFullConfig{MaxSize: 100})
	hybrid, _ := NewHybridCache(fc, sc)

	resp := &CompletionResponse{ID: "r1", Content: "from hybrid"}
	_ = hybrid.Set(context.Background(), "test query", resp)

	got, ok := hybrid.Get(context.Background(), "TEST QUERY", 0)
	if !ok {
		t.Fatal("hybrid should hit via fingerprint")
	}
	if got.Content != "from hybrid" {
		t.Errorf("Content = %q", got.Content)
	}
}

func TestHybridCache_SemanticFallback(t *testing.T) {
	fc := NewFingerprintCache(100, time.Hour)
	sc := NewInMemoryCacheWithFullConfig(InMemoryCacheFullConfig{MaxSize: 100, MinScore: 0.3})
	hybrid, _ := NewHybridCache(fc, sc)

	resp := &CompletionResponse{ID: "r1", Content: "semantic answer"}
	_ = sc.Set(context.Background(), "explain machine learning basics", resp)

	got, ok := hybrid.Get(context.Background(), "what is machine learning", 0.3)
	if !ok {
		t.Fatal("hybrid should fall back to semantic cache")
	}
	if got.Content != "semantic answer" {
		t.Errorf("Content = %q", got.Content)
	}
}

func TestCacheManager_GetWithTracer(t *testing.T) {
	tracer := &mockCacheTracer{}
	cache := NewFingerprintCache(100, time.Hour)
	mgr := NewCacheManager(CacheManagerConfig{
		Cache:   cache,
		Tracer:  tracer,
		Enabled: true,
	})

	resp := &CompletionResponse{ID: "r1", Content: "mgr test"}
	_ = mgr.Set(context.Background(), "query", resp)

	got, ok := mgr.Get(context.Background(), "query", 0)
	if !ok {
		t.Fatal("cache manager should return cached result")
	}
	if got.Content != "mgr test" {
		t.Errorf("Content = %q", got.Content)
	}
	if !tracer.spanCreated {
		t.Error("tracer should have created span")
	}
}

type mockCacheTracer struct {
	spanCreated bool
}

func (m *mockCacheTracer) StartSpan(ctx context.Context, name string) (context.Context, SpanLike) {
	m.spanCreated = true
	return ctx, &mockSpan{}
}

func (m *mockCacheTracer) EndSpan(_ context.Context, _ SpanLike, _ error) {}

type mockSpan struct{}

func (s *mockSpan) SetAttribute(key string, value any) {}

func TestCacheManager_Disabled(t *testing.T) {
	cache := NewFingerprintCache(100, time.Hour)
	mgr := NewCacheManager(CacheManagerConfig{
		Cache:   cache,
		Enabled: false,
	})

	_, ok := mgr.Get(context.Background(), "anything", 0)
	if ok {
		t.Error("disabled cache manager should always miss")
	}
}

func TestCacheManager_EnableToggle(t *testing.T) {
	cache := NewFingerprintCache(100, time.Hour)
	mgr := NewCacheManager(CacheManagerConfig{Cache: cache, Enabled: true})

	resp := &CompletionResponse{ID: "r1", Content: "toggle"}
	_ = mgr.Set(context.Background(), "q", resp)

	_, ok := mgr.Get(context.Background(), "q", 0)
	if !ok {
		t.Fatal("should hit when enabled")
	}

	mgr.Enable(false)
	_, ok = mgr.Get(context.Background(), "q", 0)
	if ok {
		t.Error("should miss when disabled")
	}

	mgr.Enable(true)
	resp2, ok := mgr.Get(context.Background(), "q", 0)
	if !ok || resp2.Content != "toggle" {
		t.Error("should hit again when re-enabled")
	}
}

func cosineSim(a, b []float32) float32 {
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / float32(math.Sqrt(float64(normA))*math.Sqrt(float64(normB)))
}

func TestSemanticVector_SimilarQueries(t *testing.T) {
	v1 := fingerprintToVector("what is the weather today")
	v2 := fingerprintToVector("whats the weather like today")
	v3 := fingerprintToVector("how to cook pasta with tomato sauce")

	sim12 := cosineSim(v1, v2)
	sim13 := cosineSim(v1, v3)

	if sim12 <= sim13 {
		t.Errorf("similar queries (%.3f) should be more similar than unrelated (%.3f)", sim12, sim13)
	}
	if sim12 < 0.3 {
		t.Errorf("weather queries should have similarity > 0.3, got %.3f", sim12)
	}
}

func TestSemanticVector_DifferentQueries(t *testing.T) {
	v1 := fingerprintToVector("write a python function")
	v2 := fingerprintToVector("explain quantum physics")

	sim := cosineSim(v1, v2)
	if sim > 0.5 {
		t.Errorf("unrelated queries should have low similarity, got %.3f", sim)
	}
}

func TestSemanticVector_EmptyInput(t *testing.T) {
	vec := fingerprintToVector("")
	if len(vec) != 64 {
		t.Errorf("empty input should return 64-dim zero vector, got len=%d", len(vec))
	}
	allZero := true
	for _, v := range vec {
		if v != 0 {
			allZero = false
			break
		}
	}
	if !allZero {
		t.Error("empty input should produce zero vector")
	}
}

func TestSemanticVector_CaseInsensitive(t *testing.T) {
	v1 := fingerprintToVector("Hello World Test")
	v2 := fingerprintToVector("hello world test")

	sim := cosineSim(v1, v2)
	if sim < 0.99 {
		t.Errorf("case-insensitive queries should be nearly identical, got %.3f", sim)
	}
}
