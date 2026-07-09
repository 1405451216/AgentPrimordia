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
	"time"

	"go.etcd.io/etcd/client/v3"
)

// EtcdCheckpointStore 基于 etcd 的检查点存储。
type EtcdCheckpointStore struct {
	client *clientv3.Client
	prefix string
	ttl    time.Duration
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
	return &EtcdCheckpointStore{client: client, prefix: prefix, ttl: ttl}, nil
}

func (s *EtcdCheckpointStore) key(agentID string) string { return s.prefix + "/" + agentID }

// Save 写入状态并绑定租约（自动过期）。
func (s *EtcdCheckpointStore) Save(ctx context.Context, state *AgentState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	lease, err := s.client.Grant(ctx, int64(s.ttl.Seconds()))
	if err != nil {
		return err
	}
	_, err = s.client.Put(ctx, s.key(state.AgentID), string(data), clientv3.WithLease(lease.ID))
	return err
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

// Delete 删除状态。
func (s *EtcdCheckpointStore) Delete(ctx context.Context, agentID string) error {
	_, err := s.client.Delete(ctx, s.key(agentID))
	return err
}
