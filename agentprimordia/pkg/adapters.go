// Stability: 混合 —
//
//	适配器主接口（AgentAdapter / LLMAdapter / MemoryAdapter / ToolAdapter）: Stable。
//	适配器实现（OpenAI / Anthropic / Gemini 等）: Stable。
//	高阶组合（MultiAgentAdapter / PipelineAdapter）: Experimental。
package ap

import (
	"context"
	"fmt"
	"strings"

	"agentprimordia/internal/agent"
	"agentprimordia/internal/events"
	"agentprimordia/internal/llm"
	"agentprimordia/internal/memory"
	"agentprimordia/internal/metrics"
	"agentprimordia/internal/tools/builtin"
)

// ===== 兼容性适配器（Deprecated）=====

// NewMemoryAdapter 将 memory.Memory 适配为 agent.MemoryStore。
//
// Deprecated: memory.Memory 已直接满足 agent.MemoryStore 接口，无需适配器。
// 直接传入 memory.Memory 即可，将在 v2.0.0 移除。
func NewMemoryAdapter(store memory.Memory) agent.MemoryStore {
	return store
}

// NewMetricsAdapter 将 metrics.AgentMetrics 适配为 agent.MetricsRecorder。
//
// Deprecated: metrics.AgentMetrics 已直接满足 agent.MetricsRecorder 接口，无需适配器。
// 直接传入 *metrics.AgentMetrics 即可，将在 v2.0.0 移除。
func NewMetricsAdapter(m *metrics.AgentMetrics) agent.MetricsRecorder {
	return m
}

// ===== EventPublisher 适配器 =====

// eventBusAdapter 将 events.Bus 适配为 agent.EventPublisher
type eventBusAdapter struct {
	bus *events.Bus
}

// NewEventBusAdapter 将 events.Bus 适配为 agent.EventPublisher，用于注入到 ReActConfig.EventPublisher
func NewEventBusAdapter(bus *events.Bus) agent.EventPublisher {
	return &eventBusAdapter{bus: bus}
}

func (a *eventBusAdapter) PublishAsync(eventType string, source string, payload any) error {
	evt := events.Event{
		Type:    events.EventType(eventType),
		Source:  source,
		Payload: payload,
	}
	return a.bus.PublishAsync(evt)
}

// ===== EmbeddingProvider 适配器 =====

// embeddingAdapter 将 llm.Provider 适配为 memory.EmbeddingProvider
type embeddingAdapter struct {
	provider llm.Provider
	dim      int
}

const defaultEmbeddingDimensions = 1536 // OpenAI text-embedding-3-small 默认维度

// NewEmbeddingAdapter 将 llm.Provider 适配为 memory.EmbeddingProvider，dimensions 指定向量维度（默认 1536）
func NewEmbeddingAdapter(provider llm.Provider, dimensions int) memory.EmbeddingProvider {
	if dimensions <= 0 {
		dimensions = defaultEmbeddingDimensions
	}
	return &embeddingAdapter{provider: provider, dim: dimensions}
}

func (a *embeddingAdapter) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if embedder, ok := a.provider.(llm.Embedder); ok {
		return embedder.Embeddings(ctx, texts)
	}
	return nil, fmt.Errorf("embeddings not supported by provider %T", a.provider)
}

func (a *embeddingAdapter) Dimensions() int {
	return a.dim
}

// ===== RAGStore 工厂 =====

// NewRAGStore 创建 RAG 存储实例，集成 Memory + Embedding + Vector，提供混合检索能力
func NewRAGStore(memStore memory.Memory, embedder memory.EmbeddingProvider) *memory.RAGStore {
	return memory.NewRAGStore(memStore, embedder)
}

// ===== RAGProvider 适配器 =====

// ragProviderAdapter 将 memory.RAGStore 适配为 agent.RAGProvider
type ragProviderAdapter struct {
	store *memory.RAGStore
}

// NewRAGProviderAdapter 将 memory.RAGStore 适配为 agent.RAGProvider，用于注入到 ReActConfig.RAG.Provider
func NewRAGProviderAdapter(store *memory.RAGStore) agent.RAGProvider {
	return &ragProviderAdapter{store: store}
}

func (a *ragProviderAdapter) Search(ctx context.Context, query string, topK int) ([]*agent.RAGDocument, error) {
	results, err := a.store.HybridSearch(ctx, query, topK)
	if err != nil {
		return nil, err
	}

	docs := make([]*agent.RAGDocument, 0, len(results))
	for _, r := range results {
		docs = append(docs, &agent.RAGDocument{
			ID:      r.Episode.ID,
			Content: r.Episode.Content,
			Score:   r.Score,
			Source:  formatSources(r.Sources),
			Role:    r.Episode.Role,
		})
	}
	return docs, nil
}

// ===== KnowledgeSearcher 适配器 =====

// knowledgeSearcherAdapter 将 memory.RAGStore 适配为 builtin.KnowledgeSearcher
type knowledgeSearcherAdapter struct {
	store *memory.RAGStore
}

// NewKnowledgeSearcherAdapter 将 memory.RAGStore 适配为 builtin.KnowledgeSearcher，用于注入到 ToolkitConfig
func NewKnowledgeSearcherAdapter(store *memory.RAGStore) builtin.KnowledgeSearcher {
	return &knowledgeSearcherAdapter{store: store}
}

func (a *knowledgeSearcherAdapter) SearchKnowledge(ctx context.Context, query string, topK int) ([]*builtin.KnowledgeDoc, error) {
	results, err := a.store.HybridSearch(ctx, query, topK)
	if err != nil {
		return nil, err
	}

	docs := make([]*builtin.KnowledgeDoc, 0, len(results))
	for _, r := range results {
		docs = append(docs, &builtin.KnowledgeDoc{
			ID:      r.Episode.ID,
			Content: r.Episode.Content,
			Score:   r.Score,
			Source:  formatSources(r.Sources),
		})
	}
	return docs, nil
}

// formatSources 将来源切片格式化为字符串
func formatSources(sources []string) string {
	return strings.Join(sources, "+")
}
