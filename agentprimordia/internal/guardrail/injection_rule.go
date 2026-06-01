package guardrail

import (
	"regexp"
	"strings"
)

// normalizeForCheck 对输入进行归一化处理，检测变形攻击
func normalizeForCheck(s string) string {
	s = strings.ToLower(s)
	// leet-speak 替换
	replacements := []struct{ from, to string }{
		{"0", "o"}, {"1", "i"}, {"3", "e"}, {"5", "s"}, {"7", "t"}, {"@", "a"},
	}
	for _, r := range replacements {
		s = strings.ReplaceAll(s, r.from, r.to)
	}
	return s
}

// PromptInjectionRule Prompt 注入检测规则
// 检测常见的 Prompt 注入攻击模式
type PromptInjectionRule struct {
	action   Action
	severity Severity
	patterns []*regexp.Regexp
	keywords []string
}

// PromptInjectionConfig Prompt 注入检测配置
type PromptInjectionConfig struct {
	Action   Action
	Severity Severity
}

// NewPromptInjectionRule 创建 Prompt 注入检测规则
func NewPromptInjectionRule(config PromptInjectionConfig) *PromptInjectionRule {
	r := &PromptInjectionRule{
		action:   config.Action,
		severity: config.Severity,
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)ignore\s+(previous|above|all)\s+instructions`),
			regexp.MustCompile(`(?i)forget\s+(everything|all|previous)`),
			regexp.MustCompile(`(?i)you\s+are\s+now\s+a`),
			regexp.MustCompile(`(?i)pretend\s+(you\s+are|to\s+be)`),
			regexp.MustCompile(`(?i)disregard\s+(all|any|previous|the)\s+(rules|instructions|guidelines)`),
			regexp.MustCompile(`(?i)system\s*:\s*`),
			regexp.MustCompile(`(?i)<\|im_start\|>`),
			regexp.MustCompile(`(?i)\[INST\]`),
			regexp.MustCompile(`(?i)jailbreak`),
			regexp.MustCompile(`(?i)DAN\s+mode`),
		},
		keywords: []string{
			"system prompt",
			"忽略之前的指令",
			"忽略以上指令",
			"忽略所有指令",
			"越狱",
			"解锁模式",
		},
	}
	return r
}

// Name 返回规则名
func (r *PromptInjectionRule) Name() string { return "prompt_injection" }

// Check 检测输入中的 Prompt 注入
func (r *PromptInjectionRule) Check(input string, point CheckPoint) (*Result, error) {
	lower := strings.ToLower(input)
	normalized := normalizeForCheck(input)
	var detected []string

	for _, p := range r.patterns {
		if p.MatchString(normalized) {
			detected = append(detected, p.String())
		}
	}

	for _, kw := range r.keywords {
		kwLower := strings.ToLower(kw)
		if strings.Contains(lower, kwLower) || strings.Contains(normalized, kwLower) {
			detected = append(detected, kw)
		}
	}

	if len(detected) == 0 {
		return &Result{RuleName: r.Name(), Action: ActionPass}, nil
	}

	return &Result{
		RuleName: r.Name(),
		Action:   r.action,
		Severity: r.severity,
		Message:  "potential prompt injection detected",
		Metadata: map[string]any{"matches": detected},
	}, nil
}
