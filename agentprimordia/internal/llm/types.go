package llm

import (
	"context"
)

// Provider 是 LLM Provider 的统一接口。
// 实现者需提供同步补全（Complete）、流式补全（Stream）和tool调用（CallTools）能力。
// Stream 返回只读 channel（<-chan Chunk），调用方通过 range 消费，channel 在流结束后由 Provider 关闭。
type Provider interface {
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
	Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error)
	CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error)
	Info() ModelInfo
}

// Embedder 嵌入接口，用于需要嵌入功能的场景
// 不支持 Embeddings 的 Provider 可不实现此接口，调用方通过类型断言检查
type Embedder interface {
	Embeddings(ctx context.Context, texts []string) ([][]float32, error)
}

type Config struct {
	APIKey      string         `json:"-"`
	BaseURL     string         `json:"base_url,omitempty"`
	Model       string         `json:"model"`
	Temperature float64        `json:"temperature,omitempty"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
	Extra       map[string]any `json:"extra,omitempty"`
}

type CompletionRequest struct {
	Messages       []ChatMessage   `json:"messages"`
	Model          string          `json:"model,omitempty"`
	Temperature    *float64        `json:"temperature,omitempty"` // 指针类型，用于区分"未设置"和显式设为 0
	MaxTokens      int             `json:"max_tokens,omitempty"`
	Stream         bool            `json:"stream,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"` // 结构化输出格式控制
}

type ChatMessage struct {
	Role        string         `json:"role"`
	Content     string         `json:"content"`
	ToolCalls   []FunctionCall `json:"tool_calls,omitempty"`
	ToolCallID  string         `json:"tool_call_id,omitempty"`
	IsToolError bool           `json:"is_error,omitempty"`
}

type CompletionResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Content string `json:"content"`
	Role    string `json:"role"`
	Usage   Usage  `json:"usage"`
}

type Chunk struct {
	Content string `json:"content"`
	Done    bool   `json:"done"`
	Usage   *Usage `json:"usage,omitempty"`
}

type ToolCallRequest struct {
	Messages []ChatMessage    `json:"messages"`
	Tools    []ToolDefinition `json:"tools"`
	Model    string           `json:"model,omitempty"`
}

type ToolDefinition struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

type FunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ToolCallResponse struct {
	Content   string         `json:"content"`
	ToolCalls []FunctionCall `json:"tool_calls,omitempty"`
	Usage     Usage          `json:"usage"`
}

type FunctionCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ModelInfo struct {
	Name              string `json:"name"`
	Provider          string `json:"provider"`
	MaxContext        int    `json:"max_context"`
	SupportsTools     bool   `json:"supports_tools"`
	SupportsStreaming bool   `json:"supports_streaming"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ResolveTemperature 从请求和配置中解析有效温度。
// 优先级：req.Temperature（如果已设置）> config.Temperature（如果 > 0）> 未设置。
// 如果没有显式设置温度，则返回 nil。
func ResolveTemperature(reqTemp *float64, configTemp float64) *float64 {
	if reqTemp != nil {
		return reqTemp
	}
	if configTemp > 0 {
		return &configTemp
	}
	return nil
}

// Float64Ptr 是从字面值创建 *float64 的辅助函数。
func Float64Ptr(v float64) *float64 {
	return &v
}
