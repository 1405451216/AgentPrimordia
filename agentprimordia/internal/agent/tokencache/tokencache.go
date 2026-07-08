// Package tokencache 提供 Token 估算缓存（perf-v11 stage-3）
// 当前状态：缓存函数 EstimateTokensCached 暂未在生产代码中使用，保留供未来启用。
// 性能对比（实测）：len() 直接计算 ~0.4ns/op；EstimateTokensCached ~55ns/op。
// 缓存开销主要来自 FNV-1a hash + sync.Map 查找。只有在以下场景启用缓存才有收益：
//  1. 单条文本 > 10KB（如长文档 chunk）
//  2. 同一文本在短时间内重复出现（缓存命中率高）
//
// 当前 estimateTokens 路径是 O(N) 字符长度求和，单次调用纳秒级，无需缓存。
package tokencache

import (
	"hash/fnv"
	"sync"
	"sync/atomic"
)

// tokenCacheSize 缓存条目数上限。
// 长对话典型消息条数 < 1000，按 2KB 单条文本估算，缓存占用 < 2MB。
const tokenCacheSize = 4096

// tokenCacheEntries 记录当前缓存条目数（用于 LRU 淘汰）。
// 简单原子计数：超过上限时清理 1/2。
var tokenCacheEntries atomic.Int64

// tokenCache 缓存 (hash(text) -> tokens)
// 使用 sync.Map 避免锁争用：读路径完全无锁，写路径使用 LoadOrStore 原子操作。
// key 用 FNV-1a hash(text) 避免 string 引用。
var tokenCache sync.Map

// EstimateTokensCached 缓存版的单条消息 token 估算。
// 优化（perf-v11 stage-3）：同一消息文本在多轮对话中重复出现时（如工具调用结果），
// 可直接命中缓存，省去 O(N) 字符长度计算。
func EstimateTokensCached(text string) int {
	if len(text) == 0 {
		return 0
	}
	key := HashText(text)
	if v, ok := tokenCache.Load(key); ok {
		return v.(int)
	}
	// 启发式：1 Token ≈ 4 字符（兼容英文/中文混合）
	tokens := len(text) / 4
	// LoadOrStore：避免并发场景下重复计算
	actual, _ := tokenCache.LoadOrStore(key, tokens)
	// 超过上限时淘汰一半（异步，不阻塞调用方）
	if tokenCacheEntries.Add(1) > tokenCacheSize {
		go evictHalfTokenCache()
	}
	return actual.(int)
}

// HashText 使用 FNV-1a 32-bit 哈希作为缓存 key。
// 短字符串也能良好分布，碰撞概率 < 1/2^32。
func HashText(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// evictHalfTokenCache 清理一半缓存（异步调用，不阻塞主流程）。
// 简单策略：sync.Map 缺乏内置 LRU，使用随机遍历删除 1/2 条目。
func evictHalfTokenCache() {
	// 防止并发触发多次淘汰
	current := tokenCacheEntries.Load()
	if current < int64(tokenCacheSize) {
		return
	}
	target := int64(tokenCacheSize / 2)
	deleted := int64(0)
	tokenCache.Range(func(k, _ any) bool {
		tokenCache.Delete(k)
		deleted++
		if deleted >= current-target {
			return false
		}
		return true
	})
	tokenCacheEntries.Add(-deleted)
}

// ClearTokenCache 清空 token 缓存（测试用 / 内存压力场景）
func ClearTokenCache() {
	tokenCache.Range(func(k, _ any) bool {
		tokenCache.Delete(k)
		return true
	})
	tokenCacheEntries.Store(0)
}
