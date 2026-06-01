package a2a

import (
	"encoding/json"
	"testing"
)

func TestMessageBridge_A2AMessageToParts(t *testing.T) {
	bridge := NewMessageBridge()
	msg := &A2AMessage{
		Role:  "user",
		Parts: []Part{NewTextPart("hello"), NewTextPart("world")},
	}

	parts := bridge.A2AMessageToParts(msg)
	if len(parts) != 2 {
		t.Errorf("应有 2 个 Part, got %d", len(parts))
	}
}

func TestMessageBridge_A2AMessageToPartsNil(t *testing.T) {
	bridge := NewMessageBridge()
	parts := bridge.A2AMessageToParts(nil)
	if parts != nil {
		t.Error("nil 消息应返回 nil Parts")
	}
}

func TestMessageBridge_PartsToA2AMessage(t *testing.T) {
	bridge := NewMessageBridge()
	parts := []Part{NewTextPart("test")}
	msg := bridge.PartsToA2AMessage("agent", parts)

	if msg == nil {
		t.Fatal("不应返回 nil")
	}
	if msg.Role != "agent" {
		t.Errorf("Role 应为 agent, got %s", msg.Role)
	}
	if len(msg.Parts) != 1 {
		t.Errorf("应有 1 个 Part, got %d", len(msg.Parts))
	}
}

func TestMessageBridge_PartsToA2AMessageEmpty(t *testing.T) {
	bridge := NewMessageBridge()
	msg := bridge.PartsToA2AMessage("agent", nil)
	if msg != nil {
		t.Error("空 Parts 应返回 nil")
	}
}

func TestMessageBridge_ExtractText(t *testing.T) {
	bridge := NewMessageBridge()
	msg := &A2AMessage{
		Role:  "user",
		Parts: []Part{NewTextPart("hello "), NewTextPart("world")},
	}

	text := bridge.ExtractText(msg)
	if text != "hello world" {
		t.Errorf("文本不匹配: got %q", text)
	}
}

func TestMessageBridge_ExtractTextNil(t *testing.T) {
	bridge := NewMessageBridge()
	text := bridge.ExtractText(nil)
	if text != "" {
		t.Errorf("nil 消息应返回空字符串, got %q", text)
	}
}

func TestMessageBridge_TaskToStatusMessage(t *testing.T) {
	bridge := NewMessageBridge()
	task := &Task{
		ID:     "task-001",
		State:  TaskCompleted,
		Status: &TaskStatus{State: TaskCompleted, ErrorMessage: "处理完成"},
	}

	msg := bridge.TaskToStatusMessage(task)
	if msg == nil {
		t.Fatal("不应返回 nil")
	}
	text := ExtractTextFromParts(msg.Parts)
	if text != "completed: 处理完成" {
		t.Errorf("状态消息不匹配: got %q", text)
	}
}

func TestMessageBridge_TaskToStatusMessageNoStatus(t *testing.T) {
	bridge := NewMessageBridge()
	task := &Task{ID: "task-002", State: TaskWorking}

	msg := bridge.TaskToStatusMessage(task)
	text := ExtractTextFromParts(msg.Parts)
	if text != "working" {
		t.Errorf("无 Status 时应只显示状态, got %q", text)
	}
}

func TestMessageBridge_TaskToStatusMessageNil(t *testing.T) {
	bridge := NewMessageBridge()
	msg := bridge.TaskToStatusMessage(nil)
	if msg != nil {
		t.Error("nil Task 应返回 nil")
	}
}

func TestMessageBridge_MergeMessages(t *testing.T) {
	bridge := NewMessageBridge()
	msgs := []*A2AMessage{
		{Role: "agent", Parts: []Part{NewTextPart("part1")}},
		{Role: "agent", Parts: []Part{NewTextPart("part2")}},
	}

	merged := bridge.MergeMessages(msgs)
	if merged == nil {
		t.Fatal("不应返回 nil")
	}
	if len(merged.Parts) != 2 {
		t.Errorf("合并后应有 2 个 Part, got %d", len(merged.Parts))
	}
}

func TestMessageBridge_MergeMessagesEmpty(t *testing.T) {
	bridge := NewMessageBridge()
	msg := bridge.MergeMessages(nil)
	if msg != nil {
		t.Error("空列表应返回 nil")
	}
}

func TestMessageBridge_FilterPartsByType(t *testing.T) {
	bridge := NewMessageBridge()
	msg := &A2AMessage{
		Role: "user",
		Parts: []Part{
			NewTextPart("text content"),
			NewDataPart(json.RawMessage(`{"key":"value"}`)),
			NewTextPart("more text"),
		},
	}

	textParts := bridge.FilterPartsByType(msg, "text")
	if len(textParts) != 2 {
		t.Errorf("应有 2 个 text Part, got %d", len(textParts))
	}

	dataParts := bridge.FilterPartsByType(msg, "data")
	if len(dataParts) != 1 {
		t.Errorf("应有 1 个 data Part, got %d", len(dataParts))
	}
}

func TestMessageBridge_FilterPartsNilMessage(t *testing.T) {
	bridge := NewMessageBridge()
	parts := bridge.FilterPartsByType(nil, "text")
	if parts != nil {
		t.Error("nil 消息应返回 nil")
	}
}
