package memory

import (
	"context"
	"testing"
)

// mockEmbeddingProvider 简单的 mock EmbeddingProvider 用于测试
type mockEmbeddingProvider struct{}

func (m *mockEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// 返回零向量（用于测试 FTS-only 路径）
	results := make([][]float32, len(texts))
	for i := range results {
		results[i] = make([]float32, 8)
	}
	return results, nil
}

func (m *mockEmbeddingProvider) Dimensions() int { return 8 }

// TestRAGFusionConfig_Default 验证默认融合配置。
func TestRAGFusionConfig_Default(t *testing.T) {
	cfg := DefaultRAGFusionConfig()
	if cfg.FusionMode != FusionLinear {
		t.Errorf("默认应为 FusionLinear，实际 %d", cfg.FusionMode)
	}
	if cfg.FTSWeight <= 0 || cfg.VectorWeight <= 0 {
		t.Errorf("权重应为正数：FTS=%f Vector=%f", cfg.FTSWeight, cfg.VectorWeight)
	}
	if cfg.RRFK != rrfK {
		t.Errorf("RRF k 应为 %d，实际 %d", rrfK, cfg.RRFK)
	}
}

// TestRAGStore_SetGetFusionConfig 验证配置动态调整。
func TestRAGStore_SetGetFusionConfig(t *testing.T) {
	mem := NewInMemoryStore()
	store := NewRAGStore(mem, &mockEmbeddingProvider{})

	cfg1 := store.GetFusionConfig()
	if cfg1.FusionMode != FusionLinear {
		t.Errorf("初始应为 FusionLinear")
	}

	newCfg := DefaultRAGFusionConfig()
	newCfg.FusionMode = FusionRRF
	newCfg.RRFK = 30
	store.SetFusionConfig(newCfg)

	cfg2 := store.GetFusionConfig()
	if cfg2.FusionMode != FusionRRF {
		t.Errorf("切换后应为 FusionRRF，实际 %d", cfg2.FusionMode)
	}
	if cfg2.RRFK != 30 {
		t.Errorf("RRFK 应为 30，实际 %d", cfg2.RRFK)
	}
}

// TestRAGStore_SetGetFusionConfig_Concurrent 验证并发读写配置安全。
func TestRAGStore_SetGetFusionConfig_Concurrent(t *testing.T) {
	mem := NewInMemoryStore()
	store := NewRAGStore(mem, &mockEmbeddingProvider{})

	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func(idx int) {
			for j := 0; j < 100; j++ {
				cfg := DefaultRAGFusionConfig()
				if j%2 == 0 {
					cfg.FusionMode = FusionRRF
				} else {
					cfg.FusionMode = FusionLinear
				}
				store.SetFusionConfig(cfg)
			}
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 20; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = store.GetFusionConfig()
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 40; i++ {
		<-done
	}
}

// TestHybridSearch_ModeSwitch 验证 Linear 与 RRF 模式切换。
// 使用 WithInMemory() 因为它支持 SQLite FTS 全文搜索。
func TestHybridSearch_ModeSwitch(t *testing.T) {
	mem, _ := WithInMemory()
	defer mem.Close()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		mem.Add(ctx, &Episode{
			ID:        string(rune('a' + i)),
			SessionID: "s1",
			Role:      "user",
			Content:   "test query about golang programming language",
		})
	}

	store := NewRAGStore(mem, &mockEmbeddingProvider{})

	cfg := DefaultRAGFusionConfig()
	cfg.FusionMode = FusionLinear
	store.SetFusionConfig(cfg)
	linearResults, err := store.HybridSearch(ctx, "golang", 3)
	if err != nil {
		t.Fatalf("Linear search failed: %v", err)
	}

	cfg.FusionMode = FusionRRF
	store.SetFusionConfig(cfg)
	rrfResults, err := store.HybridSearch(ctx, "golang", 3)
	if err != nil {
		t.Fatalf("RRF search failed: %v", err)
	}

	if len(linearResults) == 0 {
		t.Error("Linear 应返回结果")
	}
	if len(rrfResults) == 0 {
		t.Error("RRF 应返回结果")
	}
	if len(rrfResults) > 3 {
		t.Errorf("RRF 结果不应超过 topK=3，实际 %d", len(rrfResults))
	}
}

// TestHybridSearch_FTSSourceTags 验证 sources 字段标记正确。
func TestHybridSearch_FTSSourceTags(t *testing.T) {
	mem, _ := WithInMemory()
	defer mem.Close()
	ctx := context.Background()

	mem.Add(ctx, &Episode{
		ID:        "ep1",
		SessionID: "s1",
		Role:      "user",
		Content:   "Go is a statically typed, compiled programming language",
	})

	store := NewRAGStore(mem, nil)
	cfg := DefaultRAGFusionConfig()
	cfg.FusionMode = FusionLinear
	store.SetFusionConfig(cfg)
	results, err := store.HybridSearch(ctx, "Go programming", 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected results")
	}

	for _, r := range results {
		if len(r.Sources) == 0 {
			t.Errorf("result %s missing sources", r.Episode.ID)
		}
		hasFTS := false
		for _, s := range r.Sources {
			if s == "fts" {
				hasFTS = true
				break
			}
		}
		if !hasFTS {
			t.Errorf("result %s 应标记为 fts 源，实际 %v", r.Episode.ID, r.Sources)
		}
	}
}

// TestHybridSearchRRF_KBounds 验证 RRF k 参数边界。
func TestHybridSearchRRF_KBounds(t *testing.T) {
	mem, _ := WithInMemory()
	defer mem.Close()
	ctx := context.Background()

	mem.Add(ctx, &Episode{ID: "ep1", SessionID: "s1", Role: "user", Content: "test content for RRF bounds"})

	store := NewRAGStore(mem, nil)

	// k=0 应 fallback 到默认 rrfK
	cfg := DefaultRAGFusionConfig()
	cfg.FusionMode = FusionRRF
	cfg.RRFK = 0
	store.SetFusionConfig(cfg)
	results, err := store.HybridSearch(ctx, "test", 5)
	if err != nil {
		t.Fatalf("RRF with k=0 failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("RRF with k=0 should return fallback results")
	}

	// k=100 也应正常工作
	cfg.RRFK = 100
	store.SetFusionConfig(cfg)
	results2, err := store.HybridSearch(ctx, "test", 5)
	if err != nil {
		t.Fatalf("RRF with k=100 failed: %v", err)
	}
	if len(results2) == 0 {
		t.Error("RRF with k=100 should return results")
	}
}
