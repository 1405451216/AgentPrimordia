package guardrail

import "testing"

func TestPIIRule_Phone(t *testing.T) {
	rule := NewPIIRule(PIIRuleConfig{
		Action:      ActionSanitize,
		Severity:    SeverityHigh,
		DetectPhone: true,
	})
	result, err := rule.Check("my phone is 13812345678", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionSanitize {
		t.Errorf("action = %q, want %q", result.Action, ActionSanitize)
	}
	if result.Sanitized != "my phone is 1381*******" {
		t.Errorf("sanitized = %q, want masked phone", result.Sanitized)
	}
}

func TestPIIRule_IDCard(t *testing.T) {
	rule := NewPIIRule(PIIRuleConfig{
		Action:       ActionSanitize,
		Severity:     SeverityHigh,
		DetectIDCard: true,
	})
	result, err := rule.Check("id: 110101199001011234", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionSanitize {
		t.Errorf("action = %q, want %q", result.Action, ActionSanitize)
	}
}

func TestPIIRule_Email(t *testing.T) {
	rule := NewPIIRule(PIIRuleConfig{
		Action:      ActionSanitize,
		Severity:    SeverityMedium,
		DetectEmail: true,
	})
	result, err := rule.Check("contact: user@example.com", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionSanitize {
		t.Errorf("action = %q, want %q", result.Action, ActionSanitize)
	}
	if result.Sanitized == "contact: user@example.com" {
		t.Error("email should be masked")
	}
}

func TestPIIRule_BankCard(t *testing.T) {
	rule := NewPIIRule(PIIRuleConfig{
		Action:         ActionSanitize,
		Severity:       SeverityCritical,
		DetectBankCard: true,
	})
	result, err := rule.Check("card: 6222021234567890123", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionSanitize {
		t.Errorf("action = %q, want %q", result.Action, ActionSanitize)
	}
}

func TestPIIRule_IPv4(t *testing.T) {
	rule := NewPIIRule(PIIRuleConfig{
		Action:     ActionSanitize,
		Severity:   SeverityMedium,
		DetectIPv4: true,
	})
	result, err := rule.Check("server at 192.168.1.1", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionSanitize {
		t.Errorf("action = %q, want %q", result.Action, ActionSanitize)
	}
}

func TestPIIRule_NoPII(t *testing.T) {
	rule := NewPIIRule(DefaultPIIRuleConfig())
	result, err := rule.Check("hello world", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionPass {
		t.Errorf("action = %q, want %q", result.Action, ActionPass)
	}
}

func TestPIIRule_RejectAction(t *testing.T) {
	rule := NewPIIRule(PIIRuleConfig{
		Action:      ActionReject,
		Severity:    SeverityCritical,
		DetectPhone: true,
	})
	result, err := rule.Check("call 13812345678", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionReject {
		t.Errorf("action = %q, want %q", result.Action, ActionReject)
	}
}

func TestPIIRule_MultiplePII(t *testing.T) {
	rule := NewPIIRule(DefaultPIIRuleConfig())
	result, err := rule.Check("phone 13812345678 email test@example.com", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	types := result.Metadata["types"].([]string)
	if len(types) < 2 {
		t.Errorf("expected at least 2 PII types, got %d", len(types))
	}
}

func TestPIIRule_Name(t *testing.T) {
	rule := NewPIIRule(DefaultPIIRuleConfig())
	if rule.Name() != "pii_detection" {
		t.Errorf("name = %q, want %q", rule.Name(), "pii_detection")
	}
}
