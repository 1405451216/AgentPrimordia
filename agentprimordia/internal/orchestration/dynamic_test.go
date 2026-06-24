package orchestration

import (
	"context"
	"testing"
)

func TestDynamicDAG_AddNodeAtRuntime(t *testing.T) {
	dag := NewDynamicDAG("dynamic-wf")

	dag.AddNode(DynamicNodeHandler{ID: "step-1", Handler: func(ctx context.Context, input any) (any, error) {
		return "result-1", nil
	}})

	// 运行时添加节点
	dag.AddNode(DynamicNodeHandler{ID: "step-2", Handler: func(ctx context.Context, input any) (any, error) {
		return "result-2", nil
	}})
	dag.AddEdge("step-1", "step-2")

	if dag.NodeCount() != 2 {
		t.Errorf("节点数 = %d, 期望 2", dag.NodeCount())
	}
}

func TestDynamicDAG_RemoveNodeAtRuntime(t *testing.T) {
	dag := NewDynamicDAG("dynamic-wf")

	dag.AddNode(DynamicNodeHandler{ID: "step-1", Handler: noopHandler})
	dag.AddNode(DynamicNodeHandler{ID: "step-2", Handler: noopHandler})
	dag.AddEdge("step-1", "step-2")

	// 运行时移除节点
	dag.RemoveNode("step-2")

	if dag.NodeCount() != 1 {
		t.Errorf("移除后节点数 = %d, 期望 1", dag.NodeCount())
	}
}

func TestDynamicDAG_ConditionalRouting(t *testing.T) {
	dag := NewDynamicDAG("router-wf")

	executed := ""
	dag.AddNode(DynamicNodeHandler{ID: "router", Handler: func(ctx context.Context, input any) (any, error) {
		return "go-b", nil
	}})
	dag.AddNode(DynamicNodeHandler{ID: "branch-a", Handler: func(ctx context.Context, input any) (any, error) {
		executed = "a"
		return nil, nil
	}})
	dag.AddNode(DynamicNodeHandler{ID: "branch-b", Handler: func(ctx context.Context, input any) (any, error) {
		executed = "b"
		return nil, nil
	}})

	dag.AddConditionalEdge("router", map[string]string{
		"go-a": "branch-a",
		"go-b": "branch-b",
	})

	_, err := dag.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	if executed != "b" {
		t.Errorf("应执行 branch-b, 实际执行 = %q", executed)
	}
}

func noopHandler(ctx context.Context, input any) (any, error) {
	return nil, nil
}
