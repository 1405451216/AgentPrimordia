# A2A 协议

A2A（Agent2Agent）是 AgentPrimordia 实现的 Agent 间协作协议，支持 Agent Card 自动发现、Task 异步执行、gRPC 实时推送和多种认证方式。

> A2A 的公共 API 统一由 `pkg/a2a.go`（`ap` 包）通过类型别名导出；
> `internal/agent/a2a` 为内部实现，**不可直接导入**。自 v1.x 起
> A2A 的默认传输是 gRPC（JSON-RPC over HTTP/SSE 兼容层已在 v4.0.0 移除，
> 开放协议互操作见 `pkg/a2a_interop.go`）。

## 核心概念

| 概念 | 说明 |
|------|------|
| **Agent Card** | Agent 的能力描述（ID、名称、技能、端点、安全方案） |
| **Task** | 一次 Agent 间协作任务，拥有独立状态机 |
| **Message** | Task 中的消息单元 |
| **Artifact** | Task 产出的结果（文本、文件、结构化数据） |
| **gRPC** | 实时订阅任务更新的传输方式 |

## Task 状态机

```
submitted → working → completed
     ↓          ↓
  rejected   input-required
                ↓
            canceled / failed
```

## 发布 Agent Card（服务端）

```go
card := ap.NewA2AAgentCard("agent-1", "ResearchAgent")
card.Description = "搜索并总结信息"
card.Skills = []ap.A2AAgentSkill{{
    ID:          "search",
    Name:        "search",
    Description: "搜索并总结信息",
    InputModes:  []string{"text"},
    OutputModes: []string{"text"},
}}

tm := ap.NewA2ATaskManager()
service := ap.NewA2AService(card, tm)

// NewA2AGRPCServerWithService 返回已注册 A2A 服务的 *grpc.Server
gserver := ap.NewA2AGRPCServerWithService(service)
lis, err := net.Listen("tcp", ":50051")
if err != nil {
    log.Fatal(err)
}
log.Println("A2A gRPC server listening on :50051")
log.Fatal(gserver.Serve(lis))
```

可选：通过 `ap.WithGRPCAuth(...)` 注入认证函数，或使用
`ap.NewA2AAPIKeyAuthenticator(keys, headerName)` /
`ap.NewA2ABearerTokenAuthenticator(validate)` 构造认证器。

## 发送 Task（客户端）

```go
client, err := ap.NewA2AGRPCClient("localhost:50051")
if err != nil {
    log.Fatal(err)
}
defer client.Close()

msg := &ap.A2AMessage{
    Role:  "user",
    Parts: []ap.A2APart{ap.NewA2ATextPart("帮我查一下 Go 1.26 的新特性")},
}
task, err := client.CreateTask(ctx, msg, "task-001")
if err != nil {
    log.Fatal(err)
}

// 查询任务状态
task, err = client.GetTask(ctx, task.ID)
```

## 订阅实时更新

```go
events, err := client.StreamEvents(ctx, task.ID)
if err != nil {
    log.Fatal(err)
}
for event := range events {
    if event.State != nil {
        fmt.Println(event.TaskID, *event.State)
    }
}
```

## 自动发现

```go
// 本地（进程内）服务发现
discovery := ap.NewA2ALocalDiscovery()

// 注册本进程 Agent 的能力描述
if err := discovery.Register(card); err != nil {
    log.Fatal(err)
}

// 解析目标 Agent
registry, err := discovery.Resolve("research-agent")
if err != nil {
    log.Fatal(err)
}

// 远端发现：直接拉取对方的 Agent Card
remoteCard, err := client.FetchAgentCard(ctx)
if err != nil {
    log.Fatal(err)
}
fmt.Println(remoteCard.Name, remoteCard.Endpoints)
```

## 与编排的区别

| 维度 | A2A | Orchestration |
|------|-----|---------------|
| 边界 | 跨进程 / 跨服务 | 进程内 |
| 发现 | Agent Card | 代码显式组合 |
| 通信 | gRPC | 函数调用 |
| 适用 | 异构 Agent 系统 | 同构模块协作 |

## 下一步

- 阅读 [A2A API 参考](../api/a2a.md)
- 查看 [多 Agent 示例](../../ecosystem/examples/multi-agent-collab/main.go)
