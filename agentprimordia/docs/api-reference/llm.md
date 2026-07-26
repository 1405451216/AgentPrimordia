# LLM API Reference

## Provider Interface

```go
type Provider interface {
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    StreamComplete(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error)
    Name() string
}
```

## Config

```go
type Config struct {
    APIKey      string
    Model       string
    BaseURL     string        // 可选，覆盖默认 API 地址
    Timeout     time.Duration // 默认 120s
    MaxRetries  int
    Temperature float64
    MaxTokens   int
}
```

## CompletionRequest / CompletionResponse

```go
type CompletionRequest struct {
    Messages    []Message
    Model       string
    Temperature float64
    MaxTokens   int
    Tools       []ToolDefinition
    Stream      bool
}

type CompletionResponse struct {
    Content          string
    ToolCalls        []ToolCall
    PromptTokens     int
    CompletionTokens int
    Model            string
    FinishReason     string
}
```

## 构造函数

```go
func NewOpenAIProvider(cfg Config) (*OpenAIProvider, error)
func NewAnthropicProvider(cfg Config) (*AnthropicProvider, error)
func NewGeminiProvider(cfg Config) (*GeminiProvider, error)
func NewOllamaProvider(cfg Config) (*OllamaProvider, error)
func NewAzureProvider(cfg Config) (*AzureProvider, error)
func NewCohereProvider(cfg Config) (*CohereProvider, error)
func NewMistralProvider(cfg Config) (*MistralProvider, error)
func NewQwenProvider(cfg Config) (*QwenProvider, error)
func NewGLMProvider(cfg Config) (*GLMProvider, error)
func NewDeepSeekProvider(cfg Config) (*DeepSeekProvider, error)
```

## 弹性包装器

```go
func NewResilientProvider(primary Provider, opts ...ResilientOption) *ResilientProvider
func NewCachedProvider(inner Provider, cache Cache) *CachedProvider
func NewModelRouter(rules ...RoutingRule) *ModelRouter
```

## Sentinel Errors

```go
var (
    ErrLLMCallFailed       // LLM 调用失败
    ErrNotSupported        // 操作不被支持
    ErrCircuitOpen         // 熔断器已打开
    ErrAPIKeyRequired      // API Key 未提供
    ErrEmptyResponse       // 空响应
    ErrResponseParseFailed // 响应解析失败
    ErrRetriesExhausted    // 重试耗尽
    ErrFallbackFailed      // 所有降级 Provider 均失败
)
```
