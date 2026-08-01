package memory

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"time"
)

const (
	defaultHalfLifeHours      = 720
	minRerankScore            = 0.01
	defaultDiversityThreshold = 0.85
	defaultMaxDuplicates      = 2
)

// ===== 重排序接口 =====

// Reranker 重排序器接口
type Reranker interface {
	// Rerank 对初始搜索结果重新排序，返回重排后的结果
	Rerank(ctx context.Context, query string, results []*RAGResult) ([]*RAGResult, error)
	Name() string
}

// ===== MMR (Maximal Marginal Relevance) 重排序 =====

// MMRReranker 使用最大边际相关性算法进行重排序
// 在相关性和多样性之间取得平衡：选择与查询相关但与已选结果不太相似的结果
type MMRReranker struct {
	Lambda     float64 // 相关性权重 [0, 1]，越高越偏向相关性；默认 0.7
	similarity func(a, b string) float32
	logger     *slog.Logger
}

// MMRConfig MMR 重排序配置
type MMRConfig struct {
	Lambda float64 // 相关性权重，默认 0.7
}

// NewMMRReranker 创建 MMR 重排序器
func NewMMRReranker(cfg MMRConfig) *MMRReranker {
	if cfg.Lambda <= 0 || cfg.Lambda > 1 {
		cfg.Lambda = 0.7
	}
	return &MMRReranker{
		Lambda:     cfg.Lambda,
		similarity: jaccardSimilarity,
		logger:     slog.Default(),
	}
}

// Name 返回重排序器名称
func (m *MMRReranker) Name() string { return "mmr" }

// Rerank 执行 MMR 重排序
func (m *MMRReranker) Rerank(_ context.Context, query string, results []*RAGResult) ([]*RAGResult, error) {
	n := len(results)
	if n <= 2 {
		return results, nil
	}

	selected := make([]*RAGResult, 0, n)
	remaining := make([]*RAGResult, n)
	copy(remaining, results)

	for len(remaining) > 0 && len(selected) < n {
		bestIdx := -1
		bestScore := float64(-1)

		for i, item := range remaining {
			relevancePart := m.Lambda * float64(item.Score)

			if len(selected) == 0 {
				if relevancePart > bestScore {
					bestScore = relevancePart
					bestIdx = i
				}
				continue
			}

			maxSim := float64(0)
			for _, sel := range selected {
				sim := float64(m.similarity(item.Episode.Content, sel.Episode.Content))
				if sim > maxSim {
					maxSim = sim
				}
			}

			diversityPart := (1 - m.Lambda) * maxSim
			mmrScore := relevancePart - diversityPart

			if mmrScore > bestScore {
				bestScore = mmrScore
				bestIdx = i
			}
		}

		if bestIdx >= 0 {
			selected = append(selected, remaining[bestIdx])
			remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
		} else {
			break
		}
	}

	selected = append(selected, remaining...)
	return selected, nil
}

// jaccardSimilarity 计算 Jaccard 相似度（基于字符级 shingle）
func jaccardSimilarity(a, b string) float32 {
	const shingleSize = 3

	setA := shingleSet(a, shingleSize)
	setB := shingleSet(b, shingleSize)

	if len(setA) == 0 && len(setB) == 0 {
		return 1.0
	}

	intersection := 0
	union := make(map[string]bool)
	for k := range setA {
		union[k] = true
		if setB[k] {
			intersection++
		}
	}
	for k := range setB {
		union[k] = true
	}

	if len(union) == 0 {
		return 0
	}
	return float32(intersection) / float32(len(union))
}

func shingleSet(text string, size int) map[string]bool {
	runes := []rune(text)
	result := make(map[string]bool)
	for i := 0; i+size <= len(runes); i++ {
		result[string(runes[i:i+size])] = true
	}
	return result
}

// ===== 分数融合重排序 =====

// ScoreFusionReranker 基于多信号分数融合的重排序器
type ScoreFusionReranker struct {
	Weights    ScoreWeights // 各维度权重
	Normalizer ScoreNormalizer
	logger     *slog.Logger
}

// ScoreWeights 融合权重配置
type ScoreWeights struct {
	Relevance  float64 // 初始检索相关性权重
	Recency    float64 // 时间新鲜度权重（新内容优先）
	Importance float64 // 重要性评分权重
	Diversity  float64 // 多样性惩罚权重
	Position   float64 // 原始位置偏置权重（靠前的略优）
}

// DefaultScoreWeights 默认融合权重
func DefaultScoreWeights() ScoreWeights {
	return ScoreWeights{
		Relevance:  0.5,
		Recency:    0.15,
		Importance: 0.2,
		Diversity:  0.1,
		Position:   0.05,
	}
}

// ScoreNormalizer 分数归一化接口
type ScoreNormalizer interface {
	Normalize(score float64, min, max float64) float64
}

// MinMaxNormalizer Min-Max 归一化
type MinMaxNormalizer struct{}

func (n *MinMaxNormalizer) Normalize(score, min, max float64) float64 {
	if max == min {
		return 0.5
	}
	return (score - min) / (max - min)
}

// FusionConfig 融合重排序配置
type FusionConfig struct {
	Weights    ScoreWeights
	Normalizer ScoreNormalizer
}

// NewScoreFusionReranker 创建分数融合重排序器
func NewScoreFusionReranker(cfg FusionConfig) *ScoreFusionReranker {
	if cfg.Weights.Relevance == 0 {
		cfg.Weights = DefaultScoreWeights()
	}
	if cfg.Normalizer == nil {
		cfg.Normalizer = &MinMaxNormalizer{}
	}
	return &ScoreFusionReranker{
		Weights:    cfg.Weights,
		Normalizer: cfg.Normalizer,
		logger:     slog.Default(),
	}
}

// Name 返回名称
func (f *ScoreFusionReranker) Name() string { return "score_fusion" }

// Rerank 执行分数融合重排序
func (f *ScoreFusionReranker) Rerank(_ context.Context, _ string, results []*RAGResult) ([]*RAGResult, error) {
	n := len(results)
	if n <= 1 {
		return results, nil
	}

	scored := make([]fusionItem, n)

	maxScore := float64(0)
	maxImportance := float64(0)
	for i, r := range results {
		if float64(r.Score) > maxScore {
			maxScore = float64(r.Score)
		}
		if r.Episode.Importance > maxImportance {
			maxImportance = r.Episode.Importance
		}
		scored[i] = fusionItem{result: r, origIndex: i}
	}

	for i := range scored {
		item := &scored[i]

		normRelevance := f.Normalizer.Normalize(float64(item.result.Score), 0, maxScore)
		normImportance := f.Normalizer.Normalize(float64(item.result.Episode.Importance), 0, maxImportance)
		normPosition := f.Normalizer.Normalize(float64(n-1-item.origIndex), 0, float64(n-1))

		recencyScore := recencyScore(item.result.Episode.CreatedAt)

		item.fusedScore = f.Weights.Relevance*normRelevance +
			f.Weights.Recency*recencyScore +
			f.Weights.Importance*normImportance +
			f.Weights.Position*normPosition

		item.diversityPenalty = f.computeDiversityPenalty(scored[:i], item.result)
		item.fusedScore -= f.Weights.Diversity * item.diversityPenalty
	}

	// 优化（Task 19）：使用泛型 slices.SortFunc 替代 sort.Slice，避免反射开销
	slices.SortFunc(scored, func(a, b fusionItem) int { return cmp.Compare(b.fusedScore, a.fusedScore) })

	reranked := make([]*RAGResult, n)
	for i, item := range scored {
		reranked[i] = item.result
	}
	return reranked, nil
}

func (f *ScoreFusionReranker) computeDiversityPenalty(previous []fusionItem, current *RAGResult) float64 {
	if len(previous) == 0 {
		return 0
	}
	maxSim := float64(0)
	for _, prev := range previous {
		sim := float64(jaccardSimilarity(current.Episode.Content, prev.result.Episode.Content))
		if sim > maxSim {
			maxSim = sim
		}
	}
	return maxSim
}

type fusionItem struct {
	result           *RAGResult
	origIndex        int
	fusedScore       float64
	diversityPenalty float64
}

// recencyScore 根据时间戳计算新鲜度分数（越新越高）
func recencyScore(timestamp string) float64 {
	if timestamp == "" {
		return 0.5
	}
	const halfLifeHours = defaultHalfLifeHours // 30 天半衰期
	age := timeSinceHours(timestamp)
	if age < 0 {
		return 0.5
	}
	score := math.Pow(0.5, age/float64(halfLifeHours))
	if score < minRerankScore {
		return minRerankScore
	}
	return score
}

// ===== 多样性重排序 =====

// DiversityReranker 确保结果多样性的重排序器
// 通过聚类或贪心选择避免返回过于相似的结果
type DiversityReranker struct {
	SimThreshold  float32 // 相似度阈值，超过则认为重复
	MaxDuplicates int     // 允许的最大相似结果数
	logger        *slog.Logger
}

// DiversityConfig 多样性重排序配置
type DiversityConfig struct {
	SimThreshold  float32
	MaxDuplicates int
}

// NewDiversityReranker 创建多样性重排序器
func NewDiversityReranker(cfg DiversityConfig) *DiversityReranker {
	if cfg.SimThreshold <= 0 {
		cfg.SimThreshold = defaultDiversityThreshold
	}
	if cfg.MaxDuplicates <= 0 {
		cfg.MaxDuplicates = defaultMaxDuplicates
	}
	return &DiversityReranker{
		SimThreshold:  cfg.SimThreshold,
		MaxDuplicates: cfg.MaxDuplicates,
		logger:        slog.Default(),
	}
}

// Name 返回名称
func (d *DiversityReranker) Name() string { return "diversity" }

// Rerank 执行多样性重排序
func (d *DiversityReranker) Rerank(_ context.Context, _ string, results []*RAGResult) ([]*RAGResult, error) {
	if len(results) <= d.MaxDuplicates {
		return results, nil
	}

	var deduped []*RAGResult
	groupCounts := make(map[int]int)

	for _, result := range results {
		foundGroup := -1
		for gi, existing := range deduped {
			sim := jaccardSimilarity(result.Episode.Content, existing.Episode.Content)
			if sim >= d.SimThreshold {
				foundGroup = gi
				break
			}
		}

		if foundGroup >= 0 && groupCounts[foundGroup] >= d.MaxDuplicates {
			continue
		}

		if foundGroup >= 0 {
			groupCounts[foundGroup]++
		} else {
			foundGroup = len(deduped)
			groupCounts[foundGroup] = 1
		}

		deduped = append(deduped, result)
	}

	return deduped, nil
}

// ===== 链式重排序 =====

// ChainedReranker 将多个重排序器串联执行
type ChainedReranker struct {
	rerankers []Reranker
}

// NewChainedReranker 创建链式重排序器
func NewChainedReranker(rerankers ...Reranker) *ChainedReranker {
	return &ChainedReranker{rerankers: rerankers}
}

// Name 返回名称
func (c *ChainedReranker) Name() string {
	names := make([]string, len(c.rerankers))
	for i, r := range c.rerankers {
		names[i] = r.Name()
	}
	return fmt.Sprintf("chained(%v)", names)
}

// Rerank 依次执行每个重排序器
func (c *ChainedReranker) Rerank(ctx context.Context, query string, results []*RAGResult) ([]*RAGResult, error) {
	current := results
	for _, reranker := range c.rerankers {
		var err error
		current, err = reranker.Rerank(ctx, query, current)
		if err != nil {
			return nil, fmt.Errorf("reranker %q execution failed: %w", reranker.Name(), err)
		}
	}
	return current, nil
}

// timeSinceHours 计算时间戳距今的小时数
func timeSinceHours(timestamp string) float64 {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return -1
	}
	return time.Since(t).Hours()
}
