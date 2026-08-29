# A2A API

Agent-to-Agent 通信协议 API 参考文档。

## Server

```go
func NewA2AServer(tm *TaskManager, opts ...ServerOption) *A2AServer
```

创建 A2A 服务端，支持 HTTP JSON-RPC 和 gRPC 双协议。

### ServerOption

| 选项 | 说明 |
|------|------|
| `WithCard(card *AgentCard)` | 设置 Agent 能力卡片 |
| `WithTaskHandler(handler TaskHandler)` | 设置任务处理器 |
| `WithAuth(auth Authenticator)` | 设置认证器 |

### 主要方法

| 方法 | 说明 |
|------|------|
| `Handler() http.Handler` | HTTP 处理入口 |
| `ServeHTTP(w, r)` | HTTP 请求处理 |
| `SendTask(ctx, req)` | 处理发送任务 |
| `GetTask(ctx, id)` | 获取任务 |
| `CancelTask(ctx, id)` | 取消任务 |
| `SubscribeTask(ctx, id)` | SSE 订阅 |

### AgentCard

```go
func NewAgentCard(agentID, name string) *AgentCard

type AgentCard struct {
    Protocol        string            `json:"protocol"`
    AgentID         string            `json:"agent_id"`
    Name            string            `json:"name"`
    Description     string            `json:"description,omitempty"`
    Capabilities    AgentCapabilities `json:"capabilities"`
    Endpoints       AgentEndpoints    `json:"endpoints"`
    SecuritySchemes []SecurityScheme  `json:"security_schemes"`
    Skills          []AgentSkill      `json:"skills,omitempty"`
    Metadata        map[string]string `json:"metadata,omitempty"`
}

type AgentSkill struct {
    ID          string   `json:"id"`
    Name        string   `json:"name"`
    Description string   `json:"description,omitempty"`
    InputModes  []string `json:"input_modes,omitempty"`
    OutputModes []string `json:"output_modes,omitempty"`
}
```

**示例：**

=== "Go"

    ```go
    tm := a2a.NewTaskManager()
    defer tm.Cleanup()

    card := a2a.NewAgentCard("agent-1", "ResearchAgent")
    card.Description = "研究助手"
    card.Skills = []a2a.AgentSkill{
        {ID: "search", Name: "搜索", Description: "搜索并总结信息"},
    }

    server := a2a.NewA2AServer(tm,
        a2a.WithCard(card),
        a2a.WithTaskHandler(&myHandler{}),
        a2a.WithAuth(a2a.NewBearerTokenAuthenticator(tokenFunc)),
    )

    http.ListenAndServe(":8080", server.Handler())
    ```

=== "TypeScript"

    ```typescript
    import { A2AServer } from '@agentprimordia/sdk';

    const server = new A2AServer({
      agentCard: {
        agentId: 'agent-1',
        name: 'ResearchAgent',
        skills: [{ id: 'search', name: '搜索' }],
      },
      taskHandler: myHandler,
    });

    server.listen(8080);
    ```

## Client

```go
func NewA2AClient(baseURL string, opts ...ClientOption) *A2AClient
```

### ClientOption

| 选项 | 说明 |
|------|------|
| `WithAPIKey(key string)` | API Key 认证 |
| `WithBearerToken(token string)` | Bearer Token 认证 |
| `WithHTTPClient(c *http.Client)` | 自定义 HTTP 客户端 |

### 主要方法

| 方法 | 说明 |
|------|------|
| `FetchAgentCard() (*AgentCard, error)` | 获取远端 Agent 卡片 |
| `CreateTask(message *A2AMessage, taskID string) (*Task, error)` | 创建任务 |
| `GetTask(ctx, taskID string) (*Task, error)` | 获取任务状态 |
| `CancelTask(ctx, taskID string) (*Task, error)` | 取消任务 |
| `SubscribeTask(ctx, taskID string) (<-chan StreamEvent, error)` | SSE 订阅任务更新 |

**示例：**

```go
client := a2a.NewA2AClient("http://localhost:8080",
    a2a.WithBearerToken("my-token"),
)

// 获取 Agent Card
card, err := client.FetchAgentCard()

// 发送任务
task, err := client.CreateTask(&a2a.A2AMessage{
    Role:  "user",
    Parts: []a2a.Part{a2a.NewTextPart("帮我分析数据")},
}, "")

// 订阅更新
stream, err := client.SubscribeTask(ctx, task.ID)
for event := range stream {
    fmt.Println(event.State, event.Message.Content)
}
```

## gRPC 支持

v0.8.0 新增 gRPC 协议支持（基于 protobuf）：

### gRPC Server

```go
func NewA2AGRPCServerWithService(service *A2AService, opts ...GRPCServerOption) *A2AGRPCServer

// 启动
grpcServer := ap.NewA2AGRPCServerWithService(service,
    ap.WithGRPCAuth(auth),
)
grpcServer.Start(":8083")
```

### gRPC Client

```go
func NewA2AGRPCClient(target string, opts ...GRPCClientOption) (*A2AGRPCClient, error)

client, _ := ap.NewA2AGRPCClient("localhost:8083",
    ap.WithGRPCClientBearerToken("token"),
)

card, _ := client.FetchAgentCard(ctx)
task, _ := client.CreateTask(ctx, message, "")

// 流式订阅
ch, _ := client.StreamEvents(ctx, task.ID)
for ev := range ch {
    fmt.Printf("event: %+v\n", ev)
}
```

## Task 状态机

```
submitted → working → completed
     ↓          ↓
  rejected   input-required
                ↓
            canceled / failed
```

终态（不可变更）：`completed`、`failed`、`canceled`、`rejected`

## 核心类型

| 类型 | 说明 |
|------|------|
| `AgentCard` | Agent 能力卡片（ID、名称、技能、端点、安全方案） |
| `Task` | 任务对象（ID、状态、消息、产物） |
| `TaskState` | 任务状态（submitted / working / completed / failed / canceled / rejected / input-required） |
| `A2AMessage` | 消息（角色 + Parts 多模态内容） |
| `Part` | 消息内容片段（文本 / 文件 / 数据） |
| `AgentSkill` | Agent 技能描述 |
| `SecurityScheme` | 安全方案（API Key / Bearer） |
| `Authenticator` | 认证器接口 |

## 认证

```go
// API Key 认证
auth := a2a.NewAPIKeyAuthenticator(map[string]string{
    "client-a": "key-aaa",
}, "X-API-Key")

// Bearer Token 认证
auth := a2a.NewBearerTokenAuthenticator(func(token string) (*a2a.Principal, error) {
    if token == "valid-token" {
        return &a2a.Principal{ID: "user-1"}, nil
    }
    return nil, fmt.Errorf("invalid token")
})

// 无认证（开发环境）
auth := a2a.NewNoopAuthenticator()
```

## 与编排的区别

| 维度 | A2A | Orchestration |
|------|-----|---------------|
| 边界 | 跨进程 / 跨服务 | 进程内 |
| 发现 | Agent Card | 代码显式组合 |
| 通信 | HTTP / gRPC | 函数调用 |
| 适用 | 异构 Agent 系统 | 同构模块协作 |
