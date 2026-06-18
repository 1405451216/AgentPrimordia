package agent

import (
	"fmt"
	"strings"
)

// VisualizeConfig 可视化配置
type VisualizeConfig struct {
	// Direction 图形方向："TD" (上到下), "LR" (左到右)
	Direction string
	// HighlightPath 需要高亮的执行路径节点 ID 集合
	HighlightPath []string
	// FailedNodes 失败节点 ID 集合
	FailedNodes []string
	// ShowLabels 是否在边上显示条件标签
	ShowLabels bool
}

// DefaultVisualizeConfig 返回默认可视化配置
func DefaultVisualizeConfig() VisualizeConfig {
	return VisualizeConfig{
		Direction:     "TD",
		HighlightPath: nil,
		FailedNodes:   nil,
		ShowLabels:    true,
	}
}

// ToMermaid 将工作流生成 Mermaid flowchart 字符串
func (w *WorkflowExecution) ToMermaid() string {
	return w.ToMermaidWithConfig(DefaultVisualizeConfig())
}

// ToMermaidWithConfig 使用指定配置生成 Mermaid flowchart
func (w *WorkflowExecution) ToMermaidWithConfig(cfg VisualizeConfig) string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	direction := cfg.Direction
	if direction == "" {
		direction = "TD"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("flowchart %s\n", direction))

	// 收集高亮和失败节点集合
	highlightSet := makeStringSet(cfg.HighlightPath)
	failedSet := makeStringSet(cfg.FailedNodes)

	// 生成节点定义
	for id, node := range w.nodes {
		shape := mermaidNodeShape(node.Type)
		label := sanitizeMermaidLabel(node.Name)
		if label == "" {
			label = id
		}

		// 节点定义
		sb.WriteString(fmt.Sprintf("    %s%s\"%s\"%s\n", id, shape.open, label, shape.close))

		// 根据状态添加样式（优先级：失败 > 高亮 > 起始 > 默认）
		if _, failed := failedSet[id]; failed {
			sb.WriteString(fmt.Sprintf("    style %s fill:#ff6b6b,stroke:#c0392b,color:#fff\n", id))
		} else if _, highlighted := highlightSet[id]; highlighted {
			sb.WriteString(fmt.Sprintf("    style %s fill:#51cf66,stroke:#2b8a3e,color:#fff\n", id))
		} else if id == w.startNodeID {
			sb.WriteString(fmt.Sprintf("    style %s fill:#339af0,stroke:#1864ab,color:#fff\n", id))
		}
	}

	// 处理并行节点：将子节点放入子图
	parallelGroups := identifyParallelGroups(w.nodes, w.transitions)
	for groupID, childIDs := range parallelGroups {
		sb.WriteString(fmt.Sprintf("    subgraph %s_parallel\n", groupID))
		for _, childID := range childIDs {
			sb.WriteString(fmt.Sprintf("        %s\n", childID))
		}
		sb.WriteString("    end\n")
	}

	// 生成边
	for fromID, transitions := range w.transitions {
		for _, trans := range transitions {
			label := ""
			if cfg.ShowLabels && trans.Condition != nil && trans.Condition.Type != "always" {
				label = sanitizeMermaidLabel(transitionLabel(trans))
			}

			// 高亮执行路径上的边
			_, fromHighlighted := highlightSet[fromID]
			_, toHighlighted := highlightSet[trans.To]
			if fromHighlighted && toHighlighted {
				if label != "" {
					sb.WriteString(fmt.Sprintf("    %s ==> |\"%s\"| %s\n", fromID, label, trans.To))
				} else {
					sb.WriteString(fmt.Sprintf("    %s ==> %s\n", fromID, trans.To))
				}
			} else if label != "" {
				sb.WriteString(fmt.Sprintf("    %s -.->|\"%s\"| %s\n", fromID, label, trans.To))
			} else {
				sb.WriteString(fmt.Sprintf("    %s --> %s\n", fromID, trans.To))
			}
		}
	}

	return sb.String()
}

// ToMermaidWithExecution 根据执行结果生成带高亮的 Mermaid 图
func (w *WorkflowExecution) ToMermaidWithExecution(result *WorkflowResult) string {
	cfg := DefaultVisualizeConfig()

	if result != nil {
		cfg.HighlightPath = result.PathTaken
		cfg.FailedNodes = extractFailedNodes(result)
	}

	return w.ToMermaidWithConfig(cfg)
}

// ToDot 将工作流生成 Graphviz DOT 格式字符串
func (w *WorkflowExecution) ToDot() string {
	return w.ToDotWithConfig(DefaultVisualizeConfig())
}

// ToDotWithConfig 使用指定配置生成 DOT 格式
func (w *WorkflowExecution) ToDotWithConfig(cfg VisualizeConfig) string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	highlightSet := makeStringSet(cfg.HighlightPath)
	failedSet := makeStringSet(cfg.FailedNodes)

	var sb strings.Builder
	sb.WriteString("digraph workflow {\n")
	sb.WriteString("    rankdir=TD;\n")
	sb.WriteString("    node [fontname=\"Arial\", fontsize=10];\n")
	sb.WriteString("    edge [fontname=\"Arial\", fontsize=9];\n")

	// 生成节点
	for id, node := range w.nodes {
		shape := dotNodeShape(node.Type)
		label := node.Name
		if label == "" {
			label = id
		}

		attrs := fmt.Sprintf("label=\"%s\", shape=%s", escapeDotLabel(label), shape)

		if _, failed := failedSet[id]; failed {
			attrs += ", style=filled, fillcolor=\"#ff6b6b\", fontcolor=white"
		} else if _, highlighted := highlightSet[id]; highlighted {
			attrs += ", style=filled, fillcolor=\"#51cf66\", fontcolor=white"
		} else if id == w.startNodeID {
			attrs += ", style=filled, fillcolor=\"#339af0\", fontcolor=white"
		}

		sb.WriteString(fmt.Sprintf("    \"%s\" [%s];\n", id, attrs))
	}

	// 生成边
	for fromID, transitions := range w.transitions {
		for _, trans := range transitions {
			label := ""
			if cfg.ShowLabels && trans.Condition != nil && trans.Condition.Type != "always" {
				label = transitionLabel(trans)
			}

			attrs := ""
			if label != "" {
				attrs = fmt.Sprintf(" [label=\"%s\"]", escapeDotLabel(label))
			}

			// 高亮执行路径上的边
			_, fromHighlighted := highlightSet[fromID]
			_, toHighlighted := highlightSet[trans.To]
			if fromHighlighted && toHighlighted {
				if attrs == "" {
					attrs = " [color=\"#51cf66\", penwidth=2]"
				} else {
					attrs = fmt.Sprintf(" [label=\"%s\", color=\"#51cf66\", penwidth=2]", escapeDotLabel(label))
				}
			}

			sb.WriteString(fmt.Sprintf("    \"%s\" -> \"%s\"%s;\n", fromID, trans.To, attrs))
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

// ToDotWithExecution 根据执行结果生成带高亮的 DOT 图
func (w *WorkflowExecution) ToDotWithExecution(result *WorkflowResult) string {
	cfg := DefaultVisualizeConfig()

	if result != nil {
		cfg.HighlightPath = result.PathTaken
		cfg.FailedNodes = extractFailedNodes(result)
	}

	return w.ToDotWithConfig(cfg)
}

// mermaidNodeShape 返回 Mermaid 节点形状标记
type mermaidShape struct {
	open  string
	close string
}

func mermaidNodeShape(nodeType NodeType) mermaidShape {
	switch nodeType {
	case ConditionNode:
		// 菱形
		return mermaidShape{"{", "}"}
	case ParallelNode:
		// 六边形
		return mermaidShape{"{{", "}}"}
	case LoopStartNode, LoopEndNode:
		// 圆柱形
		return mermaidShape{"[(", ")]"}
	case FallbackNode:
		// 平行四边形
		return mermaidShape{"[/", "/]"}
	default:
		// 矩形
		return mermaidShape{"[", "]"}
	}
}

// dotNodeShape 返回 DOT 节点形状
func dotNodeShape(nodeType NodeType) string {
	switch nodeType {
	case ConditionNode:
		return "diamond"
	case ParallelNode:
		return "hexagon"
	case LoopStartNode, LoopEndNode:
		return "cylinder"
	case FallbackNode:
		return "parallelogram"
	default:
		return "box"
	}
}

// transitionLabel 生成转换边的标签
func transitionLabel(trans *Transition) string {
	if trans.Condition == nil {
		return ""
	}

	switch trans.Condition.Type {
	case "always":
		return ""
	case "probability":
		return fmt.Sprintf("p=%.1f", trans.Condition.Probability)
	case "comparison":
		if trans.Condition.Field != "" {
			return fmt.Sprintf("%s %s %v", trans.Condition.Field, trans.Condition.Operator, trans.Condition.Value)
		}
		return ""
	case "custom":
		if trans.Condition.Expression != "" {
			return trans.Condition.Expression
		}
		return ""
	default:
		if trans.Condition.Field != "" {
			return fmt.Sprintf("%s %s %v", trans.Condition.Field, trans.Condition.Operator, trans.Condition.Value)
		}
		return trans.Condition.Type
	}
}

// identifyParallelGroups 识别并行节点组
func identifyParallelGroups(nodes map[string]*WorkflowNode, transitions map[string][]*Transition) map[string][]string {
	groups := make(map[string][]string)

	for id, node := range nodes {
		if node.Type == ParallelNode {
			var children []string
			if transList, ok := transitions[id]; ok {
				for _, trans := range transList {
					children = append(children, trans.To)
				}
			}
			if len(children) > 0 {
				groups[id] = children
			}
		}
	}

	return groups
}

// extractFailedNodes 从执行结果中提取失败节点 ID
func extractFailedNodes(result *WorkflowResult) []string {
	if result == nil || len(result.Records) == 0 {
		return nil
	}

	var failed []string
	for _, record := range result.Records {
		if record.Status == NodeFailed {
			failed = append(failed, record.NodeID)
		}
	}
	return failed
}

// makeStringSet 将字符串切片转为集合
func makeStringSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[item] = true
	}
	return set
}

// sanitizeMermaidLabel 清理 Mermaid 标签中的特殊字符
func sanitizeMermaidLabel(s string) string {
	s = strings.ReplaceAll(s, "\"", "'")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "[", "(")
	s = strings.ReplaceAll(s, "]", ")")
	s = strings.ReplaceAll(s, "{", "(")
	s = strings.ReplaceAll(s, "}", ")")
	return s
}

// escapeDotLabel 转义 DOT 标签中的特殊字符
func escapeDotLabel(s string) string {
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
