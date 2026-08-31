// backdiff_test.go — 回溯差异测试：确定性 + 乱序/重复节点/环 对抗用例
package worldmodel

import (
	"reflect"
	"testing"
)

func TestComparePaths(t *testing.T) {
	cases := []struct {
		name          string
		planned       []string
		actual        []string
		wantDiverged  string
		wantSkipped   []string
		wantUnplanned []string
	}{
		{
			name:         "完全一致",
			planned:      []string{"a", "b", "c"},
			actual:       []string{"a", "b", "c"},
			wantDiverged: "",
		},
		{
			name:          "计划未走完（提前终止）",
			planned:       []string{"a", "b", "c"},
			actual:        []string{"a"},
			wantDiverged:  "b",
			wantSkipped:   []string{"b", "c"},
			wantUnplanned: nil,
		},
		{
			name:          "计划外追加",
			planned:       []string{"a", "b"},
			actual:        []string{"a", "b", "x"},
			wantDiverged:  "x",
			wantSkipped:   nil,
			wantUnplanned: []string{"x"},
		},
		{
			name:          "中途分歧",
			planned:       []string{"a", "b", "c"},
			actual:        []string{"a", "x"},
			wantDiverged:  "b",
			wantSkipped:   []string{"b", "c"},
			wantUnplanned: []string{"x"},
		},
		{
			// 乱序：内容全部执行但顺序偏离计划——靠锚点暴露，不误报跳过/计划外
			name:         "对抗：乱序执行（内容重排）",
			planned:      []string{"a", "b", "c"},
			actual:       []string{"a", "c", "b"},
			wantDiverged: "b",
		},
		{
			// 重复节点：步骤 a 重复执行——重复由锚点暴露，全长出现过即不算计划外
			name:         "对抗：重复节点（步骤重复执行）",
			planned:      []string{"a", "b"},
			actual:       []string{"a", "a", "b"},
			wantDiverged: "b",
		},
		{
			name:         "空计划空轨迹",
			planned:      nil,
			actual:       nil,
			wantDiverged: "",
		},
		{
			name:          "计划为空而实际有动作",
			planned:       nil,
			actual:        []string{"a"},
			wantDiverged:  "a",
			wantUnplanned: []string{"a"},
		},
		{
			name:         "计划有而轨迹为空",
			planned:      []string{"a"},
			actual:       nil,
			wantDiverged: "a",
			wantSkipped:  []string{"a"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := ComparePaths(tc.planned, tc.actual)
			if got.DivergedAt != tc.wantDiverged {
				t.Errorf("DivergedAt：got %q want %q", got.DivergedAt, tc.wantDiverged)
			}
			if !reflect.DeepEqual(got.PlannedButSkipped, tc.wantSkipped) {
				t.Errorf("PlannedButSkipped：got %v want %v", got.PlannedButSkipped, tc.wantSkipped)
			}
			if !reflect.DeepEqual(got.ExecutedButUnplanned, tc.wantUnplanned) {
				t.Errorf("ExecutedButUnplanned：got %v want %v", got.ExecutedButUnplanned, tc.wantUnplanned)
			}
		})
	}
}

func TestComparePathsDeterministic(t *testing.T) {
	planned := []string{"a", "b", "c", "d"}
	actual := []string{"a", "x", "b", "d"}
	want := ComparePaths(planned, actual)
	for i := 0; i < 10; i++ {
		if got := ComparePaths(planned, actual); !reflect.DeepEqual(got, want) {
			t.Fatalf("第 %d 次结果与首次不一致：got %+v want %+v", i+1, got, want)
		}
	}
}

func TestPlanPath(t *testing.T) {
	p := Plan{Goal: "g", Steps: []PlanStep{{ID: "s2", Summary: "B"}, {ID: "s1", Summary: "A"}}}
	if got := p.Path(); !reflect.DeepEqual(got, []string{"s2", "s1"}) {
		t.Errorf("Path 应保持计划步骤序：got %v", got)
	}
	if got := (Plan{}).Path(); got == nil || len(got) != 0 {
		t.Errorf("空计划 Path 应为空切片：got %v", got)
	}
}

// 对抗：环。实际轨迹取自含环状态图（PathTo 必须有限终止），
// 再与计划路径对账——环不得破坏回溯差异的确定性。
func TestComparePathsWithCyclicTrajectoryGraph(t *testing.T) {
	g := NewStateGraph()
	r := mustAdd(t, g, KindTask, "任务", 0)
	a := mustAdd(t, g, KindToolCall, "步骤A", 0)
	b := mustAdd(t, g, KindToolCall, "步骤B", 0)
	c := mustAdd(t, g, KindObservation, "意外观察", 0)
	_ = g.AddEdge(r, a, EdgePlan)
	_ = g.AddEdge(a, b, EdgeCause)
	_ = g.AddEdge(b, c, EdgeCause)
	_ = g.AddEdge(c, b, EdgeCause) // 环 B→C→B

	actual := g.PathTo(c)
	if !reflect.DeepEqual(actual, []string{r, a, b, c}) {
		t.Fatalf("含环图的实际轨迹应为有限路径：got %v", actual)
	}
	plan := Plan{Goal: "G", Steps: []PlanStep{
		{ID: r, Summary: "任务"},
		{ID: a, Summary: "步骤A"},
		{ID: b, Summary: "步骤B"},
	}}
	got := ComparePaths(plan.Path(), actual)
	if got.DivergedAt != c {
		t.Errorf("分歧锚点应为计划外观察节点：got %q want %q", got.DivergedAt, c)
	}
	if len(got.PlannedButSkipped) != 0 {
		t.Errorf("计划步骤均已执行，不应有跳过：got %v", got.PlannedButSkipped)
	}
	if !reflect.DeepEqual(got.ExecutedButUnplanned, []string{c}) {
		t.Errorf("计划外执行应为 %q：got %v", c, got.ExecutedButUnplanned)
	}
}
