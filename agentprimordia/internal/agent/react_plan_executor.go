// react_plan_executor.go — R1.3 Planning 接入（G1-1）
// 在 ReAct Agent 入口处，对复杂任务先调用 planner 分解为子任务，
// 然后按依赖图（DAG）分层调度执行。每层内串行执行（同层并行留作后续优化）。
package agent

import (
	"context"
	"fmt"
	"time"

	"agentprimordia/internal/agent/planning"
	"agentprimordia/internal/persist"
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
//   - 子任务失败自动重试（默认 1 次）；重试耗尽仍失败则保存 failed checkpoint 并返回
//   - 每个子任务完成后保存带 Plan 进度的 checkpoint，支持断点续跑整个计划
//   - 最终 Content = 最后一个完成子任务的 Content（合并策略可后续扩展）
//
// 这是 R1.3 G1-1 闭环的接入点。调用方应先调用 getPlannerOrNil() 检查 planner
// 是否存在，并对 GeneratePlan 失败做优雅降级（直接返回错误或回退到正常 runLoop）。
func (a *ReActAgent) executePlan(ctx context.Context, history []Message, plan *planning.Plan, cfg loopConfig, startTime time.Time, totalLLMLatency time.Duration, totalToolLatency time.Duration, toolCount int) (*Response, error) {
	return a.executePlanWithState(ctx, history, plan, cfg, startTime, totalLLMLatency, totalToolLatency, toolCount, nil)
}

// defaultPlanSubtaskRetries 子任务默认重试次数（失败时额外尝试的次数）
const defaultPlanSubtaskRetries = 1

// planProgress Plan 执行的中间进度（checkpoint 与恢复共用）
type planProgress struct {
	subtasks  []planning.SubTask
	completed []string
	results   map[string]string
	tools     int
	llmL      time.Duration
	toolL     time.Duration
}

// runSubtaskWithRetry 带重试地执行子任务执行函数。
// 失败时最多额外重试 maxRetries 次（总计尝试 maxRetries+1 次）；ctx 取消时立即返回。
func runSubtaskWithRetry(ctx context.Context, run func() (*Response, error), maxRetries int) (*Response, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		resp, err := run()
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// runPlanSubtask 执行单个子任务：优先使用注入的执行器（测试/扩展），
// 否则走默认 executeSubTask；失败时按 defaultPlanSubtaskRetries 自动重试。
func (a *ReActAgent) runPlanSubtask(ctx context.Context, st planning.SubTask, history []Message, cfg loopConfig) (*Response, error) {
	run := func() (*Response, error) {
		if a.subtaskExecutor != nil {
			return a.subtaskExecutor(ctx, st, history, cfg)
		}
		return a.executeSubTask(ctx, st, history, cfg)
	}
	return runSubtaskWithRetry(ctx, run, defaultPlanSubtaskRetries)
}

// executePlanWithState 是 executePlan 的实现，支持从既有进度（initial）恢复。
// initial 为 nil 表示全新执行；恢复时跳过已完成子任务、沿用其结果。
func (a *ReActAgent) executePlanWithState(ctx context.Context, history []Message, plan *planning.Plan, cfg loopConfig, startTime time.Time, totalLLMLatency time.Duration, totalToolLatency time.Duration, toolCount int, initial *planProgress) (*Response, error) {
	if plan == nil || len(plan.SubTasks) == 0 {
		return nil, fmt.Errorf("empty plan")
	}
	if len(plan.SubTasks) == 1 && initial == nil {
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

	// 初始化进度：全新执行或从 checkpoint 恢复
	pp := &planProgress{subtasks: plan.SubTasks, results: make(map[string]string)}
	if initial != nil {
		pp.completed = append([]string(nil), initial.completed...)
		for k, v := range initial.results {
			pp.results[k] = v
		}
		pp.tools = initial.tools
		pp.llmL = initial.llmL
		pp.toolL = initial.toolL
	}
	done := make(map[string]bool, len(pp.completed))
	for _, id := range pp.completed {
		done[id] = true
	}

	var lastOutput string

	for _, layer := range layers {
		for _, st := range layer {
			// 恢复路径：跳过已完成子任务，沿用其输出
			if done[st.ID] {
				if r, ok := pp.results[st.ID]; ok {
					lastOutput = r
				}
				continue
			}

			a.emitStream(cfg, StreamEvent{Type: StreamEventThought, Content: fmt.Sprintf("[Plan] 执行子任务 %s: %s", st.ID, st.Description)})
			resp, err := a.runPlanSubtask(ctx, st, history, cfg)
			if err != nil {
				a.logger.Warn("Plan 子任务失败",
					"subtask_id", st.ID,
					"subtask_desc", st.Description,
					"error", err,
				)
				// 保存 failed checkpoint（含已完成进度），供断点续跑
				a.savePlanCheckpoint(ctx, history, pp, "failed")
				return nil, fmt.Errorf("subtask %s failed: %w", st.ID, err)
			}
			pp.completed = append(pp.completed, st.ID)
			if resp != nil {
				lastOutput = resp.Content
				pp.results[st.ID] = resp.Content
				pp.tools += resp.Metrics.TotalTools
				pp.llmL += resp.Metrics.LLMLatency
				pp.toolL += resp.Metrics.ToolLatency
			}
			a.savePlanCheckpoint(ctx, history, pp, "running")
		}
	}

	totalLLMLatency += pp.llmL
	totalToolLatency += pp.toolL
	toolCount += pp.tools

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

// savePlanCheckpoint 保存带 Plan 进度的 checkpoint（v3.4-1）。
// 与 saveCheckpoint 并存：saveCheckpoint 由 runLoop 每轮调用（无 plan 语义），
// 本方法由 executePlan 在子任务边界调用（含 plan/子任务进度）。
func (a *ReActAgent) savePlanCheckpoint(ctx context.Context, history []Message, pp *planProgress, status string) {
	cs := a.getCheckpointStore()
	if cs == nil {
		return
	}

	msgs := make([]persist.CheckpointMessage, len(history))
	for i, m := range history {
		msgs[i] = persist.CheckpointMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}

	cpSubtasks := make([]persist.CheckpointSubTask, 0, len(pp.subtasks))
	for _, st := range pp.subtasks {
		cpSubtasks = append(cpSubtasks, persist.CheckpointSubTask{
			ID:          st.ID,
			Description: st.Description,
			DependsOn:   st.DependsOn,
		})
	}

	state := &persist.AgentState{
		AgentID:   a.config.Name,
		SessionID: a.config.SessionID,
		Status:    status,
		Messages:  msgs,
		TurnCount: len(pp.completed),
		Metrics: persist.CheckpointMetrics{
			TotalTurns: len(pp.completed),
			TotalTools: pp.tools,
			LLMLatency: pp.llmL.String(),
			ToolLatency: pp.toolL.String(),
		},
		Plan: &persist.CheckpointPlan{
			Subtasks:     cpSubtasks,
			Completed:    pp.completed,
			Results:      pp.results,
			TotalTools:   pp.tools,
			LLMLatencyNs: pp.llmL.Nanoseconds(),
			ToolLatencyNs: pp.toolL.Nanoseconds(),
		},
		SavedAt: time.Now().UTC(),
	}

	if err := cs.Save(ctx, state); err != nil {
		a.logger.Warn("保存 plan 检查点失败", "error", err)
	}
}

// buildPlanProgressFromState 从 checkpoint 状态重建 plan 进度，供断点续跑使用。
func buildPlanProgressFromState(cp *persist.CheckpointPlan) *planProgress {
	if cp == nil {
		return nil
	}
	subtasks := make([]planning.SubTask, 0, len(cp.Subtasks))
	for _, s := range cp.Subtasks {
		subtasks = append(subtasks, planning.SubTask{
			ID:          s.ID,
			Description: s.Description,
			DependsOn:   s.DependsOn,
		})
	}
	return &planProgress{
		subtasks:  subtasks,
		completed: append([]string(nil), cp.Completed...),
		results:   cp.Results,
		tools:     cp.TotalTools,
		llmL:      time.Duration(cp.LLMLatencyNs),
		toolL:     time.Duration(cp.ToolLatencyNs),
	}
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
