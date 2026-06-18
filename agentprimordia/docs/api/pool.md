# Pool API

## Pool

```go
func NewPool(config PoolConfig) *Pool
```

创建多 Agent 调度池。

### PoolConfig

```go
type PoolConfig struct {
    MaxConcurrency   int
    MaxTurns         int
    MaxRetainedTasks int
    Timeout          time.Duration
    Model            llm.Provider
    Toolkit          *tools.Registry
}
```

### 主要方法

| 方法 | 说明 |
|------|------|
| `Dispatch(ctx, config)` | 分发任务 |
| `WaitForTask(ctx, taskID)` | 等待任务结果 |
| `GetTask(taskID)` | 获取任务状态 |
| `GetTasksBySession(sessionID)` | 按会话查询任务 |
| `CancelBySession(sessionID)` | 取消会话下所有任务 |
| `Subscribe()` | 订阅 Pool 事件 |
| `SetAgentFactory(factory)` | 设置 Agent 工厂 |
| `Close()` | 关闭 Pool |

## AgentFactory

```go
type AgentFactory interface {
    Create(config AgentConfig) (Agent, error)
    Register(agentType string, creator AgentCreator)
}
```

## TaskConfig

```go
type TaskConfig struct {
    AgentType string
    SessionID string
    Input     string
    MaxTurns  int
    Timeout   time.Duration
}
```

## TaskResult

```go
type TaskResult struct {
    TaskID    string
    SessionID string
    Output    string
    Error     string
    Duration  time.Duration
}
```
