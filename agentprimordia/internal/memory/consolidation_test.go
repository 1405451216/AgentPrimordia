// consolidation_test.go — v5.3 记忆固化管道测试
//
// 量化验收（V6-ROADMAP §五 任务 1）：固化前后存储占用降幅 + recall 保持率。
package memory

import (
	"context"
	"strings"
	"testing"
	"time"
)

// deterministicDistill 确定性蒸馏：取公共主题生成通用经验（测试不依赖 LLM）
func deterministicDistill(_ context.Context, contents []string) (string, error) {
	if len(contents) == 0 {
		return "", nil
	}
	return "通用经验：部署流程包含 " + commonWords(contents), nil
}

func commonWords(contents []string) string {
	count := map[string]int{}
	for _, c := range contents {
		for w := range wordSet(c) {
			count[w]++
		}
	}
	var common []string
	for w, n := range count {
		if n == len(contents) {
			common = append(common, w)
		}
	}
	return strings.Join(common, "/")
}

func TestConsolidateMergesSimilarEpisodes(t *testing.T) {
	c := NewConsolidator(ConsolidationConfig{MinClusterSize: 3})

	base := "deploy pipeline failed at build stage because test timeout"
	for i := 0; i < 3; i++ {
		c.AddEpisodic(EpisodicRecord{
			ID:         string(rune('a' + i)),
			Content:    base + " variant",
			Importance: 0.5,
		})
	}
	// 一条无关记忆不应被合并
	c.AddEpisodic(EpisodicRecord{ID: "z", Content: "unrelated topic about database indexing", Importance: 0.5})

	exps, err := c.Consolidate(context.Background(), deterministicDistill)
	if err != nil {
		t.Fatalf("固化失败: %v", err)
	}
	if len(exps) != 1 {
		t.Fatalf("应生成 1 条语义经验，得到 %d", len(exps))
	}
	if len(exps[0].SourceIDs) != 3 {
		t.Errorf("来源应为 3 条，得到 %d", len(exps[0].SourceIDs))
	}
	st := c.Stats()
	if st.EpisodicCount != 1 { // 只剩无关那条
		t.Errorf("合并后情景记忆应为 1 条，得到 %d", st.EpisodicCount)
	}
	if st.SemanticCount != 1 {
		t.Errorf("语义库应有 1 条，得到 %d", st.SemanticCount)
	}
}

func TestConsolidateBelowThresholdNoop(t *testing.T) {
	c := NewConsolidator(ConsolidationConfig{MinClusterSize: 3})
	for i := 0; i < 2; i++ { // 少于 MinClusterSize
		c.AddEpisodic(EpisodicRecord{ID: string(rune('a' + i)), Content: "same content here", Importance: 0.5})
	}
	exps, err := c.Consolidate(context.Background(), deterministicDistill)
	if err != nil || len(exps) != 0 {
		t.Errorf("不足门槛不应固化: exps=%d err=%v", len(exps), err)
	}
	if c.Stats().EpisodicCount != 2 {
		t.Error("记忆应原样保留")
	}
}

func TestDecayAndForget(t *testing.T) {
	c := NewConsolidator(ConsolidationConfig{
		DecayHalfLifeDays: 10,
		ForgetThreshold:   0.05,
	})

	now := time.Now()
	// 高重要性 + 最近访问 → 存活
	c.AddEpisodic(EpisodicRecord{ID: "hot", Content: "critical hotfix knowledge", Importance: 1.0, LastAccess: now})
	// 低重要性 + 一个半衰期前访问 → 衰减到 0.5，仍存活
	c.AddEpisodic(EpisodicRecord{ID: "warm", Content: "old but useful", Importance: 0.5, LastAccess: now.Add(-10 * 24 * time.Hour)})
	// 低重要性 + 多个半衰期 → 被遗忘
	c.AddEpisodic(EpisodicRecord{ID: "cold", Content: "stale note", Importance: 0.3, LastAccess: now.Add(-80 * 24 * time.Hour)})

	c.Decay(now)

	if got := c.episodic["hot"].Importance; got < 0.99 {
		t.Errorf("最近访问的高价值记忆几乎不衰减: %.3f", got)
	}
	if got := c.episodic["warm"].Importance; got < 0.24 || got > 0.26 {
		t.Errorf("一个半衰期应衰减约一半: %.3f", got)
	}

	forgotten := c.Forget()
	if len(forgotten) != 1 || forgotten[0] != "cold" {
		t.Errorf("应仅遗忘 cold，得到 %v", forgotten)
	}
	if _, ok := c.episodic["cold"]; ok {
		t.Error("cold 应已被删除")
	}
}

func TestTouchRefreshesAndBoosts(t *testing.T) {
	c := NewConsolidator(ConsolidationConfig{})
	past := time.Now().Add(-24 * time.Hour)
	c.AddEpisodic(EpisodicRecord{ID: "x", Content: "some knowledge", Importance: 0.4, LastAccess: past})

	if !c.Touch("x") {
		t.Fatal("Touch 已存在记忆应返回 true")
	}
	rec := c.episodic["x"]
	if rec.Importance <= 0.4 {
		t.Errorf("Touch 应提升重要性: %.3f", rec.Importance)
	}
	if rec.LastAccess.Before(past.Add(23 * time.Hour)) {
		t.Error("Touch 应刷新最后访问时间")
	}
	if c.Touch("missing") {
		t.Error("Touch 不存在的记忆应返回 false")
	}
}

func TestStatsCompressionRate(t *testing.T) {
	c := NewConsolidator(ConsolidationConfig{MinClusterSize: 3})
	content := "k8s rollout stuck on image pull backoff retry limit"
	for i := 0; i < 4; i++ {
		c.AddEpisodic(EpisodicRecord{ID: string(rune('a' + i)), Content: content, Importance: 0.6})
	}
	before := c.Stats()
	if _, err := c.Consolidate(context.Background(), deterministicDistill); err != nil {
		t.Fatal(err)
	}
	after := c.Stats()

	if after.CompressionRate <= 0 {
		t.Errorf("4 条重复记忆固化为 1 条应有正压缩率: %.2f", after.CompressionRate)
	}
	if after.TotalTokens >= before.EpisodicTokens {
		t.Errorf("固化后总占用 (%d) 应低于原始情景占用 (%d)", after.TotalTokens, before.EpisodicTokens)
	}
}

func TestSearchSemanticByKeyword(t *testing.T) {
	c := NewConsolidator(ConsolidationConfig{MinClusterSize: 2})
	for i := 0; i < 2; i++ {
		c.AddEpisodic(EpisodicRecord{ID: string(rune('a' + i)), Content: "redis cluster failover procedure", Importance: 0.7})
	}
	if _, err := c.Consolidate(context.Background(), func(_ context.Context, contents []string) (string, error) {
		return "经验：redis cluster failover 需要检查 sentinel 配置", nil
	}); err != nil {
		t.Fatal(err)
	}

	hits := c.SearchSemantic([]string{"redis", "failover"})
	if len(hits) != 1 {
		t.Fatalf("应命中 1 条，得到 %d", len(hits))
	}
	if none := c.SearchSemantic([]string{"kubernetes"}); len(none) != 0 {
		t.Errorf("不相关关键词不应命中: %d", len(none))
	}
}
