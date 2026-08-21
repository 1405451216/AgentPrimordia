// consolidation.go — v5.3 记忆认知化：episodic → semantic 固化管道。
//
// 记忆不再是检索仓库，而是会沉淀、会遗忘的经验系统：
//  1. 固化（Consolidate）：相似情景记忆经 LLM 蒸馏为一条语义经验
//  2. 衰减（Decay）：importance 随时间与访问频率衰减
//  3. 主动遗忘（Forget）：低于阈值的记忆被清理，存储占用与检索质量双向可控
//
// 量化验收（V6-ROADMAP §五 任务 1）：固化前后存储占用降幅 + recall 保持率。
package memory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// ConsolidationConfig 固化管道配置
type ConsolidationConfig struct {
	// SimilarityThreshold 相似度阈值：超过则两条记忆可合并（0-1，默认 0.82）
	SimilarityThreshold float64
	// DecayHalfLifeDays 重要性半衰期（天，默认 30）
	DecayHalfLifeDays float64
	// ForgetThreshold 遗忘阈值：衰减后 importance 低于此值被清理（默认 0.05）
	ForgetThreshold float64
	// MinClusterSize 触发固化的最小相似记忆数（默认 3）
	MinClusterSize int
}

func (c *ConsolidationConfig) fillDefaults() {
	if c.SimilarityThreshold <= 0 {
		c.SimilarityThreshold = 0.82
	}
	if c.DecayHalfLifeDays <= 0 {
		c.DecayHalfLifeDays = 30
	}
	if c.ForgetThreshold <= 0 {
		c.ForgetThreshold = 0.05
	}
	if c.MinClusterSize <= 0 {
		c.MinClusterSize = 3
	}
}

// SemanticExperience 固化产物：一条语义经验
type SemanticExperience struct {
	ID        string    `json:"id"`
	Summary   string    `json:"summary"`          // 蒸馏后的通用经验
	SourceIDs []string  `json:"source_ids"`       // 来源情景记忆 ID
	Importance float64  `json:"importance"`       // 继承簇内最高重要性
	CreatedAt time.Time `json:"created_at"`
}

// Consolidator 记忆固化器
type Consolidator struct {
	mu sync.Mutex
	cfg ConsolidationConfig

	// episodic 情景记忆池（id → 内容）
	episodic map[string]EpisodicRecord
	// semantic 语义经验库
	semantic map[string]SemanticExperience
}

// EpisodicRecord 情景记忆条目
type EpisodicRecord struct {
	ID         string
	Content    string
	Importance float64
	Tokens     int // 存储占用估算
	CreatedAt  time.Time
	LastAccess time.Time
}

// NewConsolidator 创建固化器
func NewConsolidator(cfg ConsolidationConfig) *Consolidator {
	cfg.fillDefaults()
	return &Consolidator{
		cfg:      cfg,
		episodic: make(map[string]EpisodicRecord),
		semantic: make(map[string]SemanticExperience),
	}
}

// AddEpisodic 写入情景记忆
func (c *Consolidator) AddEpisodic(rec EpisodicRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if rec.Tokens == 0 {
		rec.Tokens = len(rec.Content)/4 + 1
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	if rec.LastAccess.IsZero() {
		rec.LastAccess = rec.CreatedAt
	}
	c.episodic[rec.ID] = rec
}

// jaccard 词级 Jaccard 相似度（确定性、零依赖，可替换为向量相似度）
func jaccard(a, b string) float64 {
	setA := wordSet(a)
	setB := wordSet(b)
	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}
	inter := 0
	for w := range setA {
		if setB[w] {
			inter++
		}
	}
	return float64(inter) / float64(len(setA)+len(setB)-inter)
}

func wordSet(s string) map[string]bool {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !('a' <= r && r <= 'z' || '0' <= r && r <= '9' || r > 127)
	})
	m := make(map[string]bool, len(fields))
	for _, f := range fields {
		if len(f) > 1 {
			m[f] = true
		}
	}
	return m
}

// Consolidate 执行一次固化：把相似度超阈值的记忆簇蒸馏为语义经验并移除源记忆。
// distill 为蒸馏函数（LLM 或确定性摘要）；返回本次生成的经验。
func (c *Consolidator) Consolidate(ctx context.Context, distill func(ctx context.Context, contents []string) (string, error)) ([]SemanticExperience, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 贪心聚类：按写入顺序找相似簇
	clustered := make(map[string]bool)
	var experiences []SemanticExperience
	ids := make([]string, 0, len(c.episodic))
	for id := range c.episodic {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		if clustered[id] {
			continue
		}
		seed := c.episodic[id]
		cluster := []EpisodicRecord{seed}
		clustered[id] = true
		for _, otherID := range ids {
			if clustered[otherID] {
				continue
			}
			other := c.episodic[otherID]
			if jaccard(seed.Content, other.Content) >= c.cfg.SimilarityThreshold {
				cluster = append(cluster, other)
				clustered[otherID] = true
			}
		}
		if len(cluster) < c.cfg.MinClusterSize {
			// 未达固化门槛：归还
			for _, m := range cluster {
				clustered[m.ID] = false
			}
			continue
		}

		contents := make([]string, len(cluster))
		maxImp := 0.0
		tokensFreed := 0
		srcIDs := make([]string, len(cluster))
		for i, m := range cluster {
			contents[i] = m.Content
			if m.Importance > maxImp {
				maxImp = m.Importance
			}
			tokensFreed += m.Tokens
			srcIDs[i] = m.ID
		}
		summary, err := distill(ctx, contents)
		if err != nil {
			return nil, fmt.Errorf("memory: 蒸馏失败: %w", err)
		}
		exp := SemanticExperience{
			ID:         fmt.Sprintf("sem-%d", time.Now().UnixNano()),
			Summary:    summary,
			SourceIDs:  srcIDs,
			Importance: math.Max(maxImp, 0.6), // 固化经验保底重要性
			CreatedAt:  time.Now(),
		}
		c.semantic[exp.ID] = exp
		experiences = append(experiences, exp)
		_ = tokensFreed // 占用降幅由 Stats() 反映
		for _, m := range cluster {
			delete(c.episodic, m.ID)
		}
	}
	return experiences, nil
}

// Decay 对全部记忆执行一次重要性衰减：
// 新衰减系数 = 0.5^(距最后访问天数/半衰期)，高频访问（LastAccess 近）衰减少。
func (c *Consolidator) Decay(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, rec := range c.episodic {
		days := now.Sub(rec.LastAccess).Hours() / 24
		factor := math.Pow(0.5, days/c.cfg.DecayHalfLifeDays)
		rec.Importance *= factor
		c.episodic[id] = rec
	}
}

// Forget 清理衰减后低于阈值的记忆，返回被遗忘的 ID 列表
func (c *Consolidator) Forget() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var forgotten []string
	for id, rec := range c.episodic {
		if rec.Importance < c.cfg.ForgetThreshold {
			delete(c.episodic, id)
			forgotten = append(forgotten, id)
		}
	}
	return forgotten
}

// Touch 访问记忆：刷新 LastAccess 并小幅提升重要性（用进废退）
func (c *Consolidator) Touch(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	rec, ok := c.episodic[id]
	if !ok {
		return false
	}
	rec.LastAccess = time.Now()
	rec.Importance = math.Min(1.0, rec.Importance+0.02)
	c.episodic[id] = rec
	return true
}

// ConsolidationStats 固化管道统计（存储占用与质量双向度量）
type ConsolidationStats struct {
	EpisodicCount   int     `json:"episodic_count"`
	EpisodicTokens  int     `json:"episodic_tokens"`
	SemanticCount   int     `json:"semantic_count"`
	SemanticTokens  int     `json:"semantic_tokens"`
	TotalTokens     int     `json:"total_tokens"`
	CompressionRate float64 `json:"compression_rate"` // 0-1；相对全量情景化的占用降幅
}

// Stats 当前统计：压缩率 = 1 - 现有占用 / (情景+语义原始占用)
func (c *Consolidator) Stats() ConsolidationStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := ConsolidationStats{}
	rawTokens := 0
	for _, r := range c.episodic {
		st.EpisodicCount++
		st.EpisodicTokens += r.Tokens
		rawTokens += r.Tokens
	}
	for _, s := range c.semantic {
		st.SemanticCount++
		// 语义经验占用 = 摘要长度估算 + 来源指针开销常数
		tok := len(s.Summary)/4 + 8
		st.SemanticTokens += tok
		rawTokens += tok * len(s.SourceIDs) // 原始等价占用 = 摘要 × 来源数
	}
	st.TotalTokens = st.EpisodicTokens + st.SemanticTokens
	if rawTokens > 0 {
		st.CompressionRate = 1 - float64(st.TotalTokens)/float64(rawTokens)
	}
	return st
}

// SearchSemantic 按关键词查询语义经验（返回摘要含全部关键词者）
func (c *Consolidator) SearchSemantic(keywords []string) []SemanticExperience {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []SemanticExperience
	for _, s := range c.semantic {
		match := true
		for _, kw := range keywords {
			if !strings.Contains(s.Summary, kw) {
				match = false
				break
			}
		}
		if match {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Importance > out[j].Importance })
	return out
}
