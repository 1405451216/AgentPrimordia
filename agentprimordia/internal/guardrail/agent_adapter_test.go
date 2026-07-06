package guardrail

import (
	"strings"
	"testing"
)

// TestNewAgentOutputGuardAdapter_Sanitize 验证适配器在 Sanitize 动作下的行为
func TestNewAgentOutputGuardAdapter_Sanitize(t *testing.T) {
	engine := NewEngine()
	engine.AddRule(NewPIIRule(DefaultPIIRuleConfig()))

	adapter := NewAgentOutputGuardAdapter(engine)

	// LLM 输出包含 PII
	sanitized, blocked, err := adapter("用户邮箱 zhangsan@example.com")
	if err != nil {
		t.Fatalf("adapter error: %v", err)
	}
	if blocked {
		t.Error("should not block on sanitize")
	}
	if sanitized == "" {
		t.Error("sanitized content should not be empty")
	}
	if strings.Contains(sanitized, "zhangsan@example.com") {
		t.Error("email should be masked")
	}
}

// TestNewAgentOutputGuardAdapter_Pass 验证适配器在 Pass 动作下的行为
func TestNewAgentOutputGuardAdapter_Pass(t *testing.T) {
	engine := NewEngine()
	engine.AddRule(NewPIIRule(DefaultPIIRuleConfig()))

	adapter := NewAgentOutputGuardAdapter(engine)

	sanitized, blocked, err := adapter("正常的回复内容")
	if err != nil {
		t.Fatalf("adapter error: %v", err)
	}
	if blocked {
		t.Error("should not block on pass")
	}
	if sanitized != "" {
		t.Errorf("sanitized = %q, want empty", sanitized)
	}
}

// TestNewAgentOutputGuardAdapter_Reject 验证适配器在 Reject 动作下的行为
func TestNewAgentOutputGuardAdapter_Reject(t *testing.T) {
	engine := NewEngine()
	engine.AddRule(NewPIIRule(PIIRuleConfig{
		Action:      ActionReject,
		Severity:    SeverityCritical,
		DetectPhone: true,
	}))

	adapter := NewAgentOutputGuardAdapter(engine)

	sanitized, blocked, err := adapter("回复用户电话 13812345678")
	if err != nil {
		t.Fatalf("adapter error: %v", err)
	}
	if !blocked {
		t.Error("should block on reject")
	}
	_ = sanitized
}

// TestNewAgentOutputGuardAdapter_MultipleRules 验证多规则下的处理顺序
func TestNewAgentOutputGuardAdapter_MultipleRules(t *testing.T) {
	engine := NewEngine()
	engine.AddRule(NewPIIRule(DefaultPIIRuleConfig()))

	adapter := NewAgentOutputGuardAdapter(engine)

	// 同时包含邮箱和手机号
	sanitized, blocked, err := adapter("邮箱 zhangsan@example.com 电话 13812345678")
	if err != nil {
		t.Fatalf("adapter error: %v", err)
	}
	if blocked {
		t.Error("should not block")
	}
	if strings.Contains(sanitized, "zhangsan@example.com") || strings.Contains(sanitized, "13812345678") {
		t.Errorf("PII should be sanitized, got: %q", sanitized)
	}
}
