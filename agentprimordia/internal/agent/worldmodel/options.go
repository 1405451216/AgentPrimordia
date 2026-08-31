// options.go — WithWorldModel agent 选项骨架（v6.1 第一切片，提案 §2.1）
//
// 本切片刻意「只定义、不接线」：
//  1. worldmodel 位于 internal/agent 之下，按分层约束不得 import 上层包
//     （含 agent 本身）——故此处定义结构等价的局部 Option，待接线切片由
//     agent 层适配器转换（若 worldmodel import agent，未来 agent 接入
//     worldmodel 时立即构成循环依赖，故必须反向）；
//  2. 不调用本选项时 agent 默认构造零变化（铁律 7 / 提案 §2.1 opt-in）；
//  3. 全部接线点以注释形式固定在文末「接线点（下一切片）」，留下一切片完成。
package worldmodel

// WorldModelOptions 世界模型相关 agent 选项的聚合载体（骨架）。
type WorldModelOptions struct {
	// Tracker 世界模型跟踪器；nil = 不启用世界模型（默认，行为与 v6.0 一致）。
	Tracker *WorldModelTracker
}

// Option 链式选项函数——与 internal/agent / pkg/agent.go 的 Option 同风格；
// 本切片不接线，由未来 agent 层适配器转换/应用到 ReActAgent。
type Option func(*WorldModelOptions)

// WithWorldModel 启用世界模型：把 tracker 绑定进选项。
// v6.1 语义：显式 opt-in、默认关闭——不调用时不得改变任何默认 ReAct 行为
// （docs/提案-世界模型默认策略切换.md §2.1，铁律 7）。
// tracker 允许 nil（等价不启用，便于调用方条件装配）。
func WithWorldModel(tracker *WorldModelTracker) Option {
	return func(o *WorldModelOptions) {
		if o == nil {
			return
		}
		o.Tracker = tracker
	}
}

// NewWorldModelOptions 依次应用选项并返回聚合结果（nil 选项跳过，
// 多个 WithWorldModel 后者覆盖前者）。
func NewWorldModelOptions(opts ...Option) WorldModelOptions {
	var o WorldModelOptions
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&o)
	}
	return o
}

// —— 接线点（下一切片实现；提案 §2.1：runLoop 接入与 E1–E3 逐点对应）——
//
// 前置：agent 层新增适配器（如 internal/agent/worldmodel_hook.go），把
// WorldModelOptions.Tracker 挂进 loopConfig；默认 nil 时全部挂钩短路，
// 保证默认路径一个字节都不经过。
//
//  1. reactLoopEngine 构造 loopConfig 时应用适配后的 Option，取 Tracker；
//  2. assistantMsg 追加后（提案 E3：internal/agent/react_loop_core.go:275）：
//     tracker.Apply(PlanRevised{...}) / Apply(HypothesisFormed{...})；
//  3. executeToolCalls 工具结果回写后（提案 E3：react_loop_core.go:278）：
//     tracker.Apply(ToolObserved{Turn: turn, ToolName: ..., ToolInput: ..., Observation: ...})；
//  4. trimContext 裁剪发生时（提案 E6：internal/agent/react_persist.go trimContext，
//     defaultMaxHistoryMessages=100 滑动窗口）：tracker.TrimNotification(被裁消息, turn)；
//  5. 工具执行前（预演 gate）：Rehearse(当前计划, tracker.Graph()) 不过 →
//     写失败库（internal/persist FailureStore 通道，参照 learning/feedback.go 失败入库路径）；
//  6. 行动后（回溯校验）：ComparePaths(当前计划 Path(), tracker.Graph().PathTo(最新节点))
//     结果写失败库并触发审计（审计口径沿用 AuditEvent）。
//
// 以上全部位于 opt-in 分支内；「评价线默认开」「v7.0 翻默认」分属提案
// §2.2/§2.3，与本切片无关。
