package agent

import (
	"bytes"
	"sync"
	"testing"
)

// TestAcquireReleaseBuffer 验证 buffer 池的基本存取流程。
func TestAcquireReleaseBuffer(t *testing.T) {
	buf := AcquireBuffer()
	if buf == nil {
		t.Fatal("AcquireBuffer 返回 nil")
	}
	if buf.Len() != 0 {
		t.Errorf("新获取的 buffer 长度应为 0，实际 %d", buf.Len())
	}
	buf.WriteString("hello")
	if buf.String() != "hello" {
		t.Errorf("写入后内容错误：%q", buf.String())
	}
	ReleaseBuffer(buf)

	// 重新获取，buffer 应已被重置
	buf2 := AcquireBuffer()
	if buf2.Len() != 0 {
		t.Errorf("复用后 buffer 长度应为 0，实际 %d", buf2.Len())
	}
	ReleaseBuffer(buf2)
}

// TestReleaseBuffer_NilSafe 验证 ReleaseBuffer 对 nil 输入安全。
func TestReleaseBuffer_NilSafe(t *testing.T) {
	// 不应 panic
	ReleaseBuffer(nil)
}

// TestReleaseBuffer_LargeTruncation 验证大 buffer 归还时会被截断。
func TestReleaseBuffer_LargeTruncation(t *testing.T) {
	buf := AcquireBuffer()
	// 写入 8KB 数据，触发扩容
	buf.Grow(8192)
	for i := 0; i < 8192; i++ {
		buf.WriteByte('x')
	}
	if buf.Cap() < 4096 {
		t.Fatalf("扩容失败：Cap=%d", buf.Cap())
	}
	ReleaseBuffer(buf)

	buf2 := AcquireBuffer()
	// 大 buffer 截断后应为 0 容量
	if buf2.Cap() > 4096 {
		t.Errorf("大 buffer 未被截断：Cap=%d", buf2.Cap())
	}
	ReleaseBuffer(buf2)
}

// TestAcquireBufferWithSize 验证预分配容量的 buffer。
func TestAcquireBufferWithSize(t *testing.T) {
	buf := AcquireBufferWithSize(2048)
	if buf.Cap() < 2048 {
		t.Errorf("预分配 buffer 容量不足：Cap=%d", buf.Cap())
	}
	ReleaseBuffer(buf)
}

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

// TestBufferPool_Concurrent 验证 buffer 池在并发场景下无竞态。
func TestBufferPool_Concurrent(t *testing.T) {
	const goroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				buf := AcquireBuffer()
				buf.WriteString("data")
				ReleaseBuffer(buf)
			}
		}()
	}
	wg.Wait()
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

// TestBufferPool_ContentIndependence 验证归还后新获取的 buffer 不带旧内容。
func TestBufferPool_ContentIndependence(t *testing.T) {
	buf1 := AcquireBuffer()
	buf1.WriteString("secret-data")
	ReleaseBuffer(buf1)

	buf2 := AcquireBuffer()
	if bytes.Contains(buf2.Bytes(), []byte("secret")) {
		t.Error("buffer 复用导致旧数据泄露")
	}
	ReleaseBuffer(buf2)
}
