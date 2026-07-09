// Package governance 实现"策略即代码"：用 YAML 定义 Agent 行为策略，
// 运行时通过 PolicyEnforcer 强制执行（工具限制 / 成本限制 / 输出护栏 / 行为约束）。
//
// 设计取舍：
//   - 策略以 YAML 描述，加载期解析为 Policy 结构；
//   - 运行时通过 Enforcer 接口暴露检查能力，供上层（agent 包）依赖，
//     避免 governance 反向引用 agent（遵循模块依赖方向）；
//   - 并发安全：PolicyEnforcer 用 sync.Mutex 保护运行时计数与累计成本。
package governance

import (
	"errors"
	"regexp"
)

// 策略执行相关错误
var (
	// ErrToolCallLimitExceeded 单次运行内某工具调用次数/会话总调用超过上限
	ErrToolCallLimitExceeded = errors.New("tool call limit exceeded")
	// ErrCostLimitExceeded 成本超过策略上限
	ErrCostLimitExceeded = errors.New("cost limit exceeded")
	// ErrBlockedArgument 工具参数命中禁止模式
	ErrBlockedArgument = errors.New("blocked argument pattern")
	// ErrOutputTooLong 输出长度超过策略上限
	ErrOutputTooLong = errors.New("output exceeds max length")
	// ErrPIIDetected 输出触发 strict PII 过滤
	ErrPIIDetected = errors.New("output triggered strict PII filter")
	// ErrPolicyNotFound 未加载策略
	ErrPolicyNotFound = errors.New("policy not loaded")
)

// 常见 PII 模式（strict 模式下的简化检测）
var (
	rePhone = regexp.MustCompile(`1[3-9]\d{9}`)
	reID    = regexp.MustCompile(`\d{17}[\dXx]`)
)

// Policy 顶层策略定义
type Policy struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Metadata   PolicyMetadata `yaml:"metadata"`
	Spec       PolicySpec     `yaml:"spec"`
}

// PolicyMetadata 策略元信息
type PolicyMetadata struct {
	Name string `yaml:"name"`
}

// PolicySpec 策略规约
type PolicySpec struct {
	ToolRestrictions    []ToolRestriction   `yaml:"toolRestrictions"`
	CostLimits          CostLimits          `yaml:"costLimits"`
	OutputGuardrail     OutputGuardrail     `yaml:"outputGuardrail"`
	BehaviorConstraints BehaviorConstraints `yaml:"behaviorConstraints"`
}

// ToolRestriction 单工具限制
type ToolRestriction struct {
	Tool            string   `yaml:"tool"`
	RequireApproval bool     `yaml:"requireApproval"`
	MaxCallsPerRun  int      `yaml:"maxCallsPerRun"`
	BlockedArgs     []string `yaml:"blockedArgs"`
	AllowedDomains  []string `yaml:"allowedDomains"`
}

// CostLimits 成本限制（美元）
type CostLimits struct {
	PerRequest float64 `yaml:"perRequest"`
	PerDay     float64 `yaml:"perDay"`
	PerSession float64 `yaml:"perSession"`
}

// OutputGuardrail 输出护栏
type OutputGuardrail struct {
	PIIFilter      string `yaml:"piiFilter"`      // strict | moderate | off
	InjectionBlock bool   `yaml:"injectionBlock"` // 是否拦截注入
	MaxLength      int    `yaml:"maxLength"`
}

// BehaviorConstraints 行为约束
type BehaviorConstraints struct {
	MaxTurns          int  `yaml:"maxTurns"`
	MaxToolCalls      int  `yaml:"maxToolCalls"`
	RequireReflection bool `yaml:"requireReflection"`
}
