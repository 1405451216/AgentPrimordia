// react_loop.go — ReAct 循环核心引擎
// 包含 ReActAgent 结构体定义、构造函数、Run/StreamRun 入口、reactLoopEngine 和 runLoop 核心循环
package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"agentprimordia/internal/llm"
	"agentprimordia/internal/memory"
	"agentprimordia/internal/persist"
	"agentprimordia/internal/tools"
	"agentprimordia/pkg/logger"
)

// idCounter 用于生成唯一 ID 的原子计数器
type idCounter struct {
	n int64
}

// next 生成唯一的记忆 ID（优化：避免 fmt.Sprintf）
func (c *idCounter) next() string {
	n := atomic.AddInt64(&c.n, 1)
	ts := time.Now().UnixNano()
	return "msg_" + strconv.FormatInt(ts, 10) + "_" + strconv.FormatInt(n, 10)
}

// ===== 集成接口 =====

// MemoryStore 是 Agent 所需的记忆存储接口
type MemoryStore interface {
	Add(ctx context.Context, episode *memory.Episode) error
	// UpdateSummary 更新指定 episode 的摘要和标签（M2 修复：异步摘要存储）。
	UpdateSummary(ctx context.Context, id, summary, topics string) error
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

// LabeledMetricsRecorder 可选接口：带标签维度的指标记录
// 实现此接口的记录器（如 AgentMetrics）可输出 provider/model/tool_name 等维度，
// 供 Grafana PromQL 多维聚合查询使用。
// Agent 运行时通过类型断言发现此能力，未实现则回退到无标签记录。
type LabeledMetricsRecorder interface {
	RecordLLMCallWithLabels(duration time.Duration, err error, provider, model string)
	RecordToolCallWithLabels(duration time.Duration, err error, toolName string)
	RecordTurnWithAgent(duration time.Duration, agentName string)
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

	// Lifecycle 生命周期管理器（默认自动创建）
	Lifecycle *Lifecycle

	// Logger 结构化日志，默认使用 logger.Default()
	Logger *slog.Logger
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
	mu        sync.Mutex
	hitlMgr   *HITLManager
	idGen     idCounter // 实例级 ID 生成器，消除全局可变状态

	// currentRequestID 当前运行的请求 ID，用于可观测性关联
	currentRequestID string

	// memWg 追踪异步记忆写入 goroutine，agent 结束时等待刷盘
	memWg sync.WaitGroup

	// hookCtx 用于 fireHook 调用，绑定到当前运行的 context
	// 确保 agent 取消时 hook 也能被取消
	hookCtx context.Context

	// self 自引用，指向最外层的 Agent 包装器
	// 用于协议式微内核的接口发现：引擎通过 a.self.(XxxCapable) 检测能力
	// 默认指向自身；WithXxx 链式调用时更新为 CapabilityAgent
	self Agent

	// ===== Task 1: saveMemory 异步化 =====
	// 每个 Run() 都有独立的 channel + goroutine + doneCh
	// flushMemoryWriter 关闭 channel 后等待 doneCh，下次 Run 重新创建
	memoryCh      chan *memory.Episode
	memoryDoneCh  chan struct{}
	memorySetupMu sync.Mutex

	// ===== Task 1.5: Executor 复用 =====
	// 缓存的 *tools.Executor，避免每轮工具调用时重新分配
	toolExecutor     *tools.Executor
	toolExecutorOnce sync.Once

	// ===== Task 2: 能力查找缓存 =====
	// 单次 Run() 期间不变的能力引用，reactLoopEngine 入口处一次性查找
	// runLoop 内通过 capCache 访问，避免每轮重复类型断言
	capCache *capabilityCache
}

// NewReActAgent 创建基于 ReAct 循环的 Agent 实例
//
// Deprecated: 使用 NewAgent 代替。NewReActAgent 仅接受标量配置，能力通过链式 API 注入。
// NewAgent 通过 Functional Options 注入能力，构造后不可变。
// 将在 v1.0.0 中移除。
// 迁移指南: ecosystem/docs/migration/v0-deprecations.md
func NewReActAgent(cfg ReActConfig) *ReActAgent {
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = 50
	}
	if cfg.Lifecycle == nil {
		cfg.Lifecycle = NewLifecycle()
	}
	if cfg.Logger == nil {
		cfg.Logger = logger.Default()
	}

	a := &ReActAgent{
		config:    cfg,
		lifecycle: cfg.Lifecycle,
		hooks:     nil, // 通过链式 API WithHooks 注入
		logger:    cfg.Logger,
		stats: AgentStats{
			Status:      StatusIdle,
			ToolsCalled: make(map[string]int),
		},
		hitlMgr: nil, // 通过链式 API WithHITL 注入
	}
	a.initSelf()
	return a
}

// loopConfig 循环配置
type loopConfig struct {
	stream    bool
	streamCh  chan StreamEvent
	streamCtx context.Context
	requestID string
}

// capabilityCache 缓存单次 Run() 期间不变的能力查找结果。
// 优化（Task 2）：ReAct 循环中每轮调用的 getTracer/getCostTracker/getMemoryStore 等
// 在 Run() 入口一次性查找并缓存到此处，避免每轮重复类型断言。
// 优化（perf-v2）：新增 toolDefinitions 缓存，避免每轮重复转换工具定义。
type capabilityCache struct {
	requestID        string
	tracer           Tracer
	costTracker      *CostTracker
	memoryStore      MemoryStore
	metricsRecorder  MetricsRecorder
	labeledRecorder  LabeledMetricsRecorder
	eventPublisher   EventPublisher
	checkpointStore  persist.CheckpointStore
	contextWindow    ContextWindowStrategy
	summarizer       memory.SummaryExtractor
	fileScope        []string
	toolkit          *tools.Registry
	toolDefinitions  []llm.ToolDefinition // 优化（perf-v2）：缓存转换后的工具定义
	systemInfoCached bool
	provider         string
	model            string
}

// resolveCapabilities 一次性查找所有能力并填充到 capabilityCache。
// 必须在 reactLoopEngine 入口处调用，且只在 Run() 期间使用。
func (a *ReActAgent) resolveCapabilities(requestID string) *capabilityCache {
	c := &capabilityCache{
		requestID:       requestID,
		tracer:          a.getTracer(),
		costTracker:     a.getCostTracker(),
		memoryStore:     a.getMemoryStore(),
		metricsRecorder: a.getMetricsRecorder(),
		eventPublisher:  a.getEventPublisher(),
		checkpointStore: a.getCheckpointStore(),
		contextWindow:   a.getContextWindowStrategy(),
		summarizer:      a.getSummarizer(),
		fileScope:       a.getFileScope(),
		toolkit:         a.getToolkit(),
	}
	// 缓存 labeled 记录器
	if c.metricsRecorder != nil {
		if lm, ok := c.metricsRecorder.(LabeledMetricsRecorder); ok {
			c.labeledRecorder = lm
		}
	}
	// 缓存模型信息（用于 recordLLM）
	if info := a.config.Model.Info(); info.Name != "" {
		c.systemInfoCached = true
		c.provider = info.Provider
		c.model = info.Name
	}
	// 优化（perf-v2）：预转换工具定义，避免每轮重复转换
	if c.toolkit != nil {
		if defs := c.toolkit.Definitions(); len(defs) > 0 {
			c.toolDefinitions = convertToolDefsToLLMDefinitions(defs)
		}
	}
	return c
}

// emitStream 在流式模式下向通道发送事件
func (a *ReActAgent) emitStream(cfg loopConfig, event StreamEvent) {
	if cfg.stream && cfg.streamCh != nil {
		// 自动填充请求 ID
		if event.RequestID == "" {
			event.RequestID = cfg.requestID
		}
		select {
		case cfg.streamCh <- event:
		case <-cfg.streamCtx.Done():
		}
	}
}

// Run executes the agent with the given input message
func (a *ReActAgent) Run(ctx context.Context, input Message) (*Response, error) {
	// 注入请求 ID：若 context 中已有则复用，否则生成新的
	reqID := RequestIDFromCtx(ctx)
	if reqID == "" {
		reqID = NewRequestID()
		ctx = WithRequestID(ctx, reqID)
	}
	a.mu.Lock()
	a.currentRequestID = reqID
	a.mu.Unlock()

	return a.reactLoopEngine(ctx, input, loopConfig{stream: false, requestID: reqID})
}

// StreamRun 执行 Agent 并以流式方式输出结果
// 返回一个事件 channel，调用者可以逐事件消费
func (a *ReActAgent) StreamRun(ctx context.Context, input Message) (<-chan StreamEvent, error) {
	// 注入请求 ID
	reqID := RequestIDFromCtx(ctx)
	if reqID == "" {
		reqID = NewRequestID()
		ctx = WithRequestID(ctx, reqID)
	}
	a.mu.Lock()
	a.currentRequestID = reqID
	a.mu.Unlock()

	ch := make(chan StreamEvent, 32)
	go func() {
		defer close(ch)
		if _, err := a.reactLoopEngine(ctx, input, loopConfig{stream: true, streamCh: ch, streamCtx: ctx, requestID: reqID}); err != nil {
			select {
			case ch <- StreamEvent{Type: StreamEventError, RequestID: reqID, Content: err.Error()}:
			case <-ctx.Done():
			}
		}
	}()
	return ch, nil
}

// reactLoopEngine ReAct 循环核心引擎
// 统一处理流式和非流式两种运行模式，消除 Run/StreamRun 之间的代码重复
// runLoop ReAct 循环核心体，被 reactLoopEngine 和 ResumeFromCheckpoint 共享
// 封装从 startTurn 开始的主循环逻辑，包括 LLM 调用、工具执行、checkpoint 保存等
func (a *ReActAgent) runLoop(ctx context.Context, history []Message, startTurn int, cfg loopConfig, totalLLMLatency time.Duration, totalToolLatency time.Duration, toolCount int) (*Response, error) {
	// 优化（Task 2）：从 capCache 一次性取所有能力引用；capCache 由 reactLoopEngine 在 Run 入口处填充
	tracer := Tracer(nil)
	costTracker := (*CostTracker)(nil)
	if a.capCache != nil {
		tracer = a.capCache.tracer
		costTracker = a.capCache.costTracker
	}
	for turn := startTurn; turn < a.config.MaxTurns; turn++ {
		turnStart := time.Now()

		if a.lifecycle.IsStopped() {
			_ = a.lifecycle.SetStatus(StatusCancelled)
			a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: ErrAgentStopped.Error()})
			return &Response{RequestID: cfg.requestID, Error: ErrAgentStopped}, ErrAgentStopped
		}

		if ctx.Err() != nil {
			_ = a.lifecycle.SetStatus(StatusCancelled)
			a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: ctx.Err().Error()})
			return &Response{RequestID: cfg.requestID, Error: ctx.Err()}, ctx.Err()
		}

		a.statsMu.Lock()
		a.stats.CurrentTurn = turn + 1
		a.stats.TotalMessages = len(history)
		a.statsMu.Unlock()

		_ = a.fireHook(HookBeforeTurn, &HookContext{Turn: turn})
		// 优化（Task 3）：仅在存在订阅者时构造 payload map，避免热点路径上的堆分配
		if a.hasEventSubscriber() {
			a.publishEvent("turn.start", map[string]int{"turn": turn})
		}

		// 优化（Task 2）：使用外层 capCache 缓存的 tracer，避免每轮重复类型断言

		var turnSpan Span = &NoopSpan{}
		if tracer != nil {
			turnSpan = tracer.Start(
				"turn."+strconv.Itoa(turn),
				SpanKindInternal,
				WithAttributes(map[string]any{"agent": a.config.Name, "turn": turn}),
			)
		}

		if costTracker != nil && costTracker.CheckBudget() {
			a.logger.Warn("Agent 超出预算", "name", a.config.Name)
			_ = a.lifecycle.SetStatus(StatusFailed)
			return &Response{RequestID: cfg.requestID, Error: ErrBudgetExceeded}, ErrBudgetExceeded
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
			ragContext, ragDocs := a.searchRAG(ctx, ragQuery)
			if ragContext != "" {
				history = a.injectRAGContext(history, ragContext)
				a.logger.Debug("RAG 上下文已注入", "turn", turn, "query_len", len(ragQuery), "docs", len(ragDocs))
				a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: "[RAG] 知识库上下文已注入"})
			}
		}

		trimmedHistory := a.trimContext(history, 0)
		llmMessages := convertToLLMMessages(trimmedHistory)

		// 优化（Task 2 / Task 2.5 / perf-v2）：使用 capCache.toolkit 和预转换的 toolDefinitions，
		// 避免每轮重复获取 toolkit 和反解工具定义
		var toolDefinitions []llm.ToolDefinition
		if a.capCache != nil && a.capCache.toolDefinitions != nil {
			toolDefinitions = a.capCache.toolDefinitions
		} else {
			var toolDefs []map[string]any
			var toolkit *tools.Registry
			if a.capCache != nil {
				toolkit = a.capCache.toolkit
			} else {
				toolkit = a.getToolkit()
			}
			if toolkit != nil {
				toolDefs = toolkit.Definitions()
			}
			toolDefinitions = convertToolDefsToLLMDefinitions(toolDefs)
		}

		llmStart := time.Now()
		if a.hasEventSubscriber() {
			a.publishEvent("llm.call", map[string]int{"turn": turn})
		}

		var llmSpan Span = &NoopSpan{}
		if tracer != nil {
			llmSpan = tracer.Start(
				"llm.call",
				SpanKindClient,
				WithParent(turnSpan.SpanContext()),
				WithAttributes(map[string]any{"agent": a.config.Name, "turn": turn}),
			)
		}

		var thought Thought

		if cfg.stream {
			thought = a.streamReasoning(ctx, cfg, llmMessages, toolDefinitions, llmStart)
			if thought.Content == "" && len(thought.ToolCalls) == 0 {
				return &Response{RequestID: cfg.requestID, Error: fmt.Errorf("stream reasoning failed")}, fmt.Errorf("stream reasoning failed")
			}
		} else {
			var err error
			thought, err = a.syncReasoning(ctx, llmMessages, toolDefinitions, llmStart)
			if err != nil {
				a.handleOnError(ctx, err)
				_ = a.lifecycle.SetStatus(StatusFailed)
				return &Response{RequestID: cfg.requestID, Error: err}, err
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
		if a.hasEventSubscriber() {
			a.publishEvent("llm.response", map[string]int{"turn": turn})
		}

		if len(thought.ToolCalls) == 0 {
			duration := time.Since(a.startTime)

			response := &Response{
				RequestID: cfg.requestID,
				Content:   thought.Content,
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
			a.recordTurn(time.Since(turnStart))
			if a.hasEventSubscriber() {
				a.publishEvent("turn.end", map[string]int{"turn": turn})
			}

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
			if a.hasEventSubscriber() {
				a.publishEvent("tool.call", map[string]string{"tool": tc.Name, "turn": strconv.Itoa(turn)})
			}

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
			if tracer != nil {
				toolSpan = tracer.Start(
					"tool."+tc.Name,
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
			a.recordTool(toolLatency, err, tc.Name)

			if err != nil {
				a.emitStream(cfg, StreamEvent{Type: StreamEventError, Content: fmt.Sprintf("tool %s error: %v", tc.Name, err)})
				_ = a.fireHook(HookOnError, &HookContext{Error: err, Turn: turn})
				if a.hasEventSubscriber() {
					a.publishEvent("agent.error", map[string]string{"tool": tc.Name, "error": err.Error()})
				}
			} else {
				a.emitStream(cfg, StreamEvent{Type: StreamEventToolResult, Content: result.Content, Data: result})
			}

			if cfg.stream {
				if err == nil {
					_ = a.fireHook(HookAfterTool, &HookContext{ToolResult: result, Turn: turn})
				}
			} else {
				_ = a.fireHook(HookAfterTool, &HookContext{ToolResult: result, Turn: turn})
				if a.hasEventSubscriber() {
					a.publishEvent("tool.result", map[string]string{"tool": tc.Name})
				}
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
		a.recordTurn(time.Since(turnStart))
		if a.hasEventSubscriber() {
			a.publishEvent("turn.end", map[string]int{"turn": turn})
		}

		if a.lifecycle.IsGracefulShutdown() {
			a.logger.Info("Agent 优雅关闭：当前 turn 已完成，退出循环", "name", a.config.Name, "turn", turn+1)
			_ = a.lifecycle.SetStatusWithReason(StatusCancelled, "graceful shutdown")
			duration := time.Since(a.startTime)
			response := &Response{
				RequestID: cfg.requestID,
				Content:   thought.Content,
				Error:     ErrAgentStopped,
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
		RequestID: cfg.requestID,
		Error:     ErrMaxTurnsExceeded,
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

func (a *ReActAgent) reactLoopEngine(ctx context.Context, input Message, cfg loopConfig) (resp *Response, err error) {
	a.runMu.Lock()
	defer a.runMu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			a.logger.Error("ReAct 循环 panic 恢复", "error", r)
			_ = a.lifecycle.SetStatus(StatusFailed)
			a.publishEvent("agent.panic", map[string]string{"name": a.config.Name, "error": fmt.Sprintf("%v", r)})
			err = fmt.Errorf("agent panic recovered: %v", r)
			resp = &Response{RequestID: cfg.requestID, Error: err}
		}
		// 优化（Task 1）：flush 异步记忆写入队列，确保所有 saveMemory 调用完成
		a.flushMemoryWriter()
		// 清理 capCache，避免下次 Run() 误用旧引用
		a.capCache = nil
	}()

	a.startTime = time.Now()
	a.stats.StartTime = a.startTime
	a.stats.RequestID = cfg.requestID
	a.hookCtx = ctx
	_ = a.lifecycle.SetStatus(StatusRunning)
	a.stats.Status = StatusRunning

	// 优化（Task 2）：Run() 入口处一次性查找所有能力引用，避免每轮重复类型断言
	a.capCache = a.resolveCapabilities(cfg.requestID)

	if cfg.stream {
		a.logger.Info("Agent 流式启动", "name", a.config.Name, "session", a.config.SessionID)
	} else {
		a.logger.Info("Agent 启动", "name", a.config.Name, "session", a.config.SessionID)
	}
	a.publishEvent("agent.start", map[string]string{"name": a.config.Name})
	_ = a.fireHook(HookBeforeRun, &HookContext{})

	if a.capCache.metricsRecorder != nil {
		a.capCache.metricsRecorder.IncActiveAgents()
		defer a.capCache.metricsRecorder.DecActiveAgents()
	}

	defer func() {
		if a.lifecycle.Status() != StatusCompleted &&
			a.lifecycle.Status() != StatusFailed &&
			a.lifecycle.Status() != StatusCancelled {
			_ = a.lifecycle.SetStatus(StatusCompleted)
		}
		// 等待所有异步记忆写入完成，避免 agent 退出后 goroutine 悬空
		a.memWg.Wait()
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
