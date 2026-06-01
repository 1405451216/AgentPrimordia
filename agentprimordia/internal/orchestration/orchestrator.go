package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"agentprimordia/internal/agent"
)

const (
	defaultMaxRetries    = 3
	defaultOrchTimeout   = 5 * time.Minute
	orchEventBufferSize  = 100
	defaultBackoff       = time.Second
)

// OrchestratorMode 编排模式
type OrchestratorMode string

const (
	// SequentialMode 顺序模式：Agent按顺序执行
	SequentialMode OrchestratorMode = "sequential"
	// ParallelMode 并行模式：所有Agent并行执行
	ParallelMode OrchestratorMode = "parallel"
	// DAGMode DAG模式：有向无环图工作流
	DAGMode OrchestratorMode = "dag"
)

// OrchestratorConfig 配置
type OrchestratorConfig struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Mode        OrchestratorMode  `json:"mode"`
	MaxRetries  int               `json:"max_retries"` // 失败重试次数（默认3）
	Timeout     time.Duration     `json:"timeout"`     // 全局超时（默认5分钟）
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// AgentStep 单个Agent步骤定义
type AgentStep struct {
	ID          string        `json:"id"`                     // 唯一标识
	Name        string        `json:"name"`                   // 显示名称
	Agent       agent.Agent   `json:"-"`                      // Agent实例
	Prompt      string        `json:"prompt,omitempty"`       // 初始提示
	InputFrom   []string      `json:"input_from,omitempty"`   // 输入来源（其他步骤ID）
	OutputKey   string        `json:"output_key,omitempty"`   // 输出键名
	Condition   StepCondition `json:"condition,omitempty"`    // 执行条件
	RetryPolicy *RetryPolicy  `json:"retry_policy,omitempty"` // 重写全局重试策略
	Timeout     time.Duration `json:"timeout,omitempty"`      // 步骤超时
	Priority    int           `json:"priority"`               // 优先级（用于并行排序）
}

// StepCondition 步骤执行条件
type StepCondition struct {
	Type     string `json:"type"`               // "always", "on_success", "on_failure", "custom"
	Field    string `json:"field,omitempty"`    // 检查的字段
	Operator string `json:"operator,omitempty"` // "==", "!=", "contains", "empty", "not_empty"
	Value    any    `json:"value,omitempty"`    // 期望值
}

// RetryPolicy 重试策略
type RetryPolicy struct {
	MaxRetries int           `json:"max_retries"`
	Backoff    time.Duration `json:"backoff"` // 退避时间
	Jitter     bool          `json:"jitter"`  // 是否添加随机抖动
}

// DAGEdge DAG边定义
type DAGEdge struct {
	From string `json:"from"` // 源节点ID
	To   string `json:"to"`   // 目标节点ID
}

// ExecutionResult 执行结果
type ExecutionResult struct {
	OrchestratorID string                 `json:"orchestrator_id"`
	Mode           OrchestratorMode       `json:"mode"`
	Status         ExecutionStatus        `json:"status"`
	Duration       time.Duration          `json:"duration"`
	StartTime      time.Time              `json:"start_time"`
	EndTime        time.Time              `json:"end_time"`
	Steps          map[string]*StepResult `json:"steps"` // key=step.ID
	FinalOutput    map[string]any         `json:"final_output"`
	Error          error                  `json:"error,omitempty"`
	Metrics        ExecutionMetrics       `json:"metrics"`
}

// StepResult 单步执行结果
type StepResult struct {
	StepID       string          `json:"step_id"`
	StepName     string          `json:"step_name"`
	Status       StepStatus      `json:"status"`
	Response     *agent.Response `json:"response,omitempty"`
	Output       map[string]any  `json:"output"`
	Error        error           `json:"error,omitempty"`
	Duration     time.Duration   `json:"duration"`
	StartTime    time.Time       `json:"start_time"`
	EndTime      time.Time       `json:"end_time"`
	RetryCount   int             `json:"retry_count"`
	Dependencies []string        `json:"dependencies_completed"`
}

// ExecutionStatus 整体状态
type ExecutionStatus string

const (
	StatusRunning   ExecutionStatus = "running"
	StatusCompleted ExecutionStatus = "completed"
	StatusFailed    ExecutionStatus = "failed"
	StatusPartial   ExecutionStatus = "partial" // 部分成功（DAG/Parallel）
	StatusCancelled ExecutionStatus = "cancelled"
)

// StepStatus 单步状态
type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepCompleted StepStatus = "completed"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

// ExecutionMetrics 执行指标
type ExecutionMetrics struct {
	TotalSteps      int           `json:"total_steps"`
	CompletedSteps  int           `json:"completed_steps"`
	FailedSteps     int           `json:"failed_steps"`
	SkippedSteps    int           `json:"skipped_steps"`
	TotalDuration   time.Duration `json:"total_duration"`
	AvgStepDuration time.Duration `json:"avg_step_duration"`
	MaxStepDuration time.Duration `json:"max_step_duration"`
	MinStepDuration time.Duration `json:"min_step_duration"`
	TotalRetries    int           `json:"total_retries"`
	ConcurrencyUsed int           `json:"concurrency_used"` // 实际并发数
}

// Orchestrator 多Agent编排器
type Orchestrator struct {
	config   OrchestratorConfig
	steps    []*AgentStep
	dagEdges []DAGEdge
	mu       sync.RWMutex
	eventCh  chan *OrchestrationEvent
}

// OrchestrationEvent 编排事件
type OrchestrationEvent struct {
	Type      string    `json:"type"` // step_started, step_completed, step_failed, execution_started, execution_completed
	Timestamp time.Time `json:"timestamp"`
	StepID    string    `json:"step_id,omitempty"`
	Data      any       `json:"data,omitempty"`
}

// NewOrchestrator 创建新的编排器
func NewOrchestrator(config OrchestratorConfig) *Orchestrator {
	if config.MaxRetries <= 0 {
		config.MaxRetries = defaultMaxRetries
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultOrchTimeout
	}
	if config.Mode == "" {
		config.Mode = SequentialMode
	}

	return &Orchestrator{
		config:   config,
		steps:    make([]*AgentStep, 0),
		dagEdges: make([]DAGEdge, 0),
		eventCh:  make(chan *OrchestrationEvent, orchEventBufferSize),
	}
}

// AddStep 添加步骤
func (o *Orchestrator) AddStep(step *AgentStep) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, s := range o.steps {
		if s.ID == step.ID {
			return fmt.Errorf("duplicate step ID: %s", step.ID)
		}
	}

	o.steps = append(o.steps, step)
	return nil
}

// AddEdge 添加DAG边（仅DAG模式有效）
func (o *Orchestrator) AddEdge(from, to string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.config.Mode != DAGMode {
		return fmt.Errorf("edges only supported in DAG mode")
	}

	o.dagEdges = append(o.dagEdges, DAGEdge{From: from, To: to})
	return nil
}

// Execute 执行编排流程
func (o *Orchestrator) Execute(ctx context.Context, initialInput map[string]any) (*ExecutionResult, error) {
	startTime := time.Now()

	result := &ExecutionResult{
		OrchestratorID: o.config.Name,
		Mode:           o.config.Mode,
		Status:         StatusRunning,
		StartTime:      startTime,
		Steps:          make(map[string]*StepResult),
		FinalOutput:    make(map[string]any),
	}

	o.emitEvent("execution_started", "", map[string]any{
		"mode":  o.config.Mode,
		"steps": len(o.steps),
		"input": initialInput,
	})

	var err error
	switch o.config.Mode {
	case SequentialMode:
		err = o.executeSequential(ctx, initialInput, result)
	case ParallelMode:
		err = o.executeParallel(ctx, initialInput, result)
	case DAGMode:
		err = o.executeDAG(ctx, initialInput, result)
	default:
		err = fmt.Errorf("unsupported mode: %s", o.config.Mode)
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	result.Error = err
	result.Metrics = o.calculateMetrics(result)

	if err != nil && len(result.Steps) > 0 {
		hasCompleted := false
		hasFailed := false
		for _, sr := range result.Steps {
			switch sr.Status {
			case StepCompleted:
				hasCompleted = true
			case StepFailed:
				hasFailed = true
			}
		}
		if hasCompleted && hasFailed {
			result.Status = StatusPartial
		} else if hasFailed {
			result.Status = StatusFailed
		} else {
			result.Status = StatusCompleted
		}
	} else if err == nil {
		result.Status = StatusCompleted
	} else {
		result.Status = StatusFailed
	}

	o.emitEvent("execution_completed", "", map[string]any{
		"status":   result.Status,
		"duration": result.Duration,
		"error":    err,
	})

	return result, err
}

// executeSequential 顺序执行
func (o *Orchestrator) executeSequential(ctx context.Context, input map[string]any, result *ExecutionResult) error {
	if input == nil {
		input = make(map[string]any)
	}
	currentInput := input

	for _, step := range o.steps {
		stepCtx, cancel := context.WithTimeout(ctx, o.getStepTimeout(step))

		stepResult := o.executeStep(stepCtx, step, currentInput)
		result.Steps[step.ID] = stepResult

		if stepResult.Status == StepFailed {
			if step.RetryPolicy != nil || o.config.MaxRetries > 0 {
				retries := o.getMaxRetries(step)
				for i := 0; i < retries; i++ {
					time.Sleep(o.getBackoff(step, i))
					stepResult = o.executeStep(stepCtx, step, currentInput)
					result.Steps[step.ID] = stepResult
					if stepResult.Status == StepCompleted {
						break
					}
				}
			}

			if stepResult.Status == StepFailed {
				cancel()
				return fmt.Errorf("step '%s' failed after retries: %w", step.Name, stepResult.Error)
			}
		}

		// 合并输出到当前输入，供后续步骤使用
		if stepResult.Output != nil {
			for k, v := range stepResult.Output {
				currentInput[k] = v
			}
		}
		if step.OutputKey != "" && stepResult.Response != nil {
			currentInput[step.OutputKey] = stepResult.Response.Content
		}

		result.FinalOutput = currentInput
		cancel()
	}

	return nil
}

// executeParallel 并行执行
func (o *Orchestrator) executeParallel(ctx context.Context, input map[string]any, result *ExecutionResult) error {
	if input == nil {
		input = make(map[string]any)
	}
	var wg sync.WaitGroup
	resultsCh := make(chan *StepResult, len(o.steps))
	errorsCh := make(chan error, len(o.steps))

	// 按优先级排序
	sortedSteps := make([]*AgentStep, len(o.steps))
	copy(sortedSteps, o.steps)
	sortStepsByPriority(sortedSteps)

	for _, step := range sortedSteps {
		wg.Add(1)
		go func(s *AgentStep) {
			defer wg.Done()
			stepCtx, cancel := context.WithTimeout(ctx, o.getStepTimeout(s))
			defer cancel()

			stepResult := o.executeStep(stepCtx, s, input)
			resultsCh <- stepResult

			if stepResult.Status == StepFailed {
				if s.RetryPolicy != nil || o.config.MaxRetries > 0 {
					retries := o.getMaxRetries(s)
					for i := 0; i < retries; i++ {
						time.Sleep(o.getBackoff(s, i))
						stepResult = o.executeStep(stepCtx, s, input)
						if stepResult.Status == StepCompleted {
							break
						}
					}
					resultsCh <- stepResult
				}
				if stepResult.Status == StepFailed {
					errorsCh <- fmt.Errorf("parallel step '%s' failed: %w", s.Name, stepResult.Error)
				}
			}
		}(step)
	}

	go func() {
		wg.Wait()
		close(resultsCh)
		close(errorsCh)
	}()

	// 收集结果
	for stepResult := range resultsCh {
		result.Steps[stepResult.StepID] = stepResult
		if stepResult.Output != nil {
			for k, v := range stepResult.Output {
				result.FinalOutput[k] = v
			}
		}
	}

	// 收集错误
	var errors []error
	for err := range errorsCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("%d parallel steps failed, first error: %w", len(errors), errors[0])
	}

	return nil
}

// executeDAG DAG执行（拓扑排序+依赖检查）
func (o *Orchestrator) executeDAG(ctx context.Context, input map[string]any, result *ExecutionResult) error {
	if input == nil {
		input = make(map[string]any)
	}
	// 构建依赖图
	deps := o.buildDependencyGraph()
	completed := make(map[string]bool)
	outputs := make(map[string]map[string]any)

	// 拓扑排序
	sortedOrder, err := topologicalSort(o.steps, o.dagEdges)
	if err != nil {
		return fmt.Errorf("DAG validation failed: %w", err)
	}

	// 按拓扑顺序执行
	for _, stepID := range sortedOrder {
		step := o.findStepByID(stepID)
		if step == nil {
			continue
		}

		// 检查依赖是否完成
		stepDeps := deps[stepID]
		allDepsComplete := true
		for _, depID := range stepDeps {
			if !completed[depID] {
				allDepsComplete = false
				break
			}
		}

		if !allDepsComplete {
			skippedResult := &StepResult{
				StepID:   step.ID,
				StepName: step.Name,
				Status:   StepSkipped,
				Error:    fmt.Errorf("dependencies not met"),
			}
			result.Steps[step.ID] = skippedResult
			continue
		}

		// 合并依赖输出作为当前步骤输入
		stepInput := make(map[string]any)
		for k, v := range input {
			stepInput[k] = v
		}
		for _, depID := range stepDeps {
			if depOutput, ok := outputs[depID]; ok {
				for k, v := range depOutput {
					stepInput[k] = v
				}
			}
		}

		// 执行步骤
		stepCtx, cancel := context.WithTimeout(ctx, o.getStepTimeout(step))

		stepResult := o.executeStep(stepCtx, step, stepInput)
		result.Steps[step.ID] = stepResult

		if stepResult.Status == StepFailed && (step.RetryPolicy != nil || o.config.MaxRetries > 0) {
			retries := o.getMaxRetries(step)
			for i := 0; i < retries; i++ {
				time.Sleep(o.getBackoff(step, i))
				stepResult = o.executeStep(stepCtx, step, stepInput)
				if stepResult.Status == StepCompleted {
					break
				}
			}
			result.Steps[step.ID] = stepResult
		}

		if stepResult.Status == StepCompleted {
			completed[stepID] = true
			outputs[stepID] = stepResult.Output
			if stepResult.Output != nil {
				for k, v := range stepResult.Output {
					result.FinalOutput[k] = v
				}
			}
		}
		cancel()
	}

	// 检查是否有失败的关键步骤
	for _, sr := range result.Steps {
		if sr.Status == StepFailed {
			return fmt.Errorf("DAG step '%s' failed: %w", sr.StepName, sr.Error)
		}
	}

	return nil
}

// executeStep 执行单个步骤
func (o *Orchestrator) executeStep(ctx context.Context, step *AgentStep, input map[string]any) *StepResult {
	startTime := time.Now()
	result := &StepResult{
		StepID:    step.ID,
		StepName:  step.Name,
		Status:    StepRunning,
		StartTime: startTime,
		Output:    make(map[string]any),
	}

	o.emitEvent("step_started", step.ID, map[string]any{
		"name": step.Name,
	})

	// 检查执行条件
	if !o.checkCondition(step, input) {
		result.Status = StepSkipped
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result
	}

	// 构建提示词
	prompt := step.Prompt
	if prompt == "" {
		prompt = o.buildPromptFromInput(input, step.InputFrom)
	}

	// 执行Agent
	resp, err := step.Agent.Run(ctx, agent.UserMessage(prompt))

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	result.Response = resp

	if err != nil {
		result.Status = StepFailed
		result.Error = err
		o.emitEvent("step_failed", step.ID, map[string]any{
			"error": err.Error(),
		})
		return result
	}

	result.Status = StepCompleted
	if resp.Content != "" {
		result.Output["content"] = resp.Content
	}
	if step.OutputKey != "" {
		result.Output[step.OutputKey] = resp.Content
	}
	if resp.Metrics.TotalTurns > 0 {
		result.Output["turns"] = resp.Metrics.TotalTurns
	}

	o.emitEvent("step_completed", step.ID, map[string]any{
		"duration": result.Duration,
		"turns":    resp.Metrics.TotalTurns,
	})

	return result
}

// checkCondition 检查执行条件
func (o *Orchestrator) checkCondition(step *AgentStep, input map[string]any) bool {
	if step.Condition.Type == "" || step.Condition.Type == "always" {
		return true
	}

	value, exists := input[step.Condition.Field]
	if !exists {
		return false
	}

	switch step.Condition.Operator {
	case "==":
		return fmt.Sprintf("%v", value) == fmt.Sprintf("%v", step.Condition.Value)
	case "!=":
		return fmt.Sprintf("%v", value) != fmt.Sprintf("%v", step.Condition.Value)
	case "contains":
		strValue := fmt.Sprintf("%v", value)
		return strings.Contains(strings.ToLower(strValue), strings.ToLower(fmt.Sprintf("%v", step.Condition.Value)))
	case "empty":
		return value == nil || fmt.Sprintf("%v", value) == ""
	case "not_empty":
		return value != nil && fmt.Sprintf("%v", value) != ""
	default:
		return true
	}
}

// buildPromptFromInput 从输入构建提示词
func (o *Orchestrator) buildPromptFromInput(input map[string]any, inputKeys []string) string {
	if len(inputKeys) == 0 {
		data, _ := json.MarshalIndent(input, "", "  ")
		return fmt.Sprintf("请基于以下上下文信息进行处理:\n\n%s", string(data))
	}

	var parts []string
	for _, key := range inputKeys {
		if val, ok := input[key]; ok {
			parts = append(parts, fmt.Sprintf("[%s]:\n%v", key, val))
		}
	}

	return fmt.Sprintf("请基于以下信息进行处理:\n\n%s", strings.Join(parts, "\n\n"))
}

// calculateMetrics 计算执行指标
func (o *Orchestrator) calculateMetrics(result *ExecutionResult) ExecutionMetrics {
	metrics := ExecutionMetrics{
		TotalSteps: len(result.Steps),
	}

	var totalDuration time.Duration
	var durations []time.Duration

	for _, sr := range result.Steps {
		switch sr.Status {
		case StepCompleted:
			metrics.CompletedSteps++
		case StepFailed:
			metrics.FailedSteps++
		case StepSkipped:
			metrics.SkippedSteps++
		}

		if sr.Duration > 0 {
			totalDuration += sr.Duration
			durations = append(durations, sr.Duration)
			metrics.TotalRetries += sr.RetryCount
		}
	}

	if len(durations) > 0 {
		metrics.AvgStepDuration = totalDuration / time.Duration(len(durations))
		metrics.MaxStepDuration = durations[0]
		metrics.MinStepDuration = durations[0]
		for _, d := range durations {
			if d > metrics.MaxStepDuration {
				metrics.MaxStepDuration = d
			}
			if d < metrics.MinStepDuration {
				metrics.MinStepDuration = d
			}
		}
	}

	metrics.TotalDuration = totalDuration

	switch result.Mode {
	case ParallelMode:
		metrics.ConcurrencyUsed = len(result.Steps)
	default:
		metrics.ConcurrencyUsed = 1
	}

	return metrics
}

// 辅助方法
func (o *Orchestrator) getStepTimeout(step *AgentStep) time.Duration {
	if step.Timeout > 0 {
		return step.Timeout
	}
	return o.config.Timeout / time.Duration(len(o.steps)+1)
}

func (o *Orchestrator) getMaxRetries(step *AgentStep) int {
	if step.RetryPolicy != nil && step.RetryPolicy.MaxRetries > 0 {
		return step.RetryPolicy.MaxRetries
	}
	return o.config.MaxRetries
}

func (o *Orchestrator) getBackoff(step *AgentStep, attempt int) time.Duration {
	baseBackoff := defaultBackoff
	if step.RetryPolicy != nil && step.RetryPolicy.Backoff > 0 {
		baseBackoff = step.RetryPolicy.Backoff
	}

	backoff := baseBackoff * time.Duration(attempt+1)
	if step.RetryPolicy != nil && step.RetryPolicy.Jitter {
		backoff += time.Duration((attempt+1)*100) * time.Millisecond
	}

	return backoff
}

func (o *Orchestrator) findStepByID(id string) *AgentStep {
	for _, s := range o.steps {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func (o *Orchestrator) buildDependencyGraph() map[string][]string {
	deps := make(map[string][]string)

	// 从边的定义构建
	for _, edge := range o.dagEdges {
		deps[edge.To] = append(deps[edge.To], edge.From)
	}

	// 从 InputFrom 构建
	for _, step := range o.steps {
		for _, inputFrom := range step.InputFrom {
			deps[step.ID] = append(deps[step.ID], inputFrom)
		}
	}

	return deps
}

func (o *Orchestrator) emitEvent(eventType, stepID string, data any) {
	select {
	case o.eventCh <- &OrchestrationEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		StepID:    stepID,
		Data:      data,
	}:
	default:
	}
}

// Events 返回事件通道
func (o *Orchestrator) Events() <-chan *OrchestrationEvent {
	return o.eventCh
}

// Stats 返回统计信息
func (o *Orchestrator) Stats() map[string]any {
	o.mu.RLock()
	defer o.mu.RUnlock()

	return map[string]any{
		"name":        o.config.Name,
		"mode":        o.config.Mode,
		"total_steps": len(o.steps),
		"description": o.config.Description,
	}
}

// Export 导出配置为JSON
func (o *Orchestrator) Export() ([]byte, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	data := map[string]any{
		"config":    o.config,
		"steps":     o.steps,
		"dag_edges": o.dagEdges,
	}
	return json.MarshalIndent(data, "", "  ")
}

// ===== 工具函数 =====

func sortStepsByPriority(steps []*AgentStep) {
	for i := 0; i < len(steps)-1; i++ {
		for j := i + 1; j < len(steps); j++ {
			if steps[i].Priority > steps[j].Priority {
				steps[i], steps[j] = steps[j], steps[i]
			}
		}
	}
}

// topologicalSort 拓扑排序
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
