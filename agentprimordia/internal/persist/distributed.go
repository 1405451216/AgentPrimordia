// distributed.go — 分布式检查点存储（G2-3）
//
// 将单机 CheckpointStore 升级为支持跨节点恢复：
//   - Save 获取并保持分布式锁（同节点重入自动续约），表明本节点正在活跃持有该 agent。
//   - Load / List 不持锁，允许任意节点读取以做恢复决策。
//   - Owner 返回当前持锁节点；ForceTakeover 在租约过期后强制接管，实现"故障节点恢复"。
//   - Release 主动释放本节点持有的锁（Agent 结束或让出时调用）。
//
// 仅依赖标准库，符合 AGENTS.md 白名单。etcd/redis 生产后端见
// etcd_checkpoint.go / redis_checkpoint.go（build tag 提供）。
package persist

import (
	"context"
	"fmt"
	"sync"
)

// DistributedCheckpointStore 分布式检查点存储。
// 组合一个基础 CheckpointStore（如 SQLite）与一个 Coordinator。
type DistributedCheckpointStore struct {
	store  CheckpointStore
	coord  Coordinator
	nodeID string

	mu   sync.Mutex
	held map[string]Lease // agentID -> 本节点持有的租约
}

// NewDistributedCheckpointStore 创建分布式检查点存储。
func NewDistributedCheckpointStore(store CheckpointStore, coord Coordinator, nodeID string) *DistributedCheckpointStore {
	return &DistributedCheckpointStore{
		store:  store,
		coord:  coord,
		nodeID: nodeID,
		held:   make(map[string]Lease),
	}
}

// acquire 获取（或续约）某 agent 的写锁，记录到 held。
// 若被其他节点持有且未过期，返回 *ErrLockHeld。
func (d *DistributedCheckpointStore) acquire(ctx context.Context, agentID string) (Lease, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if l, ok := d.held[agentID]; ok {
		if err := l.Renew(ctx); err == nil {
			return l, nil
		}
		// 续约失败（锁已丢失），丢弃后重新获取
		delete(d.held, agentID)
	}
	l, err := d.coord.Acquire(ctx, agentID)
	if err != nil {
		return nil, err
	}
	d.held[agentID] = l
	return l, nil
}

// Save 获取并保持写锁后写入状态。
// 若锁被其他节点持有且未过期，返回 *ErrLockHeld。
func (d *DistributedCheckpointStore) Save(ctx context.Context, state *AgentState) error {
	if _, err := d.acquire(ctx, state.AgentID); err != nil {
		return fmt.Errorf("distributed checkpoint save: %w", err)
	}
	return d.store.Save(ctx, state)
}

// Load 读取状态（不持锁，允许任意节点读取以做恢复决策）。
func (d *DistributedCheckpointStore) Load(ctx context.Context, agentID string) (*AgentState, error) {
	return d.store.Load(ctx, agentID)
}

// List 列出某会话的所有检查点。
func (d *DistributedCheckpointStore) List(ctx context.Context, sessionID string) ([]*AgentState, error) {
	return d.store.List(ctx, sessionID)
}

// Release 释放本节点持有的某 agent 写锁（Agent 结束或主动让出时调用）。
func (d *DistributedCheckpointStore) Release(ctx context.Context, agentID string) error {
	d.mu.Lock()
	l, ok := d.held[agentID]
	delete(d.held, agentID)
	d.mu.Unlock()
	if !ok {
		// 未持有则尝试直接向协调器释放（兼容外部持有的同节点租约）
		if owner, _ := d.coord.Owner(ctx, agentID); owner == d.nodeID {
			if l2, err := d.coord.Acquire(ctx, agentID); err == nil {
				return d.coord.Release(ctx, l2)
			}
		}
		return nil
	}
	return d.coord.Release(ctx, l)
}

// Delete 删除检查点并释放本节点持有的锁。
func (d *DistributedCheckpointStore) Delete(ctx context.Context, agentID string) error {
	if _, err := d.acquire(ctx, agentID); err != nil {
		// 锁已被他人持有：仍尝试删除（读取侧允许），但返回锁错误
		if delErr := d.store.Delete(ctx, agentID); delErr != nil {
			return delErr
		}
		return fmt.Errorf("distributed checkpoint delete: %w", err)
	}
	if err := d.store.Delete(ctx, agentID); err != nil {
		return err
	}
	return d.Release(ctx, agentID)
}

// Owner 返回当前持有某 agent 写入锁的节点（空表示无主）。
func (d *DistributedCheckpointStore) Owner(ctx context.Context, agentID string) (string, error) {
	return d.coord.Owner(ctx, agentID)
}

// ForceTakeover 强制接管一个 agent 的写入权。
// 仅当租约已过期（无人持有或持有者过期）时成功；否则返回 *ErrLockHeld。
// 这是"故障节点恢复"的核心：新节点顶替失联的旧节点继续写入。
func (d *DistributedCheckpointStore) ForceTakeover(ctx context.Context, agentID string) error {
	// 尝试获取锁：若被他人持有且未过期则失败
	lease, err := d.coord.Acquire(ctx, agentID)
	if err != nil {
		return fmt.Errorf("distributed checkpoint takeover: %w", err)
	}
	// 获取成功说明无有效持锁者，立即释放（真正写入时再按需获取）
	_ = d.coord.Release(ctx, lease)
	return nil
}
