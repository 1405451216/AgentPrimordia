package orchestration

import "testing"

func TestBuildExecutionPlan_Sequential(t *testing.T) {
	steps := []*AgentStep{
		{ID: "s1", Name: "step1"},
		{ID: "s2", Name: "step2"},
		{ID: "s3", Name: "step3"},
	}
	plan, err := BuildExecutionPlan(SequentialMode, steps, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Mode != SequentialMode {
		t.Errorf("mode mismatch")
	}
	if !plan.DepGraph.Ready("s1") {
		t.Errorf("s1 should be ready")
	}
	if plan.DepGraph.Ready("s2") || plan.DepGraph.Ready("s3") {
		t.Errorf("s2/s3 should not be ready initially")
	}
}

func TestBuildExecutionPlan_Parallel(t *testing.T) {
	steps := []*AgentStep{
		{ID: "p1"},
		{ID: "p2"},
	}
	plan, err := BuildExecutionPlan(ParallelMode, steps, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !plan.DepGraph.Ready("p1") || !plan.DepGraph.Ready("p2") {
		t.Errorf("all parallel steps should be ready")
	}
}

func TestBuildExecutionPlan_DAG(t *testing.T) {
	steps := []*AgentStep{
		{ID: "a"},
		{ID: "b"},
		{ID: "c"},
	}
	edges := []DAGEdge{{From: "a", To: "c"}, {From: "b", To: "c"}}
	plan, err := BuildExecutionPlan(DAGMode, steps, edges)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !plan.DepGraph.Ready("a") || !plan.DepGraph.Ready("b") {
		t.Errorf("a and b should be ready")
	}
	if plan.DepGraph.Ready("c") {
		t.Errorf("c should wait for a and b")
	}
}
