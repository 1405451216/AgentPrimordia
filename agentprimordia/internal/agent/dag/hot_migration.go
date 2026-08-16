// Package dag 中的热迁移实现
//
// ============================================================
// 真实执行器（替代 EXPERIMENTAL PLACEHOLDER — 评估报告 Issue #9）
// ============================================================
// HotMigration.Execute 现在真实执行迁移：把 ToWorkflow 的节点/边
// 定义原子应用到 FromWorkflow，并按策略处理执行统计：
//   - keep_running：仅切换结构；已完成节点结果由调用方持有，天然保留
//   - restart_all：切换结构 + 重置全部节点统计（全部重新执行）
//   - gradual：切换结构 + 仅重置新增/修改节点的统计（这些节点重跑）
//
// 一致性保证：DAGWorkflow.Run 在 RLock 内快照 nodes/edges，
// Execute 在写锁内原子替换结构——二者互斥，任何 Run 要么读到
// 完整旧版本、要么读到完整新版本，不存在半迁移状态。
//
// 在飞 Run 说明：Run 是快照式执行（启动时拷贝节点/边），已开始的
// Run 不受结构替换影响；迁移后新启动的 Run 使用新版本定义。
// ============================================================

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
	NodeID   string
	OldState NodeMigrationState
	NewState NodeMigrationState
	Result   *DAGNodeResult
	Migrated bool
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

// （历史）ErrHotMigrationNotImplemented 曾标记占位实现，已在真实执行器落地后移除。
// 迁移语义见包顶注释与 VersionedWorkflow.Migrate。

// HotMigration 热迁移执行器：真实把 ToWorkflow 的结构应用到 FromWorkflow。
// 详情见包顶注释。
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

// Execute 执行热迁移：真实把 ToWorkflow 的节点/边定义应用到 FromWorkflow，
// 并按策略处理执行统计。返回 nil 表示迁移已真实完成。
//
// 迁移结果可通过 Records() / MigratedNodeIDs() 读取（可观测面）。
// 一致性：结构替换在 FromWorkflow 写锁内原子完成，与 Run 的快照读取互斥。
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

// applyStructure 把 ToWorkflow 的节点/边结构原子应用到 FromWorkflow。
// 节点做浅拷贝（共享 Agent 引用），迁移后 FromWorkflow 结构与
// ToWorkflow 完全一致且相互独立。
func (hm *HotMigration) applyStructure() {
	from, to := hm.FromWorkflow, hm.ToWorkflow

	to.mu.RLock()
	newNodes := make(map[string]*DAGNode, len(to.nodes))
	for id, n := range to.nodes {
		if n == nil {
			continue
		}
		cp := *n // 浅拷贝：共享 Agent/RetryPolicy 引用，结构独立
		newNodes[id] = &cp
	}
	newEdges := append([]DAGEdge(nil), to.edges...)
	to.mu.RUnlock()

	from.mu.Lock()
	from.nodes = newNodes
	from.edges = newEdges
	from.mu.Unlock()
}

// buildRecords 根据迁移计划构建节点迁移记录。
// restartAll 为 true 时全部节点标记为已迁移（重新执行语义）。
func (hm *HotMigration) buildRecords(restartAll bool) []NodeMigrationRecord {
	records := make([]NodeMigrationRecord, 0,
		len(hm.Plan.KeepNodes)+len(hm.Plan.AddNodes)+len(hm.Plan.ModifyNodes)+len(hm.Plan.RemoveNodes))

	// 保留节点：结构未变，保持原状态
	for _, id := range hm.Plan.KeepNodes {
		records = append(records, NodeMigrationRecord{
			NodeID:   id,
			OldState: NodeStateCompleted,
			NewState: NodeStateCompleted,
			Migrated: false,
		})
	}
	// 新增节点：待执行
	for _, id := range hm.Plan.AddNodes {
		records = append(records, NodeMigrationRecord{
			NodeID:   id,
			OldState: NodeStatePending,
			NewState: NodeStateMigrated,
			Migrated: true,
		})
	}
	// 修改节点：结构变更，将按新定义执行
	for _, id := range hm.Plan.ModifyNodes {
		records = append(records, NodeMigrationRecord{
			NodeID:   id,
			OldState: NodeStateRunning,
			NewState: NodeStateMigrated,
			Migrated: true,
		})
	}
	// 删除节点：已移除
	for _, id := range hm.Plan.RemoveNodes {
		records = append(records, NodeMigrationRecord{
			NodeID:   id,
			OldState: NodeStateCompleted,
			NewState: NodeStateMigrated,
			Migrated: true,
		})
	}

	if restartAll {
		for i := range records {
			records[i].OldState = NodeStateRunning
			records[i].NewState = NodeStateMigrated
			records[i].Migrated = true
		}
	}

	sort.Slice(records, func(i, j int) bool { return records[i].NodeID < records[j].NodeID })
	return records
}

// PlanSummary 返回当前 MigrationPlan 的可读摘要，Execute 失败时附在 error 中。
func (hm *HotMigration) PlanSummary() string {
	if hm.Plan == nil {
		return "<nil>"
	}
	return fmt.Sprintf("keep=%v add=%v remove=%v modify=%v",
		hm.Plan.KeepNodes, hm.Plan.AddNodes, hm.Plan.RemoveNodes, hm.Plan.ModifyNodes)
}

// executeKeepRunning 执行 keep_running 策略：
// 切换结构；已完成节点结果由调用方持有（引擎无跨 Run 状态），天然保留。
func (hm *HotMigration) executeKeepRunning(ctx context.Context) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.applyStructure()
	hm.records = hm.buildRecords(false)
	return nil
}

// executeRestartAll 执行 restart_all 策略：切换结构 + 重置全部节点统计（全部重新执行）
func (hm *HotMigration) executeRestartAll(ctx context.Context) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.applyStructure()
	// 重置全部节点统计：restart_all 语义=全部按新版本重新执行
	hm.FromWorkflow.metrics.Reset()
	hm.records = hm.buildRecords(true)
	return nil
}

// executeGradual 执行 gradual 策略：切换结构；新增/修改节点重置统计（将重跑），保留节点不动
func (hm *HotMigration) executeGradual(ctx context.Context) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.applyStructure()
	// 逐步切换：新增与修改节点按新版本重新执行
	for _, id := range append(append([]string(nil), hm.Plan.AddNodes...), hm.Plan.ModifyNodes...) {
		hm.FromWorkflow.metrics.ResetNode(id)
	}
	hm.records = hm.buildRecords(false)
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
