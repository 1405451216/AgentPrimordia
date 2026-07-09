//go:build redis

// redis_checkpoint.go — 基于 Redis 的分布式检查点后端（G2-3 生产实现）
//
// 本文件通过 build tag `redis` 启用，依赖 github.com/redis/go-redis/v9。
// 该依赖已在 AGENTS.md §2.1 白名单中获批（G2-3 生产实现）。
//
// 设计要点：
//   - 使用 Redis SET key value NX EX 实现分布式锁（原子加锁 + TTL）。
//   - 状态以 JSON 存储，key 结构：prefix:agent:{agentID}。
//   - 跨节点恢复：依赖 Redis 单点一致性；多副本部署时建议开启 WAIT。
package persist

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCheckpointStore 基于 Redis 的检查点存储。
type RedisCheckpointStore struct {
	client *redis.Client
	prefix string
	ttl    time.Duration
}

// NewRedisCheckpointStore 创建 Redis 后端。
func NewRedisCheckpointStore(opts *redis.Options, prefix string, ttl time.Duration) *RedisCheckpointStore {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &RedisCheckpointStore{client: redis.NewClient(opts), prefix: prefix, ttl: ttl}
}

func (s *RedisCheckpointStore) key(agentID string) string     { return s.prefix + ":agent:" + agentID }
func (s *RedisCheckpointStore) lockKey(agentID string) string { return s.prefix + ":lock:" + agentID }

// Save 写入状态（先抢锁，失败返回 ErrLockHeld 的等价错误）。
func (s *RedisCheckpointStore) Save(ctx context.Context, state *AgentState) error {
	ok, err := s.client.SetNX(ctx, s.lockKey(state.AgentID), "self", s.ttl).Result()
	if err != nil {
		return err
	}
	if !ok {
		return &ErrLockHeld{Key: state.AgentID, Holder: "other"}
	}
	defer s.client.Del(ctx, s.lockKey(state.AgentID))
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.key(state.AgentID), data, s.ttl).Err()
}

// Load 读取状态。
func (s *RedisCheckpointStore) Load(ctx context.Context, agentID string) (*AgentState, error) {
	data, err := s.client.Get(ctx, s.key(agentID)).Bytes()
	if err == redis.Nil {
		return nil, ErrCheckpointNotFound
	}
	if err != nil {
		return nil, err
	}
	var state AgentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// List 列出所有 prefix:agent:* 键。
func (s *RedisCheckpointStore) List(ctx context.Context, sessionID string) ([]*AgentState, error) {
	keys, err := s.client.Keys(ctx, s.prefix+":agent:*").Result()
	if err != nil {
		return nil, err
	}
	var out []*AgentState
	for _, k := range keys {
		data, err := s.client.Get(ctx, k).Bytes()
		if err != nil {
			continue
		}
		var st AgentState
		if err := json.Unmarshal(data, &st); err != nil {
			continue
		}
		if st.SessionID == sessionID {
			out = append(out, &st)
		}
	}
	return out, nil
}

// Delete 删除状态与锁。
func (s *RedisCheckpointStore) Delete(ctx context.Context, agentID string) error {
	s.client.Del(ctx, s.lockKey(agentID))
	return s.client.Del(ctx, s.key(agentID)).Err()
}
