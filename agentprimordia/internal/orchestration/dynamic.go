package orchestration

import (
	"context"
	"fmt"
	"sync"
)

// DynamicNodeHandler 动态节点处理器
type DynamicNodeHandler struct {
	ID      string
	Handler func(ctx context.Context, input any) (any, error)
}

// DynamicDAG 支持运行时修改拓扑的 DAG
type DynamicDAG struct {
	mu         sync.RWMutex
	name       string
	nodes      map[string]*dynamicNode
	edges      map[string][]string         // from -> []to
	conditions map[string]map[string]string // from -> {output -> toID}
}

type dynamicNode struct {
	handler DynamicNodeHandler
}

// NewDynamicDAG 创建动态 DAG
func NewDynamicDAG(name string) *DynamicDAG {
	return &DynamicDAG{
		name:       name,
		nodes:      make(map[string]*dynamicNode),
		edges:      make(map[string][]string),
		conditions: make(map[string]map[string]string),
	}
}

// AddNode 添加节点（线程安全）
func (d *DynamicDAG) AddNode(handler DynamicNodeHandler) {
	d.mu.Lock()
	d.nodes[handler.ID] = &dynamicNode{handler: handler}
	d.mu.Unlock()
}

// RemoveNode 移除节点及其相关边
func (d *DynamicDAG) RemoveNode(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.nodes, id)
	delete(d.edges, id)
	delete(d.conditions, id)

	// 清理指向该节点的边
	for from, tos := range d.edges {
		filtered := make([]string, 0, len(tos))
		for _, to := range tos {
			if to != id {
				filtered = append(filtered, to)
			}
		}
		d.edges[from] = filtered
	}
}

// AddEdge 添加边
func (d *DynamicDAG) AddEdge(from, to string) {
	d.mu.Lock()
	d.edges[from] = append(d.edges[from], to)
	d.mu.Unlock()
}

// AddConditionalEdge 添加条件边
func (d *DynamicDAG) AddConditionalEdge(from string, routing map[string]string) {
	d.mu.Lock()
	d.conditions[from] = routing
	d.mu.Unlock()
}

// NodeCount 返回节点数
func (d *DynamicDAG) NodeCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.nodes)
}

// Execute 执行 DAG
func (d *DynamicDAG) Execute(ctx context.Context, input any) (any, error) {
	d.mu.RLock()
	startNodes := d.findStartNodes()
	d.mu.RUnlock()

	if len(startNodes) == 0 {
		return nil, fmt.Errorf("no start nodes found")
	}

	current := input
	for _, startID := range startNodes {
		result, err := d.executeFrom(ctx, startID, current)
		if err != nil {
			return nil, err
		}
		current = result
	}
	return current, nil
}

func (d *DynamicDAG) executeFrom(ctx context.Context, nodeID string, input any) (any, error) {
	d.mu.RLock()
	node, ok := d.nodes[nodeID]
	d.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("node %q not found", nodeID)
	}

	result, err := node.handler.Handler(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("node %q failed: %w", nodeID, err)
	}

	// 检查条件路由
	d.mu.RLock()
	routing, hasCondition := d.conditions[nodeID]
	d.mu.RUnlock()

	if hasCondition {
		routeKey := fmt.Sprintf("%v", result)
		if nextID, ok := routing[routeKey]; ok {
			return d.executeFrom(ctx, nextID, result)
		}
	}

	// 普通边
	d.mu.RLock()
	nexts := d.edges[nodeID]
	d.mu.RUnlock()

	for _, nextID := range nexts {
		r, err := d.executeFrom(ctx, nextID, result)
		if err != nil {
			return nil, err
		}
		result = r
	}

	return result, nil
}

func (d *DynamicDAG) findStartNodes() []string {
	hasIncoming := make(map[string]bool)
	for _, tos := range d.edges {
		for _, to := range tos {
			hasIncoming[to] = true
		}
	}

	var starts []string
	for id := range d.nodes {
		if !hasIncoming[id] {
			starts = append(starts, id)
		}
	}
	return starts
}
