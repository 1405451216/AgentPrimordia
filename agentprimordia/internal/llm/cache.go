package llm

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const defaultCacheMaxSize = 100

// CacheEntry 缓存条目
type CacheEntry struct {
	Key       string              `json:"key"`
	Query     string              `json:"query"`
	Response  *CompletionResponse `json:"response"`
	CreatedAt time.Time           `json:"created_at"`
	HitCount  int                 `json:"hit_count"`
	Model     string              `json:"model"`
	vector    []float32
}

// CacheStats 缓存统计
type CacheStats struct {
	TotalQueries int64   `json:"total_queries"`
	CacheHits    int64   `json:"cache_hits"`
	CacheMisses  int64   `json:"cache_misses"`
	HitRate      float64 `json:"hit_rate"`
	EntryCount   int     `json:"entry_count"`
	TokensSaved  int64   `json:"tokens_saved"`
	CostSavedUSD float64 `json:"cost_saved_usd"`
}

// EmbeddingFunc 文本向量化函数
type EmbeddingFunc func(ctx context.Context, text string) ([]float32, error)

// LLMCache LLM 缓存接口
type LLMCache interface {
	Get(ctx context.Context, query string, similarity float32) (*CompletionResponse, bool)
	Set(ctx context.Context, query string, resp *CompletionResponse) error
	Stats(ctx context.Context) CacheStats
	Clear(ctx context.Context) error
	Invalidate(ctx context.Context, prompt string) error
}

// InMemoryCache 内存缓存（基于向量相似度）
type InMemoryCache struct {
	entries    []*CacheEntry
	embedder   EmbeddingFunc
	maxSize    int
	minScore   float32
	ttl        time.Duration
	mu         sync.RWMutex
	totalQuery int64
	hits       int64
	misses     int64
	tokensSave int64
}

// NewInMemoryCache 创建内存缓存
func NewInMemoryCache(embedder EmbeddingFunc, maxSize int, minScore float32) *InMemoryCache {
	if minScore < 0 {
		minScore = 0
	}
	if minScore > 1 {
		minScore = 1
	}
	return NewInMemoryCacheWithFullConfig(InMemoryCacheFullConfig{
		Embedder: embedder,
		MaxSize:  maxSize,
		MinScore: minScore,
	})
}

type InMemoryCacheFullConfig struct {
	Embedder EmbeddingFunc
	MaxSize  int
	MinScore float32
	TTL      time.Duration
}

func NewInMemoryCacheWithFullConfig(cfg InMemoryCacheFullConfig) *InMemoryCache {
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = defaultCacheMaxSize
	}
	return &InMemoryCache{
		entries:  make([]*CacheEntry, 0),
		embedder: cfg.Embedder,
		maxSize:  cfg.MaxSize,
		minScore: cfg.MinScore,
		ttl:      cfg.TTL,
	}
}

// Get 查找缓存，similarity 为最低相似度阈值（0-1）
func (c *InMemoryCache) Get(ctx context.Context, query string, similarity float32) (*CompletionResponse, bool) {
	var queryVec []float32
	if c.embedder != nil {
		v, err := c.embedder(ctx, query)
		if err != nil {
			return nil, false
		}
		queryVec = v
	} else {
		queryVec = fingerprintToVector(query)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	atomicAdd(&c.totalQuery, 1)

	var bestEntry *CacheEntry
	var bestScore float32

	for _, entry := range c.entries {
		if c.ttl > 0 && time.Since(entry.CreatedAt) > c.ttl {
			continue
		}
		score := cosineSimilarity(queryVec, entry.vector)
		if score > bestScore {
			bestScore = score
			bestEntry = entry
		}
	}

	if bestEntry != nil && bestScore >= similarity {
		bestEntry.HitCount++
		atomicAdd(&c.hits, 1)
		if bestEntry.Response != nil {
			atomicAdd(&c.tokensSave, int64(bestEntry.Response.Usage.TotalTokens))
		}
		return bestEntry.Response, true
	}

	atomicAdd(&c.misses, 1)
	return nil, false
}

// Set 写入缓存
func (c *InMemoryCache) Set(ctx context.Context, query string, resp *CompletionResponse) error {
	var vec []float32
	if c.embedder != nil {
		v, err := c.embedder(ctx, query)
		if err != nil {
			return err
		}
		vec = v
	} else {
		vec = fingerprintToVector(query)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= c.maxSize {
		// 淘汰最旧的条目并创建新切片，释放底层数组内存
		newEntries := make([]*CacheEntry, 0, c.maxSize)
		newEntries = append(newEntries, c.entries[1:]...)
		c.entries = newEntries
	}

	c.entries = append(c.entries, &CacheEntry{
		Query:     query,
		Response:  resp,
		CreatedAt: time.Now(),
		Model:     resp.Model,
		vector:    vec,
	})

	return nil
}

// Stats 返回缓存统计
func (c *InMemoryCache) Stats(_ context.Context) CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.totalQuery
	hits := c.hits
	misses := c.misses

	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return CacheStats{
		TotalQueries: total,
		CacheHits:    hits,
		CacheMisses:  misses,
		HitRate:      hitRate,
		EntryCount:   len(c.entries),
		TokensSaved:  c.tokensSave,
	}
}

// Clear 清空缓存
func (c *InMemoryCache) Clear(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make([]*CacheEntry, 0)
	c.totalQuery = 0
	c.hits = 0
	c.misses = 0
	c.tokensSave = 0
	return nil
}

// InvalidateAll 清空缓存并重置统计（同Clear，语义更明确）
func (c *InMemoryCache) InvalidateAll(ctx context.Context) error { return c.Clear(ctx) }

func (c *InMemoryCache) Invalidate(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	filtered := make([]*CacheEntry, 0, len(c.entries))
	for _, e := range c.entries {
		if !strings.Contains(e.Query, key) && e.Key != key {
			filtered = append(filtered, e)
		}
	}
	c.entries = filtered
	return nil
}

func (c *InMemoryCache) SetWithVector(query string, resp *CompletionResponse, vector []float32) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= c.maxSize {
		// 淘汰最旧的条目并创建新切片，释放底层数组内存
		newEntries := make([]*CacheEntry, 0, c.maxSize)
		newEntries = append(newEntries, c.entries[1:]...)
		c.entries = newEntries
	}
	c.entries = append(c.entries, &CacheEntry{
		Key: PromptFingerprint(query), Query: query, Response: resp,
		CreatedAt: time.Now(), Model: resp.Model, vector: vector,
	})
	return nil
}

// CachedProvider 带 LLM 缓存的 Provider 装饰器
type CachedProvider struct {
	inner    Provider
	cache    LLMCache
	manager  *CacheManager
	minScore float32
}

// NewCachedProvider 创建带缓存的 Provider
func NewCachedProvider(inner Provider, cache LLMCache, minScore float32) (*CachedProvider, error) {
	if inner == nil {
		return nil, fmt.Errorf("inner provider must not be nil")
	}
	if cache == nil {
		return nil, fmt.Errorf("cache must not be nil")
	}
	return &CachedProvider{
		inner:    inner,
		cache:    cache,
		minScore: minScore,
	}, nil
}

// NewCachedProviderWithManager 创建带 CacheManager 的缓存 Provider
func NewCachedProviderWithManager(inner Provider, mgr *CacheManager, minScore float32) (*CachedProvider, error) {
	if inner == nil {
		return nil, fmt.Errorf("inner provider must not be nil")
	}
	if mgr == nil {
		return nil, fmt.Errorf("cache manager must not be nil")
	}
	return &CachedProvider{
		inner:    inner,
		cache:    mgr.cache,
		manager:  mgr,
		minScore: minScore,
	}, nil
}

// Complete 实现 Provider 接口 — 先查缓存，未命中再调用内部 Provider
func (p *CachedProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	query := extractLastUserQuery(req.Messages)

	if query != "" {
		var cached *CompletionResponse
		var ok bool
		if p.manager != nil {
			cached, ok = p.manager.Get(ctx, query, p.minScore)
		} else {
			cached, ok = p.cache.Get(ctx, query, p.minScore)
		}
		if ok {
			return cached, nil
		}
	}

	resp, err := p.inner.Complete(ctx, req)
	if err != nil {
		return nil, err
	}

	if query != "" {
		if p.manager != nil {
			_ = p.manager.Set(ctx, query, resp)
		} else {
			_ = p.cache.Set(ctx, query, resp)
		}
	}

	return resp, nil
}

// Stream 实现 Provider 接口 — 不缓存流式请求
func (p *CachedProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error) {
	return p.inner.Stream(ctx, req)
}

// CallTools 实现 Provider 接口 — 不缓存工具调用
func (p *CachedProvider) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
	return p.inner.CallTools(ctx, req)
}

// Embeddings 实现 Embedder 接口，通过类型断言委托给底层 Provider
func (p *CachedProvider) Embeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if embedder, ok := p.inner.(Embedder); ok {
		return embedder.Embeddings(ctx, texts)
	}
	return nil, ErrNotSupported
}

// Info 实现 Provider 接口
func (p *CachedProvider) Info() ModelInfo {
	return p.inner.Info()
}

// CacheStats 返回缓存统计信息
func (p *CachedProvider) CacheStats() CacheStats {
	if p.manager != nil {
		return p.manager.Stats(context.Background())
	}
	return p.cache.Stats(context.Background())
}

// EnableCache 启用/禁用缓存
func (p *CachedProvider) EnableCache(enabled bool) {
	if p.manager != nil {
		p.manager.Enable(enabled)
	}
}

// InvalidateCache 使指定 key 的缓存失效
func (p *CachedProvider) InvalidateCache(key string) {
	ctx := context.Background()
	if p.manager != nil {
		_ = p.manager.Invalidate(ctx, key)
	} else {
		_ = p.cache.Invalidate(ctx, key)
	}
}

// ClearCache 清空所有缓存
func (p *CachedProvider) ClearCache() {
	ctx := context.Background()
	if p.manager != nil {
		_ = p.manager.Clear(ctx)
	} else {
		_ = p.cache.Clear(ctx)
	}
}

// cosineSimilarity 计算两个向量的余弦相似度
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

// extractLastUserQuery 提取最后一条用户消息
func extractLastUserQuery(messages []ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

// atomicAdd 原子加法辅助
func atomicAdd(val *int64, delta int64) {
	atomic.AddInt64(val, delta)
}
