// react_persist.go — 记忆保存 + 检查点 + 上下文裁剪
// 负责 Agent 运行过程中的持久化操作：记忆存储、检查点保存、上下文窗口裁剪
package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"agentprimordia/internal/memory"
	"agentprimordia/internal/persist"
)

// saveMemoryChBuffer 是异步 saveMemory 队列容量。
// 高吞吐场景下消息数远大于此值时会触发丢弃，但持久化路径不应阻塞主循环。
const saveMemoryChBuffer = 256

// memoryWriter 封装异步记忆写入队列，从 ReActAgent 中剥离独立管理。
// 每个 Run() 都有独立的 channel + goroutine + doneCh，
// flush 关闭 channel 后等待 doneCh，下次 saveMemory 重新创建。
type memoryWriter struct {
	ch     chan *memory.Episode
	doneCh chan struct{}
	mu     sync.Mutex
	logger *slog.Logger
	mem    MemoryStore
}

// newMemoryWriter 创建 memoryWriter 实例（不启动 goroutine，首次 submit 时懒启动）
func newMemoryWriter(mem MemoryStore, logger *slog.Logger) *memoryWriter {
	return &memoryWriter{logger: logger, mem: mem}
}

// ensureStarted 确保消费 goroutine 已启动（懒启动，锁内完成）
func (w *memoryWriter) ensureStarted() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ch == nil {
		ch := make(chan *memory.Episode, saveMemoryChBuffer)
		w.ch = ch
		doneCh := make(chan struct{})
		w.doneCh = doneCh
		mem := w.mem
		logger := w.logger
		go func() {
			defer close(doneCh)
			for ep := range ch {
				writeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := mem.Add(writeCtx, ep); err != nil {
					logger.Warn("保存记忆失败", "error", err, "role", ep.Role)
				}
				cancel()
			}
		}()
	}
}

// submit 非阻塞提交 Episode 到写入队列
func (w *memoryWriter) submit(ep *memory.Episode) {
	w.ensureStarted()
	select {
	case w.ch <- ep:
	default:
		w.logger.Warn("异步记忆队列已满，丢弃写入", "role", ep.Role)
	}
}

// flush 关闭写入队列并等待所有待写入完成
// 关闭后下次 submit 会重新创建 channel 和 goroutine
func (w *memoryWriter) flush() {
	w.mu.Lock()
	ch := w.ch
	doneCh := w.doneCh
	w.ch = nil
	w.doneCh = nil
	w.mu.Unlock()

	if ch == nil {
		return
	}
	close(ch)
	if doneCh != nil {
		<-doneCh
	}
}

// saveMemory 将消息保存到 Memory。
// 优化（Task 1）：将 mem.Add() 写入异步化到独立的 goroutine 中，
// 避免 SQLite 同步写入阻塞 ReAct 主循环。
// 优化（perf-v3）：优先使用 capCache 缓存的 memoryStore 和 summarizer，避免每轮重复类型断言。
func (a *ReActAgent) saveMemory(ctx context.Context, msg Message) {
	// 优先使用 capCache 中缓存的能力引用，避免每轮重复类型断言
	var mem MemoryStore
	var summarizer memory.SummaryExtractor
	if a.capCache != nil {
		mem = a.capCache.memoryStore
		summarizer = a.capCache.summarizer
	} else {
		mem = a.getMemoryStore()
		summarizer = a.getSummarizer()
	}
	if mem == nil {
		return
	}
	ep := &memory.Episode{
		ID:        a.idGen.next(),
		SessionID: a.config.SessionID,
		Role:      string(msg.Role),
		Content:   msg.Content,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if ep.SessionID == "" {
		ep.SessionID = a.config.Name
	}

	// 非阻塞提交到异步写入队列
	if a.memWriter == nil {
		a.memWriter = newMemoryWriter(mem, a.logger)
	} else {
		// 更新 memWriter 的 mem 引用（可能因 capCache 变化而不同）
		a.memWriter.mem = mem
	}
	a.memWriter.submit(ep)

	// 异步提取摘要（绑定到 agent 的 hookCtx 防止泄漏）
	if summarizer != nil && ep.ID != "" {
		epID := ep.ID
		epContent := ep.Content
		parentCtx := a.hookCtx
		if parentCtx == nil {
			parentCtx = context.Background()
		}
		go func() {
			defer func() {
				if r := recover(); r != nil {
					a.logger.Warn("异步摘要提取 panic", "error", r)
				}
			}()
			// 使用父级 context + 超时，防止泄漏
			sumCtx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
			defer cancel()
			result, err := summarizer.ExtractSummary(sumCtx, epContent)
			if err != nil {
				a.logger.Warn("异步摘要提取失败", "id", epID, "error", err)
				return
			}
			a.logger.Info("异步摘要提取成功", "id", epID, "summary_len", len(result.Summary), "topics", result.Topics)
			// M2 修复：存储摘要结果，不再只记录日志丢弃
			if err := mem.UpdateSummary(sumCtx, epID, result.Summary, result.Topics); err != nil {
				a.logger.Warn("异步摘要存储失败", "id", epID, "error", err)
			}
		}()
	}
}

// flushMemoryWriter 关闭异步写入队列并等待所有待写入完成。
// 应在 agent 运行结束（reactLoopEngine 的 defer）时调用。
// 关闭后下次 saveMemory 会重新创建 channel 和 goroutine。
func (a *ReActAgent) flushMemoryWriter() {
	if a.memWriter != nil {
		a.memWriter.flush()
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

// defaultMaxHistoryMessages 默认历史消息保留上限（perf-v4 Task 11）
// 默认场景下 history 不再无界增长，同时减少 LLM token 消耗
const defaultMaxHistoryMessages = 100

// trimContext 应用上下文窗口策略裁剪历史
// perf-v4 Task 11：当未配置自定义策略时，默认使用滑动窗口
// （保留系统提示词 + 最近 N 条消息），避免长对话无界增长
// 优化（perf-v3）：优先使用 capCache 缓存的 contextWindow，避免每轮重复类型断言
func (a *ReActAgent) trimContext(history []Message, maxMessages int) []Message {
	var cw ContextWindowStrategy
	if a.capCache != nil {
		cw = a.capCache.contextWindow
	} else {
		cw = a.getContextWindowStrategy()
	}
	if cw != nil {
		return cw.Trim(history, maxMessages)
	}
	// 默认滑动窗口策略
	if maxMessages <= 0 {
		maxMessages = defaultMaxHistoryMessages
	}
	if len(history) <= maxMessages {
		return history
	}
	// 保留第一条系统消息（如果有）+ 最近 N-1 条
	result := make([]Message, 0, maxMessages)
	start := 0
	if len(history) > 0 && history[0].Role == RoleSystem {
		result = append(result, history[0])
		start = 1
	}
	tail := maxMessages - len(result)
	if tail < 0 {
		tail = 0
	}
	nonSystem := history[start:]
	if len(nonSystem) > tail {
		nonSystem = nonSystem[len(nonSystem)-tail:]
	}
	result = append(result, nonSystem...)
	return result
}
