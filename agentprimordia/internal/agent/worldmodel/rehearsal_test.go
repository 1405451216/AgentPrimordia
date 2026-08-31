// rehearsal_test.go — 预演门测试：前提满足/缺失/自依赖/顺序倒置/环/重复步骤
package worldmodel

import (
	"reflect"
	"strings"
	"testing"
)

func TestRehearse(t *testing.T) {
	t.Run("表驱动：纯计划结构检查（nil 图视作空图）", func(t *testing.T) {
		cases := []struct {
			name       string
			plan       Plan
			wantPass   bool
			wantSubstr []string // 每条缺陷消息应包含的子串（按序）
		}{
			{
				name:     "空计划直接通过",
				plan:     Plan{},
				wantPass: true,
			},
			{
				name: "依赖前序步骤：通过",
				plan: Plan{Steps: []PlanStep{
					{ID: "s1", Summary: "编译"},
					{ID: "s2", Summary: "测试", DependsOn: []string{"s1"}},
				}},
				wantPass: true,
			},
			{
				name:     "无依赖步骤：通过",
				plan:     Plan{Steps: []PlanStep{{ID: "s1", Summary: "自由动作"}}},
				wantPass: true,
			},
			{
				name:       "前提缺失：既非计划步骤也非图节点",
				plan:       Plan{Steps: []PlanStep{{ID: "s1", Summary: "动作", DependsOn: []string{"ghost"}}}},
				wantPass:   false,
				wantSubstr: []string{"缺少可执行前提：ghost"},
			},
			{
				name:       "自依赖不可满足",
				plan:       Plan{Steps: []PlanStep{{ID: "s1", Summary: "动作", DependsOn: []string{"s1"}}}},
				wantPass:   false,
				wantSubstr: []string{"自依赖"},
			},
			{
				name: "顺序倒置：依赖更晚的步骤",
				plan: Plan{Steps: []PlanStep{
					{ID: "s1", Summary: "先做", DependsOn: []string{"s2"}},
					{ID: "s2", Summary: "后做"},
				}},
				wantPass:   false,
				wantSubstr: []string{"顺序倒置"},
			},
			{
				name: "传递失败：依赖预演未通过的前序",
				plan: Plan{Steps: []PlanStep{
					{ID: "s1", Summary: "缺失前提", DependsOn: []string{"ghost"}},
					{ID: "s2", Summary: "后续", DependsOn: []string{"s1"}},
				}},
				wantPass:   false,
				wantSubstr: []string{"缺少可执行前提", "预演未通过"},
			},
			{
				name: "对抗：环（A 依赖 B，B 依赖 A）",
				plan: Plan{Steps: []PlanStep{
					{ID: "a", Summary: "A", DependsOn: []string{"b"}},
					{ID: "b", Summary: "B", DependsOn: []string{"a"}},
				}},
				wantPass:   false,
				wantSubstr: []string{"顺序倒置", "预演未通过"},
			},
			{
				name: "对抗：重复步骤 ID",
				plan: Plan{Steps: []PlanStep{
					{ID: "s1", Summary: "第一次"},
					{ID: "s1", Summary: "第二次"},
				}},
				wantPass:   false,
				wantSubstr: []string{"重复"},
			},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				got := Rehearse(tc.plan, nil)
				if got.Pass != tc.wantPass {
					t.Fatalf("Pass：got %v want %v（缺陷：%v）", got.Pass, tc.wantPass, got.MissingPreconditions)
				}
				if tc.wantPass {
					if got.MissingPreconditions != nil {
						t.Errorf("通过时不应有缺陷条目：got %v", got.MissingPreconditions)
					}
					return
				}
				if len(got.MissingPreconditions) != len(tc.wantSubstr) {
					t.Fatalf("缺陷条目数：got %d want %d（%v）", len(got.MissingPreconditions), len(tc.wantSubstr), got.MissingPreconditions)
				}
				for i, sub := range tc.wantSubstr {
					if !strings.Contains(got.MissingPreconditions[i], sub) {
						t.Errorf("缺陷[%d] 应包含 %q：got %q", i, sub, got.MissingPreconditions[i])
					}
				}
			})
		}
	})

	t.Run("依赖状态图既有节点：通过", func(t *testing.T) {
		g := NewStateGraph()
		dep := mustAdd(t, g, KindObservation, "依赖产物", 0)
		plan := Plan{Steps: []PlanStep{{ID: "s1", Summary: "消费", DependsOn: []string{dep}}}}
		got := Rehearse(plan, g)
		if !got.Pass {
			t.Errorf("图上有前提节点应通过：缺陷 %v", got.MissingPreconditions)
		}
	})

	t.Run("nil 图：计划外前提一律缺失", func(t *testing.T) {
		plan := Plan{Steps: []PlanStep{{ID: "s1", Summary: "动作", DependsOn: []string{"x"}}}}
		got := Rehearse(plan, nil)
		if got.Pass || len(got.MissingPreconditions) != 1 {
			t.Fatalf("nil 图下计划外前提应缺失：%+v", got)
		}
	})

	t.Run("与 tracker 集成：当前计划 + 图联合预演", func(t *testing.T) {
		tr := NewWorldModelTracker()
		tr.Apply(PlanRevised{Turn: 1, Task: "T", Goal: "G", Steps: []PlanStep{
			{ID: "s1", Summary: "编译"},
			{ID: "s2", Summary: "测试", DependsOn: []string{"s1"}},
		}})
		plan, ok := tr.CurrentPlan()
		if !ok {
			t.Fatal("应有当前计划")
		}
		got := Rehearse(plan, tr.Graph())
		if !got.Pass {
			t.Errorf("tracker 集成预演应通过：缺陷 %v", got.MissingPreconditions)
		}
	})

	t.Run("确定性：相同输入重复预演结果一致", func(t *testing.T) {
		plan := Plan{Steps: []PlanStep{
			{ID: "a", Summary: "A", DependsOn: []string{"b"}},
			{ID: "b", Summary: "B", DependsOn: []string{"a"}},
		}}
		want := Rehearse(plan, nil)
		for i := 0; i < 5; i++ {
			if got := Rehearse(plan, nil); !reflect.DeepEqual(got, want) {
				t.Fatalf("第 %d 次预演结果漂移：got %+v want %+v", i+1, got, want)
			}
		}
	})
}
