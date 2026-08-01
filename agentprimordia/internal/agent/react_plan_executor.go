// react_plan_executor.go — R1.3 Planning 接入（G1-1）
// 在 ReAct Agent 入口处，对复杂任务先调用 planner 分解为子任务，
// 然后按依赖图（DAG）分层调度执行。每层内串行执行（同层并行留作后续优化）。
package agent

import (
	"context"
	"fmt"
	"time"

	"agentprimordia/internal/agent/planning"
)

// getPlannerOrNil 通过 capCache 获取 planner（nil-safe）
func (a *ReActAgent) getPlannerOrNil() planning.Planner {
	if a.capCache == nil {
		return a.getPlanner()
	}
	return a.capCache.planner
}

// extractUserInput 从 history 中取出最后一条 UserMessage 的内容
func extractUserInput(history []Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == RoleUser {
			return history[i].Content
		}
	}
	return ""
}

// executePlan 按依赖关系调度子任务，返回最终 Response。
//
// 行为契约：
//   - 仅有 1 个子任务时直接执行（视为普通任务，不走 Plan 分支）
//   - 拓扑排序后按层执行；同层子任务目前串行执行（同层并行是后续优化）
//   - 任一子任务失败立即返回（fast-fail）；后续子任务不再执行
//   - 最终 Content = 最后一个完成子任务的 Content（合并策略可后续扩展）
//
// 这是 R1.3 G1-1 闭环的接入点。调用方应先调用 getPlannerOrNil() 检查 planner
// 是否存在，并对 GeneratePlan 失败做优雅降级（直接返回错误或回退到正常 runLoop）。
func (a *ReActAgent) executePlan(ctx context.Context, history []Message, plan *planning.Plan, cfg loopConfig, startTime time.Time, totalLLMLatency time.Duration, totalToolLatency time.Duration, toolCount int) (*Response, error) {
	if plan == nil || len(plan.SubTasks) == 0 {
		return nil, fmt.Errorf("empty plan")
	}
	if len(plan.SubTasks) == 1 {
		// 单子任务视为普通任务：复用 runLoop
		return a.executeSubTask(ctx, plan.SubTasks[0], history, cfg)
	}

	// 构建 DAG 并拓扑排序
	graph := buildDependencyGraph(plan.SubTasks)
	layers, err := graph.topologicalLayers()
	if err != nil {
		return nil, fmt.Errorf("plan DAG invalid: %w", err)
	}

	a.logger.Info("Plan 执行开始",
		"name", a.config.Name,
		"subtasks", len(plan.SubTasks),
		"layers", len(layers),
	)

	var lastOutput string
	var executedTools int
	var planLLMLatency time.Duration
	var planToolLatency time.Duration

	for layerIdx, layer := range layers {
		for _, st := range layer {
			a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: fmt.Sprintf("[Plan] 执行子任务 %s: %s", st.ID, st.Description)})
			resp, err := a.executeSubTask(ctx, st, history, cfg)
			if err != nil {
				a.logger.Warn("Plan 子任务失败",
					"subtask_id", st.ID,
					"subtask_desc", st.Description,
					"error", err,
				)
				return nil, fmt.Errorf("subtask %s failed: %w", st.ID, err)
			}
			lastOutput = resp.Content
			if resp != nil && resp.Metrics.TotalTools > 0 {
				executedTools += resp.Metrics.TotalTools
				planLLMLatency += resp.Metrics.LLMLatency
				planToolLatency += resp.Metrics.ToolLatency
			}
		}
		_ = layerIdx // 当前未使用，但保留供后续按层粒度做并行优化
	}

	totalLLMLatency += planLLMLatency
	totalToolLatency += planToolLatency
	toolCount += executedTools

	return &Response{
		RequestID: cfg.requestID,
		Content:   lastOutput,
		Metrics: Metrics{
			TotalTurns:  len(plan.SubTasks),
			TotalTools:  toolCount,
			Duration:    time.Since(startTime),
			LLMLatency:  totalLLMLatency,
			ToolLatency: totalToolLatency,
		},
	}, nil
}

// executeSubTask 执行单个子任务（复用 runLoop 的核心逻辑）
//
// 实现：把子任务描述追加为新的 UserMessage，然后递归调用 runLoop。
// 这样每个子任务都可以独立调用tool、产生 ReAct 循环。
func (a *ReActAgent) executeSubTask(ctx context.Context, task planning.SubTask, history []Message, cfg loopConfig) (*Response, error) {
	taskHistory := make([]Message, 0, len(history)+1)
	taskHistory = append(taskHistory, history...)
	taskHistory = append(taskHistory, UserMessage(task.Description))
	return a.runLoop(ctx, taskHistory, 0, cfg, 0, 0, 0)
}

// ===== 简单 DAG 实现 =====
// 内部使用 map[string][]string 表示依赖关系；
// 提供拓扑分层（Kahn 算法），输出每层可独立执行的子任务集合。

// depGraph 简单依赖图
type depGraph struct {
	nodes    map[string]planning.SubTask
	inEdges  map[string]int      // 节点的入度（依赖数）
	outEdges map[string][]string // 节点 → 它解锁的下游
	allIDs   []string
}

// buildDependencyGraph 从 SubTask 列表构建依赖图
func buildDependencyGraph(tasks []planning.SubTask) *depGraph {
	g := &depGraph{
		nodes:    make(map[string]planning.SubTask, len(tasks)),
		inEdges:  make(map[string]int, len(tasks)),
		outEdges: make(map[string][]string, len(tasks)),
		allIDs:   make([]string, 0, len(tasks)),
	}
	// 初始化
	for _, t := range tasks {
		g.nodes[t.ID] = t
		g.inEdges[t.ID] = 0
		g.allIDs = append(g.allIDs, t.ID)
	}
	// 构建依赖边（注意：ID 不存在时容错，跳过该依赖）
	for _, t := range tasks {
		for _, dep := range t.DependsOn {
			if _, ok := g.nodes[dep]; !ok {
				continue // 跳过悬空依赖
			}
			g.outEdges[dep] = append(g.outEdges[dep], t.ID)
			g.inEdges[t.ID]++
		}
	}
	return g
}

// topologicalLayers 用 Kahn 算法做拓扑分层。
// 返回各层子任务列表；同层任务可独立执行（无依赖关系）。
// 当存在环时返回错误。
func (g *depGraph) topologicalLayers() ([][]planning.SubTask, error) {
	// 复制入度用于就地修改
	indeg := make(map[string]int, len(g.inEdges))
	for k, v := range g.inEdges {
		indeg[k] = v
	}

	var layers [][]planning.SubTask
	processed := 0
	for processed < len(g.allIDs) {
		var layer []planning.SubTask
		for _, id := range g.allIDs {
			if indeg[id] == 0 {
				layer = append(layer, g.nodes[id])
			}
		}
		if len(layer) == 0 {
			return nil, fmt.Errorf("cycle detected: %d unprocessed nodes", len(g.allIDs)-processed)
		}
		layers = append(layers, layer)
		// 标记当前层节点为已处理：把它们入度设为 -1（已访问）
		for _, t := range layer {
			indeg[t.ID] = -1
			processed++
		}
		// 减少下游入度
		for _, t := range layer {
			for _, downstream := range g.outEdges[t.ID] {
				if indeg[downstream] > 0 {
					indeg[downstream]--
				}
			}
		}
	}
	return layers, nil
}
