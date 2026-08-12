// Package dag 中的热迁移实现
//
// ============================================================
// ⚠️ EXPERIMENTAL PLACEHOLDER — 评估报告 Issue #9
// ============================================================
// 当前 HotMigration.Execute 仅生成 NodeMigrationRecord 列表，
// 不会真实把已运行节点切换到新版本——即"plan generator 而非
// 执行器"。在 v6.x 评估报告中被列为严重问题（运行时无一致性
// 保护，"SetActive" 之后旧 Run 不感知新版本）。
//
// 使用约束（v6.x 起强制）：
//  1. 所有 HotMigration.Execute 调用都返回 ErrHotMigrationNotImplemented
//     并附带一份 MigrationPlan 调用方可读取的"应该迁移什么"清单。
//  2. 需要真实热迁移能力的调用方请改用 VersionedWorkflow.SetActive
//     + 自行编排 in-flight Run 的 cancel/replay；本 API 不承诺
//     在 v6.x 之前完成执行器实现。
//  3. 代码保留仅为：(a) MigrationPlan 结构与节点比较逻辑；(b) 给
//     上层提供"列出应迁移节点"的可观测面。
//
// 迁移路径（v7.x 路线图）：将 Execute 拆分为
// (a) Plan() — 仅生成 MigrationPlan；(b) Apply(ctx) — 实际驱动
// Workflow.SetActive + 在飞 Run 的 cancel/replay。
// ============================================================

package dag

import (
	"context"
	"errors"
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

// ErrHotMigrationNotImplemented 标记 HotMigration.Execute 当前不会真正执行迁移。
//
// v6.x 修复（评估报告 Issue #9）：
//   - 旧实现：Execute 只生成 NodeMigrationRecord 列表后返回 nil，
//     调用方误以为迁移已完成，线上"SetActive" 之后旧 Run 仍在
//     旧版本上跑、新 Run 在新版本上跑，无一致性保证。
//   - 新实现：Execute 立刻返回 ErrHotMigrationNotImplemented，但仍
//     把 MigrationPlan 写入 hm.records 供调用方"列出应迁移节点"。
//
// 移除时机：v7.x 引入真实执行器后，可与 ErrHotMigrationNotImplemented 同步移除。
var ErrHotMigrationNotImplemented = errors.New("dag: HotMigration.Execute is an experimental placeholder; does not perform real migration. Use VersionedWorkflow.SetActive + manual cancel/replay instead")

// HotMigration 热迁移"计划生成器"。
//
// EXPERIMENTAL: 当前 Execute 仅生成 MigrationPlan 列表并返回
// ErrHotMigrationNotImplemented，不会真实切换在飞 Run 的执行版本。
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

// Execute 执行热迁移
//
// EXPERIMENTAL: 当前实现仅生成 MigrationPlan 列表（通过 execute* 方法
// 写入 hm.records）并返回 ErrHotMigrationNotImplemented。不会真实切换
// 在飞 Run 的执行版本。调用方应改用 VersionedWorkflow.SetActive +
// 自行编排 in-flight Run 的 cancel/replay。
//
// 返回的 records 列表可通过 Records() / MigratedNodeIDs() 读取，用于：
//   - 可观测：列出当前版本切换需要处理的节点
//   - 决策：辅助调用方决定是否要手工 cancel/replay
//
// v6.x 行为变更：旧实现返回 nil 但无任何效果，会让 SetActive 误以为
// 切换成功。新实现让"未实现"在错误路径上显式可见。
func (hm *HotMigration) Execute(ctx context.Context) error {
	if hm.Plan == nil {
		return fmt.Errorf("dag: migration plan is nil")
	}
	if hm.FromWorkflow == nil || hm.ToWorkflow == nil {
		return fmt.Errorf("dag: migration workflows are nil")
	}

	var execErr error
	switch hm.Strategy {
	case HotMigrationKeepRunning:
		execErr = hm.executeKeepRunning(ctx)
	case HotMigrationRestartAll:
		execErr = hm.executeRestartAll(ctx)
	case HotMigrationGradual:
		execErr = hm.executeGradual(ctx)
	default:
		return fmt.Errorf("dag: unknown migration strategy %q", hm.Strategy)
	}

	// execute* 仅生成 records；只要 plan 生成成功，叠加 EXPERIMENTAL 标记
	if execErr == nil {
		return fmt.Errorf("%w: plan=%+v", ErrHotMigrationNotImplemented, hm.PlanSummary())
	}
	return execErr
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
