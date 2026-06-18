# 流式输出

AgentPrimordia 支持流式输出，让 Agent 的回答可以逐 token 返回给调用方。

## 快速开始

```go
agent := NewReActAgent(cfg)

stream, err := agent.StreamRun(ctx, UserMessage("讲一个关于 AI 的故事"))
if err != nil {
    panic(err)
}

for event := range stream {
    switch event.Type {
    case StreamEventTypeToken:
        fmt.Print(event.Content)
    case StreamEventTypeToolStart:
        fmt.Printf("\n[调用工具: %s]\n", event.ToolName)
    case StreamEventTypeToolEnd:
        fmt.Printf("\n[工具结果: %s]\n", event.Content)
    case StreamEventTypeFinal:
        fmt.Printf("\n[最终答案: %s]\n", event.Content)
    }
}
```

## StreamEvent

```go
type StreamEvent struct {
    Type      StreamEventType
    Content   string
    ToolName  string
    Error     error
}
```

## 事件类型

| 类型 | 说明 |
|------|------|
| `StreamEventTypeToken` | LLM 生成的 token |
| `StreamEventTypeToolStart` | 开始调用工具 |
| `StreamEventTypeToolEnd` | 工具调用结束 |
| `StreamEventTypeFinal` | 最终答案 |
| `StreamEventTypeError` | 发生错误 |

## 取消流式请求

```go
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()

stream, _ := agent.StreamRun(ctx, input)
```

## 下一步

- 了解 [ReAct 循环](../concepts/react-loop.md)
- 查看 [API 参考](../api/agent.md)
