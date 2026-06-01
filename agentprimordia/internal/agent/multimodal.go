package agent

import (
	"agentprimordia/internal/llm"
	"time"
)

// MultimodalAdapter 多模态消息适配器
// 将 agent.ContentPart 转换为 llm.MultimodalContent
type MultimodalAdapter struct{}

// ToLLMContents 将 agent ContentParts 转换为 llm.MultimodalContent 列表
func (a *MultimodalAdapter) ToLLMContents(parts []ContentPart) []*llm.MultimodalContent {
	if len(parts) == 0 {
		return nil
	}

	result := make([]*llm.MultimodalContent, len(parts))
	for i, p := range parts {
		c := &llm.MultimodalContent{}

		switch p.Type {
		case "text":
			c.Type = llm.ContentTypeText
			c.Text = p.Text
		case "image_url":
			c.Type = llm.ContentTypeImageURL
			c.URL = p.URL
			if p.Detail != "" {
				c.Detail = p.Detail
			} else {
				c.Detail = "auto"
			}
		case "image_b64":
			c.Type = llm.ContentTypeImageB64
			c.Data = p.Data
			c.MIME = p.MIME
			if p.Detail != "" {
				c.Detail = p.Detail
			} else {
				c.Detail = "auto"
			}
		case "audio":
			c.Type = llm.ContentTypeAudio
			c.Data = p.Data
			c.MIME = p.MIME
		case "video":
			c.Type = llm.ContentTypeVideo
			c.Data = p.Data
			c.MIME = p.MIME
		default:
			c.Type = llm.ContentTypeText
			c.Text = p.Text
		}

		result[i] = c
	}
	return result
}

// HistoryHasMultimodal 检查历史消息中是否包含多模态内容
func (a *MultimodalAdapter) HistoryHasMultimodal(history []Message) bool {
	for _, m := range history {
		if m.HasMultimodal() {
			return true
		}
	}
	return false
}

// ConvertHistoryToExt 将历史消息转换为 LLM ChatMessageExt 格式
// 如果消息包含 ContentParts，使用多模态格式；否则降级为纯文本
func (a *MultimodalAdapter) ConvertHistoryToExt(history []Message) []*llm.ChatMessageExt {
	msgs := make([]*llm.ChatMessageExt, 0, len(history))
	for _, m := range history {
		ext := &llm.ChatMessageExt{
			Role: string(m.Role),
		}

		if len(m.ContentParts) > 0 {
			ext.Contents = a.ToLLMContents(m.ContentParts)
		} else {
			ext.Contents = []*llm.MultimodalContent{
				llm.NewTextContent(m.Content),
			}
		}

		if m.Role == RoleAssistant && len(m.ToolCalls) > 0 {
			ext.ToolCalls = make([]llm.FunctionCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				ext.ToolCalls[j] = llm.FunctionCall{
					ID:        tc.ID,
					Name:      tc.Name,
					Arguments: tc.Args,
				}
			}
		}

		if m.Role == RoleTool {
			if id, ok := m.Metadata.Extra["tool_call_id"]; ok {
				ext.ToolCallID = id
			}
			if isError, ok := m.Metadata.Extra["is_error"]; ok && isError == "true" {
				ext.IsToolError = true
			}
		}

		msgs = append(msgs, ext)
	}
	return msgs
}

// UserMultimodalMessage 创建多模态用户消息
func UserMultimodalMessage(parts ...ContentPart) Message {
	return Message{
		Role:         RoleUser,
		ContentParts: parts,
		Metadata:     Metadata{Timestamp: time.Now()},
	}
}

// UserImageMessage 便捷函数：创建图片用户消息
func UserImageMessage(text, imageURL string) Message {
	return Message{
		Role: RoleUser,
		ContentParts: []ContentPart{
			{Type: "text", Text: text},
			{Type: "image_url", URL: imageURL, Detail: "auto"},
		},
		Metadata: Metadata{Timestamp: time.Now()},
	}
}
