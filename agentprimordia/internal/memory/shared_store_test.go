package memory

import (
	"context"
	"testing"
)

func TestSharedStore_WriteAndRead(t *testing.T) {
	shared := NewSharedStore()
	mem1 := NewInMemoryStore()
	mem2 := NewInMemoryStore()

	ctx := context.Background()

	// Agent-1 写入共享记忆
	shared.Bind("agent-1", mem1)
	_ = shared.Publish(ctx, "agent-1", &Episode{
		ID:        "shared-1",
		SessionID: "shared",
		Content:   "共享知识：项目使用 Go 1.26",
		Role:      "user",
		Metadata:  map[string]string{"scope": "team"},
	})

	// Agent-2 读取共享记忆
	shared.Bind("agent-2", mem2)
	results, err := shared.SearchShared(ctx, "agent-2", "Go")
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}

	if len(results) == 0 {
		t.Error("Agent-2 应能搜索到 Agent-1 发布的共享记忆")
	}
}

func TestSharedStore_ScopeIsolation(t *testing.T) {
	shared := NewSharedStore()
	mem1 := NewInMemoryStore()
	mem2 := NewInMemoryStore()

	ctx := context.Background()

	shared.Bind("agent-1", mem1)
	shared.Bind("agent-2", mem2)

	// Agent-1 写入私有记忆（不共享）
	_ = mem1.Add(ctx, &Episode{
		ID:        "private-1",
		SessionID: "private",
		Content:   "Agent-1 的私有数据",
		Role:      "user",
	})

	// Agent-1 写入共享记忆
	_ = shared.Publish(ctx, "agent-1", &Episode{
		ID:        "shared-1",
		SessionID: "shared",
		Content:   "团队共享数据",
		Role:      "user",
		Metadata:  map[string]string{"scope": "team"},
	})

	// Agent-2 搜索只能看到共享的
	results, _ := shared.SearchShared(ctx, "agent-2", "数据")
	for _, r := range results {
		if r.ID == "private-1" {
			t.Error("Agent-2 不应看到 Agent-1 的私有记忆")
		}
	}
}
