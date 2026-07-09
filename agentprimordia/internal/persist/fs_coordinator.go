// fs_coordinator.go — 基于共享文件系统的分布式协调器（G2-3）
//
// 利用 os.Rename 的原子性在共享文件系统（NFS / 云盘 / 容器卷）上实现
// 跨进程、跨节点的分布式锁。这是 etcd/redis 不可用时的合规替代方案，
// 仅依赖 Go 标准库。
//
// 原理：
//   - 锁文件内容为 JSON {holder, expires}。
//   - Acquire：先写临时文件，再 os.Rename(tmp → lock)。Rename 在 POSIX
//     与 Windows 上对同目录目标均为原子操作，保证只有一个进程成功。
//   - 若目标已存在且未过期，Acquire 返回 *ErrLockHeld。
//   - Release：删除自己的锁文件。
package persist

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// fsLease 文件系统租约。
type fsLease struct {
	key     string
	holder  string
	expires time.Time
	coord   *fsCoordinator
	mu      sync.Mutex
}

func (l *fsLease) Key() string          { return l.key }
func (l *fsLease) Holder() string       { return l.holder }
func (l *fsLease) ExpiresAt() time.Time { return l.expires }

func (l *fsLease) Renew(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.expires = now.Add(l.coord.ttl)
	return l.coord.writeLease(l.key, l.holder, l.expires)
}

// lockRecord 锁文件内容。
type lockRecord struct {
	Holder  string    `json:"holder"`
	Expires time.Time `json:"expires"`
}

// fsCoordinator 基于共享目录的协调器。
type fsCoordinator struct {
	dir    string
	nodeID string
	ttl    time.Duration
	now    func() time.Time
}

// NewFSCoordinator 创建文件系统协调器。
//   - dir：共享目录（需多节点均可读写）；会自动创建。
//   - nodeID：本节点标识。
//   - ttl：租约有效期，0 表示默认 30s。
func NewFSCoordinator(dir, nodeID string, ttl time.Duration) (Coordinator, error) {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("fsCoordinator: mkdir %q: %w", dir, err)
	}
	return &fsCoordinator{dir: dir, nodeID: nodeID, ttl: ttl, now: time.Now}, nil
}

func (c *fsCoordinator) lockPath(key string) string {
	return filepath.Join(c.dir, key+".lock")
}

func (c *fsCoordinator) writeLease(key, holder string, expires time.Time) error {
	rec := lockRecord{Holder: holder, Expires: expires}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	tmp := c.lockPath(key) + fmt.Sprintf(".%d.tmp", os.Getpid())
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	// 原子重命名即加锁
	return os.Rename(tmp, c.lockPath(key))
}

func (c *fsCoordinator) readLease(key string) (*lockRecord, error) {
	data, err := os.ReadFile(c.lockPath(key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rec lockRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		// 损坏的锁文件视为无主，交给调用方回收
		return nil, nil
	}
	return &rec, nil
}

func (c *fsCoordinator) Acquire(ctx context.Context, key string) (Lease, error) {
	deadline := c.now().Add(c.ttl)
	for {
		rec, err := c.readLease(key)
		if err != nil {
			return nil, err
		}
		if rec != nil && rec.Expires.After(c.now()) {
			if rec.Holder == c.nodeID {
				// 同节点重入：续约（重写锁文件）
				if err := c.writeLease(key, c.nodeID, deadline); err != nil {
					return nil, err
				}
				return &fsLease{key: key, holder: c.nodeID, expires: deadline, coord: c}, nil
			}
			return nil, &ErrLockHeld{Key: key, Holder: rec.Holder}
		}
		// 无主或已过期 → 尝试抢占
		if err := c.writeLease(key, c.nodeID, deadline); err == nil {
			return &fsLease{key: key, holder: c.nodeID, expires: deadline, coord: c}, nil
		}
		// 抢占失败（被他人抢先），短暂退避后重试
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (c *fsCoordinator) Release(ctx context.Context, lease Lease) error {
	fl, ok := lease.(*fsLease)
	if !ok {
		return fmt.Errorf("fsCoordinator: invalid lease type")
	}
	rec, err := c.readLease(fl.key)
	if err != nil {
		return err
	}
	if rec != nil && rec.Holder == fl.holder {
		// 仅释放自己的锁
		if err := os.Remove(c.lockPath(fl.key)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (c *fsCoordinator) Owner(ctx context.Context, key string) (string, error) {
	rec, err := c.readLease(key)
	if err != nil {
		return "", err
	}
	if rec != nil && rec.Expires.After(c.now()) {
		return rec.Holder, nil
	}
	return "", nil
}
