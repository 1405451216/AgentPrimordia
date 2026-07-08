package agent

import (
	"sync"
	"testing"
)

// TestAcquireHookContext 验证 HookContext 池的基本存取。
func TestAcquireHookContext(t *testing.T) {
	hctx := AcquireHookContext()
	if hctx == nil {
		t.Fatal("AcquireHookContext 返回 nil")
	}
	// 设置字段
	hctx.AgentID = "test-agent"
	hctx.Turn = 5
	if hctx.AgentID != "test-agent" || hctx.Turn != 5 {
		t.Error("字段设置失败")
	}
	ReleaseHookContext(hctx)

	// 重新获取，所有字段应已重置
	hctx2 := AcquireHookContext()
	if hctx2.AgentID != "" {
		t.Errorf("AgentID 未重置：%q", hctx2.AgentID)
	}
	if hctx2.Turn != 0 {
		t.Errorf("Turn 未重置：%d", hctx2.Turn)
	}
	ReleaseHookContext(hctx2)
}

// TestReleaseHookContext_NilSafe 验证 ReleaseHookContext 对 nil 安全。
func TestReleaseHookContext_NilSafe(t *testing.T) {
	// 不应 panic
	ReleaseHookContext(nil)
}

// TestHookContextReset_MapCleanup 验证 Metadata map 在 Reset 时被清空但保留容量。
func TestHookContextReset_MapCleanup(t *testing.T) {
	hctx := AcquireHookContext()
	hctx.Metadata = make(map[string]any, 10)
	hctx.Metadata["key1"] = "value1"
	hctx.Metadata["key2"] = "value2"
	if len(hctx.Metadata) != 2 {
		t.Fatalf("Metadata 长度错误：%d", len(hctx.Metadata))
	}

	hctx.Reset()
	if len(hctx.Metadata) != 0 {
		t.Errorf("Reset 后 Metadata 应为空，实际 %d", len(hctx.Metadata))
	}
	// map 容量应保留（map 不重置容量，delete 仅删除键值）
	ReleaseHookContext(hctx)
}

// TestHookContextPool_Concurrent 验证 HookContext 池在并发场景下无竞态。
func TestHookContextPool_Concurrent(t *testing.T) {
	const goroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				hctx := AcquireHookContext()
				hctx.Turn = j
				hctx.AgentID = "agent"
				ReleaseHookContext(hctx)
			}
		}()
	}
	wg.Wait()
}
