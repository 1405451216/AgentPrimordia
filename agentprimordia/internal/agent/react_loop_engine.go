// react_loop_engine.go — ReAct 循环引擎入口
// 包含 reactLoopEngine 函数，负责初始化环境、构建系统提示词、启动 runLoop
package agent

import (
	"context"
	"fmt"
	"time"
)

// reactLoopEngine ReAct 循环核心引擎
// 统一处理流式和非流式两种运行模式，消除 Run/StreamRun 之间的代码重复
func (a *ReActAgent) reactLoopEngine(ctx context.Context, input Message, cfg loopConfig) (resp *Response, err error) {
	a.runMu.Lock()
	defer a.runMu.Unlock()

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

