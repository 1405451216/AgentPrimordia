// physical_tenant.go — v4.6-1 强租户隔离：过滤器级 → 物理分库
//
// PhysicalTenantStore 为每个租户创建独立的底层 Memory 实例（如独立
// SQLite 文件），物理层面隔离——即使调用方绕过 Metadata 过滤也无法
// 读取其他租户的数据。对比 TenantScoped（过滤器级）：物理分库是
// 存储层隔离，不依赖 metadata 约定。
package memory

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
)

// TenantStoreFactory 每租户底层实例工厂。
type TenantStoreFactory func(tenantID string) (Memory, error)

// SQLiteTenantFactory 返回按租户分文件的 SQLite 工厂（物理分库）。
func SQLiteTenantFactory(dir string) TenantStoreFactory {
	return func(tenantID string) (Memory, error) {
		dsn := filepath.Join(dir, "tenant-"+tenantID+".db")
		return NewSQLiteStore(dsn)
	}
}

// PhysicalTenantStore 物理分库的多租户 Memory：按 tenantID 路由到独立实例。
type PhysicalTenantStore struct {
	mu      sync.RWMutex
	factory TenantStoreFactory
	stores  map[string]Memory
}

// NewPhysicalTenantStore 创建物理分库存储。
func NewPhysicalTenantStore(factory TenantStoreFactory) *PhysicalTenantStore {
	return &PhysicalTenantStore{
		factory: factory,
		stores:  make(map[string]Memory),
	}
}

// TenantStore 获取指定租户的底层实例（懒创建）。
func (p *PhysicalTenantStore) TenantStore(tenantID string) (Memory, error) {
	if tenantID == "" {
		return nil, ErrEmptyTenant
	}
	p.mu.RLock()
	s, ok := p.stores[tenantID]
	p.mu.RUnlock()
	if ok {
		return s, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.stores[tenantID]; ok {
		return s, nil
	}
	s, err := p.factory(tenantID)
	if err != nil {
		return nil, fmt.Errorf("memory: 创建租户 %s 物理库失败: %w", tenantID, err)
	}
	p.stores[tenantID] = s
	return s, nil
}

// withTenant 从 ctx 取租户并路由。未注入租户 → ErrEmptyTenant（物理分库拒绝未租户化访问）。
func (p *PhysicalTenantStore) withTenant(ctx context.Context) (Memory, error) {
	tenantID := TenantFromContext(ctx)
	return p.TenantStore(tenantID)
}

// ---- MemoryReader ----

func (p *PhysicalTenantStore) Get(ctx context.Context, id string) (*Episode, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (p *PhysicalTenantStore) GetBatch(ctx context.Context, ids []string) (map[string]*Episode, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return nil, err
	}
	return s.GetBatch(ctx, ids)
}

func (p *PhysicalTenantStore) Search(ctx context.Context, query string, opts *SearchOptions) ([]*Episode, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return nil, err
	}
	return s.Search(ctx, query, opts)
}

func (p *PhysicalTenantStore) List(ctx context.Context, opts *ListOptions) ([]*Episode, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return nil, err
	}
	return s.List(ctx, opts)
}

func (p *PhysicalTenantStore) Count(ctx context.Context, sessionID string) (int64, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return 0, err
	}
	return s.Count(ctx, sessionID)
}

func (p *PhysicalTenantStore) Stats(ctx context.Context) (*MemoryStats, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return nil, err
	}
	return s.Stats(ctx)
}

// ---- MemoryWriter ----

func (p *PhysicalTenantStore) Add(ctx context.Context, episode *Episode) error {
	s, err := p.withTenant(ctx)
	if err != nil {
		return err
	}
	return s.Add(ctx, episode)
}

func (p *PhysicalTenantStore) AddBatch(ctx context.Context, episodes []*Episode) error {
	s, err := p.withTenant(ctx)
	if err != nil {
		return err
	}
	return s.AddBatch(ctx, episodes)
}

func (p *PhysicalTenantStore) Delete(ctx context.Context, id string) error {
	s, err := p.withTenant(ctx)
	if err != nil {
		return err
	}
	return s.Delete(ctx, id)
}

func (p *PhysicalTenantStore) DeleteBatch(ctx context.Context, ids []string) error {
	s, err := p.withTenant(ctx)
	if err != nil {
		return err
	}
	return s.DeleteBatch(ctx, ids)
}

func (p *PhysicalTenantStore) UpdateSummary(ctx context.Context, id, summary, topics string) error {
	s, err := p.withTenant(ctx)
	if err != nil {
		return err
	}
	return s.UpdateSummary(ctx, id, summary, topics)
}

func (p *PhysicalTenantStore) SetImportance(ctx context.Context, episodeID string, importance float64) error {
	s, err := p.withTenant(ctx)
	if err != nil {
		return err
	}
	return s.SetImportance(ctx, episodeID, importance)
}

// ---- MemorySearcher ----

func (p *PhysicalTenantStore) SearchAdvanced(ctx context.Context, opts SearchOptions) ([]*SearchResult, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return nil, err
	}
	return s.SearchAdvanced(ctx, opts)
}

func (p *PhysicalTenantStore) SearchByTag(ctx context.Context, tag string, opts *SearchOptions) ([]*Episode, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return nil, err
	}
	return s.SearchByTag(ctx, tag, opts)
}

func (p *PhysicalTenantStore) GetImportant(ctx context.Context, threshold float64, limit int) ([]*Episode, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return nil, err
	}
	return s.GetImportant(ctx, threshold, limit)
}

func (p *PhysicalTenantStore) GetTimeline(ctx context.Context, days int) (map[string][]*Episode, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return nil, err
	}
	return s.GetTimeline(ctx, days)
}

// ---- MemoryLifecycle ----

func (p *PhysicalTenantStore) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var firstErr error
	for id, s := range p.stores {
		if err := s.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("memory: 关闭租户 %s 物理库: %w", id, err)
		}
	}
	return firstErr
}

func (p *PhysicalTenantStore) CleanupExpired(ctx context.Context, maxAgeDays int) (int64, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var total int64
	for _, s := range p.stores {
		n, err := s.CleanupExpired(ctx, maxAgeDays)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (p *PhysicalTenantStore) ClearAll(ctx context.Context, sessionID string) error {
	s, err := p.withTenant(ctx)
	if err != nil {
		return err
	}
	return s.ClearAll(ctx, sessionID)
}

// ---- MemoryExporter ----

func (p *PhysicalTenantStore) ExportMemories(ctx context.Context, sessionID, format string) ([]byte, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return nil, err
	}
	return s.ExportMemories(ctx, sessionID, format)
}

func (p *PhysicalTenantStore) ImportMemories(ctx context.Context, data []byte, format string) (int, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return 0, err
	}
	return s.ImportMemories(ctx, data, format)
}

// ---- MemoryQuery ----

func (p *PhysicalTenantStore) GetMemoriesByTag(ctx context.Context, tag string, limit int) ([]*Episode, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return nil, err
	}
	return s.GetMemoriesByTag(ctx, tag, limit)
}

func (p *PhysicalTenantStore) GetMemoriesBySession(ctx context.Context, sessionID string) ([]*Episode, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return nil, err
	}
	return s.GetMemoriesBySession(ctx, sessionID)
}

func (p *PhysicalTenantStore) GetImportantMemories(ctx context.Context, threshold float64, limit int) ([]*Episode, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return nil, err
	}
	return s.GetImportantMemories(ctx, threshold, limit)
}

func (p *PhysicalTenantStore) GetMemoryTimeline(ctx context.Context, days int) ([]*MemoryTimelineGroup, error) {
	s, err := p.withTenant(ctx)
	if err != nil {
		return nil, err
	}
	return s.GetMemoryTimeline(ctx, days)
}

// ---- MemoryToolUse ----

func (p *PhysicalTenantStore) RecordToolUse(ctx context.Context, sessionID, agentName, toolName, args, result string) error {
	s, err := p.withTenant(ctx)
	if err != nil {
		return err
	}
	return s.RecordToolUse(ctx, sessionID, agentName, toolName, args, result)
}

// compile-time 断言
var _ Memory = (*PhysicalTenantStore)(nil)
