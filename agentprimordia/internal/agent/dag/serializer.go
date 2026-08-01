package dag

import (
	"encoding/json"
	"fmt"
	"sort"
)

// ===== DAG JSON 互通协议 =====
//
// 本文件定义 DAGWorkflow 与 JSON 格式之间的双向转换。
// JSON 格式与 TS 端 DAGWorkflow 互通，字段命名遵循 camelCase → snake_case 约定。
//
// 结构示例:
//   {
//     "version": "1.0",
//     "name": "research-workflow",
//     "nodes": [
//       {
//         "id": "search",
//         "type": "agent",
//         "config": {"label": "Web Search"},
//         "inputs": [],
//         "outputs": ["extract"],
//         "depends_on": []
//       }
//     ],
//     "edges": [
//       {"from": "search", "to": "extract"}
//     ],
//     "metadata": {"author": "ap"}
//   }

// DAGJSON 为 DAG 的 JSON 兼容表示。
type DAGJSON struct {
	Version  string            `json:"version"`
	Name     string            `json:"name"`
	Nodes    []DAGNodeJSON     `json:"nodes"`
	Edges    []DAGEdgeJSON     `json:"edges"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// DAGNodeJSON 为节点的 JSON 表示。
type DAGNodeJSON struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"` // "agent" / "tool" / "condition"
	Config    map[string]any `json:"config"`
	Inputs    []string       `json:"inputs"`
	Outputs   []string       `json:"outputs"`
	DependsOn []string       `json:"depends_on,omitempty"`
}

// DAGEdgeJSON 为边的 JSON 表示。
type DAGEdgeJSON struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Label     string `json:"label,omitempty"`
	Condition bool   `json:"condition"` // true 表示有条件跳转
}

// 节点类型常量。
const (
	NodeTypeAgent     = "agent"
	NodeTypeTool      = "tool"
	NodeTypeCondition = "condition"

	// DAGJSONVersion 当前 DAG JSON 协议版本。
	DAGJSONVersion = "1.0"
)

// ===== 序列化 =====

// SerializeDAG 将 DAGWorkflow 序列化为 DAGJSON。
func SerializeDAG(workflow *DAGWorkflow) (*DAGJSON, error) {
	if workflow == nil {
		return nil, fmt.Errorf("dag: cannot serialize nil workflow")
	}

	workflow.mu.RLock()
	defer workflow.mu.RUnlock()

	result := &DAGJSON{
		Version:  DAGJSONVersion,
		Name:     workflow.name,
		Nodes:    make([]DAGNodeJSON, 0, len(workflow.nodes)),
		Edges:    make([]DAGEdgeJSON, 0, len(workflow.edges)),
		Metadata: make(map[string]string),
	}

	// 收集节点 ID 用于排序输出（确定性）
	nodeIDs := make([]string, 0, len(workflow.nodes))
	for id := range workflow.nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)

	// 构建 from→to 的邻接映射：节点的 outputs 是所有 from 为此节点的边的 to
	outgoingMap := make(map[string][]string)
	for _, edge := range workflow.edges {
		outgoingMap[edge.From] = append(outgoingMap[edge.From], edge.To)
	}
	// 构建入边映射：节点的 depends_on 是所有 to 为此节点的边的 from
	incomingMap := make(map[string][]string)
	for _, edge := range workflow.edges {
		incomingMap[edge.To] = append(incomingMap[edge.To], edge.From)
	}

	for _, id := range nodeIDs {
		node := workflow.nodes[id]

		// 排除内部 metadata 标签，复制其余 metadata
		config := make(map[string]any)
		for k, v := range node.Metadata {
			config[k] = v
		}
		// 把节点 ID 也放入 config 便于比对
		if _, ok := config["_node_id"]; !ok {
			config["_node_id"] = id
		}

		dependsOn := incomingMap[id]
		sort.Strings(dependsOn)
		outputs := outgoingMap[id]
		sort.Strings(outputs)

		nodeJSON := DAGNodeJSON{
			ID:        id,
			Type:      NodeTypeAgent, // DAG 节点默认类型为 agent
			Config:    config,
			Inputs:    []string{}, // inputs 是运行时字段，序列化时保留空列表
			Outputs:   outputs,
			DependsOn: dependsOn,
		}
		result.Nodes = append(result.Nodes, nodeJSON)
	}

	for _, edge := range workflow.edges {
		edgeJSON := DAGEdgeJSON{
			From:      edge.From,
			To:        edge.To,
			Label:     edge.Label,
			Condition: edge.Condition != nil,
		}
		result.Edges = append(result.Edges, edgeJSON)
	}

	return result, nil
}

// SerializeDAGToJSON 将 DAGWorkflow 序列化为 JSON 字节。
func SerializeDAGToJSON(workflow *DAGWorkflow) ([]byte, error) {
	dagJSON, err := SerializeDAG(workflow)
	if err != nil {
		return nil, err
	}
	return json.Marshal(dagJSON)
}

// ===== 反序列化 =====

// DeserializeDAG 从 DAGJSON 重建 DAGWorkflow。
//
// 注意：DAGNode 中的 Agent 字段无法从 JSON 恢复（Agent 是接口），
// 所以反序列化后的节点 Agent 为 nil。调用方需要自行注入 Agent 实现。
func DeserializeDAG(dagJSON *DAGJSON) (*DAGWorkflow, error) {
	if dagJSON == nil {
		return nil, fmt.Errorf("dag: cannot deserialize nil JSON")
	}

	workflow := NewDAGWorkflow().WithName(dagJSON.Name)

	for _, nodeJSON := range dagJSON.Nodes {
		if nodeJSON.ID == "" {
			return nil, fmt.Errorf("dag: node ID cannot be empty")
		}
		metadata := make(map[string]string)
		for k, v := range nodeJSON.Config {
			if s, ok := v.(string); ok {
				metadata[k] = s
			}
		}
		node := &DAGNode{
			ID:       nodeJSON.ID,
			Metadata: metadata,
		}
		if err := workflow.AddNode(node); err != nil {
			return nil, fmt.Errorf("dag: add node %q: %w", nodeJSON.ID, err)
		}
	}

	for _, edgeJSON := range dagJSON.Edges {
		edge := DAGEdge{
			From:  edgeJSON.From,
			To:    edgeJSON.To,
			Label: edgeJSON.Label,
		}
		if err := workflow.AddEdge(edge); err != nil {
			return nil, fmt.Errorf("dag: add edge %s→%s: %w", edgeJSON.From, edgeJSON.To, err)
		}
	}

	return workflow, nil
}

// DeserializeDAGFromJSON 从 JSON 字节反序列化 DAGWorkflow。
func DeserializeDAGFromJSON(data []byte) (*DAGWorkflow, error) {
	var dagJSON DAGJSON
	if err := json.Unmarshal(data, &dagJSON); err != nil {
		return nil, fmt.Errorf("dag: unmarshal JSON: %w", err)
	}
	return DeserializeDAG(&dagJSON)
}

// ===== 跨语言兼容tool =====

// CanonicalDAGJSON 返回紧凑、确定性的 JSON 字节。
// 用于跨语言比对测试。
func CanonicalDAGJSON(workflow *DAGWorkflow) ([]byte, error) {
	dagJSON, err := SerializeDAG(workflow)
	if err != nil {
		return nil, err
	}
	// 使用标准 json.Marshal，然后重新 Unmarshal 到 map 再 Marshal
	// 确保 JSON 格式稳定
	var generic map[string]any
	if err := json.Unmarshal(mustMarshal(dagJSON), &generic); err != nil {
		return nil, err
	}
	return json.Marshal(generic)
}

func mustMarshal(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}
