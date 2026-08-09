// sensitive_tool_rule.go — v4.6-3 guardrail 强化：敏感工具调用审计
//
// SensitiveToolRule 对工具调用输入做敏感操作检测（如 shell 高危命令、
// 文件删除/权限变更），命中时：
//   - Blocked 列表 → ActionReject（拦截，工具不执行）
//   - AuditOnly 列表 → ActionFlag（放行但标记审计，调用方落审计事件）
//
// 与引擎规则体系无缝集成（CheckInput 时工具参数以文本形式传入）。
package guardrail

import (
	"fmt"
	"strings"
)

// SensitiveToolRule 敏感工具调用审计规则。
type SensitiveToolRule struct {
	// Blocked 命中即拦截的工具调用子串（如 "rm -rf"、"DROP TABLE"）
	Blocked []string
	// AuditOnly 命中即审计放行的子串（如 "git push"、"kubectl delete"）
	AuditOnly []string
}

// Name 规则名。
func (r *SensitiveToolRule) Name() string { return "sensitive_tool" }

// Priority 高优先级（先于常规规则执行）。
func (r *SensitiveToolRule) Priority() int { return PriorityHigh }

// Check 检查工具调用文本（工具名 + 参数序列化），命中返回对应动作。
func (r *SensitiveToolRule) Check(input string, point CheckPoint) (*Result, error) {
	for _, s := range r.Blocked {
		if strings.Contains(input, s) {
			return &Result{
				RuleName: r.Name(),
				Action:   ActionReject,
				Severity: SeverityCritical,
				Message:  fmt.Sprintf("敏感工具调用被拦截: %q", s),
				Metadata: map[string]any{"blocked": s},
			}, nil
		}
	}
	for _, s := range r.AuditOnly {
		if strings.Contains(input, s) {
			return &Result{
				RuleName: r.Name(),
				Action:   ActionFlag,
				Severity: SeverityHigh,
				Message:  fmt.Sprintf("敏感工具调用（审计）: %q", s),
				Metadata: map[string]any{"audited": s},
			}, nil
		}
	}
	return &Result{RuleName: r.Name(), Action: ActionPass, Severity: SeverityLow}, nil
}
