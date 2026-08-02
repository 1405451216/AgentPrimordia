package autonomy

import (
	"context"
	"sync"
	"testing"
)

// mockMemoryStore 模拟记忆存储
type mockMemoryStore struct {
	mu      sync.Mutex
	entries []MemoryEntry
}

func (m *mockMemoryStore) Save(ctx context.Context, entry MemoryEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockMemoryStore) Query(ctx context.Context, goalID string) ([]MemoryEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []MemoryEntry
	for _, e := range m.entries {
		if e.GoalID == goalID {
			result = append(result, e)
		}
	}
	return result, nil
}

// TestMemorySaveAndQuery 验证记忆存取
func TestMemorySaveAndQuery(t *testing.T) {
	store := &mockMemoryStore{}
	mem := NewGoalMemory(store)

	ctx := context.Background()
	err := mem.SaveContext(ctx, "goal-1", "执行中发现数据源不可用")
	if err != nil {
		t.Fatalf("save context: %v", err)
	}

	err = mem.SaveLesson(ctx, "goal-1", "数据源不可用时应先检查连接池")
	if err != nil {
		t.Fatalf("save lesson: %v", err)
	}

	entries, err := mem.Query(ctx, "goal-1")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].Type != MemoryTypeContext {
		t.Errorf("entry[0] type = %s, want context", entries[0].Type)
	}
	if entries[1].Type != MemoryTypeLesson {
		t.Errorf("entry[1] type = %s, want lesson", entries[1].Type)
	}
}

// TestMemoryFailureFeedback 验证失败经验反馈
func TestMemoryFailureFeedback(t *testing.T) {
	store := &mockMemoryStore{}
	mem := NewGoalMemory(store)
	ctx := context.Background()

	// 记录失败尝试
	err := mem.SaveFailure(ctx, "goal-1", "s1", "连接超时", "重试3次后放弃")
	if err != nil {
		t.Fatalf("save failure: %v", err)
	}

	// 查询失败经验
	failures, err := mem.QueryFailures(ctx, "goal-1")
	if err != nil {
		t.Fatalf("query failures: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	if failures[0].StepID != "s1" {
		t.Errorf("failure step = %q, want %q", failures[0].StepID, "s1")
	}
	if failures[0].Error != "连接超时" {
		t.Errorf("failure error = %q, want %q", failures[0].Error, "连接超时")
	}
}

// TestMemoryIsolation 验证目标间记忆隔离
func TestMemoryIsolation(t *testing.T) {
	store := &mockMemoryStore{}
	mem := NewGoalMemory(store)
	ctx := context.Background()

	_ = mem.SaveContext(ctx, "goal-1", "目标1的上下文")
	_ = mem.SaveContext(ctx, "goal-2", "目标2的上下文")

	entries1, _ := mem.Query(ctx, "goal-1")
	entries2, _ := mem.Query(ctx, "goal-2")

	if len(entries1) != 1 || entries1[0].Content != "目标1的上下文" {
		t.Errorf("goal-1 entries = %v", entries1)
	}
	if len(entries2) != 1 || entries2[0].Content != "目标2的上下文" {
		t.Errorf("goal-2 entries = %v", entries2)
	}
}
