package memory

import (
	"context"
	"testing"
)

func TestInMemoryVectorStore_CreateCollection(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryVectorStore()

	if err := store.CreateCollection(ctx, "docs", 4); err != nil {
		t.Fatalf("CreateCollection error: %v", err)
	}
	if err := store.CreateCollection(ctx, "docs", 4); err != ErrCollectionExists {
		t.Errorf("expected ErrCollectionExists, got %v", err)
	}
}

func TestInMemoryVectorStore_DropCollection(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryVectorStore()

	_ = store.CreateCollection(ctx, "docs", 4)
	if err := store.DropCollection(ctx, "docs"); err != nil {
		t.Fatalf("DropCollection error: %v", err)
	}
	if err := store.DropCollection(ctx, "docs"); err != ErrCollectionNotFound {
		t.Errorf("expected ErrCollectionNotFound, got %v", err)
	}
}

func TestInMemoryVectorStore_InsertAndSearch(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryVectorStore()
	_ = store.CreateCollection(ctx, "docs", 4)

	records := []*VectorRecord{
		{ID: "a", Vector: []float32{1, 0, 0, 0}, Metadata: map[string]any{"topic": "science"}},
		{ID: "b", Vector: []float32{0, 1, 0, 0}, Metadata: map[string]any{"topic": "art"}},
		{ID: "c", Vector: []float32{0.9, 0.1, 0, 0}, Metadata: map[string]any{"topic": "science"}},
	}
	if err := store.Insert(ctx, "docs", records); err != nil {
		t.Fatalf("Insert error: %v", err)
	}

	results, err := store.Search(ctx, "docs", []float32{1, 0, 0, 0}, VectorSearchOptions{TopK: 3})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].ID != "a" {
		t.Errorf("top result should be a, got %s (score=%.4f)", results[0].ID, results[0].Score)
	}
}

func TestInMemoryVectorStore_DimensionMismatch(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryVectorStore()
	_ = store.CreateCollection(ctx, "docs", 4)

	err := store.Insert(ctx, "docs", []*VectorRecord{
		{ID: "x", Vector: []float32{1, 0, 0}},
	})
	if err != ErrDimensionMismatch {
		t.Errorf("expected ErrDimensionMismatch, got %v", err)
	}
}

func TestInMemoryVectorStore_Delete(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryVectorStore()
	_ = store.CreateCollection(ctx, "docs", 4)

	_ = store.Insert(ctx, "docs", []*VectorRecord{
		{ID: "a", Vector: []float32{1, 0, 0, 0}},
		{ID: "b", Vector: []float32{0, 1, 0, 0}},
	})

	if err := store.Delete(ctx, "docs", []string{"a"}); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	results, _ := store.Search(ctx, "docs", []float32{1, 0, 0, 0}, VectorSearchOptions{TopK: 10})
	if len(results) != 1 || results[0].ID != "b" {
		t.Errorf("after delete expected only b, got %d results", len(results))
	}
}

func TestInMemoryVectorStore_Threshold(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryVectorStore()
	_ = store.CreateCollection(ctx, "docs", 4)

	_ = store.Insert(ctx, "docs", []*VectorRecord{
		{ID: "a", Vector: []float32{1, 0, 0, 0}},
		{ID: "b", Vector: []float32{0, 1, 0, 0}},
	})
	results, _ := store.Search(ctx, "docs", []float32{1, 0, 0, 0}, VectorSearchOptions{
		TopK: 10, Threshold: 0.9,
	})
	if len(results) != 1 {
		t.Errorf("expected 1 result with threshold 0.9, got %d", len(results))
	}
}

func TestInMemoryVectorStore_Filter(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryVectorStore()
	_ = store.CreateCollection(ctx, "docs", 4)

	_ = store.Insert(ctx, "docs", []*VectorRecord{
		{ID: "a", Vector: []float32{1, 0, 0, 0}, Metadata: map[string]any{"topic": "science"}},
		{ID: "b", Vector: []float32{0.9, 0.1, 0, 0}, Metadata: map[string]any{"topic": "art"}},
	})
	results, _ := store.Search(ctx, "docs", []float32{1, 0, 0, 0}, VectorSearchOptions{
		TopK: 10, Filter: map[string]any{"topic": "science"},
	})
	if len(results) != 1 || results[0].ID != "a" {
		t.Errorf("filter should return only a, got %d results", len(results))
	}
}

func TestInMemoryVectorStore_CollectionNames(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryVectorStore()
	_ = store.CreateCollection(ctx, "a", 4)
	_ = store.CreateCollection(ctx, "b", 4)
	names := store.CollectionNames()
	if len(names) != 2 {
		t.Errorf("expected 2 collections, got %d", len(names))
	}
}

func TestInMemoryVectorStore_Count(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryVectorStore()
	_ = store.CreateCollection(ctx, "docs", 4)
	_ = store.Insert(ctx, "docs", []*VectorRecord{
		{ID: "a", Vector: []float32{1, 0, 0, 0}},
		{ID: "b", Vector: []float32{0, 1, 0, 0}},
	})
	if store.Count("docs") != 2 {
		t.Errorf("expected count 2, got %d", store.Count("docs"))
	}
	if store.Count("nonexistent") != 0 {
		t.Errorf("expected count 0 for nonexistent, got %d", store.Count("nonexistent"))
	}
}

func TestInMemoryVectorStore_InsertNonexistent(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryVectorStore()
	err := store.Insert(ctx, "missing", []*VectorRecord{
		{ID: "a", Vector: []float32{1, 0, 0, 0}},
	})
	if err != ErrCollectionNotFound {
		t.Errorf("expected ErrCollectionNotFound, got %v", err)
	}
}

func TestInMemoryVectorStore_SearchNonexistent(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryVectorStore()
	_, err := store.Search(ctx, "missing", []float32{1, 0, 0, 0}, VectorSearchOptions{})
	if err != ErrCollectionNotFound {
		t.Errorf("expected ErrCollectionNotFound, got %v", err)
	}
}
