package agent

import (
	"agentprimordia/internal/llm"
	"context"
	"time"
)

// MultimodalMessage 多模态消息（扩展 Message 支持图片/音频/视频）
type MultimodalMessage struct {
	Role      Role                     `json:"role"`
	Contents  []*llm.MultimodalContent `json:"contents,omitempty"` // 多模态内容列表
	Content   string                   `json:"content,omitempty"`  // 纯文本内容（向后兼容）
	ToolCalls []ToolCall               `json:"tool_calls,omitempty"`
	Metadata  Metadata                 `json:"metadata,omitempty"`
}

// HasNonTextContent 是否包含非文本内容
func (m *MultimodalMessage) HasNonTextContent() bool {
	for _, c := range m.Contents {
		if c.Type != llm.ContentTypeText {
			return true
		}
	}
	return false
}

// ExtractText 提取所有文本内容
func (m *MultimodalMessage) ExtractText() string {
	if m.Content != "" {
		return m.Content
	}
	result := ""
	for _, c := range m.Contents {
		if c.Text != "" {
			result += c.Text
		}
	}
	return result
}

// ToChatMessageExt 转换为 LLM 多模态消息格式
func (m *MultimodalMessage) ToChatMessageExt() *llm.ChatMessageExt {
	ext := &llm.ChatMessageExt{
		Role:        string(m.Role),
		Contents:    m.Contents,
		ToolCalls:   make([]llm.FunctionCall, len(m.ToolCalls)),
		IsToolError: false,
	}

	if len(m.Contents) == 0 && m.Content != "" {
		ext.Contents = []*llm.MultimodalContent{
			{Type: llm.ContentTypeText, Text: m.Content},
		}
	}

	for i, tc := range m.ToolCalls {
		ext.ToolCalls[i] = llm.FunctionCall{
			ID:        tc.ID,
			Name:      tc.Name,
			Arguments: tc.Args,
		}
	}

	return ext
}

// NewUserMultimodalMessage 创建用户多模态消息
func NewUserMultimodalMessage(contents ...*llm.MultimodalContent) *MultimodalMessage {
	return &MultimodalMessage{
		Role:     RoleUser,
		Contents: contents,
		Metadata: Metadata{Timestamp: time.Now()},
	}
}

// NewAssistantMultimodalMessage 创建助手多模态消息
func NewAssistantMultimodalMessage(contents ...*llm.MultimodalContent) *MultimodalMessage {
	return &MultimodalMessage{
		Role:     RoleAssistant,
		Contents: contents,
		Metadata: Metadata{Timestamp: time.Now()},
	}
}

// NewSystemMultimodalMessage 创建系统多模态消息
func NewSystemMultimodalMessage(contents ...*llm.MultimodalContent) *MultimodalMessage {
	return &MultimodalMessage{
		Role:     RoleSystem,
		Contents: contents,
		Metadata: Metadata{Timestamp: time.Now()},
	}
}

// UserImageB64Message 创建包含 Base64 图片的用户消息（便捷方法）
func UserImageB64Message(text string, imageBase64 string, mimeType string) *MultimodalMessage {
	return NewUserMultimodalMessage(
		&llm.MultimodalContent{Type: llm.ContentTypeText, Text: text},
		&llm.MultimodalContent{Type: llm.ContentTypeImageB64, Data: imageBase64, MIME: mimeType},
	)
}

// UserImageURLMessage 创建包含图片 URL 的用户消息（便捷方法）
func UserImageURLMessage(text string, imageURL string) *MultimodalMessage {
	return NewUserMultimodalMessage(
		&llm.MultimodalContent{Type: llm.ContentTypeText, Text: text},
		&llm.MultimodalContent{Type: llm.ContentTypeImageURL, URL: imageURL},
	)
}

// ToMessage 转换为标准 Message（降级）
func (m *MultimodalMessage) ToMessage() Message {
	msg := Message{
		Role:      m.Role,
		Content:   m.ExtractText(),
		ToolCalls: m.ToolCalls,
		Metadata:  m.Metadata,
	}
	return msg
}

// MultimodalResponse 多模态响应
type MultimodalResponse struct {
	Response
	MultimodalContent *llm.CompletionResponse // 原始多模态响应
}

// RunMultimodal 执行多模态 Agent 推理
func (a *ReActAgent) RunMultimodal(ctx context.Context, input *MultimodalMessage) (*MultimodalResponse, error) {
	standardInput := input.ToMessage()
	resp, err := a.Run(ctx, standardInput)
	if err != nil {
		return nil, err
	}
	return &MultimodalResponse{Response: *resp}, nil
}

// StreamRunMultimodal 执行流式多模态 Agent 推理
func (a *ReActAgent) StreamRunMultimodal(ctx context.Context, input *MultimodalMessage) (<-chan StreamEvent, error) {
	standardInput := input.ToMessage()
	return a.StreamRun(ctx, standardInput)
}

// convertToLLMMessagesExt 将多模态历史消息转换为 LLM ChatMessageExt 格式
func convertToLLMMessagesExt(history []*MultimodalMessage) []*llm.ChatMessageExt {
	msgs := make([]*llm.ChatMessageExt, 0, len(history))
	for _, m := range history {
		ext := m.ToChatMessageExt()
		msgs = append(msgs, ext)
	}
	return msgs
}

// IsMultimodalProvider 检查 LLM Provider 是否支持多模态
func IsMultimodalProvider(p llm.Provider) bool {
	if mp, ok := p.(interface {
		CompleteMultimodal(ctx context.Context, req *llm.CompletionRequestExt) (*llm.CompletionResponse, error)
	}); ok {
		_ = mp
		return true
	}
	return false
}
