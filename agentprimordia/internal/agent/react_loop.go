package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"agentprimordia/internal/llm"
	"agentprimordia/internal/memory"
	"agentprimordia/internal/persist"
	"agentprimordia/internal/tools"
)

// memoryIDCounter 用于生成唯一 MemoryEpisode ID
var memoryIDCounter int64

// nextMemoryID 生成唯一的记忆 ID
func nextMemoryID() string {
	n := atomic.AddInt64(&memoryIDCounter, 1)
	return fmt.Sprintf("msg_%d_%d", time.Now().UnixNano(), n)
}

// ===== 集成接口 =====

// MemoryStore 是 Agent 所需的记忆存储接口
type MemoryStore interface {
	Add(ctx context.Context, episode *MemoryEpisode) error
}

// MemoryEpisode 是 Agent 使用的一集记忆
type MemoryEpisode struct {
	ID         string
	SessionID  string
	Role       string
	Content    string
	Summary    string
	Topics     string
	Importance float64
	Metadata   map[string]string
	CreatedAt  string
}

// EventPublisher 是 Agent 所需的事件发布接口
type EventPublisher interface {
	PublishAsync(eventType string, source string, payload any) error
}

// MetricsRecorder 是 Agent 所需的指标记录接口
type MetricsRecorder interface {
	RecordLLMCall(duration time.Duration, err error)
	RecordToolCall(duration time.Duration, err error)
	RecordTurn(duration time.Duration)
	RecordTokenUsage(model string, promptTokens, completionTokens int)
	IncActiveAgents()
	DecActiveAgents()
}

// ReActConfig holds configuration for a ReAct-based agent
type ReActConfig struct {
	// ===== 核心配置（必填） =====
	Name           string
	SystemPrompt   string
	PromptTemplate *PromptTemplate
	Model          llm.Provider
	MaxTurns       int
	Temperature    float64
	SessionID      string

	// ===== 可选能力（推荐使用链式 API 注入） =====
	// 使用 WithXxx 链式方法替代直接设置这些字段：
	//   agent := NewReActAgent(ReActConfig{...}).WithMemory(mem).WithRAG(ragCfg)
	//
	// 直接设置字段仍然有效（向后兼容），但链式 API 提供更好的类型安全和接口发现。
	//
	// 废弃时间表：
	//   v0.6.0（当前）— Deprecated 标注生效，编译期 warning
	//   v0.7.0         — 升级为编译期 warning + 文档强提示
	//   v1.0.0         — panic if non-nil，强制迁移
	//   v2.0.0         — 字段移除（见 // Removed in v2.0.）
	// 详细迁移指南：docs/migration/v0-deprecations.md

	// Toolkit 工具注册表
	// Deprecated: 使用 .WithToolkit(registry) 链式方法注入
	// Removed in v2.0.
	Toolkit *tools.Registry

	// Memory 存储对话和记忆片段
	// Deprecated: 使用 .WithMemory(store) 链式方法注入
	// Removed in v2.0.
	Memory MemoryStore

	// EventPublisher 发布 Agent 生命周期事件
	// Deprecated: 使用 .WithEvents(publisher) 链式方法注入
	// Removed in v2.0.
	EventPublisher EventPublisher

	// Metrics 指标收集器
	// Deprecated: 使用 .WithMetrics(recorder) 链式方法注入
	// Removed in v2.0.
	Metrics MetricsRecorder

	// ContextWindow 上下文窗口裁剪策略
	// Deprecated: 使用 .WithContextWindow(strategy) 链式方法注入
	// Removed in v2.0.
	ContextWindow ContextWindowStrategy

	// CheckpointStore 状态持久化
	// Deprecated: 使用 .WithCheckpointStore(store) 链式方法注入
	// Removed in v2.0.
	CheckpointStore persist.CheckpointStore

	// RAG 知识库检索配置，启用后 Agent 在推理前自动查询知识库
	// Deprecated: 使用 .WithRAG(config) 链式方法注入
	// Removed in v2.0.
	RAG *RAGConfig

	// Hooks Hook 管理器
	// Deprecated: 使用 .WithHooks(hooks) 链式方法注入
	// Removed in v2.0.
	Hooks Hooks

	// Lifecycle 生命周期管理器（默认自动创建）
	Lifecycle *Lifecycle

	// Logger 结构化日志，默认使用 slog.Default()
	Logger *slog.Logger

	// Summarizer 记忆摘要生成器
	// Deprecated: 使用 .WithSummarizer(summarizer) 链式方法注入
	// Removed in v2.0.
	Summarizer memory.SummaryExtractor

	// FileScope 文件范围限制，指定 Agent 可操作的文件或目录列表
	// Deprecated: 使用 .WithFileScope(scopes) 链式方法注入
	// Removed in v2.0.
	FileScope []string

	// HITL 人机协作配置，启用后 Agent 在指定中断点暂停等待人类确认
	// Deprecated: 使用 .WithHITL(config) 链式方法注入
	// Removed in v2.0.
	HITL *HITLConfig

	// CostTracker 成本追踪器，启用后自动记录每轮 LLM Usage 并追踪成本
	// Deprecated: 使用 .WithCostTracker(tracker) 链式方法注入
	// Removed in v2.0.
	CostTracker *CostTracker

	// Tracer 分布式追踪器，启用后自动在 ReAct Loop 关键点创建 Span
	// Deprecated: 使用 .WithTracer(tracer) 链式方法注入
	// Removed in v2.0.
	Tracer Tracer

	// Cache LLM 响应缓存，启用后自动缓存 Complete 调用结果以减少重复请求
	// Deprecated: 使用 .WithCache(cache) 链式方法注入
	// Removed in v2.0.
	Cache llm.LLMCache
}

// ReActAgent implements the ReAct (Reasoning + Acting) pattern
type ReActAgent struct {
	config    ReActConfig
	lifecycle *Lifecycle
	hooks     Hooks
	logger    *slog.Logger
	startTime time.Time
	stats     AgentStats
	statsMu   sync.RWMutex
	runMu     sync.Mutex
	hitlMgr   *HITLManager

	// self 自引用，指向最外层的 Agent 包装器
	// 用于协议式微内核的接口发现：引擎通过 a.self.(XxxCapable) 检测能力
	// 默认指向自身；WithXxx 链式调用时更新为 CapabilityAgent
	self Agent
}

// NewReActAgent creates a new ReAct-based agent
func NewReActAgent(cfg ReActConfig) *ReActAgent {
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = 50
	}
	if cfg.Lifecycle == nil {
		cfg.Lifecycle = NewLifecycle()
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	if cfg.Cache != nil {
		cached, err := llm.NewCachedProvider(cfg.Model, cfg.Cache, 0.8)
		if err == nil {
			cfg.Model = cached
		}
	}

	a := &ReActAgent{
		config:    cfg,
		lifecycle: cfg.Lifecycle,
		hooks:     cfg.Hooks,
		logger:    cfg.Logger,
		stats: AgentStats{
			Status:      StatusIdle,
			ToolsCalled: make(map[string]int),
		},
		hitlMgr: func() *HITLManager {
			if cfg.HITL != nil {
				return NewHITLManager(*cfg.HITL)
			}
			return nil
		}(),
	}
	a.initSelf()
	return a
}

// initSelf 初始化自引用，必须在构造后调用（因为需要返回值赋值后再设置）
func (a *ReActAgent) initSelf() {
	if a.self == nil {
		a.self = a
	}
}

// ===== 协议式微内核：接口发现辅助方法 =====
// 引擎通过 a.self.(XxxCapable) 检测能力，优先使用接口发现，回退到 config 字段

// getMemoryStore 获取记忆存储，优先通过 MemoryCapable 接口发现
func (a *ReActAgent) getMemoryStore() MemoryStore {
	if c, ok := a.self.(MemoryCapable); ok && c.GetMemoryStore() != nil {
		return c.GetMemoryStore()
	}
	return a.config.Memory
}

// getRAGConfig 获取 RAG 配置，优先通过 RAGCapable 接口发现
func (a *ReActAgent) getRAGConfig() *RAGConfig {
	if c, ok := a.self.(RAGCapable); ok && c.GetRAGConfig() != nil {
		return c.GetRAGConfig()
	}
	return a.config.RAG
}

// getEventPublisher 获取事件发布器，优先通过 EventCapable 接口发现
func (a *ReActAgent) getEventPublisher() EventPublisher {
	if c, ok := a.self.(EventCapable); ok && c.GetEventPublisher() != nil {
		return c.GetEventPublisher()
	}
	return a.config.EventPublisher
}

// getMetricsRecorder 获取指标记录器，优先通过 MetricsCapable 接口发现
func (a *ReActAgent) getMetricsRecorder() MetricsRecorder {
	if c, ok := a.self.(MetricsCapable); ok && c.GetMetricsRecorder() != nil {
		return c.GetMetricsRecorder()
	}
	return a.config.Metrics
}

// getTracer 获取追踪器，优先通过 TraceCapable 接口发现
func (a *ReActAgent) getTracer() Tracer {
	if c, ok := a.self.(TraceCapable); ok && c.GetTracer() != nil {
		return c.GetTracer()
	}
	return a.config.Tracer
}

// getCostTracker 获取成本追踪器，优先通过 CostCapable 接口发现
func (a *ReActAgent) getCostTracker() *CostTracker {
	if c, ok := a.self.(CostCapable); ok && c.GetCostTracker() != nil {
		return c.GetCostTracker()
	}
	return a.config.CostTracker
}

// getCheckpointStore 获取检查点存储，优先通过 CheckpointCapable 接口发现
func (a *ReActAgent) getCheckpointStore() persist.CheckpointStore {
	if c, ok := a.self.(CheckpointCapable); ok && c.GetCheckpointStore() != nil {
		return c.GetCheckpointStore()
	}
	return a.config.CheckpointStore
}

// getContextWindowStrategy 获取上下文窗口策略，优先通过 ContextWindowCapable 接口发现
func (a *ReActAgent) getContextWindowStrategy() ContextWindowStrategy {
	if c, ok := a.self.(ContextWindowCapable); ok && c.GetContextWindowStrategy() != nil {
		return c.GetContextWindowStrategy()
	}
	return a.config.ContextWindow
}

// getSummarizer 获取摘要提取器，优先通过 SummarizerCapable 接口发现
func (a *ReActAgent) getSummarizer() memory.SummaryExtractor {
	if c, ok := a.self.(SummarizerCapable); ok && c.GetSummarizer() != nil {
		return c.GetSummarizer()
	}
	return a.config.Summarizer
}

// getFileScope 获取文件作用域，优先通过 FileScopeCapable 接口发现
func (a *ReActAgent) getFileScope() []string {
	if c, ok := a.self.(FileScopeCapable); ok && c.GetFileScope() != nil {
		return c.GetFileScope()
	}
	return a.config.FileScope
}

func (a *ReActAgent) fireHook(point HookPoint, hctx *HookContext) error {
	if a.hooks != nil {
		hctx.Point = point
		if err := a.hooks.Fire(context.Background(), hctx); err != nil {
			a.logger.Warn("Hook 执行失败", "point", point, "error", err)
		}
	}
	return nil
}

// publishEvent 向 EventPublisher 发布事件
func (a *ReActAgent) publishEvent(eventType string, payload any) {
	if ep := a.getEventPublisher(); ep != nil {
		if err := ep.PublishAsync(eventType, a.config.Name, payload); err != nil {
			a.logger.Warn("发布事件失败", "error", err, "type", eventType)
		}
	}
}

// saveMemory 将消息保存到 Memory
func (a *ReActAgent) saveMemory(ctx context.Context, msg Message) {
	mem := a.getMemoryStore()
	if mem == nil {
		return
	}
	ep := &MemoryEpisode{
		ID:        nextMemoryID(),
		SessionID: a.config.SessionID,
		Role:      string(msg.Role),
		Content:   msg.Content,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if ep.SessionID == "" {
		ep.SessionID = a.config.Name
	}
	if err := mem.Add(ctx, ep); err != nil {
		a.logger.Warn("保存记忆失败", "error", err, "role", msg.Role)
	}

	// 异步提取摘要
	summarizer := a.getSummarizer()
	if summarizer != nil && ep.ID != "" {
		epID := ep.ID
		epContent := ep.Content
		go func() {
			defer func() {
				if r := recover(); r != nil {
					a.logger.Warn("异步摘要提取 panic", "error", r)
				}
			}()
			// 使用超时 context 防止摘要提取无限挂起
			sumCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, err := summarizer.ExtractSummary(sumCtx, epContent)
			if err != nil {
				a.logger.Warn("异步摘要提取失败", "id", epID, "error", err)
				return
			}
			a.logger.Info("异步摘要提取成功", "id", epID, "summary_len", len(result.Summary), "topics", result.Topics)
		}()
	}
}

// saveCheckpoint 保存 Agent 状态
func (a *ReActAgent) saveCheckpoint(ctx context.Context, history []Message, turnCount int, m Metrics) {
	cs := a.getCheckpointStore()
	if cs == nil {
		return
	}

	// 转换消息格式
	msgs := make([]persist.CheckpointMessage, len(history))
	for i, m := range history {
		msgs[i] = persist.CheckpointMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}

	state := &persist.AgentState{
		AgentID:   a.config.Name,
		SessionID: a.config.SessionID,
		Status:    string(a.lifecycle.Status()),
		Messages:  msgs,
		TurnCount: turnCount,
		Metrics: persist.CheckpointMetrics{
			TotalTurns:  m.TotalTurns,
			TotalTools:  m.TotalTools,
			Duration:    m.Duration.String(),
			LLMLatency:  m.LLMLatency.String(),
			ToolLatency: m.ToolLatency.String(),
		},
		SavedAt: time.Now().UTC(),
	}

	if err := cs.Save(ctx, state); err != nil {
		a.logger.Warn("保存检查点失败", "error", err)
	}
}

// trimContext 应用上下文窗口策略裁剪历史
func (a *ReActAgent) trimContext(history []Message, maxMessages int) []Message {
	if cw := a.getContextWindowStrategy(); cw != nil {
		return cw.Trim(history, maxMessages)
	}
	return history
}

// shouldRAG 判断当前轮次是否需要执行 RAG 检索
func (a *ReActAgent) shouldRAG(turn int) bool {
	rag := a.getRAGConfig()
	if rag == nil || rag.Provider == nil {
		return false
	}
	switch rag.Mode {
	case RAGModeFirst:
		return turn == 0
	case RAGModeOnDemand:
		return false // 由 knowledge_search 工具主动触发
	case RAGModeAuto:
		fallthrough
	default:
		return true
	}
}

// ragTopK 返回 RAG 检索的 TopK 值
func (a *ReActAgent) ragTopK() int {
	if rag := a.getRAGConfig(); rag != nil && rag.TopK > 0 {
		return rag.TopK
	}
	return 5
}

// ragMinScore 返回 RAG 检索的最低相关度阈值
func (a *ReActAgent) ragMinScore() float32 {
	if rag := a.getRAGConfig(); rag != nil && rag.MinScore > 0 {
		return rag.MinScore
	}
	return 0.3
}

// searchRAG 执行 RAG 检索并返回格式化上下文
func (a *ReActAgent) searchRAG(ctx context.Context, query string) (string, []*RAGDocument) {
	_ = a.fireHook(HookBeforeRAG, &HookContext{Metadata: map[string]any{"query": query}})

	rag := a.getRAGConfig()
	docs, err := rag.Provider.Search(ctx, query, a.ragTopK())
	if err != nil {
		a.logger.Warn("RAG 检索失败", "error", err, "query", query)
		_ = a.fireHook(HookOnError, &HookContext{Error: err})
		return "", nil
	}

	// 过滤低分结果
	minScore := a.ragMinScore()
	filtered := make([]*RAGDocument, 0, len(docs))
	for _, doc := range docs {
		if doc.Score >= minScore {
			filtered = append(filtered, doc)
		}
	}

	_ = a.fireHook(HookAfterRAG, &HookContext{Metadata: map[string]any{"results": len(filtered), "query": query}})

	if len(filtered) == 0 {
		return "", nil
	}

	// 格式化 RAG 上下文
	context := FormatRAGDocuments(filtered)
	return context, filtered
}

// injectRAGContext 将 RAG 检索结果注入到历史消息中
// 如果已存在 RAG 上下文消息，则替换；否则在 system 消息之后插入
func (a *ReActAgent) injectRAGContext(history []Message, ragContext string) []Message {
	if ragContext == "" {
		return history
	}

	ragMsg := SystemMessage(ragContext)
	if ragMsg.Metadata.Extra == nil {
		ragMsg.Metadata.Extra = make(map[string]string)
	}
	ragMsg.Metadata.Extra["rag_context"] = "true"

	// 查找已有的 RAG 上下文消息并替换
	for i, m := range history {
		if m.Role == RoleSystem && m.Metadata.Extra["rag_context"] == "true" {
			// 替换现有的 RAG 上下文消息
			history[i] = ragMsg
			return history
		}
	}

	// 没有找到已有的 RAG 消息，在 system 消息之后插入
	systemEnd := 0
	for i, m := range history {
		if m.Role != RoleSystem {
			systemEnd = i
			break
		}
		if i == len(history)-1 {
			systemEnd = len(history)
		}
	}

	newHistory := make([]Message, 0, len(history)+1)
	newHistory = append(newHistory, history[:systemEnd]...)
	newHistory = append(newHistory, ragMsg)
	newHistory = append(newHistory, history[systemEnd:]...)

	return newHistory
}

// loopConfig 循环配置
type loopConfig struct {
	stream    bool
	streamCh  chan StreamEvent
	streamCtx context.Context
}

// emitStream 在流式模式下向通道发送事件
func (a *ReActAgent) emitStream(cfg loopConfig, event StreamEvent) {
	if cfg.stream && cfg.streamCh != nil {
		select {
		case cfg.streamCh <- event:
		case <-cfg.streamCtx.Done():
		}
	}
}

// Run executes the agent with the given input message
func (a *ReActAgent) Run(ctx context.Context, input Message) (*Response, error) {
	return a.reactLoopEngine(ctx, input, loopConfig{stream: false})
}

// StreamRun 执行 Agent 并以流式方式输出结果
// 返回一个事件 channel，调用者可以逐事件消费
func (a *ReActAgent) StreamRun(ctx context.Context, input Message) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 32)
	go func() {
		defer close(ch)
		_, _ = a.reactLoopEngine(ctx, input, loopConfig{stream: true, streamCh: ch, streamCtx: ctx})
	}()
	return ch, nil
}

// reactLoopEngine ReAct 循环核心引擎
// 统一处理流式和非流式两种运行模式，消除 Run/StreamRun 之间的代码重复
// runLoop ReAct 循环核心体，被 reactLoopEngine 和 ResumeFromCheckpoint 共享
// 封装从 startTurn 开始的主循环逻辑，包括 LLM 调用、工具执行、checkpoint 保存等
func (a *ReActAgent) runLoop(ctx context.Context, history []Message, startTurn int, cfg loopConfig, totalLLMLatency time.Duration, totalToolLatency time.Duration, toolCount int) (*Response, error) {
	for turn := startTurn; turn < a.config.MaxTurns; turn++ {
		turnStart := time.Now()

		if a.lifecycle.IsStopped() {
			_ = a.lifecycle.SetStatus(StatusCancelled)
			a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: ErrAgentStopped.Error()})
			return &Response{Error: ErrAgentStopped}, ErrAgentStopped
		}

		if ctx.Err() != nil {
			_ = a.lifecycle.SetStatus(StatusCancelled)
			a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: ctx.Err().Error()})
			return &Response{Error: ctx.Err()}, ctx.Err()
		}

		a.statsMu.Lock()
		a.stats.CurrentTurn = turn + 1
		a.stats.TotalMessages = len(history)
		a.statsMu.Unlock()

		_ = a.fireHook(HookBeforeTurn, &HookContext{Turn: turn})
		a.publishEvent("turn.start", map[string]int{"turn": turn})

		var turnSpan Span = &NoopSpan{}
		if tracer := a.getTracer(); tracer != nil {
			turnSpan = tracer.Start(
				fmt.Sprintf("turn.%d", turn),
				SpanKindInternal,
				WithAttributes(map[string]any{"agent": a.config.Name, "turn": turn}),
			)
		}

		if ct := a.getCostTracker(); ct != nil && ct.CheckBudget() {
			a.logger.Warn("Agent 超出预算", "name", a.config.Name)
			_ = a.lifecycle.SetStatus(StatusFailed)
			return &Response{Error: ErrBudgetExceeded}, ErrBudgetExceeded
		}

		var ragQuery string
		if turn == startTurn && len(history) > 0 {
			for i := len(history) - 1; i >= 0; i-- {
				if history[i].Role == RoleUser {
					ragQuery = history[i].Content
					break
				}
			}
		} else if turn < len(history) {
			ragQuery = history[turn].Content
		}

		if a.shouldRAG(turn) && ragQuery != "" {
			ragContext, _ := a.searchRAG(ctx, ragQuery)
			if ragContext != "" {
				history = a.injectRAGContext(history, ragContext)
				a.logger.Debug("RAG 上下文已注入", "turn", turn, "query_len", len(ragQuery))
				a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: "[RAG] 知识库上下文已注入"})
			}
		}

		trimmedHistory := a.trimContext(history, 0)
		llmMessages := convertToLLMMessages(trimmedHistory)

		var toolDefs []map[string]any
		if a.config.Toolkit != nil {
			toolDefs = a.config.Toolkit.Definitions()
		}

		llmStart := time.Now()
		a.publishEvent("llm.call", map[string]int{"turn": turn})

		var llmSpan Span = &NoopSpan{}
		if tracer := a.getTracer(); tracer != nil {
			llmSpan = tracer.Start(
				"llm.call",
				SpanKindClient,
				WithParent(turnSpan.SpanContext()),
				WithAttributes(map[string]any{"agent": a.config.Name, "turn": turn}),
			)
		}

		var thought Thought

		if cfg.stream {
			thought = a.streamReasoning(ctx, cfg, llmMessages, toolDefs, llmStart)
			if thought.Content == "" && len(thought.ToolCalls) == 0 {
				return &Response{Error: fmt.Errorf("stream reasoning failed")}, fmt.Errorf("stream reasoning failed")
			}
		} else {
			var err error
			thought, err = a.syncReasoning(ctx, llmMessages, toolDefs, llmStart)
			if err != nil {
				a.handleOnError(ctx, err)
				_ = a.lifecycle.SetStatus(StatusFailed)
				return &Response{Error: err}, err
			}
		}

		llmLatency := time.Since(llmStart)
		totalLLMLatency += llmLatency

		llmSpan.SetAttribute("latency_ms", llmLatency.Milliseconds())
		llmSpan.End()

		a.recordUsage(thought.Usage)

		assistantMsg := Message{
			Role:      RoleAssistant,
			Content:   thought.Content,
			ToolCalls: thought.ToolCalls,
		}
		a.saveMemory(ctx, assistantMsg)

		_ = a.fireHook(HookAfterLLM, &HookContext{Turn: turn})
		a.publishEvent("llm.response", map[string]int{"turn": turn})

		if len(thought.ToolCalls) == 0 {
			duration := time.Since(a.startTime)

			response := &Response{
				Content: thought.Content,
				Metrics: Metrics{
					TotalTurns:  turn + 1,
					TotalTools:  toolCount,
					Duration:    duration,
					LLMLatency:  totalLLMLatency,
					ToolLatency: totalToolLatency,
				},
			}

			_ = a.lifecycle.SetStatus(StatusCompleted)
			a.saveCheckpoint(ctx, history, turn+1, response.Metrics)
			_ = a.fireHook(HookOnComplete, &HookContext{Response: response})
			turnSpan.End()
			_ = a.fireHook(HookAfterTurn, &HookContext{Turn: turn})
			if m := a.getMetricsRecorder(); m != nil {
				m.RecordTurn(time.Since(turnStart))
			}
			a.publishEvent("turn.end", map[string]int{"turn": turn})

			a.emitStream(cfg, StreamEvent{Type: StreamEventComplete, Content: thought.Content, Data: response})

			if cfg.stream {
				a.logger.Info("Agent 流式完成", "name", a.config.Name, "turns", turn+1, "duration", duration)
			} else {
				a.logger.Info("Agent 完成", "name", a.config.Name, "turns", turn+1, "duration", duration)
			}
			return response, nil
		}

		history = append(history, assistantMsg)

		for _, tc := range thought.ToolCalls {
			a.emitStream(cfg, StreamEvent{Type: StreamEventToolCall, Content: tc.Name, Data: tc})
			_ = a.fireHook(HookBeforeTool, &HookContext{ToolCall: &tc, Turn: turn})
			a.publishEvent("tool.call", map[string]string{"tool": tc.Name, "turn": fmt.Sprintf("%d", turn)})

			if a.hitlMgr != nil && a.hitlMgr.ShouldInterrupt(tc.Name, InterruptToolConfirm) {
				a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: fmt.Sprintf("[HITL] 工具 %s 需要人类确认", tc.Name)})
				_ = a.lifecycle.WaitForInput(fmt.Sprintf("工具 %s 需确认", tc.Name))

				humanResp, hitlErr := a.hitlMgr.RequestInterrupt(ctx, &InterruptRequest{
					Reason:  InterruptToolConfirm,
					Message: fmt.Sprintf("Agent 请求执行工具 %s，参数: %s", tc.Name, tc.Args),
					Data:    map[string]any{"tool": tc.Name, "args": tc.Args},
					Turn:    turn,
				})

				_ = a.lifecycle.SetStatus(StatusRunning)

				if hitlErr != nil {
					a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: fmt.Sprintf("[HITL] 等待人类确认失败: %v", hitlErr)})
					result := &ToolResult{ToolCallID: tc.ID, Content: fmt.Sprintf("人类确认超时: %v", hitlErr), IsError: true}
					history = append(history, result.ToMessage())
					a.saveMemory(ctx, result.ToMessage())
					continue
				}

				if !humanResp.Approved {
					a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: fmt.Sprintf("[HITL] 人类拒绝执行工具 %s", tc.Name)})
					rejectMsg := "人类拒绝执行此操作"
					if humanResp.Input != "" {
						rejectMsg = humanResp.Input
					}
					result := &ToolResult{ToolCallID: tc.ID, Content: rejectMsg, IsError: true}
					history = append(history, result.ToMessage())
					a.saveMemory(ctx, result.ToMessage())
					continue
				}

				if humanResp.Modified != nil {
					if modifiedArgs, ok := humanResp.Modified["args"].(string); ok {
						tc.Args = modifiedArgs
					}
				}

				a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: fmt.Sprintf("[HITL] 人类已确认执行工具 %s", tc.Name)})
			}

			toolStart := time.Now()

			var toolSpan Span = &NoopSpan{}
			if tracer := a.getTracer(); tracer != nil {
				toolSpan = tracer.Start(
					fmt.Sprintf("tool.%s", tc.Name),
					SpanKindClient,
					WithParent(turnSpan.SpanContext()),
					WithAttributes(map[string]any{"tool": tc.Name, "agent": a.config.Name}),
				)
			}

			result, err := a.executeTool(ctx, tc)
			toolLatency := time.Since(toolStart)
			totalToolLatency += toolLatency

			toolSpan.SetAttribute("latency_ms", toolLatency.Milliseconds())
			if err != nil {
				toolSpan.SetStatus(SpanStatusError, err.Error())
			}
			toolSpan.End()
			if m := a.getMetricsRecorder(); m != nil {
				m.RecordToolCall(toolLatency, err)
			}

			if err != nil {
				a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: fmt.Sprintf("tool %s error: %v", tc.Name, err)})
				_ = a.fireHook(HookOnError, &HookContext{Error: err, Turn: turn})
				a.publishEvent("agent.error", map[string]string{"tool": tc.Name, "error": err.Error()})
			} else {
				a.emitStream(cfg, StreamEvent{Type: StreamEventToolResult, Content: result.Content, Data: result})
			}

			if cfg.stream {
				if err == nil {
					_ = a.fireHook(HookAfterTool, &HookContext{ToolResult: result, Turn: turn})
				}
			} else {
				_ = a.fireHook(HookAfterTool, &HookContext{ToolResult: result, Turn: turn})
				a.publishEvent("tool.result", map[string]string{"tool": tc.Name})
			}

			toolCount++
			a.statsMu.Lock()
			a.stats.ToolsCalled[tc.Name]++
			a.statsMu.Unlock()

			history = append(history, result.ToMessage())
			a.saveMemory(ctx, result.ToMessage())
		}

		turnSpan.End()
		_ = a.fireHook(HookAfterTurn, &HookContext{Turn: turn})
		if m := a.getMetricsRecorder(); m != nil {
			m.RecordTurn(time.Since(turnStart))
		}
		a.publishEvent("turn.end", map[string]int{"turn": turn})

		if a.lifecycle.IsGracefulShutdown() {
			a.logger.Info("Agent 优雅关闭：当前 turn 已完成，退出循环", "name", a.config.Name, "turn", turn+1)
			_ = a.lifecycle.SetStatusWithReason(StatusCancelled, "graceful shutdown")
			duration := time.Since(a.startTime)
			response := &Response{
				Content: thought.Content,
				Error:   ErrAgentStopped,
				Metrics: Metrics{
					TotalTurns:  turn + 1,
					TotalTools:  toolCount,
					Duration:    duration,
					LLMLatency:  totalLLMLatency,
					ToolLatency: totalToolLatency,
				},
			}
			a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: "graceful shutdown: agent stopped after turn completion"})
			return response, ErrAgentStopped
		}

		a.saveCheckpoint(ctx, history, turn+1, Metrics{
			TotalTurns:  turn + 1,
			TotalTools:  toolCount,
			Duration:    time.Since(a.startTime),
			LLMLatency:  totalLLMLatency,
			ToolLatency: totalToolLatency,
		})
	}

	_ = a.lifecycle.SetStatus(StatusFailed)
	a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: ErrMaxTurnsExceeded.Error()})
	_ = a.fireHook(HookOnError, &HookContext{Error: ErrMaxTurnsExceeded})
	a.logger.Warn("Agent 超出最大轮次", "name", a.config.Name, "max_turns", a.config.MaxTurns)

	response := &Response{
		Error: ErrMaxTurnsExceeded,
		Metrics: Metrics{
			TotalTurns:  a.config.MaxTurns,
			TotalTools:  toolCount,
			Duration:    time.Since(a.startTime),
			LLMLatency:  totalLLMLatency,
			ToolLatency: totalToolLatency,
		},
	}

	return response, ErrMaxTurnsExceeded
}

func (a *ReActAgent) reactLoopEngine(ctx context.Context, input Message, cfg loopConfig) (*Response, error) {
	a.runMu.Lock()
	defer a.runMu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			a.logger.Error("ReAct 循环 panic 恢复", "error", r)
			_ = a.lifecycle.SetStatus(StatusFailed)
			a.publishEvent("agent.panic", map[string]string{"name": a.config.Name, "error": fmt.Sprintf("%v", r)})
		}
	}()

	a.startTime = time.Now()
	a.stats.StartTime = a.startTime
	_ = a.lifecycle.SetStatus(StatusRunning)
	a.stats.Status = StatusRunning

	if cfg.stream {
		a.logger.Info("Agent 流式启动", "name", a.config.Name, "session", a.config.SessionID)
	} else {
		a.logger.Info("Agent 启动", "name", a.config.Name, "session", a.config.SessionID)
	}
	a.publishEvent("agent.start", map[string]string{"name": a.config.Name})
	_ = a.fireHook(HookBeforeRun, &HookContext{})

	if m := a.getMetricsRecorder(); m != nil {
		m.IncActiveAgents()
		defer m.DecActiveAgents()
	}

	defer func() {
		if a.lifecycle.Status() != StatusCompleted &&
			a.lifecycle.Status() != StatusFailed &&
			a.lifecycle.Status() != StatusCancelled {
			_ = a.lifecycle.SetStatus(StatusCompleted)
		}
		a.publishEvent("agent.stop", map[string]string{
			"name":   a.config.Name,
			"status": string(a.lifecycle.Status()),
		})
	}()

	var systemPrompt string

	if a.config.PromptTemplate != nil {
		rendered, err := a.config.PromptTemplate.WithVar("AgentName", a.config.Name).Render()
		if err != nil {
			a.logger.Warn("渲染 PromptTemplate 失败", "error", err)
		} else if rendered != "" {
			// 使用局部变量保存渲染结果，避免修改原始配置导致重复运行时模板变量被覆盖
			systemPrompt = rendered
		}
	}

	// 使用 PromptTemplate 构建系统提示词
	if systemPrompt == "" && a.config.SystemPrompt != "" {
		if fs := a.getFileScope(); len(fs) > 0 {
			tmpl := CodeAssistantTemplate(a.config.SystemPrompt, fs)
			var err error
			systemPrompt, err = tmpl.Render()
			if err != nil {
				a.logger.Warn("渲染系统提示词模板失败，使用原始提示词", "error", err)
				systemPrompt = a.config.SystemPrompt
			}
		} else {
			systemPrompt = a.config.SystemPrompt
		}
	} else if systemPrompt == "" {
		tmpl := DefaultSystemPrompt().WithVar("AgentName", a.config.Name)
		if fs := a.getFileScope(); len(fs) > 0 {
			tmpl.WithScopeRules(fs)
		}
		var err error
		systemPrompt, err = tmpl.Render()
		if err != nil {
			a.logger.Warn("渲染默认系统提示词模板失败", "error", err)
			systemPrompt = fmt.Sprintf("你是一个 AI 助手，名为 %s。", a.config.Name)
		}
	}

	history := []Message{}
	if systemPrompt != "" {
		history = append(history, SystemMessage(systemPrompt))
	}
	history = append(history, input)
	a.saveMemory(ctx, input)

	return a.runLoop(ctx, history, 0, cfg, 0, 0, 0)
}
// syncReasoning 非流式推理阶段
func (a *ReActAgent) syncReasoning(ctx context.Context, llmMessages []llm.ChatMessage, toolDefs []map[string]any, llmStart time.Time) (Thought, error) {
	var thought Thought

	if len(toolDefs) > 0 {
		resp, err := a.callToolsWithRetry(ctx, llmMessages, toolDefs)
		if err != nil {
			if m := a.getMetricsRecorder(); m != nil {
				m.RecordLLMCall(time.Since(llmStart), err)
			}
			return Thought{}, err
		}
		if len(resp.ToolCalls) == 0 && resp.Content == "" {
			completeResp, completeErr := a.completeWithRetry(ctx, llmMessages)
			if completeErr != nil {
				if m := a.getMetricsRecorder(); m != nil {
					m.RecordLLMCall(time.Since(llmStart), completeErr)
				}
				return Thought{}, completeErr
			}
			thought = Thought{Content: completeResp.Content, Usage: completeResp.Usage}
		} else {
			thought = Thought{
				Content:   resp.Content,
				ToolCalls: convertToToolCalls(resp.ToolCalls),
				Usage:     resp.Usage,
			}
		}
		if m := a.getMetricsRecorder(); m != nil {
			m.RecordLLMCall(time.Since(llmStart), nil)
		}
	} else {
		resp, err := a.completeWithRetry(ctx, llmMessages)
		if err != nil {
			if m := a.getMetricsRecorder(); m != nil {
				m.RecordLLMCall(time.Since(llmStart), err)
			}
			return Thought{}, err
		}
		thought = Thought{Content: resp.Content, Usage: resp.Usage}
		if m := a.getMetricsRecorder(); m != nil {
			m.RecordLLMCall(time.Since(llmStart), nil)
		}
	}

	return thought, nil
}

// streamReasoning 流式推理阶段
// 先尝试 Stream 接口，失败则回退到非流式调用
func (a *ReActAgent) streamReasoning(ctx context.Context, cfg loopConfig, llmMessages []llm.ChatMessage, toolDefs []map[string]any, llmStart time.Time) Thought {
	streamCh, streamErr := a.config.Model.Stream(ctx, &llm.CompletionRequest{
		Messages:    llmMessages,
		Temperature: llm.Float64Ptr(a.config.Temperature),
	})

	if streamErr == nil {
		var fullContent string
		for chunk := range streamCh {
			if ctx.Err() != nil {
				_ = a.lifecycle.SetStatus(StatusCancelled)
				a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: ctx.Err().Error()})
				return Thought{}
			}
			if chunk.Content != "" {
				fullContent += chunk.Content
				a.emitStream(cfg, StreamEvent{Type: StreamEventToken, Content: chunk.Content})
			}
			if chunk.Done {
				break
			}
		}
		thought := Thought{Content: fullContent}

		if a.config.Toolkit != nil {
			td := a.config.Toolkit.Definitions()
			if len(td) > 0 {
				resp, err := a.callToolsWithRetry(ctx, llmMessages, td)
				if err == nil && len(resp.ToolCalls) > 0 {
					thought = Thought{
						Content:   resp.Content,
						ToolCalls: convertToToolCalls(resp.ToolCalls),
					}
				}
			}
		}

		if m := a.getMetricsRecorder(); m != nil {
			m.RecordLLMCall(time.Since(llmStart), nil)
		}
		return thought
	}

	// Fallback: 非流式调用
	if len(toolDefs) > 0 {
		resp, err := a.callToolsWithRetry(ctx, llmMessages, toolDefs)
		if err != nil {
			if m := a.getMetricsRecorder(); m != nil {
				m.RecordLLMCall(time.Since(llmStart), err)
			}
			_ = a.lifecycle.SetStatus(StatusFailed)
			a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: err.Error()})
			return Thought{}
		}
		if len(resp.ToolCalls) == 0 && resp.Content == "" {
			completeResp, completeErr := a.completeWithRetry(ctx, llmMessages)
			if completeErr != nil {
				if m := a.getMetricsRecorder(); m != nil {
					m.RecordLLMCall(time.Since(llmStart), completeErr)
				}
				_ = a.lifecycle.SetStatus(StatusFailed)
				a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: completeErr.Error()})
				return Thought{}
			}
			a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: completeResp.Content})
			if m := a.getMetricsRecorder(); m != nil {
				m.RecordLLMCall(time.Since(llmStart), nil)
			}
			return Thought{Content: completeResp.Content}
		}
		a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: resp.Content})
		if m := a.getMetricsRecorder(); m != nil {
			m.RecordLLMCall(time.Since(llmStart), nil)
		}
		return Thought{
			Content:   resp.Content,
			ToolCalls: convertToToolCalls(resp.ToolCalls),
		}
	}

	resp, err := a.completeWithRetry(ctx, llmMessages)
	if err != nil {
		if m := a.getMetricsRecorder(); m != nil {
			m.RecordLLMCall(time.Since(llmStart), err)
		}
		_ = a.lifecycle.SetStatus(StatusFailed)
		a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: err.Error()})
		return Thought{}
	}
	a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: resp.Content})
	if m := a.getMetricsRecorder(); m != nil {
		m.RecordLLMCall(time.Since(llmStart), nil)
	}
	return Thought{Content: resp.Content}
}

// ErrMaxTurnsExceeded is returned when the agent exceeds max turn limit
var ErrMaxTurnsExceeded = errors.New("max turns exceeded")

var ErrBudgetExceeded = errors.New("budget exceeded")

var ErrNoToolkit = errors.New("no toolkit configured")

// executeTool runs a single tool call
func (a *ReActAgent) executeTool(ctx context.Context, tc ToolCall) (*ToolResult, error) {
	if a.config.Toolkit == nil {
		return &ToolResult{
			ToolCallID: tc.ID,
			Content:    "error: no toolkit configured",
			IsError:    true,
		}, ErrNoToolkit
	}

	executor := tools.NewExecutor(a.config.Toolkit)
	fc := tools.FunctionCall{
		ID:   tc.ID,
		Name: tc.Name,
		Args: tc.Args,
	}

	result, err := executor.Execute(ctx, &fc)
	if err != nil {
		return &ToolResult{
			ToolCallID: tc.ID,
			Content:    err.Error(),
			IsError:    true,
		}, err
	}

	return &ToolResult{
		ToolCallID: tc.ID,
		Content:    result.Content,
		IsError:    result.IsError,
	}, nil
}

// callToolsWithRetry calls LLM with function calling support
func (a *ReActAgent) callToolsWithRetry(ctx context.Context, messages []llm.ChatMessage, toolDefs []map[string]any) (*llm.ToolCallResponse, error) {
	definitions := make([]llm.ToolDefinition, 0, len(toolDefs))
	for _, def := range toolDefs {
		fn, ok := def["function"].(map[string]any)
		if !ok {
			continue
		}
		typ, _ := def["type"].(string)
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]any)
		if name == "" {
			continue
		}
		definitions = append(definitions, llm.ToolDefinition{
			Type: typ,
			Function: llm.FunctionDefinition{
				Name:        name,
				Description: desc,
				Parameters:  params,
			},
		})
	}

	req := &llm.ToolCallRequest{
		Messages: messages,
		Tools:    definitions,
	}

	return a.config.Model.CallTools(ctx, req)
}

// completeWithRetry calls LLM for simple completion
func (a *ReActAgent) completeWithRetry(ctx context.Context, messages []llm.ChatMessage) (*llm.CompletionResponse, error) {
	req := &llm.CompletionRequest{
		Messages:    messages,
		Temperature: llm.Float64Ptr(a.config.Temperature),
	}

	return a.config.Model.Complete(ctx, req)
}

// handleOnError triggers error hook
func (a *ReActAgent) handleOnError(_ context.Context, err error) {
	_ = a.fireHook(HookOnError, &HookContext{Error: err})
}

// Stats returns current agent statistics
func (a *ReActAgent) Stats() AgentStats {
	a.statsMu.RLock()
	stats := a.stats
	a.statsMu.RUnlock()

	// 在锁外设置 status，避免嵌套锁定（lifecycle.Status 有自己的锁）
	stats.Status = a.lifecycle.Status()

	// Deep copy the ToolsCalled map to prevent caller from mutating internal state
	toolsCopy := make(map[string]int, len(stats.ToolsCalled))
	maps.Copy(toolsCopy, stats.ToolsCalled)
	stats.ToolsCalled = toolsCopy
	return stats
}

// recordUsage 记录 LLM Usage 到 CostTracker 和 Metrics
func (a *ReActAgent) recordUsage(usage llm.Usage) {
	if usage.TotalTokens == 0 {
		return
	}

	if ct := a.getCostTracker(); ct != nil {
		modelName := ""
		if info := a.config.Model.Info(); info.Name != "" {
			modelName = info.Name
		}
		_ = ct.Record(modelName, a.config.SessionID, a.config.Name, usage)
	}

	if m := a.getMetricsRecorder(); m != nil {
		modelName := ""
		if info := a.config.Model.Info(); info.Name != "" {
			modelName = info.Name
		}
		m.RecordTokenUsage(modelName, usage.PromptTokens, usage.CompletionTokens)
	}
}

// Stop 优雅停止 Agent，发送停止信号
func (a *ReActAgent) Stop() {
	a.lifecycle.Stop()
	a.logger.Info("Agent 收到停止信号", "name", a.config.Name)
}

// GracefulShutdown 优雅关闭 Agent
// 请求在当前 turn 完成后停止，而不是立即中断
// 如果 ctx 超时仍未完成，则回退到强制停止
func (a *ReActAgent) GracefulShutdown(ctx context.Context) error {
	a.logger.Info("Agent 请求优雅关闭", "name", a.config.Name)
	return a.lifecycle.GracefulShutdown(ctx)
}

// Name 返回 Agent 名称
func (a *ReActAgent) Name() string {
	return a.config.Name
}

// Pause 暂停 Agent
func (a *ReActAgent) Pause() {
	a.lifecycle.Pause()
	a.logger.Info("Agent 已暂停", "name", a.config.Name)
}

// Resume 恢复暂停的 Agent
func (a *ReActAgent) Resume() {
	a.lifecycle.Resume()
	a.logger.Info("Agent 已恢复", "name", a.config.Name)
}

// ResumeFromCheckpoint 从检查点恢复 Agent 执行
// 加载 CheckpointStore 中的状态，从上次中断的位置继续运行
func (a *ReActAgent) ResumeFromCheckpoint(ctx context.Context) (*Response, error) {
	a.runMu.Lock()
	defer a.runMu.Unlock()

	cs := a.getCheckpointStore()
	if cs == nil {
		return nil, fmt.Errorf("checkpoint store not configured")
	}

	state, err := cs.Load(ctx, a.config.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	if state.Status != "paused" && state.Status != "failed" && state.Status != "cancelled" {
		return nil, fmt.Errorf("cannot resume from status %q, expected paused/failed/cancelled", state.Status)
	}

	a.logger.Info("Agent 从检查点恢复", "name", a.config.Name, "turn", state.TurnCount, "saved_at", state.SavedAt)

	history := make([]Message, 0, len(state.Messages))
	for _, m := range state.Messages {
		history = append(history, Message{
			Role:    Role(m.Role),
			Content: m.Content,
		})
	}

	a.startTime = time.Now()
	a.statsMu.Lock()
	a.stats.StartTime = a.startTime
	a.stats.CurrentTurn = state.TurnCount
	a.stats.TotalMessages = len(history)
	a.stats.Status = StatusRunning
	a.statsMu.Unlock()
	_ = a.lifecycle.SetStatus(StatusRunning)

	if m := a.getMetricsRecorder(); m != nil {
		m.IncActiveAgents()
		defer m.DecActiveAgents()
	}

	defer func() {
		if a.lifecycle.Status() != StatusCompleted &&
			a.lifecycle.Status() != StatusFailed &&
			a.lifecycle.Status() != StatusCancelled {
			_ = a.lifecycle.SetStatus(StatusCompleted)
		}
	}()

	_ = a.fireHook(HookBeforeRun, &HookContext{})
	a.publishEvent("agent.resume", map[string]string{"name": a.config.Name, "from_turn": fmt.Sprintf("%d", state.TurnCount)})

	prevMetrics := state.Metrics
	var totalLLMLatency time.Duration
	var totalToolLatency time.Duration
	if d, parseErr := time.ParseDuration(prevMetrics.LLMLatency); parseErr == nil {
		totalLLMLatency = d
	}
	if d, parseErr := time.ParseDuration(prevMetrics.ToolLatency); parseErr == nil {
		totalToolLatency = d
	}
	toolCount := prevMetrics.TotalTools

	return a.runLoop(ctx, history, state.TurnCount, loopConfig{}, totalLLMLatency, totalToolLatency, toolCount)
}




















































































































































// Helper functions for type conversion

func convertToLLMMessages(history []Message) []llm.ChatMessage {
	msgs := make([]llm.ChatMessage, 0, len(history))
	for _, m := range history {
		msg := llm.ChatMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}

		if m.HasMultimodal() {
			msg.Content = buildMultimodalContent(m.ContentParts)
		}

		if m.Role == RoleAssistant && len(m.ToolCalls) > 0 {
			msg.ToolCalls = make([]llm.FunctionCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				msg.ToolCalls[j] = llm.FunctionCall{
					ID:        tc.ID,
					Name:      tc.Name,
					Arguments: tc.Args,
				}
			}
		}
		if m.Role == RoleTool {
			if id, ok := m.Metadata.Extra["tool_call_id"]; ok {
				msg.ToolCallID = id
			}
			if isError, ok := m.Metadata.Extra["is_error"]; ok && isError == "true" {
				msg.IsToolError = true
			}
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

// buildMultimodalContent 将 ContentParts 转换为 OpenAI 兼容的多模态 content JSON 字符串
func buildMultimodalContent(parts []ContentPart) string {
	type contentItem struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		ImageURL *struct {
			URL    string `json:"url"`
			Detail string `json:"detail,omitempty"`
		} `json:"image_url,omitempty"`
	}

	items := make([]contentItem, len(parts))
	for i, p := range parts {
		switch p.Type {
		case "text":
			items[i] = contentItem{Type: "text", Text: p.Text}
		case "image_url":
			items[i] = contentItem{
				Type: "image_url",
				ImageURL: &struct {
					URL    string `json:"url"`
					Detail string `json:"detail,omitempty"`
				}{URL: p.URL, Detail: p.Detail},
			}
		case "image_b64":
			items[i] = contentItem{
				Type: "image_url",
				ImageURL: &struct {
					URL    string `json:"url"`
					Detail string `json:"detail,omitempty"`
				}{URL: "data:" + p.MIME + ";base64," + p.Data, Detail: p.Detail},
			}
		default:
			items[i] = contentItem{Type: "text", Text: p.Text}
		}
	}

	data, _ := json.Marshal(items)
	return string(data)
}

func convertToToolCalls(calls []llm.FunctionCall) []ToolCall {
	tcs := make([]ToolCall, len(calls))
	for i, c := range calls {
		tcs[i] = ToolCall{
			ID:   c.ID,
			Name: c.Name,
			Args: c.Arguments,
		}
	}
	return tcs
}
