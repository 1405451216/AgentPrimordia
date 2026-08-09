package memory

// sqlite_close_test.go — SQLiteStore Close 安全回归测试（评估报告 §三.1-③）
//
// 修复前：Close() 将 s.db 置 nil，而 Search/Stats 等无锁读 s.db →
// 关闭后调用空指针 panic、关闭与读并发为指针数据竞争。
// 修复后：Close 置 closed 标志（不再置 nil），所有方法返回 ErrStoreClosed。

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestSQLiteStore_ClosedMethodsReturnErr 验证 Close 后所有读/写方法返回
// ErrStoreClosed 而非 panic。
func TestSQLiteStore_ClosedMethodsReturnErr(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatalf("WithInMemory: %v", err)
	}
	if err := store.Add(context.Background(), MustEpisode("s1", "user", "hello")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	ctx := context.Background()
	checks := []struct {
		name string
		fn   func() error
	}{
		{"Stats", func() error { _, err := store.Stats(ctx); return err }},
		{"Search", func() error { _, err := store.Search(ctx, "hello", nil); return err }},
		{"SearchAdvanced", func() error {
			_, err := store.SearchAdvanced(ctx, SearchOptions{Query: "hello"})
			return err
		}},
		{"Add", func() error { return store.Add(ctx, MustEpisode("s2", "user", "world")) }},
		{"Get", func() error { _, err := store.Get(ctx, "x"); return err }},
		{"List", func() error { _, err := store.List(ctx, nil); return err }},
		{"Count", func() error { _, err := store.Count(ctx, "s1"); return err }},
		{"GetImportant", func() error { _, err := store.GetImportant(ctx, 0.5, 10); return err }},
		{"GetTimeline", func() error { _, err := store.GetTimeline(ctx, 7); return err }},
		{"CleanupExpired", func() error { _, err := store.CleanupExpired(ctx, 30); return err }},
		{"UpdateSummary", func() error { return store.UpdateSummary(ctx, "x", "s", "t") }},
		{"Delete", func() error { return store.Delete(ctx, "x") }},
	}
	for _, c := range checks {
		if err := c.fn(); !errors.Is(err, ErrStoreClosed) {
			t.Errorf("%s: err = %v, want ErrStoreClosed", c.name, err)
		}
	}
}

// TestSQLiteStore_ConcurrentCloseAndOps 并发 Close + 读写操作。
// 本机无 race 检测器时仅作压力冒烟；CI Linux -race 下验证指针字段无数据竞争。
func TestSQLiteStore_ConcurrentCloseAndOps(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatalf("WithInMemory: %v", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	// 读者 goroutine：Close 前后持续调用，允许 ErrStoreClosed / sql 错误，
	// 但绝不允许 panic 或数据竞争。
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, _ = store.Search(ctx, "hello", nil)
				_ = store.Add(ctx, MustEpisode("s1", "user", "hello"))
				_, _ = store.Stats(ctx)
			}
		}()
	}

	time.Sleep(20 * time.Millisecond)
	_ = store.Close()
	close(stop)
	wg.Wait()
}
