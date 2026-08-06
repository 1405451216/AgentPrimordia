// Stability: Experimental — v3.4.0 新增技能进化能力，API 可能随使用场景演进而调整。
package ap

import (
	"agentprimordia/internal/agent/skills"
)

// --- 核心类型导出 ---

// Skill 技能抽象
type Skill = skills.Skill

// StepDef 技能步骤定义
type StepDef = skills.StepDef

// IOSchema 输入/输出 schema
type IOSchema = skills.IOSchema

// SkillStatus 技能状态
type SkillStatus = skills.SkillStatus

// Store 技能库
type SkillStore = skills.Store

// Matcher 技能匹配器
type SkillMatcher = skills.Matcher

// MatchResult 匹配结果
type SkillMatchResult = skills.MatchResult

// Acquisition 技能习得流水线
type SkillAcquisition = skills.Acquisition

// UsageTracker 技能使用追踪器
type SkillUsageTracker = skills.UsageTracker

// --- 状态常量导出 ---

const (
	// SkillDraft 草稿
	SkillDraft = skills.SkillDraft
	// SkillVerified 已验证
	SkillVerified = skills.SkillVerified
	// SkillActive 活跃
	SkillActive = skills.SkillActive
	// SkillDeprecated 已弃用
	SkillDeprecated = skills.SkillDeprecated
)

// --- 构造器导出 ---

var (
	// NewSkill 创建新技能
	NewSkill = skills.NewSkill
	// NewSkillStore 创建技能库
	NewSkillStore = skills.NewStore
	// NewSkillMatcher 创建技能匹配器
	NewSkillMatcher = skills.NewMatcher
	// NewSkillUsageTracker 创建使用追踪器
	NewSkillUsageTracker = skills.NewUsageTracker
)
