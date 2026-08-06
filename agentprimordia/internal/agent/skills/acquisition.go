package skills

import (
	"context"
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

// Acquisition 技能习得流水线
type Acquisition struct {
	distiller    SkillDistiller
	trajectories []Trajectory
	validator    *Validator
}

// NewAcquisition 创建习得流水线
func NewAcquisition(distiller SkillDistiller) *Acquisition {
	return &Acquisition{
		distiller: distiller,
		validator: NewValidator(),
	}
}

// RecordTrajectory 记录成功轨迹
func (a *Acquisition) RecordTrajectory(t Trajectory) {
	a.trajectories = append(a.trajectories, t)
}

// Acquire 从最近轨迹中习得技能
func (a *Acquisition) Acquire(ctx context.Context, trajectory Trajectory) (*Skill, error) {
	skill, err := a.distiller.Distill(ctx, trajectory)
	if err != nil {
		return nil, err
	}
	// 校验新技能规范
	if err := a.validator.Validate(skill); err != nil {
		return nil, err
	}
	skill.Status = SkillDraft
	return skill, nil
}

// TrajectoryCount 返回已记录轨迹数
func (a *Acquisition) TrajectoryCount() int {
	return len(a.trajectories)
}
