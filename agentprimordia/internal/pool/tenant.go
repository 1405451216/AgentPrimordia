// Package pool 的多租户配额层（Phase 5 Task 5）。
//
// 设计目标：
//   - 在 Pool 层为不同 tenant 设置独立的并发配额
//   - AcquireForTenant 在调度任务前校验 tenant 配额
//   - 与 memory.TenantScoped 配合，实现"Memory 隔离 + Pool 配额"双层防护
//
// 公开 API：
//   - TenantQuota：单个 tenant 的配额（MaxConcurrency / MaxTasksPerMinute / Burst）
//   - TenantRegistry：集中管理多个 tenant 的配额
//   - Pool.AcquireForTenant / SubmitForTenant：在 Pool 上接入租户维度
//
// 限制：
//   - 当前实现是"软配额"——超限返回 ErrTenantQuotaExceeded，由调用方决定重试/拒绝
//   - 不持久化配额状态，重启后从配置重建
//   - Burst 是令牌桶容量；MaxTasksPerMinute 是补充速率
package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// TenantQuota 描述单个租户的并发与速率配额。
//
// 零值表示"无任何限制"。DefaultTenantQuota() 提供保守默认值。
type TenantQuota struct {
	// MaxConcurrency 同 tenant 最大并发任务数；<=0 表示不限制
	MaxConcurrency int

	// MaxTasksPerMinute 每分钟最大任务数；<=0 表示不限制
	MaxTasksPerMinute int

	// Burst 令牌桶容量；默认等于 MaxTasksPerMinute 的 1/6（10 秒突发）
	Burst int
}

// DefaultTenantQuota 返回默认保守配额：并发 4，每分钟 60 次任务。
func DefaultTenantQuota() TenantQuota {
	return TenantQuota{
		MaxConcurrency:    4,
		MaxTasksPerMinute: 60,
	}
}

// Validate 检查配额合法性。
func (q TenantQuota) Validate() error {
	if q.MaxConcurrency < 0 {
		return fmt.Errorf("pool: TenantQuota.MaxConcurrency 不能为负")
	}
	if q.MaxTasksPerMinute < 0 {
		return fmt.Errorf("pool: TenantQuota.MaxTasksPerMinute 不能为负")
	}
	if q.Burst < 0 {
		return fmt.Errorf("pool: TenantQuota.Burst 不能为负")
	}
	return nil
}

// tenantBucket 是单个 tenant 的令牌桶状态。
type tenantBucket struct {
	tokens    float64
	lastRefill time.Time
}

// refill 按时间间隔补充令牌。
func (b *tenantBucket) refill(now time.Time, ratePerMin, burst int) {
	if ratePerMin <= 0 {
		return // 无限流
	}
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed <= 0 {
		return
	}
	tokensToAdd := elapsed * float64(ratePerMin) / 60.0
	b.tokens = min(float64(burst), b.tokens+tokensToAdd)
	b.lastRefill = now
}

// consume 尝试消费 1 个令牌；返回是否成功。
func (b *tenantBucket) consume(now time.Time, ratePerMin, burst int) bool {
	b.refill(now, ratePerMin, burst)
	if b.tokens >= 1 {
		b.tokens -= 1
		return true
	}
	return false
}

// TenantEntry 集中存储某个 tenant 的配额与运行时状态。
type TenantEntry struct {
	quota       TenantQuota
	concurrency atomic.Int64

	bucketMu sync.Mutex
	bucket   tenantBucket
}

// NewTenantEntry 用给定的 quota 构造 entry。
func NewTenantEntry(quota TenantQuota) *TenantEntry {
	return &TenantEntry{
		quota: quota,
		bucket: tenantBucket{
			tokens:    float64(quota.Burst),
			lastRefill: time.Now(),
		},
	}
}

// Quota 返回 entry 的配额（拷贝）。
func (e *TenantEntry) Quota() TenantQuota { return e.quota }

// Concurrency 返回当前并发数。
func (e *TenantEntry) Concurrency() int64 { return e.concurrency.Load() }

// tryAcquire 尝试抢占 1 个并发槽位 + 1 个令牌。
func (e *TenantEntry) tryAcquire(now time.Time) error {
	if e.quota.MaxConcurrency > 0 {
		cur := e.concurrency.Add(1)
		if cur > int64(e.quota.MaxConcurrency) {
			e.concurrency.Add(-1)
			return fmt.Errorf("%w: tenant 并发已满 (cur=%d max=%d)", ErrTenantQuotaExceeded, cur-1, e.quota.MaxConcurrency)
		}
	}

	if e.quota.MaxTasksPerMinute > 0 {
		e.bucketMu.Lock()
		ok := e.bucket.consume(now, e.quota.MaxTasksPerMinute, e.quota.Burst)
		e.bucketMu.Unlock()
		if !ok {
			if e.quota.MaxConcurrency > 0 {
				e.concurrency.Add(-1) // 回滚并发槽
			}
			return fmt.Errorf("%w: tenant 速率已满 (burst=%d)", ErrTenantQuotaExceeded, e.quota.Burst)
		}
	}

	return nil
}

// release 释放并发槽位。
func (e *TenantEntry) release() {
	if e.quota.MaxConcurrency > 0 {
		e.concurrency.Add(-1)
	}
}

// ErrTenantQuotaExceeded 表示 tenant 配额耗尽（并发或速率）。
var ErrTenantQuotaExceeded = errors.New("pool: tenant 配额耗尽")

// TenantRegistry 集中管理 tenant 配额。
type TenantRegistry struct {
	mu       sync.RWMutex
	entries  map[string]*TenantEntry
	factory  func(tenantID string) (TenantQuota, error)
	defaultQ TenantQuota
}

// NewTenantRegistry 用 factory 构造 registry；factory 返回 tenant 对应的配额。
//
//   - factory == nil 时使用 defaultQ 作为所有 tenant 的配额
//   - defaultQ 在 factory 对未知 tenant 返回零值时使用
func NewTenantRegistry(factory func(tenantID string) (TenantQuota, error), defaultQ TenantQuota) *TenantRegistry {
	return &TenantRegistry{
		entries:  make(map[string]*TenantEntry),
		factory:  factory,
		defaultQ: defaultQ,
	}
}

// GetOrCreate 返回指定 tenant 的 entry；不存在则按 factory/defaultQ 创建。
func (r *TenantRegistry) GetOrCreate(tenantID string) (*TenantEntry, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("pool: tenantID 不能为空")
	}
	r.mu.RLock()
	entry, ok := r.entries[tenantID]
	r.mu.RUnlock()
	if ok {
		return entry, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if entry, ok := r.entries[tenantID]; ok {
		return entry, nil
	}

	var q TenantQuota
	if r.factory != nil {
		var err error
		q, err = r.factory(tenantID)
		if err != nil {
			return nil, fmt.Errorf("pool: 查询 tenant %s 配额失败：%w", tenantID, err)
		}
	}
	if q.MaxConcurrency == 0 && q.MaxTasksPerMinute == 0 {
		q = r.defaultQ
	}
	if err := q.Validate(); err != nil {
		return nil, err
	}
	entry = NewTenantEntry(q)
	r.entries[tenantID] = entry
	return entry, nil
}

// Forget 从 registry 中移除 tenant。
func (r *TenantRegistry) Forget(tenantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, tenantID)
}

// ForgetAll 清空 registry。
func (r *TenantRegistry) ForgetAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = make(map[string]*TenantEntry)
}

// Tenants 返回已注册 tenantID 列表。
func (r *TenantRegistry) Tenants() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.entries))
	for id := range r.entries {
		out = append(out, id)
	}
	return out
}

// Snapshot 返回所有 tenant 的快照。
func (r *TenantRegistry) Snapshot() []TenantSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]TenantSnapshot, 0, len(r.entries))
	for id, e := range r.entries {
		out = append(out, TenantSnapshot{
			TenantID:    id,
			Quota:       e.quota,
			Concurrency: e.concurrency.Load(),
		})
	}
	return out
}

// TenantSnapshot 描述一个 tenant 的当前状态。
type TenantSnapshot struct {
	TenantID    string
	Quota       TenantQuota
	Concurrency int64
}

// ===========================================================================
// Pool 集成：AcquireForTenant / SubmitForTenant
// ===========================================================================

// AcquireForTenant 抢占 tenant 维度的执行槽；返回 release 函数。
//
// 错误：
//   - ErrTenantQuotaExceeded：并发或速率超限
//   - 其他错误：tenantID 为空 / factory 失败
//
// 调用方负责在任务结束时调用 release()。
func (p *Pool) AcquireForTenant(tenantID string) (release func(), err error) {
	if p.tenantRegistry == nil {
		// Pool 未启用 tenant 配额，直接返回 no-op release
		return func() {}, nil
	}
	entry, err := p.tenantRegistry.GetOrCreate(tenantID)
	if err != nil {
		return nil, err
	}
	if err := entry.tryAcquire(time.Now()); err != nil {
		return nil, err
	}
	var released int32
	release = func() {
		if atomic.CompareAndSwapInt32(&released, 0, 1) {
			entry.release()
		}
	}
	return release, nil
}

// SubmitForTenant 在 tenant 配额下提交单任务。
//
// 与 Dispatch 的区别：先校验 tenant 配额，超限立即返回错误（不会排队）。
// 适合"按租户优先级排队"的场景。
//
// 注意：当前实现仅做准入校验（配额通过后立即释放并发槽）。
// 真正的并发执行仍由 Pool 调度器统一控制；调用方可在拿到 *TaskResult
// 后通过其他机制做租户维度的限流。
func (p *Pool) SubmitForTenant(tenantID string, task TaskConfig) (*TaskResult, error) {
	release, err := p.AcquireForTenant(tenantID)
	if err != nil {
		return nil, err
	}
	// 立即释放并发槽——只做准入校验
	release()

	results, err := p.Dispatch(context.Background(), []TaskConfig{task})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("pool: Dispatch 返回空结果")
	}
	return results[0], nil
}

// TenantRegistry 返回 Pool 关联的租户 registry（如未设置返回 nil）。
func (p *Pool) TenantRegistry() *TenantRegistry { return p.tenantRegistry }

// EnableTenantRegistry 在 Pool 上启用 tenant 配额管理；启用后可通过
// AcquireForTenant / SubmitForTenant 接入。
func (p *Pool) EnableTenantRegistry(reg *TenantRegistry) {
	p.tenantRegistry = reg
}

// TenantStats 返回当前 Pool 的所有 tenant 快照。
func (p *Pool) TenantStats() []TenantSnapshot {
	if p.tenantRegistry == nil {
		return nil
	}
	return p.tenantRegistry.Snapshot()
}