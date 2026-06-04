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
	"time"

	"agentprimordia/internal/agent"
	"agentprimordia/internal/events"
	"agentprimordia/internal/llm"
	"agentprimordia/internal/memory"
	"agentprimordia/internal/metrics"
	"agentprimordia/internal/tools/builtin"
)

// ===== 可选接口定义 =====

// pkgMemorySearcher 可选搜索能力接口
type pkgMemorySearcher interface {
	SearchByTag(ctx context.Context, tag string, opts *memory.SearchOptions) ([]*memory.Episode, error)
	GetImportant(ctx context.Context, threshold float64, limit int) ([]*memory.Episode, error)
	GetTimeline(ctx context.Context, days int) (map[string][]*memory.Episode, error)
}

// pkgMemoryExporter 可选导入导出接口
type pkgMemoryExporter interface {
	ExportMemories(ctx context.Context, sessionID, format string) ([]byte, error)
	ImportMemories(ctx context.Context, data []byte, format string) (int, error)
}

// pkgMemoryQuery 可选辅助查询接口
type pkgMemoryQuery interface {
	GetMemoriesByTag(ctx context.Context, tag string, limit int) ([]*memory.Episode, error)
	GetMemoriesBySession(ctx context.Context, sessionID string) ([]*memory.Episode, error)
	GetImportantMemories(ctx context.Context, threshold float64, limit int) ([]*memory.Episode, error)
	GetMemoryTimeline(ctx context.Context, days int) ([]*memory.MemoryTimelineGroup, error)
}

// pkgMemoryLifecycle 可选生命周期接口
type pkgMemoryLifecycle interface {
	CleanupExpired(ctx context.Context, maxAgeDays int) (int64, error)
	ClearAll(ctx context.Context, sessionID string) error
}

// pkgMemoryToolUse 可选工具使用记录接口
type pkgMemoryToolUse interface {
	RecordToolUse(ctx context.Context, sessionID, agentName, toolName, args, result string) error
}

// ===== MemoryStore 适配器 =====

// memoryStoreAdapter 将 memory.Memory 适配为 agent.MemoryStore
type memoryStoreAdapter struct {
	store memory.Memory
}

// NewMemoryAdapter 将 memory.Memory 适配为 agent.MemoryStore，用于注入到 ReActConfig.Memory
func NewMemoryAdapter(store memory.Memory) agent.MemoryStore {
	return &memoryStoreAdapter{store: store}
}

func (a *memoryStoreAdapter) Add(ctx context.Context, ep *agent.MemoryEpisode) error {
	episode := &memory.Episode{
		ID:         ep.ID,
		SessionID:  ep.SessionID,
		Role:       ep.Role,
		Content:    ep.Content,
		Summary:    ep.Summary,
		Topics:     ep.Topics,
		Importance: ep.Importance,
		Metadata:   ep.Metadata,
		CreatedAt:  ep.CreatedAt,
	}
	return a.store.Add(ctx, episode)
}

func (a *memoryStoreAdapter) Search(ctx context.Context, query string, opts *memory.SearchOptions) ([]*memory.Episode, error) {
	return a.store.Search(ctx, query, opts)
}

func (a *memoryStoreAdapter) Get(ctx context.Context, id string) (*memory.Episode, error) {
	return a.store.Get(ctx, id)
}

func (a *memoryStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.store.Delete(ctx, id)
}

func (a *memoryStoreAdapter) Count(ctx context.Context, sessionID string) (int64, error) {
	return a.store.Count(ctx, sessionID)
}

func (a *memoryStoreAdapter) List(ctx context.Context, opts *memory.ListOptions) ([]*memory.Episode, error) {
	return a.store.List(ctx, opts)
}

func (a *memoryStoreAdapter) UpdateSummary(ctx context.Context, id string, summary, topics string) error {
	return a.store.UpdateSummary(ctx, id, summary, topics)
}

func (a *memoryStoreAdapter) SetImportance(ctx context.Context, episodeID string, importance float64) error {
	return a.store.SetImportance(ctx, episodeID, importance)
}

func (a *memoryStoreAdapter) SearchByTag(ctx context.Context, tag string, opts *memory.SearchOptions) ([]*memory.Episode, error) {
	if searcher, ok := a.store.(pkgMemorySearcher); ok {
		return searcher.SearchByTag(ctx, tag, opts)
	}
	return nil, fmt.Errorf("search by tag not supported")
}

func (a *memoryStoreAdapter) GetImportant(ctx context.Context, threshold float64, limit int) ([]*memory.Episode, error) {
	if searcher, ok := a.store.(pkgMemorySearcher); ok {
		return searcher.GetImportant(ctx, threshold, limit)
	}
	return nil, fmt.Errorf("get important not supported")
}

func (a *memoryStoreAdapter) GetTimeline(ctx context.Context, days int) (map[string][]*memory.Episode, error) {
	if searcher, ok := a.store.(pkgMemorySearcher); ok {
		return searcher.GetTimeline(ctx, days)
	}
	return nil, fmt.Errorf("get timeline not supported")
}

func (a *memoryStoreAdapter) GetMemoriesByTag(ctx context.Context, tag string, limit int) ([]*memory.Episode, error) {
	if q, ok := a.store.(pkgMemoryQuery); ok {
		return q.GetMemoriesByTag(ctx, tag, limit)
	}
	return nil, fmt.Errorf("get memories by tag not supported")
}

func (a *memoryStoreAdapter) GetMemoriesBySession(ctx context.Context, sessionID string) ([]*memory.Episode, error) {
	if q, ok := a.store.(pkgMemoryQuery); ok {
		return q.GetMemoriesBySession(ctx, sessionID)
	}
	return nil, fmt.Errorf("get memories by session not supported")
}

func (a *memoryStoreAdapter) GetImportantMemories(ctx context.Context, threshold float64, limit int) ([]*memory.Episode, error) {
	if q, ok := a.store.(pkgMemoryQuery); ok {
		return q.GetImportantMemories(ctx, threshold, limit)
	}
	return nil, fmt.Errorf("get important memories not supported")
}

func (a *memoryStoreAdapter) GetMemoryTimeline(ctx context.Context, days int) ([]*memory.MemoryTimelineGroup, error) {
	if q, ok := a.store.(pkgMemoryQuery); ok {
		return q.GetMemoryTimeline(ctx, days)
	}
	return nil, fmt.Errorf("get memory timeline not supported")
}

func (a *memoryStoreAdapter) CleanupExpired(ctx context.Context, maxAgeDays int) (int64, error) {
	if lc, ok := a.store.(pkgMemoryLifecycle); ok {
		return lc.CleanupExpired(ctx, maxAgeDays)
	}
	return 0, fmt.Errorf("cleanup expired not supported")
}

func (a *memoryStoreAdapter) Stats(ctx context.Context) (*memory.MemoryStats, error) {
	return a.store.Stats(ctx)
}

func (a *memoryStoreAdapter) RecordToolUse(ctx context.Context, sessionID, agentName, toolName, args, result string) error {
	if tu, ok := a.store.(pkgMemoryToolUse); ok {
		return tu.RecordToolUse(ctx, sessionID, agentName, toolName, args, result)
	}
	return fmt.Errorf("record tool use not supported")
}

func (a *memoryStoreAdapter) ClearAll(ctx context.Context, sessionID string) error {
	if lc, ok := a.store.(pkgMemoryLifecycle); ok {
		return lc.ClearAll(ctx, sessionID)
	}
	return fmt.Errorf("clear all not supported")
}

func (a *memoryStoreAdapter) ExportMemories(ctx context.Context, sessionID, format string) ([]byte, error) {
	if exp, ok := a.store.(pkgMemoryExporter); ok {
		return exp.ExportMemories(ctx, sessionID, format)
	}
	return nil, fmt.Errorf("export memories not supported")
}

func (a *memoryStoreAdapter) ImportMemories(ctx context.Context, data []byte, format string) (int, error) {
	if exp, ok := a.store.(pkgMemoryExporter); ok {
		return exp.ImportMemories(ctx, data, format)
	}
	return 0, fmt.Errorf("import memories not supported")
}

func (a *memoryStoreAdapter) Close() error {
	return a.store.Close()
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

// ===== MetricsRecorder 适配器 =====

// metricsAdapter 将 metrics.AgentMetrics 适配为 agent.MetricsRecorder
type metricsAdapter struct {
	metrics *metrics.AgentMetrics
}

// NewMetricsAdapter 将 metrics.AgentMetrics 适配为 agent.MetricsRecorder，用于注入到 ReActConfig.Metrics
func NewMetricsAdapter(m *metrics.AgentMetrics) agent.MetricsRecorder {
	return &metricsAdapter{metrics: m}
}

func (a *metricsAdapter) RecordLLMCall(duration time.Duration, err error) {
	a.metrics.RecordLLMCall(duration, err)
}

func (a *metricsAdapter) RecordToolCall(duration time.Duration, err error) {
	a.metrics.RecordToolCall(duration, err)
}

func (a *metricsAdapter) RecordTurn(duration time.Duration) {
	a.metrics.RecordTurn(duration)
}

func (a *metricsAdapter) IncActiveAgents() {
	a.metrics.IncActiveAgents()
}

func (a *metricsAdapter) DecActiveAgents() {
	a.metrics.DecActiveAgents()
}

func (a *metricsAdapter) RecordTokenUsage(model string, promptTokens, completionTokens int) {
	a.metrics.RecordTokenUsage(model, promptTokens, completionTokens)
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
