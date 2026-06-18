# A2A 协议

A2A（Agent2Agent）是 AgentPrimordia 实现的 Agent 间协作协议，支持 Agent Card 自动发现、Task 异步执行、SSE/gRPC 实时推送和多种认证方式。

## 核心概念

| 概念 | 说明 |
|------|------|
| **Agent Card** | Agent 的能力描述（ID、名称、技能、端点、安全方案） |
| **Task** | 一次 Agent 间协作任务，拥有独立状态机 |
| **Message** | Task 中的消息单元 |
| **Artifact** | Task 产出的结果（文本、文件、结构化数据） |
| **SSE / gRPC** | 实时订阅任务更新的传输方式 |

## Task 状态机

```
submitted → working → completed
     ↓          ↓
  rejected   input-required
                ↓
            canceled / failed
```

## 发布 Agent Card

```go
server := a2a.NewServer(a2a.ServerConfig{
    AgentCard: a2a.NewAgentCard("agent-1", "ResearchAgent").
        WithSkill("search", "搜索并总结信息", []string{"text"}, []string{"text"}),
    AuthType: a2a.AuthBearer,
})

http.ListenAndServe(":8080", server)
```

## 发送 Task

```go
client := a2a.NewClient(a2a.ClientConfig{
    BaseURL: "http://localhost:8080",
    Auth:    a2a.BearerAuth("token"),
})

task, err := client.SendTask(ctx, a2a.TaskRequest{
    ID:      "task-001",
    Message: a2a.Message{Role: "user", Content: "帮我查一下 Go 1.26 的新特性"},
})
```

## 订阅实时更新

```go
stream, err := client.SubscribeTask(ctx, "task-001")
for event := range stream {
    fmt.Println(event.State, event.Message.Content)
}
```

## 自动发现

```go
discovery := a2a.NewDiscovery(a2a.DiscoveryConfig{
    RegistryURL: "http://localhost:9000",
})

card, err := discovery.FindAgent(ctx, "research")
```

## 与编排的区别

| 维度 | A2A | Orchestration |
|------|-----|---------------|
| 边界 | 跨进程 / 跨服务 | 进程内 |
| 发现 | Agent Card | 代码显式组合 |
| 通信 | HTTP / gRPC | 函数调用 |
| 适用 | 异构 Agent 系统 | 同构模块协作 |

## 下一步

- 阅读 [A2A API 参考](../api/a2a.md)
- 查看 [多 Agent 示例](../../ecosystem/examples/multi-agent-collab/main.go)
