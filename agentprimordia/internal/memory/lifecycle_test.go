package memory

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultRetentionPolicy(t *testing.T) {
	policy := DefaultRetentionPolicy()
	if policy.WorkingMemoryTTL != 7*24*time.Hour {
		t.Errorf("unexpected working TTL: %v", policy.WorkingMemoryTTL)
	}
	if policy.SemanticMemoryTTL != 90*24*time.Hour {
		t.Errorf("unexpected semantic TTL: %v", policy.SemanticMemoryTTL)
	}
	if policy.EpisodeMemoryTTL != 365*24*time.Hour {
		t.Errorf("unexpected episode TTL: %v", policy.EpisodeMemoryTTL)
	}
	if policy.SessionTTL != 30*24*time.Hour {
		t.Errorf("unexpected session TTL: %v", policy.SessionTTL)
	}
}

func TestLifecycleManager_SetGetPolicy(t *testing.T) {
	mgr := NewLifecycleManager()
	custom := RetentionPolicy{
		WorkingMemoryTTL:  24 * time.Hour,
		SemanticMemoryTTL: 48 * time.Hour,
	}
	mgr.SetPolicy("tenant-a", custom)

	policy, err := mgr.GetPolicy("tenant-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if policy.WorkingMemoryTTL != 24*time.Hour {
		t.Errorf("unexpected policy: %v", policy)
	}
}

func TestLifecycleManager_GetPolicyNotFound(t *testing.T) {
	mgr := NewLifecycleManager()
	_, err := mgr.GetPolicy("nonexistent")
	if err != ErrPolicyNotFound {
		t.Errorf("expected ErrPolicyNotFound, got %v", err)
	}
}

func TestLifecycleManager_Enforce(t *testing.T) {
	hook := &mockLifecycleHook{}
	mgr := NewLifecycleManager()
	mgr.SetHook(hook)

	report, err := mgr.Enforce(context.Background())
	if err != nil {
		t.Fatalf("Enforce error: %v", err)
	}
	if report.ExecutedAt.IsZero() {
		t.Error("ExecutedAt should be set")
	}
	if !hook.archiveCalled.Load() {
		t.Error("expected archive to be called")
	}
	if !hook.deleteCalled.Load() {
		t.Error("expected delete to be called")
	}
	if !hook.compressCalled.Load() {
		t.Error("expected compress to be called")
	}
}

func TestLifecycleManager_ScheduleArchive(t *testing.T) {
	hook := &mockLifecycleHook{}
	mgr := NewLifecycleManager()
	mgr.SetHook(hook)

	err := mgr.ScheduleArchive(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatalf("ScheduleArchive error: %v", err)
	}
	if !hook.archiveCalled.Load() {
		t.Error("expected archive to be called")
	}
}

func TestLifecycleManager_ScheduleArchiveInvalid(t *testing.T) {
	mgr := NewLifecycleManager()
	err := mgr.ScheduleArchive(context.Background(), 0)
	if err == nil {
		t.Error("expected error for zero olderThan")
	}
}

func TestLifecycleManager_DeleteByUser(t *testing.T) {
	hook := &mockLifecycleHook{}
	mgr := NewLifecycleManager()
	mgr.SetHook(hook)

	err := mgr.DeleteByUser(context.Background(), "user-123")
	if err != nil {
		t.Fatalf("DeleteByUser error: %v", err)
	}
	if !hook.deleteCalled.Load() {
		t.Error("expected delete to be called")
	}
}

func TestLifecycleManager_DeleteByUserEmpty(t *testing.T) {
	mgr := NewLifecycleManager()
	err := mgr.DeleteByUser(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty userID")
	}
}

func TestLifecycleManager_Policies(t *testing.T) {
	mgr := NewLifecycleManager()
	policies := mgr.Policies()
	if len(policies) != 1 {
		t.Errorf("expected 1 default policy, got %d", len(policies))
	}
}

// mockLifecycleHook 用于测试
type mockLifecycleHook struct {
	archiveCalled  atomic.Bool
	deleteCalled   atomic.Bool
	compressCalled atomic.Bool
}

func (h *mockLifecycleHook) OnArchive(ctx context.Context, episodeIDs []string) error {
	_ = ctx
	_ = episodeIDs
	h.archiveCalled.Store(true)
	return nil
}

func (h *mockLifecycleHook) OnDelete(ctx context.Context, episodeIDs []string) error {
	_ = ctx
	_ = episodeIDs
	h.deleteCalled.Store(true)
	return nil
}

func (h *mockLifecycleHook) OnCompress(ctx context.Context, episodeIDs []string) (int64, error) {
	_ = ctx
	_ = episodeIDs
	h.compressCalled.Store(true)
	return 5, nil
}
