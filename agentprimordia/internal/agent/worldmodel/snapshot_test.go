// snapshot_test.go — 世界模型快照往返与校验（state-checkpoint 协议）
package worldmodel

import (
	"encoding/json"
	"testing"
)

func mustAddCause(t *testing.T, g *StateGraph, call, obs string) {
	t.Helper()
	cid, _ := g.AddNode(KindToolCall, call, 1)
	oid, _ := g.AddNode(KindObservation, obs, 1)
	g.AddEdge(cid, oid, EdgeCause)
}

// TestSnapshotRoundTrip 快照→JSON→Restore 后图/计划/轨迹逐项等价
func TestSnapshotRoundTrip(t *testing.T) {
	t.Parallel()
	src := NewWorldModelTracker()
	src.Apply(PlanRevised{Turn: 1, Task: "整理任务", Goal: "整理", Steps: []PlanStep{
		{ID: NodeID(KindToolCall, "read a"), Summary: "read a"},
		{ID: NodeID(KindToolCall, "write b"), Summary: "write b", DependsOn: []string{NodeID(KindToolCall, "read a")}},
	}})
	src.Apply(ToolObserved{Turn: 1, ToolName: "read", ToolInput: "a", Observation: "内容A"})
	src.Apply(ToolObserved{Turn: 2, ToolName: "write", ToolInput: "b", Observation: "已写入B"})
	src.Apply(HypothesisFormed{Turn: 2, Text: "假设H"})
	src.TrimNotification([]TrimmedMessage{{Role: "user", Content: "被裁历史"}}, 2)

	snap := src.Snapshot()
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("快照序列化失败: %v", err)
	}
	var restored Snapshot
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("快照反序列化失败: %v", err)
	}

	dst := NewWorldModelTracker()
	if err := dst.Restore(restored); err != nil {
		t.Fatalf("Restore 失败: %v", err)
	}

	// 图逐节点对账
	srcNodes, dstNodes := src.Graph().Nodes(), dst.Graph().Nodes()
	if len(srcNodes) != len(dstNodes) {
		t.Fatalf("节点数不一致: src=%d dst=%d", len(srcNodes), len(dstNodes))
	}
	for i := range srcNodes {
		if srcNodes[i].ID != dstNodes[i].ID || srcNodes[i].Summary != dstNodes[i].Summary ||
			srcNodes[i].Kind != dstNodes[i].Kind || len(srcNodes[i].Edges) != len(dstNodes[i].Edges) {
			t.Fatalf("节点 %d 不一致:\nsrc=%+v\ndst=%+v", i, srcNodes[i], dstNodes[i])
		}
		for j := range srcNodes[i].Edges {
			if srcNodes[i].Edges[j] != dstNodes[i].Edges[j] {
				t.Fatalf("节点 %s 边 %d 不一致", srcNodes[i].ID, j)
			}
		}
	}
	// 计划与轨迹
	planSrc, _ := src.CurrentPlan()
	planDst, ok := dst.CurrentPlan()
	if !ok || planDst.Goal != planSrc.Goal || len(planDst.Steps) != len(planSrc.Steps) {
		t.Fatalf("计划恢复不一致: %+v vs %+v", planSrc, planDst)
	}
	srcTraj, dstTraj := src.PlanTrajectory(), dst.PlanTrajectory()
	if len(srcTraj) != len(dstTraj) {
		t.Fatalf("轨迹恢复不一致: %v vs %v", srcTraj, dstTraj)
	}
	for i := range srcTraj {
		if srcTraj[i] != dstTraj[i] {
			t.Fatalf("轨迹 %d 不一致: %s vs %s", i, srcTraj[i], dstTraj[i])
		}
	}

	// 续知后增量可用：恢复后的 tracker 能继续应用事件且去重不重复建节点
	dst.Apply(ToolObserved{Turn: 3, ToolName: "read", ToolInput: "a", Observation: "内容A"})
	if _, isNew := dst.Graph().AddNode(KindToolCall, "read a", 3); isNew {
		t.Fatal("恢复后重复观察应去重")
	}
}

// TestSnapshotRestoreValidation 结构校验：空 ID/重复节点/悬挂边/悬挂轨迹引用
func TestSnapshotRestoreValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		snap Snapshot
	}{
		{"空节点 ID", Snapshot{Nodes: []StateNode{{Kind: KindTask, Summary: "x"}}}},
		{"重复节点 ID", Snapshot{Nodes: []StateNode{
			{ID: "a", Kind: KindTask}, {ID: "a", Kind: KindTask},
		}}},
		{"悬挂边", Snapshot{Nodes: []StateNode{
			{ID: "a", Kind: KindTask, Edges: []StateEdge{{To: "ghost", Kind: EdgeCause}}},
		}}},
		{"重复边", Snapshot{Nodes: []StateNode{
			{ID: "a", Kind: KindTask, Edges: []StateEdge{{To: "b", Kind: EdgePlan}, {To: "b", Kind: EdgePlan}}},
			{ID: "b", Kind: KindPlan},
		}}},
		{"悬挂轨迹引用", Snapshot{
			Nodes:    []StateNode{{ID: "a", Kind: KindTask}},
			HasPlan:  true,
			PlanTraj: []string{"ghost"},
		}},
		{"计划依赖悬挂", Snapshot{
			Nodes:   []StateNode{{ID: "a", Kind: KindTask}},
			Plan:    Plan{Goal: "g", Steps: []PlanStep{{ID: "s1", DependsOn: []string{"ghost"}}}},
			HasPlan: true,
		}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := NewWorldModelTracker()
			mustAddCause(t, tr.Graph(), "seed call", "seed obs")
			if err := tr.Restore(tc.snap); err == nil {
				t.Fatal("非法快照应返回错误")
			}
			// 失败的 Restore 不得改动现有状态
			if got := len(tr.Graph().Nodes()); got != 2 {
				t.Fatalf("Restore 失败后原状态不得被改动，节点数=%d", got)
			}
		})
	}
}

// TestSnapshotEmptyTracker 空 tracker 快照往返：空图 + 无计划
func TestSnapshotEmptyTracker(t *testing.T) {
	t.Parallel()
	src := NewWorldModelTracker()
	snap := src.Snapshot()
	if len(snap.Nodes) != 0 || snap.HasPlan || snap.PlanTraj != nil {
		t.Fatalf("空快照字段不符合预期: %+v", snap)
	}
	dst := NewWorldModelTracker()
	mustAddCause(t, dst.Graph(), "c", "o")
	if err := dst.Restore(snap); err != nil {
		t.Fatalf("空快照应可恢复: %v", err)
	}
	if got := len(dst.Graph().Nodes()); got != 0 {
		t.Fatalf("恢复后应为空图，got %d", got)
	}
	if _, ok := dst.CurrentPlan(); ok {
		t.Fatal("恢复后不应有计划")
	}
}

// TestSnapshotDefensiveCopy 快照深拷贝：外部改动不回流 tracker
func TestSnapshotDefensiveCopy(t *testing.T) {
	t.Parallel()
	tr := NewWorldModelTracker()
	mustAddCause(t, tr.Graph(), "c", "o")
	snap := tr.Snapshot()
	snap.Nodes[0].Summary = "tampered"
	if n, _ := tr.Graph().Node(snap.Nodes[0].ID); n.Summary == "tampered" {
		t.Fatal("快照改动不得回流内部状态")
	}
}
