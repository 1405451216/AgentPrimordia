// react_capabilities.go — 协议式微内核接口发现 + Hook/Event 辅助
// 引擎通过 a.self.(XxxCapable) 检测能力，优先使用接口发现，回退到 config 字段
package agent

import (
	"context"
	"time"

	"agentprimordia/internal/memory"
	"agentprimordia/internal/persist"
)

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

// getLabeledRecorder 尔回带标签维度的 MetricsRecorder（可能为 nil）
func (a *ReActAgent) getLabeledRecorder() LabeledMetricsRecorder {
	if m := a.getMetricsRecorder(); m != nil {
		if lm, ok := m.(LabeledMetricsRecorder); ok {
			return lm
		}
	}
	return nil
}

// recordLLM 记录 LLM 调用，优先使用带标签的记录器（内部已调用 RecordLLMCall）
func (a *ReActAgent) recordLLM(duration time.Duration, err error) {
	if lm := a.getLabeledRecorder(); lm != nil {
		provider, model := "", ""
		if info := a.config.Model.Info(); info.Name != "" {
			provider = info.Provider
			model = info.Name
		}
		lm.RecordLLMCallWithLabels(duration, err, provider, model)
	} else if m := a.getMetricsRecorder(); m != nil {
		m.RecordLLMCall(duration, err)
	}
}

// recordTool 记录工具调用，优先使用带标签的记录器（内部已调用 RecordToolCall）
func (a *ReActAgent) recordTool(duration time.Duration, err error, toolName string) {
	if lm := a.getLabeledRecorder(); lm != nil {
		lm.RecordToolCallWithLabels(duration, err, toolName)
	} else if m := a.getMetricsRecorder(); m != nil {
		m.RecordToolCall(duration, err)
	}
}

// recordTurn 记录 Turn 耗时，优先使用带标签的记录器（内部已调用 RecordTurn）
func (a *ReActAgent) recordTurn(duration time.Duration) {
	if lm := a.getLabeledRecorder(); lm != nil {
		lm.RecordTurnWithAgent(duration, a.config.Name)
	} else if m := a.getMetricsRecorder(); m != nil {
		m.RecordTurn(duration)
	}
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
		// 自动填充请求 ID 和 Agent ID
		if hctx.RequestID == "" {
			a.mu.Lock()
			hctx.RequestID = a.currentRequestID
			a.mu.Unlock()
		}
		if hctx.AgentID == "" {
			hctx.AgentID = a.config.Name
		}
		// 使用 agent 的运行 context 而非 Background，确保取消能传播到 hook
		ctx := a.hookCtx
		if ctx == nil {
			ctx = context.Background()
		}
		if err := a.hooks.Fire(ctx, hctx); err != nil {
			a.logger.Warn("Hook 执行失败", "point", point, "error", err)
			return err
		}
	}
	return nil
}

// publishEvent 向 EventPublisher 发布事件，自动注入 request_id
func (a *ReActAgent) publishEvent(eventType string, payload any) {
	if ep := a.getEventPublisher(); ep != nil {
		// 如果 payload 是 map[string]string，注入 request_id
		if m, ok := payload.(map[string]string); ok {
			a.mu.Lock()
			m["request_id"] = a.currentRequestID
			a.mu.Unlock()
		}
		if err := ep.PublishAsync(eventType, a.config.Name, payload); err != nil {
			a.logger.Warn("发布事件失败", "error", err, "type", eventType)
		}
	}
}
