// hybrid_retrieval.go — v5.3 任务 2/3：混合检索路由 + 跨任务经验迁移。
//
// 任务 2（图-向量混合检索）：按查询类型分流——精确实体/ID/数字查询走
// 关键词通道，语义相似查询走向量通道，混合查询双通道融合去重。
// 任务 3（记忆迁移）：相似任务自动注入历史经验（TransferIndex）。
package memory

import (
	"context"
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
	// embedder 可选语义嵌入 Provider（S0-3 接线）。nil 时向量通道退回
	// textPseudoVec 降级位（见其注释）——现有默认行为零变更（铁律 7）。
	embedder EmbeddingProvider
}

// NewHybridRetriever 创建检索器（默认无嵌入 Provider，向量通道走降级位）
func NewHybridRetriever() *HybridRetriever { return &HybridRetriever{} }

// SetEmbeddingProvider 注入语义嵌入 Provider（S0-3：6.x 仅新增，不注入则维持降级位）。
// 注入后语义/混合通道的查询侧用 Provider 生成向量，与 HybridDoc.Vector 做余弦——
// 调用方必须用同一 Provider 生成文档向量后写入 HybridDoc.Vector，否则量纲不匹配。
// 单次查询嵌入失败时该次查询退回 textPseudoVec 降级位（检索不因嵌入后端抖动而失败）。
func (r *HybridRetriever) SetEmbeddingProvider(p EmbeddingProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.embedder = p
}

// retrieveQueryVector 计算查询向量：注入 Provider 时优先真实语义嵌入，
// 失败/未注入时退回降级位。调用方须在文档读锁外调用（Provider 可能走 HTTP）。
func (r *HybridRetriever) retrieveQueryVector(query string) []float64 {
	if r.embedder != nil {
		if vecs, err := r.embedder.Embed(context.Background(), []string{query}); err == nil && len(vecs) == 1 {
			return float32ToFloat64(vecs[0])
		}
	}
	return textPseudoVec(query) // 无 key 降级位（见 textPseudoVec 注释）
}

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
	docs := r.docs
	r.mu.RUnlock()
	kind := ClassifyQuery(query)
	// 查询向量在锁外计算：注入的 Provider 可能走 HTTP，不能占用文档读锁
	qvec := r.retrieveQueryVector(query)
	qw := wordSet(query)
	type scored struct {
		doc HybridDoc
		s   float64
	}
	var out []scored
	for _, d := range docs {
		var s float64
		switch kind {
		case QueryKeyword:
			s = keywordScore(qw, d.Text)
		case QuerySemantic:
			s = cosineSlices(d.Vector, qvec)
		case QueryHybrid:
			s = 0.5*keywordScore(qw, d.Text) + 0.5*cosineSlices(d.Vector, qvec)
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
	result := make([]HybridDoc, len(out))
	for i, o := range out {
		result[i] = o.doc
	}
	return result
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

// textPseudoVec 确定性文本伪向量：字符级哈希到 64 维（CJK 友好，无需分词）。
//
// S0-3 降级说明：这是「无 key 降级位」——仅在未注入 EmbeddingProvider 时兜底使用
// （HybridRetriever.SetEmbeddingProvider / RAGStore 构造参数；跨包实现见
// internal/llm.EmbeddingProvider 经 AsRAGEmbedder 适配）。词面统计而非语义嵌入，
// 其结果不得计入任何语义检索验收数字（docs/V7路线图.md §二 S0-3）。
// 生产语义路径应注入真实 Provider；真实部署替换为 embedding 输出的历史 TODO 就此关闭。
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

// float32ToFloat64 float32 向量转 float64（Provider 接口口径 → 检索余弦口径）。
func float32ToFloat64(v []float32) []float64 {
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = float64(x)
	}
	return out
}
