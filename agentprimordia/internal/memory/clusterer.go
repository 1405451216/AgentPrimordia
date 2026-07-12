// clusterer.go — 语义聚类器（分层记忆深化的聚类层）。
//
// 对 Memory 条目列表执行聚类，输出 MemoryCluster 列表。
// 支持两种算法：DBSCAN（基于密度）和 Agglomerative（层次聚合）。
// 优先使用 Embedding 做余弦相似度；无 Embedding 则退化到关键词重叠。

package memory

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

// ClusterAlgorithm 是聚类算法类型。
type ClusterAlgorithm string

const (
	DBSCAN        ClusterAlgorithm = "dbscan"
	Agglomerative ClusterAlgorithm = "agglomerative"
)

// semanticClusterer 是语义聚类器。
type semanticClusterer struct {
	Threshold float64
	Algorithm ClusterAlgorithm
	MinPoints int
}

// NewSemanticClusterer 创建聚类器，默认使用 DBSCAN。
func NewSemanticClusterer(threshold float64, algo ClusterAlgorithm) *semanticClusterer {
	if threshold <= 0 {
		threshold = 0.5
	}
	if algo == "" {
		algo = DBSCAN
	}
	return &semanticClusterer{
		Threshold: threshold,
		Algorithm: algo,
		MinPoints: 2,
	}
}

// MemoryCluster 是聚类结果。
type MemoryCluster struct {
	ID      string
	Center  []float32
	Members []*memoryDoc
	Topic   string
}

// memoryDoc 是带 embedding 的记忆条目。
type memoryDoc struct {
	ID         string
	SessionID  string
	Role       string
	Content    string
	Summary    string
	Topics     string
	Importance float64
	Metadata   map[string]string
	CreatedAt  string
	Embedding  []float32
}

// newMemoryDoc 从 Episode 创建 memoryDoc。
func newMemoryDoc(ep *Episode, embedding []float32) *memoryDoc {
	return &memoryDoc{
		ID:         ep.ID,
		SessionID:  ep.SessionID,
		Role:       ep.Role,
		Content:    ep.Content,
		Summary:    ep.Summary,
		Topics:     ep.Topics,
		Importance: ep.Importance,
		Metadata:   ep.Metadata,
		CreatedAt:  ep.CreatedAt,
		Embedding:  embedding,
	}
}

var clusterIDCounter atomic.Int64

func generateClusterID() string {
	n := clusterIDCounter.Add(1)
	return "cl_" + strconv.FormatInt(n, 10)
}

// Cluster 对记忆列表执行聚类。
func (c *semanticClusterer) Cluster(docs []*memoryDoc) ([]*MemoryCluster, error) {
	if len(docs) == 0 {
		return nil, nil
	}

	filtered := make([]*memoryDoc, 0, len(docs))
	for _, m := range docs {
		if m != nil {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) == 0 {
		return nil, nil
	}

	switch c.Algorithm {
	case Agglomerative:
		return c.agglomerative(filtered)
	case DBSCAN:
		return c.dbscan(filtered)
	default:
		return nil, errors.New("clusterer: unknown algorithm")
	}
}

func (c *semanticClusterer) dbscan(docs []*memoryDoc) ([]*MemoryCluster, error) {
	n := len(docs)
	visited := make([]bool, n)
	clusterLabels := make([]int, n)
	for i := range clusterLabels {
		clusterLabels[i] = -1
	}

	hasEmbeddings := false
	for _, m := range docs {
		if len(m.Embedding) > 0 {
			hasEmbeddings = true
			break
		}
	}

	simFunc := func(i, j int) float64 {
		if hasEmbeddings {
			return float64(cosineSimilarity(docs[i].Embedding, docs[j].Embedding))
		}
		return keywordOverlapMemoryDoc(docs[i], docs[j])
	}

	label := 0
	for i := 0; i < n; i++ {
		if visited[i] {
			continue
		}
		visited[i] = true

		neighbors := c.regionQuery(i, n, simFunc)
		if len(neighbors) < c.MinPoints {
			continue
		}

		clusterLabels[i] = label
		seed := make([]int, len(neighbors))
		copy(seed, neighbors)

		for j := 0; j < len(seed); j++ {
			q := seed[j]
			if !visited[q] {
				visited[q] = true
				qNeighbors := c.regionQuery(q, n, simFunc)
				if len(qNeighbors) >= c.MinPoints {
					seed = append(seed, qNeighbors...)
				}
			}
			if clusterLabels[q] == -1 {
				clusterLabels[q] = label
			}
		}
		label++
	}

	for i := 0; i < n; i++ {
		if clusterLabels[i] == -1 {
			bestCluster := c.nearestCluster(i, docs, clusterLabels, simFunc)
			if bestCluster >= 0 {
				clusterLabels[i] = bestCluster
			} else {
				clusterLabels[i] = label
				label++
			}
		}
	}

	return c.buildClusters(docs, clusterLabels, label), nil
}

func (c *semanticClusterer) regionQuery(i, n int, simFunc func(i, j int) float64) []int {
	var neighbors []int
	for j := 0; j < n; j++ {
		if i == j {
			continue
		}
		if simFunc(i, j) >= c.Threshold {
			neighbors = append(neighbors, j)
		}
	}
	return neighbors
}

func (c *semanticClusterer) nearestCluster(i int, docs []*memoryDoc, labels []int, simFunc func(i, j int) float64) int {
	bestLabel := -1
	bestSim := -1.0
	for j := range docs {
		if i == j || labels[j] == -1 {
			continue
		}
		sim := simFunc(i, j)
		if sim > bestSim {
			bestSim = sim
			bestLabel = labels[j]
		}
	}
	return bestLabel
}

func (c *semanticClusterer) buildClusters(docs []*memoryDoc, labels []int, numClusters int) []*MemoryCluster {
	clusterMap := make(map[int][]*memoryDoc, numClusters)
	for i, label := range labels {
		clusterMap[label] = append(clusterMap[label], docs[i])
	}

	result := make([]*MemoryCluster, 0, len(clusterMap))
	for _, members := range clusterMap {
		if len(members) == 0 {
			continue
		}
		cluster := &MemoryCluster{
			ID:      generateClusterID(),
			Members: members,
		}
		cluster.Topic = cluster.extractTopic()
		result = append(result, cluster)
	}

	return result
}

func (c *semanticClusterer) agglomerative(docs []*memoryDoc) ([]*MemoryCluster, error) {
	n := len(docs)
	if n == 0 {
		return nil, nil
	}

	hasEmbeddings := false
	for _, m := range docs {
		if len(m.Embedding) > 0 {
			hasEmbeddings = true
			break
		}
	}

	simFunc := func(i, j int) float64 {
		if hasEmbeddings {
			return float64(cosineSimilarity(docs[i].Embedding, docs[j].Embedding))
		}
		return keywordOverlapMemoryDoc(docs[i], docs[j])
	}

	parent := make([]int, n)
	rank := make([]int, n)
	for i := range parent {
		parent[i] = i
	}

	type pair struct {
		i, j int
		sim  float64
	}
	pairs := make([]pair, 0, n*(n-1)/2)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			pairs = append(pairs, pair{i, j, simFunc(i, j)})
		}
	}
	sort.Slice(pairs, func(a, b int) bool {
		return pairs[a].sim > pairs[b].sim
	})

	find := func(x int) int {
		root := x
		for parent[root] != root {
			root = parent[root]
		}
		for parent[x] != root {
			next := parent[x]
			parent[x] = root
			x = next
		}
		return root
	}

	for _, p := range pairs {
		if p.sim < c.Threshold {
			break
		}
		rx, ry := find(p.i), find(p.j)
		if rx == ry {
			continue
		}
		if rank[rx] < rank[ry] {
			parent[rx] = ry
		} else if rank[rx] > rank[ry] {
			parent[ry] = rx
		} else {
			parent[ry] = rx
			rank[rx]++
		}
	}

	clusterMap := make(map[int][]*memoryDoc)
	for i := range docs {
		root := find(i)
		clusterMap[root] = append(clusterMap[root], docs[i])
	}

	result := make([]*MemoryCluster, 0, len(clusterMap))
	for _, members := range clusterMap {
		cluster := &MemoryCluster{
			ID:      generateClusterID(),
			Members: members,
		}
		cluster.Topic = cluster.extractTopic()
		result = append(result, cluster)
	}

	return result, nil
}

func (c *MemoryCluster) extractTopic() string {
	freq := make(map[string]int)
	for _, m := range c.Members {
		for _, kw := range extractKeywordsFromMem(m) {
			freq[kw]++
		}
	}
	if len(freq) == 0 {
		return ""
	}

	type kv struct {
		key string
		val int
	}
	sorted := make([]kv, 0, len(freq))
	for k, v := range freq {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(a, b int) bool {
		return sorted[a].val > sorted[b].val
	})

	top := 3
	if len(sorted) < top {
		top = len(sorted)
	}
	topWords := make([]string, top)
	for i := 0; i < top; i++ {
		topWords[i] = sorted[i].key
	}
	return strings.Join(topWords, ",")
}

func extractKeywordsFromMem(mem *memoryDoc) []string {
	text := mem.Content + " " + mem.Summary + " " + mem.Topics
	tokens := tokenizeRe.Split(strings.ToLower(text), -1)
	var result []string
	for _, t := range tokens {
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}

func keywordOverlapMemoryDoc(a, b *memoryDoc) float64 {
	wordsA := extractKeywordsFromMem(a)
	wordsB := extractKeywordsFromMem(b)

	setA := make(map[string]struct{}, len(wordsA))
	for _, w := range wordsA {
		setA[w] = struct{}{}
	}
	setB := make(map[string]struct{}, len(wordsB))
	for _, w := range wordsB {
		setB[w] = struct{}{}
	}

	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}

	intersection := 0
	for w := range setA {
		if _, ok := setB[w]; ok {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}