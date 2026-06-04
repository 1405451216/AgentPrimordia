package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
)

const (
	ftsBaseWeight        float32 = 0.7
	ftsDecayFactor       float32 = 0.05
	ftsMinScore          float32 = 0.3
	hybridExistingWeight float32 = 0.4
	hybridNewWeight      float32 = 0.6
	ragContextHeader             = "=== 相关记忆 ===\n"
	ragContextFooter             = "=== 记忆结束 ===\n"
)

// EmbeddingProvider 是向量化接口，由 LLM Provider 实现
type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}

// RAGStore 封装了 Memory + VectorStore + EmbeddingProvider，提供 RAG 检索能力
type RAGStore struct {
	memory   Memory
	vectors  *VectorStore
	embedder EmbeddingProvider
	logger   *slog.Logger
}

// NewRAGStore 创建 RAG 存储实例
func NewRAGStore(memory Memory, embedder EmbeddingProvider) *RAGStore {
	dim := defaultVectorDim
	if embedder != nil {
		dim = embedder.Dimensions()
	}
	return &RAGStore{
		memory:   memory,
		vectors:  NewVectorStore(dim),
		embedder: embedder,
		logger:   slog.Default(),
	}
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
func (r *RAGStore) HybridSearch(ctx context.Context, query string, topK int) ([]*RAGResult, error) {
	// 1. FTS 全文搜索
	ftsResults, err := r.memory.Search(ctx, query, &SearchOptions{Limit: topK})
	if err != nil {
		ftsResults = []*Episode{} // FTS 失败不影响向量搜索
	}

	// 构建结果 map 用于去重
	resultMap := make(map[string]*RAGResult)

	for i, ep := range ftsResults {
		// Use decreasing score based on rank position for FTS results
		// FTS results are already ordered by relevance from SQLite FTS5 rank
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

	// 2. 向量相似度搜索（如果 embedder 可用）
	if r.embedder != nil {
		vecResults, err := r.Query(ctx, query, topK)
		if err == nil {
			for _, vr := range vecResults {
				if existing, ok := resultMap[vr.Episode.ID]; ok {
					// 同时被 FTS 和向量命中的结果，加权融合
					existing.Score = existing.Score*hybridExistingWeight + vr.Score*hybridNewWeight
					existing.Sources = append(existing.Sources, "vector")
				} else {
					vr.Score = vr.Score * hybridNewWeight
					vr.Sources = []string{"vector"}
					resultMap[vr.Episode.ID] = vr
				}
			}
		}
	}

	// 转为切片并排序
	results := make([]*RAGResult, 0, len(resultMap))
	for _, r := range resultMap {
		results = append(results, r)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if topK < len(results) {
		results = results[:topK]
	}

	return results, nil
}

// GetMemory 返回底层 Memory 接口
func (r *RAGStore) GetMemory() Memory {
	return r.memory
}

// GetVectors 返回底层 VectorStore
func (r *RAGStore) GetVectors() *VectorStore {
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
