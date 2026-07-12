package tokencache

import (
	"container/list"
	"context"
	"sync"
	"time"
)

// LRUCache 精确匹配的内存 LRU 缓存（L1）
type LRUCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List
	entries  map[string]*cacheItem
}

type cacheItem struct {
	key       string
	response  *ProviderResponse
	expiresAt time.Time
}

// NewLRUCache 创建 LRU 缓存
func NewLRUCache(capacity int) *LRUCache {
	if capacity <= 0 {
		capacity = 256
	}
	return &LRUCache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		order:    list.New(),
		entries:  make(map[string]*cacheItem),
	}
}

// Get 从 LRU 缓存获取
func (c *LRUCache) Get(key string) (*ProviderResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.items[key]
	if !ok {
		return nil, false
	}
	item := elem.Value.(*cacheItem)
	if time.Now().After(item.expiresAt) {
		c.removeElement(elem)
		return nil, false
	}
	c.order.MoveToFront(elem)
	return item.response, true
}

// Put 放入 LRU 缓存
func (c *LRUCache) Put(key string, response *ProviderResponse, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		elem.Value.(*cacheItem).response = response
		elem.Value.(*cacheItem).expiresAt = time.Now().Add(ttl)
		return
	}
	if c.order.Len() >= c.capacity {
		c.removeOldest()
	}
	item := &cacheItem{
		key:       key,
		response:  response,
		expiresAt: time.Now().Add(ttl),
	}
	elem := c.order.PushFront(item)
	c.items[key] = elem
	c.entries[key] = item
}

// Len 返回缓存条目数
func (c *LRUCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

func (c *LRUCache) removeElement(elem *list.Element) {
	item := elem.Value.(*cacheItem)
	delete(c.items, item.key)
	delete(c.entries, item.key)
	c.order.Remove(elem)
}

func (c *LRUCache) removeOldest() {
	back := c.order.Back()
	if back != nil {
		c.removeElement(back)
	}
}

// MultiLevelCache L1 (LRU) + L2 (Semantic) 多级缓存
type MultiLevelCache struct {
	l1 *LRUCache
	l2 SemanticCache
}

// NewMultiLevelCache 创建多级缓存
func NewMultiLevelCache(l1Capacity, l2MaxSize int) *MultiLevelCache {
	return &MultiLevelCache{
		l1: NewLRUCache(l1Capacity),
		l2: NewSemanticCache(l2MaxSize),
	}
}

// Lookup L1 -> L2 逐级查找
func (m *MultiLevelCache) Lookup(ctx context.Context, prompt string, threshold float64) (*CachedResponse, bool) {
	// L1: 精确匹配
	if resp, ok := m.l1.Get(prompt); ok {
		return &CachedResponse{Response: resp}, true
	}
	// L2: 语义匹配
	if cached, ok := m.l2.Lookup(ctx, prompt, threshold); ok {
		// 回填 L1
		m.l1.Put(prompt, cached.Response, 5*time.Minute)
		return cached, true
	}
	return nil, false
}

// Store 同时写入 L1 和 L2
func (m *MultiLevelCache) Store(ctx context.Context, prompt string, response *ProviderResponse, ttl time.Duration) error {
	m.l1.Put(prompt, response, ttl)
	return m.l2.Store(ctx, prompt, response, ttl)
}

// Stats 返回合并统计
func (m *MultiLevelCache) Stats() CacheStats {
	return m.l2.Stats()
}
