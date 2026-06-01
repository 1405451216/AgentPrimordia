package orchestration

import (
	"fmt"
	"strings"
)

type Visualizer struct{}

func NewVisualizer() *Visualizer {
	return &Visualizer{}
}

func (v *Visualizer) ToMermaid(wf *WorkflowExecution) string {
	var sb strings.Builder
	sb.WriteString("flowchart TD\n")

	wf.mu.RLock()
	defer wf.mu.RUnlock()

	for id, node := range wf.nodes {
		shape := v.nodeShape(node.Type)
		label := escapeMermaid(node.Name)
		safeID := mermaidSafeID(id)
		sb.WriteString(fmt.Sprintf("    %s%s\"%s\"%s\n", safeID, shape.prefix, label, shape.suffix))
	}

	for fromID, transitions := range wf.transitions {
		for _, tr := range transitions {
			label := ""
			if tr.Condition != nil && tr.Condition.Type != "always" {
				label = escapeMermaid(tr.Condition.Expression)
				if label == "" && tr.Condition.Field != "" {
					label = fmt.Sprintf("%s %s %v", tr.Condition.Field, tr.Condition.Operator, tr.Condition.Value)
				}
			}
			safeFrom := mermaidSafeID(fromID)
			safeTo := mermaidSafeID(tr.To)
			if label != "" {
				sb.WriteString(fmt.Sprintf("    %s -->|%s| %s\n", safeFrom, label, safeTo))
			} else {
				sb.WriteString(fmt.Sprintf("    %s --> %s\n", safeFrom, safeTo))
			}
		}
	}

	if wf.startNodeID != "" {
		sb.WriteString(fmt.Sprintf("    START((start)) --> %s\n", mermaidSafeID(wf.startNodeID)))
	}

	for _, endID := range wf.endNodeIDs {
		sb.WriteString(fmt.Sprintf("    %s --> END((end))\n", mermaidSafeID(endID)))
	}

	return sb.String()
}

func (v *Visualizer) ToMermaidWithStatus(wf *WorkflowExecution) string {
	var sb strings.Builder
	sb.WriteString("flowchart TD\n")

	wf.mu.RLock()
	defer wf.mu.RUnlock()

	statusStyles := map[NodeExecutionStatus]string{
		NodePending:   "fill:#f9f9f9,stroke:#999",
		NodeRunning:   "fill:#fff3cd,stroke:#ffc107",
		NodeCompleted: "fill:#d4edda,stroke:#28a745",
		NodeSkipped:   "fill:#e2e3e5,stroke:#6c757d",
		NodeFailed:    "fill:#f8d7da,stroke:#dc3545",
	}

	nodeStatuses := make(map[string]NodeExecutionStatus)
	for _, rec := range wf.history {
		nodeStatuses[rec.NodeID] = rec.Status
	}

	for id, node := range wf.nodes {
		shape := v.nodeShape(node.Type)
		label := escapeMermaid(node.Name)
		safeID := mermaidSafeID(id)
		sb.WriteString(fmt.Sprintf("    %s%s\"%s\"%s\n", safeID, shape.prefix, label, shape.suffix))

		if status, ok := nodeStatuses[id]; ok {
			if style, hasStyle := statusStyles[status]; hasStyle {
				sb.WriteString(fmt.Sprintf("    style %s %s\n", safeID, style))
			}
		}
	}

	for fromID, transitions := range wf.transitions {
		for _, tr := range transitions {
			label := ""
			if tr.Condition != nil && tr.Condition.Type != "always" {
				label = escapeMermaid(tr.Condition.Expression)
			}
			safeFrom := mermaidSafeID(fromID)
			safeTo := mermaidSafeID(tr.To)
			if label != "" {
				sb.WriteString(fmt.Sprintf("    %s -->|%s| %s\n", safeFrom, label, safeTo))
			} else {
				sb.WriteString(fmt.Sprintf("    %s --> %s\n", safeFrom, safeTo))
			}
		}
	}

	if wf.startNodeID != "" {
		sb.WriteString(fmt.Sprintf("    START((start)) --> %s\n", mermaidSafeID(wf.startNodeID)))
	}
	for _, endID := range wf.endNodeIDs {
		sb.WriteString(fmt.Sprintf("    %s --> END((end))\n", mermaidSafeID(endID)))
	}

	return sb.String()
}

func (v *Visualizer) ToPlantUML(wf *WorkflowExecution) string {
	var sb strings.Builder
	sb.WriteString("@startuml\n")
	sb.WriteString("skinparam backgroundColor #FEFEFE\n")
	sb.WriteString("skinparam activity {\n  BackgroundColor #F9F9F9\n  BorderColor #333333\n}\n")

	wf.mu.RLock()
	defer wf.mu.RUnlock()

	sb.WriteString("start\n")

	if wf.startNodeID != "" {
		v.renderPlantUMLNode(&sb, wf, wf.startNodeID, make(map[string]bool))
	}

	sb.WriteString("stop\n")
	sb.WriteString("@enduml\n")
	return sb.String()
}

func (v *Visualizer) renderPlantUMLNode(sb *strings.Builder, wf *WorkflowExecution, nodeID string, visited map[string]bool) {
	if visited[nodeID] {
		return
	}
	visited[nodeID] = true

	node, ok := wf.nodes[nodeID]
	if !ok {
		return
	}

	switch node.Type {
	case ConditionNode:
		sb.WriteString(fmt.Sprintf("if (%s) then\n", escapePlantUML(node.Name)))
		transitions := wf.transitions[nodeID]
		thenBranch := ""
		elseBranch := ""
		for _, tr := range transitions {
			if tr.Condition != nil && tr.Condition.Type != "always" {
				thenBranch = tr.To
				sb.WriteString(fmt.Sprintf("  ->%s\n", tr.To))
			} else {
				elseBranch = tr.To
			}
		}
		if thenBranch != "" {
			v.renderPlantUMLNode(sb, wf, thenBranch, visited)
		}
		if elseBranch != "" {
			sb.WriteString("else\n")
			sb.WriteString(fmt.Sprintf("  ->%s\n", elseBranch))
			v.renderPlantUMLNode(sb, wf, elseBranch, visited)
		}
		sb.WriteString("endif\n")

	case LoopStartNode:
		sb.WriteString(fmt.Sprintf("repeat\n"))
		sb.WriteString(fmt.Sprintf("  :%s;\n", escapePlantUML(node.Name)))
		transitions := wf.transitions[nodeID]
		for _, tr := range transitions {
			v.renderPlantUMLNode(sb, wf, tr.To, visited)
		}
		sb.WriteString("repeat while (continue?)\n")

	case ParallelNode:
		sb.WriteString(fmt.Sprintf("fork\n"))
		transitions := wf.transitions[nodeID]
		for i, tr := range transitions {
			if i > 0 {
				sb.WriteString("fork again\n")
			}
			v.renderPlantUMLNode(sb, wf, tr.To, visited)
		}
		sb.WriteString("end fork\n")

	default:
		sb.WriteString(fmt.Sprintf(":%s;\n", escapePlantUML(node.Name)))
		transitions := wf.transitions[nodeID]
		for _, tr := range transitions {
			v.renderPlantUMLNode(sb, wf, tr.To, visited)
		}
	}
}

func (v *Visualizer) ToDot(wf *WorkflowExecution) string {
	var sb strings.Builder
	sb.WriteString("digraph workflow {\n")
	sb.WriteString("    rankdir=TD;\n")
	sb.WriteString("    node [shape=box, style=rounded];\n")

	wf.mu.RLock()
	defer wf.mu.RUnlock()

	for id, node := range wf.nodes {
		shape := "box"
		switch node.Type {
		case ConditionNode:
			shape = "diamond"
		case ParallelNode:
			shape = "hexagon"
		case LoopStartNode, LoopEndNode:
			shape = "ellipse"
		}
		sb.WriteString(fmt.Sprintf("    %s [label=\"%s\", shape=%s];\n", id, escapeDot(node.Name), shape))
	}

	if wf.startNodeID != "" {
		sb.WriteString("    START [label=\"Start\", shape=circle];\n")
		sb.WriteString(fmt.Sprintf("    START -> %s;\n", wf.startNodeID))
	}

	for _, endID := range wf.endNodeIDs {
		sb.WriteString(fmt.Sprintf("    %s -> END;\n", endID))
	}
	sb.WriteString("    END [label=\"End\", shape=circle];\n")

	for fromID, transitions := range wf.transitions {
		for _, tr := range transitions {
			label := ""
			if tr.Condition != nil && tr.Condition.Type != "always" {
				label = escapeDot(tr.Condition.Expression)
			}
			if label != "" {
				sb.WriteString(fmt.Sprintf("    %s -> %s [label=\"%s\"];\n", fromID, tr.To, label))
			} else {
				sb.WriteString(fmt.Sprintf("    %s -> %s;\n", fromID, tr.To))
			}
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

type nodeShapeInfo struct {
	prefix string
	suffix string
}

func (v *Visualizer) nodeShape(nodeType NodeType) nodeShapeInfo {
	switch nodeType {
	case ConditionNode:
		return nodeShapeInfo{"{", "}"}
	case ParallelNode:
		return nodeShapeInfo{"([", "])"}
	case LoopStartNode, LoopEndNode:
		return nodeShapeInfo{"(((", ")))"}
	case FallbackNode:
		return nodeShapeInfo{"[[", "]]"}
	default:
		return nodeShapeInfo{"[", "]"}
	}
}

func escapeMermaid(s string) string {
	s = strings.ReplaceAll(s, `"`, "'")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

var mermaidReserved = map[string]bool{
	"end": true, "graph": true, "flowchart": true, "subgraph": true,
	"style": true, "classDef": true, "click": true,
}

func mermaidSafeID(id string) string {
	if mermaidReserved[id] {
		return "n_" + id
	}
	return id
}

func escapePlantUML(s string) string {
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.ReplaceAll(s, ";", "-")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func escapeDot(s string) string {
	s = strings.ReplaceAll(s, `"`, "'")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "{", "\\{")
	s = strings.ReplaceAll(s, "}", "\\}")
	return s
}
