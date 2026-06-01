package guardrail

import "testing"

func TestSanitizer_Mask(t *testing.T) {
	s := NewSanitizer(SanitizerConfig{Strategy: StrategyMask, MaskChar: '*'})
	result := s.Sanitize("phone 13812345678 here", []Position{
		{Start: 6, End: 17, Label: "phone"},
	})
	if result != "phone *********** here" {
		t.Errorf("mask result = %q", result)
	}
}

func TestSanitizer_Redact(t *testing.T) {
	s := NewSanitizer(SanitizerConfig{Strategy: StrategyRedact, ReplText: "[REMOVED]"})
	result := s.Sanitize("email test@example.com end", []Position{
		{Start: 6, End: 22, Label: "email"},
	})
	if result != "email [REMOVED] end" {
		t.Errorf("redact result = %q", result)
	}
}

func TestSanitizer_Replace(t *testing.T) {
	s := NewSanitizer(SanitizerConfig{Strategy: StrategyReplace})
	result := s.Sanitize("card 6222021234567890123 end", []Position{
		{Start: 5, End: 24, Label: "bank_card"},
	})
	if result != "card [BANK_CARD_REDACTED] end" {
		t.Errorf("replace result = %q", result)
	}
}

func TestSanitizer_Hash(t *testing.T) {
	s := NewSanitizer(SanitizerConfig{Strategy: StrategyHash})
	result := s.Sanitize("data 12345 end", []Position{
		{Start: 5, End: 10, Label: "id"},
	})
	if result != "data [#5] end" {
		t.Errorf("hash result = %q", result)
	}
}

func TestSanitizer_NoPositions(t *testing.T) {
	s := NewSanitizer(SanitizerConfig{Strategy: StrategyMask})
	result := s.Sanitize("hello world", nil)
	if result != "hello world" {
		t.Errorf("should return original text when no positions")
	}
}

func TestSanitizer_InvalidPositions(t *testing.T) {
	s := NewSanitizer(SanitizerConfig{Strategy: StrategyMask})
	result := s.Sanitize("hello", []Position{
		{Start: -1, End: 3},
		{Start: 2, End: 1},
		{Start: 0, End: 100},
	})
	if result != "hello" {
		t.Errorf("should handle invalid positions gracefully, got %q", result)
	}
}

func TestSanitizeReport(t *testing.T) {
	report := &Report{
		Passed: false,
		Results: []Result{
			{RuleName: "pii", Action: ActionSanitize, Sanitized: "phone *** here"},
		},
	}
	result := SanitizeReport("phone 138 here", report, StrategyMask)
	if result != "phone *** here" {
		t.Errorf("sanitize report = %q", result)
	}
}

func TestSanitizeReport_Passed(t *testing.T) {
	result := SanitizeReport("hello", &Report{Passed: true}, StrategyMask)
	if result != "hello" {
		t.Errorf("should return original when passed")
	}
}

func TestSanitizeReport_Nil(t *testing.T) {
	result := SanitizeReport("hello", nil, StrategyMask)
	if result != "hello" {
		t.Errorf("should return original when nil report")
	}
}
