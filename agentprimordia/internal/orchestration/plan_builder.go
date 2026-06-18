package orchestration

import "fmt"

// ExecutionPlan 是统一执行计划。
type ExecutionPlan struct {
	Mode     OrchestratorMode
	Steps    []*StepNode
	DepGraph *DependencyGraph
}

// BuildExecutionPlan 将不同编排模式转换为统一的执行计划。
func BuildExecutionPlan(mode OrchestratorMode, steps []*AgentStep, edges []DAGEdge) (*ExecutionPlan, error) {
	switch mode {
	case SequentialMode:
		return buildSequentialPlan(steps)
	case ParallelMode:
		return buildParallelPlan(steps)
	case DAGMode:
		return buildDAGPlan(steps, edges)
	default:
		return nil, fmt.Errorf("unsupported orchestrator mode: %s", mode)
	}
}

func buildSequentialPlan(steps []*AgentStep) (*ExecutionPlan, error) {
	edges := make([]DAGEdge, 0, len(steps)-1)
	for i := 1; i < len(steps); i++ {
		edges = append(edges, DAGEdge{From: steps[i-1].ID, To: steps[i].ID})
	}
	dg, err := NewDependencyGraph(steps, edges)
	if err != nil {
		return nil, err
	}
	return &ExecutionPlan{Mode: SequentialMode, Steps: nodesFromSteps(steps), DepGraph: dg}, nil
}

func buildParallelPlan(steps []*AgentStep) (*ExecutionPlan, error) {
	dg, err := NewDependencyGraph(steps, nil)
	if err != nil {
		return nil, err
	}
	return &ExecutionPlan{Mode: ParallelMode, Steps: nodesFromSteps(steps), DepGraph: dg}, nil
}

func buildDAGPlan(steps []*AgentStep, edges []DAGEdge) (*ExecutionPlan, error) {
	if _, err := topologicalSort(steps, edges); err != nil {
		return nil, fmt.Errorf("DAG validation failed: %w", err)
	}
	dg, err := NewDependencyGraph(steps, edges)
	if err != nil {
		return nil, err
	}
	return &ExecutionPlan{Mode: DAGMode, Steps: nodesFromSteps(steps), DepGraph: dg}, nil
}

func nodesFromSteps(steps []*AgentStep) []*StepNode {
	nodes := make([]*StepNode, len(steps))
	for i, s := range steps {
		nodes[i] = &StepNode{Step: s, Status: StepPending}
	}
	return nodes
}

// topologicalSort 对步骤进行拓扑排序，用于检测 DAG 环。
func topologicalSort(steps []*AgentStep, edges []DAGEdge) ([]string, error) {
	adjacency := make(map[string][]string)
	inDegree := make(map[string]int)

	for _, step := range steps {
		adjacency[step.ID] = []string{}
		inDegree[step.ID] = 0
	}

	for _, edge := range edges {
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
		inDegree[edge.To]++
	}

	queue := []string{}
	for node, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, node)
		}
	}

	var result []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		for _, neighbor := range adjacency[node] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(result) != len(steps) {
		return nil, fmt.Errorf("cycle detected in DAG")
	}

	return result, nil
}
