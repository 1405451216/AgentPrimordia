// coordinator.go — 分布式协调器抽象（G2-3 分布式检查点）
//
// 设计目标：让 Agent 状态（Checkpoint）可以从单机 SQLite 升级为
// "跨节点可恢复" 的分布式存储。协调器负责分布式锁与租约（lease），
// 保证同一时刻只有一个节点持有某个 agent 的写入权。
//
// 本文件仅依赖 Go 标准库，符合仓库 AGENTS.md §2 白名单约束。
// 生产级的 etcd/redis 后端见 etcd_checkpoint.go / redis_checkpoint.go
// （通过 build tag 提供，需扩展白名单方可启用）。
package persist

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrLockHeld 表示锁已被其他节点持有且未过期。
type ErrLockHeld struct {
	Key    string
	Holder string
}

func (e *ErrLockHeld) Error() string {
	return fmt.Sprintf("coordinator: lock %q held by %q", e.Key, e.Holder)
}

// Lease 表示一个已获取的分布式锁租约。
type Lease interface {
	// Key 返回被锁定的资源键（通常是 agentID）。
	Key() string
	// Holder 返回持锁节点 ID。
	Holder() string
	// ExpiresAt 返回租约过期时间。
	ExpiresAt() time.Time
	// Renew 续约（延长过期时间）。
	Renew(ctx context.Context) error
}

// Coordinator 分布式协调器接口。
// 实现需保证 Acquire 的原子性（同 key 同时只有一个 holder）。
type Coordinator interface {
	// Acquire 尝试获取 key 的锁。
	//   - 成功：返回 Lease。
	//   - 失败（被他人持有且未过期）：返回 *ErrLockHeld。
	Acquire(ctx context.Context, key string) (Lease, error)
	// Release 释放锁。
	Release(ctx context.Context, lease Lease) error
	// Owner 返回当前持锁节点 ID；无人持锁返回空字符串与 nil。
	Owner(ctx context.Context, key string) (string, error)
}

// memoryLease 进程内租约实现。
type memoryLease struct {
	key     string
	holder  string
	expires time.Time
	coord   *memoryCoordinator
	mu      sync.Mutex
}

func (l *memoryLease) Key() string          { return l.key }
func (l *memoryLease) Holder() string       { return l.holder }
func (l *memoryLease) ExpiresAt() time.Time { return l.expires }

func (l *memoryLease) Renew(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.expires = time.Now().Add(l.coord.ttl)
	return nil
}

// memoryCoordinator 进程内协调器（用于单测与单节点场景）。
// 多节点场景下应使用 fsCoordinator 或 etcd/redis 后端。
type memoryCoordinator struct {
	nodeID string
	ttl    time.Duration
	mu     sync.Mutex
	locks  map[string]*memoryLease
	now    func() time.Time
}

// NewMemoryCoordinator 创建进程内协调器。
//   - nodeID：本节点标识。
//   - ttl：租约有效期，0 表示默认 30s。
func NewMemoryCoordinator(nodeID string, ttl time.Duration) Coordinator {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &memoryCoordinator{
		nodeID: nodeID,
		ttl:    ttl,
		locks:  make(map[string]*memoryLease),
		now:    time.Now,
	}
}

func (c *memoryCoordinator) Acquire(ctx context.Context, key string) (Lease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	if existing, ok := c.locks[key]; ok {
		if existing.expires.After(now) {
			if existing.holder == c.nodeID {
				// 同节点重入：续约
				existing.expires = now.Add(c.ttl)
				return existing, nil
			}
			return nil, &ErrLockHeld{Key: key, Holder: existing.holder}
		}
		// 已过期，回收
		delete(c.locks, key)
	}
	lease := &memoryLease{
		key:     key,
		holder:  c.nodeID,
		expires: now.Add(c.ttl),
		coord:   c,
	}
	c.locks[key] = lease
	return lease, nil
}

func (c *memoryCoordinator) Release(ctx context.Context, lease Lease) error {
	ml, ok := lease.(*memoryLease)
	if !ok {
		return errors.New("coordinator: invalid lease type")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cur, ok := c.locks[ml.key]; ok && cur == ml {
		delete(c.locks, ml.key)
	}
	return nil
}

func (c *memoryCoordinator) Owner(ctx context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if l, ok := c.locks[key]; ok && l.expires.After(c.now()) {
		return l.holder, nil
	}
	return "", nil
}
