# 使用 Pool 调度多 Agent

Pool 是 AgentPrimordia 的多 Agent 调度器，负责任务队列、并发控制、超时、重试和事件通知。

## 创建 Pool

```go
pool := NewPool(PoolConfig{
    MaxConcurrency:   16,            // 最大并发数
    MaxTurns:         50,            // 单个任务最大轮数
    MaxRetainedTasks: 1000,          // 保留已完成任务上限（防内存泄漏）
    Timeout:          5 * time.Minute,
    Model:            provider,      // 默认模型
    Toolkit:          toolkit,       // 默认工具集
})
```

## 注册 Agent

```go
factory := NewDefaultAgentFactory()
factory.Register("researcher", func(cfg AgentConfig) (Agent, error) {
    return NewReActAgent(ReActConfig{
        Name:         "researcher",
        SystemPrompt: "你是研究专家",
        Model:        cfg.Model,
        MaxTurns:     cfg.MaxTurns,
    }).WithToolkit(cfg.Toolkit), nil
})

pool.SetAgentFactory(factory)
```

## 分发任务

```go
task, err := pool.Dispatch(ctx, TaskConfig{
    AgentType: "researcher",
    SessionID: "session-001",
    Input:     "研究 Go 1.26 新特性",
})
if err != nil {
    panic(err)
}

// 等待结果
result := pool.WaitForTask(ctx, task.ID)
fmt.Println(result.Output)
```

## 按会话查询与取消

```go
// 查询会话下所有任务
tasks := pool.GetTasksBySession("session-001")

// 取消会话下所有任务
_ = pool.CancelBySession("session-001")
```

## 订阅事件

```go
sub := pool.Subscribe()
for event := range sub.Ch {
    fmt.Println(event.Type, event.TaskID, event.Status)
}
```

## 与编排对比

| 特性 | Pool | Orchestration |
|------|------|---------------|
| 关注点 | 任务调度与会话管理 | 任务执行流程 |
| 动态创建 | 通过 AgentFactory | 预定义 Agent |
| 适用 | 在线服务、多会话 | 批处理、工作流 |

## 最佳实践

1. 设置 `MaxRetainedTasks` 防止长期运行内存泄漏
2. 根据 CPU 核数调整 `MaxConcurrency`
3. 为不同 Agent 类型配置合理的 `MaxTurns`
4. 通过事件总线解耦监控逻辑

## 下一步

- 阅读 [Pool API 参考](../api/pool.md)
- 查看 [多 Agent 示例](../../ecosystem/examples/multi-agent/main.go)
