package guardrail

import "testing"

func TestTopicConstraint_Denylist(t *testing.T) {
	rule := NewTopicConstraintRule(TopicConstraintConfig{
		Action:   ActionReject,
		Severity: SeverityHigh,
		Mode:     TopicModeDenylist,
		Topics:   []string{"赌博", "毒品"},
	})
	result, err := rule.Check("如何参与赌博", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionReject {
		t.Errorf("action = %q, want %q", result.Action, ActionReject)
	}
}

func TestTopicConstraint_Denylist_Pass(t *testing.T) {
	rule := NewTopicConstraintRule(TopicConstraintConfig{
		Action:   ActionReject,
		Severity: SeverityHigh,
		Mode:     TopicModeDenylist,
		Topics:   []string{"赌博", "毒品"},
	})
	result, err := rule.Check("今天天气怎么样", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionPass {
		t.Errorf("action = %q, want %q", result.Action, ActionPass)
	}
}

func TestTopicConstraint_Allowlist(t *testing.T) {
	rule := NewTopicConstraintRule(TopicConstraintConfig{
		Action:   ActionReject,
		Severity: SeverityMedium,
		Mode:     TopicModeAllowlist,
		Topics:   []string{"天气", "新闻", "科技"},
	})
	result, err := rule.Check("今天的科技新闻", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionPass {
		t.Errorf("action = %q, want %q", result.Action, ActionPass)
	}
}

func TestTopicConstraint_Allowlist_Reject(t *testing.T) {
	rule := NewTopicConstraintRule(TopicConstraintConfig{
		Action:   ActionReject,
		Severity: SeverityMedium,
		Mode:     TopicModeAllowlist,
		Topics:   []string{"天气", "新闻", "科技"},
	})
	result, err := rule.Check("如何做饭", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionReject {
		t.Errorf("action = %q, want %q", result.Action, ActionReject)
	}
}

func TestTopicConstraint_FlagAction(t *testing.T) {
	rule := NewTopicConstraintRule(TopicConstraintConfig{
		Action:   ActionFlag,
		Severity: SeverityLow,
		Mode:     TopicModeDenylist,
		Topics:   []string{"政治"},
	})
	result, err := rule.Check("讨论政治话题", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != ActionFlag {
		t.Errorf("action = %q, want %q", result.Action, ActionFlag)
	}
}

func TestTopicConstraint_Name(t *testing.T) {
	rule := NewTopicConstraintRule(TopicConstraintConfig{
		Mode:   TopicModeDenylist,
		Topics: []string{},
	})
	if rule.Name() != "topic_constraint" {
		t.Errorf("name = %q, want %q", rule.Name(), "topic_constraint")
	}
}
