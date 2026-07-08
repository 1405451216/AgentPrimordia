package pgvector

import (
	"context"
	"testing"
)

const testConnString = "postgres://postgres:postgres@localhost:5432/ap_test"

func getTestStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	cfg := Config{
		ConnString: testConnString,
		TableName:  "test_vectors",
		Dimensions: 4,
		Distance:   CosineDistance,
		IndexType:  HNSWIndex,
	}
	store, err := New(ctx, cfg)
	if err != nil {
		t.Skipf("skipping: no PostgreSQL: %v", err)
	}
	_, _ = store.db.Exec(ctx, "DELETE FROM test_vectors")
	return store
}

func TestStore_Add(t *testing.T) {
	store := getTestStore(t)
	defer store.Close()
	ctx := context.Background()
	vec := []float32{0.1, 0.2, 0.3, 0.4}
	meta := map[string]string{"tenant": "t1"}
	if err := store.Add(ctx, "ep_1", vec, meta); err != nil {
		t.Fatalf("Add: %v", err)
	}
	vec2 := []float32{0.5, 0.6, 0.7, 0.8}
	if err := store.Add(ctx, "ep_1", vec2, meta); err != nil {
		t.Fatalf("Add upsert: %v", err)
	}
}

func TestStore_Add_DimensionMismatch(t *testing.T) {
	store := getTestStore(t)
	defer store.Close()
	err := store.Add(context.Background(), "ep", []float32{0.1}, nil)
	if err == nil {
		t.Fatal("expected dimension mismatch")
	}
}

func TestStore_Get(t *testing.T) {
	store := getTestStore(t)
	defer store.Close()
	ctx := context.Background()
	_ = store.Add(ctx, "ep_1", []float32{0.1, 0.2, 0.3, 0.4}, map[string]string{"tenant": "t1"})
	entry, err := store.Get(ctx, "ep_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry.ID != "ep_1" {
		t.Errorf("ID = %q", entry.ID)
	}
	if len(entry.Vector) != 4 {
		t.Errorf("Vector len = %d", len(entry.Vector))
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	store := getTestStore(t)
	defer store.Close()
	_, err := store.Get(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected not found")
	}
}

func TestStore_Search(t *testing.T) {
	store := getTestStore(t)
	defer store.Close()
	ctx := context.Background()
	_ = store.Add(ctx, "d1", []float32{0.1, 0.2, 0.3, 0.4}, map[string]string{"tenant": "t1"})
	_ = store.Add(ctx, "d2", []float32{0.9, 0.8, 0.7, 0.6}, map[string]string{"tenant": "t1"})
	_ = store.Add(ctx, "d3", []float32{0.1, 0.2, 0.3, 0.5}, map[string]string{"tenant": "t1"})
	results, err := store.Search(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 2, map[string]string{"tenant": "t1"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("results = %d", len(results))
	}
	if results[0].ID != "d1" {
		t.Errorf("first = %q", results[0].ID)
	}
}

func TestStore_Delete(t *testing.T) {
	store := getTestStore(t)
	defer store.Close()
	ctx := context.Background()
	_ = store.Add(ctx, "del", []float32{0.1, 0.2, 0.3, 0.4}, nil)
	if err := store.Delete(ctx, "del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := store.Get(ctx, "del")
	if err == nil {
		t.Fatal("expected not found")
	}
}

func TestVectorStringConversions(t *testing.T) {
	v := []float32{0.1, 0.2, 0.3}
	s := float32SliceToVectorString(v)
	if s != "[0.1,0.2,0.3]" {
		t.Errorf("String = %q", s)
	}
	back, err := float32SliceToFloat32(s)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(back) != 3 {
		t.Errorf("Len = %d", len(back))
	}
}
