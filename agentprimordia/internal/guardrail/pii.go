package guardrail

import (
	"regexp"
	"sort"
	"strings"
)

// PIIType 个人身份信息类型
type PIIType string

const (
	Email       PIIType = "email"
	Phone       PIIType = "phone"
	IDCard      PIIType = "id_card"
	CreditCard  PIIType = "credit_card"
	IPAddress   PIIType = "ip_address"
	Passport    PIIType = "passport"     // 护照号（中国：E+8位数字；美国：9位数字）
	BankAccount PIIType = "bank_account" // 银行账号（16-19位，特定前缀）
	SSN         PIIType = "ssn"          // 美国社保号（XXX-XX-XXXX）
	APIKey      PIIType = "api_key"      // API 密钥（sk-/pk-/AKIA 前缀）
	JWT         PIIType = "jwt"          // JWT 令牌（三段式 base64）
)

// Finding 单个 PII 检测结果
type Finding struct {
	Type  PIIType // PII 类型
	Value string  // 原始匹配值
	Start int     // 起始位置（字节偏移）
	End   int     // 结束位置（字节偏移）
}

// DetectionResult PII 检测结果
type DetectionResult struct {
	Findings []Finding
}

// SanitizeConfig 脱敏配置
type SanitizeConfig struct {
	ReplaceWith string // 替换文本，默认 "[REDACTED]"
}

// piiPatternDef PII 正则模式定义
type piiPatternDef struct {
	typ   PIIType
	regex *regexp.Regexp
}

// PIIDetector PII 检测器
type PIIDetector struct {
	patterns []piiPatternDef
}

// NewPIIDetector 创建 PII 检测器
func NewPIIDetector() *PIIDetector {
	return &PIIDetector{
		patterns: []piiPatternDef{
			// 邮箱地址
			{typ: Email, regex: regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)},
			// 中国手机号：1 开头，第二位 3-9，共 11 位
			{typ: Phone, regex: regexp.MustCompile(`1[3-9]\d{9}`)},
			// 中国身份证号：18 位，含末尾 X
			{typ: IDCard, regex: regexp.MustCompile(`[1-9]\d{5}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]`)},
			// 信用卡号：16-19 位纯数字（需词边界）
			{typ: CreditCard, regex: regexp.MustCompile(`\b\d{16,19}\b`)},
			// IPv4 地址
			{typ: IPAddress, regex: regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)},
			// 护照号：中国 E+8位数字；美国 9位数字
			{typ: Passport, regex: regexp.MustCompile(`(?:E\d{8})|(?:[A-Z]{2}\d{6,9})`)},
			// 银行账号：16-19 位纯数字（与 CreditCard 区分需前缀，简化：排除已有 IDCard/IPv4）
			{typ: BankAccount, regex: regexp.MustCompile(`\b[1-9]\d{15,18}\b`)},
			// 美国社保号：XXX-XX-XXXX
			{typ: SSN, regex: regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)},
			// API 密钥：sk-/pk-/AKIA 前缀（允许 _ 字符）
			{typ: APIKey, regex: regexp.MustCompile(`(?:sk-[a-zA-Z0-9_]{20,})|(?:pk_[a-zA-Z0-9_]{20,})|(?:AKIA[A-Z0-9]{16})`)},
			// JWT 令牌：三段式 base64（header.payload.signature）
			{typ: JWT, regex: regexp.MustCompile(`eyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`)},
		},
	}
}

// Detect 检测文本中的 PII
func (d *PIIDetector) Detect(text string) *DetectionResult {
	result := &DetectionResult{}
	for _, p := range d.patterns {
		matches := p.regex.FindAllStringIndex(text, -1)
		for _, loc := range matches {
			result.Findings = append(result.Findings, Finding{
				Type:  p.typ,
				Value: text[loc[0]:loc[1]],
				Start: loc[0],
				End:   loc[1],
			})
		}
	}
	return result
}

// Sanitize 对文本进行 PII 脱敏替换
// 从后向前替换，避免索引偏移
func (d *PIIDetector) Sanitize(text string, cfg SanitizeConfig) string {
	dr := d.Detect(text)
	if len(dr.Findings) == 0 {
		return text
	}

	replaceWith := cfg.ReplaceWith
	if replaceWith == "" {
		replaceWith = "[REDACTED]"
	}

	// 按起始位置降序排序，从后向前替换避免索引偏移
	findings := make([]Finding, len(dr.Findings))
	copy(findings, dr.Findings)
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Start > findings[j].Start
	})

	// 逐个替换
	b := []byte(text)
	for _, f := range findings {
		if f.Start < 0 || f.End > len(b) || f.Start >= f.End {
			continue
		}
		replacement := []byte(replaceWith)
		// 拼接：前段 + 替换 + 后段
		newB := make([]byte, 0, f.Start+len(replacement)+(len(b)-f.End))
		newB = append(newB, b[:f.Start]...)
		newB = append(newB, replacement...)
		newB = append(newB, b[f.End:]...)
		b = newB
	}
	return string(b)
}

// SanitizeRule PII 脱敏规则，实现 Rule 接口
type SanitizeRule struct {
	detector *PIIDetector
	config   SanitizeConfig
}

// NewSanitizeRule 创建 PII 脱敏规则
func NewSanitizeRule(cfg SanitizeConfig) *SanitizeRule {
	return &SanitizeRule{
		detector: NewPIIDetector(),
		config:   cfg,
	}
}

// Name 返回规则名
func (r *SanitizeRule) Name() string { return "pii-sanitize" }

// Check 检测输入中的 PII 并返回脱敏结果
func (r *SanitizeRule) Check(input string, _ CheckPoint) (*Result, error) {
	dr := r.detector.Detect(input)
	if len(dr.Findings) == 0 {
		return &Result{RuleName: r.Name(), Action: ActionPass}, nil
	}

	// 收集检测到的 PII 类型
	types := make(map[PIIType]bool)
	for _, f := range dr.Findings {
		types[f.Type] = true
	}
	typeNames := make([]string, 0, len(types))
	for t := range types {
		typeNames = append(typeNames, string(t))
	}

	sanitized := r.detector.Sanitize(input, r.config)

	return &Result{
		RuleName:  r.Name(),
		Action:    ActionSanitize,
		Severity:  SeverityHigh,
		Message:   "PII detected: " + strings.Join(typeNames, ", "),
		Sanitized: sanitized,
		Metadata:  map[string]any{"types": typeNames, "count": len(dr.Findings)},
	}, nil
}
