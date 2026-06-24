package memory

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
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

type VectorStore struct {
	mu      sync.RWMutex
	entries map[string]*VectorEntry
	dim     int
	hnsw    *HNSWIndex
}

func NewVectorStore(dimensions int) *VectorStore {
	if dimensions <= 0 {
		dimensions = defaultVectorDim
	}
	return &VectorStore{
		entries: make(map[string]*VectorEntry),
		dim:     dimensions,
	}
}

// NewVectorStoreWithHNSW 创建带 HNSW 索引的向量存储
func NewVectorStoreWithHNSW(dimensions int, cfg HNSWConfig) *VectorStore {
	cfg.Dimensions = dimensions
	vs := NewVectorStore(dimensions)
	vs.hnsw = NewHNSWIndex(cfg)
	return vs
}

func (s *VectorStore) Add(ctx context.Context, id string, vector []float32, metadata map[string]string) error {
	if len(vector) != s.dim {
		return fmt.Errorf("%w: expected %d, got %d", ErrDimensionMismatch, s.dim, len(vector))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[id] = &VectorEntry{
		ID:       id,
		Vector:   vector,
		Metadata: metadata,
	}

	// 同步到 HNSW 索引
	if s.hnsw != nil {
		s.hnsw.Insert(ctx, id, vector, metadata)
	}

	return nil
}

func (s *VectorStore) Search(ctx context.Context, query []float32, topK int) ([]*VectorSearchResult, error) {
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

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if topK > len(results) {
		topK = len(results)
	}

	return results[:topK], nil
}

func (s *VectorStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entries[id]; !exists {
		return fmt.Errorf("%w: %s", ErrVectorNotFound, id)
	}
	delete(s.entries, id)

	// 同步到 HNSW 索引
	if s.hnsw != nil {
		s.hnsw.Delete(id)
	}

	return nil
}

func (s *VectorStore) Get(ctx context.Context, id string) (*VectorEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.entries[id]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrVectorNotFound, id)
	}
	return entry, nil
}

func (s *VectorStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

func (s *VectorStore) Dimensions() int {
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
