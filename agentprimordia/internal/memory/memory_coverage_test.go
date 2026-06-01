package memory

import (
	"context"
	"testing"
)

func TestSQLiteStore_GetMemoriesByTag_EmptyTag(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep := MustEpisode("s1", "user", "Message")
	ep.Topics = "go,programming"
	store.Add(ctx, ep)

	results, err := store.GetMemoriesByTag(ctx, "", 10)
	if err != nil {
		t.Fatalf("GetMemoriesByTag() error = %v", err)
	}
	// 空标签 LIKE %% 会匹配所有记录
	if len(results) != 1 {
		t.Errorf("GetMemoriesByTag('') length = %d, want %d", len(results), 1)
	}
}

func TestSQLiteStore_GetMemoriesBySession_Empty(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()

	results, err := store.GetMemoriesBySession(ctx, "")
	if err != nil {
		t.Fatalf("GetMemoriesBySession() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("GetMemoriesBySession('') length = %d, want %d", len(results), 0)
	}
}

func TestSQLiteStore_GetImportantMemories_Empty(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()

	results, err := store.GetImportantMemories(ctx, 0.5, 10)
	if err != nil {
		t.Fatalf("GetImportantMemories() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("GetImportantMemories() length = %d, want %d", len(results), 0)
	}
}

func TestSQLiteStore_GetMemoryTimeline_Empty(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()

	results, err := store.GetMemoryTimeline(ctx, 7)
	if err != nil {
		t.Fatalf("GetMemoryTimeline() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("GetMemoryTimeline() length = %d, want %d", len(results), 0)
	}
}

func TestSQLiteStore_SetImportance_NotFound(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()

	err := store.SetImportance(ctx, "non-existent-id", 0.5)
	if err == nil {
		t.Fatal("expected error for non-existent episode")
	}
}
