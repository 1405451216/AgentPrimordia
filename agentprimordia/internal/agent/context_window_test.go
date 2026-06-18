package agent

import (
	"testing"
)

func TestDefaultTrim_UnderLimit(t *testing.T) {
	strategy := NewDefaultStrategy(80)

	messages := []Message{
		SystemMessage("system"),
		UserMessage("hello"),
		{Role: RoleAssistant, Content: "hi"},
	}

	result := strategy.Trim(messages, 10)
	if len(result) != 3 {
		t.Errorf("expected 3 messages, got %d", len(result))
	}
}

func TestDefaultTrim_ExceedLimit(t *testing.T) {
	strategy := NewDefaultStrategy(5)

	messages := []Message{
		SystemMessage("system"),
		UserMessage("msg1"),
		{Role: RoleAssistant, Content: "reply1"},
		UserMessage("msg2"),
		{Role: RoleAssistant, Content: "reply2"},
		UserMessage("msg3"),
		{Role: RoleAssistant, Content: "reply3"},
		UserMessage("msg4"),
		{Role: RoleAssistant, Content: "reply4"},
		UserMessage("msg5"),
		{Role: RoleAssistant, Content: "reply5"},
	}

	result := strategy.Trim(messages, 6)
	if len(result) != 6 {
		t.Errorf("expected 6 messages, got %d", len(result))
	}

	if result[0].Role != RoleSystem {
		t.Error("first message should be system message")
	}

	if result[1].Content != "reply3" {
		t.Errorf("expected second message to be 'reply3', got '%s'", result[1].Content)
	}
}

func TestDefaultTrim_OnlySystem(t *testing.T) {
	strategy := NewDefaultStrategy(80)

	messages := []Message{
		SystemMessage("system only"),
	}

	result := strategy.Trim(messages, 10)
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}

func TestDefaultTrim_EmptyMessages(t *testing.T) {
	strategy := NewDefaultStrategy(80)

	messages := []Message{}

	result := strategy.Trim(messages, 10)
	if len(result) != 0 {
		t.Errorf("expected 0 messages, got %d", len(result))
	}
}

func TestDefaultTrim_CustomKeepLast(t *testing.T) {
	strategy := NewDefaultStrategy(3)

	messages := []Message{
		SystemMessage("system"),
		UserMessage("msg1"),
		{Role: RoleAssistant, Content: "reply1"},
		UserMessage("msg2"),
		{Role: RoleAssistant, Content: "reply2"},
		UserMessage("msg3"),
		{Role: RoleAssistant, Content: "reply3"},
	}

	result := strategy.Trim(messages, 4)
	if len(result) != 4 {
		t.Errorf("expected 4 messages, got %d", len(result))
	}

	if result[0].Role != RoleSystem {
		t.Error("first message should be system message")
	}

	if result[1].Content != "reply2" {
		t.Errorf("expected second message to be 'reply2', got '%s'", result[1].Content)
	}
}

func TestDefaultTrim_ZeroKeepLast(t *testing.T) {
	strategy := NewDefaultStrategy(0)

	messages := []Message{
		SystemMessage("system"),
		UserMessage("msg1"),
		{Role: RoleAssistant, Content: "reply1"},
	}

	result := strategy.Trim(messages, 1)
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}

	if result[0].Role != RoleSystem {
		t.Error("first message should be system message")
	}
}

func TestDefaultTrim_ZeroMaxMessages(t *testing.T) {
	strategy := NewDefaultStrategy(80)

	messages := []Message{
		SystemMessage("system"),
		UserMessage("msg1"),
	}

	result := strategy.Trim(messages, 0)
	if len(result) != 2 {
		t.Errorf("expected 2 messages (no trimming), got %d", len(result))
	}
}

func TestDefaultStrategy_DefaultKeepLast(t *testing.T) {
	strategy := NewDefaultStrategy(0)
	if strategy.KeepLast != 80 {
		t.Errorf("expected default KeepLast=80, got %d", strategy.KeepLast)
	}
}

// perf-v4 Task 11：当未配置自定义 ContextWindowStrategy 时，trimContext
// 应回退到默认滑动窗口（保留系统消息 + 最近 N 条）
func TestReActAgent_TrimContext_DefaultSlidingWindow(t *testing.T) {
	agent := &ReActAgent{} // contextWindow 为 nil，触发默认路径

	// 场景 1：消息数 < 上限，原样返回
	small := makeMessages(defaultMaxHistoryMessages - 1)
	if got := agent.trimContext(small, 0); len(got) != len(small) {
		t.Errorf("under-limit 应原样返回，期望 %d 条，实际 %d 条", len(small), len(got))
	}

	// 场景 2：消息数 > 上限，保留系统消息 + 最近 N-1 条
	big := makeMessages(defaultMaxHistoryMessages + 50)
	result := agent.trimContext(big, 0)
	if len(result) != defaultMaxHistoryMessages {
		t.Errorf("期望保留 %d 条，实际 %d 条", defaultMaxHistoryMessages, len(result))
	}
	if result[0].Role != RoleSystem {
		t.Errorf("首位应为系统消息，实际为 %s", result[0].Role)
	}

	// 场景 3：自定义 maxMessages 应被尊重
	big2 := makeMessages(200)
	result2 := agent.trimContext(big2, 10)
	if len(result2) != 10 {
		t.Errorf("maxMessages=10 应保留 10 条，实际 %d 条", len(result2))
	}
	if result2[0].Role != RoleSystem {
		t.Errorf("首位应为系统消息")
	}
}

// makeMessages 构造包含 1 条系统消息 + n-1 条 user/assistant 交替的 history
func makeMessages(n int) []Message {
	if n <= 0 {
		return nil
	}
	msgs := make([]Message, 0, n)
	msgs = append(msgs, SystemMessage("system prompt"))
	for i := 1; i < n; i++ {
		if i%2 == 1 {
			msgs = append(msgs, UserMessage("user msg"))
		} else {
			msgs = append(msgs, Message{Role: RoleAssistant, Content: "assistant msg"})
		}
	}
	return msgs
}
