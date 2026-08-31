// embedding_cache_test.go — S0-3 语义缓存命中率基线单测：命中/未命中计数、LRU 淘汰、
// 模型隔离、批内去重与并发安全。
package llm

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// stubEmbedder 测试桩：记录调用并返回可预期的向量。
type stubEmbedder struct {
	calls  int
	texts  []string
	failOn map[string]bool
	dim    int
	mu     sync.Mutex
}

func (s *stubEmbedder) Embeddings(_ context.Context, texts []string) ([][]float32, error) {
	s.mu.Lock()
	s.calls++
	s.texts = append(s.texts, texts...)
	s.mu.Unlock()
	out := make([][]float32, len(texts))
	for i, t := range texts {
		if s.failOn[t] {
			return nil, errors.New("stub failure: " + t)
		}
		v := make([]float32, s.dim)
		v[0] = float32(len(t))
		out[i] = v
	}
	return out, nil
}

// 编译期断言 + 元信息委托：stub 即满足 EmbeddingProvider。
func (s *stubEmbedder) Dimension() int { return s.dim }

func (s *stubEmbedder) Model() string { return "stub-model" }

func (s *stubEmbedder) Semantic() bool { return true }

var _ EmbeddingProvider = (*stubEmbedder)(nil)

// TestEmbeddingCache_HitMissCounters 命中/未命中计数与 HitRate。
func TestEmbeddingCache_HitMissCounters(t *testing.T) {
	c := NewEmbeddingCache(8)
	if _, ok := c.Get("m", "a"); ok {
		t.Fatal("空缓存不应命中")
	}
	c.Put("m", "a", []float32{1, 2})
	v, ok := c.Get("m", "a")
	if !ok || v[1] != 2 {
		t.Fatalf("命中失败: ok=%v v=%v", ok, v)
	}
	stats := c.Stats()
	if stats.Hits != 1 || stats.Misses != 1 {
		t.Fatalf("Hits/Misses = %d/%d, want 1/1", stats.Hits, stats.Misses)
	}
	if math_Abs(stats.HitRate-0.5) > 1e-9 {
		t.Fatalf("HitRate = %v, want 0.5", stats.HitRate)
	}
	// 返回的是副本：调用方突变不得污染缓存
	v[0] = 99
	again, _ := c.Get("m", "a")
	if again[0] == 99 {
		t.Fatal("Get 返回副本的约束被破坏")
	}
	if stats.Entries != 1 {
		t.Fatalf("Entries = %d, want 1", stats.Entries)
	}
}

// math_Abs 测试内联绝对值（避免只为测试引入 math 别名）。
func math_Abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// TestEmbeddingCache_LRUEviction 超限淘汰最久未用条目并计数。
func TestEmbeddingCache_LRUEviction(t *testing.T) {
	c := NewEmbeddingCache(2)
	c.Put("m", "a", []float32{1})
	c.Put("m", "b", []float32{2})
	_, _ = c.Get("m", "a") // a 变为最近使用 → b 成为 LRU
	c.Put("m", "c", []float32{3})
	if _, ok := c.Get("m", "b"); ok {
		t.Fatal("b 应被 LRU 淘汰")
	}
	if _, ok := c.Get("m", "a"); !ok {
		t.Fatal("a 不应被淘汰（刚被访问过）")
	}
	if s := c.Stats(); s.Evictions != 1 {
		t.Fatalf("Evictions = %d, want 1", s.Evictions)
	}
}

// TestEmbeddingCache_ModelKeyIsolation 不同模型同名文本互不串用。
func TestEmbeddingCache_ModelKeyIsolation(t *testing.T) {
	c := NewEmbeddingCache(8)
	c.Put("model-a", "x", []float32{1})
	if v, ok := c.Get("model-b", "x"); ok {
		t.Fatalf("不同模型不应命中: %v", v)
	}
	c.Put("model-b", "x", []float32{2})
	va, _ := c.Get("model-a", "x")
	vb, _ := c.Get("model-b", "x")
	if va[0] != 1 || vb[0] != 2 {
		t.Fatalf("模型隔离失败: a=%v b=%v", va, vb)
	}
}

// TestEmbeddingCache_Concurrent 并发读写安全（-race 下验证）。
func TestEmbeddingCache_Concurrent(t *testing.T) {
	c := NewEmbeddingCache(64)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				key := string(rune('a' + g))
				c.Put("m", key, []float32{float32(g), float32(i)})
				_, _ = c.Get("m", key)
			}
		}(g)
	}
	wg.Wait()
	if s := c.Stats(); s.Entries > 64 {
		t.Fatalf("Entries = %d 超过上限 64", s.Entries)
	}
}

// TestCachedEmbeddingProvider_HitMissAndDedup 批内去重 + 二次调用全命中。
func TestCachedEmbeddingProvider_HitMissAndDedup(t *testing.T) {
	stub := &stubEmbedder{dim: 4}
	p := NewCachedEmbeddingProvider(stub, 16)

	// 首批：a/b/c，其中 c 重复 → 远端只调用 a/b/c 三个唯一文本
	first := []string{"a", "b", "c", "c"}
	vecs, err := p.Embeddings(context.Background(), first)
	if err != nil {
		t.Fatalf("Embeddings failed: %v", err)
	}
	if stub.calls != 1 {
		t.Fatalf("远端调用次数 = %d, want 1", stub.calls)
	}
	if len(stub.texts) != 3 {
		t.Fatalf("远端唯一文本数 = %d, want 3（批内去重）", len(stub.texts))
	}
	if vecs[2][0] != 1 || vecs[3][0] != 1 {
		t.Fatalf("重复文本应取同一向量: %v", vecs)
	}

	// 二批：全命中，不触发远端
	if _, err := p.Embeddings(context.Background(), []string{"c", "b"}); err != nil {
		t.Fatalf("Embeddings(2nd) failed: %v", err)
	}
	if stub.calls != 1 {
		t.Fatalf("二次调用不应触发远端: calls = %d", stub.calls)
	}
	stats := p.CacheStats()
	// 首批 4 次查询全 miss（缓存视角），二批 2 次全 hit
	if stats.Hits != 2 || stats.Misses != 4 {
		t.Fatalf("Hits/Misses = %d/%d, want 2/4", stats.Hits, stats.Misses)
	}
	if math_Abs(stats.HitRate-1.0/3.0) > 1e-9 {
		t.Fatalf("HitRate = %v, want 1/3", stats.HitRate)
	}
}

// TestCachedEmbeddingProvider_ErrorPropagation 内层失败 → 错误透传，不缓存半成品。
func TestCachedEmbeddingProvider_ErrorPropagation(t *testing.T) {
	stub := &stubEmbedder{dim: 2, failOn: map[string]bool{"bad": true}}
	p := NewCachedEmbeddingProvider(stub, 8)

	if _, err := p.Embeddings(context.Background(), []string{"bad"}); err == nil {
		t.Fatal("内层失败应透传错误")
	}
	// 修复桩后重试：失败结果不得被缓存
	stub.failOn = nil
	vecs, err := p.Embeddings(context.Background(), []string{"bad"})
	if err != nil {
		t.Fatalf("重试失败: %v", err)
	}
	if len(vecs) != 1 || vecs[0][0] != 3 {
		t.Fatalf("重试结果意外: %v", vecs)
	}
}

// TestCachedEmbeddingProvider_Delegates 元信息委托：缓存不改变语义性。
func TestCachedEmbeddingProvider_Delegates(t *testing.T) {
	fallback := NewCachedEmbeddingProvider(NewLexicalEmbedder(), 8)
	if fallback.Semantic() {
		t.Fatal("降级位包缓存后 Semantic() 仍必须为 false")
	}
	if fallback.Model() != "lexical-fallback-v1" || fallback.Dimension() != 256 {
		t.Fatalf("委托失败: %q/%d", fallback.Model(), fallback.Dimension())
	}
}
