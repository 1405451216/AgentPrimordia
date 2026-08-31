// graph.go — 世界模型状态图内核（V7 路线图 v6.1「具模」工程地板第一切片）
//
// 设计定位（docs/V7路线图.md §三 / docs/提案-世界模型默认策略切换.md §2.1）：
//   - 纯 Go 标准库实现，零外部依赖；opt-in 旁路，不接默认 ReAct 路径；
//   - 世界 = 状态图：任务/计划/工具调用/观察/假设五类节点 +
//     因果/计划/上下文/假设四类边（预演态与观测态在图中靠边分型）；
//   - 同一 (Kind, 规范化 Summary) 只产生一个节点（确定性 ID 去重）——
//     「计划步骤」与「实际工具调用」因此收敛到同一节点，执行后追加因果边
//     即完成「预演态→观测态」分型（提案 §四 风险缓解③）；
//   - 并发安全（sync.RWMutex）；对外只暴露值快照，内部状态不外泄。
package worldmodel

import (
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// NodeKind 状态节点种类——对应路线图 §三 的五类世界事实。
type NodeKind string

const (
	// KindTask 任务：正在执行的顶层任务。
	KindTask NodeKind = "task"
	// KindPlan 计划：任务的目标分解。
	KindPlan NodeKind = "plan"
	// KindToolCall 工具调用：计划步骤（预演态）与实际调用（观测态）共用。
	KindToolCall NodeKind = "tool_call"
	// KindObservation 观察：工具结果与被裁剪的历史事实。
	KindObservation NodeKind = "observation"
	// KindHypothesis 假设：推理产物，非观测事实。
	KindHypothesis NodeKind = "hypothesis"
)

// EdgeKind 边种类——预演态/观测态在状态图中强制分型的载体（提案 §四）。
type EdgeKind string

const (
	// EdgeCause 因果边：工具调用 → 观察。仅真实观测后出现，是「观测态」标记。
	EdgeCause EdgeKind = "cause"
	// EdgePlan 计划边：任务 → 计划、计划 → 计划步骤。仅预演态持有。
	EdgePlan EdgeKind = "plan"
	// EdgeContext 上下文边：任务 → 被裁剪进图的历史事实。
	EdgeContext EdgeKind = "context"
	// EdgeHypothesis 假设边：任务 → 假设（推理产物）。
	EdgeHypothesis EdgeKind = "hypothesis"
)

// StateEdge 有向边：由出发节点持有，指向目标节点。
type StateEdge struct {
	To   string   // 目标节点 ID
	Kind EdgeKind // 边种类（分型载体，调用方必须显式给出）
}

// StateNode 状态图节点：一条结构化世界事实。
// ID 由 (Kind, 规范化 Summary) 确定性派生（见 stateNodeID），跨进程可复算。
type StateNode struct {
	ID            string      // 确定性 ID："<kind>:<fnv1a64 十六进制>"
	Kind          NodeKind    // 节点种类
	Summary       string      // 规范化摘要（去首尾空白、空白折叠为单空格）
	CreatedAtTurn int         // 首次进入图的轮次（首见优先，重复观察不回改）
	Edges         []StateEdge // 出边（插入序，(To,Kind) 去重）
}

// StateGraph 有向状态图。
// 并发安全：内部读写锁；Node/Nodes 返回防御性拷贝，调用方无法改动内部状态。
type StateGraph struct {
	mu    sync.RWMutex
	nodes map[string]*StateNode
	rev   map[string][]string // 入边反向索引：目标 ID → 出发节点列表（插入序）
}

// NewStateGraph 构造空状态图。
func NewStateGraph() *StateGraph {
	return &StateGraph{
		nodes: make(map[string]*StateNode),
		rev:   make(map[string][]string),
	}
}

// AddNode 添加（或命中去重）节点，返回节点 ID 与是否新建。
// 去重键 = (Kind, 规范化 Summary)；重复添加返回既有节点且不新建，
// CreatedAtTurn 首见优先、不回改——同一事实重复观察不产生重复节点。
func (g *StateGraph) AddNode(kind NodeKind, summary string, createdAtTurn int) (string, bool) {
	norm := normalizeSummary(summary)
	id := stateNodeID(kind, norm)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lazyInit()
	if _, ok := g.nodes[id]; ok {
		return id, false
	}
	g.nodes[id] = &StateNode{
		ID:            id,
		Kind:          kind,
		Summary:       norm,
		CreatedAtTurn: createdAtTurn,
	}
	return id, true
}

// AddEdge 添加有向边，返回是否新建。
// 端点不存在或同 (To,Kind) 边已存在时不新建并返回 false——
// 重复因果观察不产生重复边，图对「同结果重复回写」幂等。
func (g *StateGraph) AddEdge(from, to string, kind EdgeKind) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lazyInit()
	fromNode, ok := g.nodes[from]
	if !ok {
		return false
	}
	if _, ok := g.nodes[to]; !ok {
		return false
	}
	for _, e := range fromNode.Edges {
		if e.To == to && e.Kind == kind {
			return false
		}
	}
	fromNode.Edges = append(fromNode.Edges, StateEdge{To: to, Kind: kind})
	g.rev[to] = append(g.rev[to], from)
	return true
}

// Node 返回节点值快照（Edges 防御性拷贝）；不存在时 ok=false。
func (g *StateGraph) Node(id string) (StateNode, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.nodes[id]
	if !ok {
		return StateNode{}, false
	}
	return cloneNode(*n), true
}

// Nodes 返回全部节点快照，按 ID 升序（确定性遍历序）。
func (g *StateGraph) Nodes() []StateNode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]StateNode, 0, len(g.nodes))
	for _, n := range g.nodes {
		out = append(out, cloneNode(*n))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Ancestors 返回沿入边反向可达的全部祖先节点 ID（升序）。
// 不含查询节点本身；环/自环安全（visited 集合兜底必终止）。
// 节点不存在或无祖先时返回 nil。
func (g *StateGraph) Ancestors(id string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if _, ok := g.nodes[id]; !ok {
		return nil
	}
	visited := map[string]bool{id: true}
	queue := append([]string(nil), g.rev[id]...)
	var out []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur] {
			continue
		}
		visited[cur] = true
		out = append(out, cur)
		queue = append(queue, g.rev[cur]...)
	}
	sort.Strings(out)
	return out
}

// PathTo 返回从某个根节点（入度为零）到目标节点的一条 BFS 最短路径
// （含两端；根即目标时返回 [目标]）。确定性约定：根按 ID 升序尝试，
// 邻接按目标 ID 升序扩展——相同图必得相同路径。
// 环安全：visited 集合兜底必终止；无根可达（孤立连通分量/纯环）时返回 nil。
func (g *StateGraph) PathTo(id string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if _, ok := g.nodes[id]; !ok {
		return nil
	}
	// 根 = 无入边节点，升序尝试
	var roots []string
	for nodeID := range g.nodes {
		if len(g.rev[nodeID]) == 0 {
			roots = append(roots, nodeID)
		}
	}
	sort.Strings(roots)
	for _, root := range roots {
		if path := bfsPath(g.nodes, root, id); path != nil {
			return path
		}
	}
	return nil
}

// NodeID 由 (Kind, 摘要) 派生确定性节点 ID——先规范化摘要再哈希，
// 与 AddNode 内部派生规则完全一致。接入层据此在事件构造前预知节点 ID，
// 使「计划步骤 ID」与「实际工具调用节点 ID」收敛到同一确定性 ID 空间
// （回溯差异 ComparePaths 的可比性前提；接线点②⑥契约，见 options.go）。
func NodeID(kind NodeKind, summary string) string {
	return stateNodeID(kind, normalizeSummary(summary))
}

// bfsPath 在持锁前提下从 root 做 BFS（邻接按目标 ID 升序），
// 返回 root→target 的首达路径；不可达返回 nil。
func bfsPath(nodes map[string]*StateNode, root, target string) []string {
	if root == target {
		return []string{root}
	}
	visited := map[string]bool{root: true}
	parent := map[string]string{}
	queue := []string{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		nexts := make([]string, 0, len(nodes[cur].Edges))
		for _, e := range nodes[cur].Edges {
			nexts = append(nexts, e.To)
		}
		sort.Strings(nexts)
		for _, next := range nexts {
			if visited[next] {
				continue
			}
			visited[next] = true
			parent[next] = cur
			if next == target {
				// 回溯重建路径后反转
				path := []string{target}
				for p := parent[target]; p != root; p = parent[p] {
					path = append(path, p)
				}
				path = append(path, root)
				for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
					path[i], path[j] = path[j], path[i]
				}
				return path
			}
			queue = append(queue, next)
		}
	}
	return nil
}

// lazyInit 懒初始化内部 map（须持写锁；零值 StateGraph 可直接使用）。
func (g *StateGraph) lazyInit() {
	if g.nodes == nil {
		g.nodes = make(map[string]*StateNode)
	}
	if g.rev == nil {
		g.rev = make(map[string][]string)
	}
}

// cloneNode 节点防御性拷贝（Edges 深拷贝，出参改动不回流内部状态）。
func cloneNode(n StateNode) StateNode {
	if len(n.Edges) > 0 {
		edges := make([]StateEdge, len(n.Edges))
		copy(edges, n.Edges)
		n.Edges = edges
	} else {
		n.Edges = nil
	}
	return n
}

// normalizeSummary 摘要规范化：按 Unicode 空白切分后以单空格重组。
// 使「同义同形、仅空白差异」的摘要获得同一去重键（跨轮次、跨平台稳定）。
func normalizeSummary(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// stateNodeID 由 (Kind, 规范化摘要) 派生确定性节点 ID：
// FNV-1a 64 位（标准库 hash/fnv），Kind 与摘要以 NUL 分隔防拼接歧义。
// 同输入必同 ID——重复观察、计划步骤与实际调用因此收敛到同一节点。
func stateNodeID(kind NodeKind, normalized string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(kind))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(normalized))
	return string(kind) + ":" + strconv.FormatUint(h.Sum64(), 16)
}
