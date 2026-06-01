package guardrail

import (
	"errors"
	"testing"
)

type mockRule struct {
	name   string
	result *Result
	err    error
}

func (m *mockRule) Name() string { return m.name }
func (m *mockRule) Check(_ string, _ CheckPoint) (*Result, error) {
	return m.result, m.err
}

func TestEngine_Check_AllPass(t *testing.T) {
	e := NewEngine()
	e.AddRule(&mockRule{name: "r1", result: &Result{RuleName: "r1", Action: ActionPass}})
	e.AddRule(&mockRule{name: "r2", result: &Result{RuleName: "r2", Action: ActionPass}})

	report, err := e.Check("hello", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Passed {
		t.Error("should pass when all rules pass")
	}
	if report.Action != ActionPass {
		t.Errorf("action = %q, want %q", report.Action, ActionPass)
	}
}

func TestEngine_Check_Reject(t *testing.T) {
	e := NewEngine()
	e.AddRule(&mockRule{name: "r1", result: &Result{RuleName: "r1", Action: ActionPass}})
	e.AddRule(&mockRule{name: "r2", result: &Result{RuleName: "r2", Action: ActionReject, Message: "blocked"}})

	report, err := e.Check("bad input", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Passed {
		t.Error("should not pass when a rule rejects")
	}
	if report.Action != ActionReject {
		t.Errorf("action = %q, want %q", report.Action, ActionReject)
	}
}

func TestEngine_Check_Sanitize(t *testing.T) {
	e := NewEngine()
	e.AddRule(&mockRule{
		name: "pii",
		result: &Result{
			RuleName:  "pii",
			Action:    ActionSanitize,
			Sanitized: "my phone is ***",
		},
	})

	report, err := e.Check("my phone is 13812345678", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Passed {
		t.Error("should not pass when sanitized")
	}
	if report.Action != ActionSanitize {
		t.Errorf("action = %q, want %q", report.Action, ActionSanitize)
	}
}

func TestEngine_Check_Flag(t *testing.T) {
	e := NewEngine()
	e.AddRule(&mockRule{name: "flag", result: &Result{RuleName: "flag", Action: ActionFlag}})

	report, err := e.Check("suspicious", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Passed {
		t.Error("flag should not fail the check")
	}
	if report.Action != ActionFlag {
		t.Errorf("action = %q, want %q", report.Action, ActionFlag)
	}
}

func TestEngine_Check_RuleError(t *testing.T) {
	e := NewEngine()
	e.AddRule(&mockRule{name: "err", err: errors.New("rule error")})

	_, err := e.Check("input", CheckInput)
	if err == nil {
		t.Error("should propagate rule error")
	}
}

func TestEngine_Check_NilResult(t *testing.T) {
	e := NewEngine()
	e.AddRule(&mockRule{name: "nil", result: nil})

	report, err := e.Check("input", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Passed {
		t.Error("nil result should be treated as pass")
	}
}

func TestEngine_Rules(t *testing.T) {
	e := NewEngine()
	e.AddRule(&mockRule{name: "r1"})
	e.AddRule(&mockRule{name: "r2"})

	names := e.Rules()
	if len(names) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(names))
	}
	if names[0] != "r1" || names[1] != "r2" {
		t.Errorf("names = %v, want [r1 r2]", names)
	}
}

func TestEngine_CheckInput_CheckOutput(t *testing.T) {
	e := NewEngine()
	e.AddRule(&mockRule{name: "r1", result: &Result{RuleName: "r1", Action: ActionPass}})

	r1, _ := e.CheckInput("hello")
	if !r1.Passed {
		t.Error("CheckInput should pass")
	}
	r2, _ := e.CheckOutput("world")
	if !r2.Passed {
		t.Error("CheckOutput should pass")
	}
}

func TestEngine_RejectStopsEarly(t *testing.T) {
	e := NewEngine()
	var secondCalled bool
	e.AddRule(&mockRule{name: "r1", result: &Result{RuleName: "r1", Action: ActionReject}})
	e.AddRule(&mockRule{name: "r2", result: &Result{RuleName: "r2", Action: ActionPass}})

	report, err := e.Check("input", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Passed {
		t.Error("should be rejected")
	}
	_ = secondCalled
	if len(report.Results) != 1 {
		t.Errorf("should stop after reject, got %d results", len(report.Results))
	}
}
