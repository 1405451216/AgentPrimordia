# 流式输出（Streaming）

AgentPrimordia 支持 Agent 的流式输出，让客户端实时接收推理过程和工具调用事件。

## StreamRun API

```go
ch, err := agent.StreamRun(ctx, ap.Message{
    Role:    ap.RoleUser,
    Content: "帮我写一个排序算法",
})
if err != nil {
    log.Fatal(err)
}

for event := range ch {
    switch event.Type {
    case ap.StreamEventThought:
        // LLM 推理文本（逐 token 输出）
        fmt.Print(event.Content)
    case ap.StreamEventToolCall:
        // 工具调用开始
        fmt.Printf("\n[调用工具: %s(%s)]\n", event.ToolName, event.Args)
    case ap.StreamEventToolResult:
        // 工具执行结果
        fmt.Printf("[工具结果: %s]\n", event.Content)
    case ap.StreamEventDone:
        // 执行完成
        fmt.Println("\n[完成]")
    case ap.StreamEventError:
        // 错误
        log.Printf("错误: %s", event.Content)
    }
}
```

## StreamEvent 类型

| 事件类型 | 说明 |
|----------|------|
| `thought` | LLM 推理文本片段 |
| `tool_call` | 工具调用请求 |
| `tool_result` | 工具执行结果 |
| `done` | 执行完成，附带最终响应 |
| `error` | 执行错误 |

## 与 LLM 流式的关系

`StreamRun` 内部调用 LLM Provider 的 `StreamComplete` 方法，将 token 级别的流式数据聚合为语义级别的 StreamEvent。如果 Provider 不支持流式，会自动降级为单次调用后一次性输出。

## 注意事项

- channel 在 Agent 完成或 context 取消时自动关闭
- 并发安全：多个 goroutine 可同时消费同一个 channel
- 背压：channel 有缓冲区，消费过慢时 Agent 会阻塞等待
