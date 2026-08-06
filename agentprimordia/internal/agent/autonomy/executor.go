package autonomy

import (
	"context"
	"fmt"
	"sync"
)

// StepExecutor 步骤执行器接口（由外部实现具体执行逻辑）
type StepExecutor interface {
	// ExecuteStep 执行单个步骤，返回结果或错误
	ExecuteStep(ctx context.Context, step PlanStep) (string, error)
}

// GoalExecutorConfig 执行器配置
type GoalExecutorConfig struct {
	// StepExecutor 步骤执行器实现
	StepExecutor StepExecutor
	// MaxRetries 步骤级最大重试次数（默认 3）
	MaxRetries int
	// OnStepComplete 步骤完成回调
	OnStepComplete func(stepID string, result string)
	// OnStepFail 步骤失败回调
	OnStepFail func(stepID string, err error, retryCount int)
}

// GoalExecutor 目标执行体：按计划逐步执行，支持并行与重试
type GoalExecutor struct {
	cfg GoalExecutorConfig
}

// NewGoalExecutor 创建目标执行器
func NewGoalExecutor(cfg GoalExecutorConfig) *GoalExecutor {
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	return &GoalExecutor{cfg: cfg}
}

// Execute 执行整个计划（阻塞直到完成或失败）
func (e *GoalExecutor) Execute(ctx context.Context, plan *GoalPlan) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("autonomy: 执行被取消: %w", ctx.Err())
		default:
		}

		if plan.IsComplete() {
			return nil
		}

		ready := plan.ReadySteps()
		if len(ready) == 0 {
			// 无就绪步骤且未完成 → 存在失败步骤阻塞
			return fmt.Errorf("autonomy: 无就绪步骤，计划阻塞")
		}

		// 分离并行与顺序步骤
		var parallelSteps, sequentialSteps []PlanStep
		for _, s := range ready {
			if s.Strategy == StepStrategyParallel {
				parallelSteps = append(parallelSteps, s)
			} else {
				sequentialSteps = append(sequentialSteps, s)
			}
		}

		// 并行执行所有并行步骤
		if len(parallelSteps) > 0 {
			if err := e.executeParallel(ctx, plan, parallelSteps); err != nil {
				return err
			}
		}

		// 顺序执行（每次取第一个就绪的顺序步骤）
		if len(sequentialSteps) > 0 {
			step := sequentialSteps[0]
			if err := e.executeStepWithRetry(ctx, plan, step); err != nil {
				return err
			}
		}
	}
}

// executeParallel 并行执行一组步骤
func (e *GoalExecutor) executeParallel(ctx context.Context, plan *GoalPlan, steps []PlanStep) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(steps))

	for _, step := range steps {
		wg.Add(1)
		go func(s PlanStep) {
			defer wg.Done()
			if err := e.executeStepWithRetry(ctx, plan, s); err != nil {
				errCh <- err
			}
		}(step)
	}

	wg.Wait()
	close(errCh)

	// 返回第一个错误（如有）
	for err := range errCh {
		return err
	}
	return nil
}

// executeStepWithRetry 带重试的步骤执行
func (e *GoalExecutor) executeStepWithRetry(ctx context.Context, plan *GoalPlan, step PlanStep) error {
	plan.MarkStepRunning(step.ID)

	var lastErr error
	for attempt := 0; attempt <= e.cfg.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			plan.MarkStepFailed(step.ID, ctx.Err().Error())
			return fmt.Errorf("autonomy: 执行被取消: %w", ctx.Err())
		default:
		}

		result, err := e.cfg.StepExecutor.ExecuteStep(ctx, step)
		if err == nil {
			// 成功
			plan.mu.Lock()
			for i := range plan.Steps {
				if plan.Steps[i].ID == step.ID {
					plan.Steps[i].Result = result
					break
				}
			}
			plan.mu.Unlock()
			plan.MarkStepCompleted(step.ID)

			if e.cfg.OnStepComplete != nil {
				e.cfg.OnStepComplete(step.ID, result)
			}
			return nil
		}

		lastErr = err
		if e.cfg.OnStepFail != nil {
			e.cfg.OnStepFail(step.ID, err, attempt+1)
		}
	}

	// 重试耗尽
	plan.MarkStepFailed(step.ID, lastErr.Error())
	return fmt.Errorf("autonomy: 步骤 %s 重试 %d 次后仍失败: %w", step.ID, e.cfg.MaxRetries, lastErr)
}
