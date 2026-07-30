package memory

import (
	"cmp"
	"container/heap"
	"context"
	"math"
	"math/rand"
	"slices"
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
	id        string
	vector    []float32
	metadata  map[string]string
	neighbors [][]string // 每层的邻居列表
	level     int
	deleted   bool
}

// HNSWSearchResult HNSW 搜索结果
type HNSWSearchResult struct {
	ID       string
	Distance float32
	Metadata map[string]string
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
func (idx *HNSWIndex) Insert(ctx context.Context, id string, vector []float32, metadata map[string]string) {
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

	for lev := hnswMin(level, idx.maxLvl); lev >= 0; lev-- {
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
func (idx *HNSWIndex) Search(ctx context.Context, query []float32, k int) []HNSWSearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.entry == nil || len(idx.nodes) == 0 {
		return nil
	}

	ep := idx.entry.id
	for lev := idx.maxLvl; lev > 0; lev-- {
		ep = idx.greedyClosest(ep, query, lev)
	}

	candidates := idx.searchLayer(ep, query, hnswMax(k, idx.cfg.EfSearch), 0)

	// 返回 top-k
	results := make([]HNSWSearchResult, 0, k)
	for i := 0; i < k && i < len(candidates); i++ {
		c := candidates[i]
		if node, ok := idx.nodes[c.id]; ok && !node.deleted {
			results = append(results, HNSWSearchResult{
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

func (idx *HNSWIndex) searchLayer(entryID string, query []float32, ef int, level int) []hnswScoredNode {
	visited := map[string]bool{entryID: true}
	// candidates: 最小堆，优先探索最近的候选
	candidates := &hnswMinHeap{}
	// results: 最大堆，保留 ef 个最近邻，堆顶是结果中最远的
	results := &hnswMaxHeap{}

	entryNode := idx.nodes[entryID]
	if entryNode == nil {
		return nil
	}
	dist := hnswCosineDistance(entryNode.vector, query)
	heap.Push(candidates, hnswScoredNode{entryID, dist})
	heap.Push(results, hnswScoredNode{entryID, dist})

	for candidates.Len() > 0 {
		current := heap.Pop(candidates).(hnswScoredNode)

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

			d := hnswCosineDistance(neighbor.vector, query)

			if results.Len() < ef || d < (*results)[0].distance {
				heap.Push(candidates, hnswScoredNode{neighborID, d})
				heap.Push(results, hnswScoredNode{neighborID, d})
				if results.Len() > ef {
					heap.Pop(results)
				}
			}
		}
	}

	// 从最大堆中提取并按距离升序排列
	sorted := make([]hnswScoredNode, results.Len())
	for i := range sorted {
		sorted[i] = heap.Pop(results).(hnswScoredNode)
	}
	// 最大堆 Pop 出来是降序，需要反转为升序
	for i, j := 0, len(sorted)-1; i < j; i, j = i+1, j-1 {
		sorted[i], sorted[j] = sorted[j], sorted[i]
	}
	return sorted
}

func (idx *HNSWIndex) greedyClosest(entryID string, query []float32, level int) string {
	current := entryID
	currentNode := idx.nodes[current]
	if currentNode == nil {
		return entryID
	}
	currentDist := hnswCosineDistance(currentNode.vector, query)

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
			d := hnswCosineDistance(neighbor.vector, query)
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
			dists = append(dists, dist{nID, hnswCosineDistance(node.vector, n.vector)})
		}
	}
	// 优化（Task 3.7）：使用泛型 slices.SortFunc 替代 sort.Slice，避免反射开销
	slices.SortFunc(dists, func(a, b dist) int { return cmp.Compare(a.d, b.d) })

	kept := make([]string, 0, maxConn)
	for i := 0; i < maxConn && i < len(dists); i++ {
		kept = append(kept, dists[i].id)
	}
	node.neighbors[level] = kept
}

// --- 辅助类型 ---

type hnswScoredNode struct {
	id       string
	distance float32
}

type hnswMinHeap []hnswScoredNode
type hnswMaxHeap []hnswScoredNode

func (h hnswMinHeap) Len() int            { return len(h) }
func (h hnswMinHeap) Less(i, j int) bool  { return h[i].distance < h[j].distance }
func (h hnswMinHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *hnswMinHeap) Push(x interface{}) { *h = append(*h, x.(hnswScoredNode)) }
func (h *hnswMinHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

func (h hnswMaxHeap) Len() int            { return len(h) }
func (h hnswMaxHeap) Less(i, j int) bool  { return h[i].distance > h[j].distance }
func (h hnswMaxHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *hnswMaxHeap) Push(x interface{}) { *h = append(*h, x.(hnswScoredNode)) }
func (h *hnswMaxHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// hnswCosineDistance 余弦距离 = 1 - cosine_similarity
func hnswCosineDistance(a, b []float32) float32 {
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

func hnswMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func hnswMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
