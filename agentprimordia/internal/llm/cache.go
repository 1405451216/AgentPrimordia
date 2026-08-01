package llm

import (
	"math"
	"sync"
)

const defaultCacheMaxSize = 100

// perf-v6 Task 3 + Task 1：多表 LSH 索引（替代简单单桶分桶）
// 多哈希表 + 多探测：每个 entry 插入到 N 个 hash table 的多个 bucket；
// 查询时优先扫描 candidate buckets，避免全表 O(N) 扫描
const (
	lshNumTables     = 4  // 哈希表数量（更多表 → 更高召回）
	lshNumBuckets    = 64 // 每个表 bucket 数量
	lshBitsPerVec    = 8  // 用向量的前 8 个非零维度做 hash
	lshProbeNeighbor = 2  // 多探测邻域半径（probe 邻近 bucket）
)

// lshBucketCache 维护每个 hash table 的 bucket → entries 列表（perf-v6 Task 1）
type lshBucketCache struct {
	tables [lshNumTables]map[int][]*CacheEntry
	rand   [lshNumTables][lshBitsPerVec]uint32 // 每个表的随机 hash 系数
}

// newLSHCache 创建 LSH 索引（perf-v6 Task 1）
func newLSHCache() *lshBucketCache {
	c := &lshBucketCache{}
	for t := 0; t < lshNumTables; t++ {
		c.tables[t] = make(map[int][]*CacheEntry, 256)
		for b := 0; b < lshBitsPerVec; b++ {
			// 用确定性 hash（避免引入 math/rand 全局锁）
			c.rand[t][b] = uint32((t+1)*7919) ^ uint32((b+1)*65537)
		}
	}
	return c
}

// hashForTable 计算向量在指定 hash table 的 bucket（perf-v6 Task 1）
func (c *lshBucketCache) hashForTable(vec []float32, tableIdx int) int {
	var h uint32 = 0
	for b := 0; b < lshBitsPerVec && b < len(vec); b++ {
		// 二值化：> 0 为 1，否则为 0
		bit := uint32(0)
		if vec[b] > 0 {
			bit = 1
		}
		h ^= bit * c.rand[tableIdx][b]
	}
	return int(h) % lshNumBuckets
}

// add 向 LSH 索引添加 entry（perf-v6 Task 1）
func (c *lshBucketCache) add(entry *CacheEntry, vec []float32) {
	for t := 0; t < lshNumTables; t++ {
		bucket := c.hashForTable(vec, t)
		c.tables[t][bucket] = append(c.tables[t][bucket], entry)
	}
}

// remove 从 LSH 索引移除 entry（perf-v6 Task 1）
func (c *lshBucketCache) remove(entry *CacheEntry, vec []float32) {
	for t := 0; t < lshNumTables; t++ {
		bucket := c.hashForTable(vec, t)
		entries := c.tables[t][bucket]
		for i, e := range entries {
			if e == entry {
				c.tables[t][bucket] = append(entries[:i], entries[i+1:]...)
				break
			}
		}
	}
}

// probeCandidates 返回 query 的候选 entries（合并多表 + 多探测）
// perf-v6 Task 1：返回 *set* 避免重复
func (c *lshBucketCache) probeCandidates(vec []float32) []*CacheEntry {
	seen := make(map[*CacheEntry]struct{}, 256)
	for t := 0; t < lshNumTables; t++ {
		baseBucket := c.hashForTable(vec, t)
		// 多探测：probe 邻近 bucket（+/- lshProbeNeighbor）
		for offset := -lshProbeNeighbor; offset <= lshProbeNeighbor; offset++ {
			bucket := (baseBucket + offset + lshNumBuckets) % lshNumBuckets
			for _, e := range c.tables[t][bucket] {
				seen[e] = struct{}{}
			}
		}
	}
	candidates := make([]*CacheEntry, 0, len(seen))
	for e := range seen {
		candidates = append(candidates, e)
	}
	return candidates
}

// 保留原简单分桶作为 fallback（perf-v6 Task 3）。
// 当前 LSH 为活动实现，分桶方案未被调用；按原工程师意图保留供未来回退。
// 以下符号为有意保留的死代码，故抑制 unused 检查。
//
//nolint:unused // 保留的分桶 fallback 常量
const (
	bucketCount        = 16 // 16 个 bucket
	maxBucketScanSkips = 32 // 连续跳过这么多后放弃分桶过滤
)

// perf-v6 Task 5：embedding 结果 LRU 缓存
// 多次相同 query 走 fingerprint 缓存时也需 embedder；缓存 embedder 结果避免重复远端调用
const embeddingCacheMaxEntries = 256

var embeddingCache sync.Map // map[string][]float32（无锁读，存为 []float32 浅拷贝）

// getCachedEmbedding 获取缓存的 embedding（perf-v6 Task 5）
func getCachedEmbedding(query string) ([]float32, bool) {
	if v, ok := embeddingCache.Load(query); ok {
		// 浅拷贝：避免外部修改污染缓存
		src := v.([]float32)
		dst := make([]float32, len(src))
		copy(dst, src)
		return dst, true
	}
	return nil, false
}

// putCachedEmbedding 写入 embedding 缓存（perf-v6 Task 5）
// 简单 LRU 淘汰：size 超过限制时清理 1/4
func putCachedEmbedding(query string, vec []float32) {
	if embeddingCacheSize() >= embeddingCacheMaxEntries {
		// 超过上限清理 1/4（简单策略；sync.Map 缺乏内置 LRU）
		count := 0
		embeddingCache.Range(func(k, _ any) bool {
			if count >= embeddingCacheMaxEntries/4 {
				return false
			}
			embeddingCache.Delete(k)
			count++
			return true
		})
	}
	// 深拷贝存
	dst := make([]float32, len(vec))
	copy(dst, vec)
	embeddingCache.Store(query, dst)
}

// embeddingCacheSize 估算 sync.Map 当前大小（perf-v6 Task 5）
func embeddingCacheSize() int {
	count := 0
	embeddingCache.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// bucketKey 根据 queryVec 第一维计算 bucket（perf-v6 Task 3 fallback，有意保留）
//
//nolint:unused // 保留的分桶 fallback 函数
func bucketKey(vec []float32) int {
	if len(vec) == 0 {
		return 0
	}
	bits := math.Float32bits(vec[0])
	return int(bits) % bucketCount
}

// entryBucket 根据 fingerprint key 字符串计算 bucket（perf-v6 Task 3 fallback，有意保留）
//
//nolint:unused // 保留的分桶 fallback 函数
func entryBucket(key string) int {
	if len(key) == 0 {
		return 0
	}
	var sum int
	for i := 0; i < len(key) && i < 4; i++ {
		sum = sum*31 + int(key[i])
	}
	if sum < 0 {
		sum = -sum
	}
	return sum % bucketCount
}

// CacheEntry 缓存条目（perf-v6 Task B：HitCount 改 atomic.Int64，避免并发计数竞争）
