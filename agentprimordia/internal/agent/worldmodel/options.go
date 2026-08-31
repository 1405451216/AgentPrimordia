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

// —— 接线点（第二切片已全部接线，见 internal/agent/worldmodel_hook.go）——
//
// 接线层实现：internal/agent/worldmodel_hook.go（WorldModelCapable 接口发现 +
// 事件转换挂钩，全部 nil-safe：tracker 未注入时默认路径一个字节都不经过）。
//
//  1. ✅ 注入链：agent.WithWorldModel(tracker)（config.Cognition.WorldModel /
//     NewAgent 选项 / CapabilityAgent 链式）→ capabilityCache.worldTracker；
//  2. ✅ assistantMsg 追加后（提案 E3：react_loop_core.go）：本轮 ToolCalls →
//     PlanRevised（步骤 ID 用 NodeID(KindToolCall, 摘要) 派生，与执行后调用
//     节点收敛）；思考文本 → HypothesisFormed；planner 粗粒度计划 →
//     PlanRevised + 组建期预演门；
//  3. ✅ processToolResult 工具结果回写后（提案 E3）：ToolObserved
//     （tool_call → observation 因果链，失败观察同样落图）；
//  4. ✅ trimContext 裁剪发生后（提案 E6）：wmNotifyTrimmed 以公共前缀定位
//     被裁消息 → tracker.TrimNotification 落为 observation 事实节点；
//  5. ✅ 工具执行前预演门（观察模式）：Rehearse(当前计划, tracker.Graph())
//     不过 → persist.FailureRecord + 审计 worldmodel.rehearsal_failed
//     （不拦截执行——阻断语义待治理策略）；
//  6. ✅ 行动后回溯校验（观察模式）：ComparePaths(计划路径, tracker
//     .PlanTrajectory()) 偏离 → 失败库 + 审计 worldmodel.backdiff_diverged。
//
// 第三切片（已完成）：state-checkpoint 协议——worldmodel.Snapshot/Restore
// 快照层 + persist.AgentState.WorldState（json.RawMessage 透传）+
// saveCheckpoint 嵌入 / resumeFromState「续知而非重放」载入（提案 E7–E10）。
// 剩余随行项：CI 状态断言一致性门（提案 §三.2）。
// 「评价线默认开」「v7.0 翻默认」分属提案 §2.2/§2.3，与本切片无关。
