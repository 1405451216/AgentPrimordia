// tracker_test.go — WorldModelTracker 测试：事件增量、裁剪搬图、乱序/重复对抗、并发
package worldmodel

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
)

func TestApplyToolObserved(t *testing.T) {
	t.Run("建立 工具调用→观察 因果边", func(t *testing.T) {
		tr := NewWorldModelTracker()
		tr.Apply(ToolObserved{Turn: 2, ToolName: "read_file", ToolInput: "a.txt", Observation: "hello"})
		g := tr.Graph()
		nodes := g.Nodes()
		if len(nodes) != 2 {
			t.Fatalf("应产生 2 个节点（调用+观察）：got %d", len(nodes))
		}
		var call, obs *StateNode
		for i := range nodes {
			switch nodes[i].Kind {
			case KindToolCall:
				call = &nodes[i]
			case KindObservation:
				obs = &nodes[i]
			}
		}
		if call == nil || obs == nil {
			t.Fatalf("应包含 tool_call 与 observation 节点：%+v", nodes)
		}
		if call.Summary != "read_file a.txt" {
			t.Errorf("调用节点摘要应为『工具名 输入』：got %q", call.Summary)
		}
		if call.CreatedAtTurn != 2 || obs.CreatedAtTurn != 2 {
			t.Errorf("CreatedAtTurn 应取事件自带 Turn：got %d/%d", call.CreatedAtTurn, obs.CreatedAtTurn)
		}
		if len(call.Edges) != 1 || call.Edges[0].To != obs.ID || call.Edges[0].Kind != EdgeCause {
			t.Errorf("调用节点应恰有一条指向观察的 cause 边：got %+v", call.Edges)
		}
	})

	t.Run("重复事件幂等（节点与边去重）", func(t *testing.T) {
		tr := NewWorldModelTracker()
		ev := ToolObserved{Turn: 1, ToolName: "ls", ToolInput: ".", Observation: "empty"}
		tr.Apply(ev)
		tr.Apply(ev)
		if got := len(tr.Graph().Nodes()); got != 2 {
			t.Errorf("重复事件不应产生新节点：got %d want 2", got)
		}
	})
}

func TestApplyPlanRevised(t *testing.T) {
	t.Run("任务→计划→步骤 的计划边结构与当前计划", func(t *testing.T) {
		tr := NewWorldModelTracker()
		tr.Apply(PlanRevised{
			Turn: 1,
			Task: "修复构建",
			Goal: "先测试后提交",
			Steps: []PlanStep{
				{ID: "s1", Summary: "go build ./..."},
				{ID: "s2", Summary: "go test ./...", DependsOn: []string{"s1"}},
			},
		})
		if got := len(tr.Graph().Nodes()); got != 4 {
			t.Fatalf("应有 任务+计划+两步骤 共 4 节点：got %d", got)
		}
		plan, ok := tr.CurrentPlan()
		if !ok {
			t.Fatal("PlanRevised 后应有当前计划")
		}
		if plan.Goal != "先测试后提交" || len(plan.Steps) != 2 {
			t.Fatalf("当前计划内容不符：%+v", plan)
		}
		if plan.Steps[1].DependsOn[0] != "s1" {
			t.Errorf("DependsOn 应保留：%v", plan.Steps[1].DependsOn)
		}
	})

	t.Run("步骤 ID 缺省时确定性派生", func(t *testing.T) {
		tr := NewWorldModelTracker()
		tr.Apply(PlanRevised{Turn: 0, Goal: "g", Steps: []PlanStep{{Summary: "a"}, {Summary: "b"}}})
		plan, _ := tr.CurrentPlan()
		if plan.Steps[0].ID != "step-1" || plan.Steps[1].ID != "step-2" {
			t.Errorf("缺省步骤 ID 应为 step-N：got %q,%q", plan.Steps[0].ID, plan.Steps[1].ID)
		}
	})

	t.Run("同目标修订复用计划节点并覆盖当前计划", func(t *testing.T) {
		tr := NewWorldModelTracker()
		tr.Apply(PlanRevised{Turn: 1, Task: "T", Goal: "G", Steps: []PlanStep{{ID: "a", Summary: "A"}}})
		tr.Apply(PlanRevised{Turn: 2, Task: "T", Goal: "G", Steps: []PlanStep{{ID: "b", Summary: "B"}}})
		planCount := 0
		for _, n := range tr.Graph().Nodes() {
			if n.Kind == KindPlan {
				planCount++
			}
		}
		if planCount != 1 {
			t.Errorf("同目标计划节点应去重复用：got %d", planCount)
		}
		plan, _ := tr.CurrentPlan()
		if len(plan.Steps) != 1 || plan.Steps[0].ID != "b" {
			t.Errorf("修订为覆盖语义，当前计划应为新步骤：got %+v", plan.Steps)
		}
	})

	t.Run("空 Task 跳过任务节点", func(t *testing.T) {
		tr := NewWorldModelTracker()
		tr.Apply(PlanRevised{Turn: 0, Goal: "G", Steps: nil})
		for _, n := range tr.Graph().Nodes() {
			if n.Kind == KindTask {
				t.Errorf("空 Task 不应产生任务节点：%+v", n)
			}
		}
	})

	t.Run("CurrentPlan 返回深拷贝", func(t *testing.T) {
		tr := NewWorldModelTracker()
		tr.Apply(PlanRevised{Turn: 0, Goal: "G", Steps: []PlanStep{{ID: "s1", Summary: "S", DependsOn: []string{"x"}}}})
		p1, _ := tr.CurrentPlan()
		p1.Steps[0].DependsOn[0] = "篡改"
		p2, _ := tr.CurrentPlan()
		if p2.Steps[0].DependsOn[0] != "x" {
			t.Error("CurrentPlan 应返回深拷贝，外部改动不得回流")
		}
	})
}

func TestApplyHypothesisFormed(t *testing.T) {
	t.Run("有任务时假设挂到任务（hypothesis 边）", func(t *testing.T) {
		tr := NewWorldModelTracker()
		tr.Apply(PlanRevised{Turn: 1, Task: "排查", Goal: "G"})
		tr.Apply(HypothesisFormed{Turn: 2, Text: "可能是超时"})
		var hyp StateNode
		found := false
		for _, n := range tr.Graph().Nodes() {
			if n.Kind == KindHypothesis {
				hyp, found = n, true
			}
		}
		if !found {
			t.Fatal("应产生 hypothesis 节点")
		}
		if hyp.CreatedAtTurn != 2 {
			t.Errorf("CreatedAtTurn 应为 2：got %d", hyp.CreatedAtTurn)
		}
		anc := tr.Graph().Ancestors(hyp.ID)
		if len(anc) != 1 {
			t.Fatalf("假设应有唯一祖先（任务）：got %v", anc)
		}
		taskNode, _ := tr.Graph().Node(anc[0])
		if taskNode.Kind != KindTask {
			t.Errorf("假设的祖先应为任务节点：got kind=%s", taskNode.Kind)
		}
	})

	t.Run("无任务时假设孤立成节点", func(t *testing.T) {
		tr := NewWorldModelTracker()
		tr.Apply(HypothesisFormed{Turn: 0, Text: "独立假设"})
		nodes := tr.Graph().Nodes()
		if len(nodes) != 1 || nodes[0].Kind != KindHypothesis {
			t.Fatalf("应只有孤立假设节点：got %+v", nodes)
		}
		if anc := tr.Graph().Ancestors(nodes[0].ID); len(anc) != 0 {
			t.Errorf("孤立假设不应有祖先：got %v", anc)
		}
	})
}

func TestTrimNotification(t *testing.T) {
	t.Run("被裁消息转为观察节点并挂到任务（世界搬进图）", func(t *testing.T) {
		tr := NewWorldModelTracker()
		tr.Apply(PlanRevised{Turn: 1, Task: "长程任务", Goal: "G"})
		created := tr.TrimNotification([]TrimmedMessage{
			{Role: "user", Content: "最初的需求是部署服务", Turn: 0},
			{Role: "tool", Content: "步骤一已生成配置", Turn: 1},
		}, 9)
		if len(created) != 2 {
			t.Fatalf("应新建 2 个事实节点：got %v", created)
		}
		g := tr.Graph()
		for i, id := range created {
			n, ok := g.Node(id)
			if !ok {
				t.Fatalf("新建节点应存在：%q", id)
			}
			if n.Kind != KindObservation {
				t.Errorf("被裁事实应为 observation 节点：got %s", n.Kind)
			}
			if want := []int{0, 1}[i]; n.CreatedAtTurn != want {
				t.Errorf("事实节点轮次应取消息自带轮次：got %d want %d", n.CreatedAtTurn, want)
			}
			anc := g.Ancestors(id)
			if len(anc) != 1 {
				t.Fatalf("事实节点应挂到任务：got ancestors %v", anc)
			}
			tn, _ := g.Node(anc[0])
			if tn.Kind != KindTask {
				t.Errorf("事实节点的祖先应为任务节点：got %s", tn.Kind)
			}
		}
	})

	t.Run("消息轮次未知时回退裁剪轮次", func(t *testing.T) {
		tr := NewWorldModelTracker()
		created := tr.TrimNotification([]TrimmedMessage{{Role: "user", Content: "未知轮次", Turn: -1}}, 7)
		n, _ := tr.Graph().Node(created[0])
		if n.CreatedAtTurn != 7 {
			t.Errorf("未知轮次应回退为裁剪轮次：got %d want 7", n.CreatedAtTurn)
		}
	})

	t.Run("对抗：重复裁剪同内容不产生重复节点（幂等）", func(t *testing.T) {
		tr := NewWorldModelTracker()
		first := tr.TrimNotification([]TrimmedMessage{{Content: "同一条事实", Turn: 0}}, 1)
		second := tr.TrimNotification([]TrimmedMessage{{Content: "同一条事实", Turn: 0}}, 2)
		if len(first) != 1 || len(second) != 0 {
			t.Errorf("重复裁剪应幂等：first=%v second=%v", first, second)
		}
		if got := len(tr.Graph().Nodes()); got != 1 {
			t.Errorf("图中应只有 1 个节点：got %d", got)
		}
	})

	t.Run("对抗：空白差异的重复内容同样去重", func(t *testing.T) {
		tr := NewWorldModelTracker()
		tr.TrimNotification([]TrimmedMessage{{Content: "部署  服务", Turn: 0}}, 1)
		second := tr.TrimNotification([]TrimmedMessage{{Content: " 部署\t服务 \n", Turn: 1}}, 2)
		if len(second) != 0 {
			t.Errorf("规范化后同内容应去重：got %v", second)
		}
	})
}

// 对抗：乱序事件流。事件自带 Turn，tracker 按提交序增量应用，
// 不重排、不假设时间单调；计划步骤与先到的实际调用经去重收敛同一节点。
func TestTrackerAdversarialOutOfOrderEvents(t *testing.T) {
	tr := NewWorldModelTracker()
	tr.Apply(HypothesisFormed{Turn: 5, Text: "先有假设"})                                     // 计划尚未出现
	tr.Apply(ToolObserved{Turn: 3, ToolName: "ls", ToolInput: ".", Observation: "empty"}) // 先于计划执行
	tr.Apply(PlanRevised{Turn: 1, Task: "任务", Goal: "G", Steps: []PlanStep{{ID: "s1", Summary: "ls ."}}})
	tr.Apply(ToolObserved{Turn: 2, ToolName: "ls", ToolInput: ".", Observation: "empty"}) // 与计划步骤去重收敛

	g := tr.Graph()
	// 节点：假设 + 工具调用(与计划步骤同一节点) + 观察 + 任务 + 计划 = 5
	if got := len(g.Nodes()); got != 5 {
		t.Fatalf("乱序流后应有 5 个节点：got %d", got)
	}

	// 计划步骤与实际调用去重收敛：步骤节点执行后出现因果边（观测态分型）
	var callID string
	for _, n := range g.Nodes() {
		if n.Kind == KindToolCall {
			callID = n.ID
		}
	}
	n, _ := g.Node(callID)
	if len(n.Edges) != 1 || n.Edges[0].Kind != EdgeCause {
		t.Errorf("步骤节点执行后应恰有因果边（观测态分型）：got %+v", n.Edges)
	}
	obs, _ := g.Node(n.Edges[0].To)
	if obs.Kind != KindObservation {
		t.Errorf("因果边应指向观察节点：got %s", obs.Kind)
	}

	// 确定性：同序列重放得到相同节点集合
	tr2 := NewWorldModelTracker()
	tr2.Apply(HypothesisFormed{Turn: 5, Text: "先有假设"})
	tr2.Apply(ToolObserved{Turn: 3, ToolName: "ls", ToolInput: ".", Observation: "empty"})
	tr2.Apply(PlanRevised{Turn: 1, Task: "任务", Goal: "G", Steps: []PlanStep{{ID: "s1", Summary: "ls ."}}})
	tr2.Apply(ToolObserved{Turn: 2, ToolName: "ls", ToolInput: ".", Observation: "empty"})
	if !reflect.DeepEqual(nodeIDSet(g), nodeIDSet(tr2.Graph())) {
		t.Errorf("乱序流重放应确定：\n got %v\nwant %v", nodeIDSet(g), nodeIDSet(tr2.Graph()))
	}
}

func TestApplyPointerEvents(t *testing.T) {
	tr := NewWorldModelTracker()
	tr.Apply(&ToolObserved{Turn: 1, ToolName: "ls", ToolInput: ".", Observation: "x"})
	tr.Apply(&PlanRevised{Turn: 1, Task: "T", Goal: "G"})
	tr.Apply(&HypothesisFormed{Turn: 1, Text: "H"})
	if got := len(tr.Graph().Nodes()); got != 5 {
		t.Errorf("指针事件应与值事件等效：got %d want 5", got)
	}
}

type fakeUnknownEvent struct{}

func (fakeUnknownEvent) eventType() EventType { return "unknown" }

func TestApplyIgnoresNilAndUnknown(t *testing.T) {
	tr := NewWorldModelTracker()
	tr.Apply(nil)
	tr.Apply(fakeUnknownEvent{})
	var nilPtr *ToolObserved
	tr.Apply(nilPtr) // nil 指针事件：忽略不 panic
	if got := len(tr.Graph().Nodes()); got != 0 {
		t.Errorf("nil/未知事件应被忽略：got %d 节点", got)
	}
}

func TestTrackerConcurrent(t *testing.T) {
	tr := NewWorldModelTracker()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tr.Apply(ToolObserved{Turn: i, ToolName: "t", ToolInput: fmt.Sprint(i), Observation: "o"})
		}(i)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tr.Apply(PlanRevised{Turn: i, Task: fmt.Sprintf("任务 %d", i), Goal: "G"})
		}(i)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tr.TrimNotification([]TrimmedMessage{{Content: fmt.Sprintf("事实 %d", i), Turn: i}}, i)
		}(i)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = tr.CurrentPlan()
			_ = tr.Graph().Nodes()
		}()
	}
	wg.Wait()
	// 节点计数：8 调用 + 1 观察（同内容 "o" 去重）+ 4 任务 + 1 计划（同 Goal 去重）+ 4 事实 = 18
	if got := len(tr.Graph().Nodes()); got != 18 {
		t.Errorf("并发事件后节点数应为 18：got %d", got)
	}
}

// nodeIDSet 返回按 ID 升序的节点 ID 集合（Nodes() 已排序，可直接 DeepEqual）。
func nodeIDSet(g *StateGraph) []string {
	var ids []string
	for _, n := range g.Nodes() {
		ids = append(ids, n.ID)
	}
	return ids
}
