package llm

import "encoding/json"

// openaiChatRequest OpenAI 兼容的 chat completions 请求体（perf-v6 Task C: 减少反射）
type openaiChatRequest struct {
	Model          string            `json:"model"`
	Messages       []map[string]any  `json:"messages"`
	Temperature    *float64          `json:"temperature,omitempty"`
	MaxTokens      int               `json:"max_tokens,omitempty"`
	Stream         bool              `json:"stream,omitempty"`
	Tools          []openaiTool      `json:"tools,omitempty"`
	ResponseFormat *openaiRespFormat `json:"response_format,omitempty"`
}

// openaiRespFormat OpenAI response_format typed struct
type openaiRespFormat struct {
	Type       string            `json:"type"`
	JSONSchema *openaiJSONSchema `json:"json_schema,omitempty"`
}

// openaiJSONSchema OpenAI json_schema typed struct
type openaiJSONSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	Strict      bool            `json:"strict,omitempty"`
}

// openaiEmbedRequest OpenAI embeddings 请求体
type openaiEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// azureChatRequest Azure OpenAI chat completions 请求体（model 由 deployment 路径承载，不写入 body）
type azureChatRequest struct {
	Messages    []map[string]any `json:"messages"`
	Temperature *float64         `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
	Tools       []openaiTool     `json:"tools,omitempty"`
}

// buildAzureMessages 复用 OpenAI 消息格式
var buildAzureMessages = BuildOpenAIMessages

func convertResponseFormatToTyped(rf *ResponseFormat) *openaiRespFormat {
	if rf == nil {
		return nil
	}
	out := &openaiRespFormat{Type: string(rf.Type)}
	if rf.Type == ResponseFormatJSONSchema && rf.JSONSchema != nil {
		inner := &openaiJSONSchema{
			Name:        rf.JSONSchema.Name,
			Description: rf.JSONSchema.Description,
			Strict:      rf.JSONSchema.Strict,
		}
		// Schema 是 map[string]any，需要重新 marshal 为 json.RawMessage 以保持 typed struct 的零反射序列化
		if rf.JSONSchema.Schema != nil {
			raw, err := json.Marshal(rf.JSONSchema.Schema)
			if err == nil {
				inner.Schema = raw
			}
		}
		out.JSONSchema = inner
	}
	return out
}
