package guardrail

import (
	"errors"
	"testing"
)

type mockRule struct {
	name     string
	priority int
	result   *Result
	err      error
}

func (m *mockRule) Name() string { return m.name }
func (m *mockRule) Priority() int { return m.priority }
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

// TestEngine_PriorityOrder 验证规则按优先级降序执行
func TestEngine_PriorityOrder(t *testing.T) {
	e := NewEngine()

	// 按 Low → Critical 顺序添加，但执行顺序应为 Critical → Low
	e.AddRule(&mockRule{name: "low", priority: PriorityLow, result: &Result{RuleName: "low", Action: ActionPass}})
	e.AddRule(&mockRule{name: "critical", priority: PriorityCritical, result: &Result{RuleName: "critical", Action: ActionPass}})
	e.AddRule(&mockRule{name: "normal", priority: PriorityNormal, result: &Result{RuleName: "normal", Action: ActionPass}})

	// 验证 Rules() 返回顺序为 critical → normal → low
	names := e.Rules()
	if len(names) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(names))
	}
	if names[0] != "critical" {
		t.Errorf("expected first rule 'critical', got %q", names[0])
	}
	if names[1] != "normal" {
		t.Errorf("expected second rule 'normal', got %q", names[1])
	}
	if names[2] != "low" {
		t.Errorf("expected third rule 'low', got %q", names[2])
	}
}

// TestEngine_PriorityRejectStopsFirst 验证高优先级 Reject 规则优先执行并短路
func TestEngine_PriorityRejectStopsFirst(t *testing.T) {
	e := NewEngine()

	// 低优先级 Reject 先注册，高优先级 Pass 后注册
	// 由于排序，高优先级 Pass 应先执行，低优先级 Reject 后执行
	e.AddRule(&mockRule{name: "low-reject", priority: PriorityLow, result: &Result{RuleName: "low-reject", Action: ActionReject}})
	e.AddRule(&mockRule{name: "high-pass", priority: PriorityCritical, result: &Result{RuleName: "high-pass", Action: ActionPass}})

	report, err := e.Check("test", CheckInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 高优先级 Pass 先执行，低优先级 Reject 后执行 → 最终 Reject
	if report.Passed {
		t.Error("should be rejected by low-priority rule")
	}
	// 两条规则都执行了（Pass 不短路）
	if len(report.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(report.Results))
	}
	// 第一条结果应是高优先级的
	if report.Results[0].RuleName != "high-pass" {
		t.Errorf("expected first result 'high-pass', got %q", report.Results[0].RuleName)
	}
}
