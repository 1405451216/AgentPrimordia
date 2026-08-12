//go:build etcd

// etcd_checkpoint.go — 基于 etcd 的分布式检查点后端（G2-3 生产实现）
//
// 本文件通过 build tag `etcd` 启用，依赖 go.etcd.io/etcd/client/v3。
// 该依赖已在 AGENTS.md §2.1 白名单中获批（G2-3 生产实现）。
//
// 设计要点：
//   - 使用 etcd 租约（Lease）实现自动过期的分布式锁与状态 TTL。
//   - key 结构：prefix + "/" + agentID。
//   - 跨节点恢复：etcd 自身的强一致性与 Watch 机制保证接管安全。
package persist

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.etcd.io/etcd/client/v3"
)

// EtcdCheckpointStore 基于 etcd 的检查点存储。
type EtcdCheckpointStore struct {
	client *clientv3.Client
	prefix string
	ttl    time.Duration

	// v6.x（评估报告 Issue #6）：每个 agentID 复用同一个 lease，
	// 避免每次 Save 都 Grant 新租约导致旧租约堆积到 TTL 才过期。
	leaseMu  sync.Mutex
	leases   map[string]clientv3.LeaseID
}

// NewEtcdCheckpointStore 创建 etcd 后端。
func NewEtcdCheckpointStore(endpoints []string, prefix string, ttl time.Duration) (*EtcdCheckpointStore, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &EtcdCheckpointStore{client: client, prefix: prefix, ttl: ttl, leases: make(map[string]clientv3.LeaseID)}, nil
}

func (s *EtcdCheckpointStore) key(agentID string) string { return s.prefix + "/" + agentID }

// acquireLease 获取（或复用并续约）agentID 的租约。
//
// v6.x：只对同一 agentID 保留一个 lease；每次 Save 调用 KeepAliveOnce
// 续约，替代旧实现"每次 Grant 新 lease"导致同一 agentID 有 N 个
// 同时存活的旧 lease（状态生命周期与 lease 不一致，TTL 形同虚设）。
func (s *EtcdCheckpointStore) acquireLease(ctx context.Context, agentID string) (clientv3.LeaseID, error) {
	s.leaseMu.Lock()
	defer s.leaseMu.Unlock()

	if id, ok := s.leases[agentID]; ok {
		// 复用已有租约并续约
		_, err := s.client.KeepAliveOnce(ctx, id)
		if err == nil {
			return id, nil
		}
		// 租约可能已过期，重新 Grant
		delete(s.leases, agentID)
	}

	lease, err := s.client.Grant(ctx, int64(s.ttl.Seconds()))
	if err != nil {
		return 0, fmt.Errorf("etcd grant lease: %w", err)
	}
	s.leases[agentID] = lease.ID
	return lease.ID, nil
}

// Save 写入状态并绑定（复用的）租约。
func (s *EtcdCheckpointStore) Save(ctx context.Context, state *AgentState) error {
	if state == nil || state.AgentID == "" {
		return fmt.Errorf("etcd checkpoint: nil state or empty agentID")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	leaseID, err := s.acquireLease(ctx, state.AgentID)
	if err != nil {
		return err
	}
	_, err = s.client.Put(ctx, s.key(state.AgentID), string(data), clientv3.WithLease(leaseID))
	if err != nil {
		return err
	}
	return nil
}

// Load 读取状态。
func (s *EtcdCheckpointStore) Load(ctx context.Context, agentID string) (*AgentState, error) {
	resp, err := s.client.Get(ctx, s.key(agentID))
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		return nil, ErrCheckpointNotFound
	}
	var state AgentState
	if err := json.Unmarshal(resp.Kvs[0].Value, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// List 按会话前缀列出。
func (s *EtcdCheckpointStore) List(ctx context.Context, sessionID string) ([]*AgentState, error) {
	resp, err := s.client.Get(ctx, s.prefix+"/", clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	var out []*AgentState
	for _, kv := range resp.Kvs {
		var st AgentState
		if err := json.Unmarshal(kv.Value, &st); err != nil {
			continue
		}
		if st.SessionID == sessionID {
			out = append(out, &st)
		}
	}
	return out, nil
}

// Delete 删除状态并撤销对应租约（释放资源）。
func (s *EtcdCheckpointStore) Delete(ctx context.Context, agentID string) error {
	_, err := s.client.Delete(ctx, s.key(agentID))
	if err != nil {
		return err
	}
	// v6.x：删除状态后主动撤销该 agentID 的租约，避免 lease 残留到 TTL。
	s.leaseMu.Lock()
	if id, ok := s.leases[agentID]; ok {
		delete(s.leases, agentID)
		_, _ = s.client.Revoke(ctx, id)
	}
	s.leaseMu.Unlock()
	return nil
}

// Close 关闭 etcd 客户端并撤销所有持有的租约。
func (s *EtcdCheckpointStore) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.leaseMu.Lock()
	for agentID, id := range s.leases {
		_, _ = s.client.Revoke(ctx, id)
		delete(s.leases, agentID)
	}
	s.leaseMu.Unlock()

	return s.client.Close()
}
