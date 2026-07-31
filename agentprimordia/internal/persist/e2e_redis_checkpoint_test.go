//go:build redis

// e2e_redis_checkpoint_test.go — 分布式 Checkpoint E2E 验证（Redis 后端）
//
// 运行方式：
//
//	# redis 后端测试
//	go test -tags=redis -run TestE2E_RedisCheckpoint -v ./internal/persist/
//
// 环境要求：
//   - redis: docker run -d -p 6379:6379 redis:7-alpine
//   - 可通过 REDIS_ADDR 环境变量自定义地址。
package persist

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// ===== TestE2E_RedisCheckpoint_SaveLoadListDelete =====

// TestE2E_RedisCheckpoint_SaveLoadListDelete 验证 Redis 后端的完整 CRUD 操作。
func TestE2E_RedisCheckpoint_SaveLoadListDelete(t *testing.T) {
	requireRedis(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := NewRedisCheckpointStore(&redis.Options{
		Addr: getRedisAddr(),
	}, "e2e-test", 60*time.Second)
	defer store.client.Close()

	agentID := "e2e-redis-crud-1"
	sessionID := "e2e-redis-session-1"

	// 清理残留
	_ = store.Delete(ctx, agentID)

	t.Run("Save", func(t *testing.T) {
		state := &AgentState{
			AgentID:   agentID,
			SessionID: sessionID,
			Status:    "active",
			Messages: []CheckpointMessage{
				{Role: "user", Content: "hello redis"},
			},
			TurnCount: 3,
			SavedAt:   time.Now(),
		}
		if err := store.Save(ctx, state); err != nil {
			t.Fatalf("Save 失败: %v", err)
		}
		t.Log("Save 成功")
	})

	t.Run("Load", func(t *testing.T) {
		loaded, err := store.Load(ctx, agentID)
		if err != nil {
			t.Fatalf("Load 失败: %v", err)
		}
		if loaded.AgentID != agentID {
			t.Errorf("AgentID = %q, want %q", loaded.AgentID, agentID)
		}
		if loaded.TurnCount != 3 {
			t.Errorf("TurnCount = %d, want 3", loaded.TurnCount)
		}
		t.Logf("Load 成功: status=%s", loaded.Status)
	})

	t.Run("List", func(t *testing.T) {
		list, err := store.List(ctx, sessionID)
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if len(list) < 1 {
			t.Errorf("List 数量 = %d, want >= 1", len(list))
		}
		t.Logf("List 成功: count=%d", len(list))
	})

	t.Run("Delete", func(t *testing.T) {
		if err := store.Delete(ctx, agentID); err != nil {
			t.Fatalf("Delete 失败: %v", err)
		}
		_, err := store.Load(ctx, agentID)
		if err == nil {
			t.Error("删除后 Load 应返回错误")
		}
		t.Log("Delete 成功")
	})
}
