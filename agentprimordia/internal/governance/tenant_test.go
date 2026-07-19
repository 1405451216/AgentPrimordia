package governance

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultQuota(t *testing.T) {
	tests := []struct {
		plan         TenantPlan
		wantAgents   int
		wantSessions int
		wantTokens   int64
		wantStorage  int64
		wantQPS      int
	}{
		{PlanFree, 3, 10, 100000, 1, 5},
		{PlanPro, 20, 100, 5000000, 50, 50},
		{PlanEnterprise, 0, 0, 0, 0, 500},
		{"", 3, 10, 100000, 1, 5}, // 默认
	}

	for _, tt := range tests {
		q := DefaultQuota(tt.plan)
		if q.MaxAgents != tt.wantAgents {
			t.Errorf("DefaultQuota(%q).MaxAgents = %d, want %d", tt.plan, q.MaxAgents, tt.wantAgents)
		}
		if q.MaxSessions != tt.wantSessions {
			t.Errorf("DefaultQuota(%q).MaxSessions = %d, want %d", tt.plan, q.MaxSessions, tt.wantSessions)
		}
		if q.MaxTokensPerDay != tt.wantTokens {
			t.Errorf("DefaultQuota(%q).MaxTokensPerDay = %d, want %d", tt.plan, q.MaxTokensPerDay, tt.wantTokens)
		}
		if q.MaxStorageGB != tt.wantStorage {
			t.Errorf("DefaultQuota(%q).MaxStorageGB = %d, want %d", tt.plan, q.MaxStorageGB, tt.wantStorage)
		}
		if q.MaxQPS != tt.wantQPS {
			t.Errorf("DefaultQuota(%q).MaxQPS = %d, want %d", tt.plan, q.MaxQPS, tt.wantQPS)
		}
	}
}

func TestTenantActive(t *testing.T) {
	tenant := &Tenant{Status: TenantActive}
	if !tenant.Active() {
		t.Error("Active tenant should return true")
	}
	tenant.Status = TenantDisabled
	if tenant.Active() {
		t.Error("Disabled tenant should return false")
	}
	tenant.Status = TenantArchived
	if tenant.Active() {
		t.Error("Archived tenant should return false")
	}
}

func TestHashAPIKey(t *testing.T) {
	h1 := HashAPIKey("test-key")
	h2 := HashAPIKey("test-key")
	if h1 != h2 {
		t.Error("Same API key should produce same hash")
	}
	h3 := HashAPIKey("different-key")
	if h1 == h3 {
		t.Error("Different API keys should produce different hashes")
	}
	if len(h1) != 64 { // SHA-256 hex = 64 chars
		t.Errorf("hash length = %d, want 64", len(h1))
	}
}

func TestGenerateAPIKey(t *testing.T) {
	k1 := GenerateAPIKey()
	if !strings.HasPrefix(k1, "apk_") {
		t.Errorf("API key prefix = %q, want apk_", k1[:4])
	}
	k2 := GenerateAPIKey()
	if k1 == k2 {
		t.Error("Generated keys should be unique")
	}
}

func TestTenantManager_CreateTenant(t *testing.T) {
	ctx := context.Background()
	mgr := NewTenantManager()

	tenant, apiKey, err := mgr.CreateTenant(ctx, "Test Corp", PlanPro, TenantQuota{}, true)
	if err != nil {
		t.Fatalf("CreateTenant error: %v", err)
	}
	if tenant.ID == "" {
		t.Error("tenant ID should not be empty")
	}
	if tenant.Name != "Test Corp" {
		t.Errorf("tenant name = %q, want Test Corp", tenant.Name)
	}
	if tenant.Plan != PlanPro {
		t.Errorf("tenant plan = %q, want pro", tenant.Plan)
	}
	if tenant.Status != TenantActive {
		t.Errorf("new tenant status = %q, want active", tenant.Status)
	}
	if apiKey == "" {
		t.Error("API key should not be empty when storeAPIKey=true")
	}
	// 配额应为 PlanPro 默认值
	if tenant.Quotas.MaxAgents != 20 {
		t.Errorf("default pro quota MaxAgents = %d, want 20", tenant.Quotas.MaxAgents)
	}
}

func TestTenantManager_CreateTenant_DefaultPlan(t *testing.T) {
	ctx := context.Background()
	mgr := NewTenantManager()

	tenant, _, err := mgr.CreateTenant(ctx, "Free Corp", "", TenantQuota{}, false)
	if err != nil {
		t.Fatalf("CreateTenant error: %v", err)
	}
	if tenant.Plan != PlanFree {
		t.Errorf("empty plan should default to free, got %q", tenant.Plan)
	}
}

func TestTenantManager_CreateTenant_EmptyName(t *testing.T) {
	ctx := context.Background()
	mgr := NewTenantManager()

	_, _, err := mgr.CreateTenant(ctx, "", PlanFree, TenantQuota{}, false)
	if err == nil {
		t.Error("expected error for empty tenant name")
	}
}

func TestTenantManager_GetTenant(t *testing.T) {
	ctx := context.Background()
	mgr := NewTenantManager()

	created, _, _ := mgr.CreateTenant(ctx, "Test", PlanFree, TenantQuota{}, false)

	got, err := mgr.GetTenant(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTenant error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("got ID = %q, want %q", got.ID, created.ID)
	}
	if got.Name != created.Name {
		t.Errorf("got Name = %q, want %q", got.Name, created.Name)
	}
}

func TestTenantManager_GetTenant_NotFound(t *testing.T) {
	ctx := context.Background()
	mgr := NewTenantManager()

	_, err := mgr.GetTenant(ctx, "nonexistent")
	if err == nil {
		t.Error("expected ErrTenantNotFound")
	}
}

func TestTenantManager_UpdateTenant(t *testing.T) {
	ctx := context.Background()
	mgr := NewTenantManager()

	created, _, _ := mgr.CreateTenant(ctx, "Original", PlanFree, TenantQuota{}, false)

	err := mgr.UpdateTenant(ctx, created.ID, func(t *Tenant) error {
		t.Name = "Updated"
		t.Plan = PlanEnterprise
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateTenant error: %v", err)
	}

	got, _ := mgr.GetTenant(ctx, created.ID)
	if got.Name != "Updated" {
		t.Errorf("name = %q, want Updated", got.Name)
	}
	if got.Plan != PlanEnterprise {
		t.Errorf("plan = %q, want enterprise", got.Plan)
	}
}

func TestTenantManager_DisableTenant(t *testing.T) {
	ctx := context.Background()
	mgr := NewTenantManager()

	created, _, _ := mgr.CreateTenant(ctx, "Test", PlanFree, TenantQuota{}, false)
	err := mgr.DisableTenant(ctx, created.ID)
	if err != nil {
		t.Fatalf("DisableTenant error: %v", err)
	}

	got, _ := mgr.GetTenant(ctx, created.ID)
	if got.Status != TenantDisabled {
		t.Errorf("status = %q, want disabled", got.Status)
	}
}

func TestTenantManager_ListTenants(t *testing.T) {
	ctx := context.Background()
	mgr := NewTenantManager()

	mgr.CreateTenant(ctx, "A", PlanFree, TenantQuota{}, false)
	mgr.CreateTenant(ctx, "B", PlanPro, TenantQuota{}, false)

	list := mgr.ListTenants(ctx)
	if len(list) != 2 {
		t.Errorf("list length = %d, want 2", len(list))
	}
}

func TestTenantManager_APIKeyBinding(t *testing.T) {
	ctx := context.Background()
	mgr := NewTenantManager()

	created, apiKey, _ := mgr.CreateTenant(ctx, "Test", PlanFree, TenantQuota{}, true)
	if apiKey == "" {
		t.Fatal("expected API key")
	}

	// 验证 API Key → 租户
	tenantID, err := mgr.TenantByAPIKey(apiKey)
	if err != nil {
		t.Fatalf("TenantByAPIKey error: %v", err)
	}
	if tenantID != created.ID {
		t.Errorf("tenantID = %q, want %q", tenantID, created.ID)
	}

	// ValidateAPIKey
	tenant, err := mgr.ValidateAPIKey(ctx, apiKey)
	if err != nil {
		t.Fatalf("ValidateAPIKey error: %v", err)
	}
	if tenant.ID != created.ID {
		t.Errorf("validated tenant ID = %q, want %q", tenant.ID, created.ID)
	}

	// 禁用后验证失败
	mgr.DisableTenant(ctx, created.ID)
	_, err = mgr.ValidateAPIKey(ctx, apiKey)
	if err != ErrTenantDisabled {
		t.Errorf("ValidateAPIKey after disable = %v, want ErrTenantDisabled", err)
	}

	// 撤销 API Key
	mgr.UpdateTenant(ctx, created.ID, func(t *Tenant) error {
		t.Status = TenantActive
		return nil
	})
	err = mgr.RevokeAPIKey(ctx, apiKey)
	if err != nil {
		t.Fatalf("RevokeAPIKey error: %v", err)
	}
	_, err = mgr.TenantByAPIKey(apiKey)
	if err != ErrInvalidAPIKey {
		t.Errorf("after revoke TenantByAPIKey = %v, want ErrInvalidAPIKey", err)
	}
}

func TestTenantManager_BindAPIKey(t *testing.T) {
	ctx := context.Background()
	mgr := NewTenantManager()

	created, _, _ := mgr.CreateTenant(ctx, "Test", PlanFree, TenantQuota{}, false)

	// 不存初始 Key，后面手动绑定
	key1, err := mgr.BindAPIKey(ctx, created.ID)
	if err != nil {
		t.Fatalf("BindAPIKey error: %v", err)
	}
	if key1 == "" {
		t.Error("BindAPIKey should return non-empty key")
	}

	key2, err := mgr.BindAPIKey(ctx, created.ID)
	if err != nil {
		t.Fatalf("BindAPIKey error: %v", err)
	}
	if key1 == key2 {
		t.Error("Two bound keys should be different")
	}

	// 两个 Key 都应有效
	_, err = mgr.ValidateAPIKey(ctx, key1)
	if err != nil {
		t.Errorf("key1 validation error: %v", err)
	}
	_, err = mgr.ValidateAPIKey(ctx, key2)
	if err != nil {
		t.Errorf("key2 validation error: %v", err)
	}
}

func TestIsolation_WithTenant(t *testing.T) {
	ctx := WithTenant(context.Background(), "t_abc123")
	tid := TenantFromContext(ctx)
	if tid != "t_abc123" {
		t.Errorf("TenantFromContext = %q, want t_abc123", tid)
	}
}

func TestIsolation_TenantFromContext_Empty(t *testing.T) {
	tid := TenantFromContext(context.Background())
	if tid != "" {
		t.Errorf("TenantFromContext(empty) = %q, want empty", tid)
	}
}

func TestIsolation_TenantFromContext_Nil(t *testing.T) {
	tid := TenantFromContext(nil)
	if tid != "" {
		t.Errorf("TenantFromContext(nil) = %q, want empty", tid)
	}
}

func TestIsolation_RequireTenant(t *testing.T) {
	ctx := WithTenant(context.Background(), "t_abc")
	tid, err := RequireTenant(ctx)
	if err != nil {
		t.Fatalf("RequireTenant error: %v", err)
	}
	if tid != "t_abc" {
		t.Errorf("RequireTenant = %q, want t_abc", tid)
	}
}

func TestIsolation_RequireTenant_Missing(t *testing.T) {
	_, err := RequireTenant(context.Background())
	if err != ErrNoTenantInContext {
		t.Errorf("RequireTenant = %v, want ErrNoTenantInContext", err)
	}
}

func TestIsolation_TenantContext(t *testing.T) {
	tc := NewTenantContext(context.Background(), "t_xyz")
	if tc.TenantID() != "t_xyz" {
		t.Errorf("TenantID = %q, want t_xyz", tc.TenantID())
	}
	tid := TenantFromContext(tc)
	if tid != "t_xyz" {
		t.Errorf("TenantFromContext(tc) = %q, want t_xyz", tid)
	}
}

func TestScopedQuery(t *testing.T) {
	q := NewScopedQuery("t_abc")
	q.Set("limit", 10)
	if q.TenantID != "t_abc" {
		t.Errorf("TenantID = %q, want t_abc", q.TenantID)
	}
	v, ok := q.Get("limit")
	if !ok || v != 10 {
		t.Errorf("Get(limit) = %v, %v; want 10, true", v, ok)
	}
}

func TestTenantManager_ImmutableSnapshot(t *testing.T) {
	ctx := context.Background()
	mgr := NewTenantManager()

	created, _, _ := mgr.CreateTenant(ctx, "Test", PlanFree, TenantQuota{}, false)
	created.Metadata["original"] = "yes"

	// GetTenant 返回副本
	got, _ := mgr.GetTenant(ctx, created.ID)
	got.Metadata["original"] = "no"

	// 原有租户不受影响
	original, _ := mgr.GetTenant(ctx, created.ID)
	if original.Metadata["original"] != "yes" {
		t.Error("modifying snapshot should not affect internal state")
	}
}

func TestResourceManager_RegisterAndGet(t *testing.T) {
	ctx := context.Background()
	tm := NewTenantManager()
	rm := NewResourceManager()

	tenant, _, _ := tm.CreateTenant(ctx, "Test", PlanPro, TenantQuota{}, true)
	rm.Register(tenant.ID, tenant.Quotas)

	qm, ok := rm.Get(tenant.ID)
	if !ok {
		t.Fatal("expected to find quota manager")
	}

	// QPS 检查
	err := qm.CheckQPS()
	if err != nil {
		t.Errorf("CheckQPS error: %v", err)
	}

	err = rm.CheckRequest(ctx, tenant.ID, 100)
	if err != nil {
		t.Errorf("CheckRequest error: %v", err)
	}
}

func TestResourceManager_CheckRequest_UnknownTenant(t *testing.T) {
	ctx := context.Background()
	rm := NewResourceManager()

	err := rm.CheckRequest(ctx, "unknown", 100)
	if err != ErrTenantNotFound {
		t.Errorf("CheckRequest(unknown) = %v, want ErrTenantNotFound", err)
	}
}

func TestGenerateTenantID(t *testing.T) {
	id1 := generateTenantID()
	id2 := generateTenantID()
	if id1 == id2 {
		t.Error("tenant IDs should be unique")
	}
	if !strings.HasPrefix(id1, "t_") {
		t.Errorf("tenant ID prefix = %q, want t_", id1[:2])
	}
}
