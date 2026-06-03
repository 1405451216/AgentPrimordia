package persist

import (
	"context"
	"testing"
	"time"
)

func newTestCheckpointStore(t *testing.T) *SQLiteCheckpointStore {
	t.Helper()
	store, err := InMemoryCheckpointStore()
	if err != nil {
		t.Fatalf("failed to create checkpoint store: %v", err)
	}
	return store
}

func TestCheckpoint_SaveAndLoad(t *testing.T) {
	store := newTestCheckpointStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	state := &AgentState{
		AgentID:   "agent-1",
		SessionID: "session-1",
		Status:    "running",
		Messages: []CheckpointMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
		},
		TurnCount: 2,
		Metrics: CheckpointMetrics{
			TotalTurns: 2,
			TotalTools: 1,
			Duration:   "5s",
		},
		SavedAt: time.Now().Truncate(time.Second),
	}

	err := store.Save(ctx, state)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := store.Load(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", loaded.AgentID, "agent-1")
	}
	if loaded.SessionID != "session-1" {
		t.Errorf("SessionID = %q, want %q", loaded.SessionID, "session-1")
	}
	if loaded.Status != "running" {
		t.Errorf("Status = %q, want %q", loaded.Status, "running")
	}
	if loaded.TurnCount != 2 {
		t.Errorf("TurnCount = %d, want %d", loaded.TurnCount, 2)
	}
	if len(loaded.Messages) != 2 {
		t.Errorf("Messages length = %d, want %d", len(loaded.Messages), 2)
	}
	if loaded.Metrics.TotalTurns != 2 {
		t.Errorf("Metrics.TotalTurns = %d, want %d", loaded.Metrics.TotalTurns, 2)
	}
}

func TestCheckpoint_LoadNotFound(t *testing.T) {
	store := newTestCheckpointStore(t)
	defer func() { _ = store.Close() }()

	_, err := store.Load(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent checkpoint")
	}
}

func TestCheckpoint_SaveOverwrite(t *testing.T) {
	store := newTestCheckpointStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	state1 := &AgentState{
		AgentID:   "agent-1",
		SessionID: "session-1",
		Status:    "running",
		Messages:  []CheckpointMessage{{Role: "user", Content: "first"}},
		TurnCount: 1,
		SavedAt:   time.Now(),
	}
	_ = store.Save(ctx, state1)

	state2 := &AgentState{
		AgentID:   "agent-1",
		SessionID: "session-1",
		Status:    "completed",
		Messages:  []CheckpointMessage{{Role: "user", Content: "first"}, {Role: "user", Content: "second"}},
		TurnCount: 2,
		SavedAt:   time.Now(),
	}
	_ = store.Save(ctx, state2)

	loaded, err := store.Load(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Status != "completed" {
		t.Errorf("Status = %q, want %q", loaded.Status, "completed")
	}
	if loaded.TurnCount != 2 {
		t.Errorf("TurnCount = %d, want %d", loaded.TurnCount, 2)
	}
}

func TestCheckpoint_List(t *testing.T) {
	store := newTestCheckpointStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	_ = store.Save(ctx, &AgentState{AgentID: "agent-1", SessionID: "session-1", Status: "running", SavedAt: time.Now()})
	_ = store.Save(ctx, &AgentState{AgentID: "agent-2", SessionID: "session-1", Status: "running", SavedAt: time.Now()})
	_ = store.Save(ctx, &AgentState{AgentID: "agent-3", SessionID: "session-2", Status: "idle", SavedAt: time.Now()})

	results, err := store.List(ctx, "session-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	results2, err := store.List(ctx, "session-2")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(results2) != 1 {
		t.Errorf("expected 1 result, got %d", len(results2))
	}
}

func TestCheckpoint_ListEmpty(t *testing.T) {
	store := newTestCheckpointStore(t)
	defer func() { _ = store.Close() }()

	results, err := store.List(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestCheckpoint_Delete(t *testing.T) {
	store := newTestCheckpointStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	_ = store.Save(ctx, &AgentState{AgentID: "agent-1", SessionID: "session-1", Status: "running", SavedAt: time.Now()})

	err := store.Delete(ctx, "agent-1")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = store.Load(ctx, "agent-1")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestCheckpoint_DeleteNotFound(t *testing.T) {
	store := newTestCheckpointStore(t)
	defer func() { _ = store.Close() }()

	err := store.Delete(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for deleting nonexistent checkpoint")
	}
}
