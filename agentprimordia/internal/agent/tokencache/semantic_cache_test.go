package tokencache

import (
	"context"
	"testing"
	"time"
)

func TestSemanticCache_ExactMatch(t *testing.T) {
	cache := NewSemanticCache(100)
	ctx := context.Background()
	resp := &ProviderResponse{Content: "Hello, world!", Model: "test"}

	cache.Store(ctx, "hello", resp, time.Minute)

	cached, ok := cache.Lookup(ctx, "hello", 0.95)
	if !ok {
		t.Fatal("expected cache hit for exact match")
	}
	if cached.Response.Content != "Hello, world!" {
		t.Errorf("unexpected content: %s", cached.Response.Content)
	}
}

func TestSemanticCache_SemanticMatch(t *testing.T) {
	cache := NewSemanticCache(100)
	ctx := context.Background()
	resp := &ProviderResponse{Content: "Paris is the capital of France", Model: "test"}

	cache.Store(ctx, "What is the capital of France?", resp, time.Minute)

	// 语义相似但措辞不同
	cached, ok := cache.Lookup(ctx, "capital of France?", 0.5)
	if !ok {
		t.Fatal("expected semantic cache hit")
	}
	if cached.Response.Content != "Paris is the capital of France" {
		t.Errorf("unexpected content: %s", cached.Response.Content)
	}
}

func TestSemanticCache_Miss(t *testing.T) {
	cache := NewSemanticCache(100)
	ctx := context.Background()

	cache.Store(ctx, "apple fruit", &ProviderResponse{Content: "about apples"}, time.Minute)

	_, ok := cache.Lookup(ctx, "car automobile vehicle", 0.95)
	if ok {
		t.Fatal("expected cache miss for unrelated query")
	}
}

func TestSemanticCache_Expire(t *testing.T) {
	cache := NewSemanticCache(100)
	ctx := context.Background()

	cache.Store(ctx, "temp", &ProviderResponse{Content: "temp"}, time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	_, ok := cache.Lookup(ctx, "temp", 0.95)
	if ok {
		t.Fatal("expected cache miss after expiration")
	}
}

func TestSemanticCache_Stats(t *testing.T) {
	cache := NewSemanticCache(100)
	ctx := context.Background()

	cache.Store(ctx, "test prompt", &ProviderResponse{Content: "test"}, time.Minute)
	cache.Lookup(ctx, "test prompt", 0.95)   // hit
	cache.Lookup(ctx, "unrelated xyz", 0.95) // miss

	stats := cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.Entries != 1 {
		t.Errorf("expected 1 entry, got %d", stats.Entries)
	}
}

func TestSemanticCache_Eviction(t *testing.T) {
	cache := NewSemanticCache(2)
	ctx := context.Background()

	cache.Store(ctx, "a", &ProviderResponse{Content: "a"}, time.Minute)
	cache.Store(ctx, "b", &ProviderResponse{Content: "b"}, time.Minute)
	cache.Store(ctx, "c", &ProviderResponse{Content: "c"}, time.Minute) // evicts oldest

	if cache.Stats().Entries > 2 {
		t.Errorf("expected max 2 entries, got %d", cache.Stats().Entries)
	}
}

func TestMultiLevelCache_L1Hit(t *testing.T) {
	mlc := NewMultiLevelCache(100, 100)
	ctx := context.Background()

	resp := &ProviderResponse{Content: "L1 hit", Model: "test"}
	mlc.Store(ctx, "exact prompt", resp, time.Minute)

	cached, ok := mlc.Lookup(ctx, "exact prompt", 0.95)
	if !ok {
		t.Fatal("expected L1 hit")
	}
	if cached.Response.Content != "L1 hit" {
		t.Errorf("unexpected content: %s", cached.Response.Content)
	}
}

func TestMultiLevelCache_L2Hit(t *testing.T) {
	mlc := NewMultiLevelCache(100, 100)
	ctx := context.Background()

	resp := &ProviderResponse{Content: "L2 hit", Model: "test"}
	mlc.Store(ctx, "What is 2+2?", resp, time.Minute)

	// 语义相似
	cached, ok := mlc.Lookup(ctx, "What is 2+2?", 0.5)
	if !ok {
		t.Fatal("expected L2 hit")
	}
	if cached.Response.Content != "L2 hit" {
		t.Errorf("unexpected content: %s", cached.Response.Content)
	}
}

func TestMultiLevelCache_Miss(t *testing.T) {
	mlc := NewMultiLevelCache(100, 100)
	ctx := context.Background()

	_, ok := mlc.Lookup(ctx, "completely new prompt", 0.95)
	if ok {
		t.Fatal("expected cache miss")
	}
}

func TestLRUCache_Basic(t *testing.T) {
	lru := NewLRUCache(2)
	resp := &ProviderResponse{Content: "test"}

	lru.Put("a", resp, time.Minute)
	got, ok := lru.Get("a")
	if !ok || got.Content != "test" {
		t.Fatal("expected LRU hit")
	}

	// 超出容量
	lru.Put("b", &ProviderResponse{Content: "b"}, time.Minute)
	lru.Put("c", &ProviderResponse{Content: "c"}, time.Minute)

	_, ok = lru.Get("a")
	if ok {
		t.Fatal("expected oldest entry evicted")
	}
	if lru.Len() != 2 {
		t.Errorf("expected len 2, got %d", lru.Len())
	}
}
