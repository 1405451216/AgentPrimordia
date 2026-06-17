# ReAct 循环

ReAct（Reasoning + Acting）是 AgentPrimordia 的核心执行引擎，实现了推理与行动的交替循环。

## 工作原理

ReAct 循环由三个阶段组成：

```
┌─────────────────────────────────────────┐
│  1. Think（推理）                        │
│     - LLM 分析当前状态                  │
│     - 决定下一步行动                    │
│     - 输出思考过程或动作指令            │
└─────────────┬───────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────┐
│  2. Act（行动）                          │
│     - 执行工具调用                      │
│     - 获取执行结果                      │
│     - 更新上下文                        │
└─────────────┬───────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────┐
│  3. Observe（观察）                      │
│     - 分析行动结果                      │
│     - 判断是否达成目标                  │
│     - 决定继续循环或返回结果            │
└─────────────────────────────────────────┘
```

## 循环流程

```go
for {
    // 1. Think
    thought, action, err := agent.Think(ctx, input)
    if err != nil {
        return err
    }
    
    // 检查是否需要停止
    if action == ActionFinish {
        return thought
    }
    
    // 2. Act
    result, err := agent.Act(ctx, action)
    if err != nil {
        return err
    }
    
    // 3. Observe
    input = result  // 将结果作为下一轮输入
    
    // 检查最大迭代次数
    if iterations >= maxIterations {
        return ErrMaxIterationsReached
    }
    iterations++
}
```

## 生命周期钩子

ReAct 循环的每个阶段都可以注入自定义逻辑：

### BeforeThink

在推理前执行，用于预处理输入或记录日志：

```go
agent.WithBeforeThink(func(ctx context.Context, input string) error {
    log.Printf("开始推理，输入: %s", input)
    return nil
})
```

### AfterThink

在推理后、行动前执行，用于拦截或修改动作：

```go
agent.WithAfterThink(func(ctx context.Context, thought string, action string) error {
    if containsSensitiveAction(action) {
        return ErrActionNotAllowed
    }
    return nil
})
```

### BeforeAct

在行动前执行，用于权限检查或参数验证：

```go
agent.WithBeforeAct(func(ctx context.Context, action string) error {
    if !hasPermission(action) {
        return ErrPermissionDenied
    }
    return nil
})
```

### AfterAct

在行动后执行，用于结果处理或日志记录：

```go
agent.WithAfterAct(func(ctx context.Context, action string, result string) error {
    log.Printf("执行动作: %s, 结果: %s", action, result)
    metrics.Record(action, result)
    return nil
})
```

## 最大迭代控制

防止无限循环，默认最大迭代次数为 10：

```go
agent := NewAgent(llm, tools).
    WithMaxIterations(20)  // 自定义最大迭代次数
```

## 错误处理

ReAct 循环中的错误处理策略：

1. **Think 错误**：立即返回，停止循环
2. **Act 错误**：根据重试策略决定是否重试
3. **Observe 错误**：记录错误，继续下一轮

```go
agent := NewAgent(llm, tools).
    WithRetryPolicy(RetryPolicy{
        MaxRetries: 3,
        Backoff:    ExponentialBackoff,
    })
```

## 上下文传递

ReAct 循环支持上下文传递，用于跨迭代保持状态：

```go
ctx := context.WithValue(context.Background(), "session_id", "123")
result, err := agent.Run(ctx, "任务")
```

## 性能优化

### 缓存推理结果

对于相同的输入，可以缓存推理结果：

```go
agent := NewAgent(llm, tools).
    WithCache(NewInMemoryCache())
```

### 并行工具调用

支持并行执行多个独立的工具调用：

```go
// LLM 输出多个工具调用时，自动并行执行
actions := []string{"tool1", "tool2", "tool3"}
results := agent.ActParallel(ctx, actions)
```

## 调试技巧

### 启用详细日志

```go
agent := NewAgent(llm, tools).
    WithLogLevel(LogLevelDebug)
```

### 使用 Inspector

通过 AP Inspector 可视化查看 ReAct 循环的每一步：

```go
inspector := debugger.NewInspector()
agent := NewAgent(llm, tools).
    WithInspector(inspector)

// 启动 Inspector UI
server := debugger.NewInspectorServer(inspector)
go server.Start(":8080")
```

访问 `http://localhost:8080` 查看实时追踪。

## 最佳实践

1. **设置合理的最大迭代次数**：防止无限循环消耗资源
2. **使用生命周期钩子记录日志**：便于调试和监控
3. **实现幂等的工具**：支持重试和恢复
4. **及时释放资源**：在 Close 方法中清理资源
5. **监控循环耗时**：设置超时防止阻塞

## 下一步

- 学习 [工具系统](tools.md) 如何与 ReAct 循环集成
- 查看 [多 Agent 编排](orchestration.md) 如何组合多个 ReAct 循环
- 阅读 [API 参考](../api/agent.md) 了解详细接口定义
