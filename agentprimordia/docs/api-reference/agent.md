# Agent API 参考

> `package ap` — Agent 类型与构造函数。

## 类型定义

```go
type Agent struct {
    // 私有字段
}

type AgentConfig struct {
    Name         string         // Agent 名称
    SystemPrompt string         // 系统提示
    MaxTurns     int            // 最大 ReAct 轮次
    Memory       memory.Memory  // 后端记忆
    Tools        []Tool         // 工具列表
    LLM          LLM            // LLM Provider
    Guardrail    Guardrail      // 可选护栏
    TraceEnabled bool           // 是否开启 Trace
    Timeout      time.Duration  // 单次运行超时
}

type Response struct {
    Content  string         // 最终输出内容
    Turn     int            // 实际使用的轮次
    History  []Turn         // 完整 ReAct 历史
    Duration time.Duration  // 运行耗时
    Usage    TokenUsage     // Token 用量
}

type Turn struct {
    Thought     string
    Action      *Action
    Observation string
}

type Action struct {
    Tool string
    Args json.RawMessage
}
```

## 构造函数

```go
func NewAgent(cfg AgentConfig) *Agent
func NewAgentFromYAML(path string) (*Agent, error)
```

## 方法

```go
// Run 同步执行 Agent，返回 Response
func (a *Agent) Run(ctx context.Context, prompt string) (*Response, error)

// StreamRun 流式执行，通过 channel 逐步返回 token
func (a *Agent) StreamRun(ctx context.Context, prompt string) (<-chan StreamChunk, error)

// Stop 停止当前运行
func (a *Agent) Stop()

// Reset 重置 Agent 状态（清空历史轮次，保留 Memory）
func (a *Agent) Reset()
```

## 示例

```go
agent := ap.NewAgent(ap.AgentConfig{
    Name:         "my-agent",
    SystemPrompt: "你是助手",
    MaxTurns:     10,
    Memory:       mem,
    Tools:        []ap.Tool{webSearchTool, fileTool},
})

resp, err := agent.Run(ctx, "今天北京天气？")
fmt.Println(resp.Content)
```

## 错误处理

| 错误类型 | 说明 |
|----------|------|
| `ErrMaxTurnsExceeded` | 超过最大轮次 |
| `ErrTimeout` | 运行超时 |
| `ErrToolExecution` | 工具执行失败 |
| `ErrGuardrailBlocked` | 护栏拦截 |
