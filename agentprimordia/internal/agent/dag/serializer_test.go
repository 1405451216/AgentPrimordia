package dag

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// noopHandler 为测试用空操作节点处理函数。
var noopHandler NodeHandler = func(_ context.Context, input string) (string, error) {
	return input, nil
}

// TestSerializeDAG_Basic 验证基本 DAG 序列化。
func TestSerializeDAG_Basic(t *testing.T) {
	builder := NewDAGBuilder("test-workflow")
	builder.Node("step1", noopHandler)
	builder.Node("step2", noopHandler)
	builder.Edge("step1", "step2")

	workflow, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	dagJSON, err := SerializeDAG(workflow)
	if err != nil {
		t.Fatalf("SerializeDAG failed: %v", err)
	}

	if dagJSON.Version != DAGJSONVersion {
		t.Errorf("Version = %q, want %q", dagJSON.Version, DAGJSONVersion)
	}
	if dagJSON.Name != "test-workflow" {
		t.Errorf("Name = %q, want %q", dagJSON.Name, "test-workflow")
	}
	if len(dagJSON.Nodes) != 2 {
		t.Errorf("Nodes len = %d, want 2", len(dagJSON.Nodes))
	}
	if len(dagJSON.Edges) != 1 {
		t.Errorf("Edges len = %d, want 1", len(dagJSON.Edges))
	}
}

// TestSerializeDAG_Deterministic 验证输出是确定性的（节点按字典序排列）。
func TestSerializeDAG_Deterministic(t *testing.T) {
	builder := NewDAGBuilder("det")
	builder.Node("c", noopHandler)
	builder.Node("a", noopHandler)
	builder.Node("b", noopHandler)
	builder.Edge("a", "b")
	builder.Edge("b", "c")
	workflow := builder.MustBuild()

	dagJSON, err := SerializeDAG(workflow)
	if err != nil {
		t.Fatalf("SerializeDAG failed: %v", err)
	}

	// 节点应该按 ID 字典序排列
	if dagJSON.Nodes[0].ID != "a" || dagJSON.Nodes[1].ID != "b" || dagJSON.Nodes[2].ID != "c" {
		t.Errorf("Nodes not sorted: got %v", nodeIDs(dagJSON.Nodes))
	}
}

// TestSerializeDAG_WithMetadata 验证节点 metadata 被序列化到 config 中。
func TestSerializeDAG_WithMetadata(t *testing.T) {
	builder := NewDAGBuilder("meta")
	builder.Node("n1", noopHandler).Label("First Step").Metadata("priority", "high")
	workflow := builder.MustBuild()

	dagJSON, err := SerializeDAG(workflow)
	if err != nil {
		t.Fatalf("SerializeDAG failed: %v", err)
	}

	if len(dagJSON.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(dagJSON.Nodes))
	}
	node := dagJSON.Nodes[0]
	if node.Config["label"] != "First Step" {
		t.Errorf("Config[label] = %v, want First Step", node.Config["label"])
	}
	if node.Config["priority"] != "high" {
		t.Errorf("Config[priority] = %v, want high", node.Config["priority"])
	}
}

// TestSerializeDAG_EdgeCondition 验证条件边标记。
func TestSerializeDAG_EdgeCondition(t *testing.T) {
	builder := NewDAGBuilder("cond")
	builder.Node("a", noopHandler)
	builder.Node("b", noopHandler)
	builder.Node("c", noopHandler)
	builder.EdgeWithCondition("a", "b", func(_ context.Context, _ *DAGNodeResult) bool {
		return true
	})
	builder.Edge("a", "c")
	workflow := builder.MustBuild()

	dagJSON, err := SerializeDAG(workflow)
	if err != nil {
		t.Fatalf("SerializeDAG failed: %v", err)
	}

	foundCond := false
	for _, e := range dagJSON.Edges {
		if e.From == "a" && e.To == "b" && e.Condition {
			foundCond = true
		}
		if e.From == "a" && e.To == "c" && e.Condition {
			t.Error("edge a→c should not have condition=true")
		}
	}
	if !foundCond {
		t.Error("expected conditional edge a→b")
	}
}

// TestDeserializeDAG_Basic 验证基本 DAG 反序列化。
func TestDeserializeDAG_Basic(t *testing.T) {
	dagJSON := &DAGJSON{
		Version: DAGJSONVersion,
		Name:    "roundtrip",
		Nodes: []DAGNodeJSON{
			{ID: "x", Type: NodeTypeAgent, Config: map[string]any{"label": "X"}},
			{ID: "y", Type: NodeTypeAgent, Config: map[string]any{"label": "Y"}},
		},
		Edges: []DAGEdgeJSON{
			{From: "x", To: "y"},
		},
	}

	workflow, err := DeserializeDAG(dagJSON)
	if err != nil {
		t.Fatalf("DeserializeDAG failed: %v", err)
	}

	if workflow.NodeCount() != 2 {
		t.Errorf("NodeCount = %d, want 2", workflow.NodeCount())
	}
	if workflow.EdgeCount() != 1 {
		t.Errorf("EdgeCount = %d, want 1", workflow.EdgeCount())
	}
}

// TestDeserializeDAG_RoundTrip 验证序列化→反序列化往返。
func TestDeserializeDAG_RoundTrip(t *testing.T) {
	builder := NewDAGBuilder("rt")
	builder.Node("a", noopHandler).Label("Step A")
	builder.Node("b", noopHandler).Label("Step B")
	builder.Node("c", noopHandler).Label("Step C")
	builder.Edge("a", "b")
	builder.Edge("b", "c")
	workflow := builder.MustBuild()

	dagJSON, err := SerializeDAG(workflow)
	if err != nil {
		t.Fatalf("SerializeDAG failed: %v", err)
	}

	restored, err := DeserializeDAG(dagJSON)
	if err != nil {
		t.Fatalf("DeserializeDAG failed: %v", err)
	}

	if restored.NodeCount() != 3 {
		t.Errorf("Restored NodeCount = %d, want 3", restored.NodeCount())
	}
	if restored.EdgeCount() != 2 {
		t.Errorf("Restored EdgeCount = %d, want 2", restored.EdgeCount())
	}
}

// TestDeserializeDAGFromJSON 验证从 JSON 字节反序列化。
func TestDeserializeDAGFromJSON(t *testing.T) {
	jsonStr := `{
		"version": "1.0",
		"name": "from-json",
		"nodes": [
			{"id": "n1", "type": "agent", "config": {}, "inputs": [], "outputs": ["n2"]},
			{"id": "n2", "type": "agent", "config": {}, "inputs": [], "outputs": []}
		],
		"edges": [
			{"from": "n1", "to": "n2", "condition": false}
		]
	}`

	workflow, err := DeserializeDAGFromJSON([]byte(jsonStr))
	if err != nil {
		t.Fatalf("DeserializeDAGFromJSON failed: %v", err)
	}

	if workflow.NodeCount() != 2 {
		t.Errorf("NodeCount = %d, want 2", workflow.NodeCount())
	}
}

// TestSerializeDAG_NilWorkflow 验证 nil 输入返回错误。
func TestSerializeDAG_NilWorkflow(t *testing.T) {
	_, err := SerializeDAG(nil)
	if err == nil {
		t.Error("expected error for nil workflow")
	}
}

// TestSerializeDAGToJSON 验证 JSON 字节输出。
func TestSerializeDAGToJSON(t *testing.T) {
	builder := NewDAGBuilder("json-out")
	builder.Node("a", noopHandler)
	workflow := builder.MustBuild()

	data, err := SerializeDAGToJSON(workflow)
	if err != nil {
		t.Fatalf("SerializeDAGToJSON failed: %v", err)
	}

	// 验证是合法 JSON
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
	if generic["name"] != "json-out" {
		t.Errorf("name in JSON = %v, want json-out", generic["name"])
	}
}

// TestDAGJSONCrossLanguage 验证生成的 JSON 能被标准解析器消费。
// 确保字段命名与 TS 端一致（snake_case）。
func TestDAGJSONCrossLanguage(t *testing.T) {
	builder := NewDAGBuilder("cross")
	builder.Node("search", noopHandler).Label("Search")
	builder.Node("summarize", noopHandler).Label("Summarize")
	builder.Edge("search", "summarize")
	workflow := builder.MustBuild()

	data, err := SerializeDAGToJSON(workflow)
	if err != nil {
		t.Fatalf("SerializeDAGToJSON failed: %v", err)
	}

	jsonStr := string(data)

	// 验证字段名是 snake_case（与 TS 一致）
	if !strings.Contains(jsonStr, `"depends_on"`) {
		t.Errorf("JSON should use snake_case 'depends_on', got: %s", jsonStr)
	}
	// 验证不含 camelCase 版本
	if strings.Contains(jsonStr, `"dependsOn"`) {
		t.Errorf("JSON should not use camelCase 'dependsOn', got: %s", jsonStr)
	}
}

func nodeIDs(nodes []DAGNodeJSON) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return ids
}
