package guardrail

import (
	"strings"
	"testing"
)

// TestPIIDetector_Passport 验证护照号检测
func TestPIIDetector_Passport(t *testing.T) {
	d := NewPIIDetector()

	// 中国护照：E + 8位数字
	dr := d.Detect("我的护照号是 E12345678")
	if len(dr.Findings) == 0 {
		t.Fatal("未检测到中国护照号")
	}
	found := false
	for _, f := range dr.Findings {
		if f.Type == Passport {
			found = true
			if f.Value != "E12345678" {
				t.Errorf("Passport value = %q, want E12345678", f.Value)
			}
		}
	}
	if !found {
		t.Error("未检测到 Passport 类型")
	}
}

// TestPIIDetector_BankAccount 验证银行账号检测
func TestPIIDetector_BankAccount(t *testing.T) {
	d := NewPIIDetector()

	dr := d.Detect("账号 6222021234567890123")
	found := false
	for _, f := range dr.Findings {
		if f.Type == BankAccount {
			found = true
		}
	}
	if !found {
		t.Error("未检测到 BankAccount 类型")
	}
}

// TestPIIDetector_SSN 验证美国社保号检测
func TestPIIDetector_SSN(t *testing.T) {
	d := NewPIIDetector()

	dr := d.Detect("SSN is 123-45-6789")
	found := false
	for _, f := range dr.Findings {
		if f.Type == SSN {
			found = true
			if f.Value != "123-45-6789" {
				t.Errorf("SSN value = %q, want 123-45-6789", f.Value)
			}
		}
	}
	if !found {
		t.Error("未检测到 SSN 类型")
	}
}

// TestPIIDetector_APIKey 验证 API Key 检测
func TestPIIDetector_APIKey(t *testing.T) {
	d := NewPIIDetector()

	// OpenAI 风格
	dr := d.Detect("使用 sk-abcdefghijklmnopqrstuvwxyz123456 进行调用")
	found := false
	for _, f := range dr.Findings {
		if f.Type == APIKey {
			found = true
		}
	}
	if !found {
		t.Error("未检测到 sk- 前缀 API Key")
	}

	// Stripe 风格
	dr2 := d.Detect("Stripe key: pk_test_abcdefghijklmnopqrstuv")
	found2 := false
	for _, f := range dr2.Findings {
		if f.Type == APIKey {
			found2 = true
		}
	}
	if !found2 {
		t.Error("未检测到 pk_ 前缀 API Key")
	}

	// AWS 风格
	dr3 := d.Detect("AWS access key: AKIAIOSFODNN7EXAMPLE")
	found3 := false
	for _, f := range dr3.Findings {
		if f.Type == APIKey {
			found3 = true
		}
	}
	if !found3 {
		t.Error("未检测到 AKIA 前缀 API Key")
	}
}

// TestPIIDetector_JWT 验证 JWT Token 检测
func TestPIIDetector_JWT(t *testing.T) {
	d := NewPIIDetector()

	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	dr := d.Detect("token: " + jwt)
	found := false
	for _, f := range dr.Findings {
		if f.Type == JWT {
			found = true
			if f.Value != jwt {
				t.Errorf("JWT value 不匹配")
			}
		}
	}
	if !found {
		t.Error("未检测到 JWT Token")
	}
}

// TestPIIRule_ExtendedTypes 验证 PIIRule 对所有扩展类型的支持
func TestPIIRule_ExtendedTypes(t *testing.T) {
	rule := NewPIIRule(DefaultPIIRuleConfig())

	tests := []struct {
		name     string
		input    string
		expected string // 期望出现在 result.Metadata["types"] 中
	}{
		{"passport", "护照 E12345678", "passport"},
		{"ssn", "SSN 123-45-6789", "ssn"},
		{"api_key", "key sk-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "api_key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := rule.Check(tt.input, CheckInput)
			if err != nil {
				t.Fatalf("Check error: %v", err)
			}
			if result.Action != ActionSanitize {
				t.Errorf("Action = %v, want ActionSanitize", result.Action)
			}
			if result.Metadata == nil {
				t.Fatal("Metadata 为空")
			}
			types, ok := result.Metadata["types"].([]string)
			if !ok {
				t.Fatal("Metadata[types] 类型不正确")
			}
			found := false
			for _, typ := range types {
				if typ == tt.expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("未检测到类型 %s，实际检测到 %v", tt.expected, types)
			}
		})
	}
}

// TestPIIRule_DisabledTypes 验证禁用某些类型时不会被检测
func TestPIIRule_DisabledTypes(t *testing.T) {
	rule := NewPIIRule(PIIRuleConfig{
		Action:       ActionSanitize,
		Severity:     SeverityHigh,
		DetectEmail:  true,
		DetectPhone:  false, // 显式禁用
		DetectSSN:    true,
		DetectAPIKey: false, // 显式禁用
	})

	// 包含手机号和 SSN，应只检测到 SSN
	result, err := rule.Check("电话 13812345678, SSN 123-45-6789", CheckInput)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if result.Action != ActionSanitize {
		t.Errorf("Action = %v, want ActionSanitize", result.Action)
	}
	types, _ := result.Metadata["types"].([]string)
	for _, typ := range types {
		if typ == "phone" {
			t.Error("不应检测到 phone（已禁用）")
		}
		if typ == "api_key" {
			t.Error("不应检测到 api_key（已禁用）")
		}
	}
}

// TestPIIRule_AllTypesCount 验证默认配置支持全部 10 种类型
func TestPIIRule_AllTypesCount(t *testing.T) {
	rule := NewPIIRule(DefaultPIIRuleConfig())
	if len(rule.patterns) != 10 {
		t.Errorf("patterns = %d, want 10", len(rule.patterns))
	}
}

// TestPIIRule_Sanitize_Extended 验证扩展类型的脱敏效果
func TestPIIRule_Sanitize_Extended(t *testing.T) {
	rule := NewPIIRule(DefaultPIIRuleConfig())

	// SSN 应被全遮蔽
	result, err := rule.Check("SSN: 123-45-6789", CheckInput)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if strings.Contains(result.Sanitized, "123-45-6789") {
		t.Errorf("SSN 未脱敏: %q", result.Sanitized)
	}
	if !strings.Contains(result.Sanitized, "***") {
		t.Errorf("SSN 应被全遮蔽（***），实际: %q", result.Sanitized)
	}

	// JWT 应被全遮蔽
	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	result2, err2 := rule.Check("JWT: "+jwt, CheckInput)
	if err2 != nil {
		t.Fatalf("Check error: %v", err2)
	}
	if strings.Contains(result2.Sanitized, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9") {
		t.Errorf("JWT 未脱敏: %q", result2.Sanitized)
	}
}
