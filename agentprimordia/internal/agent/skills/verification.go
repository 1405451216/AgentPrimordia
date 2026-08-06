package skills

import (
	"context"
	"fmt"
)

// TestCase 技能验证测试用例
type TestCase struct {
	// Name 用例名称
	Name string `json:"name"`
	// Input 测试输入
	Input map[string]any `json:"input"`
	// ExpectedOutput 期望输出（关键字段匹配）
	ExpectedOutput map[string]any `json:"expected_output"`
}

// VerificationResult 验证结果
type VerificationResult struct {
	// Passed 是否全部通过
	Passed bool
	// Total 总用例数
	Total int
	// PassedCount 通过数
	PassedCount int
	// Failures 失败详情
	Failures []string
}

// SkillExecutor 技能执行器接口（验证时调用）
type SkillExecutor interface {
	Execute(ctx context.Context, skill *Skill, input map[string]any) (map[string]any, error)
}

// Verification 技能验证门：新技能必须通过测试用例才可启用
type Verification struct {
	executor SkillExecutor
}

// NewVerification 创建验证器
func NewVerification(executor SkillExecutor) *Verification {
	return &Verification{executor: executor}
}

// Verify 运行测试用例验证技能
func (v *Verification) Verify(ctx context.Context, skill *Skill, cases []TestCase) VerificationResult {
	result := VerificationResult{Total: len(cases)}

	for _, tc := range cases {
		output, err := v.executor.Execute(ctx, skill, tc.Input)
		if err != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("%s: 执行错误 %v", tc.Name, err))
			continue
		}
		if matchExpected(output, tc.ExpectedOutput) {
			result.PassedCount++
		} else {
			result.Failures = append(result.Failures, fmt.Sprintf("%s: 输出不匹配", tc.Name))
		}
	}

	result.Passed = result.PassedCount == result.Total
	return result
}

// matchExpected 检查输出是否匹配期望
func matchExpected(output map[string]any, expected map[string]any) bool {
	for k, v := range expected {
		ov, ok := output[k]
		if !ok || ov != v {
			return false
		}
	}
	return true
}
