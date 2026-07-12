package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
)

const (
	ftsBaseWeight        float32 = 0.7
	ftsDecayFactor       float32 = 0.05
	ftsMinScore          float32 = 0.3
	hybridExistingWeight float32 = 0.4
	hybridNewWeight      float32 = 0.6
	ragContextHeader             = "=== 相关记忆 ===\n"
	ragContextFooter             = "=== 记忆结束 ===\n"
	// RRF k 常数（perf-v11 stage-4）：Reciprocal Rank Fusion 算法的平滑参数
	// 经验值 60 来自原始 RRF 论文（Cormack et al., 2009），平衡高低排名结果的权重
	rrfK = 60
)

// HybridFusionMode 混合检索融合模式（perf-v11 stage-4）
// 不同业务场景下，最优融合策略不同：
//   - Linear: 原始线性加权，简单但易受量纲影响
//   - RRF: Reciprocal Rank Fusion，基于排名而非分数，鲁棒性更强
type HybridFusionMode int

const (
	// FusionLinear 线性加权融合（向后兼容默认）
	FusionLinear HybridFusionMode = iota
	// FusionRRF Reciprocal Rank Fusion（推荐用于生产）
	FusionRRF
)

// RAGFusionConfig RAG 检索融合配置（perf-v11 stage-4：支持运行时调参）
// 注意：避免与 rag_generator.go 中的 RAGConfig 同名，特意加上 Fusion 后缀。
// 字段可通过 NewRAGStoreWithFusionConfig 或 RAGStore.SetFusionConfig 调整。
type RAGFusionConfig struct {
	// FusionMode 融合模式（Linear / RRF）
	FusionMode HybridFusionMode
	// FTSWeight FTS 通道权重（仅 Linear 模式生效）
	FTSWeight float32
	// VectorWeight 向量通道权重（仅 Linear 模式生效）
	VectorWeight float32
	// RRFK RRF 平滑常数（仅 RRF 模式生效，默认 60）
	RRFK int
	// OverFetchSize 单通道预取数量，用于增加融合召回率
	// 最终 topK = min(topK + OverFetchSize, 2*topK)
	OverFetchSize int
}

// DefaultRAGFusionConfig 返回默认融合配置
func DefaultRAGFusionConfig() RAGFusionConfig {
	return RAGFusionConfig{
		FusionMode:    FusionLinear,
		FTSWeight:     hybridExistingWeight, // 0.4
		VectorWeight:  hybridNewWeight,      // 0.6
		RRFK:          rrfK,
		OverFetchSize: 5,
	}
}

// EmbeddingProvider 是向量化接口，由 LLM Provider 实现
type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}

// RAGStore 封装了 Memory + VectorStore + EmbeddingProvider，提供 RAG 检索能力
type RAGStore struct {
	memory   Memory
	vectors  *SimpleVectorStore
	embedder EmbeddingProvider
	logger   *slog.Logger
	config   RAGFusionConfig // perf-v11 stage-4：支持运行时调整融合策略
	configMu sync.Mutex      // 保护 config 并发读写
}

// NewRAGStore 创建 RAG 存储实例
func NewRAGStore(memory Memory, embedder EmbeddingProvider) *RAGStore {
	return NewRAGStoreWithFusionConfig(memory, embedder, DefaultRAGFusionConfig())
}

// NewRAGStoreWithFusionConfig 创建带自定义配置的 RAG 存储实例（perf-v11 stage-4）
func NewRAGStoreWithFusionConfig(memory Memory, embedder EmbeddingProvider, cfg RAGFusionConfig) *RAGStore {
	dim := defaultVectorDim
	if embedder != nil {
		dim = embedder.Dimensions()
	}
	return &RAGStore{
		memory:   memory,
		vectors:  NewVectorStore(dim),
		embedder: embedder,
		logger:   slog.Default(),
		config:   cfg,
	}
}

// SetFusionConfig 动态调整 RAG 检索配置（perf-v11 stage-4）
// 线程安全：通过 RAGStore.configMu 互斥锁保护
// 典型用法：根据 A/B 测试结果调整融合权重，或在 QPS 下降时切换到 RRF 模式
func (r *RAGStore) SetFusionConfig(cfg RAGFusionConfig) {
	r.configMu.Lock()
	defer r.configMu.Unlock()
	r.config = cfg
}

// GetFusionConfig 获取当前 RAG 检索配置
func (r *RAGStore) GetFusionConfig() RAGFusionConfig {
	r.configMu.Lock()
	defer r.configMu.Unlock()
	return r.config
}

// Add 添加 Episode 到 Memory，并同时生成向量索引
func (r *RAGStore) Add(ctx context.Context, episode *Episode) error {
	if err := r.memory.Add(ctx, episode); err != nil {
		return err
	}

	// 同步生成 embedding 并存入向量索引
	if r.embedder != nil {
		text := episode.Content
		if episode.Summary != "" {
			text = episode.Summary
		}
		vecs, err := r.embedder.Embed(ctx, []string{text})
		if err != nil {
			r.logger.Warn("RAG embedding 生成失败", "id", episode.ID, "error", err)
			return nil // Memory 已保存成功，embedding 失败不阻断
		}
		if len(vecs) > 0 {
			metadata := map[string]string{
				"session_id": episode.SessionID,
				"role":       episode.Role,
			}
			if err := r.vectors.Add(ctx, episode.ID, vecs[0], metadata); err != nil {
				r.logger.Warn("RAG 向量存储失败", "id", episode.ID, "error", err)
			}
		}
	}

	return nil
}

// Query 执行 RAG 查询：将查询向量化，然后在向量索引中搜索最相似的 Episode
func (r *RAGStore) Query(ctx context.Context, query string, topK int) ([]*RAGResult, error) {
	if r.embedder == nil {
		return nil, fmt.Errorf("RAG 查询需要 EmbeddingProvider")
	}

	if topK <= 0 {
		topK = 5
	}

	// 生成查询向量
	vecs, err := r.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("查询向量化失败: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("查询向量化返回空结果")
	}

	// 向量相似度搜索
	results, err := r.vectors.Search(ctx, vecs[0], topK)
	if err != nil {
		return nil, fmt.Errorf("向量搜索失败: %w", err)
	}

	// 将搜索结果与 Memory 中的 Episode 关联
	ragResults := make([]*RAGResult, 0, len(results))
	for _, sr := range results {
		episode, err := r.memory.Get(ctx, sr.ID)
		if err != nil {
			// 向量索引中的条目可能已被删除
			continue
		}
		ragResults = append(ragResults, &RAGResult{
			Episode: episode,
			Score:   sr.Score,
		})
	}

	// 按相关度降序排列
	sort.Slice(ragResults, func(i, j int) bool {
		return ragResults[i].Score > ragResults[j].Score
	})

	return ragResults, nil
}

// HybridSearch 混合检索：结合 FTS 全文搜索和向量相似度搜索
// 融合策略由 RAGStore.config.FusionMode 决定（perf-v11 stage-4）：
//   - FusionLinear: 线性加权融合（向后兼容）
//   - FusionRRF: Reciprocal Rank Fusion（推荐用于生产）
func (r *RAGStore) HybridSearch(ctx context.Context, query string, topK int) ([]*RAGResult, error) {
	cfg := r.GetFusionConfig()
	// 预取更多候选以提升融合召回率（over-fetch 然后重排）
	fetchK := topK + cfg.OverFetchSize
	if fetchK > 2*topK {
		fetchK = 2 * topK
	}

	// 1. FTS 全文搜索
	ftsResults, err := r.memory.Search(ctx, query, &SearchOptions{Limit: fetchK})
	if err != nil {
		ftsResults = []*Episode{} // FTS 失败不影响向量搜索
	}

	if cfg.FusionMode == FusionRRF {
		return r.hybridSearchRRF(ctx, query, ftsResults, topK, cfg)
	}
	return r.hybridSearchLinear(ctx, query, ftsResults, topK, cfg)
}

// hybridSearchLinear 线性加权融合（perf-v11 stage-4：原 HybridSearch 逻辑）
// FTS 通道和向量通道分别计算分数，按权重相加。
// 缺点：受量纲影响，FTS 分数范围 [-1, 0] 与向量余弦相似度 [0, 1] 不可比。
func (r *RAGStore) hybridSearchLinear(ctx context.Context, query string, ftsResults []*Episode, topK int, cfg RAGFusionConfig) ([]*RAGResult, error) {
	resultMap := make(map[string]*RAGResult)

	for i, ep := range ftsResults {
		ftsScore := ftsBaseWeight * (1.0 - float32(i)*ftsDecayFactor)
		if ftsScore < ftsMinScore {
			ftsScore = ftsMinScore
		}
		resultMap[ep.ID] = &RAGResult{
			Episode: ep,
			Score:   ftsScore,
			Sources: []string{"fts"},
		}
	}

	if r.embedder != nil {
		fetchK := topK + cfg.OverFetchSize
		if fetchK > 2*topK {
			fetchK = 2 * topK
		}
		vecResults, err := r.Query(ctx, query, fetchK)
		if err == nil {
			for _, vr := range vecResults {
				if existing, ok := resultMap[vr.Episode.ID]; ok {
					// 加权融合
					existing.Score = existing.Score*cfg.FTSWeight + vr.Score*cfg.VectorWeight
					existing.Sources = append(existing.Sources, "vector")
				} else {
					vr.Score = vr.Score * cfg.VectorWeight
					vr.Sources = []string{"vector"}
					resultMap[vr.Episode.ID] = vr
				}
			}
		}
	}

	results := make([]*RAGResult, 0, len(resultMap))
	for _, v := range resultMap {
		results = append(results, v)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

// hybridSearchRRF Reciprocal Rank Fusion 融合算法（perf-v11 stage-4）
// 公式：RRF_score(d) = Σ 1 / (k + rank_i(d))
// 其中 rank_i(d) 是文档 d 在第 i 个通道中的排名（1-based），k 为平滑常数（默认 60）
// 优势：基于排名而非分数，不受通道量纲影响，对长尾 query 鲁棒性更强。
// 论文：Cormack, G. V., Clarke, C. L., & Buettcher, S. (2009).
// "Reciprocal rank fusion outperforms condorcet and individual rank learning methods."
func (r *RAGStore) hybridSearchRRF(ctx context.Context, query string, ftsResults []*Episode, topK int, cfg RAGFusionConfig) ([]*RAGResult, error) {
	k := float32(cfg.RRFK)
	if k <= 0 {
		k = float32(rrfK) // fallback 默认
	}

	// 记录每个文档在各通道的排名（1-based），用于独立累加 RRF 分数
	ftsRanks := make(map[string]int) // FTS 通道排名
	vecRanks := make(map[string]int) // 向量通道排名
	episodes := make(map[string]*Episode)
	sources := make(map[string][]string)

	// FTS 通道排名
	for i, ep := range ftsResults {
		rank := i + 1
		if existing, ok := ftsRanks[ep.ID]; !ok || rank < existing {
			ftsRanks[ep.ID] = rank
		}
		episodes[ep.ID] = ep
		sources[ep.ID] = append(sources[ep.ID], "fts")
	}

	// 向量通道排名
	if r.embedder != nil {
		fetchK := topK + cfg.OverFetchSize
		if fetchK > 2*topK {
			fetchK = 2 * topK
		}
		vecResults, err := r.Query(ctx, query, fetchK)
		if err == nil {
			for i, vr := range vecResults {
				rank := i + 1
				if existing, ok := vecRanks[vr.Episode.ID]; !ok || rank < existing {
					vecRanks[vr.Episode.ID] = rank
				}
				if _, ok := episodes[vr.Episode.ID]; !ok {
					episodes[vr.Episode.ID] = vr.Episode
				}
				// 去重 sources
				hasVec := false
				for _, s := range sources[vr.Episode.ID] {
					if s == "vector" {
						hasVec = true
						break
					}
				}
				if !hasVec {
					sources[vr.Episode.ID] = append(sources[vr.Episode.ID], "vector")
				}
			}
		}
	}

	// 计算 RRF 分数并构造结果
	// 公式：RRF_score(d) = Σ 1 / (k + rank_i(d))，对每个命中通道独立累加
	results := make([]*RAGResult, 0, len(episodes))
	for id, ep := range episodes {
		rrfScore := float32(0)
		if rank, ok := ftsRanks[id]; ok {
			rrfScore += 1.0 / (k + float32(rank))
		}
		if rank, ok := vecRanks[id]; ok {
			rrfScore += 1.0 / (k + float32(rank))
		}
		results = append(results, &RAGResult{
			Episode: ep,
			Score:   rrfScore,
			Sources: sources[id],
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

// GetMemory 返回底层 Memory 接口
func (r *RAGStore) GetMemory() Memory {
	return r.memory
}

// GetVectors 返回底层 VectorStore
func (r *RAGStore) GetVectors() *SimpleVectorStore {
	return r.vectors
}

// RAGResult 是 RAG 查询的结果
type RAGResult struct {
	Episode *Episode `json:"episode"`
	Score   float32  `json:"score"`
	Sources []string `json:"sources,omitempty"` // "fts" 和/或 "vector"
}

// ContextForPrompt 将 RAG 结果格式化为可注入 Prompt 的上下文文本
func (r *RAGResult) ContextForPrompt() string {
	return fmt.Sprintf("[相关记忆 | 相关度: %.2f] %s: %s", r.Score, r.Episode.Role, r.Episode.Content)
}

// FormatRAGContext 批量格式化 RAG 结果为 Prompt 上下文
func FormatRAGContext(results []*RAGResult) string {
	if len(results) == 0 {
		return ""
	}
	context := ragContextHeader
	for _, r := range results {
		context += r.ContextForPrompt() + "\n"
	}
	context += ragContextFooter
	return context
}
