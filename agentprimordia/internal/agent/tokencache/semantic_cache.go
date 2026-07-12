package tokencache

import (
	"context"
	"hash/fnv"
	"strings"
	"sync"
	"time"
)

// ProviderResponse 表示 LLM 返回的响应
type ProviderResponse struct {
	Content     string `json:"content"`
	Model       string `json:"model"`
	TotalTokens int    `json:"total_tokens"`
}

// CachedResponse 缓存的响应条目
type CachedResponse struct {
	Response  *ProviderResponse `json:"response"`
	CachedAt  time.Time         `json:"cached_at"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// CacheStats 缓存统计信息
type CacheStats struct {
	Hits    int64   `json:"hits"`
	Misses  int64   `json:"misses"`
	Entries int     `json:"entries"`
	HitRate float64 `json:"hit_rate"`
}

// SemanticCache 语义缓存接口
type SemanticCache interface {
	Lookup(ctx context.Context, prompt string, threshold float64) (*CachedResponse, bool)
	Store(ctx context.Context, prompt string, response *ProviderResponse, ttl time.Duration) error
	Stats() CacheStats
}

// semanticEntry 语义缓存条目
type semanticEntry struct {
	promptHash uint32
	prompt     string
	keywords   []string
	response   *ProviderResponse
	expiresAt  time.Time
}

// SemanticCacheImpl 语义缓存实现
// 基于关键词重叠度匹配（无 embedding 时的退化方案）
type SemanticCacheImpl struct {
	mu         sync.RWMutex
	entries    []*semanticEntry
	keywords   map[string]map[uint32]struct{}
	maxSize    int
	defaultTTL time.Duration
	hits       int64
	misses     int64
}

// NewSemanticCache 创建语义缓存实例
func NewSemanticCache(maxSize int) *SemanticCacheImpl {
	if maxSize <= 0 {
		maxSize = 1024
	}
	return &SemanticCacheImpl{
		entries:    make([]*semanticEntry, 0),
		keywords:   make(map[string]map[uint32]struct{}),
		maxSize:    maxSize,
		defaultTTL: 30 * time.Minute,
	}
}

// Lookup 查找语义相似的缓存
func (c *SemanticCacheImpl) Lookup(ctx context.Context, prompt string, threshold float64) (*CachedResponse, bool) {
	if threshold <= 0 {
		threshold = 0.95
	}

	c.mu.RLock()
	now := time.Now()
	queryKeywords := extractKeywords(prompt)
	queryHash := hashStr(prompt)

	// 通过关键词索引筛选候选
	candidates := make(map[uint32]int)
	for _, kw := range queryKeywords {
		if hashes, ok := c.keywords[kw]; ok {
			for h := range hashes {
				candidates[h]++
			}
		}
	}

	var bestEntry *semanticEntry
	bestScore := 0.0

	for _, entry := range c.entries {
		if now.After(entry.expiresAt) {
			continue
		}
		// 精确匹配
		if entry.promptHash == queryHash && entry.prompt == prompt {
			bestEntry = entry
			bestScore = 1.0
			break
		}
		if _, isCand := candidates[entry.promptHash]; !isCand {
			continue
		}
		score := keywordOverlap(queryKeywords, entry.keywords)
		if score > bestScore {
			bestScore = score
			bestEntry = entry
		}
	}
	c.mu.RUnlock()

	if bestEntry != nil && bestScore >= threshold {
		c.mu.Lock()
		c.hits++
		c.mu.Unlock()
		return &CachedResponse{
			Response:  bestEntry.response,
			ExpiresAt: bestEntry.expiresAt,
		}, true
	}

	c.mu.Lock()
	c.misses++
	c.mu.Unlock()
	return nil, false
}

// Store 存储缓存条目
func (c *SemanticCacheImpl) Store(ctx context.Context, prompt string, response *ProviderResponse, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = c.defaultTTL
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	hash := hashStr(prompt)
	keywords := extractKeywords(prompt)
	now := time.Now()

	c.evictExpired(now)

	if len(c.entries) >= c.maxSize {
		c.removeOldest()
	}

	entry := &semanticEntry{
		promptHash: hash,
		prompt:     prompt,
		keywords:   keywords,
		response:   response,
		expiresAt:  now.Add(ttl),
	}
	c.entries = append(c.entries, entry)

	for _, kw := range keywords {
		if _, ok := c.keywords[kw]; !ok {
			c.keywords[kw] = make(map[uint32]struct{})
		}
		c.keywords[kw][hash] = struct{}{}
	}
	return nil
}

// Stats 返回缓存统计
func (c *SemanticCacheImpl) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	total := c.hits + c.misses
	var rate float64
	if total > 0 {
		rate = float64(c.hits) / float64(total)
	}
	return CacheStats{
		Hits:    c.hits,
		Misses:  c.misses,
		Entries: len(c.entries),
		HitRate: rate,
	}
}

func (c *SemanticCacheImpl) evictExpired(now time.Time) {
	active := make([]*semanticEntry, 0, len(c.entries))
	for _, e := range c.entries {
		if now.Before(e.expiresAt) {
			active = append(active, e)
		}
	}
	c.entries = active
	// 重建索引
	c.keywords = make(map[string]map[uint32]struct{})
	for _, e := range c.entries {
		for _, kw := range e.keywords {
			if _, ok := c.keywords[kw]; !ok {
				c.keywords[kw] = make(map[uint32]struct{})
			}
			c.keywords[kw][e.promptHash] = struct{}{}
		}
	}
}

func (c *SemanticCacheImpl) removeOldest() {
	if len(c.entries) == 0 {
		return
	}
	oldest := c.entries[0]
	c.entries = c.entries[1:]
	// 清理索引
	for _, kw := range oldest.keywords {
		if hashes, ok := c.keywords[kw]; ok {
			delete(hashes, oldest.promptHash)
			if len(hashes) == 0 {
				delete(c.keywords, kw)
			}
		}
	}
}

// extractKeywords 提取关键词（简单分词）：按非字母数字中文字符切分
func extractKeywords(text string) []string {
	text = strings.ToLower(text)
	var words []string
	var current strings.Builder
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127 {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

// keywordOverlap 计算 Jaccard 相似度
func keywordOverlap(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := make(map[string]struct{}, len(a))
	for _, w := range a {
		setA[w] = struct{}{}
	}
	intersect := 0
	for _, w := range b {
		if _, ok := setA[w]; ok {
			intersect++
		}
	}
	union := len(a) + len(b) - intersect
	if union == 0 {
		return 0
	}
	return float64(intersect) / float64(union)
}

func hashStr(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}
