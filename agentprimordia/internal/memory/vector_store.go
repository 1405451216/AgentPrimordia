package memory

import (
	"context"
	"errors"
	"sort"
	"sync"
)

var (
	ErrCollectionNotFound = errors.New("collection not found")
	ErrCollectionExists   = errors.New("collection already exists")
)

// VectorRecord 表示一条向量记录
type VectorRecord struct {
	ID       string         `json:"id"`
	Vector   []float32      `json:"vector"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// VectorMatch 表示向量搜索匹配结果
type VectorMatch struct {
	ID       string         `json:"id"`
	Score    float32        `json:"score"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// VectorSearchOptions 向量搜索选项
type VectorSearchOptions struct {
	TopK      int            `json:"top_k"`
	Threshold float32        `json:"threshold"`
	Filter    map[string]any `json:"filter,omitempty"`
}

// VectorStore 统一向量存储接口
type VectorStore interface {
	Insert(ctx context.Context, collection string, records []*VectorRecord) error
	Delete(ctx context.Context, collection string, ids []string) error
	Search(ctx context.Context, collection string, query []float32, opts VectorSearchOptions) ([]*VectorMatch, error)
	CreateCollection(ctx context.Context, name string, dim int) error
	DropCollection(ctx context.Context, name string) error
}

// InMemoryVectorStore 内存向量存储实现
type InMemoryVectorStore struct {
	mu          sync.RWMutex
	collections map[string]*vectorCollection
}

type vectorCollection struct {
	dim     int
	records map[string]*VectorRecord
}

// NewInMemoryVectorStore 创建内存向量存储实例
func NewInMemoryVectorStore() *InMemoryVectorStore {
	return &InMemoryVectorStore{
		collections: make(map[string]*vectorCollection),
	}
}

// CreateCollection 创建向量集合
func (s *InMemoryVectorStore) CreateCollection(ctx context.Context, name string, dim int) error {
	if dim <= 0 {
		dim = defaultVectorDim
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.collections[name]; exists {
		return ErrCollectionExists
	}
	s.collections[name] = &vectorCollection{
		dim:     dim,
		records: make(map[string]*VectorRecord),
	}
	return nil
}

// DropCollection 删除向量集合
func (s *InMemoryVectorStore) DropCollection(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.collections[name]; !exists {
		return ErrCollectionNotFound
	}
	delete(s.collections, name)
	return nil
}

// Insert 插入向量记录
func (s *InMemoryVectorStore) Insert(ctx context.Context, collection string, records []*VectorRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, exists := s.collections[collection]
	if !exists {
		return ErrCollectionNotFound
	}
	for _, r := range records {
		if len(r.Vector) != c.dim {
			return ErrDimensionMismatch
		}
		vec := make([]float32, len(r.Vector))
		copy(vec, r.Vector)
		mdCopy := make(map[string]any, len(r.Metadata))
		for k, v := range r.Metadata {
			mdCopy[k] = v
		}
		c.records[r.ID] = &VectorRecord{
			ID:       r.ID,
			Vector:   vec,
			Metadata: mdCopy,
		}
	}
	return nil
}

// Delete 删除向量记录
func (s *InMemoryVectorStore) Delete(ctx context.Context, collection string, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, exists := s.collections[collection]
	if !exists {
		return ErrCollectionNotFound
	}
	for _, id := range ids {
		delete(c.records, id)
	}
	return nil
}

// Search 向量搜索
func (s *InMemoryVectorStore) Search(ctx context.Context, collection string, query []float32, opts VectorSearchOptions) ([]*VectorMatch, error) {
	s.mu.RLock()
	c, exists := s.collections[collection]
	if !exists {
		s.mu.RUnlock()
		return nil, ErrCollectionNotFound
	}
	if len(query) != c.dim {
		s.mu.RUnlock()
		return nil, ErrDimensionMismatch
	}
	records := make([]*VectorRecord, 0, len(c.records))
	for _, r := range c.records {
		records = append(records, r)
	}
	s.mu.RUnlock()

	if opts.TopK <= 0 {
		opts.TopK = defaultTopK
	}

	matches := make([]*VectorMatch, 0, len(records))
	for _, r := range records {
		score := cosineSimilarity(query, r.Vector)
		if opts.Threshold > 0 && score < opts.Threshold {
			continue
		}
		if !matchFilter(r.Metadata, opts.Filter) {
			continue
		}
		mdCopy := make(map[string]any, len(r.Metadata))
		for k, v := range r.Metadata {
			mdCopy[k] = v
		}
		matches = append(matches, &VectorMatch{
			ID:       r.ID,
			Score:    score,
			Metadata: mdCopy,
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	if opts.TopK > len(matches) {
		opts.TopK = len(matches)
	}
	return matches[:opts.TopK], nil
}

// CollectionNames 返回所有集合名称
func (s *InMemoryVectorStore) CollectionNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.collections))
	for name := range s.collections {
		names = append(names, name)
	}
	return names
}

// Count 返回集合记录数量
func (s *InMemoryVectorStore) Count(collection string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, exists := s.collections[collection]
	if !exists {
		return 0
	}
	return len(c.records)
}

// matchFilter 检查元数据是否满足过滤条件
func matchFilter(metadata map[string]any, filter map[string]any) bool {
	if len(filter) == 0 {
		return true
	}
	for k, v := range filter {
		if mv, ok := metadata[k]; !ok || mv != v {
			return false
		}
	}
	return true
}
