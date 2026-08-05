package orchestration

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	defaultPipelineTimeout  = 10 * time.Minute
	defaultStageTimeout     = 2 * time.Minute
	defaultMaxStageRetries  = 3
	pipelineEventBufferSize = 50
)

// ErrorStrategy 错误处理策略
type ErrorStrategy string

const (
	// ErrorAbort 中止整个流水线
	ErrorAbort ErrorStrategy = "abort"
	// ErrorSkip 跳过当前阶段，继续执行下一阶段
	ErrorSkip ErrorStrategy = "skip"
	// ErrorRetry 重试当前阶段
	ErrorRetry ErrorStrategy = "retry"
)

// PipelineStatus 流水线状态
type PipelineStatus string

const (
	// PipelineStatusSuccess 所有阶段都成功完成
	PipelineStatusSuccess PipelineStatus = "success"
	// PipelineStatusPartial 部分阶段成功，部分失败或跳过
	PipelineStatusPartial PipelineStatus = "partial"
	// PipelineStatusFailed 流水线失败（中止或所有阶段都失败）
	PipelineStatusFailed PipelineStatus = "failed"
)

// StageHandler 阶段处理函数类型
// 接收上下文和输入字符串，返回输出字符串和错误
type StageHandler func(ctx context.Context, input string) (output string, err error)

// Stage 流水线阶段定义
type Stage struct {
	Name       string        `json:"name"`                  // 阶段名称
	Handler    StageHandler  `json:"-"`                     // 处理函数
	Timeout    time.Duration `json:"timeout"`               // 阶段超时时间
	OnError    ErrorStrategy `json:"on_error"`              // 错误处理策略
	MaxRetries int           `json:"max_retries,omitempty"` // 最大重试次数（仅当 OnError=ErrorRetry 时有效）
}

// StageResult 单个阶段的执行结果
type StageResult struct {
	StageName  string        `json:"stage_name"`       // 阶段名称
	Status     StepStatus    `json:"status"`           // 执行状态
	Input      string        `json:"input"`            // 输入数据
	Output     string        `json:"output,omitempty"` // 输出数据
	Error      error         `json:"error,omitempty"`  // 错误信息
	Duration   time.Duration `json:"duration"`         // 执行耗时
	StartTime  time.Time     `json:"start_time"`       // 开始时间
	EndTime    time.Time     `json:"end_time"`         // 结束时间
	RetryCount int           `json:"retry_count"`      // 重试次数
}

// PipelineResult 流水线执行结果
type PipelineResult struct {
	StageResults []StageResult  `json:"stage_results"`          // 各阶段结果
	Duration     time.Duration  `json:"duration"`               // 总执行耗时
	Status       PipelineStatus `json:"status"`                 // 流水线状态
	StartTime    time.Time      `json:"start_time"`             // 开始时间
	EndTime      time.Time      `json:"end_time"`               // 结束时间
	FinalOutput  string         `json:"final_output,omitempty"` // 最终输出
	Error        error          `json:"error,omitempty"`        // 错误信息
}

// PipelineEvent 流水线事件
type PipelineEvent struct {
	Type      string    `json:"type"`                 // 事件类型
	Timestamp time.Time `json:"timestamp"`            // 时间戳
	StageName string    `json:"stage_name,omitempty"` // 阶段名称
	Data      any       `json:"data,omitempty"`       // 事件数据
}

// Pipeline 流水线协作模式
// 按顺序执行多个阶段，前一阶段的输出作为下一阶段的输入
type Pipeline struct {
	mu      sync.RWMutex
	stages  []*Stage
	eventCh chan *PipelineEvent
	timeout time.Duration // 全局超时
}

// NewPipeline 创建新的流水线
func NewPipeline(timeout time.Duration) *Pipeline {
	if timeout <= 0 {
		timeout = defaultPipelineTimeout
	}

	return &Pipeline{
		stages:  make([]*Stage, 0),
		eventCh: make(chan *PipelineEvent, pipelineEventBufferSize),
		timeout: timeout,
	}
}

// AddStage 添加阶段到流水线
func (p *Pipeline) AddStage(stage *Stage) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if stage.Name == "" {
		return fmt.Errorf("stage name is required")
	}
	if stage.Handler == nil {
		return fmt.Errorf("stage handler is required")
	}

	// 检查名称是否重复
	for _, s := range p.stages {
		if s.Name == stage.Name {
			return fmt.Errorf("duplicate stage name: %s", stage.Name)
		}
	}

	// 设置默认值
	if stage.Timeout <= 0 {
		stage.Timeout = defaultStageTimeout
	}
	if stage.OnError == "" {
		stage.OnError = ErrorAbort
	}
	if stage.MaxRetries <= 0 && stage.OnError == ErrorRetry {
		stage.MaxRetries = defaultMaxStageRetries
	}

	p.stages = append(p.stages, stage)
	return nil
}

// Execute 执行流水线
// 按顺序执行每个阶段，前一阶段的输出作为下一阶段的输入
// 空流水线（无任何阶段）返回错误。
func (p *Pipeline) Execute(ctx context.Context, input string) (*PipelineResult, error) {
	p.mu.RLock()
	stageCount := len(p.stages)
	p.mu.RUnlock()
	if stageCount == 0 {
		return nil, fmt.Errorf("pipeline: no stages defined")
	}

	startTime := time.Now()

	result := &PipelineResult{
		StageResults: make([]StageResult, 0, len(p.stages)),
		StartTime:    startTime,
		Status:       PipelineStatusSuccess,
	}

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	p.emitEvent("pipeline_started", "", map[string]any{
		"stages": len(p.stages),
		"input":  input,
	})

	currentInput := input
	var pipelineErr error
	aborted := false

	for _, stage := range p.stages {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			result.Status = PipelineStatusFailed
			result.Error = ctx.Err()
			pipelineErr = ctx.Err()
			aborted = true
		default:
		}
		if aborted {
			break
		}

		stageResult := p.executeStage(ctx, stage, currentInput)

		// 如果阶段失败且策略是 Skip，将状态改为 Skipped
		if stageResult.Status == StepFailed && stage.OnError == ErrorSkip {
			stageResult.Status = StepSkipped
		}

		result.StageResults = append(result.StageResults, stageResult)

		// 根据阶段状态和错误策略决定下一步
		if stageResult.Status == StepFailed {
			switch stage.OnError {
			case ErrorAbort:
				result.Status = PipelineStatusFailed
				pipelineErr = fmt.Errorf("stage '%s' failed: %w", stage.Name, stageResult.Error)
				aborted = true

			case ErrorSkip:
				// 跳过当前阶段，继续使用之前的输入（currentInput 保持不变）

			case ErrorRetry:
				// 重试逻辑已在 executeStage 中处理
				// 如果仍然失败，则中止流水线
				if stageResult.Status == StepFailed {
					result.Status = PipelineStatusFailed
					pipelineErr = fmt.Errorf("stage '%s' failed after %d retries: %w",
						stage.Name, stageResult.RetryCount, stageResult.Error)
					aborted = true
				}
			}
		}
		if aborted {
			break
		}

		// 更新输入为当前阶段的输出
		if stageResult.Status == StepCompleted {
			currentInput = stageResult.Output
		}
	}

	// 设置最终输出和状态
	if !aborted {
		result.FinalOutput = currentInput

		// 检查是否有跳过的阶段
		hasSkipped := false
		hasFailed := false
		skippedStages := []string{}
		for _, sr := range result.StageResults {
			if sr.Status == StepSkipped {
				hasSkipped = true
				skippedStages = append(skippedStages, sr.StageName)
			}
			if sr.Status == StepFailed {
				hasFailed = true
			}
		}

		if hasFailed {
			result.Status = PipelineStatusFailed
		} else if hasSkipped {
			result.Status = PipelineStatusPartial
			pipelineErr = fmt.Errorf("stages skipped: %v", skippedStages)
		} else {
			result.Status = PipelineStatusSuccess
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	result.Error = pipelineErr

	p.emitEvent("pipeline_completed", "", map[string]any{
		"status":   result.Status,
		"duration": result.Duration,
		"error":    pipelineErr,
	})

	return result, pipelineErr
}

// executeStage 执行单个阶段
func (p *Pipeline) executeStage(ctx context.Context, stage *Stage, input string) StageResult {
	startTime := time.Now()

	result := StageResult{
		StageName: stage.Name,
		Status:    StepRunning,
		Input:     input,
		StartTime: startTime,
	}

	p.emitEvent("stage_started", stage.Name, map[string]any{
		"input_length": len(input),
	})

	// 创建带超时的上下文
	stageCtx, cancel := context.WithTimeout(ctx, stage.Timeout)
	defer cancel()

	// 使用 goroutine + select 包装 handler 调用，确保能响应 context 取消
	type handlerResult struct {
		output string
		err    error
	}
	// 优化（perf-v2）：复用同一个结果 channel，避免每次重试分配新 channel
	resultCh := make(chan handlerResult, 1)

	go func() {
		output, err := stage.Handler(stageCtx, input)
		resultCh <- handlerResult{output: output, err: err}
	}()

	// 等待 handler 完成或 context 取消
	var output string
	var err error
	select {
	case hr := <-resultCh:
		output = hr.output
		err = hr.err
	case <-stageCtx.Done():
		err = stageCtx.Err()
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)

	if err != nil {
		result.Status = StepFailed
		result.Error = err

		// 如果需要重试，进行重试
		if stage.OnError == ErrorRetry && stage.MaxRetries > 0 {
			for retry := 0; retry < stage.MaxRetries; retry++ {
				// 检查上下文是否已取消
				select {
				case <-stageCtx.Done():
					result.Error = stageCtx.Err()
					return result
				default:
				}

				result.RetryCount++
				p.emitEvent("stage_retrying", stage.Name, map[string]any{
					"retry":       retry + 1,
					"max_retries": stage.MaxRetries,
				})

				// 重试前等待一小段时间（指数退避）
				backoff := time.Duration(retry+1) * 100 * time.Millisecond
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return StageResult{Error: ctx.Err()}
				}

				// 优化（perf-v2）：复用同一个 resultCh，排空后重用
				select {
				case <-resultCh:
				default:
				}
				go func() {
					o, e := stage.Handler(stageCtx, input)
					resultCh <- handlerResult{output: o, err: e}
				}()

				select {
				case hr := <-resultCh:
					output = hr.output
					err = hr.err
				case <-stageCtx.Done():
					err = stageCtx.Err()
				}

				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)

				if err == nil {
					// 重试成功
					result.Status = StepCompleted
					result.Output = output
					result.Error = nil
					break
				}
			}
		}

		if result.Status == StepFailed {
			p.emitEvent("stage_failed", stage.Name, map[string]any{
				"error":       err.Error(),
				"duration":    result.Duration,
				"retry_count": result.RetryCount,
			})
			return result
		}
	}

	// 执行成功
	result.Status = StepCompleted
	result.Output = output

	p.emitEvent("stage_completed", stage.Name, map[string]any{
		"output_length": len(output),
		"duration":      result.Duration,
	})

	return result
}

// emitEvent 发射事件
func (p *Pipeline) emitEvent(eventType, stageName string, data any) {
	select {
	case p.eventCh <- &PipelineEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		StageName: stageName,
		Data:      data,
	}:
	default:
		// 事件通道已满，丢弃事件
	}
}

// Events 返回事件通道
func (p *Pipeline) Events() <-chan *PipelineEvent {
	return p.eventCh
}

// GetStages 获取所有阶段（只读副本）
func (p *Pipeline) GetStages() []*Stage {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stages := make([]*Stage, len(p.stages))
	copy(stages, p.stages)
	return stages
}

// StageCount 获取阶段数量
func (p *Pipeline) StageCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.stages)
}
