package memory

import (
	"context"
	"testing"
)

// FuzzRAGStoreHybridSearch 模糊测试 RAG 混合检索。
// 确保对任意查询字符串不 panic，且结果数不超过 topK。
func FuzzRAGStoreHybridSearch(f *testing.F) {
	// 种子语料
	seedQueries := []string{
		"hello world",
		"Go programming",
		"TypeScript vs Go",
		"",
		"这是一个中文查询",
		"SELECT * FROM users",
		"<script>alert('xss')</script>",
		"query with special chars: !@#$%^&*()",
		"very long query " + string(make([]byte, 1000)),
	}
	for _, s := range seedQueries {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, query string) {
		mem, _ := WithInMemory()
		defer mem.Close()
		ctx := context.Background()

		// 添加一些基础数据
		_ = mem.Add(ctx, &Episode{
			ID:        "ep1",
			SessionID: "s1",
			Role:      "user",
			Content:   "Go is a compiled language",
		})
		_ = mem.Add(ctx, &Episode{
			ID:        "ep2",
			SessionID: "s1",
			Role:      "assistant",
			Content:   "TypeScript is a typed superset of JavaScript",
		})

		store := NewRAGStore(mem, nil) // nil embedder: FTS-only
		cfg := DefaultRAGFusionConfig()
		cfg.FusionMode = FusionLinear
		store.SetFusionConfig(cfg)

		// HybridSearch 不应 panic
		results, err := store.HybridSearch(ctx, query, 5)

		// 空查询可能返回空结果或错误，都是合法的
		if err != nil {
			return // 错误是合法的
		}

		// 结果数不应超过 topK
		if len(results) > 5 {
			t.Errorf("结果数 %d 超过 topK=5", len(results))
		}
	})
}

// FuzzRAGStoreRRFSearch 模糊测试 RRF 融合模式下的混合检索。
func FuzzRAGStoreRRFSearch(f *testing.F) {
	seedQueries := []string{
		"test query",
		"",
		"RRF fusion test",
		"reciprocal rank",
		"混合检索测试",
	}
	for _, s := range seedQueries {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, query string) {
		mem, _ := WithInMemory()
		defer mem.Close()
		ctx := context.Background()

		_ = mem.Add(ctx, &Episode{
			ID:        "ep1",
			SessionID: "s1",
			Role:      "user",
			Content:   "test content for RRF fuzzing",
		})

		store := NewRAGStore(mem, nil)
		cfg := DefaultRAGFusionConfig()
		cfg.FusionMode = FusionRRF
		cfg.RRFK = 60
		store.SetFusionConfig(cfg)

		// RRF 模式不应 panic
		results, err := store.HybridSearch(ctx, query, 3)
		if err != nil {
			return
		}
		if len(results) > 3 {
			t.Errorf("RRF 结果数 %d 超过 topK=3", len(results))
		}

		// 验证 RRF 分数在合理范围内 [0, 1]
		for _, r := range results {
			if r.Score < 0 || r.Score > 1 {
				t.Errorf("RRF 分数 %f 超出 [0,1] 范围", r.Score)
			}
		}
	})
}

// FuzzRAGFusionConfigSetGet 模糊测试配置的并发设置和读取。
func FuzzRAGFusionConfigSetGet(f *testing.F) {
	seedModes := []int{0, 1, 0, 1}
	for _, m := range seedModes {
		f.Add(m, 10, 100)
		f.Add(m, 0, 0)
		f.Add(m, -1, 1000)
	}

	f.Fuzz(func(t *testing.T, mode int, rrfK int, overFetch int) {
		mem := NewInMemoryStore()
		store := NewRAGStore(mem, nil)

		cfg := DefaultRAGFusionConfig()
		if mode%2 == 0 {
			cfg.FusionMode = FusionLinear
		} else {
			cfg.FusionMode = FusionRRF
		}
		cfg.RRFK = rrfK
		cfg.OverFetchSize = overFetch

		// SetFusionConfig 不应 panic
		store.SetFusionConfig(cfg)

		// GetFusionConfig 应返回一致的配置
		got := store.GetFusionConfig()
		if got.FusionMode != cfg.FusionMode {
			t.Errorf("FusionMode 不一致：设置 %v，获取 %v", cfg.FusionMode, got.FusionMode)
		}
	})
}
