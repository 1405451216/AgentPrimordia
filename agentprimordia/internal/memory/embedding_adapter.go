// embedding_adapter.go — S0-3 接线：跨包嵌入器适配。
//
// memory 不反向依赖 llm（同层模块互不 import）；本文件用匿名结构化接口把
// 「Embeddings/Dimension 命名口径」的嵌入器（internal/llm.EmbeddingProvider 及
// 其全部实现）适配为本包 EmbeddingProvider（rag.go 定义的 Embed/Dimensions 口径），
// 从而打通：llm 装配（NewEmbeddingProviderFromEnv / 各 Provider）→ memory 向量路径
// （RAGStore / HybridRetriever）的注入链路，无需任何一方 import 对方。
package memory

import "context"

// embeddingsNamed 结构化接口：与 internal/llm.EmbeddingProvider 的方法集
// （Embeddings + Dimension）结构兼容即可注入，无需 import llm 包。
type embeddingsNamed interface {
	Embeddings(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
}

// embeddingsAdapter 适配器：Embeddings→Embed、Dimension→Dimensions。
type embeddingsAdapter struct {
	inner embeddingsNamed
}

func (a embeddingsAdapter) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return a.inner.Embeddings(ctx, texts)
}

func (a embeddingsAdapter) Dimensions() int { return a.inner.Dimension() }

// AsRAGEmbedder 将「Embeddings/Dimension 命名口径」的嵌入器适配为本包
// EmbeddingProvider。典型用法（不破坏现有默认行为——不调用即维持降级位）：
//
//	emb, _ := llm.NewEmbeddingProviderFromEnv()        // 未配置时返回降级位
//	retriever.SetEmbeddingProvider(AsRAGEmbedder(emb)) // HybridRetriever 注入
//	store := NewRAGStore(mem, AsRAGEmbedder(emb))      // RAGStore 构造注入
//
// 注意：适配器不改变语义性——注入 llm.LexicalEmbedder（降级位）时向量通道
// 仍是词面兜底，不算语义接线。
func AsRAGEmbedder(p interface {
	Embeddings(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
}) EmbeddingProvider {
	return embeddingsAdapter{inner: p}
}

// 编译期断言：适配器满足本包 EmbeddingProvider 口径。
var _ EmbeddingProvider = embeddingsAdapter{}
