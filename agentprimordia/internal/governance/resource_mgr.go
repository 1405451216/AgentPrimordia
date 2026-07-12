package governance

import (
	"context"
	"sync"
)

// ResourceManager 统一管理多个租户的配额监控。
// 每个租户对应一个 QuotaManager 实例，按需创建。
type ResourceManager struct {
	mu       sync.RWMutex
	managers map[string]*QuotaManager
}

// NewResourceManager 创建资源管理器。
func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		managers: make(map[string]*QuotaManager),
	}
}

// Register 为指定租户注册配额管理器。
func (r *ResourceManager) Register(tenantID string, quotas TenantQuota) *QuotaManager {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.managers[tenantID]; ok {
		return existing
	}
	qm := NewQuotaManager(tenantID, quotas)
	r.managers[tenantID] = qm
	return qm
}

// Get 获取指定租户的配额管理器。
func (r *ResourceManager) Get(tenantID string) (*QuotaManager, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	qm, ok := r.managers[tenantID]
	return qm, ok
}

// Remove 移除指定租户的配额管理器。
func (r *ResourceManager) Remove(tenantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.managers, tenantID)
}

// AllStatus 返回所有租户的配额状态快照。
func (r *ResourceManager) AllStatus() []QuotaStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]QuotaStatus, 0, len(r.managers))
	for _, qm := range r.managers {
		result = append(result, qm.Status())
	}
	return result
}

// CheckRequest 检查一次请求是否被允许（QPS + Token 配额）。
// 这是请求入口的统一检查点。
func (r *ResourceManager) CheckRequest(ctx context.Context, tenantID string, tokensNeeded int64) error {
	r.mu.RLock()
	qm, ok := r.managers[tenantID]
	r.mu.RUnlock()
	if !ok {
		return ErrTenantNotFound
	}

	// QPS 检查
	if err := qm.CheckQPS(); err != nil {
		return err
	}

	// Token 配额检查
	if err := qm.RecordTokens(tokensNeeded); err != nil {
		return err
	}

	return nil
}
