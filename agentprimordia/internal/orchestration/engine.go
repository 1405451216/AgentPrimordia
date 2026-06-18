package orchestration

import (
	"context"
	"time"
)

// ExecutionEngineConfig 执行引擎配置。
type ExecutionEngineConfig struct {
	MaxConcurrency int
	RetryPolicy    RetryPolicy
	FailFast       bool
	EventCh        chan<- *OrchestrationEvent
}

// ExecutionEngine 统一执行引擎。
type ExecutionEngine struct {
	cfg ExecutionEngineConfig
}

// NewExecutionEngine 创建执行引擎。
func NewExecutionEngine(cfg ExecutionEngineConfig) *ExecutionEngine {
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = defaultWorkerMaxConcurrency
	}
	return &ExecutionEngine{cfg: cfg}
}

// Run 执行编排计划。
func (e *ExecutionEngine) Run(ctx context.Context, mode OrchestratorMode, steps []*AgentStep, edges []DAGEdge, initialInput map[string]any) (*ExecutionResult, error) {
	startTime := time.Now()

	plan, err := BuildExecutionPlan(mode, steps, edges)
	if err != nil {
		return nil, err
	}

	executor := NewDefaultStepExecutor(e.cfg.EventCh)
	pool := NewWorkerPool(e.cfg.MaxConcurrency, executor)
	defer pool.Stop()

	scheduler := NewScheduler(plan, pool, SchedulerConfig{
		MaxConcurrency: e.cfg.MaxConcurrency,
		RetryPolicy:    e.cfg.RetryPolicy,
		FailFast:       e.cfg.FailFast,
	})

	stepResults, err := scheduler.Run(ctx, initialInput)
	if err != nil && len(stepResults) == 0 {
		return nil, err
	}

	return buildExecutionResult(plan, stepResults, initialInput, startTime, err), nil
}

func buildExecutionResult(plan *ExecutionPlan, stepResults map[string]*StepResult, initialInput map[string]any, startTime time.Time, runErr error) *ExecutionResult {
	result := &ExecutionResult{
		Mode:        plan.Mode,
		Status:      StatusCompleted,
		StartTime:   startTime,
		Steps:       stepResults,
		FinalOutput: make(map[string]any),
	}

	if initialInput != nil {
		for k, v := range initialInput {
			result.FinalOutput[k] = v
		}
	}
	for _, sr := range stepResults {
		if sr.Status == StepCompleted && sr.Output != nil {
			for k, v := range sr.Output {
				result.FinalOutput[k] = v
			}
		}
	}

	result.Error = runErr
	if runErr != nil {
		hasCompleted := false
		hasFailed := false
		for _, sr := range stepResults {
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
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Metrics = calculateMetrics(result)
	return result
}

// calculateMetrics 计算执行指标。
func calculateMetrics(result *ExecutionResult) ExecutionMetrics {
	metrics := ExecutionMetrics{TotalSteps: len(result.Steps)}
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
	if result.Mode == ParallelMode {
		metrics.ConcurrencyUsed = len(result.Steps)
	} else {
		metrics.ConcurrencyUsed = 1
	}
	return metrics
}
