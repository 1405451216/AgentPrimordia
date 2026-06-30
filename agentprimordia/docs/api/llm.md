# LLM API

LLM（大语言模型）API 参考文档。

## Provider 接口

```go
// Provider 是 LLM Provider 的统一接口。
// 实现者需提供同步补全（Complete）、流式补全（Stream）和工具调用（CallTools）能力。
type Provider interface {
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error)
    CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error)
    Info() ModelInfo
}
```

## Embedder 接口

```go
// Embedder 嵌入接口，用于需要嵌入功能的场景
// 不支持 Embeddings 的 Provider 可不实现此接口，调用方通过类型断言检查
type Embedder interface {
    Embeddings(ctx context.Context, texts []string) ([][]float32, error)
}
```

## Config

统一的 Provider 配置结构：

```go
type Config struct {
    APIKey      string         `json:"-"`
    BaseURL     string         `json:"base_url,omitempty"`
    Model       string         `json:"model"`
    Temperature float64        `json:"temperature,omitempty"`
    MaxTokens   int            `json:"max_tokens,omitempty"`
    Extra       map[string]any `json:"extra,omitempty"`
}
```

## 核心类型

### CompletionRequest

```go
type CompletionRequest struct {
    Messages       []ChatMessage   `json:"messages"`
    Model          string          `json:"model,omitempty"`
    Temperature    *float64        `json:"temperature,omitempty"`  // 指针类型，区分"未设置"和 0
    MaxTokens      int             `json:"max_tokens,omitempty"`
    Stream         bool            `json:"stream,omitempty"`
    ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}
```

### ChatMessage

```go
type ChatMessage struct {
    Role        string         `json:"role"`           // system / user / assistant / tool
    Content     string         `json:"content"`
    ToolCalls   []FunctionCall `json:"tool_calls,omitempty"`
    ToolCallID  string         `json:"tool_call_id,omitempty"`
    IsToolError bool           `json:"is_error,omitempty"`
}
```

### CompletionResponse

```go
type CompletionResponse struct {
    ID      string `json:"id"`
    Model   string `json:"model"`
    Content string `json:"content"`
    Role    string `json:"role"`
    Usage   Usage  `json:"usage"`
}
```

### Chunk（流式）

```go
type Chunk struct {
    Content string `json:"content"`
    Done    bool   `json:"done"`
    Usage   *Usage `json:"usage,omitempty"`
}
```

### Usage

```go
type Usage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}
```

### ModelInfo

```go
type ModelInfo struct {
    Name              string `json:"name"`
    Provider          string `json:"provider"`
    MaxContext        int    `json:"max_context"`
    SupportsTools     bool   `json:"supports_tools"`
    SupportsStreaming bool   `json:"supports_streaming"`
}
```

### 工具调用类型

```go
type ToolCallRequest struct {
    Messages []ChatMessage    `json:"messages"`
    Tools    []ToolDefinition `json:"tools"`
    Model    string           `json:"model,omitempty"`
}

type ToolDefinition struct {
    Type     string             `json:"type"`  // 固定 "function"
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
    Arguments string `json:"arguments"`  // JSON-encoded
}
```

## Provider 实现

### OpenAIProvider

```go
func NewOpenAIProvider(config Config) *OpenAIProvider
```

**示例：**

=== "Go"

    ```go
    provider := ap.NewOpenAIProvider(ap.Config{
        APIKey: os.Getenv("OPENAI_API_KEY"),
        Model:  "gpt-4o",
    })
    ```

=== "TypeScript"

    ```typescript
    const provider = new OpenAIProvider({
      apiKey: process.env.OPENAI_API_KEY!,
      model: 'gpt-4o',
    });
    ```

### AnthropicProvider

```go
func NewAnthropicProvider(config Config) *AnthropicProvider
```

**示例：**

```go
provider := ap.NewAnthropicProvider(ap.Config{
    APIKey: os.Getenv("ANTHROPIC_API_KEY"),
    Model:  "claude-haiku-4-5-20251001",
})
```

### GeminiProvider

```go
func NewGeminiProvider(config Config) *GeminiProvider
```

### OllamaProvider

```go
func NewOllamaProvider(config Config) *OllamaProvider
```

**示例：**

```go
provider := ap.NewOllamaProvider(ap.Config{
    BaseURL: "http://localhost:11434",
    Model:   "llama3",
})
```

### AzureOpenAIProvider

```go
func NewAzureOpenAIProvider(config Config) *AzureOpenAIProvider
```

### QwenProvider

```go
func NewQwenProvider(config Config) *QwenProvider
```

### GLMProvider

```go
func NewGLMProvider(config Config) *GLMProvider
```

### MistralProvider

```go
func NewMistralProvider(config Config) *MistralProvider
```

### CohereProvider

```go
func NewCohereProvider(config Config) *CohereProvider
```

> **注意：** DeepSeek 已合并到 OpenAI 兼容模式，使用 `NewOpenAIProvider` 并设置 `BaseURL` 即可：
> ```go
> provider := ap.NewOpenAIProvider(ap.Config{
>     BaseURL: "https://api.deepseek.com/v1",
>     APIKey:  os.Getenv("DEEPSEEK_API_KEY"),
>     Model:   "deepseek-chat",
> })
> ```

## ResilientProvider

带重试、熔断和降级的弹性 Provider 包装器：

```go
func NewResilientProvider(primary Provider, config ResilientConfig) *ResilientProvider
```

**ResilientConfig 结构：**

```go
type ResilientConfig struct {
    MaxRetries       int           // 最大重试次数（默认 3）
    RetryDelay       time.Duration // 重试延迟
    CircuitBreaker   bool          // 是否启用熔断器
    CBConfig         CircuitBreakerConfig
}

type CircuitBreakerConfig struct {
    MaxFailures int           // 最大失败次数（触发熔断）
    Timeout     time.Duration // 熔断恢复超时
    HalfOpenMax int           // 半开状态最大请求数
}
```

**功能：**
- **重试** — 指数退避 + 随机抖动
- **熔断** — 失败次数超阈值后熔断，定时恢复（closed / open / halfOpen 三态）
- **降级** — 主 Provider 失败后自动切换 Fallback

**示例：**

```go
primary := ap.NewOpenAIProvider(ap.Config{APIKey: key, Model: "gpt-4o"})
fallback := ap.NewGeminiProvider(ap.Config{APIKey: geminiKey, Model: "gemini-1.5-pro"})

resilient := ap.NewResilientProvider(primary, ap.DefaultResilientConfig())
resilient.AddFallback(fallback)
```

## LLMCache

```go
type LLMCache interface {
    Get(key string) (*CompletionResponse, bool)
    Set(key string, resp *CompletionResponse, ttl time.Duration)
    Stats() CacheStats
    Clear()
}
```

通过 `WithCache()` 注入 Agent：

```go
agent := ap.NewReActAgent(cfg).WithCache(llm.NewMemoryCache())
```

## 结构化输出

```go
type StructuredExtractor interface {
    Extract(ctx context.Context, text string) (any, error)
}
```

支持从 Go struct 生成 JSON Schema，预定义模板：情感分析、NER、分类、摘要。

## 完整示例

=== "Go"

    ```go
    package main

    import (
        "context"
        "fmt"
        "os"

        ap "agentprimordia/pkg"
    )

    func main() {
        provider := ap.NewOpenAIProvider(ap.Config{
            APIKey: os.Getenv("OPENAI_API_KEY"),
            Model:  "gpt-4o",
        })

        // 使用 ResilientProvider
        resilient := ap.NewResilientProvider(provider, ap.DefaultResilientConfig())

        // 创建 Agent
        agent, _ := ap.NewAgent("demo", "你是助手", resilient,
            ap.WithMaxTurns(5),
        )

        resp, err := agent.Run(context.Background(), ap.UserMessage("你好！"))
        if err != nil {
            fmt.Fatal(err)
        }

        fmt.Printf("回复: %s\n", resp.Content)
        fmt.Printf("Token: %d\n", resp.Usage.TotalTokens)
    }
    ```

=== "TypeScript"

    ```typescript
    import { ReActAgent, OpenAIProvider, ResilientProvider } from '@agentprimordia/sdk';

    const provider = new OpenAIProvider({
      apiKey: process.env.OPENAI_API_KEY!,
      model: 'gpt-4o',
    });

    const resilient = new ResilientProvider(provider, {
      maxRetries: 3,
    });

    const agent = new ReActAgent({
      name: 'demo',
      systemPrompt: '你是助手',
      model: resilient,
      maxTurns: 5,
    });

    const resp = await agent.run('你好！');
    console.log(`回复: ${resp.content}`);
    console.log(`Token: ${resp.usage.totalTokens}`);
    ```
