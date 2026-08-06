// Package skills 实现技能进化模型（v3.4 核心）。
// 提供技能的抽象、习得、验证、存储、匹配能力，
// 使 Agent 从"工具是静态注册的"变成"越用越强"。
package skills

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// SkillStatus 技能状态
type SkillStatus string

const (
	// SkillDraft 草稿（习得但未验证）
	SkillDraft SkillStatus = "draft"
	// SkillVerified 已验证（通过测试门）
	SkillVerified SkillStatus = "verified"
	// SkillActive 活跃（可被匹配调用）
	SkillActive SkillStatus = "active"
	// SkillDeprecated 已弃用
	SkillDeprecated SkillStatus = "deprecated"
)

// StepDef 技能中的单个步骤定义
type StepDef struct {
	// ID 步骤标识
	ID string `json:"id"`
	// Description 步骤描述
	Description string `json:"description"`
	// ToolName 调用的工具名
	ToolName string `json:"tool_name"`
	// InputMapping 输入参数映射（从技能输入到工具参数）
	InputMapping map[string]string `json:"input_mapping,omitempty"`
	// OutputKey 输出存储键
	OutputKey string `json:"output_key,omitempty"`
	// DependsOn 依赖的步骤 ID
	DependsOn []string `json:"depends_on,omitempty"`
}

// IOSchema 输入/输出 schema 定义
type IOSchema struct {
	// Fields 字段定义（名称 → 类型描述）
	Fields map[string]string `json:"fields"`
	// Required 必填字段
	Required []string `json:"required,omitempty"`
}

// Skill 技能抽象：可复用的多步骤能力单元
type Skill struct {
	// ID 全局唯一标识
	ID string `json:"id"`
	// Name 技能名称
	Name string `json:"name"`
	// Description 技能描述
	Description string `json:"description"`
	// Version 语义化版本
	Version Version `json:"version"`
	// Status 当前状态
	Status SkillStatus `json:"status"`
	// Steps 有序步骤列表
	Steps []StepDef `json:"steps"`
	// Input 输入 schema
	Input IOSchema `json:"input"`
	// Output 输出 schema
	Output IOSchema `json:"output"`
	// Dependencies 依赖的其他技能 ID
	Dependencies []string `json:"dependencies,omitempty"`
	// Tags 标签（用于匹配）
	Tags []string `json:"tags,omitempty"`
	// Metadata 附加元数据
	Metadata map[string]string `json:"metadata,omitempty"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updated_at"`
	// SuccessRate 历史成功率
	SuccessRate float64 `json:"success_rate"`
	// UsageCount 调用次数
	UsageCount int `json:"usage_count"`
}

// NewSkill 创建新技能
func NewSkill(name string, description string, steps []StepDef) *Skill {
	return &Skill{
		ID:          generateSkillID(),
		Name:        name,
		Description: description,
		Version:     Version{Major: 1, Minor: 0, Patch: 0},
		Status:      SkillDraft,
		Steps:       steps,
		Input:       IOSchema{Fields: make(map[string]string)},
		Output:      IOSchema{Fields: make(map[string]string)},
		Metadata:    make(map[string]string),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// Activate 激活技能（验证通过后可被匹配）
func (s *Skill) Activate() {
	s.Status = SkillActive
	s.UpdatedAt = time.Now()
}

// Deprecate 弃用技能
func (s *Skill) Deprecate() {
	s.Status = SkillDeprecated
	s.UpdatedAt = time.Now()
}

// RecordUsage 记录一次调用
func (s *Skill) RecordUsage(success bool) {
	s.UsageCount++
	if success {
		s.SuccessRate = (s.SuccessRate*float64(s.UsageCount-1) + 1) / float64(s.UsageCount)
	} else {
		s.SuccessRate = (s.SuccessRate * float64(s.UsageCount-1)) / float64(s.UsageCount)
	}
	s.UpdatedAt = time.Now()
}

// generateSkillID 生成唯一技能 ID
func generateSkillID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "skill-" + hex.EncodeToString(b)
}
