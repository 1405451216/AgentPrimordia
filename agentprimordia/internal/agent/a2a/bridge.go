package a2a

import (
	"fmt"
)

// MessageBridge 消息转换桥：在 A2A 消息和内部 Agent 消息格式间转换
type MessageBridge struct{}

func NewMessageBridge() *MessageBridge { return &MessageBridge{} }

// A2AMessageToParts 提取 A2AMessage 中的所有 Part
func (b *MessageBridge) A2AMessageToParts(msg *A2AMessage) []Part {
	if msg == nil {
		return nil
	}
	return msg.Parts
}

// PartsToA2AMessage 将 Part 列表组装为 A2AMessage
func (b *MessageBridge) PartsToA2AMessage(role string, parts []Part) *A2AMessage {
	if len(parts) == 0 {
		return nil
	}
	return &A2AMessage{Role: role, Parts: parts}
}

// ExtractText 从 A2AMessage 提取纯文本
func (b *MessageBridge) ExtractText(msg *A2AMessage) string {
	if msg == nil {
		return ""
	}
	return ExtractTextFromParts(msg.Parts)
}

// TaskToStatusMessage 将 Task 状态转换为状态消息
func (b *MessageBridge) TaskToStatusMessage(task *Task) *A2AMessage {
	if task == nil {
		return nil
	}
	stateText := string(task.State)
	if task.Status != nil && task.Status.ErrorMessage != "" {
		stateText = fmt.Sprintf("%s: %s", task.State, task.Status.ErrorMessage)
	}
	return &A2AMessage{
		Role:  "agent",
		Parts: []Part{NewTextPart(stateText)},
	}
}

// MergeMessages 合并多条消息的文本内容
func (b *MessageBridge) MergeMessages(messages []*A2AMessage) *A2AMessage {
	if len(messages) == 0 {
		return nil
	}
	var allParts []Part
	for _, msg := range messages {
		if msg != nil {
			allParts = append(allParts, msg.Parts...)
		}
	}
	if len(allParts) == 0 {
		return nil
	}
	return &A2AMessage{Role: "agent", Parts: allParts}
}

// FilterPartsByType 按 Part 类型过滤
func (b *MessageBridge) FilterPartsByType(msg *A2AMessage, partType string) []Part {
	if msg == nil {
		return nil
	}
	var result []Part
	for _, p := range msg.Parts {
		if p.Type() == partType {
			result = append(result, p)
		}
	}
	return result
}
