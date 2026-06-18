package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultMaxIterations      = 100
	defaultWorkflowTimeout    = 10 * time.Minute
	defaultWorkflowMaxRetries = 3
	workflowEventBufferSize   = 200
)

// WorkflowType 工作流类型
type WorkflowType string

const (
	// LinearWorkflow 线性工作流
	LinearWorkflow WorkflowType = "linear"
	// ConditionalWorkflow 条件分支工作流
	ConditionalWorkflow WorkflowType = "conditional"
	// LoopWorkflow 循环工作流
	LoopWorkflow WorkflowType = "loop"
	// ParallelForkJoin 并行分叉合并工作流
	ParallelForkJoin WorkflowType = "parallel_fork_join"
	// StateMachine 状态机工作流
	StateMachine WorkflowType = "state_machine"
)

// WorkflowStatus 工作流状态
type WorkflowStatus string

const (
	WfStatusPending   WorkflowStatus = "pending"
	WfStatusRunning   WorkflowStatus = "running"
	WfStatusPaused    WorkflowStatus = "paused"
	WfStatusCompleted WorkflowStatus = "completed"
	WfStatusFailed    WorkflowStatus = "failed"
	WfStatusCancelled WorkflowStatus = "cancelled"
)

// WfRetryPolicy is the workflow-level retry policy referenced by WorkflowConfig.
// It is distinct from DAG RetryPolicy in dag.go; workflow retries are governed
// at execution time by ErrorHandling.MaxRetries.
type WfRetryPolicy struct {
	MaxAttempts     int           `json:"max_attempts"`
	BackoffInterval time.Duration `json:"backoff_interval"`
}

// WorkflowConfig 工作流配置
type WorkflowConfig struct {
	Type          WorkflowType   `json:"type"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	MaxIterations int            `json:"max_iterations"`
	Timeout       time.Duration  `json:"timeout"`
	RetryPolicy   *WfRetryPolicy `json:"retry_policy,omitempty"`
	ErrorHandling ErrorHandling  `json:"error_handling"`
	EnableLogging bool           `json:"enable_logging"`
	SaveSnapshot  bool           `json:"save_snapshot"`
}

// ErrorHandling 错误处理策略
type ErrorHandling struct {
	OnError         string `json:"on_error"`
	MaxRetries      int    `json:"max_retries"`
	FallbackStep    string `json:"fallback_step,omitempty"`
	ContinueOnError bool   `json:"continue_on_error"`
}

// WorkflowNode 工作流节点
type WorkflowNode struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Type          NodeType          `json:"type"`
	Agent         Agent             `json:"-"`
	Config        *NodeConfig       `json:"config,omitempty"`
	Condition     *NodeCondition    `json:"condition,omitempty"`
	Transitions   []*Transition     `json:"transitions,omitempty"`
	InputMapping  map[string]string `json:"input_mapping,omitempty"`
	OutputMapping map[string]string `json:"output_mapping,omitempty"`
	Metadata      map[string]any    `json:"metadata,omitempty"`
}

// NodeType 节点类型
type NodeType string

const (
	TaskNode      NodeType = "task"
	ConditionNode NodeType = "condition"
	ParallelNode  NodeType = "parallel"
	LoopStartNode NodeType = "loop_start"
	LoopEndNode   NodeType = "loop_end"
	FallbackNode  NodeType = "fallback"
	SubWfNode     NodeType = "sub_workflow"
)

// NodeConfig 节点配置
type NodeConfig struct {
	PromptTemplate string                                                                  `json:"prompt_template,omitempty"`
	Parameters     map[string]any                                                          `json:"parameters,omitempty"`
	Timeout        time.Duration                                                           `json:"timeout,omitempty"`
	RetryCount     int                                                                     `json:"retry_count,omitempty"`
	CustomLogic    func(ctx context.Context, input map[string]any) (map[string]any, error) `json:"-"`
}

// NodeCondition 节点执行条件
type NodeCondition struct {
	Type       string                    `json:"type"`
	Expression string                    `json:"expression,omitempty"`
	CustomFunc func(map[string]any) bool `json:"-"`
	Field      string                    `json:"field,omitempty"`
	Operator   string                    `json:"operator,omitempty"`
	Value      any                       `json:"value,omitempty"`
}

// Transition 转换/边
type Transition struct {
	ID        string               `json:"id"`
	From      string               `json:"from"`
	To        string               `json:"to"`
	Condition *TransitionCondition `json:"condition,omitempty"`
	Weight    float64              `json:"weight"`
}

// TransitionCondition 转换条件
type TransitionCondition struct {
	Type        string  `json:"type"`
	Expression  string  `json:"expression,omitempty"`
	Probability float64 `json:"probability,omitempty"`
	Field       string  `json:"field,omitempty"`
	Operator    string  `json:"operator,omitempty"`
	Value       any     `json:"value,omitempty"`
}

// WorkflowExecution 工作流执行实例
type WorkflowExecution struct {
	mu             sync.RWMutex
	config         WorkflowConfig
	nodes          map[string]*WorkflowNode
	transitions    map[string][]*Transition
	startNodeID    string
	endNodeIDs     []string
	currentNode    *WorkflowNode
	variables      map[string]any
	history        []*ExecutionRecord
	status         WorkflowStatus
	result         *WorkflowResult
	eventCh        chan *WorkflowEvent
	executionCtx   context.Context
	cancelFunc     context.CancelFunc
	pauseCh        chan struct{} // 关闭时恢复执行
	iterationCount int
	nodeExecutions map[string]int
}

// ExecutionRecord 执行记录
type ExecutionRecord struct {
	NodeID    string              `json:"node_id"`
	NodeName  string              `json:"node_name"`
	Status    NodeExecutionStatus `json:"status"`
	Input     map[string]any      `json:"input,omitempty"`
	Output    map[string]any      `json:"output,omitempty"`
	Error     error               `json:"error,omitempty"`
	Duration  time.Duration       `json:"duration"`
	Timestamp time.Time           `json:"timestamp"`
	Iteration int                 `json:"iteration"`
}

// NodeExecutionStatus 节点执行状态
type NodeExecutionStatus string

const (
	NodePending   NodeExecutionStatus = "pending"
	NodeRunning   NodeExecutionStatus = "running"
	NodeCompleted NodeExecutionStatus = "completed"
	NodeSkipped   NodeExecutionStatus = "skipped"
	NodeFailed    NodeExecutionStatus = "failed"
)

// WorkflowResult 工作流结果
type WorkflowResult struct {
	ExecutionID string             `json:"execution_id"`
	Status      WorkflowStatus     `json:"status"`
	Output      map[string]any     `json:"output,omitempty"`
	Variables   map[string]any     `json:"variables,omitempty"`
	Records     []*ExecutionRecord `json:"records,omitempty"`
	Metrics     *WorkflowMetrics   `json:"metrics,omitempty"`
	Error       error              `json:"error,omitempty"`
	StartTime   time.Time          `json:"start_time"`
	EndTime     time.Time          `json:"end_time"`
	Duration    time.Duration      `json:"duration"`
	PathTaken   []string           `json:"path_taken,omitempty"`
}

// WorkflowMetrics 工作流指标
// perf-v6 Task 9：所有 int 计数器改 atomic.Int64（无锁累加），Duration 字段保留
type WorkflowMetrics struct {
	TotalNodes       atomic.Int64  `json:"total_nodes"`
	ExecutedNodes    atomic.Int64  `json:"executed_nodes"`
	FailedNodes      atomic.Int64  `json:"failed_nodes"`
	SkippedNodes     atomic.Int64  `json:"skipped_nodes"`
	TotalDurationNs  atomic.Int64  `json:"-"`
	AvgNodeDuration  time.Duration `json:"avg_node_duration"`
	Iterations       atomic.Int64  `json:"iterations"`
	BranchesTaken    atomic.Int64  `json:"branches_taken"`
	RetriesAttempted atomic.Int64  `json:"retries_attempted"`
}

// WorkflowEvent 工作流事件
type WorkflowEvent struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	NodeID    string    `json:"node_id,omitempty"`
	Data      any       `json:"data,omitempty"`
}

// NewWorkflowExecution 创建新的工作流执行实例
func NewWorkflowExecution(config WorkflowConfig) *WorkflowExecution {
	if config.MaxIterations <= 0 {
		config.MaxIterations = defaultMaxIterations
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultWorkflowTimeout
	}
	if config.ErrorHandling.OnError == "" {
		config.ErrorHandling.OnError = "fail"
	}
	if config.ErrorHandling.MaxRetries <= 0 {
		config.ErrorHandling.MaxRetries = defaultWorkflowMaxRetries
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &WorkflowExecution{
		config:         config,
		nodes:          make(map[string]*WorkflowNode),
		transitions:    make(map[string][]*Transition),
		variables:      make(map[string]any),
		history:        make([]*ExecutionRecord, 0),
		status:         WfStatusPending,
		eventCh:        make(chan *WorkflowEvent, workflowEventBufferSize),
		executionCtx:   ctx,
		cancelFunc:     cancel,
		pauseCh:        make(chan struct{}, 1),
		nodeExecutions: make(map[string]int),
	}
}

// AddNode 添加节点
func (w *WorkflowExecution) AddNode(node *WorkflowNode) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if node.ID == "" || (node.Agent == nil && node.Type == TaskNode) {
		return fmt.Errorf("invalid node")
	}

	w.nodes[node.ID] = node
	return nil
}

// AddTransition 添加转换
func (w *WorkflowExecution) AddTransition(transition *Transition) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if transition.From == "" || transition.To == "" {
		return fmt.Errorf("invalid transition")
	}

	w.transitions[transition.From] = append(w.transitions[transition.From], transition)
	return nil
}

// SetStartNode 设置起始节点
func (w *WorkflowExecution) SetStartNode(nodeID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, exists := w.nodes[nodeID]; !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	w.startNodeID = nodeID
	return nil
}

// SetVariable 设置变量
func (w *WorkflowExecution) SetVariable(key string, value any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.variables[key] = value
}

// GetVariable 获取变量
func (w *WorkflowExecution) GetVariable(key string) (any, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	val, exists := w.variables[key]
	return val, exists
}

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

func (w *WorkflowExecution) executeNode(ctx context.Context, node *WorkflowNode, input map[string]any) (map[string]any, error) {
	startTime := time.Now()
	w.mu.Lock()
	w.currentNode = node
	w.mu.Unlock()
	w.recordPath(node.ID)

	w.emitEvent("node_started", node.ID, map[string]any{
		"type": node.Type,
		"name": node.Name,
	})

	mappedInput := w.applyInputMapping(input, node.InputMapping)

	var output map[string]any
	var err error

	switch node.Type {
	case TaskNode:
		output, err = w.executeTaskNode(ctx, node, mappedInput)
	case ConditionNode:
		output, err = w.executeConditionNode(ctx, node, mappedInput)
	case FallbackNode:
		output, err = w.executeFallbackNode(ctx, node, mappedInput)
	default:
		output, err = w.executeTaskNode(ctx, node, mappedInput)
	}

	duration := time.Since(startTime)
	status := NodeCompleted
	if err != nil {
		status = NodeFailed

		switch w.config.ErrorHandling.OnError {
		case "retry":
			output, err = w.retryNodeExecution(ctx, node, mappedInput)
		case "skip":
			status = NodeSkipped
			err = nil
			output = input
		case "fallback":
			if w.config.ErrorHandling.FallbackStep != "" {
				fallbackNode, exists := w.nodes[w.config.ErrorHandling.FallbackStep]
				if exists {
					output, err = w.executeNode(ctx, fallbackNode, input)
				}
			}
		}
	}

	w.nodeExecutions[node.ID]++

	record := &ExecutionRecord{
		NodeID:    node.ID,
		NodeName:  node.Name,
		Status:    status,
		Input:     mappedInput,
		Output:    output,
		Error:     err,
		Duration:  duration,
		Timestamp: time.Now(),
		Iteration: w.iterationCount,
	}

	w.addToHistory(record)

	finalOutput := w.applyOutputMapping(output, node.OutputMapping)

	w.mu.Lock()
	if w.result != nil && finalOutput != nil {
		if len(node.OutputMapping) > 0 {
			for k, v := range finalOutput {
				w.result.Output[k] = v
			}
		} else {
			for k, v := range finalOutput {
				w.result.Output[node.ID+"_"+k] = v
			}
		}
	}
	w.mu.Unlock()

	w.emitEvent("node_completed", node.ID, map[string]any{
		"status":   status,
		"duration": duration,
	})

	return finalOutput, err
}

func (w *WorkflowExecution) executeTaskNode(ctx context.Context, node *WorkflowNode, input map[string]any) (map[string]any, error) {
	if node.Config != nil && node.Config.CustomLogic != nil {
		return node.Config.CustomLogic(ctx, input)
	}

	prompt := w.buildPrompt(node, input)
	resp, err := node.Agent.Run(ctx, UserMessage(prompt))
	if err != nil {
		return nil, err
	}

	output := make(map[string]any)
	output["content"] = resp.Content
	output["turns"] = resp.Metrics.TotalTurns

	var parsed map[string]any
	if err := json.Unmarshal([]byte(resp.Content), &parsed); err == nil {
		for k, v := range parsed {
			output[k] = v
		}
	}

	return output, nil
}

func (w *WorkflowExecution) executeConditionNode(ctx context.Context, node *WorkflowNode, input map[string]any) (map[string]any, error) {
	result := false

	if node.Condition != nil {
		result = w.evaluateNodeCondition(node.Condition, input)
	}

	output := make(map[string]any)
	output["condition_result"] = result
	output["matched"] = result

	return output, nil
}

func (w *WorkflowExecution) executeFallbackNode(ctx context.Context, node *WorkflowNode, input map[string]any) (map[string]any, error) {
	w.emitEvent("fallback_executed", node.ID, nil)
	return w.executeTaskNode(ctx, node, input)
}

func (w *WorkflowExecution) executeParallelNode(ctx context.Context, node *WorkflowNode, input map[string]any) (map[string]any, error) {
	childTransitions := w.transitions[node.ID]
	if len(childTransitions) == 0 {
		return input, nil
	}

	var wg sync.WaitGroup
	resultsCh := make(chan *parallelResult, len(childTransitions))

	for _, trans := range childTransitions {
		childNode, exists := w.nodes[trans.To]
		if !exists {
			continue
		}

		wg.Add(1)
		go func(cn *WorkflowNode) {
			defer wg.Done()

			output, err := w.executeNode(ctx, cn, input)
			resultsCh <- &parallelResult{
				nodeID: cn.ID,
				output: output,
				error:  err,
			}
		}(childNode)
	}

	wg.Wait()
	close(resultsCh)

	mergedOutput := make(map[string]any)
	for result := range resultsCh {
		if result.error != nil {
			mergedOutput[result.nodeID+"_error"] = result.error.Error()
		}
		if result.output != nil {
			for k, v := range result.output {
				mergedOutput[result.nodeID+"_"+k] = v
			}
		}
	}

	return mergedOutput, nil
}

type parallelResult struct {
	nodeID string
	output map[string]any
	error  error
}

func (w *WorkflowExecution) retryNodeExecution(ctx context.Context, node *WorkflowNode, input map[string]any) (map[string]any, error) {
	maxRetries := w.config.ErrorHandling.MaxRetries
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		w.emitEvent("node_retry", node.ID, map[string]any{"attempt": i + 1})

		output, err := w.executeTaskNode(ctx, node, input)
		if err == nil {
			w.result.Metrics.RetriesAttempted.Add(int64(i + 1))
			return output, nil
		}
		lastErr = err

		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}

	w.result.Metrics.RetriesAttempted.Add(int64(maxRetries))
	return nil, lastErr
}

func (w *WorkflowExecution) evaluateNodeCondition(condition *NodeCondition, input map[string]any) bool {
	if condition.CustomFunc != nil {
		return condition.CustomFunc(input)
	}

	fieldVal, exists := input[condition.Field]
	if !exists {
		return false
	}

	return compareValues(fieldVal, condition.Operator, condition.Value)
}

func (w *WorkflowExecution) evaluateTransitionCondition(transition *Transition, input map[string]any) bool {
	if transition.Condition == nil || transition.Condition.Type == "always" {
		return true
	}

	if transition.Condition.Type == "probability" {
		threshold := transition.Condition.Probability
		if threshold <= 0 {
			threshold = 0.5
		}
		if threshold > 1 {
			threshold = 1
		}
		return rand.Float64() < threshold
	}

	if transition.Condition.Field != "" {
		fieldVal, exists := input[transition.Condition.Field]
		if !exists {
			return false
		}
		return compareValues(fieldVal, transition.Condition.Operator, transition.Condition.Value)
	}

	return true
}

func compareValues(left any, operator string, right any) bool {
	switch operator {
	case "==":
		return fmt.Sprintf("%v", left) == fmt.Sprintf("%v", right)
	case "!=":
		return fmt.Sprintf("%v", left) != fmt.Sprintf("%v", right)
	case ">":
		return toFloat64(left) > toFloat64(right)
	case "<":
		return toFloat64(left) < toFloat64(right)
	case ">=":
		return toFloat64(left) >= toFloat64(right)
	case "<=":
		return toFloat64(left) <= toFloat64(right)
	default:
		return true
	}
}

func toFloat64(v any) float64 {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case float64:
		return val
	case float32:
		return float64(val)
	default:
		return 0.0
	}
}

func (w *WorkflowExecution) buildPrompt(node *WorkflowNode, input map[string]any) string {
	if node.Config != nil && node.Config.PromptTemplate != "" {
		return renderTemplate(node.Config.PromptTemplate, input)
	}

	promptParts := []string{fmt.Sprintf("[%s]", node.Name)}

	if input != nil {
		inputJSON, _ := json.MarshalIndent(input, "", "  ")
		promptParts = append(promptParts, fmt.Sprintf("\n输入数据:\n```json\n%s\n```", string(inputJSON)))
	}

	promptParts = append(promptParts, "\n请处理以上输入并返回结果。")

	return strings.Join(promptParts, "\n")
}

func (w *WorkflowExecution) applyInputMapping(input map[string]any, mapping map[string]string) map[string]any {
	if len(mapping) == 0 {
		return input
	}

	result := make(map[string]any)
	for key, value := range input {
		result[key] = value
	}

	for newKey, sourceKey := range mapping {
		if val, exists := input[sourceKey]; exists {
			result[newKey] = val
		}
	}

	return result
}

func (w *WorkflowExecution) applyOutputMapping(output map[string]any, mapping map[string]string) map[string]any {
	if len(mapping) == 0 || output == nil {
		return output
	}

	result := make(map[string]any)
	for key, value := range output {
		result[key] = value
	}

	for newKey, sourceKey := range mapping {
		if val, exists := output[sourceKey]; exists {
			result[newKey] = val
		}
	}

	return result
}

func (w *WorkflowExecution) recordExecution(node *WorkflowNode, input, output map[string]any, status NodeExecutionStatus, iteration int) {
	record := &ExecutionRecord{
		NodeID:    node.ID,
		NodeName:  node.Name,
		Status:    status,
		Input:     input,
		Output:    output,
		Timestamp: time.Now(),
		Iteration: iteration,
	}
	w.addToHistory(record)
}

func (w *WorkflowExecution) addToHistory(record *ExecutionRecord) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.history = append(w.history, record)
	if w.result != nil {
		w.result.Records = append(w.result.Records, record)
	}
	w.updateMetrics(record)
}

func (w *WorkflowExecution) updateMetrics(record *ExecutionRecord) {
	if w.result == nil || w.result.Metrics == nil {
		return
	}

	metrics := w.result.Metrics
	metrics.TotalNodes.Add(1)

	switch record.Status {
	case NodeCompleted:
		metrics.ExecutedNodes.Add(1)
		metrics.TotalDurationNs.Add(int64(record.Duration))
	case NodeFailed:
		metrics.FailedNodes.Add(1)
	case NodeSkipped:
		metrics.SkippedNodes.Add(1)
	}

	executed := metrics.ExecutedNodes.Load()
	if executed > 0 {
		metrics.AvgNodeDuration = time.Duration(metrics.TotalDurationNs.Load() / executed)
	}
}

func (w *WorkflowExecution) recordPath(nodeID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.result != nil {
		w.result.PathTaken = append(w.result.PathTaken, nodeID)
	}
}

func (w *WorkflowExecution) emitEvent(eventType, nodeID string, data any) {
	select {
	case w.eventCh <- &WorkflowEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		NodeID:    nodeID,
		Data:      data,
	}:
	default:
	}
}

// Events 返回事件通道
func (w *WorkflowExecution) Events() <-chan *WorkflowEvent {
	return w.eventCh
}

// GetResult 获取结果
func (w *WorkflowExecution) GetResult() *WorkflowResult {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.result
}

// GetStatus 获取状态
func (w *WorkflowExecution) GetStatus() WorkflowStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.status
}

// checkPause 检查是否暂停并等待恢复或取消
func (w *WorkflowExecution) checkPause(ctx context.Context) error {
	w.mu.RLock()
	if w.status != WfStatusPaused {
		w.mu.RUnlock()
		return nil
	}
	pauseCh := w.pauseCh
	w.mu.RUnlock()

	select {
	case <-pauseCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Pause 暂停执行（使用 pauseCh 阻塞而非取消 context，确保可恢复）
func (w *WorkflowExecution) Pause() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == WfStatusRunning {
		w.status = WfStatusPaused
		// 不取消 executionCtx，仅通过 pauseCh 通知执行循环暂停
		if w.pauseCh != nil {
			select {
			case w.pauseCh <- struct{}{}:
			default:
			}
		}
		w.emitEvent("workflow_paused", "", nil)
	}
}

// Resume 恢复执行
func (w *WorkflowExecution) Resume() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status != WfStatusPaused {
		return fmt.Errorf("workflow is not paused")
	}

	// 关闭 pauseCh 解除执行循环的阻塞，然后重建以支持后续暂停
	if w.pauseCh != nil {
		close(w.pauseCh)
	}
	w.pauseCh = make(chan struct{}, 1)
	w.status = WfStatusRunning
	w.emitEvent("workflow_resumed", "", nil)
	return nil
}

// Cancel 取消执行
func (w *WorkflowExecution) Cancel() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == WfStatusRunning || w.status == WfStatusPaused {
		w.status = WfStatusCancelled
		w.cancelFunc()
		w.emitEvent("workflow_cancelled", "", nil)
	}
}

// Export 导出为JSON
func (w *WorkflowExecution) Export() ([]byte, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	data := map[string]any{
		"config":      w.config,
		"nodes":       w.nodes,
		"transitions": w.transitions,
		"variables":   w.variables,
		"status":      w.status,
		"result":      w.result,
	}
	return json.MarshalIndent(data, "", "  ")
}

// GetHistory 获取执行历史
func (w *WorkflowExecution) GetHistory() []*ExecutionRecord {
	w.mu.RLock()
	defer w.mu.RUnlock()
	history := make([]*ExecutionRecord, len(w.history))
	copy(history, w.history)
	return history
}

// renderTemplate 简单模板渲染
func renderTemplate(template string, data map[string]any) string {
	result := template
	for key, value := range data {
		placeholder := fmt.Sprintf("{{%s}}", key)
		valueStr := fmt.Sprintf("%v", value)
		result = strings.ReplaceAll(result, placeholder, valueStr)
	}
	return result
}
