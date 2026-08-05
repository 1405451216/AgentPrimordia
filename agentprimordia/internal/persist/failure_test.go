package persist

import (
	"context"
	"strings"
	"testing"
	"time"
)

// newTestFailureRecord 构造一条测试用失败记录
func newTestFailureRecord(id, agentID, sessionID, errMsg string) *FailureRecord {
	return &FailureRecord{
		ID:        id,
		AgentID:   agentID,
		SessionID: sessionID,
		Phase:     PhaseRun,
		Error:     errMsg,
		Turn:      3,
		Input:     "test input",
		CreatedAt: time.Now().UTC(),
	}
}

func TestMemoryFailureStore_RecordAndGet(t *testing.T) {
	store := NewMemoryFailureStore()
	ctx := context.Background()

	rec := newTestFailureRecord("f1", "agent-a", "s1", "llm timeout")
	if err := store.Record(ctx, rec); err != nil {
		t.Fatalf("Record 失败: %v", err)
	}

	got, err := store.Get(ctx, "f1")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.ID != "f1" || got.AgentID != "agent-a" || got.Error != "llm timeout" {
		t.Fatalf("Get 返回内容不符: %+v", got)
	}
}

func TestMemoryFailureStore_GetNotFound(t *testing.T) {
	store := NewMemoryFailureStore()
	if _, err := store.Get(context.Background(), "missing"); err == nil {
		t.Fatal("期望 Get 不存在的记录返回错误")
	}
}

func TestMemoryFailureStore_ListByAgent(t *testing.T) {
	store := NewMemoryFailureStore()
	ctx := context.Background()

	_ = store.Record(ctx, newTestFailureRecord("f1", "agent-a", "s1", "e1"))
	_ = store.Record(ctx, newTestFailureRecord("f2", "agent-b", "s1", "e2"))
	_ = store.Record(ctx, newTestFailureRecord("f3", "agent-a", "s2", "e3"))

	list, err := store.List(ctx, "agent-a")
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("期望 2 条 agent-a 记录，实际 %d", len(list))
	}
	for _, r := range list {
		if r.AgentID != "agent-a" {
			t.Fatalf("List 返回了其他 Agent 的记录: %+v", r)
		}
	}
}

func TestMemoryFailureStore_ListAll(t *testing.T) {
	store := NewMemoryFailureStore()
	ctx := context.Background()

	_ = store.Record(ctx, newTestFailureRecord("f1", "agent-a", "s1", "e1"))
	_ = store.Record(ctx, newTestFailureRecord("f2", "agent-b", "s1", "e2"))

	list, err := store.List(ctx, "")
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("空 agentID 应返回全部记录，期望 2 条，实际 %d", len(list))
	}
}

func TestMemoryFailureStore_Delete(t *testing.T) {
	store := NewMemoryFailureStore()
	ctx := context.Background()

	_ = store.Record(ctx, newTestFailureRecord("f1", "agent-a", "s1", "e1"))
	if err := store.Delete(ctx, "f1"); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}
	if _, err := store.Get(ctx, "f1"); err == nil {
		t.Fatal("删除后 Get 应返回错误")
	}
}

func TestMemoryFailureStore_RecordRequiresID(t *testing.T) {
	store := NewMemoryFailureStore()
	rec := newTestFailureRecord("", "agent-a", "s1", "e1")
	if err := store.Record(context.Background(), rec); err == nil {
		t.Fatal("期望空 ID 的 Record 返回错误")
	}
}

func TestMemoryFailureStore_RecordEmbedsState(t *testing.T) {
	store := NewMemoryFailureStore()
	ctx := context.Background()

	rec := newTestFailureRecord("f1", "agent-a", "s1", "e1")
	rec.State = &AgentState{
		AgentID:   "agent-a",
		SessionID: "s1",
		Status:    "failed",
		TurnCount: 3,
		Messages:  []CheckpointMessage{{Role: "user", Content: "test input"}},
	}
	if err := store.Record(ctx, rec); err != nil {
		t.Fatalf("Record 失败: %v", err)
	}
	got, _ := store.Get(ctx, "f1")
	if got.State == nil || got.State.Status != "failed" || got.State.TurnCount != 3 {
		t.Fatalf("嵌入的 AgentState 未正确保存: %+v", got.State)
	}
}

func TestFailureRecord_Diagnose(t *testing.T) {
	rec := newTestFailureRecord("f1", "coding-agent", "s1", "subtask t2 failed: tool error")
	rec.Phase = PhasePlan
	rec.SubTaskID = "t2"
	rec.State = &AgentState{Status: "failed", TurnCount: 3}

	diag := rec.Diagnose()
	for _, want := range []string{"coding-agent", "plan", "t2", "tool error", "可重放"} {
		if !strings.Contains(diag, want) {
			t.Errorf("诊断摘要缺少 %q，实际:\n%s", want, diag)
		}
	}
}

func TestFailureRecord_DiagnoseNotReplayable(t *testing.T) {
	rec := newTestFailureRecord("f1", "coding-agent", "s1", "some error")
	diag := rec.Diagnose()
	if !strings.Contains(diag, "不可重放") {
		t.Errorf("无 State 时应标记不可重放，实际:\n%s", diag)
	}
}
