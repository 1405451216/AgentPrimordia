package memory

import (
	"context"
	"testing"
)

func TestVectorStore_AddAndSearch(t *testing.T) {
	store := NewVectorStore(4)

	store.Add(context.Background(), "doc1", []float32{1, 0, 0, 0}, nil)
	store.Add(context.Background(), "doc2", []float32{0, 1, 0, 0}, nil)
	store.Add(context.Background(), "doc3", []float32{0.9, 0.1, 0, 0}, nil)

	results, err := store.Search(context.Background(), []float32{1, 0, 0, 0}, 3)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	if results[0].ID != "doc1" {
		t.Errorf("top result should be doc1, got %s (score=%f)", results[0].ID, results[0].Score)
	}

	if results[1].ID != "doc3" {
		t.Errorf("second result should be doc3, got %s (score=%f)", results[1].ID, results[1].Score)
	}
}

func TestVectorStore_CosineSimilarity(t *testing.T) {
	store := NewVectorStore(3)

	store.Add(context.Background(), "a", []float32{1, 0, 0}, nil)
	store.Add(context.Background(), "b", []float32{0, 1, 0}, nil)

	results, _ := store.Search(context.Background(), []float32{1, 0, 0}, 2)

	if results[0].Score < 0.99 {
		t.Errorf("identical vectors should have score ~1.0, got %f", results[0].Score)
	}

	if results[1].Score > 0.01 {
		t.Errorf("orthogonal vectors should have score ~0.0, got %f", results[1].Score)
	}
}

func TestVectorStore_DimensionMismatch(t *testing.T) {
	store := NewVectorStore(4)

	err := store.Add(context.Background(), "bad", []float32{1, 0, 0}, nil)
	if err == nil {
		t.Fatal("expected error for dimension mismatch")
	}

	_, err = store.Search(context.Background(), []float32{1, 0, 0}, 5)
	if err == nil {
		t.Fatal("expected error for query dimension mismatch")
	}
}

func TestVectorStore_Delete(t *testing.T) {
	store := NewVectorStore(4)

	store.Add(context.Background(), "doc1", []float32{1, 0, 0, 0}, nil)

	err := store.Delete(context.Background(), "doc1")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if store.Count() != 0 {
		t.Errorf("expected 0 entries after delete, got %d", store.Count())
	}
}

func TestVectorStore_DeleteNotFound(t *testing.T) {
	store := NewVectorStore(4)

	err := store.Delete(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for deleting nonexistent entry")
	}
}

func TestVectorStore_Get(t *testing.T) {
	store := NewVectorStore(4)

	meta := map[string]string{"source": "test"}
	store.Add(context.Background(), "doc1", []float32{1, 0, 0, 0}, meta)

	entry, err := store.Get(context.Background(), "doc1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if entry.ID != "doc1" {
		t.Errorf("ID = %q, want %q", entry.ID, "doc1")
	}
	if entry.Metadata["source"] != "test" {
		t.Errorf("Metadata[source] = %q, want %q", entry.Metadata["source"], "test")
	}
}

func TestVectorStore_GetNotFound(t *testing.T) {
	store := NewVectorStore(4)

	_, err := store.Get(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent entry")
	}
}

func TestVectorStore_TopK(t *testing.T) {
	store := NewVectorStore(4)

	for i := 0; i < 10; i++ {
		vec := make([]float32, 4)
		vec[0] = float32(i) / 10.0
		store.Add(context.Background(), "doc"+string(rune('0'+i)), vec, nil)
	}

	results, _ := store.Search(context.Background(), []float32{1, 0, 0, 0}, 3)

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestVectorStore_DefaultDimensions(t *testing.T) {
	store := NewVectorStore(0)

	if store.Dimensions() != 16 {
		t.Errorf("expected default 16 dimensions, got %d", store.Dimensions())
	}
}

func TestVectorStore_EmptySearch(t *testing.T) {
	store := NewVectorStore(4)

	results, err := store.Search(context.Background(), []float32{1, 0, 0, 0}, 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty store, got %d", len(results))
	}
}

func TestVectorStore_Metadata(t *testing.T) {
	store := NewVectorStore(4)

	store.Add(context.Background(), "doc1", []float32{1, 0, 0, 0}, map[string]string{"tag": "important"})
	store.Add(context.Background(), "doc2", []float32{0, 1, 0, 0}, map[string]string{"tag": "normal"})

	results, _ := store.Search(context.Background(), []float32{1, 0, 0, 0}, 2)

	if results[0].Metadata["tag"] != "important" {
		t.Errorf("expected tag 'important', got %q", results[0].Metadata["tag"])
	}
}
