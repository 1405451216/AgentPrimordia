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
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// lockSeq 全局单调递增的锁序号，保证同纳秒内多次 Save 的 token 不重复。
var lockSeq int64

func nextLockSeq() int64 { return atomic.AddInt64(&lockSeq, 1) }

// redisLockHolderPrefix 是锁 value 的前缀，配合 Redis Lua 脚本做
// "只有持有者能释放"的原子比较（防止误删他人锁）。
const redisLockHolderPrefix = "holder:"

// redisReleaseLockScript 原子地"仅当 lock 的 value 与 token 匹配才删除"。
//
// v6.x（评估报告 Issue #7）：旧实现 Save 里 `defer Del` 无条件删除锁，
// 一旦其他节点在两次操作之间接管，会把新持有者的锁删掉。此脚本
// 通过比较 token 保证删除安全（compare-and-delete）。
var redisReleaseLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
`)

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

// lockToken 为一次 Save 生成唯一持有者 token（进程内自增 + 时间戳），
// 保证"锁生命周期与状态生命周期一致"——只有本次 Save 的持有者能释放锁。
func lockToken() string {
	return redisLockHolderPrefix + fmt.Sprintf("%d-%d", time.Now().UnixNano(), nextLockSeq())
}

// Save 写入状态：
//   - 先抢锁（SET NX EX），失败返回 ErrLockHeld；
//   - 持有锁期间写入状态；
//   - 通过原子 compare-and-delete 释放锁（只有自己持有才能释放）。
//
// v6.x 修复（评估报告 Issue #7）：
//   - 锁 token 不再固定为 "self"，而是每次 Save 唯一；释放时 Lua 比较，
//     避免误删接管节点的锁。
//   - 锁 value 携带 holder 信息，错误信息可定位持有者。
func (s *RedisCheckpointStore) Save(ctx context.Context, state *AgentState) error {
	if state == nil || state.AgentID == "" {
		return fmt.Errorf("redis checkpoint: nil state or empty agentID")
	}
	token := lockToken()
	ok, err := s.client.SetNX(ctx, s.lockKey(state.AgentID), token, s.ttl).Result()
	if err != nil {
		return err
	}
	if !ok {
		// 读取锁当前持有者信息（尽力而为）
		holder, _ := s.client.Get(ctx, s.lockKey(state.AgentID)).Result()
		return &ErrLockHeld{Key: state.AgentID, Holder: holder}
	}
	// 仅当锁仍归本 token 所有时才释放（原子 compare-and-delete）
	defer s.releaseLock(ctx, state.AgentID, token)

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	// 状态 TTL 与锁 TTL 一致，保证状态生命周期绑定租约
	return s.client.Set(ctx, s.key(state.AgentID), data, s.ttl).Err()
}

// releaseLock 原子释放本节点持有的锁（Lua compare-and-delete）。
func (s *RedisCheckpointStore) releaseLock(ctx context.Context, agentID, token string) {
	_ = redisReleaseLockScript.Run(ctx, s.client, []string{s.lockKey(agentID)}, token).Err()
}

// RenewLock 续约锁与状态 TTL。
//
// v6.x（评估报告 Issue #7）：旧实现无续约路径，长任务超过 ttl 后锁/状态
// 被 Redis 自动过期，另一节点可在任务仍运行时接管。调用方应周期性调用。
func (s *RedisCheckpointStore) RenewLock(ctx context.Context, agentID string) error {
	pipe := s.client.TxPipeline()
	pipe.Expire(ctx, s.lockKey(agentID), s.ttl)
	pipe.Expire(ctx, s.key(agentID), s.ttl)
	_, err := pipe.Exec(ctx)
	if err != nil && errors.Is(err, redis.Nil) {
		return nil // 键不存在视为无状态，不算错误
	}
	return err
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
