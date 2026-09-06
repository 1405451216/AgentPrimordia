// hooks.go — 工具智能 ReAct Hook：使用后画像 + 失败后缺口检测
package intelligence

import (
	"context"
	"sync"
	"time"
)

// IntelligenceHook ReAct 循环的工具智能 Hook
// 在工具调用后记录画像数据，在轮次结束时检测缺口并自动创建工具
type IntelligenceHook struct {
	mu       sync.Mutex
	profiler ToolProfiler
	detector GapDetector
	creator  ToolCreator
	trace    []ToolCallRecord
}

// NewIntelligenceHook 创建工具智能 Hook
func NewIntelligenceHook(profiler ToolProfiler, detector GapDetector, creator ToolCreator) *IntelligenceHook {
	return &IntelligenceHook{
		profiler: profiler,
		detector: detector,
		creator:  creator,
	}
}

// AfterToolCall 工具调用后的 Hook——记录使用画像 + 追加到调用轨迹
func (h *IntelligenceHook) AfterToolCall(ctx context.Context, toolName, args, result string, err error, duration time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	success := err == nil

	// 记录到 profiler（忽略错误，画像记录失败不影响主流程）
	_ = h.profiler.Record(ctx, ToolUsageRecord{
		ToolName: toolName,
		Success:  success,
		Duration: duration,
	})

	// 构造错误信息
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	// 记录到调用轨迹
	h.trace = append(h.trace, ToolCallRecord{
		ToolName:  toolName,
		Args:      args,
		Result:    result,
		Error:     errMsg,
		Duration:  duration,
		Success:   success,
		Timestamp: time.Now(),
	})
}

// OnTurnEnd 轮次结束时的 Hook——检测缺口并自动创建工具
// 将当前轨迹快照后交给缺口检测器分析，对每个检测到的缺口调用工具生成器
func (h *IntelligenceHook) OnTurnEnd(ctx context.Context) {
	h.mu.Lock()
	trace := make([]ToolCallRecord, len(h.trace))
	copy(trace, h.trace)
	// 清空轨迹，为下一轮做准备
	h.trace = h.trace[:0]
	h.mu.Unlock()

	if len(trace) == 0 {
		return
	}

	gaps, err := h.detector.Detect(ctx, trace)
	if err != nil || len(gaps) == 0 {
		return
	}

	for _, gap := range gaps {
		// 工具生成失败不阻塞后续缺口的处理
		_, _ = h.creator.Create(ctx, gap)
	}
}

// TraceLength 返回当前轨迹长度（用于测试）
func (h *IntelligenceHook) TraceLength() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.trace)
}
