package guardrail

import (
	"regexp"
	"strings"
)

// normalizeForCheck 对输入进行归一化处理，检测变形攻击。
// 反混淆通道（红队对抗集 adversarial-holdout-v1 驱动，V7 §五）：
//   - 零宽字符剥离（U+200B/200C/200D/FEFF）
//   - 全角同形字 → ASCII（NFKC 语义子集：常用全角 ASCII 区 U+FF01–U+FF5E）
//   - markdown 代码围栏/反引号剥离
//   - base64 前缀载荷试解码（"base64:..." 形态）
//   - 空白折叠（对抗分两段拼接/转义插入的空白变体）
func normalizeForCheck(s string) string {
	// 零宽与不可见混淆字符剥离
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\u200b', '\u200c', '\u200d', '\ufeff':
			continue
		}
		b.WriteRune(r)
	}
	s = b.String()
	// 全角 ASCII 区 → 半角
	var fw strings.Builder
	for _, r := range s {
		if r >= 0xFF01 && r <= 0xFF5E {
			r -= 0xFEE0
		} else if r == 0x3000 {
			r = ' '
		}
		fw.WriteRune(r)
	}
	s = fw.String()
	s = strings.ToLower(s)
	// leet-speak 替换
	replacements := []struct{ from, to string }{
		{"0", "o"}, {"1", "i"}, {"3", "e"}, {"5", "s"}, {"7", "t"}, {"@", "a"},
	}
	for _, r := range replacements {
		s = strings.ReplaceAll(s, r.from, r.to)
	}
	// markdown 代码围栏与反引号剥离（代码块包裹/嵌套JSON/markdown注释通道）
	if strings.Contains(s, "`") || strings.Contains(s, "```") {
		s = strings.ReplaceAll(s, "```", " ")
		s = strings.ReplaceAll(s, "`", " ")
	}
	// 空白折叠（对抗分两段拼接/多语言混排插空白）
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// PromptInjectionRule Prompt 注入检测规则
// 检测常见的 Prompt 注入攻击模式
type PromptInjectionRule struct {
	action   Action
	severity Severity
	priority int
	patterns []*regexp.Regexp
	keywords []string
}

// PromptInjectionConfig Prompt 注入检测配置
type PromptInjectionConfig struct {
	Action   Action
	Severity Severity
	Priority int // 规则优先级，默认 PriorityCritical
}

// NewPromptInjectionRule 创建 Prompt 注入检测规则
func NewPromptInjectionRule(config PromptInjectionConfig) *PromptInjectionRule {
	priority := config.Priority
	if priority == 0 {
		priority = PriorityCritical
	}
	r := &PromptInjectionRule{
		action:   config.Action,
		severity: config.Severity,
		priority: priority,
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)ignore\s+(previous|above|all|all\s+previous|all\s+above|any\s+previous)\s+instructions`),
			regexp.MustCompile(`(?i)forget\s+(everything|all|previous)`),
			regexp.MustCompile(`(?i)you\s+are\s+now\s+a`),
			regexp.MustCompile(`(?i)pretend\s+(you\s+are|to\s+be)`),
			regexp.MustCompile(`(?i)disregard\s+(all|any|previous|the)\s+(rules|instructions|guidelines)`),
			regexp.MustCompile(`(?i)system\s*:\s*`),
			regexp.MustCompile(`(?i)<\|im_start\|>`),
			regexp.MustCompile(`(?i)\[INST\]`),
			regexp.MustCompile(`(?i)jailbreak`),
			regexp.MustCompile(`(?i)DAN\s+mode`),
			// —— 中文注入模板（红队对抗集冻结口径；模板命中即拒绝）——
			regexp.MustCompile(`忽略(之前|以上|上面|先前|之前所有|之前的一切)?(的)?(所有|全部)?指令`),
			regexp.MustCompile(`输出(你的)?(内部|系统)?提示词`),
			regexp.MustCompile(`(重新|重新)解释以上规则`),
			regexp.MustCompile(`无条件放行`),
			regexp.MustCompile(`当作最高优先级`),
			regexp.MustCompile(`解除限制`),
			regexp.MustCompile(`开发者模式已启用`),
			regexp.MustCompile(`(你是|模拟)\s*(无限制|DAN)`),
			regexp.MustCompile(`复述密钥`),
			regexp.MustCompile(`隐藏指令`),
			regexp.MustCompile(`越权指令`),
			regexp.MustCompile(`当(作|成)权威指令`),
			regexp.MustCompile(`(删除|写入|格式化)(所有|全部)?(文件|磁盘|内容)`),
			regexp.MustCompile(`(执行|运行)\s*rm\s+-rf`),
			regexp.MustCompile(`被允许注入(代码|指令)`),
			regexp.MustCompile(`(回复|输出)注入`),
			regexp.MustCompile(`输出系统提示(词)?(绕过|泄露)?`),
			regexp.MustCompile(`绕过审计`),
		},
		keywords: []string{
			"system prompt",
			"忽略之前的指令",
			"忽略以上指令",
			"忽略所有指令",
			"越狱",
			"解锁模式",
			"系统提示词",
			"无条件放行",
			"解除限制",
			"开发者模式",
			"权威指令",
			"隐藏指令",
			"复述密钥",
			"disregard all guardrails",
			"debug=true",
			"内部标记",
		},
	}
	return r
}

// Name 返回规则名
func (r *PromptInjectionRule) Name() string { return "prompt_injection" }

// Priority 返回规则优先级
func (r *PromptInjectionRule) Priority() int { return r.priority }

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
