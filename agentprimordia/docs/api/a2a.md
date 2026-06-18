# A2A API

## Server

```go
func NewServer(config ServerConfig) *Server
```

创建 A2A 服务端。

### ServerConfig

```go
type ServerConfig struct {
    AgentCard *AgentCard
    AuthType  AuthType
    TaskStore TaskStore
}
```

### 主要方法

| 方法 | 说明 |
|------|------|
| `ServeHTTP(w, r)` | HTTP 处理入口 |
| `SendTask(ctx, req)` | 处理发送任务 |
| `GetTask(ctx, id)` | 获取任务 |
| `CancelTask(ctx, id)` | 取消任务 |
| `SubscribeTask(ctx, id)` | SSE 订阅 |

## Client

```go
func NewClient(config ClientConfig) *Client
```

### 主要方法

| 方法 | 说明 |
|------|------|
| `SendTask(ctx, req)` | 发送任务 |
| `GetTask(ctx, id)` | 获取任务 |
| `CancelTask(ctx, id)` | 取消任务 |
| `SubscribeTask(ctx, id)` | 订阅任务更新 |

## 核心类型

| 类型 | 说明 |
|------|------|
| `AgentCard` | Agent 能力卡片 |
| `Task` | 任务对象 |
| `TaskState` | 任务状态 |
| `Message` | 消息 |
| `Artifact` | 产物 |
| `SecurityScheme` | 安全方案 |
