package agent

import (
	"context"
	"errors"
	"testing"
)

func TestDAGBuilder_Basic(t *testing.T) {
	dag, err := NewDAGBuilder("test").
		Node("a", func(ctx context.Context, input string) (string, error) {
			return "a-out", nil
		}).
		Node("b", func(ctx context.Context, input string) (string, error) {
			return "b-out", nil
		}).
		Edge("a", "b").
		Build()

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if dag.NodeCount() != 2 {
		t.Errorf("NodeCount = %d, want 2", dag.NodeCount())
	}
	if dag.EdgeCount() != 1 {
		t.Errorf("EdgeCount = %d, want 1", dag.EdgeCount())
	}
}

func TestDAGBuilder_Sequential(t *testing.T) {
	dag, err := NewDAGBuilder("seq-test").
		Sequential(
			MakeNode("step1", func(ctx context.Context, input string) (string, error) { return "s1", nil }),
			MakeNode("step2", func(ctx context.Context, input string) (string, error) { return "s2", nil }),
			MakeNode("step3", func(ctx context.Context, input string) (string, error) { return "s3", nil }),
		).
		Build()

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if dag.NodeCount() != 3 {
		t.Errorf("NodeCount = %d, want 3", dag.NodeCount())
	}
	order, _ := dag.TopologicalSort()
	if len(order) != 3 {
		t.Fatal("topological sort failed")
	}
}

func TestDAGBuilder_Parallel(t *testing.T) {
	dag, err := NewDAGBuilder("par-test").
		Parallel(
			"split", func(ctx context.Context, input string) (string, error) { return "split", nil },
			"merge", func(ctx context.Context, input string) (string, error) { return "merged", nil },
			MakeNode("task_a", func(ctx context.Context, input string) (string, error) { return "a", nil }),
			MakeNode("task_b", func(ctx context.Context, input string) (string, error) { return "b", nil }),
		).
		Build()

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if dag.NodeCount() != 4 {
		t.Errorf("NodeCount = %d, want 4", dag.NodeCount())
	}
}

func TestDAGBuilder_Conditional(t *testing.T) {
	dag, err := NewDAGBuilder("cond-test").
		Node("check", func(ctx context.Context, input string) (string, error) {
			return input, nil
		}).
		Node("yes", func(ctx context.Context, input string) (string, error) {
			return "yes-branch", nil
		}).
		Node("no", func(ctx context.Context, input string) (string, error) {
			return "no-branch", nil
		}).
		Conditional("check", "yes", "no", ConditionOnOutput("approve")).
		Build()

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if dag.EdgeCount() != 2 {
		t.Errorf("EdgeCount = %d, want 2", dag.EdgeCount())
	}
}

func TestDAGBuilder_LabelAndMetadata(t *testing.T) {
	dag, _ := NewDAGBuilder("meta-test").
		Node("n1", func(ctx context.Context, input string) (string, error) {
			return "ok", nil
		}).Label("First Node").Metadata("type", "processor", "priority", "high").
		Build()

	if dag == nil {
		t.Fatal("dag is nil")
	}
	dag.mu.RLock()
	node := dag.nodes["n1"]
	dag.mu.RUnlock()
	if node == nil {
		t.Fatal("node n1 not found")
	}
	if node.Metadata["label"] != "First Node" {
		t.Errorf("label = %q, want First Node", node.Metadata["label"])
	}
	if node.Metadata["type"] != "processor" {
		t.Errorf("type = %q, want processor", node.Metadata["type"])
	}
}

func TestDAGBuilder_WithRetry(t *testing.T) {
	attempt := 0
	dag, _ := NewDAGBuilder("retry-test").
		Node("flaky", func(ctx context.Context, input string) (string, error) {
			attempt++
			if attempt < 3 {
				return "", errors.New("transient error")
			}
			return "success after retries", nil
		}).WithRetry(3, 10).
		Build()

	result, err := dag.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	flakyResult := result.NodeResults["flaky"]
	if flakyResult == nil {
		t.Fatal("flaky result is nil")
	}
	if flakyResult.Error != nil {
		t.Errorf("unexpected error: %v", flakyResult.Error)
	}
	if flakyResult.Retries < 2 {
		t.Errorf("Retries = %d, want >= 2", flakyResult.Retries)
	}
}

func TestDAGBuilder_FanOutFanIn(t *testing.T) {
	dag, err := NewDAGBuilder("fan-test").
		Node("source", func(ctx context.Context, input string) (string, error) { return "src", nil }).
		Node("worker_a", func(ctx context.Context, input string) (string, error) { return "a", nil }).
		Node("worker_b", func(ctx context.Context, input string) (string, error) { return "b", nil }).
		Node("sink", func(ctx context.Context, input string) (string, error) { return "sink", nil }).
		FanOut("source", "worker_a", "worker_b").
		FanIn("sink", "worker_a", "worker_b").
		Build()

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if dag.EdgeCount() != 4 {
		t.Errorf("EdgeCount = %d, want 4", dag.EdgeCount())
	}
}

func TestDAGBuilder_LinkTo(t *testing.T) {
	dag, err := NewDAGBuilder("link-test").
		Node("first", func(ctx context.Context, input string) (string, error) { return "1", nil }).
		LinkTo("second").
		Node("second", func(ctx context.Context, input string) (string, error) { return "2", nil }).
		Build()

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	deps := dag.GetDependencies("second")
	if len(deps) != 1 || deps[0] != "first" {
		t.Errorf("deps of second = %v, want [first]", deps)
	}
}

func TestDAGBuilder_DuplicateNode(t *testing.T) {
	_, err := NewDAGBuilder("dup-test").
		Node("x", func(ctx context.Context, input string) (string, error) { return "x", nil }).
		Node("x", func(ctx context.Context, input string) (string, error) { return "x2", nil }).
		Build()

	if err == nil {
		t.Fatal("expected error for duplicate node")
	}
}

func TestDAGBuilder_EmptyNodeID(t *testing.T) {
	_, err := NewDAGBuilder("empty-id").
		Node("", func(ctx context.Context, input string) (string, error) { return "x", nil }).
		Build()

	if err == nil {
		t.Fatal("expected error for empty node ID")
	}
}

func TestDAGBuilder_LabeledEdge(t *testing.T) {
	dag, err := NewDAGBuilder("labeled-edge").
		Node("from", func(ctx context.Context, input string) (string, error) { return "f", nil }).
		Node("to", func(ctx context.Context, input string) (string, error) { return "t", nil }).
		LabeledEdge("from", "to", "data flow").
		Build()

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	dag.mu.RLock()
	edge := dag.edges[0]
	dag.mu.RUnlock()
	if edge.Label != "data flow" {
		t.Errorf("edge label = %q, want data flow", edge.Label)
	}
}

func TestDAGBuilder_MustBuild_Panic(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on MustBuild with error")
		}
	}()

	NewDAGBuilder("panic-test").
		Node("", func(ctx context.Context, input string) (string, error) { return "x", nil }).
		MustBuild()
}

func TestConditionOnOutput(t *testing.T) {
	cond := ConditionOnOutput("approve")
	result := &DAGNodeResult{Output: "request approved by manager"}
	if !cond(context.Background(), result) {
		t.Error("should match substring")
	}

	result2 := &DAGNodeResult{Output: "request rejected"}
	if cond(context.Background(), result2) {
		t.Error("should not match")
	}
}

func TestConditionOnError(t *testing.T) {
	cond := ConditionOnError()
	resultErr := &DAGNodeResult{Error: errors.New("something wrong")}
	if !cond(context.Background(), resultErr) {
		t.Error("should be true on error")
	}
	resultOk := &DAGNodeResult{}
	if cond(context.Background(), resultOk) {
		t.Error("should be false on success")
	}
}

func TestDAGBuilder_RunEndToEnd(t *testing.T) {
	dag, err := NewDAGBuilder("e2e").
		Node("echo", func(ctx context.Context, input string) (string, error) {
			return input, nil
		}).
		Node("prefix", func(ctx context.Context, input string) (string, error) {
			return "RESULT: " + input, nil
		}).
		Edge("echo", "prefix").
		Build()

	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	result, err := dag.Run(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Failed > 0 {
		t.Errorf("%d nodes failed", result.Failed)
	}
	if result.Succeeded != 2 {
		t.Errorf("Succeeded = %d, want 2", result.Succeeded)
	}

	prefixResult := result.NodeResults["prefix"]
	if prefixResult.Output != "RESULT: hello world" {
		t.Errorf("output = %q, want RESULT: hello world", prefixResult.Output)
	}
}
