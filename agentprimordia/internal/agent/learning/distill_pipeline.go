// Phase 2.2: 学习×记忆 — 知识蒸馏管道
//
// 将 learning 包的三大组件与 memory 包集成：
//   - KnowledgeDistiller 输出 → SemanticMemory（结构化事实/模式）
//   - CapabilityEvolver 弱项 → RAG 检索相关知识
//   - FeedbackLearner 偏好 → 系统提示自动注入
//
// 使用方式：
//
//	pipeline := learning.NewDistillPipeline(
//	    learning.WithSemanticMemory(semMem),
//	    learning.WithRAGRetriever(ragStore),
//	    learning.WithFeedbackLearner(fbLearner),
//	)
//	pipeline.ProcessInteraction(ctx, interaction)

package learning

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// ===== 记忆接口（解耦 memory 包，避免循环依赖） =====

// SemanticMemorySink 语义记忆写入接口
// 由 memory.SemanticMemory 适配实现
type SemanticMemorySink interface {
	// AddFact 写入结构化事实
	AddFact(ctx context.Context, key, value string, confidence float64, source string)
	// AddPattern 写入tool使用模式
	AddPattern(ctx context.Context, pattern, description string, successRate float64, examples []string)
	// InjectPrompt 获取语义记忆的 system prompt 片段
	InjectPrompt() string
}

// RAGRetriever RAG 检索接口
// 由 memory.RAGStore 适配实现
type RAGRetriever interface {
	// Retrieve 根据查询检索相关知识片段
	Retrieve(ctx context.Context, query string, topK int) ([]RetrievedChunk, error)
}

// RetrievedChunk RAG 检索结果片段
type RetrievedChunk struct {
	Content  string            `json:"content"`
	Score    float64           `json:"score"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ===== 蒸馏管道配置 =====

// DistillPipelineConfig 管道配置
type DistillPipelineConfig struct {
	// MinConfidence 最低置信度阈值（低于此值的知识不写入语义记忆）
	MinConfidence float64
	// WeaknessThreshold 弱项能力阈值（低于此分数触发 RAG 检索）
	WeaknessThreshold float64
	// RAGTopK RAG 检索返回的最大片段数
	RAGTopK int
	// MaxPromptLength 注入系统提示的最大长度
	MaxPromptLength int
	// EnableRAG 是否启用 RAG 检索增强
	EnableRAG bool
}

// DefaultDistillPipelineConfig 默认管道配置
func DefaultDistillPipelineConfig() DistillPipelineConfig {
	return DistillPipelineConfig{
		MinConfidence:     0.6,
		WeaknessThreshold: 0.5,
		RAGTopK:           5,
		MaxPromptLength:   2000,
		EnableRAG:         true,
	}
}

// ===== 蒸馏管道 =====

// DistillPipeline 知识蒸馏管道
//
// 连接 learning 与 memory 两大子系统：
//   - 蒸馏器输出 → 语义记忆（事实/模式）
//   - 弱项能力 → RAG 检索补充知识
//   - 偏好模型 → 系统提示注入
type DistillPipeline struct {
	distiller *KnowledgeDistiller
	evolver   *CapabilityEvolver
	feedback  *FeedbackLearner
	semSink   SemanticMemorySink
	rag       RAGRetriever
	config    DistillPipelineConfig
	logger    *slog.Logger

	mu             sync.RWMutex
	lastRAGResults map[string][]RetrievedChunk // capability -> 最近 RAG 结果
	stats          PipelineStats
}

// PipelineStats 管道统计
type PipelineStats struct {
	TotalProcessed       int64     `json:"total_processed"`
	TotalFactsWritten    int64     `json:"total_facts_written"`
	TotalPatternsWritten int64     `json:"total_patterns_written"`
	TotalRAGQueries      int64     `json:"total_rag_queries"`
	LastProcessTime      time.Time `json:"last_process_time"`
}

// DistillPipelineOption 管道选项
type DistillPipelineOption func(*DistillPipeline)

// WithSemanticMemory 设置语义记忆写入目标
func WithSemanticMemory(sink SemanticMemorySink) DistillPipelineOption {
	return func(p *DistillPipeline) {
		p.semSink = sink
	}
}

// WithRAGRetriever 设置 RAG 检索器
func WithRAGRetriever(rag RAGRetriever) DistillPipelineOption {
	return func(p *DistillPipeline) {
		p.rag = rag
	}
}

// WithFeedbackLearner 设置反馈学习器
func WithFeedbackLearner(fl *FeedbackLearner) DistillPipelineOption {
	return func(p *DistillPipeline) {
		p.feedback = fl
	}
}

// WithPipelineConfig 设置管道配置
func WithPipelineConfig(cfg DistillPipelineConfig) DistillPipelineOption {
	return func(p *DistillPipeline) {
		p.config = cfg
	}
}

// NewDistillPipeline 创建知识蒸馏管道
func NewDistillPipeline(opts ...DistillPipelineOption) *DistillPipeline {
	p := &DistillPipeline{
		distiller:      NewKnowledgeDistiller(),
		evolver:        NewCapabilityEvolver(),
		feedback:       NewFeedbackLearner(),
		config:         DefaultDistillPipelineConfig(),
		logger:         slog.Default(),
		lastRAGResults: make(map[string][]RetrievedChunk),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// WithLogger 设置日志器
func (p *DistillPipeline) WithLogger(logger *slog.Logger) *DistillPipeline {
	p.logger = logger
	p.distiller.WithLogger(logger)
	return p
}

// GetDistiller 获取内部蒸馏器
func (p *DistillPipeline) GetDistiller() *KnowledgeDistiller {
	return p.distiller
}

// GetEvolver 获取内部能力进化器
func (p *DistillPipeline) GetEvolver() *CapabilityEvolver {
	return p.evolver
}

// GetFeedbackLearner 获取内部反馈学习器
func (p *DistillPipeline) GetFeedbackLearner() *FeedbackLearner {
	return p.feedback
}

// ProcessInteraction 处理一次交互（完整管道）
//
// 流程：
//  1. 蒸馏知识 → 写入语义记忆
//  2. 评估能力 → 弱项触发 RAG 检索
//  3. 记录反馈 → 更新偏好模型
func (p *DistillPipeline) ProcessInteraction(ctx context.Context, inter Interaction) (*ProcessResult, error) {
	result := &ProcessResult{InteractionID: inter.ID}

	// 1. 蒸馏知识
	items, err := p.distiller.Distill(ctx, inter)
	if err != nil {
		return nil, fmt.Errorf("distill_pipeline: distill failed: %w", err)
	}
	result.DistilledItems = items

	// 2. 写入语义记忆
	if p.semSink != nil {
		factsWritten, patternsWritten := p.writeToSemanticMemory(ctx, items)
		result.FactsWritten = factsWritten
		result.PatternsWritten = patternsWritten
	}

	// 3. 记录反馈（如果有）
	if inter.Feedback != "" {
		_ = p.feedback.RecordFeedback(FeedbackEntry{
			ID:          inter.ID,
			UserInput:   inter.UserInput,
			AgentOutput: inter.AgentOutput,
			Feedback:    inter.Feedback,
			Rating:      feedbackToRating(inter.Feedback),
			Timestamp:   inter.Timestamp,
		})
		result.FeedbackRecorded = true
	}

	// 4. 更新统计
	p.mu.Lock()
	p.stats.TotalProcessed++
	p.stats.TotalFactsWritten += int64(result.FactsWritten)
	p.stats.TotalPatternsWritten += int64(result.PatternsWritten)
	p.stats.LastProcessTime = time.Now()
	p.mu.Unlock()

	return result, nil
}

// TriggerRAGForWeaknesses 检查弱项能力并触发 RAG 检索
// 返回每个弱项能力对应的检索结果
func (p *DistillPipeline) TriggerRAGForWeaknesses(ctx context.Context) (map[string][]RetrievedChunk, error) {
	if !p.config.EnableRAG || p.rag == nil {
		return nil, nil
	}

	weaknesses := p.evolver.GetWeaknesses(p.config.WeaknessThreshold)
	if len(weaknesses) == 0 {
		return nil, nil
	}

	results := make(map[string][]RetrievedChunk)
	for _, weak := range weaknesses {
		// 构造检索查询
		query := fmt.Sprintf("how to improve %s: %s", weak.Name, weak.Description)
		chunks, err := p.rag.Retrieve(ctx, query, p.config.RAGTopK)
		if err != nil {
			p.logger.Warn("RAG 检索失败",
				"capability", weak.Name,
				"error", err,
			)
			continue
		}
		if len(chunks) > 0 {
			results[weak.Name] = chunks
		}
	}

	// 缓存结果
	p.mu.Lock()
	for k, v := range results {
		p.lastRAGResults[k] = v
	}
	p.stats.TotalRAGQueries += int64(len(weaknesses))
	p.mu.Unlock()

	if len(results) > 0 {
		p.logger.Info("弱项能力 RAG 检索完成",
			"weaknesses", len(weaknesses),
			"results", len(results),
		)
	}

	return results, nil
}

// BuildSystemPrompt 构建增强系统提示
//
// 整合三个来源：
//  1. 语义记忆中的事实/模式
//  2. RAG 检索的相关知识
//  3. 偏好模型的行为指导
func (p *DistillPipeline) BuildSystemPrompt(basePrompt string) string {
	var sections []string

	if basePrompt != "" {
		sections = append(sections, basePrompt)
	}

	// 1. 语义记忆注入
	if p.semSink != nil {
		semPrompt := p.semSink.InjectPrompt()
		if semPrompt != "" {
			sections = append(sections, semPrompt)
		}
	}

	// 2. RAG 知识注入
	p.mu.RLock()
	if len(p.lastRAGResults) > 0 {
		var ragSection strings.Builder
		ragSection.WriteString("## 补充知识（来自 RAG 检索）\n")
		for capName, chunks := range p.lastRAGResults {
			fmt.Fprintf(&ragSection, "### %s\n", capName)
			for _, chunk := range chunks {
				content := chunk.Content
				if len(content) > 200 {
					content = content[:200] + "..."
				}
				fmt.Fprintf(&ragSection, "- %s\n", content)
			}
		}
		sections = append(sections, ragSection.String())
	}
	p.mu.RUnlock()

	// 3. 偏好模型注入
	if p.feedback != nil {
		prefs := p.feedback.GetPreferences()
		if len(prefs.PositivePatterns) > 0 || len(prefs.NegativePatterns) > 0 {
			var prefSection strings.Builder
			prefSection.WriteString("## 用户偏好\n")
			if len(prefs.PositivePatterns) > 0 {
				prefSection.WriteString("用户喜欢的回复风格：\n")
				for _, pattern := range prefs.PositivePatterns {
					fmt.Fprintf(&prefSection, "- %s\n", pattern)
				}
			}
			if len(prefs.NegativePatterns) > 0 {
				prefSection.WriteString("用户不喜欢的回复风格：\n")
				for _, pattern := range prefs.NegativePatterns {
					fmt.Fprintf(&prefSection, "- 避免: %s\n", pattern)
				}
			}
			sections = append(sections, prefSection.String())
		}
	}

	// 合并并截断
	combined := strings.Join(sections, "\n\n")
	if p.config.MaxPromptLength > 0 && len(combined) > p.config.MaxPromptLength {
		combined = combined[:p.config.MaxPromptLength]
	}

	return combined
}

// GetStats 获取管道统计
func (p *DistillPipeline) GetStats() PipelineStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats
}

// ===== 内部方法 =====

// writeToSemanticMemory 将蒸馏知识写入语义记忆
func (p *DistillPipeline) writeToSemanticMemory(ctx context.Context, items []KnowledgeItem) (facts, patterns int) {
	for _, item := range items {
		// 置信度过滤
		if item.Confidence < p.config.MinConfidence {
			continue
		}

		switch item.Category {
		case "fact":
			p.semSink.AddFact(ctx,
				item.ID,
				item.Pattern,
				item.Confidence,
				"distilled",
			)
			facts++

		case "pattern":
			p.semSink.AddPattern(ctx,
				item.Pattern,
				item.Context,
				item.Confidence,
				nil,
			)
			patterns++

		case "preference":
			// 偏好类知识也作为事实写入
			p.semSink.AddFact(ctx,
				item.ID,
				item.Pattern,
				item.Confidence,
				"feedback_distilled",
			)
			facts++
		}
	}

	if facts+patterns > 0 {
		p.logger.Info("知识写入语义记忆",
			"facts", facts,
			"patterns", patterns,
		)
	}

	return facts, patterns
}

// feedbackToRating 将反馈文本转换为评分
func feedbackToRating(feedback string) int {
	lower := strings.ToLower(feedback)
	// 负面词优先检查（避免 "incorrect" 被 "correct" 误匹配）
	negative := []string{"bad", "wrong", "incorrect", "terrible", "awful", "no"}
	positive := []string{"good", "great", "correct", "right", "excellent", "perfect"}

	for _, word := range negative {
		if strings.Contains(lower, word) {
			return -1
		}
	}
	for _, word := range positive {
		if strings.Contains(lower, word) {
			return 1
		}
	}
	return 0
}

// ProcessResult 管道处理结果
type ProcessResult struct {
	InteractionID    string          `json:"interaction_id"`
	DistilledItems   []KnowledgeItem `json:"distilled_items"`
	FactsWritten     int             `json:"facts_written"`
	PatternsWritten  int             `json:"patterns_written"`
	FeedbackRecorded bool            `json:"feedback_recorded"`
}
