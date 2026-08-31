// graph_test.go — 状态图内核测试：去重/确定性/防御拷贝/环安全/并发
package worldmodel

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"
)

// mustAdd 辅助：断言式加节点，返回 ID。
func mustAdd(t *testing.T, g *StateGraph, kind NodeKind, summary string, turn int) string {
	t.Helper()
	id, created := g.AddNode(kind, summary, turn)
	if id == "" {
		t.Fatalf("AddNode 不应返回空 ID：kind=%s summary=%q", kind, summary)
	}
	if !created {
		t.Errorf("新图首次 AddNode 应报告新建：kind=%s summary=%q", kind, summary)
	}
	return id
}

func TestAddNodeDedupByID(t *testing.T) {
	t.Run("同 Kind 同摘要（仅空白差异）去重为同一节点", func(t *testing.T) {
		g := NewStateGraph()
		id1, c1 := g.AddNode(KindToolCall, "read_file  a.txt", 1)
		id2, c2 := g.AddNode(KindToolCall, "read_file \t a.txt \n", 2)
		if id1 != id2 {
			t.Errorf("规范化后摘要相同的节点应同 ID：got %q vs %q", id1, id2)
		}
		if !c1 || c2 {
			t.Errorf("首次应新建(c1=%v)，重复应命中去重(c2=%v)", c1, c2)
		}
		n, ok := g.Node(id1)
		if !ok {
			t.Fatal("去重命中后节点应存在")
		}
		if n.Summary != "read_file a.txt" {
			t.Errorf("节点摘要应为规范化形式：got %q", n.Summary)
		}
		if n.CreatedAtTurn != 1 {
			t.Errorf("CreatedAtTurn 应首见优先不回改：got %d want 1", n.CreatedAtTurn)
		}
	})

	t.Run("不同 Kind 同摘要不同 ID", func(t *testing.T) {
		g := NewStateGraph()
		idA, _ := g.AddNode(KindTask, "部署服务", 0)
		idB, _ := g.AddNode(KindPlan, "部署服务", 0)
		if idA == idB {
			t.Errorf("Kind 参与去重键，不同 Kind 不应同 ID：%q", idA)
		}
	})

	t.Run("确定性：相同插入序列在两张图产生相同 ID 序列", func(t *testing.T) {
		build := func() []string {
			g := NewStateGraph()
			var ids []string
			for _, s := range []struct {
				k NodeKind
				s string
			}{
				{KindTask, "修复缺陷"},
				{KindPlan, "三步修复"},
				{KindToolCall, "go test ./..."},
				{KindObservation, "全部通过"},
				{KindHypothesis, "空指针未判"},
			} {
				id, _ := g.AddNode(s.k, s.s, 0)
				ids = append(ids, id)
			}
			return ids
		}
		a, b := build(), build()
		if !reflect.DeepEqual(a, b) {
			t.Errorf("确定性 ID 被破坏：\n got %v\nwant %v", a, b)
		}
	})
}

func TestAddEdge(t *testing.T) {
	t.Run("正常加边与边去重", func(t *testing.T) {
		g := NewStateGraph()
		a := mustAdd(t, g, KindToolCall, "read a", 0)
		b := mustAdd(t, g, KindObservation, "内容", 0)
		if !g.AddEdge(a, b, EdgeCause) {
			t.Fatal("首条边应新建成功")
		}
		if g.AddEdge(a, b, EdgeCause) {
			t.Error("重复同种边不应重复新建")
		}
		if !g.AddEdge(a, b, EdgePlan) {
			t.Error("同端点不同 EdgeKind 是不同边，应允许新建")
		}
		n, _ := g.Node(a)
		if len(n.Edges) != 2 {
			t.Fatalf("出边数应为 2：got %d", len(n.Edges))
		}
		if n.Edges[0].To != b || n.Edges[0].Kind != EdgeCause {
			t.Errorf("首条出边应为 cause→%s：got %+v", b, n.Edges[0])
		}
	})

	t.Run("端点不存在时拒绝加边", func(t *testing.T) {
		g := NewStateGraph()
		a := mustAdd(t, g, KindTask, "任务", 0)
		if g.AddEdge(a, "ghost", EdgeCause) {
			t.Error("目标节点不存在应返回 false")
		}
		if g.AddEdge("ghost", a, EdgeCause) {
			t.Error("来源节点不存在应返回 false")
		}
	})
}

func TestNodeAndNodesDefensiveCopy(t *testing.T) {
	g := NewStateGraph()
	a := mustAdd(t, g, KindTask, "T", 0)
	b := mustAdd(t, g, KindObservation, "O", 1)
	_ = g.AddEdge(a, b, EdgeCause)

	n, _ := g.Node(a)
	n.Edges[0].To = "篡改"
	n.Summary = "篡改"
	got, _ := g.Node(a)
	if got.Summary != "T" || got.Edges[0].To != b {
		t.Error("Node 返回值应与内部状态隔离（防御性拷贝）")
	}

	nodes := g.Nodes()
	nodes[0].Summary = "篡改"
	again := g.Nodes()
	if again[0].Summary == "篡改" {
		t.Error("Nodes 返回值应与内部状态隔离")
	}
	if len(again) != 2 {
		t.Fatalf("节点数应为 2：got %d", len(again))
	}
	if again[0].ID >= again[1].ID {
		t.Errorf("Nodes 应按 ID 升序：got %q, %q", again[0].ID, again[1].ID)
	}
	if _, ok := g.Node("不存在"); ok {
		t.Error("不存在的节点不应返回 ok=true")
	}
}

func TestAncestors(t *testing.T) {
	t.Run("菱形结构收集全部祖先（升序）", func(t *testing.T) {
		g := NewStateGraph()
		a := mustAdd(t, g, KindTask, "A", 0)
		b := mustAdd(t, g, KindPlan, "B", 0)
		c := mustAdd(t, g, KindPlan, "C", 0)
		d := mustAdd(t, g, KindObservation, "D", 0)
		_ = g.AddEdge(a, b, EdgePlan)
		_ = g.AddEdge(a, c, EdgePlan)
		_ = g.AddEdge(b, d, EdgePlan)
		_ = g.AddEdge(c, d, EdgePlan)
		want := []string{a, b, c}
		sort.Strings(want)
		if got := g.Ancestors(d); !reflect.DeepEqual(got, want) {
			t.Errorf("Ancestors 应为 %v：got %v", want, got)
		}
	})

	t.Run("对抗：环与自环安全且不含查询节点自身", func(t *testing.T) {
		g := NewStateGraph()
		x := mustAdd(t, g, KindTask, "X", 0)
		y := mustAdd(t, g, KindObservation, "Y", 0)
		_ = g.AddEdge(x, y, EdgeCause)
		_ = g.AddEdge(y, x, EdgeCause) // 环 X→Y→X
		_ = g.AddEdge(x, x, EdgeCause) // 自环
		if got := g.Ancestors(x); !reflect.DeepEqual(got, []string{y}) {
			t.Errorf("环中 X 的祖先应恰为 Y：got %v", got)
		}
	})

	t.Run("节点不存在返回 nil", func(t *testing.T) {
		g := NewStateGraph()
		if got := g.Ancestors("ghost"); got != nil {
			t.Errorf("不存在节点应返回 nil：got %v", got)
		}
	})
}

func TestPathTo(t *testing.T) {
	t.Run("直线图返回根到目标全路径", func(t *testing.T) {
		g := NewStateGraph()
		r := mustAdd(t, g, KindTask, "根", 0)
		m := mustAdd(t, g, KindPlan, "中", 0)
		d := mustAdd(t, g, KindObservation, "尾", 0)
		_ = g.AddEdge(r, m, EdgePlan)
		_ = g.AddEdge(m, d, EdgeCause)
		if got := g.PathTo(d); !reflect.DeepEqual(got, []string{r, m, d}) {
			t.Errorf("PathTo 应为根→尾全路径：got %v", got)
		}
		if got := g.PathTo(r); !reflect.DeepEqual(got, []string{r}) {
			t.Errorf("根自身路径应为 [根]：got %v", got)
		}
	})

	t.Run("对抗：含环节点仍返回有限路径", func(t *testing.T) {
		g := NewStateGraph()
		r := mustAdd(t, g, KindTask, "R", 0)
		a := mustAdd(t, g, KindToolCall, "A", 0)
		b := mustAdd(t, g, KindObservation, "B", 0)
		_ = g.AddEdge(r, a, EdgePlan)
		_ = g.AddEdge(a, b, EdgeCause)
		_ = g.AddEdge(b, a, EdgeCause) // 环 A→B→A
		if got := g.PathTo(b); !reflect.DeepEqual(got, []string{r, a, b}) {
			t.Errorf("含环图 PathTo 应有限终止且路径正确：got %v", got)
		}
	})

	t.Run("对抗：纯环无根返回 nil", func(t *testing.T) {
		g := NewStateGraph()
		a := mustAdd(t, g, KindTask, "A", 0)
		b := mustAdd(t, g, KindObservation, "B", 0)
		_ = g.AddEdge(a, b, EdgeCause)
		_ = g.AddEdge(b, a, EdgeCause)
		if got := g.PathTo(b); got != nil {
			t.Errorf("无根纯环不可达，应返回 nil：got %v", got)
		}
	})

	t.Run("孤立节点与不存在节点", func(t *testing.T) {
		g := NewStateGraph()
		iso := mustAdd(t, g, KindHypothesis, "孤立", 0)
		if got := g.PathTo(iso); !reflect.DeepEqual(got, []string{iso}) {
			t.Errorf("孤立节点自身即根：got %v", got)
		}
		if got := g.PathTo("ghost"); got != nil {
			t.Errorf("不存在节点应返回 nil：got %v", got)
		}
	})

	t.Run("多根确定性：相同图必得相同路径", func(t *testing.T) {
		type ids struct{ r1, r2, d string }
		build := func() (*StateGraph, ids) {
			g := NewStateGraph()
			r1 := mustAdd(t, g, KindTask, "根1", 0)
			r2 := mustAdd(t, g, KindTask, "根2", 0)
			d := mustAdd(t, g, KindObservation, "目标", 0)
			_ = g.AddEdge(r1, d, EdgeContext)
			_ = g.AddEdge(r2, d, EdgeContext)
			return g, ids{r1, r2, d}
		}
		g1, i1 := build()
		g2, i2 := build()
		if !reflect.DeepEqual(i1, i2) {
			t.Fatalf("确定性 ID 被破坏：%v vs %v", i1, i2)
		}
		p1, p2 := g1.PathTo(i1.d), g2.PathTo(i2.d)
		if !reflect.DeepEqual(p1, p2) {
			t.Errorf("相同图两次构建 PathTo 应一致：%v vs %v", p1, p2)
		}
		if len(p1) != 2 {
			t.Fatalf("BFS 最短路径长度应为 2：got %v", p1)
		}
		if p1[0] != i1.r1 && p1[0] != i1.r2 {
			t.Errorf("路径起点应为某个根：got %v（roots %q,%q）", p1, i1.r1, i1.r2)
		}
	})
}

func TestStateGraphConcurrentAccess(t *testing.T) {
	g := NewStateGraph()
	var mu sync.Mutex
	ids := make([]string, 0, 16)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id, _ := g.AddNode(KindObservation, fmt.Sprintf("观察 %d", i), i)
			mu.Lock()
			ids = append(ids, id)
			mu.Unlock()
			_, _ = g.Node(id)
			_ = g.Nodes()
			_ = g.Ancestors(id)
			_ = g.PathTo(id)
		}(i)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, n := range g.Nodes() {
				_ = g.AddEdge(n.ID, n.ID, EdgeCause) // 并发自环写
			}
		}()
	}
	wg.Wait()
	if len(ids) != 16 {
		t.Errorf("并发加节点应全部成功：got %d want 16", len(ids))
	}
	if got := len(g.Nodes()); got != 16 {
		t.Errorf("去重后节点数应为 16：got %d", got)
	}
}

func TestZeroValueUsable(t *testing.T) {
	g := &StateGraph{} // 零值图可用：懒初始化
	id, created := g.AddNode(KindTask, "零值", 0)
	if !created || id == "" {
		t.Fatalf("零值图应可直接 AddNode：id=%q created=%v", id, created)
	}
	tr := &WorldModelTracker{} // 零值 tracker 可用：懒初始化
	tr.Apply(ToolObserved{Turn: 0, ToolName: "ls", ToolInput: ".", Observation: "o"})
	if got := len(tr.Graph().Nodes()); got != 2 {
		t.Errorf("零值 tracker 应可直接 Apply：got %d 节点", got)
	}
	tr.Apply(PlanRevised{Turn: 0, Task: "T", Goal: "G", Steps: []PlanStep{{Summary: "s"}}})
	if _, ok := tr.CurrentPlan(); !ok {
		t.Error("零值 tracker 修订计划后应有当前计划")
	}
}
