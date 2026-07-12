package guardrail

import (
	"log/slog"
	"regexp"
	"strings"
)

// OutputSafetyRule 输出安全检查规则
// 检查 LLM 输出中是否包含不安全内容
type OutputSafetyRule struct {
	action   Action
	severity Severity
	priority int
	patterns []*regexp.Regexp
}

// OutputSafetyConfig 输出安全检查配置
type OutputSafetyConfig struct {
	Action              Action
	Severity            Severity
	Priority            int // 规则优先级，默认 PriorityHigh
	DetectCodeExecution bool
	DetectURLs          bool
	DetectFilePaths     bool
	CustomPatterns      []string
}

// NewOutputSafetyRule 创建输出安全检查规则
func NewOutputSafetyRule(config OutputSafetyConfig) *OutputSafetyRule {
	priority := config.Priority
	if priority == 0 {
		priority = PriorityHigh
	}
	r := &OutputSafetyRule{
		action:   config.Action,
		severity: config.Severity,
		priority: priority,
	}
	if config.DetectCodeExecution {
		r.patterns = append(r.patterns,
			regexp.MustCompile(`(?i)rm\s+-rf\s+/`),
			regexp.MustCompile(`(?i)del\s+/[sS]\s+/[qQ]`),
			regexp.MustCompile(`(?i)format\s+[A-Za-z]:`),
			regexp.MustCompile(`(?i)curl\s+.*\|\s*sh`),
			regexp.MustCompile(`(?i)wget\s+.*\|\s*bash`),
			regexp.MustCompile(`(?i)exec\s*\(`),
			regexp.MustCompile(`(?i)eval\s*\(`),
			regexp.MustCompile(`(?i)subprocess\.(call|run|Popen)`),
		)
	}
	if config.DetectURLs {
		r.patterns = append(r.patterns,
			regexp.MustCompile(`https?://[^\s<>"]+`),
		)
	}
	if config.DetectFilePaths {
		r.patterns = append(r.patterns,
			regexp.MustCompile(`(?:/etc/|/var/|/usr/local/)[^\s<>"]+`),
			regexp.MustCompile(`[A-Za-z]:\\(?:Users|Windows|Program Files)[\\/][^\s<>"]+`),
		)
	}
	for _, p := range config.CustomPatterns {
		if re, err := regexp.Compile(p); err == nil {
			r.patterns = append(r.patterns, re)
		} else {
			slog.Warn("正则编译失败，已跳过", "pattern", p, "error", err)
		}
	}
	return r
}

// Name 返回规则名
func (r *OutputSafetyRule) Name() string { return "output_safety" }

// Priority 返回规则优先级
func (r *OutputSafetyRule) Priority() int { return r.priority }

// Check 检查输出中的不安全内容
func (r *OutputSafetyRule) Check(output string, point CheckPoint) (*Result, error) {
	if point != CheckOutput {
		return &Result{RuleName: r.Name(), Action: ActionPass}, nil
	}

	var findings []string
	for _, p := range r.patterns {
		matches := p.FindAllString(output, -1)
		if len(matches) > 0 {
			findings = append(findings, matches...)
		}
	}

	if len(findings) == 0 {
		return &Result{RuleName: r.Name(), Action: ActionPass}, nil
	}

	return &Result{
		RuleName: r.Name(),
		Action:   r.action,
		Severity: r.severity,
		Message:  "unsafe content detected in output",
		Metadata: map[string]any{"findings": findings},
	}, nil
}

// SanitizeOutput 清理输出中的不安全内容
func SanitizeOutput(output string, patterns []*regexp.Regexp) string {
	result := output
	for _, p := range patterns {
		result = p.ReplaceAllStringFunc(result, func(match string) string {
			return strings.Repeat("*", len(match))
		})
	}
	return result
}
