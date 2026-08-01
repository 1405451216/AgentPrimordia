package governance

import (
	"context"
	"testing"
)

func TestTenantManager_ArchiveTenant(t *testing.T) {
	tm := NewTenantManager()
	ctx := context.Background()

	tenant, _, err := tm.CreateTenant(ctx, "test-archive", PlanFree, DefaultQuota(PlanFree), false)
	if err != nil {
		t.Fatalf("CreateTenant error: %v", err)
	}

	err = tm.ArchiveTenant(ctx, tenant.ID)
	if err != nil {
		t.Errorf("ArchiveTenant error: %v", err)
	}

	archived, _ := tm.GetTenant(ctx, tenant.ID)
	if archived.Status != TenantArchived {
		t.Errorf("Status = %v, want TenantArchived", archived.Status)
	}
}

func TestTenantManager_ArchiveTenant_NotFound(t *testing.T) {
	tm := NewTenantManager()
	ctx := context.Background()

	err := tm.ArchiveTenant(ctx, "nonexistent")
	if err == nil {
		t.Error("ArchiveTenant should fail for nonexistent tenant")
	}
}

func TestTenantManager_ListTenants_AfterArchive(t *testing.T) {
	tm := NewTenantManager()
	ctx := context.Background()

	_, _, _ = tm.CreateTenant(ctx, "active1", PlanFree, DefaultQuota(PlanFree), false)
	t2, _, _ := tm.CreateTenant(ctx, "to-archive", PlanFree, DefaultQuota(PlanFree), false)
	_ = tm.ArchiveTenant(ctx, t2.ID)

	tenants := tm.ListTenants(ctx)
	if len(tenants) != 2 {
		t.Errorf("ListTenants returned %d, want 2", len(tenants))
	}
}
