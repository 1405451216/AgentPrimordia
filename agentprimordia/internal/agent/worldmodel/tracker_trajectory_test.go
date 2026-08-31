// tracker_trajectory_test.go — NodeID 派生与计划轨迹（切片二接线契约）
//
// 覆盖接线点②⑥的内核侧契约：
//   - NodeID 与 AddNode 的 ID 派生规则一致（计划步骤 ID 与调用节点 ID 收敛）；
//   - PlanTrajectory 随 ToolObserved 增长、随 PlanRevised 重置；
//   - 回溯差异端到端：计划步骤（NodeID 派生） vs 实际轨迹，偏离可检出。
package worldmodel

import "testing"

func TestNodeID_MatchesAddNode(t *testing.T) {
	t.Parallel()
	g := NewStateGraph()
	got, isNew := g.AddNode(KindToolCall, "read_file  {\"path\":\"a.txt\"}", 3)
	if !isNew {
		t.Fatal("首添应为新建")
	}
	want := NodeID(KindToolCall, "read_file  {\"path\":\"a.txt\"}")
	if got != want {
		t.Fatalf("NodeID 与 AddNode 派生不一致：got %s want %s", got, want)
	}
	// 规范化等价：仅空白差异 → 同一 ID
	if NodeID(KindToolCall, " read_file\t {\"path\":\"a.txt\"} ") != want {
		t.Fatal("空白规范化后应得同一 ID")
	}
	// 不同 Kind → 不同 ID
	if NodeID(KindObservation, "read_file {\"path\":\"a.txt\"}") == want {
		t.Fatal("不同 Kind 不得碰撞")
	}
}

// TestPlanTrajectory_AppendAndReset 验证：轨迹随工具观测增长、随计划修订重置、
// 计划步骤 ID（NodeID 派生）与实际调用节点 ID 同空间收敛。
func TestPlanTrajectory_AppendAndReset(t *testing.T) {
	t.Parallel()
	tr := NewWorldModelTracker()

	stepA := PlanStep{ID: NodeID(KindToolCall, "read_file a.txt"), Summary: "read_file a.txt"}
	stepB := PlanStep{ID: NodeID(KindToolCall, "write_file b.txt"), Summary: "write_file b.txt"}
	tr.Apply(PlanRevised{Turn: 1, Goal: "g", Steps: []PlanStep{stepA, stepB}})

	if got := tr.PlanTrajectory(); got != nil {
		t.Fatalf("计划形成后尚无观测，轨迹应为 nil，got %v", got)
	}

	tr.Apply(ToolObserved{Turn: 1, ToolName: "read_file", ToolInput: "a.txt", Observation: "内容"})
	tr.Apply(ToolObserved{Turn: 1, ToolName: "write_file", ToolInput: "b.txt", Observation: "已写入"})

	got := tr.PlanTrajectory()
	if len(got) != 2 {
		t.Fatalf("两次观测后轨迹长度应为 2，got %v", got)
	}
	if got[0] != stepA.ID || got[1] != stepB.ID {
		t.Fatalf("轨迹节点应与计划步骤 ID 收敛：got %v want [%s %s]", got, stepA.ID, stepB.ID)
	}

	// 覆盖式修订重置轨迹
	tr.Apply(PlanRevised{Turn: 2, Goal: "g2", Steps: []PlanStep{stepA}})
	if got := tr.PlanTrajectory(); got != nil {
		t.Fatalf("计划修订后轨迹应重置为 nil，got %v", got)
	}
	tr.Apply(ToolObserved{Turn: 2, ToolName: "read_file", ToolInput: "a.txt", Observation: "内容"})
	if got := tr.PlanTrajectory(); len(got) != 1 || got[0] != stepA.ID {
		t.Fatalf("修订后观测应只含新调用：got %v", got)
	}
}

// TestPlanTrajectory_SnapshotDefensive 防御性拷贝：外部改动不回流内部状态。
func TestPlanTrajectory_SnapshotDefensive(t *testing.T) {
	t.Parallel()
	tr := NewWorldModelTracker()
	tr.Apply(PlanRevised{Turn: 1, Goal: "g", Steps: []PlanStep{{ID: NodeID(KindToolCall, "t x"), Summary: "t x"}}})
	tr.Apply(ToolObserved{Turn: 1, ToolName: "t", ToolInput: "x", Observation: "o"})

	got := tr.PlanTrajectory()
	got[0] = "tampered"
	if again := tr.PlanTrajectory(); again[0] == "tampered" {
		t.Fatal("返回值应为防御性拷贝，外部改动不得回流")
	}
}

// TestBackDiff_WithNodeIDSpace 端到端：接线层用 NodeID 派生计划步骤 ID 后，
// ComparePaths(计划路径, 计划轨迹) 能正确区分一致与偏离（含参数被替换场景）。
func TestBackDiff_WithNodeIDSpace(t *testing.T) {
	t.Parallel()
	tr := NewWorldModelTracker()
	announceA := NodeID(KindToolCall, "search q=1")
	announceB := NodeID(KindToolCall, "fetch url=2")
	tr.Apply(PlanRevised{Turn: 1, Goal: "g", Steps: []PlanStep{
		{ID: announceA, Summary: "search q=1"},
		{ID: announceB, Summary: "fetch url=2"},
	}})

	// 一致场景：两个调用按计划执行
	tr.Apply(ToolObserved{Turn: 1, ToolName: "search", ToolInput: "q=1", Observation: "r1"})
	tr.Apply(ToolObserved{Turn: 1, ToolName: "fetch", ToolInput: "url=2", Observation: "r2"})
	plan, ok := tr.CurrentPlan()
	if !ok {
		t.Fatal("应有当前计划")
	}
	if diff := ComparePaths(plan.Path(), tr.PlanTrajectory()); diff.DivergedAt != "" {
		t.Fatalf("一致轨迹不应报偏离： %+v", diff)
	}

	// 偏离场景：第二步参数被替换（计划修订前实际执行了不同调用）
	tr.Apply(PlanRevised{Turn: 2, Goal: "g", Steps: []PlanStep{
		{ID: announceA, Summary: "search q=1"},
		{ID: announceB, Summary: "fetch url=2"},
	}})
	tr.Apply(ToolObserved{Turn: 2, ToolName: "search", ToolInput: "q=1", Observation: "r1"})
	tr.Apply(ToolObserved{Turn: 2, ToolName: "fetch", ToolInput: "url=3", Observation: "r3"})
	diff := ComparePaths(plan.Path(), tr.PlanTrajectory())
	if diff.DivergedAt != announceB {
		t.Fatalf("偏离锚点应为被替换步骤 %s，got %q", announceB, diff.DivergedAt)
	}
	if len(diff.ExecutedButUnplanned) != 1 ||
		diff.ExecutedButUnplanned[0] != NodeID(KindToolCall, "fetch url=3") {
		t.Fatalf("计划外执行应为替换后的调用节点，got %+v", diff)
	}
}
