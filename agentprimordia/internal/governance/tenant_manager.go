package governance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// TenantManager 管理租户的完整生命周期：
// 创建、查询、更新、删除，以及 API Key 与租户的绑定。
//
// 内部使用 sync.RWMutex 保护的内存存储；生产环境可替换为持久化后端，
// 只需实现 TenantStore 接口即可。
type TenantManager struct {
	mu      sync.RWMutex
	tenants map[string]*Tenant // tenantID -> Tenant
	apiKeys map[string]string  // hashed API Key -> tenantID
}

// NewTenantManager 创建一个空的 TenantManager。
func NewTenantManager() *TenantManager {
	return &TenantManager{
		tenants: make(map[string]*Tenant),
		apiKeys: make(map[string]string),
	}
}

// CreateTenant 创建新租户。
//
// 参数:
//   - name: 租户名称
//   - plan: 付费计划（若为空则默认 PlanFree）
//   - quotas: 自定义配额（若为零值则使用 DefaultQuota）
//
// 返回创建的租户和（可选的）明文 API Key。
// 如果 storeAPIKey 为 true，返回值中包含新生成的 API Key（此时应将其
// 安全地传递给调用者，之后仅哈希会被保存）。
func (m *TenantManager) CreateTenant(ctx context.Context, name string, plan TenantPlan, quotas TenantQuota, storeAPIKey bool) (*Tenant, string, error) {
	if name == "" {
		return nil, "", errors.New("governance: tenant name cannot be empty")
	}

	if plan == "" {
		plan = PlanFree
	}

	// 若配额为零值，使用默认
	if quotas == (TenantQuota{}) {
		quotas = DefaultQuota(plan)
	}

	id := generateTenantID()
	tenant := &Tenant{
		ID:        id,
		Name:      name,
		Plan:      plan,
		Quotas:    quotas,
		CreatedAt: time.Now().UTC(),
		Status:    TenantActive,
		Metadata:  make(map[string]string),
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 理论上 UUID 不会冲突，但防御性检查
	if _, exists := m.tenants[id]; exists {
		return nil, "", ErrTenantExists
	}

	m.tenants[id] = tenant

	// 生成 API Key
	var apiKeyPlain string
	if storeAPIKey {
		apiKeyPlain = GenerateAPIKey()
		m.apiKeys[HashAPIKey(apiKeyPlain)] = id
	}

	return tenant, apiKeyPlain, nil
}

// GetTenant 根据 ID 查询租户。
func (m *TenantManager) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tenant, ok := m.tenants[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrTenantNotFound, id)
	}
	// 返回副本以避免外部修改影响内部状态
	cp := *tenant
	if tenant.Metadata != nil {
		cp.Metadata = make(map[string]string, len(tenant.Metadata))
		for k, v := range tenant.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp, nil
}

// UpdateTenant 更新租户的名称、计划或配额。
func (m *TenantManager) UpdateTenant(ctx context.Context, id string, updateFn func(*Tenant) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tenant, ok := m.tenants[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrTenantNotFound, id)
	}
	if err := updateFn(tenant); err != nil {
		return err
	}
	return nil
}

// DisableTenant 禁用租户（不再允许 API 调用）。
func (m *TenantManager) DisableTenant(ctx context.Context, id string) error {
	return m.UpdateTenant(ctx, id, func(t *Tenant) error {
		t.Status = TenantDisabled
		return nil
	})
}

// ArchiveTenant 归档租户（软删除）。
func (m *TenantManager) ArchiveTenant(ctx context.Context, id string) error {
	return m.UpdateTenant(ctx, id, func(t *Tenant) error {
		t.Status = TenantArchived
		return nil
	})
}

// ListTenants 返回所有租户的快照列表。
func (m *TenantManager) ListTenants(ctx context.Context) []*Tenant {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Tenant, 0, len(m.tenants))
	for _, t := range m.tenants {
		cp := *t
		if t.Metadata != nil {
			cp.Metadata = make(map[string]string, len(t.Metadata))
			for k, v := range t.Metadata {
				cp.Metadata[k] = v
			}
		}
		result = append(result, &cp)
	}
	return result
}

// --- API Key 管理 ---

// BindAPIKey 为指定租户绑定新的 API Key。
// 返回明文 Key（应传递给调用者），内部仅存储哈希值。
func (m *TenantManager) BindAPIKey(ctx context.Context, tenantID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tenant, ok := m.tenants[tenantID]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrTenantNotFound, tenantID)
	}
	_ = tenant // 当前不限制 Key 数量，企业版可扩展

	key := GenerateAPIKey()
	m.apiKeys[HashAPIKey(key)] = tenantID
	return key, nil
}

// RevokeAPIKey 撤销指定 API Key。
func (m *TenantManager) RevokeAPIKey(ctx context.Context, apiKeyPlain string) error {
	hashed := HashAPIKey(apiKeyPlain)

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.apiKeys[hashed]; !ok {
		return ErrInvalidAPIKey
	}
	delete(m.apiKeys, hashed)
	return nil
}

// TenantByAPIKey 根据 API Key 查找租户 ID。
// 返回空字符串表示 Key 无效。
func (m *TenantManager) TenantByAPIKey(apiKeyPlain string) (string, error) {
	hashed := HashAPIKey(apiKeyPlain)

	m.mu.RLock()
	defer m.mu.RUnlock()

	tenantID, ok := m.apiKeys[hashed]
	if !ok {
		return "", ErrInvalidAPIKey
	}
	return tenantID, nil
}

// ValidateAPIKey 验证 API Key 并返回对应租户。
// 如果 Key 无效、租户不存在或租户已禁用，返回相应错误。
func (m *TenantManager) ValidateAPIKey(ctx context.Context, apiKeyPlain string) (*Tenant, error) {
	tenantID, err := m.TenantByAPIKey(apiKeyPlain)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	tenant, ok := m.tenants[tenantID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrTenantNotFound, tenantID)
	}
	if !tenant.Active() {
		return nil, ErrTenantDisabled
	}

	cp := *tenant
	return &cp, nil
}

// --- 内部工具 ---

// generateTenantID 生成租户唯一 ID（格式: t_<16字节hex>）。
func generateTenantID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return "t_" + hex.EncodeToString(buf)
}
