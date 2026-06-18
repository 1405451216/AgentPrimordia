package orchestration

import (
	"context"
	"fmt"
	"time"
)

// SchedulerConfig 调度器配置。
type SchedulerConfig struct {
	MaxConcurrency int
	RetryPolicy    RetryPolicy
	FailFast       bool
}

// Scheduler 负责按依赖关系和并发限制派发 step。
type Scheduler struct {
	plan *ExecutionPlan
	pool *WorkerPool
	cfg  SchedulerConfig
}

// NewScheduler 创建 Scheduler。
func NewScheduler(plan *ExecutionPlan, pool *WorkerPool, cfg SchedulerConfig) *Scheduler {
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = 1
	}
	return &Scheduler{plan: plan, pool: pool, cfg: cfg}
}

// Run 执行计划，返回 stepID -> StepResult 的映射。
// 优化（perf-v2）：
//  1. 就绪队列使用预分配 slice + 索引指针，避免 slice 重新分配
//  2. 重试扫描仅查找最早到期项，避免每轮遍历全部 pending 项
func (s *Scheduler) Run(ctx context.Context, initialInput map[string]any) (map[string]*StepResult, error) {
	resultsCh := make(chan *StepResult, len(s.plan.Steps))

	nodeByID := make(map[string]*StepNode, len(s.plan.Steps))
	for _, n := range s.plan.Steps {
		nodeByID[n.Step.ID] = n
	}

	running := 0
	completed := 0
	failed := false
	outputs := make(map[string]map[string]any)
	retryCount := make(map[string]int)
	pendingRetry := make(map[string]time.Time)

	// 优化（perf-v2）：预分配就绪队列容量，使用 head 指针代替 slice re-slicing
	ready := make([]*StepNode, 0, len(s.plan.Steps))
	readyHead := 0
	for _, n := range s.plan.Steps {
		if s.plan.DepGraph.Ready(n.Step.ID) {
			ready = append(ready, n)
		}
	}

	retryTimer := time.NewTimer(time.Hour)
	defer retryTimer.Stop()
	retryTimer.Stop()

	totalSteps := len(s.plan.Steps)
	for completed < totalSteps {
		// 将已到重试时间的任务加入就绪队列
		if len(pendingRetry) > 0 {
			now := time.Now()
			for id, when := range pendingRetry {
				if !now.Before(when) {
					ready = append(ready, nodeByID[id])
					delete(pendingRetry, id)
				}
			}
		}

		// 派发就绪任务（使用 head 指针避免 slice 拷贝）
		for readyHead < len(ready) && running < s.cfg.MaxConcurrency && !failed {
			node := ready[readyHead]
			readyHead++
			node.Status = StepRunning
			running++
			input := buildStepInput(node.Step, initialInput, outputs, s.plan.DepGraph)
			s.pool.Submit(ctx, node, input, resultsCh)
		}
		// 回收已消费的就绪队列内存
		if readyHead > 0 && readyHead == len(ready) {
			ready = ready[:0]
			readyHead = 0
		}

		// 若已无运行中、就绪、重试中的任务，说明无法继续推进
		if running == 0 && len(pendingRetry) == 0 && readyHead >= len(ready) {
			break
		}

		// 优化（perf-v2）：仅查找最早的重试时间，避免每轮全量遍历
		var nextRetry time.Time
		hasPending := false
		for _, when := range pendingRetry {
			if !hasPending || when.Before(nextRetry) {
				nextRetry = when
				hasPending = true
			}
		}
		if hasPending {
			retryTimer.Reset(time.Until(nextRetry))
		} else {
			retryTimer.Stop()
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-retryTimer.C:
			// 下一轮循环会把到期重试加入 ready
		case result := <-resultsCh:
			running--
			node := nodeByID[result.StepID]
			node.Result = result
			node.Status = result.Status

			if result.Status == StepCompleted {
				completed++
				outputs[node.Step.ID] = result.Output
				newlyReady := s.plan.DepGraph.Complete(node.Step.ID)
				for _, id := range newlyReady {
					ready = append(ready, nodeByID[id])
				}
			} else if result.Status == StepFailed {
				maxRetries := s.cfg.RetryPolicy.MaxRetries
				if node.Step.RetryPolicy != nil && node.Step.RetryPolicy.MaxRetries > 0 {
					maxRetries = node.Step.RetryPolicy.MaxRetries
				}
				if maxRetries <= 0 {
					maxRetries = defaultMaxRetries
				}
				if retryCount[node.Step.ID] < maxRetries {
					retryCount[node.Step.ID]++
					node.Result.RetryCount = retryCount[node.Step.ID]
					backoff := s.cfg.RetryPolicy.Backoff
					if node.Step.RetryPolicy != nil && node.Step.RetryPolicy.Backoff > 0 {
						backoff = node.Step.RetryPolicy.Backoff
					}
					if backoff <= 0 {
						backoff = defaultBackoff
					}
					backoff = backoff * time.Duration(retryCount[node.Step.ID])
					pendingRetry[node.Step.ID] = time.Now().Add(backoff)
					continue
				}

				completed++
				if s.cfg.FailFast {
					failed = true
				} else {
					// continue-on-error: 仍然解锁下游，但下游会收到 StepSkipped
					newlyReady := s.plan.DepGraph.Complete(node.Step.ID)
					for _, id := range newlyReady {
						ready = append(ready, nodeByID[id])
					}
				}
			}
		}
	}

	results := make(map[string]*StepResult, len(s.plan.Steps))
	for _, n := range s.plan.Steps {
		if n.Result == nil {
			n.Status = StepSkipped
			n.Result = &StepResult{StepID: n.Step.ID, StepName: n.Step.Name, Status: StepSkipped}
		}
		results[n.Step.ID] = n.Result
	}

	if failed {
		return results, fmt.Errorf("one or more steps failed")
	}
	return results, nil
}

// buildStepInput 构建 step 输入数据。
// 优化（perf-v2）：预分配 map 容量，减少 rehash。
func buildStepInput(step *AgentStep, initialInput map[string]any, outputs map[string]map[string]any, g *DependencyGraph) map[string]any {
	// 估算容量：初始输入 + 依赖边数 × 平均字段数
	capEstimate := len(initialInput) + len(g.inEdges[step.ID])*4
	if capEstimate < 8 {
		capEstimate = 8
	}
	input := make(map[string]any, capEstimate)
	for k, v := range initialInput {
		input[k] = v
	}
	// 按依赖边合并上游输出
	for _, depID := range g.inEdges[step.ID] {
		out, ok := outputs[depID]
		if !ok {
			continue
		}
		for _, key := range step.InputFrom {
			if v, ok := out[key]; ok {
				input[key] = v
			}
		}
		// 若未指定 InputFrom，默认合并所有上游输出
		if len(step.InputFrom) == 0 {
			for k, v := range out {
				input[k] = v
			}
		}
	}
	return input
}
