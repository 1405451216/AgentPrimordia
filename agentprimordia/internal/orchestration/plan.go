package orchestration

import "fmt"

// StepNode 是统一执行计划中的步骤节点。
type StepNode struct {
	Step   *AgentStep
	Status StepStatus
	Result *StepResult
}

// DependencyGraph 维护步骤之间的依赖关系。
type DependencyGraph struct {
	inDegree map[string]int      // stepID -> 未完成的依赖数
	outEdges map[string][]string // stepID -> 下游 stepID 列表
	inEdges  map[string][]string // stepID -> 上游 stepID 列表
}

// NewDependencyGraph 根据步骤和边构建依赖图。
func NewDependencyGraph(steps []*AgentStep, edges []DAGEdge) (*DependencyGraph, error) {
	g := &DependencyGraph{
		inDegree: make(map[string]int, len(steps)),
		outEdges: make(map[string][]string, len(steps)),
		inEdges:  make(map[string][]string, len(steps)),
	}
	for _, s := range steps {
		g.inDegree[s.ID] = 0
	}
	for _, e := range edges {
		if _, ok := g.inDegree[e.From]; !ok {
			return nil, fmt.Errorf("unknown step %q in edge", e.From)
		}
		if _, ok := g.inDegree[e.To]; !ok {
			return nil, fmt.Errorf("unknown step %q in edge", e.To)
		}
		g.inDegree[e.To]++
		g.outEdges[e.From] = append(g.outEdges[e.From], e.To)
		g.inEdges[e.To] = append(g.inEdges[e.To], e.From)
	}
	return g, nil
}

// Ready 判断指定步骤是否已无未完成的依赖。
func (g *DependencyGraph) Ready(stepID string) bool {
	return g.inDegree[stepID] == 0
}

// Complete 标记指定步骤已完成，返回因此变为就绪的下游步骤 ID 列表。
func (g *DependencyGraph) Complete(stepID string) []string {
	newlyReady := make([]string, 0)
	for _, next := range g.outEdges[stepID] {
		g.inDegree[next]--
		if g.inDegree[next] == 0 {
			newlyReady = append(newlyReady, next)
		}
	}
	return newlyReady
}
