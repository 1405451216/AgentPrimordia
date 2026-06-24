package memory

import (
	"context"
	"fmt"
	"sync"
)

// SharedStore 跨 Agent 共享记忆存储
type SharedStore struct {
	mu       sync.RWMutex
	bindings map[string]Memory // agentID -> store
	shared   Memory            // 全局共享空间
}

// NewSharedStore 创建共享记忆存储
func NewSharedStore() *SharedStore {
	return &SharedStore{
		bindings: make(map[string]Memory),
		shared:   NewInMemoryStore(),
	}
}

// Bind 绑定 Agent 到其私有存储
func (s *SharedStore) Bind(agentID string, store Memory) {
	s.mu.Lock()
	s.bindings[agentID] = store
	s.mu.Unlock()
}

// Publish 发布共享记忆
func (s *SharedStore) Publish(ctx context.Context, agentID string, episode *Episode) error {
	if episode.Metadata == nil {
		episode.Metadata = make(map[string]string)
	}
	episode.Metadata["published_by"] = agentID
	return s.shared.Add(ctx, episode)
}

// SearchShared 搜索其他 Agent 发布的共享记忆
func (s *SharedStore) SearchShared(ctx context.Context, agentID string, query string) ([]*Episode, error) {
	return s.shared.Search(ctx, query, nil)
}

// GetPrivate 获取 Agent 自己的私有记忆
func (s *SharedStore) GetPrivate(agentID string) (Memory, error) {
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
