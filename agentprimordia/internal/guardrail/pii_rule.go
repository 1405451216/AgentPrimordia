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
	priority int
	patterns []piiPattern
}

type piiPattern struct {
	name    string
	regex   *regexp.Regexp
	maskLen int
}

// PIIRuleConfig PII 规则配置
type PIIRuleConfig struct {
	Action         Action
	Severity       Severity
	Priority       int // 规则优先级，默认 PriorityHigh
	DetectPhone    bool
	DetectIDCard   bool
	DetectEmail    bool
	DetectBankCard bool
	DetectIPv4     bool
	// p2t2：扩展 PII 类型
	DetectPassport    bool
	DetectBankAccount bool
	DetectSSN         bool
	DetectAPIKey      bool
	DetectJWT         bool
}

// DefaultPIIRuleConfig 默认 PII 配置：检测所有类型，脱敏处理
func DefaultPIIRuleConfig() PIIRuleConfig {
	return PIIRuleConfig{
		Action:            ActionSanitize,
		Severity:          SeverityHigh,
		DetectPhone:       true,
		DetectIDCard:      true,
		DetectEmail:       true,
		DetectBankCard:    true,
		DetectIPv4:        true,
		DetectPassport:    true,
		DetectBankAccount: true,
		DetectSSN:         true,
		DetectAPIKey:      true,
		DetectJWT:         true,
	}
}

// NewPIIRule 创建 PII 检测规则
func NewPIIRule(config PIIRuleConfig) *PIIRule {
	priority := config.Priority
	if priority == 0 {
		priority = PriorityHigh
	}
	r := &PIIRule{
		action:   config.Action,
		severity: config.Severity,
		priority: priority,
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
	// p2t2：扩展 PII 模式
	if config.DetectPassport {
		r.patterns = append(r.patterns, piiPattern{
			name:    "passport",
			regex:   regexp.MustCompile(`(?:E\d{8})|(?:[A-Z]{2}\d{6,9})`),
			maskLen: 2,
		})
	}
	if config.DetectBankAccount {
		r.patterns = append(r.patterns, piiPattern{
			name:    "bank_account",
			regex:   regexp.MustCompile(`\b[1-9]\d{15,18}\b`),
			maskLen: 4,
		})
	}
	if config.DetectSSN {
		r.patterns = append(r.patterns, piiPattern{
			name:    "ssn",
			regex:   regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
			maskLen: 0, // 全遮蔽（高敏感）
		})
	}
	if config.DetectAPIKey {
		r.patterns = append(r.patterns, piiPattern{
			name:    "api_key",
			regex:   regexp.MustCompile(`(?:sk-[a-zA-Z0-9_]{20,})|(?:pk_[a-zA-Z0-9_]{20,})|(?:AKIA[A-Z0-9]{16})`),
			maskLen: 6,
		})
	}
	if config.DetectJWT {
		r.patterns = append(r.patterns, piiPattern{
			name:    "jwt",
			regex:   regexp.MustCompile(`eyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`),
			maskLen: 0, // 全遮蔽（高敏感）
		})
	}
	return r
}

// Name 返回规则名
func (r *PIIRule) Name() string { return "pii_detection" }

// Priority 返回规则优先级
func (r *PIIRule) Priority() int { return r.priority }

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
		RuleName: r.Name(),
		Action:   r.action,
		Severity: r.severity,
		Message:  "PII detected: " + strings.Join(findings, ", "),
		Metadata: map[string]any{"types": findings},
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
