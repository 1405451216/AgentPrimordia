package guardrail

import "testing"

// TestSensitiveToolRule_Blocked 高危工具调用（rm -rf）→ 拦截。
func TestSensitiveToolRule_Blocked(t *testing.T) {
	engine := NewEngine()
	engine.AddRule(&SensitiveToolRule{
		Blocked:   []string{"rm -rf", "DROP TABLE"},
		AuditOnly: []string{"git push", "kubectl delete"},
	})

	report, err := engine.CheckInput(`{"tool":"shell","command":"rm -rf /etc"}`)
	if err != nil {
		t.Fatalf("CheckInput: %v", err)
	}
	if report.Passed {
		t.Fatal("高危命令应被拦截")
	}
	if len(report.Results) == 0 || report.Results[0].Action != ActionReject {
		t.Errorf("results = %+v, want ActionReject", report.Results)
	}
}

// TestSensitiveToolRule_AuditOnly 敏感但允许的操作（git push）→ 审计标记放行。
func TestSensitiveToolRule_AuditOnly(t *testing.T) {
	engine := NewEngine()
	engine.AddRule(&SensitiveToolRule{
		Blocked:   []string{"rm -rf"},
		AuditOnly: []string{"git push", "kubectl delete"},
	})

	report, err := engine.CheckInput(`{"tool":"shell","command":"git push origin main"}`)
	if err != nil {
		t.Fatalf("CheckInput: %v", err)
	}
	if !report.Passed {
		t.Fatal("审计放行操作不应拦截")
	}
	found := false
	for _, r := range report.Results {
		if r.RuleName == "sensitive_tool" && r.Action == ActionFlag {
			found = true
		}
	}
	if !found {
		t.Errorf("results = %+v, want ActionFlag 审计标记", report.Results)
	}
}

// TestSensitiveToolRule_Allow 普通调用放行。
func TestSensitiveToolRule_Allow(t *testing.T) {
	engine := NewEngine()
	engine.AddRule(&SensitiveToolRule{Blocked: []string{"rm -rf"}, AuditOnly: []string{"git push"}})

	report, err := engine.CheckInput(`{"tool":"filesystem","action":"read","path":"/data/a.txt"}`)
	if err != nil {
		t.Fatalf("CheckInput: %v", err)
	}
	if !report.Passed {
		t.Errorf("普通读取不应拦截: %+v", report.Results)
	}
}
