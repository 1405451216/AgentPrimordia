// unified_test.go — UnifiedMemory 组合实现测试（v6.x 评估报告 Issue #11）
package memory

import (
	"context"
	"errors"
	"testing"
)

func newTestUnified(t *testing.T) (*UnifiedMemory, *InMemoryStore, *InMemoryVectorStore) {
	t.Helper()
	mem := NewInMemoryStore()
	vec := NewInMemoryVectorStore()
	if err := vec.CreateCollection(context.Background(), "mem:test-sess", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	return NewUnifiedMemory(mem, vec), mem, vec
}

func TestUnifiedMemory_AddWithVector_Success(t *testing.T) {
	u, mem, vec := newTestUnified(t)
	ctx := context.Background()

	ep := &Episode{
		ID:         "ep-1",
		SessionID:  "test-sess",
		Role:       "user",
		Content:    "hello memory",
		Importance: 0.9,
	}
	vector := []float32{1, 0, 0, 0}

	if err := u.AddWithVector(ctx, ep, "", vector, nil); err != nil {
		t.Fatalf("AddWithVector: %v", err)
	}

	// 记忆已写入
	if _, err := mem.Get(ctx, "ep-1"); err != nil {
		t.Fatalf("episode not in memory: %v", err)
	}
	// 向量已写入（集合自动命名 mem:test-sess）
	if n := vec.Count("mem:test-sess"); n != 1 {
		t.Fatalf("vector count = %d, want 1", n)
	}

	// SearchHybrid 能回填 Episode
	res, err := u.SearchHybrid(ctx, "test-sess", []float32{1, 0, 0, 0}, VectorSearchOptions{TopK: 5})
	if err != nil {
		t.Fatalf("SearchHybrid: %v", err)
	}
	if len(res) != 1 || res[0].ID != "ep-1" {
		t.Fatalf("SearchHybrid = %+v, want [ep-1]", res)
	}
	if res[0].Episode == nil || res[0].Episode.Content != "hello memory" {
		t.Fatalf("SearchHybrid 未回填 Episode: %+v", res[0])
	}
}

// TestUnifiedMemory_AddWithVector_Compensate 验证补偿事务：
// 向量写入失败时，已写入的记忆必须被删除（不留孤儿）。
func TestUnifiedMemory_AddWithVector_Compensate(t *testing.T) {
	mem := NewInMemoryStore()
	// 用一个"始终失败"的 VectorStore 模拟向量写入失败
	vec := &failingVectorStore{}
	u := NewUnifiedMemory(mem, vec)
	ctx := context.Background()

	ep := &Episode{ID: "ep-x", SessionID: "s", Content: "orphan-test"}
	err := u.AddWithVector(ctx, ep, "", []float32{1, 2, 3, 4}, nil)
	if err == nil {
		t.Fatal("向量写入失败时 AddWithVector 必须返回错误")
	}

	// 记忆必须被补偿删除
	if _, gerr := mem.Get(ctx, "ep-x"); gerr == nil {
		t.Fatal("补偿失败：记忆仍存在（孤儿数据）")
	}
}

func TestUnifiedMemory_DeleteWithVector(t *testing.T) {
	u, mem, vec := newTestUnified(t)
	ctx := context.Background()

	ep := &Episode{ID: "ep-2", SessionID: "test-sess", Role: "user", Content: "to delete"}
	if err := u.AddWithVector(ctx, ep, "", []float32{0, 1, 0, 0}, nil); err != nil {
		t.Fatalf("AddWithVector: %v", err)
	}

	if err := u.DeleteWithVector(ctx, "ep-2", "mem:test-sess"); err != nil {
		t.Fatalf("DeleteWithVector: %v", err)
	}
	if _, err := mem.Get(ctx, "ep-2"); err == nil {
		t.Fatal("episode 未被删除")
	}
	if n := vec.Count("mem:test-sess"); n != 0 {
		t.Fatalf("vector count = %d, want 0", n)
	}
}

func TestCollectionNameForSession(t *testing.T) {
	if got := CollectionNameForSession("abc"); got != "mem:abc" {
		t.Fatalf("CollectionNameForSession(abc) = %q", got)
	}
	if got := CollectionNameForSession(""); got != "mem:default" {
		t.Fatalf("CollectionNameForSession() = %q", got)
	}
}

// failingVectorStore 测试用：所有写入都失败。
type failingVectorStore struct{}

func (f *failingVectorStore) Insert(ctx context.Context, collection string, records []*VectorRecord) error {
	return errors.New("simulated vector insert failure")
}
func (f *failingVectorStore) Delete(ctx context.Context, collection string, ids []string) error {
	return nil
}
func (f *failingVectorStore) Search(ctx context.Context, collection string, query []float32, opts VectorSearchOptions) ([]*VectorMatch, error) {
	return nil, nil
}
func (f *failingVectorStore) CreateCollection(ctx context.Context, name string, dim int) error {
	return nil
}
func (f *failingVectorStore) DropCollection(ctx context.Context, name string) error {
	return nil
}
