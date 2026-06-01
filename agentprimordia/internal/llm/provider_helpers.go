package llm

// ResolveModel 解析使用的模型名称，优先使用请求中的模型，否则使用配置中的模型
func ResolveModel(reqModel, configModel string) string {
	if reqModel != "" {
		return reqModel
	}
	return configModel
}

// BuildOpenAIMessages 构建 OpenAI 兼容格式的消息列表
// 包含 tool_calls 和 tool_call_id 的处理，适用于 OpenAI/Qwen/Cohere/Mistral 等 Provider
func BuildOpenAIMessages(msgs []ChatMessage) []map[string]any {
	result := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		msg := map[string]any{
			"role":    m.Role,
			"content": m.Content,
		}

		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			toolCalls := make([]map[string]any, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				toolCalls[j] = map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				}
			}
			msg["tool_calls"] = toolCalls
		}

		if m.Role == "tool" {
			if m.ToolCallID != "" {
				msg["tool_call_id"] = m.ToolCallID
			}
		}

		result = append(result, msg)
	}
	return result
}

// ConvertRequestToExt 将 CompletionRequest 转换为 CompletionRequestExt
// 用于多模态 Provider 的向后兼容接口实现
func ConvertRequestToExt(req *CompletionRequest) *CompletionRequestExt {
	extReq := &CompletionRequestExt{
		Messages:    make([]*ChatMessageExt, len(req.Messages)),
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      req.Stream,
	}
	for i, m := range req.Messages {
		extReq.Messages[i] = NewUserTextMessage(m.Content)
		extReq.Messages[i].Role = m.Role
		extReq.Messages[i].ToolCalls = m.ToolCalls
		extReq.Messages[i].ToolCallID = m.ToolCallID
		extReq.Messages[i].IsToolError = m.IsToolError
	}
	return extReq
}
