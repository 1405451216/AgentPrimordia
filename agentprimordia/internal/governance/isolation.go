package governance

import (
	"context"
	"errors"
)

// ctxGovernanceKey 是 context 中存储租户 ID 的 key 类型。
// 使用独立类型避免与其他包的 key 冲突。
type ctxGovernanceKey struct{}

// WithTenant 将 tenantID 注入 context。
// 在请求入口处调用（如 HTTP / gRPC middleware），
// 后续 TenantFromContext 即可取出。
func WithTenant(ctx context.Context, tenantID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxGovernanceKey{}, tenantID)
}

// TenantFromContext 从 context 中提取 tenantID。
// 空字符串表示未注入租户信息。
func TenantFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(ctxGovernanceKey{}).(string)
	return v
}

// ErrNoTenantInContext 表示 context 中未携带租户信息。
var ErrNoTenantInContext = errors.New("governance: no tenant in context")

// RequireTenant 从 context 中提取 tenantID，若不存在则返回错误。
func RequireTenant(ctx context.Context) (string, error) {
	tid := TenantFromContext(ctx)
	if tid == "" {
		return "", ErrNoTenantInContext
	}
	return tid, nil
}

// TenantContext 是一个辅助结构体，用于携带租户信息的 context 包装。
// 它提供了一种类型安全的上下文传递方式。
type TenantContext struct {
	context.Context
	tenantID string
}

// NewTenantContext 创建一个携带租户 ID 的 context。
func NewTenantContext(ctx context.Context, tenantID string) *TenantContext {
	return &TenantContext{
		Context:  WithTenant(ctx, tenantID),
		tenantID: tenantID,
	}
}

// TenantID 返回租户 ID。
func (c *TenantContext) TenantID() string {
	return c.tenantID
}

// --- TenantScope：自动注入 tenant_id 的查询选项 ---

// TenantScopedFilter 是数据隔离的辅助接口。
// 底层存储（如 memory.TenantScoped）可实现此接口以自动注入过滤条件。
type TenantScopedFilter interface {
	// FilterByTenant 返回指定 tenant_id 的查询过滤条件。
	FilterByTenant(tenantID string) (key string, value interface{})
}

// ScopedQuery 包装查询参数，自动附加 tenant_id 过滤。
type ScopedQuery struct {
	TenantID string
	// Params 包含其他查询参数
	Params map[string]interface{}
}

// NewScopedQuery 创建带 tenant_id 的查询参数。
func NewScopedQuery(tenantID string) *ScopedQuery {
	return &ScopedQuery{
		TenantID: tenantID,
		Params:   make(map[string]interface{}),
	}
}

// Set 设置查询参数。
func (q *ScopedQuery) Set(key string, value interface{}) *ScopedQuery {
	q.Params[key] = value
	return q
}

// Get 获取查询参数。
func (q *ScopedQuery) Get(key string) (interface{}, bool) {
	v, ok := q.Params[key]
	return v, ok
}
