// embedding_cache.go — S0-3 语义缓存命中率基线：嵌入结果 LRU 缓存 + 命中/未命中计数。
//
// 验收口径（docs/V7路线图.md §二 S0-3「语义缓存命中率基线」）：
//   - 命中/未命中为可观测计数（EmbeddingCache.Stats），供基准与运行时导出；
//   - 缓存 key = 模型名 + 分隔符 + 原文：不同模型互不串用；
//   - CachedEmbeddingProvider 为装饰器：批量请求按条缓存，仅对未命中子集发起
//     远端调用，返回值按原顺序拼装（批内重复文本去重，只远端调用一次）。
package llm

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// defaultEmbeddingCacheMaxEntries 默认缓存条目上限。
const defaultEmbeddingCacheMaxEntries = 1024

// embeddingCacheKeySep 缓存 key 分隔符（ASCII 单元分隔符 US，避免与文本内容拼接歧义）。
const embeddingCacheKeySep = "\x1f"

// EmbeddingCacheStats 缓存观测快照（S0-3 语义缓存命中率基线的导出形状）。
type EmbeddingCacheStats struct {
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	Evictions int64 `json:"evictions"`
	Entries   int   `json:"entries"`
	// HitRate = Hits/(Hits+Misses)；尚无请求时为 0。
	HitRate float64 `json:"hit_rate"`
}

// embeddingCacheEntry 缓存条目（存入时深拷贝，防外部突变）。
type embeddingCacheEntry struct {
	key string
	vec []float32
}

// EmbeddingCache 嵌入结果 LRU 缓存（container/list + map，O(1) 读写）。
type EmbeddingCache struct {
	maxEntries int
	mu         sync.Mutex
	ll         *list.List // 队首 = 最近使用
	entries    map[string]*list.Element
	hits       atomic.Int64
	misses     atomic.Int64
	evictions  atomic.Int64
}

// NewEmbeddingCache 创建缓存；maxEntries <= 0 时取默认上限。
func NewEmbeddingCache(maxEntries int) *EmbeddingCache {
	if maxEntries <= 0 {
		maxEntries = defaultEmbeddingCacheMaxEntries
	}
	return &EmbeddingCache{
		maxEntries: maxEntries,
		entries:    make(map[string]*list.Element, 64),
		ll:         list.New(),
	}
}

func embeddingCacheKey(model, text string) string {
	return model + embeddingCacheKeySep + text
}

// Get 查询缓存；命中返回向量副本并移到队首。
func (c *EmbeddingCache) Get(model, text string) ([]float32, bool) {
	key := embeddingCacheKey(model, text)
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[key]
	if !ok {
		c.misses.Add(1)
		return nil, false
	}
	c.ll.MoveToFront(el)
	c.hits.Add(1)
	src := el.Value.(*embeddingCacheEntry).vec
	dst := make([]float32, len(src))
	copy(dst, src)
	return dst, true
}

// Put 写入缓存（拷贝存储）；超限时从队尾淘汰 LRU 条目。
func (c *EmbeddingCache) Put(model, text string, vec []float32) {
	key := embeddingCacheKey(model, text)
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		// 已存在：覆盖并移到队首（不产生淘汰计数）
		c.ll.MoveToFront(el)
		dst := make([]float32, len(vec))
		copy(dst, vec)
		el.Value.(*embeddingCacheEntry).vec = dst
		return
	}
	dst := make([]float32, len(vec))
	copy(dst, vec)
	c.entries[key] = c.ll.PushFront(&embeddingCacheEntry{key: key, vec: dst})
	for len(c.entries) > c.maxEntries {
		back := c.ll.Back()
		if back == nil {
			break
		}
		c.ll.Remove(back)
		delete(c.entries, back.Value.(*embeddingCacheEntry).key)
		c.evictions.Add(1)
	}
}

// Stats 返回观测快照。
func (c *EmbeddingCache) Stats() EmbeddingCacheStats {
	hits := c.hits.Load()
	misses := c.misses.Load()
	var rate float64
	if total := hits + misses; total > 0 {
		rate = float64(hits) / float64(total)
	}
	c.mu.Lock()
	entries := len(c.entries)
	c.mu.Unlock()
	return EmbeddingCacheStats{
		Hits:      hits,
		Misses:    misses,
		Evictions: c.evictions.Load(),
		Entries:   entries,
		HitRate:   rate,
	}
}

// CachedEmbeddingProvider EmbeddingProvider 装饰器：按条缓存嵌入结果。
// 命中/未命中计数即 S0-3「语义缓存命中率基线」的观测量。
type CachedEmbeddingProvider struct {
	inner EmbeddingProvider
	cache *EmbeddingCache
}

// NewCachedEmbeddingProvider 包装 inner；maxEntries <= 0 取默认上限。
func NewCachedEmbeddingProvider(inner EmbeddingProvider, maxEntries int) *CachedEmbeddingProvider {
	return &CachedEmbeddingProvider{inner: inner, cache: NewEmbeddingCache(maxEntries)}
}

// Embeddings 批量嵌入：命中条目走缓存，未命中子集批量远端调用后回填。
func (p *CachedEmbeddingProvider) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	// 批内按文本去重：同批重复文本只远端调用一次
	missing := make(map[string][]int)
	for i, t := range texts {
		if v, ok := p.cache.Get(p.inner.Model(), t); ok {
			out[i] = v
			continue
		}
		missing[t] = append(missing[t], i)
	}
	if len(missing) > 0 {
		uniqueTexts := make([]string, 0, len(missing))
		for t := range missing {
			uniqueTexts = append(uniqueTexts, t)
		}
		vecs, err := p.inner.Embeddings(ctx, uniqueTexts)
		if err != nil {
			return nil, err
		}
		if len(vecs) != len(uniqueTexts) {
			return nil, fmt.Errorf("%w: embeddings 返回条数 %d ≠ 请求条数 %d",
				ErrResponseParseFailed, len(vecs), len(uniqueTexts))
		}
		for j, t := range uniqueTexts {
			p.cache.Put(p.inner.Model(), t, vecs[j])
			for _, i := range missing[t] {
				out[i] = vecs[j]
			}
		}
	}
	return out, nil
}

// Dimension 委托内层。
func (p *CachedEmbeddingProvider) Dimension() int { return p.inner.Dimension() }

// Model 委托内层（同时用作缓存 key 前缀，不同模型互不串用）。
func (p *CachedEmbeddingProvider) Model() string { return p.inner.Model() }

// Semantic 委托内层（缓存不改变语义性：降级位包上缓存仍是降级位）。
func (p *CachedEmbeddingProvider) Semantic() bool { return p.inner.Semantic() }

// CacheStats 返回命中/未命中观测快照。
func (p *CachedEmbeddingProvider) CacheStats() EmbeddingCacheStats { return p.cache.Stats() }

// Cache 暴露底层缓存（供基准直接读 Stats / 预热）。
func (p *CachedEmbeddingProvider) Cache() *EmbeddingCache { return p.cache }

// 编译期接口断言。
var _ EmbeddingProvider = (*CachedEmbeddingProvider)(nil)
