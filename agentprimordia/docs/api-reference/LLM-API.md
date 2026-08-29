# LLM API Reference

## Provider Interface

```go
type Provider interface {
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error)
    CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error)
    Info() ModelInfo
}
```

## Config

```go
type Config struct {
    APIKey      string         `json:"-"`                 // 注入注：序列化时隐藏
    BaseURL     string         `json:"base_url,omitempty"`
    Model       string         `json:"model"`
    Temperature float64        `json:"temperature,omitempty"`
    MaxTokens   int            `json:"max_tokens,omitempty"`
    Extra       map[string]any `json:"extra,omitempty"`
}
```

> 超时/重试等弹性参数不在 `Config` 中，经 `NewResilientProvider` 的 `ResilientConfig` 配置。

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
func NewAzureOpenAIProvider(cfg AzureConfig) (*AzureOpenAIProvider, error)
func NewCohereProvider(cfg Config) (*CohereProvider, error)
func NewMistralProvider(cfg Config) (*MistralProvider, error)
func NewQwenProvider(cfg Config) (*QwenProvider, error)
func NewGLMProvider(cfg Config) (*GLMProvider, error)
```

另有环境变量驱动入口 `ProviderFromEnv()` / `ConfigFromEnv()`（读取 `AP_LLM_PROVIDER` / `AP_LLM_MODEL` / `AP_LLM_API_KEY` / `AP_LLM_BASE_URL`）。

## 弹性包装器

```go
// 弹性 Provider：自动重试 / 降级 / 熔断（cfg 可用 DefaultResilientConfig() 获取默认值）
func NewResilientProvider(primary Provider, cfg ResilientConfig) (*ResilientProvider, error)

// 缓存装饰器：minScore 为语义缓存相似度阈值（精确/混合缓存可传 0）
func NewCachedProvider(inner Provider, cache LLMCache, minScore float32) (*CachedProvider, error)
func NewCachedProviderWithManager(inner Provider, mgr *CacheManager, minScore float32) (*CachedProvider, error)

// 模型路由器：按策略路由，策略取值 StrategyCostFirst / StrategyQualityFirst / StrategyBalanced
func NewModelRouter(strategy RouteStrategy) *ModelRouter
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
