package governance

import (
	"context"
	"testing"
)

func TestResourceManager_Basic(t *testing.T) {
	rm := NewResourceManager()
	qm := rm.Register("t1", DefaultQuota(PlanFree))
	if qm == nil {
		t.Fatal("Register returned nil")
	}
	if qm.tenantID != "t1" {
		t.Errorf("tenantID = %q, want t1", qm.tenantID)
	}
}

func TestResourceManager_RegisterDuplicate(t *testing.T) {
	rm := NewResourceManager()
	qm1 := rm.Register("t1", DefaultQuota(PlanFree))
	qm2 := rm.Register("t1", DefaultQuota(PlanPro))
	if qm1 != qm2 {
		t.Error("Register with same tenantID should return existing manager")
	}
}

func TestResourceManager_Get_NotFound(t *testing.T) {
	rm := NewResourceManager()
	_, ok := rm.Get("nonexistent")
	if ok {
		t.Error("Get should return false for nonexistent tenant")
	}
}

func TestResourceManager_Remove(t *testing.T) {
	rm := NewResourceManager()
	rm.Register("t1", DefaultQuota(PlanFree))
	rm.Remove("t1")

	_, ok := rm.Get("t1")
	if ok {
		t.Error("after Remove, Get should return false")
	}
}

func TestResourceManager_Remove_Nonexistent(t *testing.T) {
	rm := NewResourceManager()
	// 不应 panic
	rm.Remove("nonexistent")
}

func TestResourceManager_AllStatus(t *testing.T) {
	rm := NewResourceManager()
	rm.Register("t1", DefaultQuota(PlanFree))
	rm.Register("t2", DefaultQuota(PlanPro))

	statuses := rm.AllStatus()
	if len(statuses) != 2 {
		t.Errorf("AllStatus returned %d entries, want 2", len(statuses))
	}
}

func TestResourceManager_AllStatus_Empty(t *testing.T) {
	rm := NewResourceManager()
	statuses := rm.AllStatus()
	if len(statuses) != 0 {
		t.Errorf("AllStatus on empty manager returned %d entries, want 0", len(statuses))
	}
}

func TestResourceManager_CheckRequest(t *testing.T) {
	rm := NewResourceManager()
	rm.Register("t1", TenantQuota{MaxQPS: 10, MaxTokensPerDay: 10000})

	ctx := context.Background()
	err := rm.CheckRequest(ctx, "t1", 100)
	if err != nil {
		t.Errorf("CheckRequest error: %v", err)
	}
}

func TestResourceManager_CheckRequest_TenantNotFound(t *testing.T) {
	rm := NewResourceManager()
	ctx := context.Background()
	err := rm.CheckRequest(ctx, "nonexistent", 100)
	if err == nil {
		t.Error("CheckRequest should fail for nonexistent tenant")
	}
}
