# 中优先级优化实施计划（3-6 个月）

> **状态：已完成** ✅（所有 Task 9-14 已实现并通过测试，2026-06-29 验证）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 升级记忆系统（HNSW 索引、压缩、共享）、增强多 Agent 编排（可视化、动态拓扑）、提升开发者体验（CLI 增强、Playground）

**Architecture:** 在现有 internal/memory、internal/orchestration、cmd/ap 基础上扩展。记忆系统引入 HNSW 图索引加速向量检索；编排系统增加动态拓扑调整能力；CLI 增加交互式向导和自动化测试生成。所有新模块仅使用 Go 标准库。

**Tech Stack:** Go 1.26+ 标准库、HNSW 算法、HTML/JS Playground（单文件）、embed.FS 模板

---

## Phase 4: 记忆系统升级（第 7-10 周）

### Task 9: HNSW 向量索引

**Files:**
- Create: `internal/memory/hnsw.go`
- Create: `internal/memory/hnsw_test.go`
- Modify: `pkg/memory.go`（导出新类型）

- [x] **Step 1: 编写 HNSW 索引测试**

```go
// internal/memory/hnsw_test.go
package memory

import (
	"context"
	"math/rand"
	"testing"
)

func TestHNSW_InsertAndSearch(t *testing.T) {
	idx := NewHNSWIndex(HNSWConfig{
		MaxConnections: 16,
		EfConstruction: 200,
		EfSearch:       50,
		Dimensions:     128,
	})

	// 插入 100 个向量
	for i := 0; i < 100; i++ {
		v := make([]float32, 128)
		for j := range v {
			v[j] = rand.Float32()
		}
		idx.Insert(context.Background(), fmt.Sprintf("vec-%d", i), v, nil)
	}

	// 搜索最近邻
	query := make([]float32, 128)
	for j := range query {
		query[j] = rand.Float32()
	}

	results := idx.Search(context.Background(), query, 10)
	if len(results) != 10 {
		t.Errorf("结果数 = %d, 期望 10", len(results))
	}

	// 验证结果按距离排序
	for i := 1; i < len(results); i++ {
		if results[i].Distance < results[i-1].Distance {
			t.Error("结果未按距离升序排列")
		}
	}
}

func TestHNSW_Recall(t *testing.T) {
	idx := NewHNSWIndex(HNSWConfig{
		MaxConnections: 16,
		EfConstruction: 200,
		EfSearch:       100,
		Dimensions:     64,
	})

	// 插入 1000 个向量
	vectors := make([][]float32, 1000)
	for i := range vectors {
		v := make([]float32, 64)
		for j := range v {
			v[j] = rand.Float32()
		}
		vectors[i] = v
		idx.Insert(context.Background(), fmt.Sprintf("vec-%d", i), v, nil)
	}

	// 查询并对比暴力搜索
	query := make([]float32, 64)
	for j := range query {
		query[j] = rand.Float32()
	}

	hnswResults := idx.Search(context.Background(), query, 10)

	// 暴力搜索求真实 top-10
	bruteForce := bruteForceSearch(vectors, query, 10)

	// 计算 recall@10
	hits := 0
	hnswIDs := make(map[string]bool)
	for _, r := range hnswResults {
		hnswIDs[r.ID] = true
	}
	for _, id := range bruteForce {
		if hnswIDs[id] {
			hits++
		}
	}

	recall := float64(hits) / 10.0
	if recall < 0.8 {
		t.Errorf("Recall@10 = %.2f, 期望 >= 0.8", recall)
	}
}

func TestHNSW_Delete(t *testing.T) {
	idx := NewHNSWIndex(HNSWConfig{
		MaxConnections: 16,
		EfConstruction: 200,
		EfSearch:       50,
		Dimensions:     32,
	})

	v := make([]float32, 32)
	for j := range v {
		v[j] = 0.5
	}
	idx.Insert(context.Background(), "vec-1", v, nil)
	idx.Insert(context.Background(), "vec-2", v, nil)

	idx.Delete("vec-1")

	results := idx.Search(context.Background(), v, 10)
	for _, r := range results {
		if r.ID == "vec-1" {
			t.Error("已删除的向量仍出现在搜索结果中")
		}
	}
}

func TestHNSW_Empty(t *testing.T) {
	idx := NewHNSWIndex(HNSWConfig{Dimensions: 32})
	results := idx.Search(context.Background(), make([]float32, 32), 10)
	if len(results) != 0 {
		t.Errorf("空索引搜索应返回空, 得到 %d 条", len(results))
	}
}

// bruteForceSearch 暴力搜索用于对比
func bruteForceSearch(vectors [][]float32, query []float32, k int) []string {
	type scored struct {
		id   string
		dist float32
	}
	var all []scored
	for i, v := range vectors {
		d := cosineDistance(v, query)
		all = append(all, scored{fmt.Sprintf("vec-%d", i), d})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].dist < all[j].dist })
	ids := make([]string, k)
	for i := 0; i < k && i < len(all); i++ {
		ids[i] = all[i].id
	}
	return ids
}
```

- [x] **Step 2: 运行测试验证失败**

Run: `go test ./internal/memory/ -run TestHNSW -v`
Expected: FAIL — `NewHNSWIndex` 未定义

- [x] **Step 3: 实现 HNSW 索引**

```go
// internal/memory/hnsw.go
package memory

import (
	"container/heap"
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
)

// HNSWConfig HNSW 索引配置
type HNSWConfig struct {
	MaxConnections int // 每层最大连接数 M
	EfConstruction int // 构建时搜索范围
	EfSearch       int // 查询时搜索范围
	Dimensions     int // 向量维度
	MaxElements    int // 最大元素数（0 表示无限）
}

// HNSWIndex HNSW 图索引实现
type HNSWIndex struct {
	cfg    HNSWConfig
	mu     sync.RWMutex
	nodes  map[string]*hnswNode
	entry  *hnswNode
	maxLvl int
}

type hnswNode struct {
	id       string
	vector   []float32
	metadata map[string]any
	neighbors [][]string // 每层的邻居列表
	level    int
	deleted  bool
}

// SearchResult 搜索结果
type SearchResult struct {
	ID       string
	Distance float32
	Metadata map[string]any
}

// NewHNSWIndex 创建 HNSW 索引
func NewHNSWIndex(cfg HNSWConfig) *HNSWIndex {
	if cfg.MaxConnections <= 0 {
		cfg.MaxConnections = 16
	}
	if cfg.EfConstruction <= 0 {
		cfg.EfConstruction = 200
	}
	if cfg.EfSearch <= 0 {
		cfg.EfSearch = 50
	}
	return &HNSWIndex{
		cfg:   cfg,
		nodes: make(map[string]*hnswNode),
	}
}

// Insert 插入向量
func (idx *HNSWIndex) Insert(ctx context.Context, id string, vector []float32, metadata map[string]any) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.cfg.MaxElements > 0 && len(idx.nodes) >= idx.cfg.MaxElements {
		return
	}

	level := idx.randomLevel()
	node := &hnswNode{
		id:        id,
		vector:    vector,
		metadata:  metadata,
		neighbors: make([][]string, level+1),
		level:     level,
	}
	for i := range node.neighbors {
		node.neighbors[i] = make([]string, 0)
	}

	idx.nodes[id] = node

	// 第一个节点直接作为入口
	if idx.entry == nil {
		idx.entry = node
		idx.maxLvl = level
		return
	}

	// 从最高层向下搜索，逐层插入连接
	ep := idx.entry.id
	for lev := idx.maxLvl; lev > level; lev-- {
		ep = idx.greedyClosest(ep, vector, lev)
	}

	for lev := min(level, idx.maxLvl); lev >= 0; lev-- {
		neighbors := idx.searchLayer(ep, vector, idx.cfg.EfConstruction, lev)

		// 选择最近的 M 个邻居
		maxConn := idx.cfg.MaxConnections
		if lev == 0 {
			maxConn = idx.cfg.MaxConnections * 2
		}
		selected := neighbors
		if len(selected) > maxConn {
			selected = selected[:maxConn]
		}

		// 建立双向连接
		node.neighbors[lev] = make([]string, len(selected))
		for i, n := range selected {
			node.neighbors[lev][i] = n.id
			idx.addConnection(n.id, id, lev)
		}

		if len(neighbors) > 0 {
			ep = neighbors[0].id
		}
	}

	if level > idx.maxLvl {
		idx.maxLvl = level
		idx.entry = node
	}
}

// Search 搜索最近邻
func (idx *HNSWIndex) Search(ctx context.Context, query []float32, k int) []SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.entry == nil || len(idx.nodes) == 0 {
		return nil
	}

	ep := idx.entry.id
	for lev := idx.maxLvl; lev > 0; lev-- {
		ep = idx.greedyClosest(ep, query, lev)
	}

	candidates := idx.searchLayer(ep, query, max(k, idx.cfg.EfSearch), 0)

	// 返回 top-k
	results := make([]SearchResult, 0, k)
	for i := 0; i < k && i < len(candidates); i++ {
		c := candidates[i]
		if node, ok := idx.nodes[c.id]; ok && !node.deleted {
			results = append(results, SearchResult{
				ID:       c.id,
				Distance: c.distance,
				Metadata: node.metadata,
			})
		}
	}
	return results
}

// Delete 标记删除（惰性删除）
func (idx *HNSWIndex) Delete(id string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if node, ok := idx.nodes[id]; ok {
		node.deleted = true
	}
}

// Len 返回元素数量（含已删除）
func (idx *HNSWIndex) Len() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.nodes)
}

// --- 内部方法 ---

func (idx *HNSWIndex) randomLevel() int {
	level := 0
	ml := 1.0 / math.Log(float64(idx.cfg.MaxConnections))
	for rand.Float64() < math.Exp(-float64(level)/ml) {
		level++
	}
	return level
}

func (idx *HNSWIndex) searchLayer(entryID string, query []float32, ef int, level int) []scoredNode {
	visited := map[string]bool{entryID: true}
	candidates := &maxHeap{}
	results := &minHeap{}

	entryNode := idx.nodes[entryID]
	if entryNode == nil {
		return nil
	}
	dist := cosineDistance(entryNode.vector, query)
	heap.Push(candidates, scoredNode{entryID, dist})
	heap.Push(results, scoredNode{entryID, dist})

	for candidates.Len() > 0 {
		current := heap.Pop(candidates).(scoredNode)

		// 剪枝：如果当前候选比结果中最远的更远，停止
		if results.Len() >= ef && current.distance > (*results)[0].distance {
			break
		}

		node := idx.nodes[current.id]
		if node == nil || level >= len(node.neighbors) {
			continue
		}

		for _, neighborID := range node.neighbors[level] {
			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true

			neighbor := idx.nodes[neighborID]
			if neighbor == nil || neighbor.deleted {
				continue
			}

			d := cosineDistance(neighbor.vector, query)

			if results.Len() < ef || d < (*results)[0].distance {
				heap.Push(candidates, scoredNode{neighborID, d})
				heap.Push(results, scoredNode{neighborID, d})
				if results.Len() > ef {
					heap.Pop(results)
				}
			}
		}
	}

	// 转为有序切片
	sorted := make([]scoredNode, results.Len())
	for i := len(sorted) - 1; i >= 0; i-- {
		sorted[i] = heap.Pop(results).(scoredNode)
	}
	return sorted
}

func (idx *HNSWIndex) greedyClosest(entryID string, query []float32, level int) string {
	current := entryID
	currentNode := idx.nodes[current]
	if currentNode == nil {
		return entryID
	}
	currentDist := cosineDistance(currentNode.vector, query)

	for {
		improved := false
		node := idx.nodes[current]
		if node == nil || level >= len(node.neighbors) {
			break
		}

		for _, neighborID := range node.neighbors[level] {
			neighbor := idx.nodes[neighborID]
			if neighbor == nil || neighbor.deleted {
				continue
			}
			d := cosineDistance(neighbor.vector, query)
			if d < currentDist {
				current = neighborID
				currentDist = d
				improved = true
			}
		}
		if !improved {
			break
		}
	}
	return current
}

func (idx *HNSWIndex) addConnection(fromID, toID string, level int) {
	node := idx.nodes[fromID]
	if node == nil || level >= len(node.neighbors) {
		return
	}

	// 避免重复
	for _, n := range node.neighbors[level] {
		if n == toID {
			return
		}
	}

	node.neighbors[level] = append(node.neighbors[level], toID)

	// 超过最大连接数时修剪
	maxConn := idx.cfg.MaxConnections
	if level == 0 {
		maxConn = idx.cfg.MaxConnections * 2
	}
	if len(node.neighbors[level]) > maxConn {
		idx.pruneConnections(fromID, level, maxConn)
	}
}

func (idx *HNSWIndex) pruneConnections(nodeID string, level int, maxConn int) {
	node := idx.nodes[nodeID]
	if node == nil {
		return
	}

	// 按距离排序邻居，保留最近的
	type dist struct {
		id string
		d  float32
	}
	dists := make([]dist, 0, len(node.neighbors[level]))
	for _, nID := range node.neighbors[level] {
		n := idx.nodes[nID]
		if n != nil {
			dists = append(dists, dist{nID, cosineDistance(node.vector, n.vector)})
		}
	}
	sort.Slice(dists, func(i, j int) bool { return dists[i].d < dists[j].d })

	kept := make([]string, 0, maxConn)
	for i := 0; i < maxConn && i < len(dists); i++ {
		kept = append(kept, dists[i].id)
	}
	node.neighbors[level] = kept
}

// --- 辅助类型 ---

type scoredNode struct {
	id       string
	distance float32
}

type minHeap []scoredNode
type maxHeap []scoredNode

func (h minHeap) Len() int            { return len(h) }
func (h minHeap) Less(i, j int) bool  { return h[i].distance < h[j].distance }
func (h minHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x interface{}) { *h = append(*h, x.(scoredNode)) }
func (h *minHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

func (h maxHeap) Len() int            { return len(h) }
func (h maxHeap) Less(i, j int) bool  { return h[i].distance > h[j].distance }
func (h maxHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *maxHeap) Push(x interface{}) { *h = append(*h, x.(scoredNode)) }
func (h *maxHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// cosineDistance 余弦距离 = 1 - cosine_similarity
func cosineDistance(a, b []float32) float32 {
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	denom := float32(math.Sqrt(float64(normA) * float64(normB)))
	if denom == 0 {
		return 1.0
	}
	return 1.0 - dot/denom
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

- [x] **Step 4: 运行测试验证通过**

Run: `go test ./internal/memory/ -run TestHNSW -v`
Expected: PASS

- [x] **Step 5: 集成到 VectorStore**

修改 `internal/memory/vector.go`，让 `VectorStore` 可选使用 HNSW 索引：

```go
// 在 VectorStore 中添加 hnsw 字段
type VectorStore struct {
    // ... 现有字段
    hnsw *HNSWIndex
}

// NewVectorStoreWithHNSW 创建带 HNSW 索引的向量存储
func NewVectorStoreWithHNSW(dimensions int, cfg HNSWConfig) *VectorStore {
    cfg.Dimensions = dimensions
    vs := NewVectorStore(dimensions)
    vs.hnsw = NewHNSWIndex(cfg)
    return vs
}
```

- [x] **Step 6: 在 pkg/ 中导出并提交**

```go
// pkg/memory.go 中添加
type HNSWIndex = memory.HNSWIndex
type HNSWConfig = memory.HNSWConfig
type HNSWSearchResult = memory.SearchResult

var NewHNSWIndex = memory.NewHNSWIndex
```

```bash
git add internal/memory/hnsw.go internal/memory/hnsw_test.go internal/memory/vector.go pkg/memory.go
git commit -m "feat: add HNSW vector index for fast approximate nearest neighbor search"
```

---

### Task 10: 记忆自动压缩

**Files:**
- Create: `internal/memory/compressor.go`
- Create: `internal/memory/compressor_test.go`

- [x] **Step 1: 编写压缩测试**

```go
// internal/memory/compressor_test.go
package memory

import (
	"context"
	"testing"
)

func TestCompressor_CompressOldEpisodes(t *testing.T) {
	mem, _ := NewMemoryStore(WithInMemory())
	defer mem.Close()

	ctx := context.Background()

	// 添加 20 条记忆
	for i := 0; i < 20; i++ {
		mem.Add(ctx, &Episode{
			ID:      fmt.Sprintf("ep-%d", i),
			Content: fmt.Sprintf("第 %d 条对话内容，包含一些常见的关键词", i),
			Role:    "user",
		})
	}

	comp := NewCompressor(CompressorConfig{
		WindowSize:   10,
		MinEpisodes:  5,
		Summarizer:   &mockSummarizer{},
	})

	// 压缩前：20 条
	episodes, _ := mem.List(ctx, nil)
	if len(episodes) != 20 {
		t.Fatalf("压缩前 = %d 条, 期望 20", len(episodes))
	}

	// 执行压缩
	err := comp.Compress(ctx, mem)
	if err != nil {
		t.Fatalf("压缩失败: %v", err)
	}

	// 压缩后：旧条目被替换为摘要，总数应减少
	episodes, _ = mem.List(ctx, nil)
	if len(episodes) >= 20 {
		t.Errorf("压缩后 = %d 条, 期望少于 20", len(episodes))
	}
}

func TestCompressor_SkipRecentEpisodes(t *testing.T) {
	mem, _ := NewMemoryStore(WithInMemory())
	defer mem.Close()

	ctx := context.Background()

	// 只添加 3 条（少于窗口大小）
	for i := 0; i < 3; i++ {
		mem.Add(ctx, &Episode{
			ID:      fmt.Sprintf("ep-%d", i),
			Content: fmt.Sprintf("最近的对话 %d", i),
			Role:    "user",
		})
	}

	comp := NewCompressor(CompressorConfig{
		WindowSize:  10,
		MinEpisodes: 5,
		Summarizer:  &mockSummarizer{},
	})

	err := comp.Compress(ctx, mem)
	if err != nil {
		t.Fatalf("压缩失败: %v", err)
	}

	// 条目少不应触发压缩
	episodes, _ := mem.List(ctx, nil)
	if len(episodes) != 3 {
		t.Errorf("不应压缩: %d 条, 期望 3", len(episodes))
	}
}

// mockSummarizer 模拟摘要器
type mockSummarizer struct{}

func (m *mockSummarizer) Summarize(ctx context.Context, episodes []*Episode) (*SummaryResult, error) {
	return &SummaryResult{
		Text: "对话摘要：用户进行了多次交流",
		Tags: []string{"summary"},
	}, nil
}
```

- [x] **Step 2: 实现记忆压缩器**

```go
// internal/memory/compressor.go
package memory

import (
	"context"
	"fmt"
	"time"
)

// CompressorConfig 压缩配置
type CompressorConfig struct {
	WindowSize  int           // 保留最近的 N 条不压缩
	MinEpisodes int           // 最少条目数才触发压缩
	Summarizer  Summarizer    // 摘要提取器
	TTL         time.Duration // 超过此时间的条目可压缩
}

// Compressor 记忆压缩器
type Compressor struct {
	cfg CompressorConfig
}

// NewCompressor 创建压缩器
func NewCompressor(cfg CompressorConfig) *Compressor {
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = 20
	}
	if cfg.MinEpisodes <= 0 {
		cfg.MinEpisodes = 10
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 24 * time.Hour
	}
	return &Compressor{cfg: cfg}
}

// Compress 压缩旧记忆
func (c *Compressor) Compress(ctx context.Context, store MemoryStore) error {
	episodes, err := store.List(ctx, nil)
	if err != nil {
		return fmt.Errorf("list episodes: %w", err)
	}

	if len(episodes) < c.cfg.MinEpisodes {
		return nil // 条目太少，不压缩
	}

	// 按时间排序，分离可压缩和保留的
	cutoff := len(episodes) - c.cfg.WindowSize
	if cutoff <= 0 {
		return nil
	}

	toCompress := episodes[:cutoff]
	if len(toCompress) < 2 {
		return nil
	}

	// 生成摘要
	summary, err := c.cfg.Summarizer.Summarize(ctx, toCompress)
	if err != nil {
		return fmt.Errorf("summarize: %w", err)
	}

	// 删除旧条目
	for _, ep := range toCompress {
		if err := store.Delete(ctx, ep.ID); err != nil {
			return fmt.Errorf("delete episode %s: %w", ep.ID, err)
		}
	}

	// 插入摘要作为新条目
	summaryEpisode := &Episode{
		ID:      fmt.Sprintf("summary-%d", time.Now().UnixNano()),
		Content: summary.Text,
		Role:    "system",
		Metadata: map[string]any{
			"type":          "compressed_summary",
			"source_count":  len(toCompress),
			"tags":          summary.Tags,
			"compressed_at": time.Now().Format(time.RFC3339),
		},
	}

	return store.Add(ctx, summaryEpisode)
}
```

- [x] **Step 3: 运行测试并提交**

Run: `go test ./internal/memory/ -run TestCompressor -v`

```bash
git add internal/memory/compressor.go internal/memory/compressor_test.go
git commit -m "feat: add automatic memory compression with summarization"
```

---

### Task 11: 跨 Agent 记忆共享

**Files:**
- Create: `internal/memory/shared_store.go`
- Create: `internal/memory/shared_store_test.go`

- [x] **Step 1: 编写共享记忆测试**

```go
// internal/memory/shared_store_test.go
package memory

import (
	"context"
	"testing"
)

func TestSharedStore_WriteAndRead(t *testing.T) {
	shared := NewSharedStore()
	mem1, _ := NewMemoryStore(WithInMemory())
	mem2, _ := NewMemoryStore(WithInMemory())

	ctx := context.Background()

	// Agent-1 写入共享记忆
	shared.Bind("agent-1", mem1)
	shared.Publish(ctx, "agent-1", &Episode{
		ID:      "shared-1",
		Content: "共享知识：项目使用 Go 1.26",
		Role:    "user",
		Metadata: map[string]any{"scope": "team"},
	})

	// Agent-2 读取共享记忆
	shared.Bind("agent-2", mem2)
	results, err := shared.SearchShared(ctx, "agent-2", "Go 版本")
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}

	if len(results) == 0 {
		t.Error("Agent-2 应能搜索到 Agent-1 发布的共享记忆")
	}
}

func TestSharedStore_ScopeIsolation(t *testing.T) {
	shared := NewSharedStore()
	mem1, _ := NewMemoryStore(WithInMemory())
	mem2, _ := NewMemoryStore(WithInMemory())

	ctx := context.Background()

	shared.Bind("agent-1", mem1)
	shared.Bind("agent-2", mem2)

	// Agent-1 写入私有记忆（不共享）
	mem1.Add(ctx, &Episode{
		ID:      "private-1",
		Content: "Agent-1 的私有数据",
		Role:    "user",
	})

	// Agent-1 写入共享记忆
	shared.Publish(ctx, "agent-1", &Episode{
		ID:      "shared-1",
		Content: "团队共享数据",
		Role:    "user",
		Metadata: map[string]any{"scope": "team"},
	})

	// Agent-2 搜索只能看到共享的
	results, _ := shared.SearchShared(ctx, "agent-2", "数据")
	for _, r := range results {
		if r.ID == "private-1" {
			t.Error("Agent-2 不应看到 Agent-1 的私有记忆")
		}
	}
}
```

- [x] **Step 2: 实现共享记忆存储**

```go
// internal/memory/shared_store.go
package memory

import (
	"context"
	"fmt"
	"sync"
)

// SharedStore 跨 Agent 共享记忆存储
type SharedStore struct {
	mu       sync.RWMutex
	bindings map[string]MemoryStore // agentID -> store
	shared   MemoryStore            // 全局共享空间
}

// NewSharedStore 创建共享记忆存储
func NewSharedStore() *SharedStore {
	shared, _ := NewMemoryStore(WithInMemory())
	return &SharedStore{
		bindings: make(map[string]MemoryStore),
		shared:   shared,
	}
}

// Bind 绑定 Agent 到其私有存储
func (s *SharedStore) Bind(agentID string, store MemoryStore) {
	s.mu.Lock()
	s.bindings[agentID] = store
	s.mu.Unlock()
}

// Publish 发布共享记忆
func (s *SharedStore) Publish(ctx context.Context, agentID string, episode *Episode) error {
	if episode.Metadata == nil {
		episode.Metadata = make(map[string]any)
	}
	episode.Metadata["published_by"] = agentID
	return s.shared.Add(ctx, episode)
}

// SearchShared 搜索其他 Agent 发布的共享记忆
func (s *SharedStore) SearchShared(ctx context.Context, agentID string, query string) ([]*Episode, error) {
	return s.shared.Search(ctx, query, nil)
}

// GetPrivate 获取 Agent 自己的私有记忆
func (s *SharedStore) GetPrivate(agentID string) (MemoryStore, error) {
	s.mu.RLock()
	store, ok := s.bindings[agentID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("agent %q not bound", agentID)
	}
	return store, nil
}

// Close 关闭共享存储
func (s *SharedStore) Close() error {
	return s.shared.Close()
}
```

- [x] **Step 3: 运行测试并提交**

Run: `go test ./internal/memory/ -run TestSharedStore -v`

```bash
git add internal/memory/shared_store.go internal/memory/shared_store_test.go
git commit -m "feat: add cross-agent shared memory store"
```

---

## Phase 5: 多 Agent 编排增强（第 11-14 周）

### Task 12: DAG 可视化编辑器 HTTP 端点

**Files:**
- Create: `internal/orchestration/visualizer.go`
- Create: `internal/orchestration/visualizer_test.go`
- Create: `internal/orchestration/static/editor.html`（embed）

- [x] **Step 1: 编写可视化测试**

```go
// internal/orchestration/visualizer_test.go
package orchestration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVisualizer_DAGExport(t *testing.T) {
	dag := NewDAGWorkflow("test-wf", WorkflowConfig{})
	dag.AddNode(DAGNode{ID: "step-1", Name: "第一步"})
	dag.AddNode(DAGNode{ID: "step-2", Name: "第二步"})
	dag.AddEdge(DAGEdge{From: "step-1", To: "step-2"})

	v := NewVisualizer(dag)
	export := v.ExportJSON()

	var result map[string]any
	if err := json.Unmarshal([]byte(export), &result); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}

	nodes, ok := result["nodes"].([]any)
	if !ok || len(nodes) != 2 {
		t.Errorf("nodes = %v, 期望 2 个节点", result["nodes"])
	}

	edges, ok := result["edges"].([]any)
	if !ok || len(edges) != 1 {
		t.Errorf("edges = %v, 期望 1 条边", result["edges"])
	}
}

func TestVisualizer_EditorEndpoint(t *testing.T) {
	dag := NewDAGWorkflow("test-wf", WorkflowConfig{})
	dag.AddNode(DAGNode{ID: "step-1", Name: "第一步"})

	v := NewVisualizer(dag)
	handler := v.EditorHandler()

	req := httptest.NewRequest("GET", "/editor", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("状态码 = %d, 期望 %d", w.Code, http.StatusOK)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, 期望 text/html", ct)
	}

	body := w.Body.String()
	if len(body) < 100 {
		t.Error("HTML 内容过短")
	}
}
```

- [x] **Step 2: 实现可视化器**

```go
// internal/orchestration/visualizer.go
package orchestration

import (
	_ "embed"
	"encoding/json"
	"net/http"
)

//go:embed static/editor.html
var editorHTML string

// DAGExport DAG 的 JSON 导出格式
type DAGExport struct {
	Name  string          `json:"name"`
	Nodes []NodeExport    `json:"nodes"`
	Edges []EdgeExport    `json:"edges"`
}

type NodeExport struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Position *Position      `json:"position,omitempty"`
}

type EdgeExport struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition,omitempty"`
}

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Visualizer DAG 可视化器
type Visualizer struct {
	dag *DAGWorkflow
}

// NewVisualizer 创建可视化器
func NewVisualizer(dag *DAGWorkflow) *Visualizer {
	return &Visualizer{dag: dag}
}

// ExportJSON 导出 DAG 为 JSON
func (v *Visualizer) ExportJSON() string {
	export := DAGExport{
		Name:  v.dag.name,
		Nodes: make([]NodeExport, 0),
		Edges: make([]EdgeExport, 0),
	}

	v.dag.mu.RLock()
	for _, node := range v.dag.nodes {
		export.Nodes = append(export.Nodes, NodeExport{
			ID:   node.ID,
			Name: node.Name,
			Type: string(node.Type),
		})
	}
	for _, edge := range v.dag.edges {
		export.Edges = append(export.Edges, EdgeExport{
			From:      edge.From,
			To:        edge.To,
			Condition: edge.Condition,
		})
	}
	v.dag.mu.RUnlock()

	data, _ := json.MarshalIndent(export, "", "  ")
	return string(data)
}

// EditorHandler 返回可视化编辑器 HTTP handler
func (v *Visualizer) EditorHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/editor", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(editorHTML))
	})

	mux.HandleFunc("/api/dag/export", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(v.ExportJSON()))
	})

	return mux
}
```

- [x] **Step 3: 创建编辑器 HTML**

```html
<!-- internal/orchestration/static/editor.html -->
<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="UTF-8">
<title>AP DAG Editor</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: system-ui, sans-serif; background: #1a1a2e; color: #eee; }
  #canvas { width: 100vw; height: 80vh; background: #16213e; position: relative; overflow: hidden; }
  .node { position: absolute; background: #0f3460; border: 2px solid #533483; border-radius: 8px; padding: 12px 20px; cursor: move; min-width: 120px; text-align: center; }
  .node:hover { border-color: #e94560; }
  .toolbar { padding: 12px; background: #0f3460; display: flex; gap: 12px; }
  .toolbar button { padding: 8px 16px; background: #533483; color: white; border: none; border-radius: 4px; cursor: pointer; }
  .toolbar button:hover { background: #e94560; }
  svg { position: absolute; top: 0; left: 0; width: 100%; height: 100%; pointer-events: none; }
  svg line { stroke: #533483; stroke-width: 2; }
</style>
</head>
<body>
<div class="toolbar">
  <button onclick="loadDAG()">Reload</button>
  <button onclick="exportJSON()">Export JSON</button>
</div>
<div id="canvas">
  <svg id="edges"></svg>
</div>
<script>
let dagData = null;
async function loadDAG() {
  const resp = await fetch('/api/dag/export');
  dagData = await resp.json();
  renderDAG();
}
function renderDAG() {
  const canvas = document.getElementById('canvas');
  canvas.querySelectorAll('.node').forEach(n => n.remove());
  dagData.nodes.forEach((node, i) => {
    const el = document.createElement('div');
    el.className = 'node';
    el.id = 'node-' + node.id;
    el.textContent = node.name || node.id;
    el.style.left = (100 + i * 200) + 'px';
    el.style.top = '200px';
    makeDraggable(el);
    canvas.appendChild(el);
  });
  renderEdges();
}
function renderEdges() {
  const svg = document.getElementById('edges');
  svg.innerHTML = '';
  dagData.edges.forEach(edge => {
    const from = document.getElementById('node-' + edge.from);
    const to = document.getElementById('node-' + edge.to);
    if (!from || !to) return;
    const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
    line.setAttribute('x1', from.offsetLeft + from.offsetWidth / 2);
    line.setAttribute('y1', from.offsetTop + from.offsetHeight / 2);
    line.setAttribute('x2', to.offsetLeft + to.offsetWidth / 2);
    line.setAttribute('y2', to.offsetTop + to.offsetHeight / 2);
    svg.appendChild(line);
  });
}
function makeDraggable(el) {
  let offsetX, offsetY, dragging = false;
  el.addEventListener('mousedown', e => {
    dragging = true;
    offsetX = e.clientX - el.offsetLeft;
    offsetY = e.clientY - el.offsetTop;
  });
  document.addEventListener('mousemove', e => {
    if (!dragging) return;
    el.style.left = (e.clientX - offsetX) + 'px';
    el.style.top = (e.clientY - offsetY) + 'px';
    renderEdges();
  });
  document.addEventListener('mouseup', () => { dragging = false; });
}
function exportJSON() {
  const pre = document.createElement('pre');
  pre.textContent = JSON.stringify(dagData, null, 2);
  document.body.appendChild(pre);
}
loadDAG();
</script>
</body>
</html>
```

- [x] **Step 4: 运行测试并提交**

Run: `go test ./internal/orchestration/ -run TestVisualizer -v`

```bash
git add internal/orchestration/visualizer.go internal/orchestration/visualizer_test.go internal/orchestration/static/editor.html
git commit -m "feat: add DAG visual editor with drag-and-drop HTML UI"
```

---

### Task 13: 动态编排拓扑调整

**Files:**
- Create: `internal/orchestration/dynamic.go`
- Create: `internal/orchestration/dynamic_test.go`

- [x] **Step 1: 编写动态编排测试**

```go
// internal/orchestration/dynamic_test.go
package orchestration

import (
	"context"
	"testing"
)

func TestDynamicDAG_AddNodeAtRuntime(t *testing.T) {
	dag := NewDynamicDAG("dynamic-wf", WorkflowConfig{})

	dag.AddNode(NodeHandler{ID: "step-1", Handler: func(ctx context.Context, input any) (any, error) {
		return "result-1", nil
	}})

	// 运行时添加节点
	dag.AddNode(NodeHandler{ID: "step-2", Handler: func(ctx context.Context, input any) (any, error) {
		return "result-2", nil
	}})
	dag.AddEdge("step-1", "step-2")

	if dag.NodeCount() != 2 {
		t.Errorf("节点数 = %d, 期望 2", dag.NodeCount())
	}
}

func TestDynamicDAG_RemoveNodeAtRuntime(t *testing.T) {
	dag := NewDynamicDAG("dynamic-wf", WorkflowConfig{})

	dag.AddNode(NodeHandler{ID: "step-1", Handler: noopHandler})
	dag.AddNode(NodeHandler{ID: "step-2", Handler: noopHandler})
	dag.AddEdge("step-1", "step-2")

	// 运行时移除节点
	dag.RemoveNode("step-2")

	if dag.NodeCount() != 1 {
		t.Errorf("移除后节点数 = %d, 期望 1", dag.NodeCount())
	}
}

func TestDynamicDAG_ConditionalRouting(t *testing.T) {
	dag := NewDynamicDAG("router-wf", WorkflowConfig{})

	executed := ""
	dag.AddNode(NodeHandler{ID: "router", Handler: func(ctx context.Context, input any) (any, error) {
		return "go-b", nil
	}})
	dag.AddNode(NodeHandler{ID: "branch-a", Handler: func(ctx context.Context, input any) (any, error) {
		executed = "a"
		return nil, nil
	}})
	dag.AddNode(NodeHandler{ID: "branch-b", Handler: func(ctx context.Context, input any) (any, error) {
		executed = "b"
		return nil, nil
	}})

	dag.AddConditionalEdge("router", map[string]string{
		"go-a": "branch-a",
		"go-b": "branch-b",
	})

	result, err := dag.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	_ = result

	if executed != "b" {
		t.Errorf("应执行 branch-b, 实际执行 = %q", executed)
	}
}

func noopHandler(ctx context.Context, input any) (any, error) {
	return nil, nil
}
```

- [x] **Step 2: 实现动态 DAG**

```go
// internal/orchestration/dynamic.go
package orchestration

import (
	"context"
	"fmt"
	"sync"
)

// DynamicDAG 支持运行时修改拓扑的 DAG
type DynamicDAG struct {
	mu         sync.RWMutex
	name       string
	cfg        WorkflowConfig
	nodes      map[string]*dynamicNode
	edges      map[string][]string         // from -> []to
	conditions map[string]map[string]string // from -> {output -> toID}
}

type dynamicNode struct {
	handler NodeHandler
}

// NewDynamicDAG 创建动态 DAG
func NewDynamicDAG(name string, cfg WorkflowConfig) *DynamicDAG {
	return &DynamicDAG{
		name:       name,
		cfg:        cfg,
		nodes:      make(map[string]*dynamicNode),
		edges:      make(map[string][]string),
		conditions: make(map[string]map[string]string),
	}
}

// AddNode 添加节点（线程安全）
func (d *DynamicDAG) AddNode(handler NodeHandler) {
	d.mu.Lock()
	d.nodes[handler.ID] = &dynamicNode{handler: handler}
	d.mu.Unlock()
}

// RemoveNode 移除节点及其相关边
func (d *DynamicDAG) RemoveNode(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.nodes, id)
	delete(d.edges, id)
	delete(d.conditions, id)

	// 清理指向该节点的边
	for from, tos := range d.edges {
		filtered := make([]string, 0)
		for _, to := range tos {
			if to != id {
				filtered = append(filtered, to)
			}
		}
		d.edges[from] = filtered
	}
}

// AddEdge 添加边
func (d *DynamicDAG) AddEdge(from, to string) {
	d.mu.Lock()
	d.edges[from] = append(d.edges[from], to)
	d.mu.Unlock()
}

// AddConditionalEdge 添加条件边
func (d *DynamicDAG) AddConditionalEdge(from string, routing map[string]string) {
	d.mu.Lock()
	d.conditions[from] = routing
	d.mu.Unlock()
}

// NodeCount 返回节点数
func (d *DynamicDAG) NodeCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.nodes)
}

// Execute 执行 DAG
func (d *DynamicDAG) Execute(ctx context.Context, input any) (any, error) {
	d.mu.RLock()
	startNodes := d.findStartNodes()
	d.mu.RUnlock()

	if len(startNodes) == 0 {
		return nil, fmt.Errorf("no start nodes found")
	}

	current := input
	for _, startID := range startNodes {
		result, err := d.executeFrom(ctx, startID, current)
		if err != nil {
			return nil, err
		}
		current = result
	}
	return current, nil
}

func (d *DynamicDAG) executeFrom(ctx context.Context, nodeID string, input any) (any, error) {
	d.mu.RLock()
	node, ok := d.nodes[nodeID]
	d.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("node %q not found", nodeID)
	}

	result, err := node.handler(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("node %q failed: %w", nodeID, err)
	}

	// 检查条件路由
	d.mu.RLock()
	routing, hasCondition := d.conditions[nodeID]
	d.mu.RUnlock()

	if hasCondition {
		routeKey := fmt.Sprintf("%v", result)
		if nextID, ok := routing[routeKey]; ok {
			return d.executeFrom(ctx, nextID, result)
		}
	}

	// 普通边
	d.mu.RLock()
	nexts := d.edges[nodeID]
	d.mu.RUnlock()

	for _, nextID := range nexts {
		r, err := d.executeFrom(ctx, nextID, result)
		if err != nil {
			return nil, err
		}
		result = r
	}

	return result, nil
}

func (d *DynamicDAG) findStartNodes() []string {
	hasIncoming := make(map[string]bool)
	for _, tos := range d.edges {
		for _, to := range tos {
			hasIncoming[to] = true
		}
	}

	var starts []string
	for id := range d.nodes {
		if !hasIncoming[id] {
			starts = append(starts, id)
		}
	}
	return starts
}
```

- [x] **Step 3: 运行测试并提交**

Run: `go test ./internal/orchestration/ -run TestDynamicDAG -v`

```bash
git add internal/orchestration/dynamic.go internal/orchestration/dynamic_test.go
git commit -m "feat: add dynamic DAG with runtime topology modification"
```

---

## Phase 6: 开发者体验（第 15-18 周）

### Task 14: CLI 交互式向导

**Files:**
- Modify: `cmd/ap/init.go`
- Create: `cmd/ap/interactive.go`
- Create: `cmd/ap/interactive_test.go`

- [x] **Step 1: 编写交互式向导测试**

```go
// cmd/ap/interactive_test.go
package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestInteractiveWizard_BasicFlow(t *testing.T) {
	input := bytes.NewBufferString("my-agent\nbasic\n")
	output := &bytes.Buffer{}

	wizard := NewWizard(input, output)
	opts, err := wizard.Run()
	if err != nil {
		t.Fatalf("向导运行失败: %v", err)
	}

	if opts.Name != "my-agent" {
		t.Errorf("Name = %q, 期望 my-agent", opts.Name)
	}
	if opts.Template != "basic" {
		t.Errorf("Template = %q, 期望 basic", opts.Template)
	}
}

func TestInteractiveWizard_ShowTemplates(t *testing.T) {
	input := bytes.NewBufferString("demo\nquickstart\n")
	output := &bytes.Buffer{}

	wizard := NewWizard(input, output)
	wizard.Run()

	prompt := output.String()
	if !strings.Contains(prompt, "quickstart") {
		t.Error("应显示 quickstart 模板选项")
	}
	if !strings.Contains(prompt, "basic") {
		t.Error("应显示 basic 模板选项")
	}
}
```

- [x] **Step 2: 实现交互式向导**

```go
// cmd/ap/interactive.go
package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Wizard 交互式项目创建向导
type Wizard struct {
	reader *bufio.Reader
	writer io.Writer
}

// NewWizard 创建向导
func NewWizard(input io.Reader, output io.Writer) *Wizard {
	return &Wizard{
		reader: bufio.NewReader(input),
		writer: output,
	}
}

// Run 运行向导，返回生成选项
func (w *Wizard) Run() (*GenerateOptions, error) {
	templates := []struct {
		name string
		desc string
	}{
		{"quickstart", "5 分钟快速入门（推荐新手）"},
		{"basic", "最小化 Agent"},
		{"with-tools", "带工具的 Agent（文件系统 + Shell + Web）"},
		{"multi-agent", "多 Agent 协作"},
		{"agent-with-cache", "Agent + LLM 响应缓存"},
		{"agent-with-rag", "Agent + 知识检索（RAG）"},
		{"agent-with-metrics", "Agent + Prometheus 指标"},
	}

	fmt.Fprintln(w.writer, "🚀 AgentPrimordia 项目创建向导")
	fmt.Fprintln(w.writer)

	// 步骤 1：项目名
	fmt.Fprint(w.writer, "项目名称: ")
	name, _ := w.reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("项目名称不能为空")
	}

	// 步骤 2：选择模板
	fmt.Fprintln(w.writer, "\n可用模板:")
	for i, t := range templates {
		fmt.Fprintf(w.writer, "  %d. %-20s %s\n", i+1, t.name, t.desc)
	}
	fmt.Fprint(w.writer, "\n选择模板 (1-7, 默认 1): ")
	choice, _ := w.reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	idx := 0
	if choice != "" {
		fmt.Sscanf(choice, "%d", &idx)
		idx--
		if idx < 0 || idx >= len(templates) {
			idx = 0
		}
	}

	template := templates[idx].name

	fmt.Fprintf(w.writer, "\n✅ 将创建项目 %q (模板: %s)\n", name, template)

	return &GenerateOptions{
		Name:     name,
		Template: template,
	}, nil
}
```

- [x] **Step 3: 运行测试并提交**

Run: `go test ./cmd/ap/ -run TestInteractiveWizard -v`

```bash
git add cmd/ap/interactive.go cmd/ap/interactive_test.go
git commit -m "feat: add interactive project creation wizard"
```

---

## 验收标准

完成所有 Phase 后：

1. `go vet ./...` 和 `go build ./...` 通过
2. 所有新测试通过
3. HNSW 索引在 1000 向量数据集上 Recall@10 >= 0.8
4. 记忆压缩正确减少条目数
5. 跨 Agent 共享记忆正确隔离私有/共享数据
6. DAG 可视化编辑器在浏览器中正确渲染
7. 动态 DAG 支持运行时增删节点和条件路由
8. CLI 向导正确引导用户创建项目
