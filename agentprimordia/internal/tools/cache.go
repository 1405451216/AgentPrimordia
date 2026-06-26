package tools

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"
)

// CacheConfig 缓存配置
type CacheConfig struct {
	MaxSize int           // 最大缓存条目数
	TTL     time.Duration // 缓存过期时间
}

// CacheStats 缓存统计信息
type CacheStats struct {
	Hits   int64 // 命中次数
	Misses int64 // 未命中次数
	Size   int   // 当前缓存大小
}

// cacheEntry 缓存条目
type cacheEntry struct {
	key       string
	result    *Result
	expiresAt time.Time
}

// Cache 工具结果缓存（LRU + TTL）
type Cache struct {
	mu        sync.RWMutex
	cfg       CacheConfig
	items     map[string]*list.Element
	order     *list.List // LRU 链表：最近使用的在链表尾部
	hits      atomic.Int64
	misses    atomic.Int64
	stopCh    chan struct{} // 停止清理 goroutine
	closeOnce sync.Once     // 防止重复 Close 导致 panic
}

// NewCache 创建工具结果缓存
func NewCache(cfg CacheConfig) *Cache {
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = 100
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 5 * time.Minute
	}

	c := &Cache{
		cfg:    cfg,
		items:  make(map[string]*list.Element),
		order:  list.New(),
		stopCh: make(chan struct{}),
	}

	// 启动后台清理过期条目
	go c.cleanupLoop()

	return c
}

// Get 获取缓存的工具结果
func (c *Cache) Get(key string) (*Result, bool) {
	c.mu.RLock()
	elem, exists := c.items[key]
	c.mu.RUnlock()

	if !exists {
		c.misses.Add(1)
		return nil, false
	}

	entry := elem.Value.(*cacheEntry)

	// 检查是否过期
	if time.Now().After(entry.expiresAt) {
		c.misses.Add(1)
		// 同步删除过期条目，避免 goroutine 泄漏
		c.Delete(key)
		return nil, false
	}

	c.hits.Add(1)

	// 移动到 LRU 链表尾部（最近使用）
	c.mu.Lock()
	c.order.MoveToBack(elem)
	c.mu.Unlock()

	return entry.result, true
}

// Set 设置缓存条目
func (c *Cache) Set(key string, result *Result) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 如果 key 已存在，更新并移动到尾部
	if elem, exists := c.items[key]; exists {
		entry := elem.Value.(*cacheEntry)
		entry.result = result
		entry.expiresAt = time.Now().Add(c.cfg.TTL)
		c.order.MoveToBack(elem)
		return
	}

	// 如果缓存已满，淘汰最久未使用的条目
	if c.order.Len() >= c.cfg.MaxSize {
		c.evictLRU()
	}

	// 添加新条目
	entry := &cacheEntry{
		key:       key,
		result:    result,
		expiresAt: time.Now().Add(c.cfg.TTL),
	}
	elem := c.order.PushBack(entry)
	c.items[key] = elem
}

// Delete 删除缓存条目
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		c.removeElement(elem)
	}
}

// Clear 清空所有缓存
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.order.Init()
}

// Size 返回当前缓存大小
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Stats 返回缓存统计信息
func (c *Cache) Stats() CacheStats {
	c.mu.RLock()
	size := len(c.items)
	c.mu.RUnlock()

	return CacheStats{
		Hits:   c.hits.Load(),
		Misses: c.misses.Load(),
		Size:   size,
	}
}

// Close 关闭缓存，停止后台清理（可安全重复调用）
func (c *Cache) Close() {
	c.closeOnce.Do(func() {
		close(c.stopCh)
	})
}

// evictLRU 淘汰最久未使用的条目（必须在持有锁时调用）
func (c *Cache) evictLRU() {
	elem := c.order.Front()
	if elem == nil {
		return
	}
	c.removeElement(elem)
}

// removeElement 移除链表元素（必须在持有锁时调用）
func (c *Cache) removeElement(elem *list.Element) {
	c.order.Remove(elem)
	entry := elem.Value.(*cacheEntry)
	delete(c.items, entry.key)
}

// cleanupLoop 定期清理过期条目
func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

// cleanup 清理所有过期条目
func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var toRemove []*list.Element

	// 收集所有过期条目
	for elem := c.order.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(*cacheEntry)
		if now.After(entry.expiresAt) {
			toRemove = append(toRemove, elem)
		}
	}

	// 批量删除
	for _, elem := range toRemove {
		c.removeElement(elem)
	}
}
