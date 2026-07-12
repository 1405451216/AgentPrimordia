package dag

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// HotMigrationStrategy 热迁移策略类型
type HotMigrationStrategy string

const (
	// HotMigrationKeepRunning 已完成节点保持，运行中节点继续，未开始节点使用新版本
	HotMigrationKeepRunning HotMigrationStrategy = "keep_running"
	// HotMigrationRestartAll 全部使用新版本重新执行
	HotMigrationRestartAll HotMigrationStrategy = "restart_all"
	// HotMigrationGradual 逐步切换：按批次迁移节点
	HotMigrationGradual HotMigrationStrategy = "gradual"
)

// NodeMigrationState 节点迁移状态
type NodeMigrationState string

const (
	// NodeStatePending 节点尚未开始执行
	NodeStatePending NodeMigrationState = "pending"
	// NodeStateRunning 节点正在执行中
	NodeStateRunning NodeMigrationState = "running"
	// NodeStateCompleted 节点已完成
	NodeStateCompleted NodeMigrationState = "completed"
	// NodeStateMigrated 节点已迁移到新版本
	NodeStateMigrated NodeMigrationState = "migrated"
)

// NodeMigrationRecord 单个节点的迁移记录
type NodeMigrationRecord struct {
	NodeID      string
	OldState    NodeMigrationState
	NewState    NodeMigrationState
	Result      *DAGNodeResult
	Migrated    bool
}

// MigrationPlan 迁移计划
type MigrationPlan struct {
	// KeepNodes 在新旧版本中均存在且无需变更的节点
	KeepNodes []string
	// AddNodes 新增的节点（旧版本中不存在）
	AddNodes []string
	// RemoveNodes 删除的节点（新版本中不存在）
	RemoveNodes []string
	// ModifyNodes 结构有变更的节点（如 Metadata、RetryPolicy 不同）
	ModifyNodes []string
	// Mapping 旧节点 ID 到新节点 ID 的映射（重命名场景）
	Mapping map[string]string
}

// HotMigration 热迁移执行器
type HotMigration struct {
	FromVersion  string
	ToVersion    string
	Strategy     HotMigrationStrategy
	Plan         *MigrationPlan
	FromWorkflow *DAGWorkflow
	ToWorkflow   *DAGWorkflow
	records      []NodeMigrationRecord
	mu           sync.Mutex
}

// buildMigrationPlan 根据新旧版本 DAG 计算迁移计划
func buildMigrationPlan(from, to *DAGWorkflow) *MigrationPlan {
	fromNodes := make(map[string]bool)
	toNodes := make(map[string]bool)

	from.mu.RLock()
	for id := range from.nodes {
		fromNodes[id] = true
	}
	from.mu.RUnlock()

	to.mu.RLock()
	for id := range to.nodes {
		toNodes[id] = true
	}
	to.mu.RUnlock()

	plan := &MigrationPlan{
		Mapping: make(map[string]string),
	}

	// 找出新增、保留、删除的节点
	for id := range toNodes {
		if fromNodes[id] {
			plan.KeepNodes = append(plan.KeepNodes, id)
			plan.Mapping[id] = id
		} else {
			plan.AddNodes = append(plan.AddNodes, id)
		}
	}

	for id := range fromNodes {
		if !toNodes[id] {
			plan.RemoveNodes = append(plan.RemoveNodes, id)
		}
	}

	// 检查保留节点的结构是否有变更
	for _, id := range plan.KeepNodes {
		fromNode := from.nodes[id]
		toNode := to.nodes[id]
		if nodesDiffer(fromNode, toNode) {
			plan.ModifyNodes = append(plan.ModifyNodes, id)
		}
	}

	sort.Strings(plan.KeepNodes)
	sort.Strings(plan.AddNodes)
	sort.Strings(plan.RemoveNodes)
	sort.Strings(plan.ModifyNodes)

	return plan
}

// nodesDiffer 比较两个节点是否结构不同
func nodesDiffer(a, b *DAGNode) bool {
	if a == nil || b == nil {
		return a != b
	}
	if a.Input != b.Input {
		return true
	}
	// 比较 Metadata
	if len(a.Metadata) != len(b.Metadata) {
		return true
	}
	for k, v := range a.Metadata {
		if bv, ok := b.Metadata[k]; !ok || bv != v {
			return true
		}
	}
	// 比较 RetryPolicy
	if (a.RetryPolicy == nil) != (b.RetryPolicy == nil) {
		return true
	}
	if a.RetryPolicy != nil && b.RetryPolicy != nil {
		if a.RetryPolicy.MaxRetries != b.RetryPolicy.MaxRetries ||
			a.RetryPolicy.Delay != b.RetryPolicy.Delay ||
			a.RetryPolicy.Backoff != b.RetryPolicy.Backoff {
			return true
		}
	}
	return false
}

// Execute 执行热迁移
func (hm *HotMigration) Execute(ctx context.Context) error {
	if hm.Plan == nil {
		return fmt.Errorf("dag: migration plan is nil")
	}
	if hm.FromWorkflow == nil || hm.ToWorkflow == nil {
		return fmt.Errorf("dag: migration workflows are nil")
	}

	switch hm.Strategy {
	case HotMigrationKeepRunning:
		return hm.executeKeepRunning(ctx)
	case HotMigrationRestartAll:
		return hm.executeRestartAll(ctx)
	case HotMigrationGradual:
		return hm.executeGradual(ctx)
	default:
		return fmt.Errorf("dag: unknown migration strategy %q", hm.Strategy)
	}
}

// executeKeepRunning 执行 keep_running 策略：
// - 已完成节点：保持结果不变
// - 运行中节点：继续执行旧版本定义
// - 未开始节点：使用新版本定义执行
func (hm *HotMigration) executeKeepRunning(ctx context.Context) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.records = make([]NodeMigrationRecord, 0)

	// 处理保留节点：状态保持（已完成节点不丢失结果）
	for _, id := range hm.Plan.KeepNodes {
		hm.records = append(hm.records, NodeMigrationRecord{
			NodeID:   id,
			OldState: NodeStateCompleted,
			NewState: NodeStateMigrated,
			Migrated: false,
		})
	}

	// 新增节点：待执行
	for _, id := range hm.Plan.AddNodes {
		hm.records = append(hm.records, NodeMigrationRecord{
			NodeID:   id,
			OldState: NodeStatePending,
			NewState: NodeStatePending,
			Migrated: true,
		})
	}

	// 删除节点：标记为已迁移（移除）
	for _, id := range hm.Plan.RemoveNodes {
		hm.records = append(hm.records, NodeMigrationRecord{
			NodeID:   id,
			OldState: NodeStateCompleted,
			NewState: NodeStateMigrated,
			Migrated: true,
		})
	}

	// 修改节点：使用新版本定义
	for _, id := range hm.Plan.ModifyNodes {
		hm.records = append(hm.records, NodeMigrationRecord{
			NodeID:   id,
			OldState: NodeStateCompleted,
			NewState: NodeStateMigrated,
			Migrated: true,
		})
	}

	return nil
}

// executeRestartAll 执行 restart_all 策略：全部节点使用新版本重新执行
func (hm *HotMigration) executeRestartAll(ctx context.Context) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.records = make([]NodeMigrationRecord, 0)

	// 所有保留节点标记为需要重新执行
	for _, id := range hm.Plan.KeepNodes {
		hm.records = append(hm.records, NodeMigrationRecord{
			NodeID:   id,
			OldState: NodeStateCompleted,
			NewState: NodeStatePending,
			Migrated: true,
		})
	}

	// 新增节点：待执行
	for _, id := range hm.Plan.AddNodes {
		hm.records = append(hm.records, NodeMigrationRecord{
			NodeID:   id,
			OldState: NodeStatePending,
			NewState: NodeStatePending,
			Migrated: true,
		})
	}

	// 删除节点：移除
	for _, id := range hm.Plan.RemoveNodes {
		hm.records = append(hm.records, NodeMigrationRecord{
			NodeID:   id,
			OldState: NodeStateCompleted,
			NewState: NodeStateMigrated,
			Migrated: true,
		})
	}

	return nil
}

// executeGradual 执行 gradual 策略：
// 第一批迁移新增节点
// 第二批迁移修改节点
// 第三批删除废弃节点
func (hm *HotMigration) executeGradual(ctx context.Context) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.records = make([]NodeMigrationRecord, 0)

	// 第一批：保留节点（不变更）
	for _, id := range hm.Plan.KeepNodes {
		hm.records = append(hm.records, NodeMigrationRecord{
			NodeID:   id,
			OldState: NodeStateCompleted,
			NewState: NodeStateMigrated,
			Migrated: false,
		})
	}

	// 第二批：新增节点立即启用
	for _, id := range hm.Plan.AddNodes {
		hm.records = append(hm.records, NodeMigrationRecord{
			NodeID:   id,
			OldState: NodeStatePending,
			NewState: NodeStateMigrated,
			Migrated: true,
		})
	}

	// 第三批：修改节点逐步切换
	for _, id := range hm.Plan.ModifyNodes {
		hm.records = append(hm.records, NodeMigrationRecord{
			NodeID:   id,
			OldState: NodeStateCompleted,
			NewState: NodeStateMigrated,
			Migrated: true,
		})
	}

	// 最后：删除废弃节点
	for _, id := range hm.Plan.RemoveNodes {
		hm.records = append(hm.records, NodeMigrationRecord{
			NodeID:   id,
			OldState: NodeStateCompleted,
			NewState: NodeStateMigrated,
			Migrated: true,
		})
	}

	return nil
}

// Records 返回迁移记录
func (hm *HotMigration) Records() []NodeMigrationRecord {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	result := make([]NodeMigrationRecord, len(hm.records))
	copy(result, hm.records)
	return result
}

// MigratedNodeIDs 返回已迁移的节点 ID 列表
func (hm *HotMigration) MigratedNodeIDs() []string {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	var ids []string
	for _, r := range hm.records {
		if r.Migrated {
			ids = append(ids, r.NodeID)
		}
	}
	sort.Strings(ids)
	return ids
}

// HasNodeChange 检查节点是否有结构变更
func (plan *MigrationPlan) HasNodeChange(nodeID string) bool {
	for _, id := range plan.ModifyNodes {
		if id == nodeID {
			return true
		}
	}
	return false
}

// IsAdded 检查节点是否为新增
func (plan *MigrationPlan) IsAdded(nodeID string) bool {
	for _, id := range plan.AddNodes {
		if id == nodeID {
			return true
		}
	}
	return false
}

// IsRemoved 检查节点是否被删除
func (plan *MigrationPlan) IsRemoved(nodeID string) bool {
	for _, id := range plan.RemoveNodes {
		if id == nodeID {
			return true
		}
	}
	return false
}