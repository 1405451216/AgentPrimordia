package cluster

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
)

// ConsistentHash 一致性哈希环
//
// 用于将 Agent/任务分片到不同节点。
// 支持虚拟节点以平衡负载分布。
type ConsistentHash struct {
	mu       sync.RWMutex
	replicas int               // 每个节点的虚拟节点数
	ring     []uint32          // 排序的哈希环
	hashMap  map[uint32]string // 哈希值 -> 节点ID
	nodeSet  map[string]bool   // 节点ID集合
}

// NewConsistentHash 创建一致性哈希环
func NewConsistentHash(replicas int) *ConsistentHash {
	if replicas <= 0 {
		replicas = 32
	}
	return &ConsistentHash{
		replicas: replicas,
		hashMap:  make(map[uint32]string),
		nodeSet:  make(map[string]bool),
	}
}

// AddNode 添加节点到哈希环
func (h *ConsistentHash) AddNode(nodeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.nodeSet[nodeID] {
		return // 已存在
	}

	h.nodeSet[nodeID] = true

	// 为每个虚拟节点计算哈希
	for i := 0; i < h.replicas; i++ {
		hash := h.hash(fmt.Sprintf("%s#%d", nodeID, i))
		h.ring = append(h.ring, hash)
		h.hashMap[hash] = nodeID
	}

	// 重新排序
	sort.Slice(h.ring, func(i, j int) bool { return h.ring[i] < h.ring[j] })
}

// RemoveNode 从哈希环移除节点
func (h *ConsistentHash) RemoveNode(nodeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.nodeSet[nodeID] {
		return // 不存在
	}

	delete(h.nodeSet, nodeID)

	// 移除所有虚拟节点
	newRing := make([]uint32, 0, len(h.ring))
	for _, hash := range h.ring {
		if h.hashMap[hash] == nodeID {
			delete(h.hashMap, hash)
		} else {
			newRing = append(newRing, hash)
		}
	}
	h.ring = newRing
}

// GetNode 获取负责指定 key 的节点
func (h *ConsistentHash) GetNode(key string) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.ring) == 0 {
		return "", false
	}

	hash := h.hash(key)

	// 二分查找第一个 >= hash 的位置
	idx := sort.Search(len(h.ring), func(i int) bool {
		return h.ring[i] >= hash
	})

	// 环回绕
	if idx >= len(h.ring) {
		idx = 0
	}

	return h.hashMap[h.ring[idx]], true
}

// GetNodes 获取负责指定 key 的前 N 个不同节点（用于副本）
func (h *ConsistentHash) GetNodes(key string, count int) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.ring) == 0 || count <= 0 {
		return nil
	}

	hash := h.hash(key)
	idx := sort.Search(len(h.ring), func(i int) bool {
		return h.ring[i] >= hash
	})

	seen := make(map[string]bool)
	result := make([]string, 0, count)

	for i := 0; i < len(h.ring) && len(result) < count; i++ {
		pos := (idx + i) % len(h.ring)
		nodeID := h.hashMap[h.ring[pos]]
		if !seen[nodeID] {
			seen[nodeID] = true
			result = append(result, nodeID)
		}
	}

	return result
}

// GetNodesList 获取环上所有节点（去重）
func (h *ConsistentHash) GetNodesList() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]string, 0, len(h.nodeSet))
	for nodeID := range h.nodeSet {
		result = append(result, nodeID)
	}
	return result
}

// RingSize 返回环上的虚拟节点数
func (h *ConsistentHash) RingSize() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.ring)
}

// hash 计算字符串的 FNV-1a 哈希
func (h *ConsistentHash) hash(s string) uint32 {
	f := fnv.New32a()
	f.Write([]byte(s))
	return f.Sum32()
}
