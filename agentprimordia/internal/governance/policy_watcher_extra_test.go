package governance

import (
	"testing"
)

func TestWatchablePolicy_Rollback(t *testing.T) {
	initial := &Policy{
		Spec: PolicySpec{
			CostLimits: CostLimits{PerRequest: 1.0},
		},
	}
	wp := NewWatchablePolicy(initial, nil)

	// 更新策略
	updated := &Policy{
		Spec: PolicySpec{
			CostLimits: CostLimits{PerRequest: 2.0},
		},
	}
	_ = wp.Swap(updated, "test")

	// 回滚
	err := wp.Rollback(initial, "test rollback")
	if err != nil {
		t.Errorf("Rollback error: %v", err)
	}

	current := wp.Current()
	if current.Spec.CostLimits.PerRequest != 1.0 {
		t.Errorf("after Rollback PerRequest = %f, want 1.0", current.Spec.CostLimits.PerRequest)
	}
}

func TestWatchablePolicy_Rollback_NilPrevious(t *testing.T) {
	wp := NewWatchablePolicy(&Policy{}, nil)
	err := wp.Rollback(nil, "test")
	if err == nil {
		t.Error("Rollback with nil previous should fail")
	}
}

func TestWatchablePolicy_GetHistory(t *testing.T) {
	wp := NewWatchablePolicy(&Policy{Spec: PolicySpec{CostLimits: CostLimits{PerRequest: 1.0}}}, nil)
	_ = wp.Swap(&Policy{Spec: PolicySpec{CostLimits: CostLimits{PerRequest: 2.0}}}, "test")

	history := wp.GetHistory()
	if history == nil {
		t.Fatal("GetHistory should not return nil")
	}
	if len(history.Versions) == 0 {
		t.Error("History should contain at least one version")
	}
}

func TestPolicyWatcher_GetHistory(t *testing.T) {
	initial := &Policy{Spec: PolicySpec{CostLimits: CostLimits{PerRequest: 1.0}}}
	pw := NewPolicyWatcher("/nonexistent/path.yaml", initial, nil)

	history := pw.GetHistory()
	if history == nil {
		t.Error("PolicyWatcher.GetHistory should not return nil")
	}
}

func TestPolicyWatcher_GetPolicy(t *testing.T) {
	initial := &Policy{Spec: PolicySpec{CostLimits: CostLimits{PerRequest: 1.0}}}
	pw := NewPolicyWatcher("/nonexistent/path.yaml", initial, nil)

	policy := pw.GetPolicy()
	if policy == nil {
		t.Error("GetPolicy should not return nil")
	}
}

func TestValidatePolicy_NegativePerRequest(t *testing.T) {
	p := &Policy{Spec: PolicySpec{CostLimits: CostLimits{PerRequest: -1.0}}}
	err := ValidatePolicy(p)
	if err == nil {
		t.Error("ValidatePolicy should reject negative PerRequest")
	}
}

func TestValidatePolicy_NegativePerDay(t *testing.T) {
	p := &Policy{Spec: PolicySpec{CostLimits: CostLimits{PerDay: -1.0}}}
	err := ValidatePolicy(p)
	if err == nil {
		t.Error("ValidatePolicy should reject negative PerDay")
	}
}

func TestValidatePolicy_NegativeMaxLength(t *testing.T) {
	p := &Policy{Spec: PolicySpec{OutputGuardrail: OutputGuardrail{MaxLength: -1}}}
	err := ValidatePolicy(p)
	if err == nil {
		t.Error("ValidatePolicy should reject negative MaxLength")
	}
}

func TestValidatePolicy_NegativeMaxTurns(t *testing.T) {
	p := &Policy{Spec: PolicySpec{BehaviorConstraints: BehaviorConstraints{MaxTurns: -1}}}
	err := ValidatePolicy(p)
	if err == nil {
		t.Error("ValidatePolicy should reject negative MaxTurns")
	}
}

func TestValidatePolicy_NegativeToolCalls(t *testing.T) {
	p := &Policy{Spec: PolicySpec{ToolRestrictions: []ToolRestriction{
		{Tool: "shell", MaxCallsPerRun: -1},
	}}}
	err := ValidatePolicy(p)
	if err == nil {
		t.Error("ValidatePolicy should reject negative MaxCallsPerRun")
	}
}

func TestValidatePolicy_NilPolicy(t *testing.T) {
	err := ValidatePolicy(nil)
	if err == nil {
		t.Error("ValidatePolicy(nil) should fail")
	}
}
