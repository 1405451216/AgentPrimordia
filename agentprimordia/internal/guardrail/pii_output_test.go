package guardrail

import (
	"strings"
	"testing"
)

// TestPIIRule_OutputCheckpoint 验证 PII 规则在 output 检查点生效
// 场景：模拟 LLM 响应中包含 PII，应在 output 检查点被拦截/脱敏
func TestPIIRule_OutputCheckpoint(t *testing.T) {
	rule := NewPIIRule(DefaultPIIRuleConfig())

	// LLM 响应中包含邮箱和手机号
	output := "用户邮箱是 zhangsan@example.com，电话 13812345678"
	result, err := rule.Check(output, CheckOutput)
	if err != nil {
		t.Fatalf("检查失败: %v", err)
	}
	if result.Action != ActionSanitize {
		t.Errorf("Action = %v, 期望 ActionSanitize", result.Action)
	}
	if strings.Contains(result.Sanitized, "zhangsan@example.com") {
		t.Error("输出端 PII 未被脱敏：邮箱仍存在")
	}
	if strings.Contains(result.Sanitized, "13812345678") {
		t.Error("输出端 PII 未被脱敏：手机号仍存在")
	}
}

// TestPIIRule_OutputCheckpoint_NoPII 验证无 PII 时不触发脱敏
func TestPIIRule_OutputCheckpoint_NoPII(t *testing.T) {
	rule := NewPIIRule(DefaultPIIRuleConfig())

	output := "这是一段正常的回复文本，不包含任何敏感信息。"
	result, err := rule.Check(output, CheckOutput)
	if err != nil {
		t.Fatalf("检查失败: %v", err)
	}
	if result.Action != ActionPass {
		t.Errorf("Action = %v, 期望 ActionPass", result.Action)
	}
}

// TestPIIRule_OutputCheckpoint_Reject 验证 reject 动作在 output 端生效
func TestPIIRule_OutputCheckpoint_Reject(t *testing.T) {
	rule := NewPIIRule(PIIRuleConfig{
		Action:      ActionReject,
		Severity:    SeverityCritical,
		DetectPhone: true,
	})

	output := "回复用户的电话 13812345678"
	result, err := rule.Check(output, CheckOutput)
	if err != nil {
		t.Fatalf("检查失败: %v", err)
	}
	if result.Action != ActionReject {
		t.Errorf("Action = %v, 期望 ActionReject", result.Action)
	}
}

// TestEngine_CheckOutput 验证 Engine 的 CheckOutput 方法调用 PII 规则
func TestEngine_CheckOutput(t *testing.T) {
	engine := NewEngine()
	engine.AddRule(NewPIIRule(DefaultPIIRuleConfig()))

	output := "用户邮箱 zhangsan@example.com"
	report, err := engine.CheckOutput(output)
	if err != nil {
		t.Fatalf("CheckOutput 失败: %v", err)
	}
	if report.Action != ActionSanitize {
		t.Errorf("report.Action = %v, 期望 ActionSanitize", report.Action)
	}
	if len(report.Results) == 0 {
		t.Fatal("期望至少有一个规则结果")
	}
	// 验证 Sanitized 字段不为空
	for _, r := range report.Results {
		if r.Action == ActionSanitize && r.Sanitized == "" {
			t.Error("PII 规则未返回 Sanitized 文本")
		}
	}
}

// TestSanitizeRule_OutputCheckpoint 验证 SanitizeRule 在 output 端生效
func TestSanitizeRule_OutputCheckpoint(t *testing.T) {
	rule := NewSanitizeRule(SanitizeConfig{ReplaceWith: "[REDACTED]"})

	output := "用户邮箱是 zhangsan@example.com，电话 13812345678"
	result, err := rule.Check(output, CheckOutput)
	if err != nil {
		t.Fatalf("检查失败: %v", err)
	}
	if result.Action != ActionSanitize {
		t.Errorf("Action = %v, 期望 ActionSanitize", result.Action)
	}
	if strings.Contains(result.Sanitized, "zhangsan@example.com") {
		t.Error("输出端 PII 未被脱敏：邮箱仍存在")
	}
	if strings.Contains(result.Sanitized, "13812345678") {
		t.Error("输出端 PII 未被脱敏：手机号仍存在")
	}
	if !strings.Contains(result.Sanitized, "[REDACTED]") {
		t.Error("Sanitized 文本中应包含 [REDACTED] 标记")
	}
}
