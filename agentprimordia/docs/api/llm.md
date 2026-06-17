# LLM API

LLM（大语言模型）API 参考文档。

## Provider 接口

```go
type Provider interface {
    // Complete 生成补全
    Complete(ctx context.Context, req Request) (Response, error)
    
    // Name 返回 Provider 名称
    Name() string
    
    // Close 关闭 Provider
    Close() error
}
```

## Request 结构

```go
type Request struct {
    // Messages 消息列表
    Messages []Message
    
    // MaxTokens 最大生成 token 数
    MaxTokens int
    
    // Temperature 温度参数（0-2）
    Temperature float64
    
    // TopP 核采样参数
    TopP float64
    
    // Stop 停止序列
    Stop []string
    
    // Tools 可用工具列表
    Tools []ToolDefinition
    
    // Stream 是否流式输出
    Stream bool
}
```

## Response 结构

```go
type Response struct {
    // Content 生成的内容
    Content string
    
    // ToolCalls 工具调用列表
    ToolCalls []ToolCall
    
    // Usage token 使用情况
    Usage Usage
    
    // FinishReason 结束原因
    FinishReason string
}
```

## Message 结构

```go
type Message struct {
    // Role 角色: system, user, assistant, tool
    Role string
    
    // Content 内容
    Content string
    
    // Name 名称（可选）
    Name string
    
    // ToolCalls 工具调用（assistant 消息）
    ToolCalls []ToolCall
    
    // ToolCallID 工具调用 ID（tool 消息）
    ToolCallID string
}
```

## ToolCall 结构

```go
type ToolCall struct {
    // ID 调用 ID
    ID string
    
    // Name 工具名称
    Name string
    
    // Arguments 参数（JSON 字符串）
    Arguments string
}
```

## Usage 结构

```go
type Usage struct {
    // PromptTokens 提示 token 数
    PromptTokens int
    
    // CompletionTokens 生成 token 数
    CompletionTokens int
    
    // TotalTokens 总 token 数
    TotalTokens int
}
```

## OpenAI Provider

### NewOpenAIProvider

```go
func NewOpenAIProvider(config OpenAIConfig) (Provider, error)
```

**OpenAIConfig 结构：**
```go
type OpenAIConfig struct {
    // APIKey API 密钥
    APIKey string
    
    // BaseURL 自定义端点（可选）
    BaseURL string
    
    // Model 模型名称
    Model string
    
    // MaxTokens 最大 token 数
    MaxTokens int
    
    // Temperature 温度
    Temperature float64
    
    // Organization 组织 ID（可选）
    Organization string
}
```

**示例：**
```go
provider, err := llm.NewOpenAIProvider(llm.OpenAIConfig{
    APIKey:      os.Getenv("OPENAI_API_KEY"),
    Model:       "gpt-4",
    MaxTokens:   4096,
    Temperature: 0.7,
})
```

## Anthropic Provider

### NewAnthropicProvider

```go
func NewAnthropicProvider(config AnthropicConfig) (Provider, error)
```

**AnthropicConfig 结构：**
```go
type AnthropicConfig struct {
    // APIKey API 密钥
    APIKey string
    
    // Model 模型名称
    Model string
    
    // MaxTokens 最大 token 数
    MaxTokens int
    
    // Temperature 温度
    Temperature float64
}
```

**示例：**
```go
provider, err := llm.NewAnthropicProvider(llm.AnthropicConfig{
    APIKey: os.Getenv("ANTHROPIC_API_KEY"),
    Model:  "claude-3-opus-20240229",
})
```

## ResilientProvider

带重试、熔断和降级的 Provider：

### NewResilientProvider

```go
func NewResilientProvider(base Provider, config ResilientConfig) Provider
```

**ResilientConfig 结构：**
```go
type ResilientConfig struct {
    // MaxRetries 最大重试次数
    MaxRetries int
    
    // RetryDelay 重试延迟
    RetryDelay time.Duration
    
    // CircuitBreaker 是否启用熔断器
    CircuitBreaker bool
    
    // CBConfig 熔断器配置
    CBConfig CircuitBreakerConfig
    
    // FallbackProvider 降级 Provider
    FallbackProvider Provider
}

type CircuitBreakerConfig struct {
    // MaxFailures 最大失败次数
    MaxFailures int
    
    // Timeout 熔断超时
    Timeout time.Duration
    
    // HalfOpenMax 半开状态最大请求数
    HalfOpenMax int
}
```

**示例：**
```go
resilient := llm.NewResilientProvider(baseProvider, llm.ResilientConfig{
    MaxRetries:     3,
    RetryDelay:     time.Second,
    CircuitBreaker: true,
    CBConfig: llm.CircuitBreakerConfig{
        MaxFailures: 5,
        Timeout:     60 * time.Second,
    },
    FallbackProvider: fallbackLLM,
})
```

## DemoLLM

演示用 LLM（无需 API Key）：

### NewDemoLLM

```go
func NewDemoLLM() Provider
```

**示例：**
```go
demoLLM := llm.NewDemoLLM()
```

## LoadBalancer

多 Provider 负载均衡：

### NewLoadBalancer

```go
func NewLoadBalancer(providers []Provider, config LBConfig) Provider
```

**LBConfig 结构：**
```go
type LBConfig struct {
    // Strategy 负载均衡策略
    Strategy LBStrategy
    
    // HealthCheck 是否启用健康检查
    HealthCheck bool
    
    // HealthCheckInterval 健康检查间隔
    HealthCheckInterval time.Duration
}

type LBStrategy int
const (
    RoundRobin LBStrategy = iota
    Random
    LeastConnections
    WeightedRoundRobin
)
```

**示例：**
```go
providers := []Provider{openAI, anthropic, local}
lb := llm.NewLoadBalancer(providers, llm.LBConfig{
    Strategy:    llm.RoundRobin,
    HealthCheck: true,
})
```

## Embedding Provider

嵌入模型 Provider：

### Embedder 接口

```go
type Embedder interface {
    // Embed 生成嵌入向量
    Embed(ctx context.Context, text string) ([]float32, error)
    
    // EmbedBatch 批量生成嵌入
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
    
    // Dimensions 返回向量维度
    Dimensions() int
    
    // Name 返回 Provider 名称
    Name() string
    
    // Close 关闭 Provider
    Close() error
}
```

### NewOpenAIEmbedder

```go
func NewOpenAIEmbedder(config OpenAIEmbedConfig) (Embedder, error)
```

**OpenAIEmbedConfig 结构：**
```go
type OpenAIEmbedConfig struct {
    // APIKey API 密钥
    APIKey string
    
    // Model 模型名称
    Model string  // 例如: text-embedding-3-small
    
    // Dimensions 向量维度（可选）
    Dimensions int
}
```

**示例：**
```go
embedder, err := llm.NewOpenAIEmbedder(llm.OpenAIEmbedConfig{
    APIKey:     os.Getenv("OPENAI_API_KEY"),
    Model:      "text-embedding-3-small",
    Dimensions: 1536,
})
```

## 错误定义

```go
var (
    // ErrProviderUnavailable Provider 不可用
    ErrProviderUnavailable = errors.New("provider unavailable")
    
    // ErrRateLimited 触发速率限制
    ErrRateLimited = errors.New("rate limited")
    
    // ErrInvalidRequest 请求无效
    ErrInvalidRequest = errors.New("invalid request")
    
    // ErrAuthenticationFailed 认证失败
    ErrAuthenticationFailed = errors.New("authentication failed")
    
    // ErrModelNotFound 模型未找到
    ErrModelNotFound = errors.New("model not found")
    
    // ErrContextLengthExceeded 上下文长度超限
    ErrContextLengthExceeded = errors.New("context length exceeded")
)
```

## 完整示例

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "time"
    
    "agentprimordia.dev/agentprimordia/pkg/llm"
)

func main() {
    // 创建 Provider
    provider, err := llm.NewOpenAIProvider(llm.OpenAIConfig{
        APIKey:      os.Getenv("OPENAI_API_KEY"),
        Model:       "gpt-4",
        MaxTokens:   4096,
        Temperature: 0.7,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer provider.Close()
    
    // 创建 ResilientProvider
    resilient := llm.NewResilientProvider(provider, llm.ResilientConfig{
        MaxRetries: 3,
        RetryDelay: time.Second,
    })
    
    // 构建请求
    req := llm.Request{
        Messages: []llm.Message{
            {Role: "system", Content: "你是一个有帮助的助手。"},
            {Role: "user", Content: "你好！"},
        },
        MaxTokens:   100,
        Temperature: 0.7,
    }
    
    // 调用
    resp, err := resilient.Complete(context.Background(), req)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("回复: %s\n", resp.Content)
    fmt.Printf("Token 使用: %d\n", resp.Usage.TotalTokens)
}
```
