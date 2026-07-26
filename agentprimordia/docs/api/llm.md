# LLM API

LLM 抽象层提供统一的 Provider 接口，屏蔽不同模型服务商的 API 差异。

## Provider 接口

```go
type Provider interface {
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    StreamComplete(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error)
    Name() string
}
```

## 支持的 Provider

| Provider | 构造函数 | 说明 |
|----------|----------|------|
| OpenAI | `NewOpenAIProvider(cfg)` | GPT-4o / o1 / o3 系列 |
| Anthropic | `NewAnthropicProvider(cfg)` | Claude 系列 |
| Gemini | `NewGeminiProvider(cfg)` | Google Gemini |
| Ollama | `NewOllamaProvider(cfg)` | 本地模型 |
| Azure | `NewAzureProvider(cfg)` | Azure OpenAI |
| Cohere | `NewCohereProvider(cfg)` | Command 系列 |
| Mistral | `NewMistralProvider(cfg)` | Mistral 系列 |
| Qwen | `NewQwenProvider(cfg)` | 通义千问 |
| GLM | `NewGLMProvider(cfg)` | 智谱 GLM |
| DeepSeek | `NewDeepSeekProvider(cfg)` | DeepSeek |

## 配置

```go
cfg := llm.Config{
    APIKey:  os.Getenv("OPENAI_API_KEY"),
    Model:   "gpt-4o",
    BaseURL: "https://api.openai.com/v1", // 可选，默认官方地址
    Timeout: 120 * time.Second,
}
provider, err := llm.NewOpenAIProvider(cfg)
```

## 弹性能力

- **ResilientProvider**：自动重试 + 熔断器 + 多 Provider 降级
- **CachedProvider**：响应缓存（内存 / SQLite）
- **ModelRouter**：按规则路由到不同模型
- **RateLimiter**：令牌桶限流

```go
resilient := llm.NewResilientProvider(primary, llm.WithFallback(secondary))
```
