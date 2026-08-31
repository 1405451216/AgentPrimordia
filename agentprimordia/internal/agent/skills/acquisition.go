package skills

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// ToolCallRecord 工具调用轨迹记录
type ToolCallRecord struct {
	// ToolName 工具名
	ToolName string `json:"tool_name"`
	// Input 输入参数
	Input map[string]any `json:"input"`
	// Output 输出结果
	Output string `json:"output"`
	// Success 是否成功
	Success bool `json:"success"`
	// Duration 耗时
	Duration time.Duration `json:"duration"`
	// Timestamp 时间戳
	Timestamp time.Time `json:"timestamp"`
}

// Trajectory 成功工具调用序列（技能习得原料）
type Trajectory struct {
	// TaskDescription 任务描述
	TaskDescription string `json:"task_description"`
	// Records 有序调用记录
	Records []ToolCallRecord `json:"records"`
	// Success 整体是否成功
	Success bool `json:"success"`
	// Timestamp 时间戳
	Timestamp time.Time `json:"timestamp"`
}

// SkillDistiller LLM 提炼接口（由外部 learning/llm_distiller.go 适配）
type SkillDistiller interface {
	// Distill 从轨迹中提炼可复用技能
	Distill(ctx context.Context, trajectory Trajectory) (*Skill, error)
}

// SkillGuardrail 习得技能描述护栏（v4.1 集成2：复用 guardrail/ 引擎的适配点）。
type SkillGuardrail interface {
	// SanitizeSkillDescription 校验/脱敏技能描述；错误 → 习得失败
	SanitizeSkillDescription(ctx context.Context, description string) (string, error)
}

// AcquisitionMetrics 技能习得目标级指标（v4.1 集成1：复用 metrics/ 的适配点）。
type AcquisitionMetrics interface {
	// RecordAcquire 记录一次习得（成功/失败 + 耗时）
	RecordAcquire(skillID string, success bool, duration time.Duration)
}

// TrajectoryMemorySink 习得轨迹记忆出口（v4.1 集成3：复用 memory/ 的适配点）。
type TrajectoryMemorySink interface {
	// SaveTrajectory 保存一条成功轨迹
	SaveTrajectory(ctx context.Context, t Trajectory) error
}

// Acquisition 技能习得流水线
type Acquisition struct {
	distiller      SkillDistiller
	trajectories   []Trajectory
	validator      *Validator
	guardrail      SkillGuardrail
	metrics        AcquisitionMetrics
	trajectorySink TrajectoryMemorySink
	sinkErrCount   atomic.Int64
}

// NewAcquisition 创建习得流水线
func NewAcquisition(distiller SkillDistiller) *Acquisition {
	return &Acquisition{
		distiller: distiller,
		validator: NewValidator(),
	}
}

// SetGuardrail 注入技能描述护栏（nil 跳过；护栏报错 → 习得失败）。
func (a *Acquisition) SetGuardrail(g SkillGuardrail) { a.guardrail = g }

// SetMetrics 注入习得指标记录器（nil 跳过）。
func (a *Acquisition) SetMetrics(m AcquisitionMetrics) { a.metrics = m }

// SetTrajectorySink 注入轨迹记忆出口（nil 跳过；写入失败计入 SinkErrorCount）。
func (a *Acquisition) SetTrajectorySink(s TrajectoryMemorySink) { a.trajectorySink = s }

// SinkErrorCount 返回轨迹记忆写入失败次数。
func (a *Acquisition) SinkErrorCount() int64 { return a.sinkErrCount.Load() }

// RecordTrajectory 记录成功轨迹（同时经轨迹记忆出口沉淀，v4.1 集成3）。
func (a *Acquisition) RecordTrajectory(t Trajectory) {
	a.trajectories = append(a.trajectories, t)
	if a.trajectorySink != nil {
		if err := a.trajectorySink.SaveTrajectory(context.Background(), t); err != nil {
			a.sinkErrCount.Add(1)
		}
	}
}

// Acquire 从最近轨迹中习得技能：提炼 → 护栏 → 规范校验 → 指标记录。
func (a *Acquisition) Acquire(ctx context.Context, trajectory Trajectory) (*Skill, error) {
	start := time.Now()
	skill, err := a.distiller.Distill(ctx, trajectory)
	if err != nil {
		a.recordAcquire("", false, start)
		return nil, err
	}
	// v4.1 集成2：习得技能描述过护栏（错误 → 习得失败，防恶意技能入库）
	if a.guardrail != nil {
		sanitized, gerr := a.guardrail.SanitizeSkillDescription(ctx, skill.Description)
		if gerr != nil {
			a.recordAcquire(skill.ID, false, start)
			return nil, fmt.Errorf("skills: 习得技能描述护栏拒绝: %w", gerr)
		}
		skill.Description = sanitized
	}
	// 校验新技能规范
	if err := a.validator.Validate(skill); err != nil {
		a.recordAcquire(skill.ID, false, start)
		return nil, err
	}
	skill.Status = SkillDraft
	a.recordAcquire(skill.ID, true, start)
	return skill, nil
}

// recordAcquire 记录习得结果指标（Metrics 未注入时跳过）。
func (a *Acquisition) recordAcquire(skillID string, success bool, start time.Time) {
	if a.metrics != nil {
		a.metrics.RecordAcquire(skillID, success, time.Since(start))
	}
}

// TrajectoryCount 返回已记录轨迹数
func (a *Acquisition) TrajectoryCount() int {
	return len(a.trajectories)
}
