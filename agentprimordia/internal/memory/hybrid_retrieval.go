// hybrid_retrieval.go — v5.3 任务 2/3：混合检索路由 + 跨任务经验迁移。
//
// 任务 2（图-向量混合检索）：按查询类型分流——精确实体/ID/数字查询走
// 关键词通道，语义相似查询走向量通道，混合查询双通道融合去重。
// 任务 3（记忆迁移）：相似任务自动注入历史经验（TransferIndex）。
package memory

import (
	"math"
	"strings"
	"sync"
)

// QueryKind 查询类型
type QueryKind int

const (
	// QueryKeyword 精确查询：含 ID/数字/专有标记 → 关键词通道
	QueryKeyword QueryKind = iota
	// QuerySemantic 语义查询 → 向量通道
	QuerySemantic
	// QueryHybrid 混合查询 → 双通道融合
	QueryHybrid
)

// ClassifyQuery 查询分类：命中精确信号（大写标识符/数字/引号/下划线连接）为关键词，
// 含语义信号（如何/为什么/类似/相似）为语义，两者兼有为混合。
func ClassifyQuery(q string) QueryKind {
	hasExact := strings.ContainsAny(q, "0123456789_\"'") || q != strings.ToLower(q)
	semanticHints := []string{"如何", "为什么", "类似", "相似", "怎么", "how", "why", "similar"}
	hasSemantic := false
	lower := strings.ToLower(q)
	for _, h := range semanticHints {
		if strings.Contains(lower, h) {
			hasSemantic = true
			break
		}
	}
	switch {
	case hasExact && hasSemantic:
		return QueryHybrid
	case hasSemantic:
		return QuerySemantic
	default:
		return QueryKeyword
	}
}

// HybridDoc 检索文档（图节点或向量条目的统一形状）
type HybridDoc struct {
	ID     string
	Text   string
	Vector []float64 // 向量通道内容（可 nil）
}

// HybridRetriever 混合检索器：关键词通道（词集匹配）+ 向量通道（余弦）+ 融合去重
type HybridRetriever struct {
	mu   sync.RWMutex
	docs []HybridDoc
}

// NewHybridRetriever 创建检索器
func NewHybridRetriever() *HybridRetriever { return &HybridRetriever{} }

// Add 写入文档
func (r *HybridRetriever) Add(d HybridDoc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.docs = append(r.docs, d)
}

// Retrieve 按查询类型路由检索，返回 topK 文档。
// 关键词通道：词集包含计分；向量通道：余弦相似度；混合：两路归一化后 0.5/0.5 融合。
func (r *HybridRetriever) Retrieve(query string, topK int) []HybridDoc {
	r.mu.RLock()
	defer r.mu.RUnlock()
	kind := ClassifyQuery(query)
	qw := wordSet(query)
	type scored struct {
		doc HybridDoc
		s   float64
	}
	var out []scored
	for _, d := range r.docs {
		var s float64
		switch kind {
		case QueryKeyword:
			s = keywordScore(qw, d.Text)
		case QuerySemantic:
			s = cosineSlices(d.Vector, textPseudoVec(query))
		case QueryHybrid:
			s = 0.5*keywordScore(qw, d.Text) + 0.5*cosineSlices(d.Vector, textPseudoVec(query))
		}
		if s > 0 {
			out = append(out, scored{d, s})
		}
	}
	// 排序截断
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].s > out[i].s {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if len(out) > topK {
		out = out[:topK]
	}
	docs := make([]HybridDoc, len(out))
	for i, o := range out {
		docs[i] = o.doc
	}
	return docs
}

func keywordScore(qw map[string]bool, text string) float64 {
	tw := wordSet(text)
	if len(tw) == 0 {
		return 0
	}
	hit := 0
	for w := range qw {
		if tw[w] {
			hit++
		}
	}
	if len(qw) == 0 {
		return 0
	}
	return float64(hit) / float64(len(qw))
}

func cosineSlices(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, na, nb float64
	for i := 0; i < n; i++ {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// textPseudoVec 确定性文本伪向量：字符级哈希到 64 维（CJK 友好，无需分词；
// 真实部署替换为 embedding 输出）
func textPseudoVec(s string) []float64 {
	v := make([]float64, 64)
	for _, r := range strings.ToLower(s) {
		if r == ' ' || r == '\n' {
			continue
		}
		h := (int(r) * 31) % 64
		if h < 0 {
			h += 64
		}
		v[h]++
	}
	return v
}

// ===== 任务 3：记忆迁移 =====

// TransferEntry 迁移索引条目：任务签名 → 经验文本
type TransferEntry struct {
	TaskSignature string // 归一化任务签名（词集序列化）
	Experience    string
	TurnsSaved    int // 该经验节省的推理轮数
}

// TransferIndex 跨任务经验迁移索引：相似任务自动召回历史经验
type TransferIndex struct {
	mu      sync.RWMutex
	entries []TransferEntry
}

// NewTransferIndex 创建迁移索引
func NewTransferIndex() *TransferIndex { return &TransferIndex{} }

// Record 记录一条任务经验
func (t *TransferIndex) Record(taskSignature, experience string, turnsSaved int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = append(t.entries, TransferEntry{taskSignature, experience, turnsSaved})
}

// Recall 为新任务召回相似历史经验（Jaccard ≥ 阈值），按相似度降序。
func (t *TransferIndex) Recall(taskDescription string, threshold float64, topK int) []TransferEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()
	type scored struct {
		e TransferEntry
		s float64
	}
	var hits []scored
	for _, e := range t.entries {
		if s := jaccard(taskDescription, e.TaskSignature); s >= threshold {
			hits = append(hits, scored{e, s})
		}
	}
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].s > hits[i].s {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
	if len(hits) > topK {
		hits = hits[:topK]
	}
	out := make([]TransferEntry, len(hits))
	for i, h := range hits {
		out[i] = h.e
	}
	return out
}
