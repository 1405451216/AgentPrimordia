package memory

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
)

const (
	defaultVectorDim = 16
	defaultTopK      = 10
)

var (
	ErrDimensionMismatch = errors.New("vector dimension mismatch")
	ErrVectorNotFound    = errors.New("vector entry not found")
)

type VectorEntry struct {
	ID       string
	Vector   []float32
	Metadata map[string]string
}

type VectorSearchResult struct {
	ID       string
	Score    float32
	Metadata map[string]string
}

type SimpleVectorStore struct {
	mu      sync.RWMutex
	entries map[string]*VectorEntry
	dim     int
	hnsw    *HNSWIndex
}

func NewVectorStore(dimensions int) *SimpleVectorStore {
	if dimensions <= 0 {
		dimensions = defaultVectorDim
	}
	return &SimpleVectorStore{
		entries: make(map[string]*VectorEntry),
		dim:     dimensions,
	}
}

// NewVectorStoreWithHNSW 创建带 HNSW 索引的向量存储
func NewVectorStoreWithHNSW(dimensions int, cfg HNSWConfig) *SimpleVectorStore {
	cfg.Dimensions = dimensions
	vs := NewVectorStore(dimensions)
	vs.hnsw = NewHNSWIndex(cfg)
	return vs
}

func (s *SimpleVectorStore) Add(ctx context.Context, id string, vector []float32, metadata map[string]string) error {
	if len(vector) != s.dim {
		return fmt.Errorf("%w: expected %d, got %d", ErrDimensionMismatch, s.dim, len(vector))
	}

	// 拷贝 vector 防止调用方突变
	vectorCopy := make([]float32, len(vector))
	copy(vectorCopy, vector)

	// 拷贝 metadata
	metadataCopy := make(map[string]string, len(metadata))
	for k, v := range metadata {
		metadataCopy[k] = v
	}

	// 先在锁内完成 map 写入
	s.mu.Lock()
	s.entries[id] = &VectorEntry{
		ID:       id,
		Vector:   vectorCopy,
		Metadata: metadataCopy,
	}
	s.mu.Unlock()

	// 解锁后同步到 HNSW 索引（避免持锁期间执行耗时操作）
	if s.hnsw != nil {
		s.hnsw.Insert(ctx, id, vectorCopy, metadataCopy)
	}

	return nil
}

func (s *SimpleVectorStore) Search(ctx context.Context, query []float32, topK int) ([]*VectorSearchResult, error) {
	if len(query) != s.dim {
		return nil, fmt.Errorf("%w: expected %d, got %d", ErrDimensionMismatch, s.dim, len(query))
	}

	if topK <= 0 {
		topK = defaultTopK
	}

	// 优先使用 HNSW 索引
	if s.hnsw != nil {
		hnswResults := s.hnsw.Search(ctx, query, topK)
		if len(hnswResults) > 0 {
			results := make([]*VectorSearchResult, 0, len(hnswResults))
			for _, r := range hnswResults {
				metadataCopy := make(map[string]string, len(r.Metadata))
				for k, v := range r.Metadata {
					metadataCopy[k] = v
				}
				results = append(results, &VectorSearchResult{
					ID:       r.ID,
					Score:    1.0 - r.Distance, // 距离转相似度
					Metadata: metadataCopy,
				})
			}
			return results, nil
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]*VectorSearchResult, 0, len(s.entries))
	for _, entry := range s.entries {
		score := cosineSimilarity(query, entry.Vector)
		// Deep copy metadata to prevent caller from mutating internal state
		metadataCopy := make(map[string]string, len(entry.Metadata))
		for k, v := range entry.Metadata {
			metadataCopy[k] = v
		}
		results = append(results, &VectorSearchResult{
			ID:       entry.ID,
			Score:    score,
			Metadata: metadataCopy,
		})
	}

	// 优化（Task 19）：使用泛型 slices.SortFunc 替代 sort.Slice，避免反射开销
	slices.SortFunc(results, func(a, b *VectorSearchResult) int { return cmp.Compare(b.Score, a.Score) })

	if topK > len(results) {
		topK = len(results)
	}

	return results[:topK], nil
}

func (s *SimpleVectorStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	if _, exists := s.entries[id]; !exists {
		s.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrVectorNotFound, id)
	}
	delete(s.entries, id)
	s.mu.Unlock()

	// 解锁后同步到 HNSW（避免持锁期间执行耗时操作）
	if s.hnsw != nil {
		s.hnsw.Delete(id)
	}

	return nil
}

func (s *SimpleVectorStore) Get(ctx context.Context, id string) (*VectorEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.entries[id]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrVectorNotFound, id)
	}
	return entry, nil
}

func (s *SimpleVectorStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

func (s *SimpleVectorStore) Dimensions() int {
	return s.dim
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}
