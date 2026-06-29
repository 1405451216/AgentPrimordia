package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// hookContextPool 复用 HookContext 实例。
// ReAct 循环每轮多次 fireHook，每次都 &HookContext{...} 产生新的逃逸对象。
// 通过 sync.Pool 复用底层结构，可显著降低高并发场景下的分配压力。
// 注意：Metadata map 仍按需分配，结构体本身可被安全复用（Reset 时清空）。
var hookContextPool = sync.Pool{
	New: func() any {
		return &HookContext{}
	},
}

// AcquireHookContext 从池中获取一个已重置的 HookContext。
// 调用方负责在使用完毕后调用 ReleaseHookContext 归还。
func AcquireHookContext() *HookContext {
	hctx := hookContextPool.Get().(*HookContext)
	hctx.Reset()
	return hctx
}

// ReleaseHookContext 归还 HookContext 到池中。
// 归还前会重置所有字段（包括内部指针），避免内存泄漏。
// 仅当 hctx 非 nil 时归还。
func ReleaseHookContext(hctx *HookContext) {
	if hctx == nil {
		return
	}
	// 显式清空引用，断开与外部对象的强引用，避免池中对象持有已释放对象。
	hctx.Reset()
	hookContextPool.Put(hctx)
}

// Reset 清空 HookContext 的所有字段。
// 注意：map 字段保留底层 bucket 复用，仅 delete 所有键。
func (h *HookContext) Reset() {
	if h == nil {
		return
	}
	h.AgentID = ""
	h.RequestID = ""
	h.SessionID = ""
	h.Point = ""
	h.Turn = 0
	h.Message = nil
	h.Response = nil
	h.ToolCall = nil
	h.ToolResult = nil
	h.Error = nil
	if h.Metadata != nil {
		for k := range h.Metadata {
			delete(h.Metadata, k)
		}
	}
	h.StreamChunk = nil
	h.Duration = 0
	h.OldState = ""
	h.NewState = ""
	h.Reason = ""
	h.MemoryQuery = ""
	h.MemoryResult = nil
	h.ContextWindowUsage = 0
	h.ContextWindowLimit = 0
}

type HookPoint string

const (
	HookBeforeRun           HookPoint = "before_run"
	HookAfterRun            HookPoint = "after_run"
	HookBeforeTurn          HookPoint = "before_turn"
	HookAfterTurn           HookPoint = "after_turn"
	HookBeforeLLM           HookPoint = "before_llm"
	HookAfterLLM            HookPoint = "after_llm"
	HookBeforeTool          HookPoint = "before_tool"
	HookAfterTool           HookPoint = "after_tool"
	HookOnError             HookPoint = "on_error"
	HookOnComplete          HookPoint = "on_complete"
	HookBeforeRAG           HookPoint = "before_rag"
	HookAfterRAG            HookPoint = "after_rag"
	HookBeforePipelineStep  HookPoint = "before_pipeline_step"
	HookAfterPipelineStep   HookPoint = "after_pipeline_step"
	HookBeforeHandoff       HookPoint = "before_handoff"
	HookAfterHandoff        HookPoint = "after_handoff"
	HookBeforeParallelAgent HookPoint = "before_parallel_agent"
	HookAfterParallelAgent  HookPoint = "after_parallel_agent"
	HookBeforeDAGNode       HookPoint = "before_dag_node"
	HookAfterDAGNode        HookPoint = "after_dag_node"

	// ===== 新增生命周期钩子 =====

	// 流式输出事件
	HookOnStream      HookPoint = "on_stream"       // 流式 chunk 输出时
	HookOnStreamStart HookPoint = "on_stream_start" // 流式开始
	HookOnStreamEnd   HookPoint = "on_stream_end"   // 流式结束

	// 记忆操作
	HookBeforeMemoryRead  HookPoint = "before_memory_read"  // 记忆读取前
	HookAfterMemoryRead   HookPoint = "after_memory_read"   // 记忆读取后
	HookBeforeMemoryWrite HookPoint = "before_memory_write" // 记忆写入前
	HookAfterMemoryWrite  HookPoint = "after_memory_write"  // 记忆写入后

	// 上下文窗口管理
	HookContextWindowUpdate HookPoint = "context_window_update" // 上下文窗口更新时
	HookContextWindowFull   HookPoint = "context_window_full"   // 上下文窗口已满

	// 工具解析
	HookBeforeToolParse HookPoint = "before_tool_parse" // LLM 返回的工具调用解析前
	HookAfterToolParse  HookPoint = "after_tool_parse"  // 工具调用解析完成后

	// 指标收集
	HookOnMetricsCollect HookPoint = "on_metrics_collect" // 指标收集点

	// 关闭生命周期
	HookBeforeShutdown HookPoint = "before_shutdown" // 关闭请求发出前
	HookAfterShutdown  HookPoint = "after_shutdown"  // 关闭完成后

	// 状态变更（Lifecycle）
	HookOnStateChange HookPoint = "on_state_change" // Agent 状态转换时
)

type HookContext struct {
	AgentID    string
	RequestID  string
	SessionID  string
	Point      HookPoint
	Turn       int
	Message    *Message
	Response   *Response
	ToolCall   *ToolCall
	ToolResult *ToolResult
	Error      error
	Metadata   map[string]any

	// 新增字段：流式数据
	StreamChunk *StreamEvent
	Duration    time.Duration

	// 新增字段：状态变更信息
	OldState string
	NewState string
	Reason   string

	// 新增字段：记忆操作
	MemoryQuery  string
	MemoryResult any

	// 新增字段：上下文窗口
	ContextWindowUsage float64
	ContextWindowLimit int
}

type HookFunc func(ctx context.Context, hctx *HookContext) error

type Hooks = *HookManager

// HookPhase 钩子执行阶段，决定执行顺序
type HookPhase int

const (
	PhaseValidation     HookPhase = iota // 护栏阶段：Guardrails 固定在此
	PhasePreProcessing                   // 预处理：日志、指标
	PhaseExecution                       // 执行：业务逻辑
	PhasePostProcessing                  // 后处理：通知、缓存
)

type Hook struct {
	Point     HookPoint
	Func      HookFunc
	Priority  int
	Condition HookCondition
	ID        string
	Phase     HookPhase
}

// HookCondition 钩子条件，满足条件才执行
type HookCondition func(ctx context.Context, hctx *HookContext) bool

// Always 总是返回 true 的条件
func Always(_ context.Context, _ *HookContext) bool { return true }

// OnTurn 在指定 turn 号时触发
func OnTurn(turnNum int) HookCondition {
	return func(_ context.Context, hctx *HookContext) bool {
		return hctx.Turn == turnNum
	}
}

// OnTurnsGreater 在 turn 大于指定值时触发
func OnTurnsGreater(threshold int) HookCondition {
	return func(_ context.Context, hctx *HookContext) bool {
		return hctx.Turn > threshold
	}
}

// OnError 当有错误时触发
func OnError() HookCondition {
	return func(_ context.Context, hctx *HookContext) bool {
		return hctx.Error != nil
	}
}

// OnMetadataKey 当 Metadata 包含指定 key 时触发
func OnMetadataKey(key string) HookCondition {
	return func(_ context.Context, hctx *HookContext) bool {
		if hctx.Metadata == nil {
			return false
		}
		_, ok := hctx.Metadata[key]
		return ok
	}
}

// OnStateTransition 指定状态转换时触发
func OnStateTransition(from, to string) HookCondition {
	return func(_ context.Context, hctx *HookContext) bool {
		return hctx.OldState == from && hctx.NewState == to
	}
}

// hookPointCount HookPoint enum 的数量（perf-v5 Task 11）
// 通过 reflect 或者手工计算得到，用于分配 atomic 数组
const hookPointCount = 64 // 大于实际 HookPoint 数量（35+），预留空间

// HookStats 钩子执行统计（perf-v5 Task 11：原子化，避免锁内 map 写）
type HookStats struct {
	TotalFired   int64
	TotalErrors  int64
	ByPoint      []atomic.Int64 // 下标对应 HookPoint 的 enum 序号
	ByPointError []atomic.Int64
	// nameIndex 记录 HookPoint string → 下标的映射（构造时一次性建好）
	nameIndex map[HookPoint]int
}

func newHookStats() *HookStats {
	idx := make(map[HookPoint]int, hookPointCount)
	// 枚举所有 HookPoint（手动列举避免引入 reflect）
	all := []HookPoint{
		HookBeforeRun, HookAfterRun, HookBeforeTurn, HookAfterTurn,
		HookBeforeLLM, HookAfterLLM, HookBeforeTool, HookAfterTool,
		HookOnError, HookOnComplete, HookBeforeRAG, HookAfterRAG,
		HookBeforePipelineStep, HookAfterPipelineStep,
		HookBeforeHandoff, HookAfterHandoff,
		HookBeforeParallelAgent, HookAfterParallelAgent,
		HookBeforeDAGNode, HookAfterDAGNode,
		HookOnStream, HookOnStreamStart, HookOnStreamEnd,
		HookBeforeMemoryRead, HookAfterMemoryRead, HookBeforeMemoryWrite, HookAfterMemoryWrite,
		HookContextWindowUpdate, HookContextWindowFull,
		HookBeforeToolParse, HookAfterToolParse,
		HookOnMetricsCollect, HookBeforeShutdown, HookAfterShutdown, HookOnStateChange,
	}
	for i, p := range all {
		idx[p] = i
	}
	return &HookStats{
		ByPoint:      make([]atomic.Int64, hookPointCount),
		ByPointError: make([]atomic.Int64, hookPointCount),
		nameIndex:    idx,
	}
}

func (s *HookStats) Record(point HookPoint, err error) {
	atomic.AddInt64(&s.TotalFired, 1)
	if idx, ok := s.nameIndex[point]; ok && idx < hookPointCount {
		s.ByPoint[idx].Add(1)
		if err != nil {
			atomic.AddInt64(&s.TotalErrors, 1)
			s.ByPointError[idx].Add(1)
		}
	}
}

func (s *HookStats) Snapshot() map[string]interface{} {
	result := map[string]interface{}{
		"total_fired":  atomic.LoadInt64(&s.TotalFired),
		"total_errors": atomic.LoadInt64(&s.TotalErrors),
		"by_point":     make(map[string]int64, len(s.nameIndex)),
		"by_errors":    make(map[string]int64, len(s.nameIndex)),
	}
	for p, idx := range s.nameIndex {
		if v := s.ByPoint[idx].Load(); v > 0 {
			result["by_point"].(map[string]int64)[string(p)] = v
		}
		if v := s.ByPointError[idx].Load(); v > 0 {
			result["by_errors"].(map[string]int64)[string(p)] = v
		}
	}
	return result
}

// HookManager 钩子管理器
// perf-v6 Task 5：用 atomic.Pointer 维护 hooks 快照，避免 Fire 路径 make+copy
type HookManager struct {
	mu         sync.RWMutex
	hooks      map[HookPoint][]Hook
	stats      *HookStats
	middleware []HookMiddleware
	// 快照：注册时同步更新，Fire 时一次 atomic load 无锁
	hooksSnap      atomic.Pointer[map[HookPoint][]Hook]
	middlewareSnap atomic.Pointer[[]HookMiddleware]
}

// HookMiddleware 中间件，可在 Fire 前后添加横切逻辑
type HookMiddleware interface {
	Before(ctx context.Context, hctx *HookContext) error
	After(ctx context.Context, hctx *HookContext, err error) error
}

// HookMiddlewareFunc 函数式中间件
type HookMiddlewareFunc struct {
	BeforeFn func(ctx context.Context, hctx *HookContext) error
	AfterFn  func(ctx context.Context, hctx *HookContext, err error) error
}

func (m *HookMiddlewareFunc) Before(ctx context.Context, hctx *HookContext) error {
	if m.BeforeFn != nil {
		return m.BeforeFn(ctx, hctx)
	}
	return nil
}

func (m *HookMiddlewareFunc) After(ctx context.Context, hctx *HookContext, err error) error {
	if m.AfterFn != nil {
		return m.AfterFn(ctx, hctx, err)
	}
	return err
}

func NewHookManager() *HookManager {
	return &HookManager{
		hooks:      make(map[HookPoint][]Hook),
		stats:      newHookStats(),
		middleware: make([]HookMiddleware, 0),
	}
}

func (m *HookManager) Register(point HookPoint, fn HookFunc) {
	m.RegisterWithPriority(point, fn, 0)
}

func (m *HookManager) RegisterWithPriority(point HookPoint, fn HookFunc, priority int) {
	m.RegisterConditional(point, fn, priority, Always, "")
}

func (m *HookManager) RegisterConditional(point HookPoint, fn HookFunc, priority int, condition HookCondition, id string) {
	m.RegisterConditionalInPhase(PhaseExecution, point, fn, priority, condition, id)
}

func (m *HookManager) RegisterInPhase(phase HookPhase, point HookPoint, fn HookFunc) {
	m.RegisterConditionalInPhase(phase, point, fn, 0, Always, "")
}

// refreshSnapshots 刷新 atomic.Pointer 快照（perf-v6 Task 5）
// 必须在持锁状态下调用
func (m *HookManager) refreshSnapshots() {
	// 拷贝 hooks map
	newHooks := make(map[HookPoint][]Hook, len(m.hooks))
	for p, hs := range m.hooks {
		newHooks[p] = append([]Hook(nil), hs...)
	}
	m.hooksSnap.Store(&newHooks)

	// 拷贝 middleware
	newMid := append([]HookMiddleware(nil), m.middleware...)
	m.middlewareSnap.Store(&newMid)
}

func (m *HookManager) RegisterConditionalInPhase(phase HookPhase, point HookPoint, fn HookFunc, priority int, condition HookCondition, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hook := Hook{
		Point:     point,
		Func:      fn,
		Priority:  priority,
		Condition: condition,
		ID:        id,
		Phase:     phase,
	}
	m.hooks[point] = append(m.hooks[point], hook)

	for i := len(m.hooks[point]) - 1; i > 0; i-- {
		if m.hooks[point][i].Priority < m.hooks[point][i-1].Priority {
			m.hooks[point][i], m.hooks[point][i-1] = m.hooks[point][i-1], m.hooks[point][i]
		} else {
			break
		}
	}
	m.refreshSnapshots() // perf-v6 Task 5
}

func (m *HookManager) Use(middleware HookMiddleware) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.middleware = append(m.middleware, middleware)
	m.refreshSnapshots() // perf-v6 Task 5
}

func (m *HookManager) Fire(ctx context.Context, hctx *HookContext) error {
	// perf-v6 Task 5：atomic.Pointer 快照无锁读取，避免 make+copy
	var hooks []Hook
	if snap := m.hooksSnap.Load(); snap != nil {
		hooks = (*snap)[hctx.Point]
	} else {
		// fallback：第一次 Fire 之前没有快照
		m.mu.RLock()
		hooks = append([]Hook(nil), m.hooks[hctx.Point]...)
		m.mu.RUnlock()
	}

	var mids []HookMiddleware
	if snap := m.middlewareSnap.Load(); snap != nil {
		mids = *snap
	} else {
		m.mu.RLock()
		mids = append([]HookMiddleware(nil), m.middleware...)
		m.mu.RUnlock()
	}

	// perf-v5 Task 12：phaseOrder 提升为包级 var，避免每次 Fire 都分配 slice
	var execErr error

	for _, mid := range mids {
		if err := mid.Before(ctx, hctx); err != nil {
			m.stats.Record(hctx.Point, err)
			return err
		}
	}

	for _, phase := range phaseOrder {
		for _, hook := range hooks {
			if hook.Phase != phase {
				continue
			}
			if hook.Condition == nil || hook.Condition(ctx, hctx) {
				if err := hook.Func(ctx, hctx); err != nil {
					execErr = err
					m.stats.Record(hctx.Point, err)
					break
				}
			}
		}
		if execErr != nil {
			break
		}
	}

	if execErr == nil {
		m.stats.Record(hctx.Point, nil)
	}

	for i := len(mids) - 1; i >= 0; i-- {
		afterErr := mids[i].After(ctx, hctx, execErr)
		if afterErr != nil {
			if afterErr == errRecovered {
				// ErrorRecoveryMiddleware 显式恢复错误
				execErr = nil
			} else {
				execErr = afterErr
			}
		}
	}

	return execErr
}

func (m *HookManager) Remove(point HookPoint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.hooks, point)
	m.refreshSnapshots() // perf-v6 Task 5
}

func (m *HookManager) RemoveByID(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for point, hooks := range m.hooks {
		filtered := hooks[:0]
		for _, h := range hooks {
			if h.ID != id {
				filtered = append(filtered, h)
			}
		}
		m.hooks[point] = filtered
	}
	m.refreshSnapshots() // perf-v6 Task 5
}

func (m *HookManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = make(map[HookPoint][]Hook)
	m.middleware = nil
	m.refreshSnapshots() // perf-v6 Task 5
}

func (m *HookManager) Count(point HookPoint) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.hooks[point])
}

func (m *HookManager) TotalCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	total := 0
	for _, hooks := range m.hooks {
		total += len(hooks)
	}
	return total
}

func (m *HookManager) Stats() *HookStats {
	return m.stats
}

func (m *HookManager) RegisteredPoints() []HookPoint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	points := make([]HookPoint, 0, len(m.hooks))
	for p := range m.hooks {
		points = append(points, p)
	}
	return points
}

// ===== 便捷方法 =====

func (m *HookManager) OnReasoning(ctx context.Context, thought *Thought) {
	_ = m.Fire(ctx, &HookContext{Point: HookAfterLLM})
}

func (m *HookManager) OnComplete(ctx context.Context, resp *Response) {
	_ = m.Fire(ctx, &HookContext{Point: HookOnComplete, Response: resp})
}

func (m *HookManager) OnTurnComplete(ctx context.Context, turn int, thought *Thought) {
	_ = m.Fire(ctx, &HookContext{Point: HookAfterTurn, Turn: turn})
}

func (m *HookManager) OnToolUse(ctx context.Context, tc *ToolCall) {
	_ = m.Fire(ctx, &HookContext{Point: HookBeforeTool, ToolCall: tc})
}

func (m *HookManager) OnToolResult(ctx context.Context, result *ToolResult) {
	_ = m.Fire(ctx, &HookContext{Point: HookAfterTool, ToolResult: result})
}

func (m *HookManager) OnError(ctx context.Context, err error) {
	_ = m.Fire(ctx, &HookContext{Point: HookOnError, Error: err})
}

// ===== 新增便捷方法 =====

func (m *HookManager) OnStream(ctx context.Context, event *StreamEvent) {
	_ = m.Fire(ctx, &HookContext{Point: HookOnStream, StreamChunk: event})
}

func (m *HookManager) OnStreamStart(ctx context.Context) {
	_ = m.Fire(ctx, &HookContext{Point: HookOnStreamStart})
}

func (m *HookManager) OnStreamEnd(ctx context.Context, duration time.Duration) {
	_ = m.Fire(ctx, &HookContext{Point: HookOnStreamEnd, Duration: duration})
}

func (m *HookManager) OnStateChange(ctx context.Context, agentID, oldState, newState, reason string) {
	_ = m.Fire(ctx, &HookContext{
		Point:    HookOnStateChange,
		AgentID:  agentID,
		OldState: oldState,
		NewState: newState,
		Reason:   reason,
	})
}

func (m *HookManager) OnMemoryRead(ctx context.Context, query string, result any) {
	_ = m.Fire(ctx, &HookContext{
		Point:        HookAfterMemoryRead,
		MemoryQuery:  query,
		MemoryResult: result,
	})
}

func (m *HookManager) OnMemoryWrite(ctx context.Context, sessionID string) {
	_ = m.Fire(ctx, &HookContext{
		Point:     HookAfterMemoryWrite,
		SessionID: sessionID,
	})
}

func (m *HookManager) OnContextWindowUpdate(ctx context.Context, usage float64, limit int) {
	_ = m.Fire(ctx, &HookContext{
		Point:              HookContextWindowUpdate,
		ContextWindowUsage: usage,
		ContextWindowLimit: limit,
	})
}

func (m *HookManager) OnShutdown(ctx context.Context) {
	_ = m.Fire(ctx, &HookContext{Point: HookBeforeShutdown})
}

func (m *HookManager) OnShutdownComplete(ctx context.Context) {
	_ = m.Fire(ctx, &HookContext{Point: HookAfterShutdown})
}

// AllHookPoints 返回所有定义的钩子点常量
func AllHookPoints() []HookPoint {
	return []HookPoint{
		HookBeforeRun, HookAfterRun,
		HookBeforeTurn, HookAfterTurn,
		HookBeforeLLM, HookAfterLLM,
		HookBeforeTool, HookAfterTool,
		HookOnError, HookOnComplete,
		HookBeforeRAG, HookAfterRAG,
		HookBeforePipelineStep, HookAfterPipelineStep,
		HookBeforeHandoff, HookAfterHandoff,
		HookBeforeParallelAgent, HookAfterParallelAgent,
		HookBeforeDAGNode, HookAfterDAGNode,
		HookOnStream, HookOnStreamStart, HookOnStreamEnd,
		HookBeforeMemoryRead, HookAfterMemoryRead,
		HookBeforeMemoryWrite, HookAfterMemoryWrite,
		HookContextWindowUpdate, HookContextWindowFull,
		HookBeforeToolParse, HookAfterToolParse,
		HookOnMetricsCollect,
		HookBeforeShutdown, HookAfterShutdown,
		HookOnStateChange,
	}
}

// HookPointCategory 钩子点分类
func HookPointCategory(p HookPoint) string {
	switch p {
	case HookBeforeRun, HookAfterRun, HookBeforeShutdown, HookAfterShutdown:
		return "lifecycle"
	case HookBeforeTurn, HookAfterTurn, HookOnComplete, HookOnError, HookOnStateChange:
		return "execution"
	case HookBeforeLLM, HookAfterLLM:
		return "llm"
	case HookBeforeTool, HookAfterTool, HookBeforeToolParse, HookAfterToolParse:
		return "tool"
	case HookBeforeRAG, HookAfterRAG, HookBeforeMemoryRead, HookAfterMemoryRead,
		HookBeforeMemoryWrite, HookAfterMemoryWrite:
		return "memory"
	case HookOnStream, HookOnStreamStart, HookOnStreamEnd:
		return "stream"
	case HookContextWindowUpdate, HookContextWindowFull:
		return "context"
	case HookBeforePipelineStep, HookAfterPipelineStep, HookBeforeHandoff, HookAfterHandoff,
		HookBeforeParallelAgent, HookAfterParallelAgent, HookBeforeDAGNode, HookAfterDAGNode:
		return "orchestration"
	case HookOnMetricsCollect:
		return "observability"
	default:
		return "unknown"
	}
}

// LoggingMiddleware 日志中间件：记录每个钩子执行的耗时和结果
func LoggingMiddleware() *HookMiddlewareFunc {
	return &HookMiddlewareFunc{
		BeforeFn: func(_ context.Context, hctx *HookContext) error { return nil },
		AfterFn:  func(_ context.Context, hctx *HookContext, err error) error { return err },
	}
}

// MetricsCollectionMiddleware 指标收集中间件
func MetricsCollectionMiddleware(stats *HookStats) *HookMiddlewareFunc {
	return &HookMiddlewareFunc{
		BeforeFn: func(_ context.Context, _ *HookContext) error { return nil },
		AfterFn: func(_ context.Context, hctx *HookContext, err error) error {
			if stats != nil {
				stats.Record(hctx.Point, err)
			}
			return err
		},
	}
}

// TimeoutMiddleware 超时中间件：单个钩子执行超限时跳过
func TimeoutMiddleware(timeout time.Duration) *HookMiddlewareFunc {
	return &HookMiddlewareFunc{
		BeforeFn: func(ctx context.Context, hctx *HookContext) error { return nil },
		AfterFn: func(ctx context.Context, _ *HookContext, err error) error {
			select {
			case <-ctx.Done():
				return fmt.Errorf("hook execution cancelled")
			default:
				return err
			}
		},
	}
}

// errRecovered 是 ErrorRecoveryMiddleware 使用的 sentinel error，
// 表示错误已被恢复处理，不应继续传播
var errRecovered = errors.New("hook error recovered")

// phaseOrder Hook 执行阶段顺序（perf-v5 Task 12：包级 var 避免每次 Fire 分配）
var phaseOrder = []HookPhase{PhaseValidation, PhasePreProcessing, PhaseExecution, PhasePostProcessing}

// ErrorRecoveryMiddleware 错误恢复中间件：记录错误但不阻断后续钩子
func ErrorRecoveryMiddleware(onError func(HookPoint, error)) *HookMiddlewareFunc {
	return &HookMiddlewareFunc{
		BeforeFn: func(_ context.Context, _ *HookContext) error { return nil },
		AfterFn: func(ctx context.Context, hctx *HookContext, err error) error {
			if err != nil && onError != nil {
				onError(hctx.Point, err)
			}
			if err != nil {
				return errRecovered // 不传播原始错误
			}
			return nil
		},
	}
}
