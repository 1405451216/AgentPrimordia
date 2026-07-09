package controller

import "testing"

func TestDecideRollout_Promote(t *testing.T) {
	d := DecideRollout(10, EvalResult{RanOK: true, PassRate: 0.95, Threshold: 0.8})
	if d.Action != ActionPromote {
		t.Fatalf("action = %q, want promote", d.Action)
	}
}

func TestDecideRollout_Rollback(t *testing.T) {
	d := DecideRollout(10, EvalResult{RanOK: true, PassRate: 0.6, Threshold: 0.8})
	if d.Action != ActionRollback {
		t.Fatalf("action = %q, want rollback", d.Action)
	}
}

func TestDecideRollout_HoldWhenEvalFailed(t *testing.T) {
	d := DecideRollout(10, EvalResult{RanOK: false, PassRate: 0.0, Threshold: 0.8})
	if d.Action != ActionHold {
		t.Fatalf("action = %q, want hold", d.Action)
	}
}

func TestDecideRollout_Boundary(t *testing.T) {
	d := DecideRollout(10, EvalResult{RanOK: true, PassRate: 0.8, Threshold: 0.8})
	if d.Action != ActionPromote {
		t.Fatalf("边界=阈值应 promote，got %q", d.Action)
	}
}

func TestRollingUpdateWithEval_InvalidPercent(t *testing.T) {
	if _, err := RollingUpdateWithEval("agent-x", 0, EvalResult{RanOK: true, PassRate: 1, Threshold: 0.8}); err == nil {
		t.Fatal("canaryPercent=0 应返回错误")
	}
}

func TestRollingUpdateWithEval_FullFlow(t *testing.T) {
	d, err := RollingUpdateWithEval("agent-x", 10, EvalResult{RanOK: true, PassRate: 0.92, Threshold: 0.8})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if d.Action != ActionPromote {
		t.Fatalf("action = %q, want promote", d.Action)
	}
	d2, err := RollingUpdateWithEval("agent-x", 10, EvalResult{RanOK: true, PassRate: 0.5, Threshold: 0.8})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if d2.Action != ActionRollback {
		t.Fatalf("action = %q, want rollback", d2.Action)
	}
}
