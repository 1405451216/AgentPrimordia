// tracker.go — WorldModelTracker：从 agent 事件流增量维护状态图
//
// 职责边界（v6.1 第一切片）：
//   - 只消费最小事件集（ToolObserved / PlanRevised / HypothesisFormed），
//     不 import agent 包、不感知 Message 结构——消息→事件的转换由接入层完成；
//   - TrimNotification 承担「世界从上下文搬进图」：上下文裁剪丢弃的消息
//     在此转为 observation 事实节点，被裁事实不因滑动窗口而丢失（提案 E6 债务）；
//   - 事件自带 Turn，tracker 按提交序增量应用、不假设时序单调——乱序安全。
package worldmodel

import (
	"fmt"
	"sync"
)

// EventType 事件种类标识。
type EventType string

const (
	// EventToolObserved 一次工具调用及其观察结果。
	EventToolObserved EventType = "tool_observed"
	// EventPlanRevised 任务计划（重新）形成。
	EventPlanRevised EventType = "plan_revised"
	// EventHypothesisFormed 形成一条假设。
	EventHypothesisFormed EventType = "hypothesis_formed"
)

// AgentEvent agent 事件流的最小事件接口。
//
// 形状决策（v6.1 切片刻意最小化）：
//   - 封闭集合：eventType 为非导出标记方法，仅本包三类事件可实现——
//     接入层只构造事件、不能自定义事件种类，防止内核面失控；
//   - 事件自带 Turn，tracker 不假设时序单调，乱序事件流安全；
//   - 上下文裁剪不是 agent 行为事件，走独立的 TrimNotification 方法（见下）。
type AgentEvent interface {
	eventType() EventType
}

// ToolObserved 一次工具调用及其观察结果。
// ToolName+ToolInput 合成调用摘要（"工具名 输入"）；与计划步骤摘要对齐时，
// 执行后与计划步骤节点去重收敛（预演态→观测态分型，见 onToolObserved）。
type ToolObserved struct {
	Turn        int    // 事件发生轮次
	ToolName    string // 工具名
	ToolInput   string // 工具输入摘要
	Observation string // 观察结果摘要
}

func (ToolObserved) eventType() EventType { return EventToolObserved }

// PlanRevised 任务计划（重新）形成。Task 为空时不建任务节点；
// Goal 与 Steps 进入当前计划（覆盖式修订），并同步写入状态图。
type PlanRevised struct {
	Turn  int        // 事件发生轮次
	Task  string     // 任务描述（空则跳过任务节点）
	Goal  string     // 计划目标
	Steps []PlanStep // 有序计划步骤（ID 缺省时按序派生 step-N）
}

func (PlanRevised) eventType() EventType { return EventPlanRevised }

// HypothesisFormed 形成一条假设（推理产物，非观测事实）。
type HypothesisFormed struct {
	Turn int    // 事件发生轮次
	Text string // 假设内容摘要
}

func (HypothesisFormed) eventType() EventType { return EventHypothesisFormed }

// TrimmedMessage 一条被上下文裁剪丢弃的消息（提案 E6：滑动窗口无差别丢史）。
type TrimmedMessage struct {
	Role    string // 消息角色（user/assistant/tool/…）
	Content string // 消息内容（规范化后作 observation 事实节点摘要）
	Turn    int    // 产生该消息的轮次；未知传 -1（回退为裁剪发生轮次）
}

// WorldModelTracker 世界模型跟踪器：增量维护状态图 + 当前计划。
// 并发安全：内部互斥锁覆盖事件应用与计划/最新任务等派生状态；
// 状态图自身另有读写锁（两层锁获取顺序恒为 tracker.mu → graph.mu，无死锁环）。
type WorldModelTracker struct {
	mu         sync.Mutex
	graph      *StateGraph
	plan       Plan   // 当前计划（PlanRevised 覆盖式修订）
	hasPlan    bool   // 区分「空计划」与「尚未计划」
	lastTaskID string // 最近任务节点 ID（"" 表示尚无）
}

// NewWorldModelTracker 构造跟踪器（内部自建状态图）。
func NewWorldModelTracker() *WorldModelTracker {
	return &WorldModelTracker{graph: NewStateGraph()}
}

// Graph 返回内部状态图（懒初始化，零值 tracker 亦安全）。
// 图方法自身并发安全，调用方可长期持有引用。
func (t *WorldModelTracker) Graph() *StateGraph {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.graphLocked()
}

// CurrentPlan 返回当前计划拷贝（Steps 与 DependsOn 深拷贝）。
// 尚未有计划时 ok=false。
func (t *WorldModelTracker) CurrentPlan() (Plan, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.hasPlan {
		return Plan{}, false
	}
	return clonePlan(t.plan), true
}

// Apply 应用单个事件到状态图（增量、幂等：重复事件经节点/边去重无副作用）。
// 并发语义：多 goroutine 同时 Apply 时按获得内部锁的顺序串行应用；
// 未知/nil 事件静默忽略（封闭接口下不可达，向前兼容）。
func (t *WorldModelTracker) Apply(ev AgentEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch e := ev.(type) {
	case ToolObserved:
		t.onToolObserved(e)
	case *ToolObserved:
		if e != nil {
			t.onToolObserved(*e)
		}
	case PlanRevised:
		t.onPlanRevised(e)
	case *PlanRevised:
		if e != nil {
			t.onPlanRevised(*e)
		}
	case HypothesisFormed:
		t.onHypothesisFormed(e)
	case *HypothesisFormed:
		if e != nil {
			t.onHypothesisFormed(*e)
		}
	default:
		// nil 接口值或未知事件：忽略（封闭接口下不可达分支）
	}
}

// onToolObserved 工具调用 → 观察的因果链（须持 t.mu）：
//
//	tool_call 节点（摘要 = "工具名 输入"）--cause--> observation 节点（摘要 = 结果）
//
// 计划步骤摘要与「工具名 输入」对齐时，步骤节点与调用节点去重收敛为同一节点：
// 执行前仅入向 plan 边（预演态），执行后出现 cause 出边（观测态）——
// 预演态/观测态在图上分型（提案 §四 风险缓解③）。
func (t *WorldModelTracker) onToolObserved(e ToolObserved) {
	g := t.graphLocked()
	callID, _ := g.AddNode(KindToolCall, e.ToolName+" "+e.ToolInput, e.Turn)
	obsID, _ := g.AddNode(KindObservation, e.Observation, e.Turn)
	g.AddEdge(callID, obsID, EdgeCause)
}

// onPlanRevised 计划（重新）形成（须持 t.mu）：
//
//	任务节点 --plan--> 计划节点 --plan--> 各步骤节点（预演态）
//
// 覆盖式修订：同 Goal 计划节点去重复用，步骤链接增量追加；被撤销的
// 步骤仅从当前计划消失（图中保留历史事实，回溯差异按当前计划对账）。
func (t *WorldModelTracker) onPlanRevised(e PlanRevised) {
	g := t.graphLocked()
	steps := make([]PlanStep, len(e.Steps))
	copy(steps, e.Steps)
	for i := range steps {
		if steps[i].ID == "" {
			steps[i].ID = fmt.Sprintf("step-%d", i+1) // 缺省步骤 ID：确定性派生
		}
	}
	if e.Task != "" {
		t.lastTaskID, _ = g.AddNode(KindTask, e.Task, e.Turn)
	}
	planID, _ := g.AddNode(KindPlan, e.Goal, e.Turn)
	if t.lastTaskID != "" {
		g.AddEdge(t.lastTaskID, planID, EdgePlan)
	}
	for _, s := range steps {
		stepID, _ := g.AddNode(KindToolCall, s.Summary, e.Turn)
		g.AddEdge(planID, stepID, EdgePlan)
	}
	t.plan = Plan{Goal: e.Goal, Steps: steps}
	t.hasPlan = true
}

// onHypothesisFormed 假设节点（须持 t.mu）：有当前任务时以 hypothesis 边
// 挂到任务（假设是推理产物、与观测分型）；无任务时先孤立成节点。
// 增量语义：后续事件不回补历史边。
func (t *WorldModelTracker) onHypothesisFormed(e HypothesisFormed) {
	g := t.graphLocked()
	hypID, _ := g.AddNode(KindHypothesis, e.Text, e.Turn)
	if t.lastTaskID != "" {
		g.AddEdge(t.lastTaskID, hypID, EdgeHypothesis)
	}
}

// TrimNotification 上下文裁剪通知：把被裁消息转为 observation 事实节点，
// 完成「世界从上下文搬进图」（提案 E6：滑动窗口丢史债务的结构化偿还）。
//   - 每条消息 → observation 节点（摘要 = 规范化内容），重复裁剪/回放幂等去重；
//   - 消息自带 Turn 未知（<0）时回退使用裁剪发生轮次 turn；
//   - 有当前任务时，事实节点以 context 边挂到任务节点。
//
// 返回本次新建节点 ID（保持消息序）；无新建时为 nil。
func (t *WorldModelTracker) TrimNotification(msgs []TrimmedMessage, turn int) []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	g := t.graphLocked()
	var created []string
	for _, m := range msgs {
		at := m.Turn
		if at < 0 {
			at = turn
		}
		id, isNew := g.AddNode(KindObservation, m.Content, at)
		if isNew {
			created = append(created, id)
		}
		if t.lastTaskID != "" {
			g.AddEdge(t.lastTaskID, id, EdgeContext)
		}
	}
	return created
}

// graphLocked 返回内部图（须持 t.mu；懒初始化保证零值 tracker 可用）。
func (t *WorldModelTracker) graphLocked() *StateGraph {
	if t.graph == nil {
		t.graph = NewStateGraph()
	}
	return t.graph
}

// clonePlan 计划防御性拷贝（Steps 与每个 DependsOn 均深拷贝）。
func clonePlan(p Plan) Plan {
	steps := make([]PlanStep, len(p.Steps))
	for i, s := range p.Steps {
		steps[i] = s
		if len(s.DependsOn) > 0 {
			deps := make([]string, len(s.DependsOn))
			copy(deps, s.DependsOn)
			steps[i].DependsOn = deps
		} else {
			steps[i].DependsOn = nil
		}
	}
	return Plan{Goal: p.Goal, Steps: steps}
}
