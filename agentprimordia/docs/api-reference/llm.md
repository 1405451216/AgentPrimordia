# LLM API 参考

> `package ap` — LLM 抽象层与 Provider 实现。

## LLM 接口

```go
type LLM interface {
    Name() string
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
    ChatStream(ctx context.Context, req ChatRequest, ch chan<- StreamChunk) error
}

type ChatRequest struct {
    Model       string
    Messages    []Message
    Temperature float64
    MaxTokens   int
    Tools       []ToolDefinition     // 工具调用定义
    Stream      bool
}

type ChatResponse struct {
    Content      string
    FinishReason string // stop / length / tool_calls
    Usage        TokenUsage
    ToolCalls    []ToolCall
}

type StreamChunk struct {
    Content string
    Done    bool
}
```

## Provider 列表

| Provider | 构造函数 | 说明 |
|----------|----------|------|
| OpenAI | `llm.NewOpenAIProvider(cfg)` | GPT-4 / GPT-4o |
| Anthropic | `llm.NewAnthropicProvider(cfg)` | Claude 3.5 / 4 |
| Gemini | `llm.NewGeminiProvider(cfg)` | Google Gemini |
| Ollama | `llm.NewOllamaProvider(cfg)` | 本地 Ollama |
| Azure | `llm.NewAzureProvider(cfg)` | Azure OpenAI |
| DeepSeek | `llm.NewDeepSeekProvider(cfg)` | DeepSeek |
| Qwen | `llm.NewQwenProvider(cfg)` | 通义千问 |

## Provider 配置

```go
type LLMConfig struct {
    Provider  string        // openai / anthropic / ...
    Model     string        // gpt-4o / claude-3-5-sonnet / ...
    APIKey    string        // 建议 ${ENV_VAR}
    Endpoint  string        // 自定义端点（本地推理）
    Timeout   time.Duration
    Retry     RetryPolicy
    Cache     CacheConfig
}
```

## Registry（多模型路由）

```go
type Registry struct{}

func NewRegistry() *Registry
func (r *Registry) Register(name string, provider LLM)
func (r *Registry) Get(name string) (LLM, bool)
func (r *Registry) SetRouter(router Router)  // 设置路由规则
```

## 缓存

```go
type CacheConfig struct {
    Enabled   bool
    Backend   CacheBackend // InMemory / Redis / Disk
    TTL       time.Duration
    Prefix    string       // key 前缀（区分不同 Provider）
}
```

## 弹性 Provider

```go
// 带重试、熔断、降级的 Provider 包装
func NewResilientProvider(primary LLM, opts ResilientOpts) LLM

type ResilientOpts struct {
    MaxRetries  int
    Backoff     time.Duration
    Fallback    LLM         // 降级 LLM
    CircuitBreaker CircuitBreakerConfig
}
```
