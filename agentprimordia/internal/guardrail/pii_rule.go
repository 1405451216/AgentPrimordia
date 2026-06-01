package guardrail

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// PIIRule PII（个人身份信息）检测规则
// 支持检测：手机号、身份证号、邮箱、银行卡号、IPv4 地址
type PIIRule struct {
	action   Action
	severity Severity
	patterns []piiPattern
}

type piiPattern struct {
	name    string
	regex   *regexp.Regexp
	maskLen int
}

// PIIRuleConfig PII 规则配置
type PIIRuleConfig struct {
	Action           Action
	Severity         Severity
	DetectPhone      bool
	DetectIDCard     bool
	DetectEmail      bool
	DetectBankCard   bool
	DetectIPv4       bool
}

// DefaultPIIRuleConfig 默认 PII 配置：检测所有类型，脱敏处理
func DefaultPIIRuleConfig() PIIRuleConfig {
	return PIIRuleConfig{
		Action:       ActionSanitize,
		Severity:     SeverityHigh,
		DetectPhone:  true,
		DetectIDCard: true,
		DetectEmail:  true,
		DetectBankCard: true,
		DetectIPv4:   true,
	}
}

// NewPIIRule 创建 PII 检测规则
func NewPIIRule(config PIIRuleConfig) *PIIRule {
	r := &PIIRule{
		action:   config.Action,
		severity: config.Severity,
	}
	if config.DetectPhone {
		r.patterns = append(r.patterns, piiPattern{
			name:    "phone",
			regex:   regexp.MustCompile(`1[3-9]\d{9}`),
			maskLen: 4,
		})
	}
	if config.DetectIDCard {
		r.patterns = append(r.patterns, piiPattern{
			name:    "id_card",
			regex:   regexp.MustCompile(`[1-9]\d{5}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]`),
			maskLen: 6,
		})
	}
	if config.DetectEmail {
		r.patterns = append(r.patterns, piiPattern{
			name:    "email",
			regex:   regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
			maskLen: 0,
		})
	}
	if config.DetectBankCard {
		r.patterns = append(r.patterns, piiPattern{
			name:    "bank_card",
			regex:   regexp.MustCompile(`\b\d{16,19}\b`),
			maskLen: 8,
		})
	}
	if config.DetectIPv4 {
		r.patterns = append(r.patterns, piiPattern{
			name:    "ipv4",
			regex:   regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
			maskLen: 0,
		})
	}
	return r
}

// Name 返回规则名
func (r *PIIRule) Name() string { return "pii_detection" }

// Check 检测输入中的 PII
func (r *PIIRule) Check(input string, _ CheckPoint) (*Result, error) {
	var findings []string
	sanitized := input

	for _, p := range r.patterns {
		matches := p.regex.FindAllString(input, -1)
		if len(matches) > 0 {
			findings = append(findings, p.name)
			if r.action == ActionSanitize {
				sanitized = p.regex.ReplaceAllStringFunc(sanitized, func(match string) string {
					return maskString(match, p.maskLen)
				})
			}
		}
	}

	if len(findings) == 0 {
		return &Result{RuleName: r.Name(), Action: ActionPass}, nil
	}

	result := &Result{
		RuleName:  r.Name(),
		Action:    r.action,
		Severity:  r.severity,
		Message:   "PII detected: " + strings.Join(findings, ", "),
		Metadata:  map[string]any{"types": findings},
	}
	if r.action == ActionSanitize {
		result.Sanitized = sanitized
	}
	return result, nil
}

func maskString(s string, keepLen int) string {
	runeCount := utf8.RuneCountInString(s)
	if keepLen <= 0 {
		return strings.Repeat("*", runeCount)
	}
	if keepLen >= runeCount {
		return s
	}
	runes := []rune(s)
	return string(runes[:keepLen]) + strings.Repeat("*", runeCount-keepLen)
}
