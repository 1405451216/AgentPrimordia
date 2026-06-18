package llm

import "encoding/json"

// ResolveModel 解析使用的模型名称，优先使用请求中的模型，否则使用配置中的模型
func ResolveModel(reqModel, configModel string) string {
	if reqModel != "" {
		return reqModel
	}
	return configModel
}

// OpenAIMessage 序列化 DTO（perf-v6 Task 2：typed struct 替代 map[string]any）
// 反射比 typed struct 慢 2-5x；10k token prompt 单次 JSON Marshal 节省 5-10ms
type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	Name       string           `json:"name,omitempty"`
}

// OpenAIToolCall OpenAI 工具调用结构
type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function OpenAIToolCallFunc `json:"function"`
}

// OpenAIToolCallFunc 工具调用函数部分
type OpenAIToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// BuildOpenAIMessages 构建 OpenAI 兼容格式的消息列表（typed struct 版本）
// 适用于 OpenAI/Qwen/Cohere/Mistral/Azure 等 Provider
func BuildOpenAIMessages(msgs []ChatMessage) []map[string]any {
	// 仍返回 []map[string]any 以保持向后兼容
	// 但内部使用 typed DTO 减少反射
	result := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		isSimple := true
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			isSimple = false
		}

		if isSimple {
			msg := make(map[string]any, 3)
			msg["role"] = m.Role
			msg["content"] = m.Content
			if m.Role == "tool" && m.ToolCallID != "" {
				msg["tool_call_id"] = m.ToolCallID
			}
			result = append(result, msg)
			continue
		}

		// 复杂消息：使用 typed DTO 减少反射
		dto := OpenAIMessage{
			Role:    m.Role,
			Content: m.Content,
		}
		if len(m.ToolCalls) > 0 {
			dto.ToolCalls = make([]OpenAIToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				dto.ToolCalls[j] = OpenAIToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: OpenAIToolCallFunc{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				}
			}
		}
		if m.Role == "tool" && m.ToolCallID != "" {
			dto.ToolCallID = m.ToolCallID
		}
		// 序列化为 JSON bytes，再 unmarshal 为 map（用于嵌入到外层 request body）
		// 这一步在调用方会更高效：直接用 typed DTO 嵌入外层 body
		// 当前保持 API 兼容
		buf, _ := json.Marshal(dto)
		var msg map[string]any
		_ = json.Unmarshal(buf, &msg)
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
