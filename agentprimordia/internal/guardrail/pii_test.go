package guardrail

import (
	"strings"
	"testing"
)

// TestPIIDetector_Email 检测邮箱地址
func TestPIIDetector_Email(t *testing.T) {
	d := NewPIIDetector()
	result := d.Detect("contact me at user@example.com please")
	if len(result.Findings) == 0 {
		t.Fatal("expected to find email PII")
	}
	found := false
	for _, f := range result.Findings {
		if f.Type == Email {
			found = true
			if f.Value != "user@example.com" {
				t.Errorf("email value = %q, want %q", f.Value, "user@example.com")
			}
		}
	}
	if !found {
		t.Error("expected to find Email type in findings")
	}
}

// TestPIIDetector_Phone 检测中国手机号
func TestPIIDetector_Phone(t *testing.T) {
	d := NewPIIDetector()
	result := d.Detect("my phone is 13812345678")
	if len(result.Findings) == 0 {
		t.Fatal("expected to find phone PII")
	}
	found := false
	for _, f := range result.Findings {
		if f.Type == Phone {
			found = true
			if f.Value != "13812345678" {
				t.Errorf("phone value = %q, want %q", f.Value, "13812345678")
			}
		}
	}
	if !found {
		t.Error("expected to find Phone type in findings")
	}
}

// TestPIIDetector_IDCard 检测中国身份证号
func TestPIIDetector_IDCard(t *testing.T) {
	d := NewPIIDetector()
	result := d.Detect("id number: 110101199001011234")
	if len(result.Findings) == 0 {
		t.Fatal("expected to find ID card PII")
	}
	found := false
	for _, f := range result.Findings {
		if f.Type == IDCard {
			found = true
			if f.Value != "110101199001011234" {
				t.Errorf("id card value = %q, want %q", f.Value, "110101199001011234")
			}
		}
	}
	if !found {
		t.Error("expected to find IDCard type in findings")
	}
}

// TestPIIDetector_Sanitize 替换 PII 为 [REDACTED]
func TestPIIDetector_Sanitize(t *testing.T) {
	d := NewPIIDetector()
	cfg := SanitizeConfig{ReplaceWith: "[REDACTED]"}

	// 测试单个 PII 替换
	text := "email: user@example.com end"
	sanitized := d.Sanitize(text, cfg)
	if strings.Contains(sanitized, "user@example.com") {
		t.Errorf("sanitized text still contains PII: %q", sanitized)
	}
	if !strings.Contains(sanitized, "[REDACTED]") {
		t.Errorf("sanitized text missing [REDACTED]: %q", sanitized)
	}

	// 测试多个 PII 替换
	text2 := "phone 13812345678 and email test@foo.com"
	sanitized2 := d.Sanitize(text2, cfg)
	if strings.Contains(sanitized2, "13812345678") || strings.Contains(sanitized2, "test@foo.com") {
		t.Errorf("sanitized text still contains PII: %q", sanitized2)
	}
	// 应该有两个 [REDACTED]
	count := strings.Count(sanitized2, "[REDACTED]")
	if count != 2 {
		t.Errorf("expected 2 [REDACTED] occurrences, got %d in %q", count, sanitized2)
	}
}

// TestPIIDetector_NoFalsePositive 正常文本不应产生误报
func TestPIIDetector_NoFalsePositive(t *testing.T) {
	d := NewPIIDetector()
	result := d.Detect("hello world, this is a normal sentence without any PII")
	if len(result.Findings) > 0 {
		t.Errorf("normal text should have no findings, got %d: %+v", len(result.Findings), result.Findings)
	}
}

// TestSanitizeRule_Integration SanitizeRule 实现 Rule 接口并与 Engine 集成
func TestSanitizeRule_Integration(t *testing.T) {
	rule := NewSanitizeRule(SanitizeConfig{ReplaceWith: "[REDACTED]"})

	// 验证 Rule 接口
	if rule.Name() != "pii-sanitize" {
		t.Errorf("rule name = %q, want %q", rule.Name(), "pii-sanitize")
	}

	// 检测含 PII 的输入
	result, err := rule.Check("contact user@example.com", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionSanitize {
		t.Errorf("action = %q, want %q", result.Action, ActionSanitize)
	}
	if result.Sanitized == "" {
		t.Error("expected non-empty sanitized output")
	}
	if strings.Contains(result.Sanitized, "user@example.com") {
		t.Errorf("sanitized still contains PII: %q", result.Sanitized)
	}

	// 无 PII 时应通过
	result2, err := rule.Check("hello world", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result2.Action != ActionPass {
		t.Errorf("action for clean text = %q, want %q", result2.Action, ActionPass)
	}

	// 与 Engine 集成测试
	engine := NewEngine()
	engine.AddRule(rule)
	report, err := engine.Check("email: user@example.com", CheckInput)
	if err != nil {
		t.Fatalf("engine check error: %v", err)
	}
	if report.Passed {
		t.Error("engine should not pass when PII is present")
	}
	if report.Action != ActionSanitize {
		t.Errorf("engine action = %q, want %q", report.Action, ActionSanitize)
	}
}
