package guardrail

import (
	"fmt"
	"strings"
)

const defaultRedactText = "[REDACTED]"

// SanitizeStrategy 脱敏策略
type SanitizeStrategy string

const (
	StrategyMask    SanitizeStrategy = "mask"
	StrategyRedact  SanitizeStrategy = "redact"
	StrategyReplace SanitizeStrategy = "replace"
	StrategyHash    SanitizeStrategy = "hash"
)

// Sanitizer 脱敏处理器
type Sanitizer struct {
	strategy SanitizeStrategy
	maskChar rune
	replText string
}

// SanitizerConfig 脱敏处理器配置
type SanitizerConfig struct {
	Strategy SanitizeStrategy
	MaskChar rune
	ReplText string
}

// NewSanitizer 创建脱敏处理器
func NewSanitizer(config SanitizerConfig) *Sanitizer {
	maskChar := config.MaskChar
	if maskChar == 0 {
		maskChar = '*'
	}
	replText := config.ReplText
	if replText == "" {
		replText = defaultRedactText
	}
	return &Sanitizer{
		strategy: config.Strategy,
		maskChar: maskChar,
		replText: replText,
	}
}

// Sanitize 对文本进行脱敏处理
func (s *Sanitizer) Sanitize(text string, positions []Position) string {
	if len(positions) == 0 {
		return text
	}
	runes := []rune(text)
	for _, pos := range positions {
		if pos.Start < 0 || pos.End > len(runes) || pos.Start >= pos.End {
			continue
		}
		length := pos.End - pos.Start
		replacement := s.applyStrategy(length, pos.Label)
		result := make([]rune, 0, len(runes)-length+len([]rune(replacement)))
		result = append(result, runes[:pos.Start]...)
		result = append(result, []rune(replacement)...)
		result = append(result, runes[pos.End:]...)
		runes = result
	}
	return string(runes)
}

// Position 脱敏位置
type Position struct {
	Start int
	End   int
	Label string
}

func (s *Sanitizer) applyStrategy(length int, label string) string {
	switch s.strategy {
	case StrategyMask:
		return strings.Repeat(string(s.maskChar), length)
	case StrategyRedact:
		return s.replText
	case StrategyReplace:
		if label != "" {
			return fmt.Sprintf("[%s_REDACTED]", strings.ToUpper(label))
		}
		return s.replText
	case StrategyHash:
		return fmt.Sprintf("[#%d]", length)
	default:
		return strings.Repeat(string(s.maskChar), length)
	}
}

// SanitizeReport 根据护栏报告对文本进行脱敏
// 有意设计为独立函数而非 Sanitizer 方法：此函数无状态，不需要 Sanitizer 实例的配置
func SanitizeReport(text string, report *Report, strategy SanitizeStrategy) string {
	if report == nil || report.Passed {
		return text
	}
	for _, result := range report.Results {
		if result.Sanitized != "" {
			text = result.Sanitized
		}
	}
	return text
}
