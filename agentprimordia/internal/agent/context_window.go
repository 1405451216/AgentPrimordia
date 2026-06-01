package agent

type ContextWindowStrategy interface {
	// Trim 裁剪消息历史，maxMessages 为最大保留条数
	// 当 maxMessages <= 0 时，由策略实现自行决定默认值
	Trim(messages []Message, maxMessages int) []Message
}

type DefaultStrategy struct {
	KeepLast int
}

func NewDefaultStrategy(keepLast int) *DefaultStrategy {
	if keepLast <= 0 {
		keepLast = 80
	}
	return &DefaultStrategy{KeepLast: keepLast}
}

func (s *DefaultStrategy) Trim(messages []Message, maxMessages int) []Message {
	if len(messages) == 0 {
		return messages
	}

	// When maxMessages is 0, use KeepLast as the effective max
	effectiveMax := maxMessages
	if effectiveMax <= 0 {
		effectiveMax = s.KeepLast
	}

	if len(messages) <= effectiveMax {
		return messages
	}

	result := make([]Message, 0, effectiveMax)
	result = append(result, messages[0])

	remaining := effectiveMax - 1
	if remaining > s.KeepLast {
		remaining = s.KeepLast
	}

	start := len(messages) - remaining
	if start < 1 {
		start = 1
	}

	result = append(result, messages[start:]...)
	return result
}
