package cluster

import (
	"sync"
	"time"
)

// DistributedState 分布式 KV 状态存储
//
// 提供带 TTL 的分布式状态存储。
// 在领导者节点上写入，其他节点通过同步获取。
type DistributedState struct {
	mu   sync.RWMutex
	data map[string]*stateEntry
}

type stateEntry struct {
	value     string
	expiresAt time.Time // 零值表示永不过期
	version   uint64    // 版本号，用于冲突解决
}

// NewDistributedState 创建分布式状态存储
func NewDistributedState() *DistributedState {
	return &DistributedState{
		data: make(map[string]*stateEntry),
	}
}

// Set 设置键值（带 TTL）
// 如果 TTL 为 0，表示永不过期
func (s *DistributedState) Set(key, value string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	expiresAt := time.Time{}
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	// 如果已存在，增加版本号
	version := uint64(1)
	if existing, ok := s.data[key]; ok {
		version = existing.version + 1
	}

	s.data[key] = &stateEntry{
		value:     value,
		expiresAt: expiresAt,
		version:   version,
	}
}

// Get 获取键值
// 返回值和是否存在（未过期）
func (s *DistributedState) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.data[key]
	if !ok {
		return "", false
	}

	// 检查是否过期
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		return "", false
	}

	return entry.value, true
}

// GetWithVersion 获取键值和版本号
func (s *DistributedState) GetWithVersion(key string) (string, uint64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.data[key]
	if !ok {
		return "", 0, false
	}

	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		return "", 0, false
	}

	return entry.value, entry.version, true
}

// Delete 删除键
func (s *DistributedState) Delete(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.data[key]
	if exists {
		delete(s.data, key)
	}
	return exists
}

// Keys 获取所有未过期的键
func (s *DistributedState) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	result := make([]string, 0, len(s.data))
	for key, entry := range s.data {
		if entry.expiresAt.IsZero() || now.Before(entry.expiresAt) {
			result = append(result, key)
		}
	}
	return result
}

// Size 返回未过期的键数量
func (s *DistributedState) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	count := 0
	for _, entry := range s.data {
		if entry.expiresAt.IsZero() || now.Before(entry.expiresAt) {
			count++
		}
	}
	return count
}

// Cleanup 清理过期的键
func (s *DistributedState) Cleanup() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	count := 0
	for key, entry := range s.data {
		if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
			delete(s.data, key)
			count++
		}
	}
	return count
}

// Snapshot 获取状态快照（用于同步）
func (s *DistributedState) Snapshot() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	result := make(map[string]string)
	for key, entry := range s.data {
		if entry.expiresAt.IsZero() || now.Before(entry.expiresAt) {
			result[key] = entry.value
		}
	}
	return result
}

// Merge 合并其他节点的状态（基于版本号冲突解决）
func (s *DistributedState) Merge(other map[string]RemoteEntry) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	merged := 0
	for key, remote := range other {
		local, exists := s.data[key]

		if !exists || remote.Version > local.version {
			// 远程版本更新，采用远程值
			s.data[key] = &stateEntry{
				value:     remote.Value,
				expiresAt: remote.ExpiresAt,
				version:   remote.Version,
			}
			merged++
		}
	}
	return merged
}

// RemoteEntry 远程状态条目（用于同步）
type RemoteEntry struct {
	Value     string
	ExpiresAt time.Time
	Version   uint64
}

// ExportForSync 导出状态用于同步
func (s *DistributedState) ExportForSync() map[string]RemoteEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	result := make(map[string]RemoteEntry)
	for key, entry := range s.data {
		if entry.expiresAt.IsZero() || now.Before(entry.expiresAt) {
			result[key] = RemoteEntry{
				Value:     entry.value,
				ExpiresAt: entry.expiresAt,
				Version:   entry.version,
			}
		}
	}
	return result
}
