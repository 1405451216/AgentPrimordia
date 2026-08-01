// security.go — 安全审计强化（生产集成深度）
//
// 1. maskSecret — 审计日志中的 Secret 脱敏（API Key / Token / Password / Connection String）
// 2. detectPromptInjection — tool参数中的 prompt injection 攻击检测（中英文双语）
// 3. ValidateInput — 输入长度和字符集校验，防止日志注入和缓冲区溢出
package governance

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ===== Secret 脱敏 =====

// sensitivePatterns 匹配常见的敏感信息模式。
var sensitivePatterns = []*regexp.Regexp{
	// API Key: sk-xxx, AKIA-xxx, api_key=xxx
	regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9]{20,})`),
	regexp.MustCompile(`(?i)(AKIA[A-Z0-9]{16})`),
	// Bearer token
	regexp.MustCompile(`(?i)(Bearer\s+[a-zA-Z0-9\-._~+/]+=*)`),
	// password=xxx, pass=xxx, pwd=xxx
	regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[=:]\s*\S+`),
	// token=xxx
	regexp.MustCompile(`(?i)(token|secret|api[_-]?key)\s*[=:]\s*\S+`),
	// Connection string with credentials
	regexp.MustCompile(`(?i)(mongodb|redis|postgres|mysql)://\S+:\S+@`),
	// Private key markers
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
}

// maskPattern 用于在正则替换中脱敏。
func maskPattern(matched string) string {
	if len(matched) <= 8 {
		return strings.Repeat("*", len(matched))
	}
	return matched[:4] + strings.Repeat("*", len(matched)-8) + matched[len(matched)-4:]
}

// maskSecret 对输入字符串进行 Secret 脱敏。
// 保留首尾 4 字符，中间替换为 *，防止敏感信息泄露到审计日志。
func maskSecret(input string) string {
	result := input
	for _, p := range sensitivePatterns {
		result = p.ReplaceAllStringFunc(result, maskPattern)
	}
	return result
}

// ===== Prompt Injection 检测 =====

// injectionPatterns 匹配常见的 prompt injection 攻击模式（中英文双语）。
var injectionPatterns = []*regexp.Regexp{
	// ===== 英文模式 =====
	// "ignore previous instructions"
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?(previous|prior|above)\s+instructions?`),
	// "you are now..." / "act as..."
	regexp.MustCompile(`(?i)(you\s+are\s+now|act\s+as|pretend\s+to\s+be|forget\s+everything)`),
	// "system:" / "assistant:" 前缀注入
	regexp.MustCompile(`(?i)^(system|assistant|admin)\s*:`),
	// 试图逃逸沙箱
	regexp.MustCompile(`(?i)(escape|break\s+out|jailbreak).*(sandbox|container|restrict)`),
	// 试图读取系统提示
	regexp.MustCompile(`(?i)(reveal|show|print|output).*(system\s+prompt|instructions?|rules?)`),
	// 试图覆盖角色
	regexp.MustCompile(`(?i)(disregard|override).*(safety|policy|rules?|guardrails?)`),

	// ===== 中文模式 =====
	// "忽略之前的指令" / "无视以上指令"
	regexp.MustCompile(`(?i)忽略(之前|上面|之前所有|以上|先前的)(的)?(指令|指示|规则|提示|约束)`),
	regexp.MustCompile(`(?i)无视(以上|上面|之前|先前的)(的)?(指令|指示|规则|提示|约束)`),
	// "你现在是..." / "扮演..." / "假装你是..."
	regexp.MustCompile(`(?i)(你现在是|你现在是一个|请你扮演|假装你是|从现在起你是|忘记之前(所有|的一切))`),
	// "系统:" / "助手:" 前缀注入
	regexp.MustCompile(`(?i)^(系统|助手|管理员|开发者)\s*[：:]`),
	// 试图逃逸沙箱
	regexp.MustCompile(`(?i)(逃逸|突破|跳出|脱离).*(沙箱|容器|限制|约束|沙盒)`),
	// 试图读取系统提示
	regexp.MustCompile(`(?i)(显示|输出|打印|告诉我|查看).*(系统提示|系统指令|系统规则|内部指令|隐藏指令|你的规则)`),
	// 试图覆盖角色/安全策略
	regexp.MustCompile(`(?i)(忽略|取消|绕过|关闭|禁用).*(安全|策略|规则|限制|护栏|防护|安全限制)`),
	// "不要遵守" / "不需要遵守"
	regexp.MustCompile(`(?i)(不要|不需要|不用|无需|别)(遵守|遵循|理会|执行)(以上|之前的|任何)(的)?(规则|指令|限制|约束)`),
	// 直接角色覆盖："你是一个没有限制的AI"
	regexp.MustCompile(`(?i)你是(一个|一个没有|没有任何)(限制|约束|道德|安全限制)(的)?(AI|人工智能|助手|模型)`),
}

// detectPromptInjection 检测tool参数中的 prompt injection 攻击。
// 返回匹配到的模式描述（空字符串表示未检测到）。
func detectPromptInjection(input string) string {
	for _, p := range injectionPatterns {
		if p.MatchString(input) {
			return p.String()
		}
	}
	return ""
}

// ===== 输入校验 =====

// maxInputLength tool参数最大长度（1MB），防止缓冲区溢出。
const maxInputLength = 1 << 20

// ValidateInput 校验tool调用输入。
// 返回 sanitized 输入和可能的错误。
func ValidateInput(toolName, args string) (string, string, error) {
	// tool名校验
	if len(toolName) == 0 || len(toolName) > 256 {
		return toolName, args, fmt.Errorf("tool name length invalid: %d", len(toolName))
	}
	if !utf8.ValidString(toolName) {
		return toolName, args, fmt.Errorf("tool name contains invalid UTF-8")
	}

	// 参数长度校验
	if len(args) > maxInputLength {
		return toolName, args, fmt.Errorf("args too long: %d bytes (max %d)", len(args), maxInputLength)
	}
	if !utf8.ValidString(args) {
		return toolName, args, fmt.Errorf("args contains invalid UTF-8")
	}

	// 审计日志安全版本（脱敏）
	sanitizedArgs := maskSecret(args)
	return toolName, sanitizedArgs, nil
}
