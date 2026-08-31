// snapshot.go — 世界模型快照（v6.1 第三切片：state-checkpoint 协议，提案 E7–E10）
//
// 目标：kill -9 后恢复语义从「重放」（用文本消息重建世界）升格为「续知」
// （状态图随检查点落盘，恢复时直接载入结构化世界状态）。
//
// 载体设计：
//   - Snapshot 为可 JSON 序列化的纯数据结构（StateNode/Plan 均为导出字段），
//     由 persist.AgentState.WorldState 以 json.RawMessage 透传——persist 层
//     不感知世界模型内部结构（依赖方向：persist 不得 import agent/*）；
//   - Snapshot/Restore 均确定性：节点按 ID 升序导出，Restore 重建 rev 索引；
//   - Restore 校验结构完整性（重复节点 ID / 悬挂边 → error），不静默丢弃——
//     世界状态损坏应显式失败而非带着残缺图继续推理；
//   - Restore 为覆盖式：以快照整体替换当前图与计划（检查点是权威状态）。
package worldmodel

import (
	"fmt"
	"sort"
)

// Snapshot 世界模型可序列化快照（state-checkpoint 协议载体）。
type Snapshot struct {
	// Nodes 全部状态节点（按 ID 升序；Edges 携带出边，入边索引恢复时重建）。
	Nodes []StateNode `json:"nodes"`
	// Plan 当前计划；HasPlan 区分「空计划」与「尚未计划」。
	Plan    Plan `json:"plan,omitempty"`
	HasPlan bool `json:"has_plan"`
	// LastTaskID 最近任务节点 ID（"" 表示尚无；假设/上下文边的挂接锚点）。
	LastTaskID string `json:"last_task_id,omitempty"`
	// PlanTraj 自当前计划形成以来的实际调用轨迹（接线点⑥轨迹端）。
	PlanTraj []string `json:"plan_traj,omitempty"`
}

// Snapshot 导出世界模型全量快照（深拷贝，节点按 ID 升序——确定性序列化）。
func (t *WorldModelTracker) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	g := t.graphLocked()
	nodes := make([]StateNode, 0, len(g.nodes))
	for _, n := range g.nodes {
		nodes = append(nodes, cloneNode(*n))
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	snap := Snapshot{
		Nodes:      nodes,
		HasPlan:    t.hasPlan,
		LastTaskID: t.lastTaskID,
	}
	if t.hasPlan {
		snap.Plan = clonePlan(t.plan)
	}
	if len(t.planTraj) > 0 {
		traj := make([]string, len(t.planTraj))
		copy(traj, t.planTraj)
		snap.PlanTraj = traj
	}
	return snap
}

// Restore 以快照覆盖式替换当前世界状态（图 + 计划 + 轨迹）。
// 校验失败返回 error 且不改动现有状态；成功后快照与 tracker 解耦
// （深拷贝入图，外部改动不回流）。
func (t *WorldModelTracker) Restore(snap Snapshot) error {
	g, err := graphFromNodes(snap.Nodes)
	if err != nil {
		return err
	}
	// 计划步骤引用校验：计划内步骤 ID 必须可解析（步骤 ID 为派生 ID 时
	// 应能在图中找到对应节点；纯计划 ID 也允许——计划可先于观测存在）
	if snap.HasPlan {
		inPlan := make(map[string]bool, len(snap.Plan.Steps))
		for _, st := range snap.Plan.Steps {
			inPlan[st.ID] = true
		}
		for _, st := range snap.Plan.Steps {
			for _, dep := range st.DependsOn {
				if dep == "" || inPlan[dep] {
					continue
				}
				if _, ok := g.nodes[dep]; !ok {
					return fmt.Errorf("计划步骤 %s 的依赖 %s 既不在计划内也不在快照图中", st.ID, dep)
				}
			}
		}
	}
	// 轨迹端校验：每个轨迹节点必须存在于快照图中
	for _, id := range snap.PlanTraj {
		if _, ok := g.nodes[id]; !ok {
			return fmt.Errorf("计划轨迹节点 %s 不在快照图中", id)
		}
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.graph = g
	t.hasPlan = snap.HasPlan
	if snap.HasPlan {
		t.plan = clonePlan(snap.Plan)
	} else {
		t.plan = Plan{}
	}
	t.lastTaskID = snap.LastTaskID
	if len(snap.PlanTraj) > 0 {
		traj := make([]string, len(snap.PlanTraj))
		copy(traj, snap.PlanTraj)
		t.planTraj = traj
	} else {
		t.planTraj = nil
	}
	return nil
}

// graphFromNodes 由节点列表重建状态图（重建入边反向索引）。
// 校验：节点 ID 非空且不重复；边端点必须存在；同 (To,Kind) 边不重复。
func graphFromNodes(nodes []StateNode) (*StateGraph, error) {
	g := NewStateGraph()
	for i := range nodes {
		n := nodes[i]
		if n.ID == "" {
			return nil, fmt.Errorf("快照含空节点 ID（位置 %d）", i)
		}
		if _, ok := g.nodes[n.ID]; ok {
			return nil, fmt.Errorf("快照含重复节点 ID: %s", n.ID)
		}
		clone := cloneNode(n)
		// 出边延后校验（端点可能在其后出现），先挂节点
		g.nodes[clone.ID] = &clone
	}
	for _, n := range g.nodes {
		seen := make(map[StateEdge]bool, len(n.Edges))
		for _, e := range n.Edges {
			if _, ok := g.nodes[e.To]; !ok {
				return nil, fmt.Errorf("节点 %s 的边指向不存在的节点 %s", n.ID, e.To)
			}
			if seen[e] {
				return nil, fmt.Errorf("节点 %s 含重复边 →%s(%s)", n.ID, e.To, e.Kind)
			}
			seen[e] = true
			g.rev[e.To] = append(g.rev[e.To], n.ID)
		}
	}
	return g, nil
}
