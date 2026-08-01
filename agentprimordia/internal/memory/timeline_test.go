package memory

import (
	"context"
	"testing"
	"time"
)

func TestSQLiteStore_GetMemoriesByTag(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep1 := MustEpisode("s1", "user", "Message about golang")
	ep1.Topics = "go,programming"
	_ = store.Add(ctx, ep1)

	ep2 := MustEpisode("s1", "user", "Message about python")
	ep2.Topics = "python,data"
	_ = store.Add(ctx, ep2)

	ep3 := MustEpisode("s1", "user", "Another go message")
	ep3.Topics = "golang,backend"
	_ = store.Add(ctx, ep3)

	results, err := store.GetMemoriesByTag(ctx, "go", 10)
	if err != nil {
		t.Fatalf("GetMemoriesByTag() error = %v", err)
	}
	if len(results) != 2 {
		t.Errorf("GetMemoriesByTag results length = %d, want %d", len(results), 2)
	}

	for _, r := range results {
		if r.Topics != "go,programming" && r.Topics != "golang,backend" {
			t.Errorf("unexpected topic %q", r.Topics)
		}
	}
}

func TestSQLiteStore_GetMemoriesByTag_NoMatch(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep := MustEpisode("s1", "user", "Message")
	ep.Topics = "java"
	_ = store.Add(ctx, ep)

	results, err := store.GetMemoriesByTag(ctx, "nonexistent", 10)
	if err != nil {
		t.Fatalf("GetMemoriesByTag() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("GetMemoriesByTag results length = %d, want %d", len(results), 0)
	}
}

func TestSQLiteStore_GetMemoriesBySession(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep1 := MustEpisode("session-a", "user", "First message")
	_ = store.Add(ctx, ep1)

	ep2 := MustEpisode("session-a", "assistant", "Second message")
	_ = store.Add(ctx, ep2)

	ep3 := MustEpisode("session-b", "user", "Other session")
	_ = store.Add(ctx, ep3)

	results, err := store.GetMemoriesBySession(ctx, "session-a")
	if err != nil {
		t.Fatalf("GetMemoriesBySession() error = %v", err)
	}
	if len(results) != 2 {
		t.Errorf("GetMemoriesBySession results length = %d, want %d", len(results), 2)
	}

	if results[0].CreatedAt > results[1].CreatedAt {
		t.Error("results should be sorted by created_at ASC")
	}
}

func TestSQLiteStore_SetImportance(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep := MustEpisode("s1", "user", "Important message")
	_ = store.Add(ctx, ep)

	err := store.SetImportance(ctx, ep.ID, 0.8)
	if err != nil {
		t.Fatalf("SetImportance() error = %v", err)
	}

	got, _ := store.Get(ctx, ep.ID)
	if got.Importance != 0.8 {
		t.Errorf("Importance = %f, want %f", got.Importance, 0.8)
	}
}

func TestSQLiteStore_SetImportance_Invalid(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep := MustEpisode("s1", "user", "Message")
	_ = store.Add(ctx, ep)

	if err := store.SetImportance(ctx, ep.ID, -0.1); err != ErrInvalidImportance {
		t.Errorf("SetImportance(-0.1) error = %v, want %v", err, ErrInvalidImportance)
	}
	if err := store.SetImportance(ctx, ep.ID, 1.1); err != ErrInvalidImportance {
		t.Errorf("SetImportance(1.1) error = %v, want %v", err, ErrInvalidImportance)
	}
}

func TestSQLiteStore_GetImportantMemories(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep1 := MustEpisode("s1", "user", "Low importance")
	ep1.Importance = 0.2
	_ = store.Add(ctx, ep1)

	ep2 := MustEpisode("s1", "user", "High importance")
	ep2.Importance = 0.9
	_ = store.Add(ctx, ep2)

	ep3 := MustEpisode("s1", "user", "Medium importance")
	ep3.Importance = 0.5
	_ = store.Add(ctx, ep3)

	results, err := store.GetImportantMemories(ctx, 0.5, 10)
	if err != nil {
		t.Fatalf("GetImportantMemories() error = %v", err)
	}
	if len(results) != 2 {
		t.Errorf("GetImportantMemories results length = %d, want %d", len(results), 2)
	}
	if results[0].Importance < results[1].Importance {
		t.Error("results should be sorted by importance DESC")
	}
}

func TestSQLiteStore_GetMemoryTimeline(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()

	now := time.Now().UTC()
	ep1 := MustEpisode("s1", "user", "Today message")
	ep1.CreatedAt = now.Format(time.RFC3339)
	_ = store.Add(ctx, ep1)

	ep2 := MustEpisode("s1", "user", "Yesterday message")
	ep2.CreatedAt = now.AddDate(0, 0, -1).Format(time.RFC3339)
	_ = store.Add(ctx, ep2)

	ep3 := MustEpisode("s1", "user", "Two days ago")
	ep3.CreatedAt = now.AddDate(0, 0, -2).Format(time.RFC3339)
	_ = store.Add(ctx, ep3)

	results, err := store.GetMemoryTimeline(ctx, 7)
	if err != nil {
		t.Fatalf("GetMemoryTimeline() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one day in timeline")
	}

	entryCount := 0
	for _, g := range results {
		entryCount += len(g.Episodes)
	}
	if entryCount != 3 {
		t.Errorf("total entries = %d, want %d", entryCount, 3)
	}
}

func TestSQLiteStore_GetMemoryTimeline_DefaultDays(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()

	now := time.Now().UTC()
	ep := MustEpisode("s1", "user", "Recent message")
	ep.CreatedAt = now.AddDate(0, 0, -5).Format(time.RFC3339)
	_ = store.Add(ctx, ep)

	results, err := store.GetMemoryTimeline(ctx, 0)
	if err != nil {
		t.Fatalf("GetMemoryTimeline() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one day in timeline with default days")
	}
}
