// react_loop_engine.go — ReAct 循环引擎入口
// 包含 reactLoopEngine 函数，负责初始化环境、构建系统提示词、启动 runLoop
package agent

import (
	"context"
	"fmt"
	"time"

	"agentprimordia/internal/observability"
)

// reactLoopEngine ReAct 循环核心引擎
// 统一处理流式和非流式两种运行模式，消除 Run/StreamRun 之间的代码重复
func (a *ReActAgent) reactLoopEngine(ctx context.Context, input Message, cfg loopConfig) (resp *Response, err error) {
	a.runMu.Lock()
	defer a.runMu.Unlock()

	// v3.4-6：运行以失败结束时自动记录失败记录（含失败时检查点）。
	// 最先注册、LIFO 最后执行——即使 panic 恢复后的 err 也能被捕获。
	defer func() { a.recordFailure(ctx, input, err) }()

	// v3.4-4：输入端护栏——用户输入进入循环前检查（脱敏或拒绝）
	if guard := a.getInputGuard(); guard != nil {
		sanitized, blocked, gerr := guard(input.Content)
		if gerr != nil {
			a.logger.Warn("输入端护栏检查失败", "error", gerr)
			_ = a.lifecycle.SetStatus(StatusFailed)
			return nil, fmt.Errorf("input guard check failed: %w", gerr)
		}
		if blocked {
			a.logger.Warn("输入端护栏拒绝输入", "name", a.config.Name)
			_ = a.lifecycle.SetStatus(StatusFailed)
			return &Response{RequestID: cfg.requestID, Error: ErrInputBlocked}, ErrInputBlocked
		}
		if sanitized != "" && sanitized != input.Content {
			a.logger.Debug("输入端护栏脱敏输入", "name", a.config.Name)
			input.Content = sanitized
		}
	}

	// 创建根 Span，包裹整个 Agent 执行生命周期
	var rootSpan Span = &NoopSpan{}
	if tracer := a.getTracer(); tracer != nil {
		rootSpan = tracer.Start(
			"agent.run",
			SpanKindServer,
			WithAttributes(map[string]any{
				"agent":   a.config.Name,
				"session": a.config.SessionID,
				"stream":  cfg.stream,
			}),
		)
	}

	defer func() {
		if r := recover(); r != nil {
			a.logger.Error("ReAct 循环 panic 恢复", "error", r)
			_ = a.lifecycle.SetStatus(StatusFailed)
			a.publishEvent(EventAgentPanic, map[string]string{"name": a.config.Name, "error": fmt.Sprintf("%v", r)})
			err = fmt.Errorf("agent panic recovered: %v", r)
			resp = &Response{RequestID: cfg.requestID, Error: err}
			rootSpan.SetStatus(SpanStatusError, fmt.Sprintf("panic: %v", r))
		}
		rootSpan.End()
		// v3.5-4：结束请求全链路关联（trace 闭合）
		if a.capCache != nil && a.capCache.observability != nil && a.capCache.traceID != "" {
			a.capCache.observability.End(a.capCache.traceID)
		}
		// 优化（Task 1）：flush 异步记忆写入队列，确保所有 saveMemory 调用完成
		a.flushMemoryWriter()
		// 清理 capCache，避免下次 Run() 误用旧引用
		a.capCache = nil
	}()

	// 在 statsMu 保护下写入 startTime：Stats() 会在锁内读取（-race 实测发现
	// 无锁写与并发 Stats() 读取构成数据竞争）
	now := time.Now()
	a.statsMu.Lock()
	a.startTime = now
	a.stats.StartTime = now
	a.stats.RequestID = cfg.requestID
	a.statsMu.Unlock()
	a.hookCtx = ctx
	_ = a.lifecycle.SetStatus(StatusRunning)
	a.stats.Status = StatusRunning

	// 优化（Task 2）：Run() 入口处一次性查找所有能力引用，避免每轮重复类型断言
	a.capCache = a.resolveCapabilities(cfg.requestID)

	// v3.5-4：登记全链路关联——以 root span 的 trace_id 关联 trace/metrics/audit
	if corr := a.capCache.observability; corr != nil {
		traceID := rootSpan.SpanContext().TraceID
		if traceID == "" {
			traceID = cfg.requestID // 无 tracer 时回退到 request_id 作为关联键
		}
		a.capCache.traceID = traceID
		corr.Start(traceID, a.config.Name, a.config.SessionID)
		corr.AddSpan(traceID, observability.SpanRecord{
			Name:       "agent.run",
			Kind:       string(SpanKindServer),
			SpanID:     rootSpan.SpanContext().SpanID,
			Status:     string(SpanStatusOK),
			Attributes: map[string]any{"agent": a.config.Name, "session": a.config.SessionID, "stream": cfg.stream},
		})
	}

	if cfg.stream {
		a.logger.Info("Agent 流式启动", "name", a.config.Name, "session", a.config.SessionID)
	} else {
		a.logger.Info("Agent 启动", "name", a.config.Name, "session", a.config.SessionID)
	}
	a.publishEvent(EventAgentStart, map[string]string{"name": a.config.Name})
	_ = a.fireHook(HookBeforeRun, &HookContext{})

	// p2t4：写入 AgentStart 审计事件
	a.writeAudit(ctx, AuditEvent{
		Actor:    a.config.Name,
		Action:   auditActionAgentStart,
		Resource: cfg.requestID,
		Result:   auditResultSuccess,
		Details: map[string]any{
			"session_id": a.config.SessionID,
			"stream":     cfg.stream,
			"max_turns":  a.config.MaxTurns,
		},
	})

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
		// flushMemoryWriter 在外层 defer 已调用，此处不再重复等待
		a.publishEvent(EventAgentStop, map[string]string{
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
			systemPrompt = a.config.Name + ": AI assistant ready to help."
		}
	}

	history := []Message{}
	if systemPrompt != "" {
		history = append(history, SystemMessage(systemPrompt))
	}
	history = append(history, input)
	a.saveMemory(ctx, input)

	return a.runLoop(ctx, history, 0, cfg, 0, 0, 0, rootSpan.SpanContext())
}
