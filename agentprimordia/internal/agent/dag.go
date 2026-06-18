package agent

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DAGNode DAG 工作流节点
type DAGNode struct {
	ID          string
	Agent       Agent
	Input       string
	Metadata    map[string]string
	RetryPolicy *RetryPolicy
}

// RetryPolicy 节点重试策略
type RetryPolicy struct {
	MaxRetries int                          // 最大重试次数，0 表示不重试
	Delay      time.Duration                // 重试间隔
	Backoff    float64                      // 退避倍数（如 2.0 表示指数退避）
	OnRetry    func(attempt int, err error) // 每次重试时的回调
}

// DAGEdge DAG 工作流边（含条件谓词）
type DAGEdge struct {
	From      string
	To        string
	Label     string // 边标签（可视化用）
	Condition func(ctx context.Context, result *DAGNodeResult) bool
}

// DAGNodeResult 节点执行结果
type DAGNodeResult struct {
	NodeID    string
	Output    string
	Error     error
	Skipped   bool
	Retries   int // 实际重试次数
	Duration  time.Duration
	Timestamp time.Time
	Inputs    []string // 来自前置节点的输入
}

// DAGResult DAG 工作流执行结果
type DAGResult struct {
	NodeResults map[string]*DAGNodeResult
	Order       []string
	Duration    time.Duration
	TotalNodes  int
	Succeeded   int
	Failed      int
	Skipped     int
}

// DAGWorkflow DAG 工作流引擎
type DAGWorkflow struct {
	nodes   map[string]*DAGNode
	edges   []DAGEdge
	hooks   Hooks
	mu      sync.RWMutex
	name    string
	metrics *DAGMetrics
	subDAGs map[string]*DAGWorkflow
	parent  *DAGWorkflow
}

// DAGMetrics DAG 执行指标
// perf-v4 Task 3：NodeStats map 注册仍需锁保护（首次写入），记录更新已无锁
type DAGMetrics struct {
	mu              sync.RWMutex
	TotalExecutions atomic.Int64
	TotalDuration   atomic.Int64 // 纳秒累加，Snapshot() 转 Duration
	NodeStats       map[string]*NodeExecutionStats
}

// NodeExecutionStats 单节点执行统计
// perf-v4 Task 3：所有计数器字段改为 atomic.Int64，record() 无锁化
type NodeExecutionStats struct {
	TotalRuns    atomic.Int64
	Successes    atomic.Int64
	Failures     atomic.Int64
	AvgDuration  atomic.Int64 // 纳秒，读取时转 Duration
	MaxDuration  atomic.Int64 // 纳秒
	MinDuration  atomic.Int64 // 纳秒
	TotalRetries atomic.Int64
}

func newDAGMetrics() *DAGMetrics {
	return &DAGMetrics{
		NodeStats: make(map[string]*NodeExecutionStats),
	}
}

// record 记录节点执行统计（perf-v4 Task 3：无锁原子更新）
// 仅在节点首次注册时获取写锁；已存在节点的更新全部使用 atomic 操作
func (m *DAGMetrics) record(nodeID string, dur time.Duration, success bool, retries int) {
	stats, ok := m.NodeStats[nodeID]
	if !ok {
		m.mu.Lock()
		// double-check：避免并发首次写入时覆盖
		stats, ok = m.NodeStats[nodeID]
		if !ok {
			stats = &NodeExecutionStats{}
			m.NodeStats[nodeID] = stats
		}
		m.mu.Unlock()
	}

	// 后续更新全部无锁（perf-v4 Task 3：原子操作）
	stats.TotalRuns.Add(1)
	if success {
		stats.Successes.Add(1)
	} else {
		stats.Failures.Add(1)
	}
	if retries > 0 {
		stats.TotalRetries.Add(int64(retries))
	}

	// MaxDuration 使用 CAS 循环更新（perf-v4 Task 3）
	durNanos := int64(dur)
	for {
		old := stats.MaxDuration.Load()
		if old != 0 && durNanos <= old {
			break
		}
		if stats.MaxDuration.CompareAndSwap(old, durNanos) {
			break
		}
	}
	// MinDuration CAS 循环（首条记录时为 0，直接 Store）
	for {
		old := stats.MinDuration.Load()
		if old != 0 && durNanos >= old {
			break
		}
		if stats.MinDuration.CompareAndSwap(old, durNanos) {
			break
		}
	}

	// AvgDuration：running sum / count，sum 通过 CAS 累加
	for {
		oldSum := stats.AvgDuration.Load()
		newSum := oldSum + durNanos
		if stats.AvgDuration.CompareAndSwap(oldSum, newSum) {
			break
		}
	}
}

// Snapshot 读取当前所有指标的快照（perf-v4 Task 3：原子读取）
func (m *DAGMetrics) Snapshot() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := map[string]interface{}{
		"total_executions": m.TotalExecutions.Load(),
		"total_duration":   time.Duration(m.TotalDuration.Load()).String(),
		"node_stats":       make(map[string]interface{}),
	}
	for id, s := range m.NodeStats {
		result["node_stats"].(map[string]interface{})[id] = map[string]interface{}{
			"total_runs":    s.TotalRuns.Load(),
			"successes":     s.Successes.Load(),
			"failures":      s.Failures.Load(),
			"avg_duration":  avgDurationFromSum(s.AvgDuration.Load(), s.TotalRuns.Load()).String(),
			"max_duration":  time.Duration(s.MaxDuration.Load()).String(),
			"min_duration":  time.Duration(s.MinDuration.Load()).String(),
			"total_retries": s.TotalRetries.Load(),
		}
	}
	return result
}

// avgDurationFromSum 根据累加总和和次数计算平均（perf-v4 Task 3 辅助函数）
func avgDurationFromSum(sumNanos int64, count int64) time.Duration {
	if count <= 0 {
		return 0
	}
	return time.Duration(sumNanos / count)
}

// NewDAGWorkflow 创建 DAG 工作流
func NewDAGWorkflow() *DAGWorkflow {
	return &DAGWorkflow{
		nodes:   make(map[string]*DAGNode),
		metrics: newDAGMetrics(),
		subDAGs: make(map[string]*DAGWorkflow),
	}
}

// WithName 设置工作流名称
func (d *DAGWorkflow) WithName(name string) *DAGWorkflow {
	d.name = name
	return d
}

// AddNode 添加节点
func (d *DAGWorkflow) AddNode(node *DAGNode) error {
	if node == nil {
		return fmt.Errorf("dag: node cannot be nil")
	}
	if node.ID == "" {
		return fmt.Errorf("dag: node ID cannot be empty")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.nodes[node.ID]; exists {
		return fmt.Errorf("dag: duplicate node ID %q", node.ID)
	}
	d.nodes[node.ID] = node
	return nil
}

// AddEdge 添加边（含可选条件）
func (d *DAGWorkflow) AddEdge(edge DAGEdge) error {
	if edge.From == "" || edge.To == "" {
		return fmt.Errorf("dag: edge From and To cannot be empty")
	}
	d.mu.RLock()
	_, fromExists := d.nodes[edge.From]
	_, toExists := d.nodes[edge.To]
	d.mu.RUnlock()
	if !fromExists {
		return fmt.Errorf("dag: edge references non-existent source node %q", edge.From)
	}
	if !toExists {
		return fmt.Errorf("dag: edge references non-existent target node %q", edge.To)
	}
	d.mu.Lock()
	d.edges = append(d.edges, edge)
	d.mu.Unlock()
	return nil
}

// SetHooks 设置钩子
func (d *DAGWorkflow) SetHooks(hooks Hooks) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hooks = hooks
}

// SetParent 设置父 DAG（用于子 DAG 嵌套）
func (d *DAGWorkflow) SetParent(parent *DAGWorkflow) {
	d.parent = parent
}

// AddSubDAG 添加子 DAG
func (d *DAGWorkflow) AddSubDAG(name string, sub *DAGWorkflow) error {
	if name == "" {
		return fmt.Errorf("dag: sub-dag name cannot be empty")
	}
	if sub == nil {
		return fmt.Errorf("dag: sub-dag cannot be nil")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.subDAGs[name] = sub
	sub.SetParent(d)
	return nil
}

// Validate 验证 DAG（检测循环、孤立节点等）
func (d *DAGWorkflow) Validate() error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, edge := range d.edges {
		if _, ok := d.nodes[edge.From]; !ok {
			return fmt.Errorf("dag: edge references non-existent source node %q", edge.From)
		}
		if _, ok := d.nodes[edge.To]; !ok {
			return fmt.Errorf("dag: edge references non-existent target node %q", edge.To)
		}
	}

	if d.hasCycle() {
		return fmt.Errorf("dag: cycle detected")
	}

	orphaned := d.findOrphanedNodes()
	if len(orphaned) > 0 {
		sort.Strings(orphaned)
		return fmt.Errorf("dag: orphaned nodes detected: %v", orphaned)
	}

	return nil
}

// findOrphanedNodes 找到完全断开连接的孤立节点（无入边且无出边）
// 当图中存在边时，既没有入边也没有出边的节点被视为孤立节点
func (d *DAGWorkflow) findOrphanedNodes() []string {
	if len(d.edges) == 0 {
		return nil
	}
	hasIncoming := make(map[string]bool)
	hasOutgoing := make(map[string]bool)
	for _, edge := range d.edges {
		hasIncoming[edge.To] = true
		hasOutgoing[edge.From] = true
	}
	var orphans []string
	for id := range d.nodes {
		if !hasIncoming[id] && !hasOutgoing[id] {
			orphans = append(orphans, id)
		}
	}
	return orphans
}

// hasCycle 使用 DFS 三色标记法检测环
func (d *DAGWorkflow) hasCycle() bool {
	const (
		white = 0
		gray  = 1
		black = 2
	)

	adj := d.buildAdjList()
	color := make(map[string]int)
	for id := range d.nodes {
		color[id] = white
	}

	var dfs func(nodeID string) bool
	dfs = func(nodeID string) bool {
		color[nodeID] = gray
		for _, neighbor := range adj[nodeID] {
			if color[neighbor] == gray {
				return true
			}
			if color[neighbor] == white && dfs(neighbor) {
				return true
			}
		}
		color[nodeID] = black
		return false
	}

	for id := range d.nodes {
		if color[id] == white {
			if dfs(id) {
				return true
			}
		}
	}
	return false
}

// buildAdjList 构建邻接表
func (d *DAGWorkflow) buildAdjList() map[string][]string {
	adj := make(map[string][]string)
	for id := range d.nodes {
		adj[id] = nil
	}
	for _, edge := range d.edges {
		adj[edge.From] = append(adj[edge.From], edge.To)
	}
	return adj
}

// TopologicalSort 拓扑排序返回执行顺序
func (d *DAGWorkflow) TopologicalSort() ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	adj := d.buildAdjList()
	inDegree := make(map[string]int)
	for id := range d.nodes {
		inDegree[id] = 0
	}
	for _, neighbors := range adj {
		for _, n := range neighbors {
			inDegree[n]++
		}
	}

	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	var order []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		order = append(order, current)
		for _, neighbor := range adj[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(order) != len(d.nodes) {
		return nil, fmt.Errorf("dag: cycle detected during topological sort")
	}
	return order, nil
}

// GetDependencies 返回指定节点的直接依赖
func (d *DAGWorkflow) GetDependencies(nodeID string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var deps []string
	for _, edge := range d.edges {
		if edge.To == nodeID {
			deps = append(deps, edge.From)
		}
	}
	sort.Strings(deps)
	return deps
}

// GetDependents 返回依赖指定节点的下游节点
func (d *DAGWorkflow) GetDependents(nodeID string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var deps []string
	for _, edge := range d.edges {
		if edge.From == nodeID {
			deps = append(deps, edge.To)
		}
	}
	sort.Strings(deps)
	return deps
}

// Run 执行 DAG 工作流
func (d *DAGWorkflow) Run(ctx context.Context, input string) (*DAGResult, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}

	d.mu.RLock()
	nodes := make(map[string]*DAGNode, len(d.nodes))
	maps.Copy(nodes, d.nodes)
	edges := make([]DAGEdge, len(d.edges))
	copy(edges, d.edges)
	hooks := d.hooks
	d.mu.RUnlock()

	start := time.Now()

	outgoing := make(map[string][]int)
	incoming := make(map[string][]int)
	for id := range nodes {
		outgoing[id] = nil
		incoming[id] = nil
	}
	for i, edge := range edges {
		outgoing[edge.From] = append(outgoing[edge.From], i)
		incoming[edge.To] = append(incoming[edge.To], i)
	}

	remainingDeps := make(map[string]int)
	for id := range nodes {
		remainingDeps[id] = len(incoming[id])
	}

	result := &DAGResult{
		NodeResults: make(map[string]*DAGNodeResult),
		Order:       make([]string, 0, len(nodes)),
		TotalNodes:  len(nodes),
	}

	var stateMu sync.Mutex

	for len(result.NodeResults) < len(nodes) {
		var ready []string
		for id := range nodes {
			if _, done := result.NodeResults[id]; done {
				continue
			}
			if remainingDeps[id] == 0 {
				ready = append(ready, id)
			}
		}

		if len(ready) == 0 {
			break
		}

		for _, nodeID := range ready {
			result.NodeResults[nodeID] = &DAGNodeResult{NodeID: nodeID}
		}

		var wg sync.WaitGroup
		for _, nodeID := range ready {
			wg.Add(1)
			go func(nid string) {
				defer wg.Done()
				node := nodes[nid]
				nr := result.NodeResults[nid]

				// 统一在 defer 中完成 Order 追加与 remainingDeps 递减，
				// 确保无论执行成功/失败/跳过/panic 都能正确传播
				defer func() {
					stateMu.Lock()
					result.Order = append(result.Order, nid)
					for _, edgeIdx := range outgoing[nid] {
						dst := edges[edgeIdx].To
						if remainingDeps[dst] > 0 {
							remainingDeps[dst]--
						}
					}
					stateMu.Unlock()
				}()

				// 评估条件边：仅统计"活跃"的入边
				stateMu.Lock()
				activeCount := 0
				for _, edgeIdx := range incoming[nid] {
					edge := edges[edgeIdx]
					if edge.Condition == nil {
						activeCount++
					} else {
						srcResult := result.NodeResults[edge.From]
						// 仅当源节点实际执行成功（非跳过）时才评估条件
						if srcResult != nil && !srcResult.Skipped && srcResult.Error == nil && edge.Condition(ctx, srcResult) {
							activeCount++
						}
					}
				}
				hasIncoming := len(incoming[nid]) > 0
				stateMu.Unlock()

				// 所有活跃条件均为 false → 跳过此节点
				if hasIncoming && activeCount == 0 {
					nr.Skipped = true
					nr.Timestamp = time.Now()
					stateMu.Lock()
					result.Skipped++
					stateMu.Unlock()
					return
				}

				nodeInput := input
				if node.Input != "" {
					nodeInput = node.Input
				}

				if hooks != nil {
					_ = hooks.Fire(ctx, &HookContext{
						Point:    HookBeforeDAGNode,
						AgentID:  nid,
						Metadata: map[string]any{"node_id": nid},
					})
				}

				nodeStart := time.Now()
				resp, retries, execErr := d.executeWithRetry(ctx, node, nodeInput)

				if hooks != nil {
					_ = hooks.Fire(ctx, &HookContext{
						Point:    HookAfterDAGNode,
						AgentID:  nid,
						Error:    execErr,
						Metadata: map[string]any{"node_id": nid},
					})
				}

				nr.Retries = retries
				nr.Timestamp = time.Now()
				nr.Duration = time.Since(nodeStart)

				if execErr != nil {
					nr.Error = execErr
					stateMu.Lock()
					result.Failed++
					stateMu.Unlock()
					d.metrics.record(nid, nr.Duration, false, retries)
				} else {
					nr.Output = resp.Content
					stateMu.Lock()
					result.Succeeded++
					stateMu.Unlock()
					d.metrics.record(nid, nr.Duration, true, retries)
				}
			}(nodeID)
		}
		wg.Wait()
	}

	for id := range nodes {
		if _, done := result.NodeResults[id]; !done {
			result.NodeResults[id] = &DAGNodeResult{
				NodeID:    id,
				Skipped:   true,
				Timestamp: time.Now(),
			}
			result.Skipped++
		}
	}

	result.Duration = time.Since(start)
	// perf-v4 Task 3：TotalExecutions / TotalDuration 改为无锁原子累加
	d.metrics.TotalExecutions.Add(1)
	d.metrics.TotalDuration.Add(int64(result.Duration))

	return result, nil
}

// executeWithRetry 带重试的节点执行
func (d *DAGWorkflow) executeWithRetry(ctx context.Context, node *DAGNode, input string) (*Response, int, error) {
	policy := node.RetryPolicy
	if policy == nil || policy.MaxRetries <= 0 {
		resp, err := node.Agent.Run(ctx, UserMessage(input))
		return resp, 0, err
	}

	var lastErr error
	var resp *Response
	delay := policy.Delay

	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		if attempt > 0 {
			if policy.OnRetry != nil {
				policy.OnRetry(attempt, lastErr)
			}
			select {
			case <-ctx.Done():
				return nil, attempt - 1, ctx.Err()
			case <-time.After(delay):
			}
			delay = time.Duration(float64(delay) * policy.Backoff)
		}

		resp, lastErr = node.Agent.Run(ctx, UserMessage(input))
		if lastErr == nil {
			return resp, attempt, nil
		}
	}

	return resp, policy.MaxRetries, lastErr
}

// Metrics 返回执行指标
func (d *DAGWorkflow) Metrics() *DAGMetrics {
	return d.metrics
}

// NodeCount 返回节点数量
func (d *DAGWorkflow) NodeCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.nodes)
}

// EdgeCount 返回边数量
func (d *DAGWorkflow) EdgeCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.edges)
}

// ===== 可视化导出 =====

// ToMermaid 导出为 Mermaid 格式
func (d *DAGWorkflow) ToMermaid() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("graph LR\n")

	sortedIDs := make([]string, 0, len(d.nodes))
	for id := range d.nodes {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)

	for _, id := range sortedIDs {
		node := d.nodes[id]
		label := node.Metadata["label"]
		if label == "" {
			label = id
		}
		fmt.Fprintf(&sb, "    %s[%q]\n", sanitizeMermaidID(id), label)
	}

	for _, edge := range d.edges {
		labelStr := ""
		if edge.Label != "" {
			labelStr = fmt.Sprintf("|%s|", edge.Label)
		}
		condStr := ""
		if edge.Condition != nil {
			condStr = "{?}"
		}
		fmt.Fprintf(&sb, "    %s -->%s%s %s\n",
			sanitizeMermaidID(edge.From), labelStr, condStr, sanitizeMermaidID(edge.To))
	}

	return sb.String()
}

// ToPlantUML 导出为 PlantUML 格式
func (d *DAGWorkflow) ToPlantUML() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("@startuml\n")

	if d.name != "" {
		fmt.Fprintf(&sb, "title %s\n", d.name)
	}

	sortedIDs := make([]string, 0, len(d.nodes))
	for id := range d.nodes {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)

	for _, id := range sortedIDs {
		node := d.nodes[id]
		label := node.Metadata["label"]
		if label == "" {
			label = id
		}
		fmt.Fprintf(&sb, "[%s]\n", label)
	}

	for _, edge := range d.edges {
		fromLabel := edge.From
		toLabel := edge.To
		if fn := d.nodes[edge.From]; fn != nil && fn.Metadata["label"] != "" {
			fromLabel = fn.Metadata["label"]
		}
		if tn := d.nodes[edge.To]; tn != nil && tn.Metadata["label"] != "" {
			toLabel = tn.Metadata["label"]
		}
		arrow := "-->"
		if edge.Condition != nil {
			arrow = "-->[dashed]"
		}
		if edge.Label != "" {
			fmt.Fprintf(&sb, "[%s] %s [%s] : %s\n", fromLabel, arrow, toLabel, edge.Label)
		} else {
			fmt.Fprintf(&sb, "[%s] %s [%s]\n", fromLabel, arrow, toLabel)
		}
	}

	sb.WriteString("@enduml\n")
	return sb.String()
}

// ToDot 导出为 Graphviz DOT 格式
func (d *DAGWorkflow) ToDot() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("digraph G {\n")
	sb.WriteString("  rankdir=LR;\n")
	sb.WriteString("  node [shape=box];\n")

	if d.name != "" {
		fmt.Fprintf(&sb, "  label=%q;\n", d.name)
	}

	sortedIDs := make([]string, 0, len(d.nodes))
	for id := range d.nodes {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)

	for _, id := range sortedIDs {
		node := d.nodes[id]
		label := node.Metadata["label"]
		if label == "" {
			label = id
		}
		fmt.Fprintf(&sb, "  %q [label=%q];\n", id, label)
	}

	for _, edge := range d.edges {
		attr := ""
		if edge.Condition != nil {
			attr += " [style=dashed]"
		}
		if edge.Label != "" {
			attr = fmt.Sprintf(" [label=%q]", edge.Label)
		}
		fmt.Fprintf(&sb, "  %q -> %q%s;\n", edge.From, edge.To, attr)
	}

	sb.WriteString("}\n")
	return sb.String()
}

// ToJSON 导出为 JSON 友好的结构化数据
func (d *DAGWorkflow) ToJSON() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	nodes := make([]map[string]string, 0, len(d.nodes))
	for id, node := range d.nodes {
		entry := map[string]string{"id": id}
		if l, ok := node.Metadata["label"]; ok {
			entry["label"] = l
		}
		nodes = append(nodes, entry)
	}

	edges := make([]map[string]string, 0, len(d.edges))
	for _, edge := range d.edges {
		e := map[string]string{"from": edge.From, "to": edge.To}
		if edge.Label != "" {
			e["label"] = edge.Label
		}
		edges = append(edges, e)
	}

	return map[string]interface{}{
		"name":  d.name,
		"nodes": nodes,
		"edges": edges,
	}
}

func sanitizeMermaidID(id string) string {
	id = strings.ReplaceAll(id, "-", "_")
	id = strings.ReplaceAll(id, ".", "_")
	id = strings.ReplaceAll(id, " ", "_")
	return id
}
