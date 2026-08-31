// embedding_adapter_test.go — S0-3 接线测试：AsRAGEmbedder 适配 + HybridRetriever
// 注入后的向量通道行为 + 失败退回降级位。
package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stubNamedEmbedder 模拟 internal/llm 口径（Embeddings/Dimension）的嵌入器。
type stubNamedEmbedder struct {
	fail bool
	dim  int
}

func (s *stubNamedEmbedder) Embeddings(_ context.Context, texts []string) ([][]float32, error) {
	if s.fail {
		return nil, errors.New("stub down")
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, s.dim)
		if strings.Contains(t, "数据库") {
			v[0] = 1 // 与文档向量同向：注入路径可控可断言
		}
		out[i] = v
	}
	return out, nil
}

func (s *stubNamedEmbedder) Dimension() int { return s.dim }

// TestAsRAGEmbedder_AdaptsNaming llm 口径 → memory 口径的结构适配。
func TestAsRAGEmbedder_AdaptsNaming(t *testing.T) {
	stub := &stubNamedEmbedder{dim: 8}
	emb := AsRAGEmbedder(stub)
	vecs, err := emb.Embed(context.Background(), []string{"x"})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 8 {
		t.Fatalf("形状意外: %v", vecs)
	}
	if emb.Dimensions() != 8 {
		t.Fatalf("Dimensions() = %d, want 8", emb.Dimensions())
	}
}

// TestHybridRetriever_InjectsProvider 注入后语义通道使用 Provider 向量。
func TestHybridRetriever_InjectsProvider(t *testing.T) {
	stub := &stubNamedEmbedder{dim: 8}
	r := NewHybridRetriever()
	// 文档向量由「同一 Provider」生成：v[0]=1 的文档应命中
	// 注：默认检查用的降级位文档单独构造（见下），注入检查用 8 维 Provider 向量
	r.Add(HybridDoc{ID: "db-doc", Text: "数据库调优实践", Vector: float32ToFloat64([]float32{1, 0, 0, 0, 0, 0, 0, 0})})
	r.Add(HybridDoc{ID: "other-doc", Text: "前端渲染优化", Vector: float32ToFloat64([]float32{0, 1, 0, 0, 0, 0, 0, 0})})

	// 未注入（默认）：降级位语义，行为与历史一致——能返回文档即可
	//（降级位是 64 维词面伪向量，须配套由同一函数生成的文档向量）
	rDefault := NewHybridRetriever()
	rDefault.Add(HybridDoc{ID: "sem", Text: "数据库性能优化指南", Vector: textPseudoVec("如何优化慢查询提升数据库性能")})
	if docs := rDefault.Retrieve("如何优化数据库", 2); len(docs) == 0 {
		t.Fatal("默认降级位路径应仍可检索（现有默认行为零变更）")
	}

	// 注入后：查询含「数据库」→ stub 查询向量 v[0]=1 → db-doc 余弦=1 应排第一
	r.SetEmbeddingProvider(AsRAGEmbedder(stub))
	docs := r.Retrieve("如何优化数据库", 1)
	if len(docs) != 1 || docs[0].ID != "db-doc" {
		t.Fatalf("注入后应命中 db-doc, got %+v", docs)
	}
}

// TestHybridRetriever_FallbackOnEmbedError 单次嵌入失败 → 该次查询退回降级位。
func TestHybridRetriever_FallbackOnEmbedError(t *testing.T) {
	stub := &stubNamedEmbedder{dim: 8, fail: true}
	r := NewHybridRetriever()
	r.Add(HybridDoc{ID: "sem", Text: "数据库性能优化指南", Vector: textPseudoVec("如何优化慢查询提升数据库性能")})
	r.SetEmbeddingProvider(AsRAGEmbedder(stub))
	docs := r.Retrieve("如何提升慢查询的数据库性能", 1)
	if len(docs) != 1 || docs[0].ID != "sem" {
		t.Fatalf("嵌入失败应退回降级位并命中历史语义文档, got %+v", docs)
	}
}
