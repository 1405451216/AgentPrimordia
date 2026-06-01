package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// LLMGenerator 是 LLM 文本生成接口，用于 RAG 端到端生成
//
// 与 llm.Provider 解耦：memory 层不依赖 llm 包，
// 通过此最小接口实现端到端 RAG 生成。
type LLMGenerator interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// RetrievalAugmentedGenerator RAG 端到端生成器
//
// 将检索增强生成流程封装为单一组件：
// 用户查询 → 检索相关文档 → 组装上下文 → LLM 生成答案
type RetrievalAugmentedGenerator struct {
	store      *RAGStore
	generator  LLMGenerator
	systemPrompt string
	topK       int
	minScore   float32
	useHybrid bool
	logger     *slog.Logger
}

// RAGConfig RAG 生成器配置
type RAGConfig struct {
	Store        *RAGStore
	Generator    LLMGenerator
	SystemPrompt string // 系统提示词模板（可使用 {context} 占位符）
	TopK         int    // 检索返回数量（默认 5）
	MinScore     float32 // 最低相关度阈值（默认 0）
	UseHybrid    bool    // 是否使用混合检索（FTS + 向量）
}

const defaultRAGSystemPrompt = `你是一个有帮助的 AI 助手。请基于以下参考信息回答用户的问题。

参考信息：
{context}

要求：
1. 仅基于参考信息回答，不要编造内容
2. 如果参考信息不足以回答问题，请明确说明
3. 引用具体的来源信息`

// NewRetrievalAugmentedGenerator 创建 RAG 端到端生成器
func NewRetrievalAugmentedGenerator(cfg RAGConfig) (*RetrievalAugmentedGenerator, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("RAG Store 不能为空")
	}
	if cfg.Generator == nil {
		return nil, fmt.Errorf("LLM Generator 不能为空")
	}
	if cfg.TopK <= 0 {
		cfg.TopK = 5
	}

	sysPrompt := cfg.SystemPrompt
	if sysPrompt == "" {
		sysPrompt = defaultRAGSystemPrompt
	}

	return &RetrievalAugmentedGenerator{
		store:        cfg.Store,
		generator:    cfg.Generator,
		systemPrompt: sysPrompt,
		topK:         cfg.TopK,
		minScore:     cfg.MinScore,
		useHybrid:    cfg.UseHybrid,
		logger:       slog.Default(),
	}, nil
}

// QueryResult RAG 查询结果
type QueryResult struct {
	Answer   string       `json:"answer"`             // 生成的答案
	Sources  []*RAGResult `json:"sources"`            // 检索到的来源
	Query    string       `json:"query"`              // 原始查询
	Context  string       `json:"context,omitempty"`  // 注入的上下文
	DurationMs int64      `json:"duration_ms"`        // 耗时（毫秒）
}

func (g *RetrievalAugmentedGenerator) buildPrompt(query string, contextText string) string {
	prompt := strings.Replace(g.systemPrompt, "{context}", contextText, 1)
	prompt += fmt.Sprintf("\n\n用户问题：%s\n\n请回答：", query)
	return prompt
}

// Ask 执行 RAG 端到端查询：检索 → 上下文组装 → LLM 生成
func (g *RetrievalAugmentedGenerator) Ask(ctx context.Context, query string) (*QueryResult, error) {
	var results []*RAGResult
	var err error

	if g.useHybrid {
		results, err = g.store.HybridSearch(ctx, query, g.topK)
	} else {
		results, err = g.store.Query(ctx, query, g.topK)
	}
	if err != nil {
		return nil, fmt.Errorf("RAG 检索失败: %w", err)
	}

	filtered := g.filterByMinScore(results)
	contextText := FormatRAGContext(filtered)
	prompt := g.buildPrompt(query, contextText)

	answer, err := g.generator.Generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM 生成失败: %w", err)
	}

	return &QueryResult{
		Answer:  answer,
		Sources: filtered,
		Query:   query,
		Context: contextText,
	}, nil
}

// AskWithStreaming 流式 RAG 查询（先检索再流式生成）
func (g *RetrievalAugmentedGenerator) AskWithStreaming(ctx context.Context, query string) (*QueryResult, <-chan string, error) {
	var results []*RAGResult
	var err error

	if g.useHybrid {
		results, err = g.store.HybridSearch(ctx, query, g.topK)
	} else {
		results, err = g.store.Query(ctx, query, g.topK)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("RAG 检索失败: %w", err)
	}

	filtered := g.filterByMinScore(results)
	contextText := FormatRAGContext(filtered)
	prompt := g.buildPrompt(query, contextText)

	if streamer, ok := g.generator.(LLMStreamGenerator); ok {
		ch, err := streamer.GenerateStream(ctx, prompt)
		if err != nil {
			return nil, nil, fmt.Errorf("LLM 流式生成失败: %w", err)
		}
		result := &QueryResult{
			Sources: filtered,
			Query:   query,
			Context: contextText,
		}
		return result, ch, nil
	}

	answer, err := g.generator.Generate(ctx, prompt)
	if err != nil {
		return nil, nil, fmt.Errorf("LLM 生成失败: %w", err)
	}

	ch := make(chan string, 1)
	ch <- answer
	close(ch)

	return &QueryResult{
		Answer:  answer,
		Sources: filtered,
		Query:   query,
		Context: contextText,
	}, ch, nil
}

// LLMStreamGenerator 流式 LLM 接口
type LLMStreamGenerator interface {
	LLMGenerator
	GenerateStream(ctx context.Context, prompt string) (<-chan string, error)
}

func (g *RetrievalAugmentedGenerator) filterByMinScore(results []*RAGResult) []*RAGResult {
	if g.minScore <= 0 {
		return results
	}
	var filtered []*RAGResult
	for _, r := range results {
		if r.Score >= g.minScore {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// RetrieveOnly 仅执行检索，不调用 LLM（用于预览检索结果）
func (g *RetrievalAugmentedGenerator) RetrieveOnly(ctx context.Context, query string) ([]*RAGResult, error) {
	if g.useHybrid {
		return g.store.HybridSearch(ctx, query, g.topK)
	}
	return g.store.Query(ctx, query, g.topK)
}

// BuildContext 手动构建带上下文的 Prompt（供外部使用）
func (g *RetrievalAugmentedGenerator) BuildContext(query string, sources []*RAGResult) string {
	contextText := FormatRAGContext(sources)
	return g.buildPrompt(query, contextText)
}
