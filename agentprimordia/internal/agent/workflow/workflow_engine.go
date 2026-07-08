// workflow_engine.go — 工作流调度器：Execute 入口 + 5 个 execute* 调度方法
// （Linear / Conditional / Loop / ParallelForkJoin / StateMachine）
//
// 每个 execute* 方法负责按其工作流类型遍历节点并执行，最终写入 w.result。
package workflow

import (
	"context"
	"fmt"
	"time"
)

// Execute 执行工作流
func (w *WorkflowExecution) Execute(initialInput map[string]any) (*WorkflowResult, error) {
	w.mu.Lock()
	w.status = WfStatusRunning
	w.result = &WorkflowResult{
		StartTime: time.Now(),
		Output:    make(map[string]any),
		Variables: make(map[string]any),
		Records:   make([]*ExecutionRecord, 0),
		Metrics:   &WorkflowMetrics{},
		PathTaken: make([]string, 0),
	}
	w.mu.Unlock()

	w.emitEvent("workflow_started", "", map[string]any{
		"type": w.config.Type,
		"name": w.config.Name,
	})

	ctx, cancel := context.WithTimeout(w.executionCtx, w.config.Timeout)
	defer cancel()

	var err error
	switch w.config.Type {
	case LinearWorkflow:
		err = w.executeLinear(ctx, initialInput)
	case ConditionalWorkflow:
		err = w.executeConditional(ctx, initialInput)
	case LoopWorkflow:
		err = w.executeLoop(ctx, initialInput)
	case ParallelForkJoin:
		err = w.executeParallelForkJoin(ctx, initialInput)
	case StateMachine:
		err = w.executeStateMachine(ctx, initialInput)
	default:
		err = fmt.Errorf("unsupported workflow type: %s", w.config.Type)
	}

	now := time.Now()
	w.mu.Lock()
	if err != nil {
		w.status = WfStatusFailed
		w.result.Error = err
	} else if ctx.Err() == context.DeadlineExceeded {
		w.status = WfStatusFailed
		w.result.Error = fmt.Errorf("workflow timeout")
	} else {
		w.status = WfStatusCompleted
	}
	w.result.Status = w.status
	w.result.EndTime = now
	w.result.Duration = now.Sub(w.result.StartTime)
	varCopy := make(map[string]any)
	for k, v := range w.variables {
		varCopy[k] = v
	}
	w.result.Variables = varCopy
	w.mu.Unlock()

	w.emitEvent("workflow_completed", "", map[string]any{
		"status":   w.status,
		"duration": w.result.Duration,
	})

	return w.result, err
}

// executeLinear 线性工作流：按转换顺序依次执行节点
func (w *WorkflowExecution) executeLinear(ctx context.Context, input map[string]any) error {
	currentID := w.startNodeID
	currentInput := input

	for currentID != "" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		node, exists := w.nodes[currentID]
		if !exists {
			return fmt.Errorf("node not found: %s", currentID)
		}

		output, err := w.executeNode(ctx, node, currentInput)
		if err != nil {
			return fmt.Errorf("node %s execution error: %w", node.ID, err)
		}

		currentInput = output

		// 检查暂停请求
		if err := w.checkPause(ctx); err != nil {
			return err
		}

		transitions := w.transitions[currentID]
		if len(transitions) == 0 {
			break
		}

		nextID := ""
		for _, trans := range transitions {
			if w.evaluateTransitionCondition(trans, output) {
				nextID = trans.To
				break
			}
		}

		currentID = nextID
	}

	return nil
}

// executeConditional 条件分支工作流：每节点检查 node.Condition，按转换条件选择 next
func (w *WorkflowExecution) executeConditional(ctx context.Context, input map[string]any) error {
	currentID := w.startNodeID
	currentInput := input

	for currentID != "" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		node, exists := w.nodes[currentID]
		if !exists {
			return fmt.Errorf("node not found: %s", currentID)
		}

		if node.Condition != nil && !w.evaluateNodeCondition(node.Condition, currentInput) {
			w.recordExecution(node, currentInput, nil, NodeSkipped, 0)
			w.emitEvent("node_skipped", node.ID, nil)
		} else {
			output, err := w.executeNode(ctx, node, currentInput)
			if err != nil {
				if w.config.ErrorHandling.ContinueOnError {
					w.emitEvent("node_error_continued", node.ID, map[string]any{"error": err.Error()})
					output = currentInput
				} else {
					return err
				}
			}
			currentInput = output
		}

		transitions := w.transitions[currentID]
		if len(transitions) == 0 {
			break
		}

		nextID := ""
		for _, trans := range transitions {
			if trans.Condition == nil || trans.Condition.Type == "always" ||
				w.evaluateTransitionCondition(trans, currentInput) {
				nextID = trans.To
				w.result.Metrics.BranchesTaken.Add(1)
				break
			}
		}

		currentID = nextID
	}

	return nil
}

// executeLoop 循环工作流：节点带 LoopEndNode 时按 node.Condition 决定是否结束
func (w *WorkflowExecution) executeLoop(ctx context.Context, input map[string]any) error {
	currentID := w.startNodeID
	currentInput := input
	loopCount := 0

	for currentID != "" && loopCount < w.config.MaxIterations {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		loopCount++
		w.iterationCount = loopCount

		node, exists := w.nodes[currentID]
		if !exists {
			return fmt.Errorf("node not found: %s", currentID)
		}

		if node.Type == LoopEndNode {
			if node.Condition != nil && !w.evaluateNodeCondition(node.Condition, currentInput) {
				break
			}
		}

		output, err := w.executeNode(ctx, node, currentInput)
		if err != nil {
			return fmt.Errorf("loop iteration %d error: %w", loopCount, err)
		}

		currentInput = output

		w.SetVariable("_iteration", loopCount)
		w.SetVariable("_loop_input", currentInput)

		transitions := w.transitions[currentID]
		if len(transitions) == 0 {
			break
		}

		nextID := ""
		for _, trans := range transitions {
			if w.evaluateTransitionCondition(trans, currentInput) {
				nextID = trans.To
				break
			}
		}

		currentID = nextID
	}

	w.result.Metrics.Iterations.Store(int64(loopCount))
	return nil
}

// executeParallelForkJoin 并行分叉合并：遇到 ParallelNode 时调用 executeParallelNode
func (w *WorkflowExecution) executeParallelForkJoin(ctx context.Context, input map[string]any) error {
	currentID := w.startNodeID
	currentInput := input

	for currentID != "" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		node, exists := w.nodes[currentID]
		if !exists {
			return fmt.Errorf("node not found: %s", currentID)
		}

		if node.Type == ParallelNode {
			output, err := w.executeParallelNode(ctx, node, currentInput)
			if err != nil {
				return err
			}
			currentInput = output

			childTransitions := w.transitions[node.ID]
			if len(childTransitions) == 0 {
				break
			}

			downstreamSet := make(map[string]bool)
			for _, trans := range childTransitions {
				childTrans := w.transitions[trans.To]
				for _, ct := range childTrans {
					downstreamSet[ct.To] = true
				}
			}

			if len(downstreamSet) == 1 {
				for nextID := range downstreamSet {
					currentID = nextID
				}
			} else if len(downstreamSet) > 1 {
				for nextID := range downstreamSet {
					currentID = nextID
					break
				}
			} else {
				break
			}
		} else {
			output, err := w.executeNode(ctx, node, currentInput)
			if err != nil {
				return err
			}
			currentInput = output

			transitions := w.transitions[currentID]
			if len(transitions) == 0 {
				break
			}

			nextID := ""
			for _, trans := range transitions {
				if w.evaluateTransitionCondition(trans, currentInput) {
					nextID = trans.To
					break
				}
			}

			currentID = nextID
		}
	}

	return nil
}

// executeStateMachine 状态机：每个状态对应一个节点，按 transition 选择 next state
func (w *WorkflowExecution) executeStateMachine(ctx context.Context, input map[string]any) error {
	currentState := w.startNodeID
	currentInput := input

	maxIter := w.config.MaxIterations
	if maxIter <= 0 {
		maxIter = defaultMaxIterations
	}
	iterations := 0

	for currentState != "" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		iterations++
		if iterations > maxIter {
			return fmt.Errorf("state machine exceeded max iterations (%d), possible infinite loop at state %q", maxIter, currentState)
		}

		node, exists := w.nodes[currentState]
		if !exists {
			return fmt.Errorf("state not found: %s", currentState)
		}

		output, err := w.executeNode(ctx, node, currentInput)
		if err != nil {
			return fmt.Errorf("state %s error: %w", currentState, err)
		}

		currentInput = output
		w.SetVariable("_current_state", currentState)

		// 检查暂停请求
		if err := w.checkPause(ctx); err != nil {
			return err
		}

		transitions := w.transitions[currentState]
		if len(transitions) == 0 {
			break
		}

		nextState := ""
		for _, trans := range transitions {
			if w.evaluateTransitionCondition(trans, output) {
				nextState = trans.To
				w.emitEvent("state_transition", currentState, map[string]any{
					"from": currentState,
					"to":   nextState,
				})
				break
			}
		}

		currentState = nextState
	}

	return nil
}
