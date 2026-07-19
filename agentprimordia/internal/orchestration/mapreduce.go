package orchestration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Mapper 对单个输入元素执行转换
type Mapper func(ctx context.Context, item any) (any, error)

// Reducer 将所有 map 输出合并为最终结果
type Reducer func(results []any) (any, error)

// Scatterer 对单个任务执行分发处理
type Scatterer func(ctx context.Context, task any) (any, error)

// Gatherer 汇总所有 scatter 输出
type Gatherer func(results []any) (any, error)

// MapReduceConfig MapReduce 编排配置
type MapReduceConfig struct {
	MapperCount int           // 并发 mapper 数量
	Mapper      Mapper        // mapper 函数
	Reducer     Reducer       // reducer 函数
	Timeout     time.Duration // 全局超时
}

// ScatterGatherConfig Scatter-Gather 编排配置
type ScatterGatherConfig struct {
	MaxGatherers   int           // 最大并发 gatherer 数量
	Scatterer      Scatterer     // scatter 函数
	Gatherer       Gatherer      // gather 函数
	Timeout        time.Duration // 全局超时
	PartialResults bool          // 是否允许部分结果
}

// ExecuteMapReduce 执行 MapReduce 模式。
// Map 阶段将 input 切片中每个元素并行送入 Mapper，
// Reduce 阶段收集所有 map 结果后调用 Reducer。
func (o *Orchestrator) ExecuteMapReduce(ctx context.Context, config MapReduceConfig, input []any) (*ExecutionResult, error) {
	startTime := time.Now()

	if config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}

	result := &ExecutionResult{
		OrchestratorID: o.config.Name,
		Mode:           ParallelMode,
		Status:         StatusRunning,
		StartTime:      startTime,
		Steps:          make(map[string]*StepResult),
		FinalOutput:    make(map[string]any),
	}

	o.emitEvent("execution_started", "", map[string]any{
		"type":  "mapreduce",
		"input": len(input),
	})

	mapperCount := config.MapperCount
	if mapperCount <= 0 {
		mapperCount = defaultWorkerMaxConcurrency
	}

	type indexedResult struct {
		idx    int
		output any
		err    error
	}

	results := make([]indexedResult, len(input))
	var wg sync.WaitGroup
	sem := make(chan struct{}, mapperCount)
	var processedCount int32

	for i, item := range input {
		if ctx.Err() != nil {
			results[i] = indexedResult{idx: i, err: ctx.Err()}
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, it any) {
			defer wg.Done()
			defer func() { <-sem }()
			out, errCall := config.Mapper(ctx, it)
			atomic.AddInt32(&processedCount, 1)
			results[idx] = indexedResult{idx: idx, output: out, err: errCall}
		}(i, item)
	}
	wg.Wait()

	if ctx.Err() != nil {
		result.Status = StatusFailed
		result.Error = ctx.Err()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		o.emitEvent("execution_completed", "", map[string]any{
			"status":   result.Status,
			"duration": result.Duration,
			"error":    ctx.Err(),
		})
		return result, ctx.Err()
	}

	// 收集非错误结果
	var mapOutputs []any
	hasFailure := false
	for _, r := range results {
		stepID := "map_" + formatIndex(r.idx)
		sr := &StepResult{
			StepID:    stepID,
			StepName:  stepID,
			StartTime: startTime,
			EndTime:   time.Now(),
			Output:    make(map[string]any),
		}
		if r.err != nil {
			sr.Status = StepFailed
			sr.Error = r.err
			hasFailure = true
		} else {
			sr.Status = StepCompleted
			sr.Output["value"] = r.output
			mapOutputs = append(mapOutputs, r.output)
		}
		result.Steps[stepID] = sr
	}

	reducerInput := mapOutputs
	reducerOutput, err := config.Reducer(reducerInput)
	if err != nil {
		result.Error = err
		if hasFailure {
			result.Status = StatusPartial
		} else {
			result.Status = StatusFailed
		}
	} else {
		result.FinalOutput["result"] = reducerOutput
		if hasFailure {
			result.Status = StatusPartial
		} else {
			result.Status = StatusCompleted
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Metrics = ExecutionMetrics{
		TotalSteps:     int(processedCount),
		CompletedSteps: int(processedCount),
	}

	o.emitEvent("execution_completed", "", map[string]any{
		"status":   result.Status,
		"duration": result.Duration,
	})

	return result, err
}

// ExecuteScatterGather 执行 Scatter-Gather 模式。
// Scatter 阶段将 tasks 并发分发给多个 Gatherer worker，
// Gather 阶段汇总所有结果。支持部分结果模式。
func (o *Orchestrator) ExecuteScatterGather(ctx context.Context, config ScatterGatherConfig, tasks []any) (*ExecutionResult, error) {
	startTime := time.Now()

	if config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}

	result := &ExecutionResult{
		OrchestratorID: o.config.Name,
		Mode:           ParallelMode,
		Status:         StatusRunning,
		StartTime:      startTime,
		Steps:          make(map[string]*StepResult),
		FinalOutput:    make(map[string]any),
	}

	o.emitEvent("execution_started", "", map[string]any{
		"type":  "scattergather",
		"tasks": len(tasks),
	})

	maxGatherers := config.MaxGatherers
	if maxGatherers <= 0 {
		maxGatherers = defaultWorkerMaxConcurrency
	}

	type scatterResult struct {
		taskID string
		output any
		err    error
	}

	scatterChan := make(chan scatterResult, len(tasks))
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxGatherers)

	for i, task := range tasks {
		if ctx.Err() != nil {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, t any) {
			defer wg.Done()
			defer func() { <-sem }()
			out, errCall := config.Scatterer(ctx, t)
			scatterChan <- scatterResult{
				taskID: "task_" + formatIndex(idx),
				output: out,
				err:    errCall,
			}
		}(i, task)
	}
	wg.Wait()
	close(scatterChan)

	var gatherInputs []any
	for sr := range scatterChan {
		srep := &StepResult{
			StepID:    sr.taskID,
			StepName:  sr.taskID,
			StartTime: startTime,
			EndTime:   time.Now(),
			Output:    make(map[string]any),
		}
		if sr.err != nil {
			srep.Status = StepFailed
			srep.Error = sr.err
		} else {
			srep.Status = StepCompleted
			srep.Output["value"] = sr.output
			gatherInputs = append(gatherInputs, sr.output)
		}
		result.Steps[sr.taskID] = srep
	}

	// 如果 PartialResults 为 false 且存在任何失败，则返回错误
	if !config.PartialResults {
		for _, sr := range result.Steps {
			if sr.Status == StepFailed {
				result.Status = StatusFailed
				result.Error = errors.New("scatter-gather failed with strict mode")
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(result.StartTime)
				return result, result.Error
			}
		}
	}

	gatherOutput, err := config.Gatherer(gatherInputs)
	if err != nil {
		result.Error = err
		result.Status = StatusFailed
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, err
	}

	result.FinalOutput["result"] = gatherOutput
	result.Status = StatusCompleted
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Metrics = ExecutionMetrics{
		TotalSteps:     len(tasks),
		CompletedSteps: len(gatherInputs),
	}

	o.emitEvent("execution_completed", "", map[string]any{
		"status":   result.Status,
		"duration": result.Duration,
	})

	return result, nil
}

func formatIndex(idx int) string {
	return fmt.Sprintf("%d", idx)
}
