package llm

import "context"

// GenericProvider 是泛型 LLM Provider 接口（v2.0 #3 泛型 Provider 重构）。
// 参数 T 预留用于 Provider 自身配置类型的特殊化。当前 AgentPrimordia
// 的所有 Provider 共享同一 Config，因此统一实例化为 GenericProvider[any]。
//
// 未来当某个 Provider 需要暴露特有配置字段时（如 Anthropic 的
// Version 字段、Google 的 SafetySettings 等），可将其特化为对应
// 具体类型而不影响其他 Provider。
//
// 现状的 Provider 接口（types.go）保持不变以避免大面积破坏；
// GenericProvider[any] 与 Provider 在方法集上等价，可共存。
type GenericProvider[T any] interface {
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
	Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error)
	CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error)
	Info() ModelInfo
}

// OpenAIBase 暴露 OpenAI 兼容 API 的公共方法。
// 所有兼容 OpenAI API 的 Provider（Qwen、DeepSeek、GLM、Mistral 等）
// 均可嵌入此类型以获得 DoRequest / BuildMessages / BuildResponseFormat
// 等共享方法，避免重复实现。
type OpenAIBase = BaseProvider

