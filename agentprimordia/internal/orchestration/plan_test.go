package orchestration

import "testing"

func TestDependencyGraph_ReadyAndComplete(t *testing.T) {
	steps := []*AgentStep{
		{ID: "a"},
		{ID: "b"},
		{ID: "c"},
	}
	edges := []DAGEdge{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
	}
	g, err := NewDependencyGraph(steps, edges)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !g.Ready("a") {
		t.Errorf("a should be ready")
	}
	if g.Ready("b") || g.Ready("c") {
		t.Errorf("b and c should not be ready yet")
	}

	ready := g.Complete("a")
	if len(ready) != 1 || ready[0] != "b" {
		t.Errorf("completing a should make b ready, got %v", ready)
	}
	if !g.Ready("b") {
		t.Errorf("b should now be ready")
	}
}
