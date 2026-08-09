package persist

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestSQLiteFailureStore(t *testing.T) *SQLiteFailureStore {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "failures.db")
	s, err := NewSQLiteFailureStore(dsn)
	if err != nil {
		t.Fatalf("NewSQLiteFailureStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func sampleFailure(id string, createdAt time.Time) *FailureRecord {
	return &FailureRecord{
		ID:        id,
		AgentID:   "agent-a",
		SessionID: "sess-1",
		Phase:     PhaseRun,
		Error:     "LLM 超时",
		Turn:      3,
		Input:     "查询数据",
		State: &AgentState{
			AgentID:   "agent-a",
			Status:    "failed",
			TurnCount: 3,
			Metrics:   CheckpointMetrics{TotalTurns: 3},
		},
		CreatedAt: createdAt,
	}
}

// TestSQLiteFailureStore_CRUD 验证 Record → Get → List → Delete 全链路。
func TestSQLiteFailureStore_CRUD(t *testing.T) {
	s := newTestSQLiteFailureStore(t)
	ctx := context.Background()
	base := time.Now()

	if err := s.Record(ctx, sampleFailure("f1", base.Add(time.Millisecond))); err != nil {
		t.Fatalf("Record f1: %v", err)
	}
	if err := s.Record(ctx, sampleFailure("f2", base.Add(2*time.Millisecond))); err != nil {
		t.Fatalf("Record f2: %v", err)
	}

	got, err := s.Get(ctx, "f1")
	if err != nil {
		t.Fatalf("Get f1: %v", err)
	}
	if got.ID != "f1" || got.Error != "LLM 超时" || got.Turn != 3 {
		t.Errorf("got = %+v, want 完整记录", got)
	}
	if got.State == nil || got.State.TurnCount != 3 {
		t.Errorf("state 应可重放（检查点 JSON 往返）")
	}

	list, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].ID != "f2" || list[1].ID != "f1" {
		t.Errorf("list = %v, want 新→旧 [f2 f1]", ids(list))
	}

	// agent 过滤
	only, err := s.List(ctx, "agent-a")
	if err != nil || len(only) != 2 {
		t.Errorf("agent filter = %d, want 2", len(only))
	}
	none, err := s.List(ctx, "agent-x")
	if err != nil || len(none) != 0 {
		t.Errorf("agent-x filter = %d, want 0", len(none))
	}

	if err := s.Delete(ctx, "f1"); err != nil {
		t.Fatalf("Delete f1: %v", err)
	}
	if _, err := s.Get(ctx, "f1"); err == nil {
		t.Error("删除后 Get 应报错")
	}
	if err := s.Delete(ctx, "f1"); err == nil {
		t.Error("重复 Delete 应报错")
	}
}

// TestSQLiteFailureStore_Persistence 验证进程间持久化：关闭后重开数据仍在。
func TestSQLiteFailureStore_Persistence(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "persist.db")
	ctx := context.Background()

	s1, err := NewSQLiteFailureStore(dsn)
	if err != nil {
		t.Fatalf("NewSQLiteFailureStore: %v", err)
	}
	if err := s1.Record(ctx, sampleFailure("p1", time.Now())); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := NewSQLiteFailureStore(dsn)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	got, err := s2.Get(ctx, "p1")
	if err != nil {
		t.Fatalf("reopen Get: %v", err)
	}
	if got.ID != "p1" || got.State == nil {
		t.Errorf("reopen 后记录应完整（含检查点）")
	}
}

// TestSQLiteFailureStore_Concurrent 验证并发写读安全。
func TestSQLiteFailureStore_Concurrent(t *testing.T) {
	s := newTestSQLiteFailureStore(t)
	ctx := context.Background()

	const n = 20
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("c%d", i)
			_ = s.Record(ctx, sampleFailure(id, time.Now().Add(time.Duration(i)*time.Millisecond)))
		}()
	}
	wg.Wait()

	list, err := s.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != n {
		t.Errorf("records = %d, want %d", len(list), n)
	}
}

// TestSQLiteFailureStore_Closed 验证 Close 后所有方法报错（不 panic）。
func TestSQLiteFailureStore_Closed(t *testing.T) {
	s := newTestSQLiteFailureStore(t)
	ctx := context.Background()
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := s.Record(ctx, sampleFailure("x", time.Now())); !errors.Is(err, ErrFailureStoreClosed) {
		t.Errorf("closed Record err = %v, want ErrFailureStoreClosed", err)
	}
	if _, err := s.Get(ctx, "x"); !errors.Is(err, ErrFailureStoreClosed) {
		t.Errorf("closed Get err = %v, want ErrFailureStoreClosed", err)
	}
	if _, err := s.List(ctx, ""); !errors.Is(err, ErrFailureStoreClosed) {
		t.Errorf("closed List err = %v, want ErrFailureStoreClosed", err)
	}
	if err := s.Delete(ctx, "x"); !errors.Is(err, ErrFailureStoreClosed) {
		t.Errorf("closed Delete err = %v, want ErrFailureStoreClosed", err)
	}
}

// TestSQLiteFailureStore_EmptyID 空 ID 拒绝（与 MemoryFailureStore 一致）。
func TestSQLiteFailureStore_EmptyID(t *testing.T) {
	s := newTestSQLiteFailureStore(t)
	if err := s.Record(context.Background(), &FailureRecord{}); err == nil {
		t.Error("空 ID Record 应报错")
	}
}

func ids(recs []*FailureRecord) []string {
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = r.ID
	}
	return out
}
