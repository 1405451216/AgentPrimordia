# Agent API 参考

> `package agent` — Agent 类型与构造函数。

## 类型定义

### CapabilityAgent

```go
type CapabilityAgent struct {
    // 私有字段
}
```

`CapabilityAgent` 是 Agent 的推荐实现（v0.7.0 起），通过 Functional Options 注入能力。

### AgentConfig

`AgentConfig` 为分组结构（能力配置各自成组），日常使用推荐直接用 Functional Options：

```go
type AgentConfig struct {
    // 核心配置（必填）
    Name           string
    SystemPrompt   string
    Model          llm.Provider
    PromptTemplate *PromptTemplate

    // 标量配置
    MaxTurns    int     // 默认 50
    Temperature float64 // 默认 0.0
    SessionID   string

    // 能力分组
    Memory        MemoryConfig
    RAG           RAGConfig
    Observability ObservabilityConfig
    Resilience    ResilienceConfig
    Tools         ToolsConfig
    Learning      LearningConfig
    Cognition     CognitionConfig
    Autonomy      AutonomyConfig
    Skills        SkillsConfig
    Realtime      RealtimeConfig

    // 运行时辅助
    Lifecycle *Lifecycle
    Logger    *slog.Logger
}
```

### Message

```go
type Message struct {
    Role         Role                     `json:"role"`           // system / user / assistant / tool
    Content      string                   `json:"content"`
    ContentParts []multimodal.ContentPart `json:"content_parts,omitempty"`  // 多模态内容
    ToolCalls    []ToolCall               `json:"tool_calls,omitempty"`
    Metadata     Metadata                 `json:"metadata,omitempty"`
}
```

### Response

```go
type Response struct {
    RequestID string     `json:"request_id"`
    Content   string     `json:"content"`
    ToolCalls []ToolCall `json:"tool_calls,omitempty"`
    Usage     Usage      `json:"usage"`
    Metrics   Metrics    `json:"metrics"`
    Error     error      `json:"-"`
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

## 构造函数

```go
// NewAgent 是创建 Agent 的推荐入口（v0.7.0 起）
// 只暴露核心字段，能力通过 Functional Options 注入
func NewAgent(name, systemPrompt string, model llm.Provider, opts ...Option) (*CapabilityAgent, error)
```

**常用 Option 函数：**

| Option | 说明 |
|--------|------|
| `WithMaxTurns(n int)` | 设置最大 ReAct 轮次 |
| `WithTemperature(t float64)` | 设置温度参数 |
| `WithMemory(mem memory.Memory)` | 注入记忆后端 |
| `WithToolkit(registry *tools.Registry)` | 注入工具注册表 |
| `WithPromptTemplate(tmpl PromptTemplate)` | 自定义提示模板 |
| `WithCache(cache llm.LLMCache)` | 注入 LLM 缓存 |
| `WithHooks(hooks *HookManager)` | 注入 Hook 管理器 |
| `WithInputGuard(engine *guardrail.Engine)` | 注入输入端护栏（用户输入进入循环前检查，v3.4-4） |

## 方法

```go
// Run 同步执行 Agent，接收 Message 输入并返回 Response
func (c *CapabilityAgent) Run(ctx context.Context, input Message) (*Response, error)

// StreamRun 流式执行，返回 StreamEvent 只读通道（channel 在流结束后由 Provider 关闭）
func (c *CapabilityAgent) StreamRun(ctx context.Context, input Message) (<-chan StreamEvent, error)

// Stop 停止当前运行
func (c *CapabilityAgent) Stop()
```

## 示例

```go
agent, err := ap.NewAgent("my-agent", "你是助手", provider,
    ap.WithMaxTurns(10),
    ap.WithMemory(mem),
    ap.WithToolkit(registry),
)
if err != nil {
    log.Fatal(err)
}

resp, err := agent.Run(ctx, ap.UserMessage("今天北京天气？"))
fmt.Println(resp.Content)
```

## 错误处理

| 错误类型 | 说明 |
|----------|------|
| `ErrMaxTurnsExceeded` | 超过最大轮次 |

```go
resp, err := agent.Run(ctx, ap.UserMessage("..."))
if errors.Is(err, agent.ErrMaxTurnsExceeded) {
    fmt.Println("Agent 达到最大轮次限制")
}
```
