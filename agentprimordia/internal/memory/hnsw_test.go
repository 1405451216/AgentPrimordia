package memory

import (
	"cmp"
	"context"
	"fmt"
	"math/rand"
	"slices"
	"testing"
)

func TestHNSW_InsertAndSearch(t *testing.T) {
	idx := NewHNSWIndex(HNSWConfig{
		MaxConnections: 16,
		EfConstruction: 200,
		EfSearch:       50,
		Dimensions:     128,
	})

	// 插入 100 个向量
	for i := 0; i < 100; i++ {
		v := make([]float32, 128)
		for j := range v {
			v[j] = rand.Float32()
		}
		idx.Insert(context.Background(), fmt.Sprintf("vec-%d", i), v, nil)
	}

	// 搜索最近邻
	query := make([]float32, 128)
	for j := range query {
		query[j] = rand.Float32()
	}

	results := idx.Search(context.Background(), query, 10)
	if len(results) != 10 {
		t.Errorf("结果数 = %d, 期望 10", len(results))
	}

	// 验证结果按距离排序
	for i := 1; i < len(results); i++ {
		if results[i].Distance < results[i-1].Distance {
			t.Error("结果未按距离升序排列")
		}
	}
}

func TestHNSW_Recall(t *testing.T) {
	idx := NewHNSWIndex(HNSWConfig{
		MaxConnections: 16,
		EfConstruction: 200,
		EfSearch:       100,
		Dimensions:     64,
	})

	// 插入 1000 个向量
	vectors := make([][]float32, 1000)
	for i := range vectors {
		v := make([]float32, 64)
		for j := range v {
			v[j] = rand.Float32()
		}
		vectors[i] = v
		idx.Insert(context.Background(), fmt.Sprintf("vec-%d", i), v, nil)
	}

	// 查询并对比暴力搜索
	query := make([]float32, 64)
	for j := range query {
		query[j] = rand.Float32()
	}

	hnswResults := idx.Search(context.Background(), query, 10)

	// 暴力搜索求真实 top-10
	bruteForce := bruteForceSearch(vectors, query, 10)

	// 计算 recall@10
	hits := 0
	hnswIDs := make(map[string]bool)
	for _, r := range hnswResults {
		hnswIDs[r.ID] = true
	}
	for _, id := range bruteForce {
		if hnswIDs[id] {
			hits++
		}
	}

	recall := float64(hits) / 10.0
	if recall < 0.8 {
		t.Errorf("Recall@10 = %.2f, 期望 >= 0.8", recall)
	}
}

func TestHNSW_Delete(t *testing.T) {
	idx := NewHNSWIndex(HNSWConfig{
		MaxConnections: 16,
		EfConstruction: 200,
		EfSearch:       50,
		Dimensions:     32,
	})

	v := make([]float32, 32)
	for j := range v {
		v[j] = 0.5
	}
	idx.Insert(context.Background(), "vec-1", v, nil)
	idx.Insert(context.Background(), "vec-2", v, nil)

	idx.Delete("vec-1")

	results := idx.Search(context.Background(), v, 10)
	for _, r := range results {
		if r.ID == "vec-1" {
			t.Error("已删除的向量仍出现在搜索结果中")
		}
	}
}

func TestHNSW_Empty(t *testing.T) {
	idx := NewHNSWIndex(HNSWConfig{Dimensions: 32})
	results := idx.Search(context.Background(), make([]float32, 32), 10)
	if len(results) != 0 {
		t.Errorf("空索引搜索应返回空, 得到 %d 条", len(results))
	}
}

// TestHNSW_Compact 验证 v6.x 修复（评估报告 Issue #12）：
// Compact 必须物理回收僵尸节点、清理邻居引用、修复入口。
func TestHNSW_Compact(t *testing.T) {
	idx := NewHNSWIndex(HNSWConfig{
		MaxConnections: 16,
		EfConstruction: 200,
		EfSearch:       50,
		Dimensions:     8,
	})

	// 插入 4 个向量
	for i := 0; i < 4; i++ {
		v := make([]float32, 8)
		v[0] = float32(i + 1)
		v[1] = float32(i + 1) * 0.1
		idx.Insert(context.Background(), fmt.Sprintf("vec-%d", i), v, nil)
	}

	if idx.Len() != 4 {
		t.Fatalf("Len = %d, want 4", idx.Len())
	}

	// 删除 2 个（惰性）
	idx.Delete("vec-0")
	idx.Delete("vec-3")
	if idx.DeletedCount() != 2 {
		t.Fatalf("DeletedCount = %d, want 2", idx.DeletedCount())
	}

	// Compact 前搜索不应返回已删除
	query := make([]float32, 8)
	query[0] = 2.0
	before := idx.Search(context.Background(), query, 10)
	for _, r := range before {
		if r.ID == "vec-0" || r.ID == "vec-3" {
			t.Fatalf("删除节点在 Compact 前出现在结果中: %s", r.ID)
		}
	}

	// Compact 物理回收
	removed := idx.Compact()
	if removed != 2 {
		t.Fatalf("Compact removed = %d, want 2", removed)
	}
	if idx.Len() != 2 {
		t.Fatalf("Compact 后 Len = %d, want 2", idx.Len())
	}
	if idx.DeletedCount() != 0 {
		t.Fatalf("Compact 后 DeletedCount = %d, want 0", idx.DeletedCount())
	}

	// 回收后搜索依然正常
	after := idx.Search(context.Background(), query, 10)
	if len(after) == 0 {
		t.Fatal("Compact 后搜索返回空")
	}
	for _, r := range after {
		if r.ID == "vec-0" || r.ID == "vec-3" {
			t.Fatalf("Compact 后仍返回删除节点: %s", r.ID)
		}
	}
}

// TestHNSW_CompactAll 验证全部删除后 Compact 重置为空索引。
func TestHNSW_CompactAll(t *testing.T) {
	idx := NewHNSWIndex(HNSWConfig{Dimensions: 4})

	v := make([]float32, 4)
	idx.Insert(context.Background(), "a", v, nil)
	idx.Insert(context.Background(), "b", v, nil)

	idx.Delete("a")
	idx.Delete("b")

	if removed := idx.Compact(); removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
	if idx.Len() != 0 {
		t.Fatalf("全部删除后 Len = %d, want 0", idx.Len())
	}
	results := idx.Search(context.Background(), v, 5)
	if len(results) != 0 {
		t.Fatalf("空索引搜索应返回空, got %d", len(results))
	}
}

// TestVectorStore_HNSWDeleteThreshold 验证自动压缩阈值：
// 删除数达阈值时自动触发 Compact。
func TestVectorStore_HNSWDeleteThreshold(t *testing.T) {
	vs := NewVectorStoreWithHNSW(8, HNSWConfig{Dimensions: 8})
	vs.WithHNSWDeleteThreshold(3) // 删除 3 次后自动压缩

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		v := make([]float32, 8)
		v[0] = float32(i)
		if err := vs.Add(ctx, fmt.Sprintf("id-%d", i), v, nil); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	// 删除 4 个：第 3 次删除触发一次自动压缩（清 3 个），
	// 第 4 次删除后剩 1 个僵尸等待下一轮。
	for i := 0; i < 4; i++ {
		if err := vs.Delete(ctx, fmt.Sprintf("id-%d", i)); err != nil {
			t.Fatalf("Delete id-%d: %v", i, err)
		}
	}

	// 向量存储层只剩 1 个条目（id-4）
	if vs.Count() != 1 {
		t.Fatalf("Count = %d, want 1", vs.Count())
	}
	// 自动压缩只触发过一次：阈值 3，故第 4 次删除后还剩 1 个僵尸
	if vs.HNSWDeletedCount() != 1 {
		t.Fatalf("HNSW 僵尸数 = %d, want 1（阈值 3，仅压缩一次后剩 1）", vs.HNSWDeletedCount())
	}

	// 手动触发第二次压缩，全部回收
	if n := vs.Compact(); n != 1 {
		t.Fatalf("手动 Compact 回收数 = %d, want 1", n)
	}
	if vs.HNSWDeletedCount() != 0 {
		t.Fatalf("手动压缩后应无僵尸节点, got %d", vs.HNSWDeletedCount())
	}
}

// bruteForceSearch 暴力搜索用于对比
func bruteForceSearch(vectors [][]float32, query []float32, k int) []string {
	type scored struct {
		id   string
		dist float32
	}
	var all []scored
	for i, v := range vectors {
		d := hnswCosineDistance(v, query)
		all = append(all, scored{fmt.Sprintf("vec-%d", i), d})
	}
	// 优化（Task 19）：使用泛型 slices.SortFunc 替代 sort.Slice，避免反射开销
	slices.SortFunc(all, func(a, b scored) int { return cmp.Compare(a.dist, b.dist) })
	ids := make([]string, k)
	for i := 0; i < k && i < len(all); i++ {
		ids[i] = all[i].id
	}
	return ids
}
