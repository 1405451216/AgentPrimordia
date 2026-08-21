package autonomy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ReplanPlanner 重规划器接口（由外部 LLM 规划器实现）
type ReplanPlanner interface {
	// Replan 根据失败步骤和原因生成新的执行步骤
	Replan(ctx context.Context, goal *AgentGoal, failedSteps []PlanStep, reason string) ([]PlanStep, error)
}

// ReplannerConfig 重规划器配置
type ReplannerConfig struct {
	// Planner 重规划器实现
	Planner ReplanPlanner
	// MaxReplans 最大重规划次数（默认 3）
	MaxReplans int
}

// ReplanRecord 重规划记录
type ReplanRecord struct {
	// Reason 重规划根因
	Reason string
	// FailedSteps 触发重规划的失败步骤
	FailedSteps []PlanStep
	// Timestamp 时间戳
	Timestamp time.Time
	// NewPlanVersion 新计划版本
	NewPlanVersion int
}

// ErrGoalBudgetExceeded 目标级成本预算耗尽（v4.9-4）。
var ErrGoalBudgetExceeded = errors.New("autonomy: 目标成本预算耗尽")

// ErrGoalNotPaused 目标未处于暂停态（v5.1 调度质量：Resume 前置校验）。
var ErrGoalNotPaused = errors.New("autonomy: 目标未处于暂停态")

// Replanner 校验与再计划引擎
type Replanner struct {
	mu       sync.Mutex
	cfg      ReplannerConfig
	count    int
	history  []ReplanRecord
}

// NewReplanner 创建重规划器
func NewReplanner(cfg ReplannerConfig) *Replanner {
	if cfg.MaxReplans <= 0 {
		cfg.MaxReplans = 3
	}
	return &Replanner{cfg: cfg}
}

// Trigger 触发重规划：校验不达标时自动重新规划剩余步骤
func (r *Replanner) Trigger(ctx context.Context, goal *AgentGoal, failedSteps []PlanStep, reason string) (*GoalPlan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.count >= r.cfg.MaxReplans {
		return nil, fmt.Errorf("autonomy: 重规划次数已达上限 %d", r.cfg.MaxReplans)
	}
	// v4.9-4 目标级预算：重规划前记账，超预算 → 拒绝（目标保留失败态由调用方处理）
	if err := goal.Charge(goal.ReplanCost()); err != nil {
		return nil, err
	}

	newSteps, err := r.cfg.Planner.Replan(ctx, goal, failedSteps, reason)
	if err != nil {
		return nil, fmt.Errorf("autonomy: 重规划失败: %w", err)
	}

	r.count++
	newPlan := NewGoalPlan(goal.ID, newSteps)
	newPlan.Version = r.count + 1 // 版本递增
	newPlan.ReplanReason = reason

	r.history = append(r.history, ReplanRecord{
		Reason:         reason,
		FailedSteps:    failedSteps,
		Timestamp:      time.Now(),
		NewPlanVersion: newPlan.Version,
	})

	return newPlan, nil
}

// History 返回重规划历史（副本）
func (r *Replanner) History() []ReplanRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := make([]ReplanRecord, len(r.history))
	copy(h, r.history)
	return h
}

// Count 返回已重规划次数
func (r *Replanner) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

// ValidationResult 校验结果
type ValidationResult struct {
	// Passed 是否全部通过
	Passed bool
	// FailedCriteria 未通过的验收标准
	FailedCriteria []string
}

// Validator 目标结果校验器
type Validator struct{}

// NewValidator 创建校验器
func NewValidator() *Validator {
	return &Validator{}
}

// Validate 校验目标执行结果是否满足验收标准
// criteriaResults: 验收标准 → 是否通过
func (v *Validator) Validate(criteria []string, criteriaResults map[string]bool) ValidationResult {
	var failed []string
	for _, c := range criteria {
		if !criteriaResults[c] {
			failed = append(failed, c)
		}
	}
	return ValidationResult{
		Passed:         len(failed) == 0,
		FailedCriteria: failed,
	}
}
