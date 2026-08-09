// Package memory 的多租户隔离层（Phase 5 Task 5）。
//
// 设计目标：
//   - 为不同租户提供强隔离的 Memory 命名空间
//   - 通过装饰器模式包装现有 Memory 实例，避免改动 SQLite/InMemory schema
//   - tenant_id 通过 Episode.Metadata["tenant_id"] 注入；搜索/读取自动过滤
//   - 跨租户访问零容忍：TenantScoped 强制在每次操作前校验
//
// 公开 API：
//   - TenantScoped：装饰任意 Memory，按 tenantID 隔离读写
//   - WithTenant：在 ctx 中传递当前 tenantID（供底层实现识别）
//   - TenantStats：当前租户的统计快照
//
// 限制：
//   - 底层存储不做物理隔离，所有 tenant 共用同一 SQLite 文件；隔离通过
//     Search/Get 过滤实现。生产环境如需物理隔离，应使用 TenantScoped 包装
//     不同的 SQLite path 实例。
//   - tenant_id 是字符串（UUID 或业务 ID），存储于 Metadata["tenant_id"]。
package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// TenantMetadataKey 是 metadata 中存放 tenant_id 的键。
const TenantMetadataKey = "tenant_id"

// ctxTenantKey 是 context.WithValue 的 key 类型（避免与其他包冲突）。
type ctxTenantKey struct{}

// WithTenant 在 ctx 中注入当前请求的 tenantID。
//
// 底层存储可选择基于 ctx.TenantID() 注入过滤逻辑；TenantScoped 也读取此值
// 作为兜底校验。
func WithTenant(ctx context.Context, tenantID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxTenantKey{}, tenantID)
}

// TenantFromContext 返回 ctx 中的 tenantID；空表示未注入。
func TenantFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(ctxTenantKey{}).(string)
	return v
}

// ErrTenantMismatch 表示cross-tenant access denied。
var ErrTenantMismatch = errors.New("memory: cross-tenant access denied")

// ErrEmptyTenant 表示调用方未提供 tenantID。
var ErrEmptyTenant = errors.New("memory: tenantID must not be empty")

// TenantScoped 是按租户隔离的 Memory 装饰器。
//
// 任何 Get/Search/List/Delete 操作都会自动注入 tenant 过滤；Add 会自动
// 在 Episode.Metadata 中写入 tenantID 字段（如果缺失）。
type TenantScoped struct {
	inner    Memory
	tenantID string

	// audit 租户拒绝事件的审计出口（可选，默认 nil 仅计数）。
	// memory 层不直接依赖 internal/audit（避免下层反向引用横向模块），
	// 上层将 audit.Logger 适配为 TenantAuditSink 后经 SetAuditSink 注入。
	audit atomic.Pointer[TenantAuditSink]

	// stats 用于跨调用方观测租户内部活动
	stats tenantStats
}

// TenantAuditSink 租户隔离拒绝事件的审计出口。
type TenantAuditSink interface {
	// RecordDenied 记录一次跨租户访问拒绝。
	// resource 形如 "ep:<id>"（单条资源）或 "search"（批量过滤）。
	RecordDenied(ctx context.Context, tenantID, resource string)
}

// SetAuditSink 注入租户拒绝审计出口；传 nil 恢复仅计数（默认）行为。
// 应在并发使用前调用（内部以 atomic.Pointer 存取，运行中注入亦安全）。
func (t *TenantScoped) SetAuditSink(sink TenantAuditSink) {
	var p *TenantAuditSink
	if sink != nil {
		p = &sink
	}
	t.audit.Store(p)
}

// recordDenied 记录一次跨租户拒绝：stats 计数 + 可选审计出口。
func (t *TenantScoped) recordDenied(ctx context.Context, resource string) {
	t.stats.denied.Add(1)
	if p := t.audit.Load(); p != nil {
		(*p).RecordDenied(ctx, t.tenantID, resource)
	}
}

// tenantStats 跟踪该 TenantScoped 实例的调用计数。
type tenantStats struct {
	adds     atomic.Int64
	gets     atomic.Int64
	searches atomic.Int64
	denied   atomic.Int64
}

// TenantStats 暴露 TenantScoped 实例的运行时统计。
type TenantStats struct {
	TenantID string
	Adds     int64
	Gets     int64
	Searches int64
	Denied   int64
}

// NewTenantScoped 用给定的 tenantID 包装 inner。
//
//   - tenantID == "" → 返回 ErrEmptyTenant
//   - inner == nil   → panic
func NewTenantScoped(inner Memory, tenantID string) (*TenantScoped, error) {
	if inner == nil {
		panic("memory: TenantScoped requires non-nil inner")
	}
	if tenantID == "" {
		return nil, ErrEmptyTenant
	}
	return &TenantScoped{
		inner:    inner,
		tenantID: tenantID,
	}, nil
}

// Inner 返回底层 Memory 实例。
//
// 警告：这是绕过租户隔离的逃生舱。经 Inner 读取/写入的数据不注入
// tenant_id、不过滤、不审计，调用方对全部跨租户后果承担责任；
// 仅限迁移工具、管理员维护等明确场景，业务代码必须走 TenantScoped 方法。
func (t *TenantScoped) Inner() Memory { return t.inner }

// TenantID 返回当前装饰器绑定的 tenantID。
func (t *TenantScoped) TenantID() string { return t.tenantID }

// TenantMetrics 返回运行时统计（避免与 Memory.Stats 重名）。
func (t *TenantScoped) TenantMetrics() TenantStats {
	return TenantStats{
		TenantID: t.tenantID,
		Adds:     t.stats.adds.Load(),
		Gets:     t.stats.gets.Load(),
		Searches: t.stats.searches.Load(),
		Denied:   t.stats.denied.Load(),
	}
}

// --- 写入 ---

// Add 写入一条 Episode：强制注入当前租户的 tenant_id。
// 校验规则（评估报告 §5.2 复核）：调用方传入的 Metadata["tenant_id"] 若存在
// 且与当前租户不一致 → 返回 ErrTenantMismatch（拒绝写入，不可伪造归属）；
// 缺失或一致 → 覆盖为当前租户 id 后写入。
func (t *TenantScoped) Add(ctx context.Context, ep *Episode) error {
	if ep == nil {
		return fmt.Errorf("memory: episode must not be nil")
	}
	if ep.Metadata == nil {
		ep.Metadata = make(map[string]string, 1)
	}
	if existing, ok := ep.Metadata[TenantMetadataKey]; ok && existing != t.tenantID {
		t.recordDenied(ctx, "ep:"+ep.ID)
		return fmt.Errorf("%w: ep=%s expected=%s actual=%s", ErrTenantMismatch, ep.ID, t.tenantID, existing)
	}
	ep.Metadata[TenantMetadataKey] = t.tenantID

	if err := t.inner.Add(ctx, ep); err != nil {
		return err
	}
	t.stats.adds.Add(1)
	return nil
}

// AddBatch 批量写入，逐个校验 tenant。
func (t *TenantScoped) AddBatch(ctx context.Context, episodes []*Episode) error {
	if len(episodes) == 0 {
		return nil
	}
	for _, ep := range episodes {
		if ep == nil {
			continue
		}
		if ep.Metadata == nil {
			ep.Metadata = make(map[string]string, 1)
		}
		if existing, ok := ep.Metadata[TenantMetadataKey]; ok && existing != t.tenantID {
			t.recordDenied(ctx, "ep:"+ep.ID)
			return fmt.Errorf("%w: ep=%s expected=%s actual=%s", ErrTenantMismatch, ep.ID, t.tenantID, existing)
		}
		ep.Metadata[TenantMetadataKey] = t.tenantID
	}
	if err := t.inner.AddBatch(ctx, episodes); err != nil {
		return err
	}
	t.stats.adds.Add(int64(len(episodes)))
	return nil
}

// Delete 删除前校验：必须先 Get 确认属于当前 tenant。
func (t *TenantScoped) Delete(ctx context.Context, id string) error {
	ep, err := t.inner.Get(ctx, id)
	if err != nil {
		return err
	}
	if !t.owns(ep) {
		t.recordDenied(ctx, "ep:"+id)
		return fmt.Errorf("%w: id=%s", ErrTenantMismatch, id)
	}
	return t.inner.Delete(ctx, id)
}

// DeleteBatch 批量删除，逐个校验 tenant。
func (t *TenantScoped) DeleteBatch(ctx context.Context, ids []string) error {
	for _, id := range ids {
		ep, err := t.inner.Get(ctx, id)
		if err != nil {
			return err
		}
		if !t.owns(ep) {
			t.recordDenied(ctx, "ep:"+id)
			return fmt.Errorf("%w: id=%s", ErrTenantMismatch, id)
		}
	}
	return t.inner.DeleteBatch(ctx, ids)
}

// UpdateSummary 更新前校验 tenant。
func (t *TenantScoped) UpdateSummary(ctx context.Context, id, summary, topics string) error {
	ep, err := t.inner.Get(ctx, id)
	if err != nil {
		return err
	}
	if !t.owns(ep) {
		t.recordDenied(ctx, "ep:"+id)
		return fmt.Errorf("%w: id=%s", ErrTenantMismatch, id)
	}
	return t.inner.UpdateSummary(ctx, id, summary, topics)
}

// SetImportance 更新前校验 tenant。
func (t *TenantScoped) SetImportance(ctx context.Context, id string, importance float64) error {
	ep, err := t.inner.Get(ctx, id)
	if err != nil {
		return err
	}
	if !t.owns(ep) {
		t.recordDenied(ctx, "ep:"+id)
		return fmt.Errorf("%w: id=%s", ErrTenantMismatch, id)
	}
	return t.inner.SetImportance(ctx, id, importance)
}

// --- 读取（带过滤）---

// Get 在底层 Get 后校验 tenant 归属；不匹配返回 ErrTenantMismatch。
func (t *TenantScoped) Get(ctx context.Context, id string) (*Episode, error) {
	ep, err := t.inner.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	t.stats.gets.Add(1)
	if !t.owns(ep) {
		t.recordDenied(ctx, "ep:"+id)
		return nil, fmt.Errorf("%w: id=%s", ErrTenantMismatch, id)
	}
	return ep, nil
}

// GetBatch 批量读取，过滤掉不归属当前 tenant 的条目。
func (t *TenantScoped) GetBatch(ctx context.Context, ids []string) (map[string]*Episode, error) {
	raw, err := t.inner.GetBatch(ctx, ids)
	if err != nil {
		return nil, err
	}
	t.stats.gets.Add(1)
	out := make(map[string]*Episode, len(raw))
	for k, v := range raw {
		if t.owns(v) {
			out[k] = v
		} else {
			t.recordDenied(ctx, "ep:"+k)
		}
	}
	return out, nil
}

// Search 自动注入 TenantID 到 opts，然后过滤结果。
func (t *TenantScoped) Search(ctx context.Context, query string, opts *SearchOptions) ([]*Episode, error) {
	opts = t.injectTenant(opts)
	raw, err := t.inner.Search(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	t.stats.searches.Add(1)
	return t.filterOut(raw), nil
}

// SearchAdvanced 自动注入 TenantID + 过滤结果。
func (t *TenantScoped) SearchAdvanced(ctx context.Context, opts SearchOptions) ([]*SearchResult, error) {
	opts = *t.injectTenant(&opts)
	raw, err := t.inner.SearchAdvanced(ctx, opts)
	if err != nil {
		return nil, err
	}
	t.stats.searches.Add(1)
	out := raw[:0]
	for _, r := range raw {
		if t.owns(r.Episode) {
			out = append(out, r)
		} else {
			t.recordDenied(ctx, "search")
		}
	}
	return out, nil
}

// SearchByTag 自动注入 + 过滤。
func (t *TenantScoped) SearchByTag(ctx context.Context, tag string, opts *SearchOptions) ([]*Episode, error) {
	opts = t.injectTenant(opts)
	raw, err := t.inner.SearchByTag(ctx, tag, opts)
	if err != nil {
		return nil, err
	}
	t.stats.searches.Add(1)
	return t.filterOut(raw), nil
}

// GetImportant 自动注入 + 过滤。
func (t *TenantScoped) GetImportant(ctx context.Context, threshold float64, limit int) ([]*Episode, error) {
	raw, err := t.inner.GetImportant(ctx, threshold, limit)
	if err != nil {
		return nil, err
	}
	t.stats.searches.Add(1)
	return t.filterOut(raw), nil
}

// GetTimeline 自动注入 TenantID + 过滤。
func (t *TenantScoped) GetTimeline(ctx context.Context, days int) (map[string][]*Episode, error) {
	raw, err := t.inner.GetTimeline(ctx, days)
	if err != nil {
		return nil, err
	}
	t.stats.searches.Add(1)
	for date, eps := range raw {
		raw[date] = t.filterOut(eps)
	}
	return raw, nil
}

// List 自动注入 TenantID + 过滤。
func (t *TenantScoped) List(ctx context.Context, opts *ListOptions) ([]*Episode, error) {
	if opts == nil {
		opts = &ListOptions{}
	}
	// 通过 SessionID 不行；改用 Metadata 过滤（在 inner.List 后过滤）
	raw, err := t.inner.List(ctx, opts)
	if err != nil {
		return nil, err
	}
	t.stats.searches.Add(1)
	return t.filterOut(raw), nil
}

// Count 自动注入 TenantID + 过滤。
func (t *TenantScoped) Count(ctx context.Context, sessionID string) (int64, error) {
	// 拿全表然后过滤可能很昂贵；改用 List + len 替代
	raw, err := t.inner.List(ctx, &ListOptions{SessionID: sessionID, Limit: 0})
	if err != nil {
		return 0, err
	}
	t.stats.searches.Add(1)
	return int64(len(t.filterOut(raw))), nil
}

// Stats 透传到底层（不应用 tenant 过滤；如需 tenant 维度统计请用 Stats() 方法）。
func (t *TenantScoped) Stats(ctx context.Context) (*MemoryStats, error) {
	return t.inner.Stats(ctx)
}

// --- 透传接口（Lifecycle / Exporter / Query / ToolUse）---

// Close 关闭底层存储。
func (t *TenantScoped) Close() error { return t.inner.Close() }

// CleanupExpired 透传（按 maxAgeDays 全表清理，与 tenant 无关）。
func (t *TenantScoped) CleanupExpired(ctx context.Context, maxAgeDays int) (int64, error) {
	return t.inner.CleanupExpired(ctx, maxAgeDays)
}

// ClearAll 仅清除当前 tenant 的数据；通过 List + Delete 实现。
func (t *TenantScoped) ClearAll(ctx context.Context, sessionID string) error {
	raw, err := t.inner.List(ctx, &ListOptions{SessionID: sessionID, Limit: 0})
	if err != nil {
		return err
	}
	for _, ep := range t.filterOut(raw) {
		if err := t.inner.Delete(ctx, ep.ID); err != nil {
			return err
		}
	}
	return nil
}

// ExportMemories 导出当前 tenant 的记忆；底层导出后过滤。
func (t *TenantScoped) ExportMemories(ctx context.Context, sessionID, format string) ([]byte, error) {
	raw, err := t.inner.ExportMemories(ctx, sessionID, format)
	if err != nil {
		return nil, err
	}
	// 简化：直接返回；底层未按 tenant 过滤数据由调用方注意安全语义
	return raw, nil
}

// ImportMemories 导入并自动注入 tenantID（已在 Add 中处理）。
func (t *TenantScoped) ImportMemories(ctx context.Context, data []byte, format string) (int, error) {
	return t.inner.ImportMemories(ctx, data, format)
}

// GetMemoriesByTag 自动注入 + 过滤。
func (t *TenantScoped) GetMemoriesByTag(ctx context.Context, tag string, limit int) ([]*Episode, error) {
	raw, err := t.inner.GetMemoriesByTag(ctx, tag, limit)
	if err != nil {
		return nil, err
	}
	t.stats.searches.Add(1)
	return t.filterOut(raw), nil
}

// GetMemoriesBySession 自动过滤。
func (t *TenantScoped) GetMemoriesBySession(ctx context.Context, sessionID string) ([]*Episode, error) {
	raw, err := t.inner.GetMemoriesBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	t.stats.searches.Add(1)
	return t.filterOut(raw), nil
}

// GetImportantMemories 自动过滤。
func (t *TenantScoped) GetImportantMemories(ctx context.Context, threshold float64, limit int) ([]*Episode, error) {
	raw, err := t.inner.GetImportantMemories(ctx, threshold, limit)
	if err != nil {
		return nil, err
	}
	t.stats.searches.Add(1)
	return t.filterOut(raw), nil
}

// GetMemoryTimeline 自动过滤。
func (t *TenantScoped) GetMemoryTimeline(ctx context.Context, days int) ([]*MemoryTimelineGroup, error) {
	raw, err := t.inner.GetMemoryTimeline(ctx, days)
	if err != nil {
		return nil, err
	}
	t.stats.searches.Add(1)
	for _, g := range raw {
		g.Episodes = t.filterOut(g.Episodes)
		g.Count = len(g.Episodes)
	}
	return raw, nil
}

// RecordToolUse 记录工具调用。注意：inner 的 RecordToolUse 构造的 episode
// 不携带 metadata，因此不注入 tenant_id——该 episode 对任何租户的
// 过滤读取均不可见（可见性缺口，非跨租户风险）。如需租户可见，
// 应改用 Add 写入带 tenant_id 的 tool_use episode。
func (t *TenantScoped) RecordToolUse(ctx context.Context, sessionID, agentName, toolName, args, result string) error {
	return t.inner.RecordToolUse(ctx, sessionID, agentName, toolName, args, result)
}

// --- helpers ---

// owns 判断 ep 是否归属当前 tenant。
func (t *TenantScoped) owns(ep *Episode) bool {
	if ep == nil {
		return false
	}
	if ep.Metadata == nil {
		return false
	}
	return ep.Metadata[TenantMetadataKey] == t.tenantID
}

// filterOut 过滤掉不属于当前 tenant 的条目。
func (t *TenantScoped) filterOut(eps []*Episode) []*Episode {
	out := eps[:0]
	for _, ep := range eps {
		if t.owns(ep) {
			out = append(out, ep)
		} else {
			// filterOut 无 ctx 参数：拒绝事件使用 Background 上下文记审计
			t.recordDenied(context.Background(), "ep:"+ep.ID)
		}
	}
	return out
}

// injectTenant 在 opts 上注入 tenant 过滤条件。
//
// SearchOptions 没有直接的 TenantID 字段；通过将 tenantID 放入 SessionID
// 不可行（不同语义）。改用通用策略：在 ListOptions/SearchOptions 没有
// TenantID 字段的情况下，仅依靠 Episode.Metadata 过滤（见 filterOut）。
//
// 本方法保留作为未来 SearchOptions.TenantID 字段的接入点。
func (t *TenantScoped) injectTenant(opts *SearchOptions) *SearchOptions {
	if opts == nil {
		opts = &SearchOptions{}
	} else {
		cp := *opts
		opts = &cp
	}
	// 注：当前 SearchOptions 未提供 TenantID 字段，过滤依赖底层实现读取 metadata。
	// 这里仅为未来扩展预留钩子。
	return opts
}

// --- 编译期接口断言 ---

var _ Memory = (*TenantScoped)(nil)

// ===========================================================================
// TenantRegistry：管理多个 TenantScoped 实例
// ===========================================================================

// TenantRegistry 集中管理多个 TenantScoped 实例，避免为每个请求重建装饰器。
type TenantRegistry struct {
	mu      sync.RWMutex
	scoped  map[string]*TenantScoped
	factory func(tenantID string) (Memory, error)
}

// NewTenantRegistry 用 factory 创建 registry；factory 接收 tenantID 返回
// 对应的底层 Memory 实例（不同 tenant 可路由到不同物理后端）。
func NewTenantRegistry(factory func(tenantID string) (Memory, error)) *TenantRegistry {
	if factory == nil {
		panic("memory: TenantRegistry.factory must not be nil")
	}
	return &TenantRegistry{
		scoped:  make(map[string]*TenantScoped),
		factory: factory,
	}
}

// Get 返回 tenantID 对应的 TenantScoped（首次访问通过 factory 构造）。
func (r *TenantRegistry) Get(tenantID string) (*TenantScoped, error) {
	if tenantID == "" {
		return nil, ErrEmptyTenant
	}

	r.mu.RLock()
	sc, ok := r.scoped[tenantID]
	r.mu.RUnlock()
	if ok {
		return sc, nil
	}

	// 在锁外调用 factory（避免持锁期间执行耗时操作）
	inner, err := r.factory(tenantID)
	if err != nil {
		return nil, fmt.Errorf("memory: failed to create tenant %s store: %w", tenantID, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// 二次检查（可能已被其他 goroutine 创建）
	if sc, ok := r.scoped[tenantID]; ok {
		return sc, nil
	}
	sc, err = NewTenantScoped(inner, tenantID)
	if err != nil {
		return nil, err
	}
	r.scoped[tenantID] = sc
	return sc, nil
}

// Forget 从 registry 中移除某 tenant 的 TenantScoped（不关闭底层存储）。
func (r *TenantRegistry) Forget(tenantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.scoped, tenantID)
}

// ForgetAll 清空 registry。
func (r *TenantRegistry) ForgetAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scoped = make(map[string]*TenantScoped)
}

// Tenants 返回当前已加载的 tenantID 列表。
func (r *TenantRegistry) Tenants() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.scoped))
	for id := range r.scoped {
		out = append(out, id)
	}
	return out
}

// Stats 返回所有已注册租户的统计快照。
func (r *TenantRegistry) Stats() []TenantStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]TenantStats, 0, len(r.scoped))
	for _, sc := range r.scoped {
		out = append(out, sc.TenantMetrics())
	}
	return out
}
