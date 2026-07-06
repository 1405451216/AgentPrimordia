package audit

import "testing"

// TestAllAuditActions_NotEmpty 验证枚举不为空
func TestAllAuditActions_NotEmpty(t *testing.T) {
	actions := AllAuditActions()
	if len(actions) == 0 {
		t.Fatal("AllAuditActions 不应为空")
	}
}

// TestAllAuditActions_NoDuplicates 验证枚举无重复
func TestAllAuditActions_NoDuplicates(t *testing.T) {
	actions := AllAuditActions()
	seen := make(map[AuditAction]bool, len(actions))
	for _, a := range actions {
		if seen[a] {
			t.Errorf("重复的审计动作: %q", a)
		}
		seen[a] = true
	}
}

// TestIsValidAuditAction 验证标准动作识别
func TestIsValidAuditAction(t *testing.T) {
	tests := []struct {
		action AuditAction
		want   bool
	}{
		{ActionLLMCall, true},
		{ActionToolCall, true},
		{ActionGuardrailBlock, true},
		{ActionPIIDetected, true},
		{AuditAction("unknown.action"), false},
		{AuditAction(""), false},
	}
	for _, tt := range tests {
		if got := IsValidAuditAction(tt.action); got != tt.want {
			t.Errorf("IsValidAuditAction(%q) = %v, want %v", tt.action, got, tt.want)
		}
	}
}

// TestAuditActionStringValues 验证动作值符合命名规范（小写、点分隔）
func TestAuditActionStringValues(t *testing.T) {
	actions := AllAuditActions()
	for _, a := range actions {
		// 简单校验：必须包含一个点（如 "agent.start"）
		if !actionContains(string(a), ".") {
			t.Errorf("审计动作 %q 缺少命名空间（应为 'category.action' 格式）", a)
		}
		// 不允许包含大写
		for _, r := range string(a) {
			if r >= 'A' && r <= 'Z' {
				t.Errorf("审计动作 %q 不应包含大写字母", a)
				break
			}
		}
	}
}

// TestAuditResultStringValues 验证结果枚举的标准值
func TestAuditResultStringValues(t *testing.T) {
	expected := []AuditResult{
		ResultSuccess, ResultDenied, ResultError, ResultBlocked,
	}
	for _, r := range expected {
		if r == "" {
			t.Error("AuditResult 不应为空字符串")
		}
	}
}

func actionContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
