package memory

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
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
	sort.Slice(all, func(i, j int) bool { return all[i].dist < all[j].dist })
	ids := make([]string, k)
	for i := 0; i < k && i < len(all); i++ {
		ids[i] = all[i].id
	}
	return ids
}
