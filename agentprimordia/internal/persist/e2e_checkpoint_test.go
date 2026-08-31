//go:build etcd

// e2e_checkpoint_test.go — 分布式 Checkpoint E2E 验证（etcd 后端）
//
// 运行方式：
//
//	# etcd 后端测试
//	go test -tags=etcd -run TestE2E_EtcdCheckpoint -v ./internal/persist/
//
// 环境要求：
//   - etcd: docker run -d -p 2379:2379 quay.io/coreos/etcd:v3.5.12
//   - 可通过 ETCD_ADDR 环境变量自定义地址。
package persist

import (
	"context"
	"testing"
	"time"
)

// ===== TestE2E_EtcdCheckpoint_SaveLoadListDelete =====

// TestE2E_EtcdCheckpoint_SaveLoadListDelete 验证 etcd 后端的完整 CRUD 操作。
func TestE2E_EtcdCheckpoint_SaveLoadListDelete(t *testing.T) {
	requireEtcd(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := NewEtcdCheckpointStore([]string{getEtcdAddr()}, "e2e-test", 60*time.Second)
	if err != nil {
		t.Fatalf("创建 etcd 存储失败: %v", err)
	}
	defer store.client.Close()

	agentID := "e2e-agent-crud-1"
	sessionID := "e2e-session-1"

	// 清理残留
	_ = store.Delete(ctx, agentID)

	t.Run("Save", func(t *testing.T) {
		state := &AgentState{
			AgentID:   agentID,
			SessionID: sessionID,
			Status:    "active",
			Messages: []CheckpointMessage{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "hi there"},
			},
			TurnCount: 5,
			Metrics: CheckpointMetrics{
				TotalTurns:  5,
				TotalTools:  3,
				Duration:    "10s",
				LLMLatency:  "200",
				ToolLatency: "50",
			},
			SavedAt: time.Now(),
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
		if loaded.SessionID != sessionID {
			t.Errorf("SessionID = %q, want %q", loaded.SessionID, sessionID)
		}
		if loaded.TurnCount != 5 {
			t.Errorf("TurnCount = %d, want 5", loaded.TurnCount)
		}
		if len(loaded.Messages) != 2 {
			t.Errorf("Messages 数量 = %d, want 2", len(loaded.Messages))
		}
		t.Logf("Load 成功: status=%s, turns=%d", loaded.Status, loaded.TurnCount)
	})

	t.Run("Load_NotFound", func(t *testing.T) {
		_, err := store.Load(ctx, "non-existent-agent")
		if err == nil {
			t.Error("期望 ErrCheckpointNotFound，但得到 nil")
		}
	})

	t.Run("List", func(t *testing.T) {
		// 创建第二个 agent
		state2 := &AgentState{
			AgentID:   "e2e-agent-crud-2",
			SessionID: sessionID,
			Status:    "idle",
			SavedAt:   time.Now(),
		}
		if err := store.Save(ctx, state2); err != nil {
			t.Fatalf("Save agent-2 失败: %v", err)
		}
		defer store.Delete(ctx, "e2e-agent-crud-2")

		list, err := store.List(ctx, sessionID)
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if len(list) < 2 {
			t.Errorf("List 数量 = %d, want >= 2", len(list))
		}
		t.Logf("List 成功: count=%d", len(list))
	})

	t.Run("Delete", func(t *testing.T) {
		if err := store.Delete(ctx, agentID); err != nil {
			t.Fatalf("Delete 失败: %v", err)
		}

		// 验证已删除
		_, err := store.Load(ctx, agentID)
		if err == nil {
			t.Error("删除后 Load 应返回错误")
		}
		t.Log("Delete 成功")
	})
}

// ===== TestE2E_EtcdCheckpoint_LeaseExpiry =====

// TestE2E_EtcdCheckpoint_LeaseExpiry 验证 etcd 租约过期后数据自动清理。
func TestE2E_EtcdCheckpoint_LeaseExpiry(t *testing.T) {
	requireEtcd(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 使用短 TTL 便于快速验证
	shortTTL := 3 * time.Second
	store, err := NewEtcdCheckpointStore([]string{getEtcdAddr()}, "e2e-lease", shortTTL)
	if err != nil {
		t.Fatalf("创建 etcd 存储失败: %v", err)
	}
	defer store.client.Close()

	agentID := "e2e-lease-agent"

	// 写入数据
	state := &AgentState{
		AgentID:   agentID,
		SessionID: "lease-session",
		Status:    "active",
		SavedAt:   time.Now(),
	}
	if err := store.Save(ctx, state); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}

	// 立即可读
	loaded, err := store.Load(ctx, agentID)
	if err != nil {
		t.Fatalf("立即 Load 失败: %v", err)
	}
	if loaded.AgentID != agentID {
		t.Errorf("AgentID = %q, want %q", loaded.AgentID, agentID)
	}

	// 等待租约过期
	t.Logf("等待 %v 租约过期...", shortTTL+2*time.Second)
	time.Sleep(shortTTL + 2*time.Second)

	// 租约过期后应无法读取
	_, err = store.Load(ctx, agentID)
	if err == nil {
		t.Error("租约过期后 Load 应返回错误")
	} else {
		t.Logf("租约过期后数据已清理（符合预期）: %v", err)
	}
}

// ===== TestE2E_EtcdCheckpoint_CrossNodeRecovery =====

// TestE2E_EtcdCheckpoint_CrossNodeRecovery 模拟节点切换，验证状态恢复。
func TestE2E_EtcdCheckpoint_CrossNodeRecovery(t *testing.T) {
	requireEtcd(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prefix := "e2e-recovery"
	ttl := 60 * time.Second

	// 模拟节点 A 写入状态
	storeA, err := NewEtcdCheckpointStore([]string{getEtcdAddr()}, prefix, ttl)
	if err != nil {
		t.Fatalf("创建 storeA 失败: %v", err)
	}
	defer storeA.client.Close()

	agentID := "e2e-recovery-agent"
	state := &AgentState{
		AgentID:   agentID,
		SessionID: "recovery-session",
		Status:    "running",
		Messages: []CheckpointMessage{
			{Role: "user", Content: "task 1"},
			{Role: "assistant", Content: "processing"},
		},
		TurnCount: 10,
		SavedAt:   time.Now(),
	}

	if err := storeA.Save(ctx, state); err != nil {
		t.Fatalf("节点 A Save 失败: %v", err)
	}
	t.Log("节点 A 写入状态成功")

	// 模拟节点 B 接管，读取节点 A 的状态
	storeB, err := NewEtcdCheckpointStore([]string{getEtcdAddr()}, prefix, ttl)
	if err != nil {
		t.Fatalf("创建 storeB 失败: %v", err)
	}
	defer storeB.client.Close()

	recovered, err := storeB.Load(ctx, agentID)
	if err != nil {
		t.Fatalf("节点 B Load 失败: %v", err)
	}

	if recovered.AgentID != agentID {
		t.Errorf("恢复的 AgentID = %q, want %q", recovered.AgentID, agentID)
	}
	if recovered.TurnCount != 10 {
		t.Errorf("恢复的 TurnCount = %d, want 10", recovered.TurnCount)
	}
	if len(recovered.Messages) != 2 {
		t.Errorf("恢复的 Messages 数量 = %d, want 2", len(recovered.Messages))
	}
	t.Logf("节点 B 成功恢复状态: status=%s, turns=%d", recovered.Status, recovered.TurnCount)

	// 节点 B 继续写入
	recovered.Status = "recovered"
	recovered.TurnCount = 11
	if err := storeB.Save(ctx, recovered); err != nil {
		t.Fatalf("节点 B Save 失败: %v", err)
	}

	// 验证最终状态
	final, err := storeB.Load(ctx, agentID)
	if err != nil {
		t.Fatalf("最终 Load 失败: %v", err)
	}
	if final.Status != "recovered" {
		t.Errorf("最终 Status = %q, want 'recovered'", final.Status)
	}

	// 清理
	_ = storeB.Delete(ctx, agentID)
}
