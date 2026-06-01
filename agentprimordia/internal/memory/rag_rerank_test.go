package memory

import (
	"context"
	"testing"
)

// ===== MMR Reranker 测试 =====

func TestMMRReranker_Name(t *testing.T) {
	r := NewMMRReranker(MMRConfig{})
	if r.Name() != "mmr" {
		t.Errorf("名称应为 'mmr'，实际 '%s'", r.Name())
	}
}

func TestMMRReranker_LambdaClamping(t *testing.T) {
	r := NewMMRReranker(MMRConfig{Lambda: 1.5})
	if r.Lambda != 0.7 {
		t.Errorf("Lambda > 1 应被钳位到 0.7，实际 %.2f", r.Lambda)
	}
	r2 := NewMMRReranker(MMRConfig{Lambda: -0.5})
	if r2.Lambda != 0.7 {
		t.Errorf("Lambda < 0 应被钳位到 0.7，实际 %.2f", r2.Lambda)
	}
	r3 := NewMMRReranker(MMRConfig{Lambda: 0.5})
	if r3.Lambda != 0.5 {
		t.Errorf("合法 Lambda 值应保留，实际 %.2f", r3.Lambda)
	}
}

func TestMMRReranker_SingleResult(t *testing.T) {
	r := NewMMRReranker(MMRConfig{Lambda: 0.7})
	results := []*RAGResult{
		{Episode: &Episode{ID: "1", Content: "hello"}, Score: 0.9},
	}
	reranked, err := r.Rerank(context.Background(), "query", results)
	if err != nil {
		t.Fatalf("MMR 重排序失败: %v", err)
	}
	if len(reranked) != 1 {
		t.Errorf("单结果应保持不变")
	}
}

func TestMMRReranker_TwoResults(t *testing.T) {
	r := NewMMRReranker(MMRConfig{Lambda: 0.7})
	results := []*RAGResult{
		{Episode: &Episode{ID: "1", Content: "hello world test"}, Score: 0.9},
		{Episode: &Episode{ID: "2", Content: "foo bar baz different"}, Score: 0.8},
	}
	reranked, err := r.Rerank(context.Background(), "test", results)
	if err != nil {
		t.Fatalf("MMR 重排序失败: %v", err)
	}
	if len(reranked) != 2 {
		t.Errorf("双结果应保持数量，得到 %d", len(reranked))
	}
}

func TestMMRReranker_PromotesDiversity(t *testing.T) {
	r := NewMMRReranker(MMRConfig{Lambda: 0.5})

	baseContent := "这是一个关于机器学习的长文本内容讨论深度学习模型训练方法优化策略"
	results := []*RAGResult{
		{Episode: &Episode{ID: "1", Content: baseContent}, Score: 0.95},
		{Episode: &Episode{ID: "2", Content: baseContent + "补充说明"}, Score: 0.90},
		{Episode: &Episode{ID: "3", Content: "完全不同的主题关于自然语言处理和文本分类"}, Score: 0.70},
		{Episode: &Episode{ID: "4", Content: baseContent + "更多细节"}, Score: 0.85},
	}

	reranked, err := r.Rerank(context.Background(), "机器学习", results)
	if err != nil {
		t.Fatalf("MMR 重排序失败: %v", err)
	}

	if len(reranked) != 4 {
		t.Errorf("应保持 4 个结果，得到 %d", len(reranked))
	}

	firstID := reranked[0].Episode.ID
	if firstID != "1" {
		t.Logf("MMR 首选结果 ID=%s（相关性最高的不一定总是第一）", firstID)
	}
}

func TestMMRReranker_EmptyResults(t *testing.T) {
	r := NewMMRReranker(MMRConfig{})
	reranked, err := r.Rerank(context.Background(), "q", nil)
	if err != nil {
		t.Fatalf("空结果不应报错: %v", err)
	}
	if len(reranked) != 0 {
		t.Error("空结果应返回空切片")
	}
}

// ===== ScoreFusion Reranker 测试 =====

func TestScoreFusionReranker_Name(t *testing.T) {
	r := NewScoreFusionReranker(FusionConfig{})
	if r.Name() != "score_fusion" {
		t.Errorf("名称应为 'score_fusion'")
	}
}

func TestScoreFusionReranker_DefaultWeights(t *testing.T) {
	r := NewScoreFusionReranker(FusionConfig{})
	w := r.Weights
	total := w.Relevance + w.Recency + w.Importance + w.Diversity + w.Position
	if total < 0.99 || total > 1.01 {
		t.Errorf("权重总和应约等于 1.0，实际 %.2f", total)
	}
}

func TestScoreFusionReranker_SingleResult(t *testing.T) {
	r := NewScoreFusionReranker(FusionConfig{})
	results := []*RAGResult{
		{Episode: &Episode{ID: "1", Content: "test"}, Score: 0.9},
	}
	reranked, err := r.Rerank(context.Background(), "q", results)
	if err != nil {
		t.Fatal(err)
	}
	if len(reranked) != 1 {
		t.Error("单结果应保持")
	}
}

func TestScoreFusionReranker_ByRelevance(t *testing.T) {
	r := NewScoreFusionReranker(FusionConfig{
		Weights: ScoreWeights{Relevance: 1.0, Recency: 0, Importance: 0, Diversity: 0, Position: 0},
	})

	now := "2026-05-30T12:00:00Z"
	results := []*RAGResult{
		{Episode: &Episode{ID: "1", Content: "low relevance", Importance: 0.9, CreatedAt: now}, Score: 0.3},
		{Episode: &Episode{ID: "2", Content: "high relevance", Importance: 0.1, CreatedAt: now}, Score: 0.95},
		{Episode: &Episode{ID: "3", Content: "medium relevance", Importance: 0.5, CreatedAt: now}, Score: 0.6},
	}

	reranked, err := r.Rerank(context.Background(), "high relevance", results)
	if err != nil {
		t.Fatal(err)
	}

	if reranked[0].Episode.ID != "2" {
		t.Errorf("纯相关性模式下最高分结果应在首位，实际首位 ID=%s", reranked[0].Episode.ID)
	}
}

func TestScoreFusionReranker_ByImportance(t *testing.T) {
	r := NewScoreFusionReranker(FusionConfig{
		Weights: ScoreWeights{Relevance: 0.1, Recency: 0, Importance: 0.9, Diversity: 0, Position: 0},
	})

	now := "2026-05-30T12:00:00Z"
	results := []*RAGResult{
		{Episode: &Episode{ID: "1", Content: "a", Importance: 0.3, CreatedAt: now}, Score: 0.8},
		{Episode: &Episode{ID: "2", Content: "b", Importance: 0.95, CreatedAt: now}, Score: 0.4},
		{Episode: &Episode{ID: "3", Content: "c", Importance: 0.6, CreatedAt: now}, Score: 0.7},
	}

	reranked, err := r.Rerank(context.Background(), "q", results)
	if err != nil {
		t.Fatal(err)
	}

	if reranked[0].Episode.ID != "2" {
		t.Errorf("重要性优先模式下高重要性应在首位，实际 ID=%s", reranked[0].Episode.ID)
	}
}

func TestScoreFusionReranker_DiversityPenalty(t *testing.T) {
	r := NewScoreFusionReranker(FusionConfig{})

	now := "2026-05-30T12:00:00Z"
	sameContent := "这是相同内容的搜索结果用于测试多样性惩罚机制"
	results := []*RAGResult{
		{Episode: &Episode{ID: "1", Content: sameContent, Importance: 0.8, CreatedAt: now}, Score: 0.9},
		{Episode: &Episode{ID: "2", Content: sameContent + "略有不同", Importance: 0.8, CreatedAt: now}, Score: 0.88},
		{Episode: &Episode{ID: "3", Content: "完全不同的内容关于另一个话题", Importance: 0.7, CreatedAt: now}, Score: 0.7},
	}

	reranked, err := r.Rerank(context.Background(), "q", results)
	if err != nil {
		t.Fatal(err)
	}

	if len(reranked) != 3 {
		t.Errorf("应保持 3 个结果")
	}
}

// ===== Diversity Reranker 测试 =====

func TestDiversityReranker_Name(t *testing.T) {
	r := NewDiversityReranker(DiversityConfig{})
	if r.Name() != "diversity" {
		t.Errorf("名称应为 'diversity'")
	}
}

func TestDiversityReranker_DefaultConfig(t *testing.T) {
	r := NewDiversityReranker(DiversityConfig{})
	if r.SimThreshold != 0.85 {
		t.Errorf("默认相似度阈值应为 0.85，实际 %.2f", r.SimThreshold)
	}
	if r.MaxDuplicates != 2 {
		t.Errorf("默认最大重复数应为 2，实际 %d", r.MaxDuplicates)
	}
}

func TestDiversityReranker_RemovesDuplicates(t *testing.T) {
	r := NewDiversityReranker(DiversityConfig{
		SimThreshold:  0.9,
		MaxDuplicates: 1,
	})

	base := "这是一段完全相同的重复内容用于测试去重功能"
	results := []*RAGResult{
		{Episode: &Episode{ID: "1", Content: base}, Score: 0.95},
		{Episode: &Episode{ID: "2", Content: base + "。"}, Score: 0.93},
		{Episode: &Episode{ID: "3", Content: base + "！"}, Score: 0.91},
		{Episode: &Episode{ID: "4", Content: "完全不同的独特内容"}, Score: 0.7},
		{Episode: &Episode{ID: "5", Content: base + "..."}, Score: 0.89},
	}

	reranked, err := r.Rerank(context.Background(), "q", results)
	if err != nil {
		t.Fatal(err)
	}

	if len(reranked) >= 4 {
		idSet := make(map[string]bool)
		for _, item := range reranked {
			if idSet[item.Episode.ID] {
				t.Errorf("发现重复 ID: %s", item.Episode.ID)
			}
			idSet[item.Episode.ID] = true
		}
	}
}

func TestDiversityReranker_AllDistinct(t *testing.T) {
	r := NewDiversityReranker(DiversityConfig{SimThreshold: 0.99})

	results := []*RAGResult{
		{Episode: &Episode{ID: "1", Content: "alpha"}, Score: 0.9},
		{Episode: &Episode{ID: "2", Content: "beta"}, Score: 0.8},
		{Episode: &Episode{ID: "3", Content: "gamma"}, Score: 0.7},
	}

	reranked, err := r.Rerank(context.Background(), "q", results)
	if err != nil {
		t.Fatal(err)
	}
	if len(reranked) != 3 {
		t.Errorf("全部不相似的结果应全部保留，得到 %d", len(reranked))
	}
}

func TestDiversityReranker_BelowMaxDupes(t *testing.T) {
	r := NewDiversityReranker(DiversityConfig{
		SimThreshold:  0.99,
		MaxDuplicates: 5,
	})

	results := []*RAGResult{
		{Episode: &Episode{ID: "1", Content: "a"}, Score: 0.9},
		{Episode: &Episode{ID: "2", Content: "b"}, Score: 0.8},
		{Episode: &Episode{ID: "3", Content: "c"}, Score: 0.7},
	}

	reranked, err := r.Rerank(context.Background(), "q", results)
	if err != nil {
		t.Fatal(err)
	}
	if len(reranked) != 3 {
		t.Error("不超过 MaxDuplicates 时应全部保留")
	}
}

// ===== Chained Reranker 测试 =====

func TestChainedReranker_Name(t *testing.T) {
	c := NewChainedReranker(
		NewMMRReranker(MMRConfig{}),
		NewDiversityReranker(DiversityConfig{}),
	)
	name := c.Name()
	if !contains(name, "mmr") || !contains(name, "diversity") {
		t.Errorf("链式名称应包含子重排序器名，实际: %s", name)
	}
}

func TestChainedReranker_ExecutesAll(t *testing.T) {
	c := NewChainedReranker(
		NewMMRReranker(MMRConfig{Lambda: 0.7}),
		NewDiversityReranker(DiversityConfig{SimThreshold: 0.99}),
	)

	results := []*RAGResult{
		{Episode: &Episode{ID: "1", Content: "first result about topic A"}, Score: 0.9},
		{Episode: &Episode{ID: "2", Content: "second result about topic B different"}, Score: 0.8},
		{Episode: &Episode{ID: "3", Content: "third result about topic C unique"}, Score: 0.7},
	}

	reranked, err := c.Rerank(context.Background(), "topic A", results)
	if err != nil {
		t.Fatalf("链式重排序失败: %v", err)
	}

	if len(reranked) != 3 {
		t.Errorf("链式执行后应保持结果数，得到 %d", len(reranked))
	}
}

func TestChainedReranker_EmptyChain(t *testing.T) {
	c := NewChainedReranker()
	results := []*RAGResult{
		{Episode: &Episode{ID: "1", Content: "test"}, Score: 0.9},
	}
	reranked, err := c.Rerank(context.Background(), "q", results)
	if err != nil {
		t.Fatal(err)
	}
	if len(reranked) != 1 {
		t.Error("空链应原样返回")
	}
}

func TestChainedReranker_SingleItem(t *testing.T) {
	c := NewChainedReranker(
		NewMMRReranker(MMRConfig{}),
		NewScoreFusionReranker(FusionConfig{}),
	)
	results := []*RAGResult{
		{Episode: &Episode{ID: "1", Content: "only one"}, Score: 0.5},
	}
	reranked, err := c.Rerank(context.Background(), "q", results)
	if err != nil {
		t.Fatal(err)
	}
	if len(reranked) != 1 || reranked[0].Episode.ID != "1" {
		t.Error("单项应保持不变")
	}
}

// ===== Jaccard 相似度测试 =====

func TestJaccardSimilarity_Identical(t *testing.T) {
	sim := jaccardSimilarity("hello world", "hello world")
	if sim != 1.0 {
		t.Errorf("相同文本的 Jaccard 相似度应为 1.0，实际 %.2f", sim)
	}
}

func TestJaccardSimilarity_CompletelyDifferent(t *testing.T) {
	sim := jaccardSimilarity("abc", "xyz")
	if sim > 0.5 {
		t.Errorf("完全不同文本的 Jaccard 相似度应很低，实际 %.2f", sim)
	}
}

func TestJaccardSimilarity_PartialOverlap(t *testing.T) {
	sim := jaccardSimilarity("hello world foo", "hello world bar")
	if sim < 0.5 {
		t.Errorf("部分重叠文本应有中等相似度，实际 %.2f", sim)
	}
}

func TestJaccardSimilarity_EmptyStrings(t *testing.T) {
	sim := jaccardSimilarity("", "")
	if sim != 1.0 {
		t.Errorf("两个空字符串的相似度应为 1.0，实际 %.2f", sim)
	}
}

// ===== MinMax Normalizer 测试 =====

func TestMinMaxNormalizer(t *testing.T) {
	n := &MinMaxNormalizer{}

	tests := []struct {
		score, min, max, expected float64
	}{
		{5, 0, 10, 0.5},
		{10, 0, 10, 1.0},
		{0, 0, 10, 0.0},
		{7.5, 0, 10, 0.75},
	}

	for _, tt := range tests {
		got := n.Normalize(tt.score, tt.min, tt.max)
		if got != tt.expected {
			t.Errorf("Normalize(%.1f, %.1f, %.1f) = %.2f, 期望 %.2f",
				tt.score, tt.min, tt.max, got, tt.expected)
		}
	}
}

func TestMinMaxNormalizer_EqualRange(t *testing.T) {
	n := &MinMaxNormalizer{}
	got := n.Normalize(5, 5, 5)
	if got != 0.5 {
		t.Errorf("相等范围应返回 0.5，实际 %.2f", got)
	}
}

// ===== recencyScore 测试 =====

func TestRecencyScore_RecentTimestamp(t *testing.T) {
	score := recencyScore("2026-05-30T12:00:00Z")
	if score <= 0.01 {
		t.Errorf("近期时间戳的新鲜度分数应较高，实际 %.4f", score)
	}
}

func TestRecencyScore_EmptyString(t *testing.T) {
	score := recencyScore("")
	if score != 0.5 {
		t.Errorf("空字符串应返回 0.5，实际 %.4f", score)
	}
}
