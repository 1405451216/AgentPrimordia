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
// 优化（Task 7）：预分配切片容量，并对只有 role+content 的简单消息使用紧凑的 map 分配路径。
func BuildOpenAIMessages(msgs []ChatMessage) []map[string]any {
	result := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		// 优化（Task 7）：根据消息类型选择不同的分配路径
		// 简单消息（仅 role+content 或 role+content+tool_call_id）走紧凑路径
		// 复杂消息（assistant 含 tool_calls）保留独立 map 分配
		isSimple := true
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			isSimple = false
		}

		if isSimple {
			// 紧凑路径：预估容量，减少 rehash
			msg := make(map[string]any, 3)
			msg["role"] = m.Role
			msg["content"] = m.Content
			if m.Role == "tool" && m.ToolCallID != "" {
				msg["tool_call_id"] = m.ToolCallID
			}
			result = append(result, msg)
			continue
		}

		// 复杂路径：assistant with tool_calls
		msg := map[string]any{
			"role":    m.Role,
			"content": m.Content,
		}

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

		if m.Role == "tool" && m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
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
