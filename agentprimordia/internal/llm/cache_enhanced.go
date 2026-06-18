package llm

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// promptFingerprintCache 缓存 prompt → fingerprint（perf-v6 Task 7 + Task B）
// 使用 container/list + map 实现 O(1) LRU，避免 sync.Map 无限制增长
type fingerprintCache struct {
	mu    sync.Mutex
	lru   *list.List               // 双向链表：Front=最旧, Back=最新
	items map[string]*list.Element // prompt → list.Element
	max   int
}

func newFingerprintCache(max int) *fingerprintCache {
	if max <= 0 {
		max = defaultFingerprintCacheSize
	}
	return &fingerprintCache{
		lru:   list.New(),
		items: make(map[string]*list.Element, max),
		max:   max,
	}
}

func (c *fingerprintCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[key]; ok {
		c.lru.MoveToBack(elem) // O(1) 升级
		return elem.Value.(*fpCacheEntry).value, true
	}
	return "", false
}

func (c *fingerprintCache) put(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[key]; ok {
		elem.Value.(*fpCacheEntry).value = value
		c.lru.MoveToBack(elem)
		return
	}
	// 淘汰最旧
	for c.lru.Len() >= c.max {
		front := c.lru.Front()
		if front == nil {
			break
		}
		c.lru.Remove(front)
		oldKey := front.Value.(*fpCacheEntry).key
		delete(c.items, oldKey)
	}
	// Value 同时存 key（用于淘汰时反向查 map）和 value
	entry := &fpCacheEntry{key: key, value: value}
	listElem := c.lru.PushBack(entry)
	c.items[key] = listElem
}

type fpCacheEntry struct {
	key, value string
}

// promptFingerprintCache 全局实例（perf-v6 Task B：限制大小 1000）
var promptFingerprintCache = newFingerprintCache(1000)

const (
	defaultFingerprintCacheSize = 100
	defaultFingerprintDim       = 64
	fingerprintTruncateLen      = 16
	fnvOffsetBasis              = 2166136261
	fnvPrime                    = 16777619
)

type CacheTracer interface {
	StartSpan(ctx context.Context, name string) (context.Context, SpanLike)
	EndSpan(ctx context.Context, span SpanLike, err error)
}

type SpanLike interface {
	SetAttribute(key string, value any)
}

type CacheManager struct {
	cache   LLMCache
	tracer  CacheTracer
	enabled bool
	mu      sync.RWMutex
}

type CacheManagerConfig struct {
	Cache   LLMCache
	Tracer  CacheTracer
	Enabled bool
}

type FingerprintCache struct {
	entries    map[string]*fingerprintEntry
	maxSize    int
	ttl        time.Duration
	mu         sync.RWMutex
	totalQuery int64
	hits       int64
	misses     int64
	tokensSave int64
}

type fingerprintEntry struct {
	response  *CompletionResponse
	createdAt time.Time
	hitCount  int
}

func NewFingerprintCache(maxSize int, ttl time.Duration) *FingerprintCache {
	if maxSize <= 0 {
		maxSize = defaultFingerprintCacheSize
	}
	return &FingerprintCache{
		entries: make(map[string]*fingerprintEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

func (c *FingerprintCache) Get(_ context.Context, query string, _ float32) (*CompletionResponse, bool) {
	key := PromptFingerprint(query)

	c.mu.Lock()
	defer c.mu.Unlock()

	atomicAdd(&c.totalQuery, 1)

	entry, ok := c.entries[key]
	if ok {
		if c.ttl > 0 && time.Since(entry.createdAt) > c.ttl {
			delete(c.entries, key)
			atomicAdd(&c.misses, 1)
			return nil, false
		}
		entry.hitCount++
		atomicAdd(&c.hits, 1)
		atomicAdd(&c.tokensSave, int64(entry.response.Usage.TotalTokens))
		return entry.response, true
	}

	atomicAdd(&c.misses, 1)
	return nil, false
}

func (c *FingerprintCache) Set(_ context.Context, query string, resp *CompletionResponse) error {
	key := PromptFingerprint(query)

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= c.maxSize {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range c.entries {
			if oldestTime.IsZero() || v.createdAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.createdAt
			}
		}
		if oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}

	c.entries[key] = &fingerprintEntry{response: resp, createdAt: time.Now()}
	return nil
}

func (c *FingerprintCache) Stats(_ context.Context) CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.totalQuery
	hits := c.hits
	misses := c.misses
	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}
	return CacheStats{TotalQueries: total, CacheHits: hits, CacheMisses: misses,
		HitRate: hitRate, EntryCount: len(c.entries), TokensSaved: c.tokensSave}
}

func (c *FingerprintCache) Clear(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*fingerprintEntry)
	c.totalQuery = 0
	c.hits = 0
	c.misses = 0
	c.tokensSave = 0
	return nil
}

func (c *FingerprintCache) Invalidate(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.entries {
		if strings.Contains(k, key) {
			delete(c.entries, k)
		}
	}
	return nil
}

type NoopCache struct{}

func (NoopCache) Get(context.Context, string, float32) (*CompletionResponse, bool) { return nil, false }
func (NoopCache) Set(context.Context, string, *CompletionResponse) error           { return nil }
func (NoopCache) Stats(_ context.Context) CacheStats                               { return CacheStats{} }
func (NoopCache) Clear(_ context.Context) error                                    { return nil }
func (NoopCache) Invalidate(_ context.Context, _ string) error                     { return nil }

type HybridCache struct {
	fingerprint LLMCache
	semantic    *InMemoryCache
}

func NewHybridCache(fp LLMCache, sem *InMemoryCache) (*HybridCache, error) {
	if fp == nil {
		return nil, fmt.Errorf("fingerprint cache must not be nil")
	}
	return &HybridCache{fingerprint: fp, semantic: sem}, nil
}

func (h *HybridCache) Get(ctx context.Context, query string, similarity float32) (*CompletionResponse, bool) {
	if resp, ok := h.fingerprint.Get(ctx, query, 0); ok {
		return resp, true
	}
	if h.semantic != nil {
		return h.semantic.Get(ctx, query, similarity)
	}
	return nil, false
}

func (h *HybridCache) Set(ctx context.Context, query string, resp *CompletionResponse) error {
	_ = h.fingerprint.Set(ctx, query, resp)
	if h.semantic != nil {
		return h.semantic.Set(ctx, query, resp)
	}
	return nil
}

func (h *HybridCache) Stats(ctx context.Context) CacheStats { return h.fingerprint.Stats(ctx) }
func (h *HybridCache) Clear(ctx context.Context) error {
	_ = h.fingerprint.Clear(ctx)
	if h.semantic != nil {
		_ = h.semantic.Clear(ctx)
	}
	return nil
}
func (h *HybridCache) Invalidate(ctx context.Context, key string) error {
	if ic, ok := h.fingerprint.(interface {
		Invalidate(context.Context, string) error
	}); ok {
		_ = ic.Invalidate(ctx, key)
	}
	if h.semantic != nil {
		_ = h.semantic.Invalidate(ctx, key)
	}
	return nil
}

func PromptFingerprint(text string) string {
	// perf-v6 Task 7：缓存 prompt → fingerprint，避免重复 sha256
	if v, ok := promptFingerprintCache.get(text); ok {
		return v
	}
	normalized := normalizeText(text)
	hash := sha256.Sum256([]byte(normalized))
	fp := hex.EncodeToString(hash[:])[:16]
	promptFingerprintCache.put(text, fp)
	return fp
}

func normalizeText(text string) string {
	normalized := strings.ToLower(strings.TrimSpace(text))
	normalized = collapseWhitespace(normalized)
	return normalized
}

func collapseWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := true
	for _, r := range s {
		isSpace := r == ' ' || r == '\t' || r == '\n' || r == '\r'
		if isSpace && prevSpace {
			continue
		}
		if isSpace {
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
		prevSpace = isSpace
	}
	return strings.TrimSpace(b.String())
}

func fingerprintToVector(text string) []float32 {
	normalized := strings.ToLower(strings.TrimSpace(text))
	normalized = collapseWhitespace(normalized)
	if normalized == "" {
		return make([]float32, defaultFingerprintDim)
	}

	tokens := tokenize(normalized)
	dim := defaultFingerprintDim
	vec := make([]float32, dim)

	for pos, token := range tokens {
		h1 := hashToken(token)
		h2 := hashToken(token + "_pos")
		idx1 := int(h1) % dim
		idx2 := int(h2) % dim
		weight := float32(1.0 + float32(pos)*0.01)
		vec[idx1] += weight
		vec[(idx2+1)%dim] += weight * 0.5

		if len(token) >= 2 {
			for i := 0; i < len(token)-1; i++ {
				bigram := token[i : i+2]
				h3 := hashToken(bigram)
				vec[int(h3)%dim] += weight * 0.3
			}
		}
	}

	for i := 1; i < len(tokens); i++ {
		bigramTok := tokens[i-1] + " " + tokens[i]
		h4 := hashToken(bigramTok)
		vec[int(h4)%dim] += 0.8
	}

	norm := float32(0)
	for _, v := range vec {
		norm += v * v
	}
	if norm > 0 {
		sqrtNorm := float32(math.Sqrt(float64(norm)))
		for i := range vec {
			vec[i] /= sqrtNorm
		}
	}
	return vec
}

func tokenize(text string) []string {
	var tokens []string
	var buf strings.Builder
	for _, r := range text {
		if isWordChar(r) {
			buf.WriteRune(r)
		} else {
			if buf.Len() > 0 {
				tokens = append(tokens, buf.String())
				buf.Reset()
			}
		}
	}
	if buf.Len() > 0 {
		tokens = append(tokens, buf.String())
	}
	return tokens
}

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_'
}

func hashToken(s string) uint32 {
	h := uint32(fnvOffsetBasis)
	for _, c := range s {
		h ^= uint32(c)
		h *= fnvPrime
	}
	return h
}

func NewCacheManager(cfg CacheManagerConfig) *CacheManager {
	return &CacheManager{cache: cfg.Cache, tracer: cfg.Tracer, enabled: cfg.Enabled}
}

func (m *CacheManager) Get(ctx context.Context, query string, similarity float32) (*CompletionResponse, bool) {
	m.mu.RLock()
	enabled := m.enabled
	tracer := m.tracer
	cache := m.cache
	m.mu.RUnlock()

	if !enabled || cache == nil {
		return nil, false
	}

	resp, hit := cache.Get(ctx, query, similarity)

	if tracer != nil {
		ctx2, span := tracer.StartSpan(ctx, "cache.lookup")
		span.SetAttribute("cache.hit", hit)
		span.SetAttribute("cache.query_length", len(query))
		if hit {
			span.SetAttribute("cache.response_id", resp.ID)
		}
		tracer.EndSpan(ctx2, span, nil)
	}

	return resp, hit
}

func (m *CacheManager) Set(ctx context.Context, query string, resp *CompletionResponse) error {
	m.mu.RLock()
	cache := m.cache
	m.mu.RUnlock()
	if cache == nil {
		return nil
	}
	return cache.Set(ctx, query, resp)
}

func (m *CacheManager) Stats(ctx context.Context) CacheStats {
	m.mu.RLock()
	cache := m.cache
	m.mu.RUnlock()
	if cache == nil {
		return CacheStats{}
	}
	return cache.Stats(ctx)
}

func (m *CacheManager) Enable(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = enabled
}

func (m *CacheManager) Invalidate(ctx context.Context, key string) error {
	m.mu.RLock()
	cache := m.cache
	m.mu.RUnlock()
	if inv, ok := cache.(interface {
		Invalidate(context.Context, string) error
	}); ok {
		return inv.Invalidate(ctx, key)
	}
	return nil
}

func (m *CacheManager) Clear(ctx context.Context) error {
	m.mu.RLock()
	cache := m.cache
	m.mu.RUnlock()
	if cache != nil {
		return cache.Clear(ctx)
	}
	return nil
}
