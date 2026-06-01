//go:build sqlite
// +build sqlite

package llm

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestCache(t *testing.T) *SQLiteCache {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cache.db")
	cache, err := NewSQLiteCache(path)
	if err != nil {
		t.Fatalf("create cache: %v", err)
	}
	t.Cleanup(func() { cache.Close() })
	return cache
}

func newTestCacheWithConfig(t *testing.T, cfg SQLiteCacheConfig) *SQLiteCache {
	t.Helper()
	if cfg.DSN == "" {
		cfg.DSN = filepath.Join(t.TempDir(), "cache.db")
	}
	cache, err := NewSQLiteCacheWithConfig(cfg)
	if err != nil {
		t.Fatalf("create cache: %v", err)
	}
	t.Cleanup(func() { cache.Close() })
	return cache
}

func TestSQLiteCache_SetGet(t *testing.T) {
	cache := newTestCache(t)

	resp := &CompletionResponse{ID: "r1", Content: "hello from sqlite"}
	err := cache.Set(context.Background(), "test query", resp)
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	got, ok := cache.Get(context.Background(), "test query", 0)
	if !ok {
		t.Fatal("should hit")
	}
	if got.Content != "hello from sqlite" {
		t.Errorf("Content = %q", got.Content)
	}
}

func TestSQLiteCache_Miss(t *testing.T) {
	cache := newTestCache(t)

	_, ok := cache.Get(context.Background(), "nonexistent", 0)
	if ok {
		t.Error("should miss for nonexistent key")
	}
}

func TestSQLiteCache_TTL(t *testing.T) {
	cache := newTestCacheWithConfig(t, SQLiteCacheConfig{
		TTL: 2 * time.Second,
	})

	resp := &CompletionResponse{ID: "r1", Content: "expiring"}
	_ = cache.Set(context.Background(), "ttl query", resp)

	got, ok := cache.Get(context.Background(), "ttl query", 0)
	if !ok {
		t.Fatal("should hit before TTL")
	}
	if got.Content != "expiring" {
		t.Errorf("Content = %q", got.Content)
	}

	time.Sleep(2500 * time.Millisecond)
	_, ok = cache.Get(context.Background(), "ttl query", 0)
	if ok {
		t.Error("should miss after TTL")
	}
}

func TestSQLiteCache_CaseInsensitive(t *testing.T) {
	cache := newTestCache(t)

	resp := &CompletionResponse{ID: "r1", Content: "case test"}
	_ = cache.Set(context.Background(), "Hello World", resp)

	got, ok := cache.Get(context.Background(), "hello world", 0)
	if !ok {
		t.Fatal("fingerprint match should be case insensitive")
	}
	if got.Content != "case test" {
		t.Errorf("Content = %q", got.Content)
	}
}

func TestSQLiteCache_Invalidate(t *testing.T) {
	cache := newTestCache(t)

	resp := &CompletionResponse{ID: "r1", Content: "to remove"}
	_ = cache.Set(context.Background(), "remove me", resp)

	cache.Invalidate(context.Background(), "remove me")
	_, ok := cache.Get(context.Background(), "remove me", 0)
	if ok {
		t.Error("should miss after invalidation")
	}
}

func TestSQLiteCache_Clear(t *testing.T) {
	cache := newTestCache(t)

	_ = cache.Set(context.Background(), "q1", &CompletionResponse{ID: "r1", Content: "a1"})
	_ = cache.Set(context.Background(), "q2", &CompletionResponse{ID: "r2", Content: "a2"})

	cache.Clear(context.Background())

	_, ok1 := cache.Get(context.Background(), "q1", 0)
	_, ok2 := cache.Get(context.Background(), "q2", 0)
	if ok1 || ok2 {
		t.Error("should miss after clear")
	}
}

func TestSQLiteCache_Stats(t *testing.T) {
	cache := newTestCache(t)

	_ = cache.Set(context.Background(), "q1", &CompletionResponse{ID: "r1", Content: "a1"})

	cache.Get(context.Background(), "q1", 0)
	cache.Get(context.Background(), "miss", 0)

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
	if stats.EntryCount != 1 {
		t.Errorf("EntryCount = %d, want 1", stats.EntryCount)
	}
}

func TestSQLiteCache_Eviction(t *testing.T) {
	cache := newTestCacheWithConfig(t, SQLiteCacheConfig{
		MaxSize: 3,
	})

	for i := 0; i < 5; i++ {
		_ = cache.Set(context.Background(), "query"+string(rune('0'+i)),
			&CompletionResponse{ID: "r" + string(rune('0'+i)), Content: "a"})
	}

	stats := cache.Stats(context.Background())
	if stats.EntryCount > 3 {
		t.Errorf("EntryCount = %d, should not exceed maxSize 3", stats.EntryCount)
	}
}

func TestSQLiteCache_SemanticMatch(t *testing.T) {
	cache := newTestCacheWithConfig(t, SQLiteCacheConfig{
		MinScore:  0.3,
		EnableSem: true,
	})

	_ = cache.Set(context.Background(), "explain machine learning basics",
		&CompletionResponse{ID: "r1", Content: "ML answer"})

	got, ok := cache.Get(context.Background(), "what is machine learning", 0.3)
	if !ok {
		t.Fatal("should hit via semantic match")
	}
	if got.Content != "ML answer" {
		t.Errorf("Content = %q", got.Content)
	}
}
