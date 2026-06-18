# Agent 架构

AgentPrimordia 的核心是 **协议式微内核架构**，通过 14 个 Capable 接口实现能力发现与组合。

## 核心设计原则

### 1. 协议优于配置

Agent 通过类型断言发现能力，而非配置文件：

```go
// 检查 Agent 是否支持记忆
if memCapable, ok := agent.(agent.MemoryCapable); ok {
    memory := memCapable.GetMemory()
    memory.Store(ctx, "key", "value")
}
```

### 2. 接口组合

Agent 通过实现不同接口组合获得不同能力：

```go
type Agent interface {
    // 基础能力
    Run(ctx context.Context, input string) (string, error)
    Close() error
    
    // 可选能力（通过接口组合）
    MemoryCapable
    ToolCapable
    ContextCapable
    // ... 14 个 Capable 接口
}
```

### 3. 生命周期钩子

ReAct 循环的每个阶段都可以注入自定义逻辑：

```go
agent := NewAgent(llm, tools).
    WithBeforeThink(func(ctx context.Context, input string) error {
        fmt.Println("即将开始推理...")
        return nil
    }).
    WithAfterAct(func(ctx context.Context, action string, result string) error {
        fmt.Printf("执行动作: %s, 结果: %s\n", action, result)
        return nil
    })
```

## Agent 类型

### 基础 Agent

最简单的 Agent 实现，包含 ReAct 循环和工具调用：

```go
agent := NewAgent(llm, tools)
result, err := agent.Run(ctx, "你好，世界")
```

### CapabilityAgent

包装任意 Agent，提供链式 API 访问所有能力：

```go
capAgent := NewCapabilityAgent(baseAgent).
    WithMemory(memory).
    WithTools(tools).
    WithContextManager(ctxMgr)

// 链式调用
capAgent.GetMemory().Store(ctx, "user:1", "Alice")
capAgent.GetTools().Register(myTool)
```

### 编排 Agent

支持多 Agent 协作的高级 Agent：

```go
// 顺序编排
orch := NewSequentialOrchestrator([]Agent{agent1, agent2, agent3})
result := orch.Run(ctx, "任务")

// DAG 编排
dag := NewDAGOrchestrator()
dag.AddNode("analyze", analyzeAgent)
dag.AddNode("process", processAgent)
dag.AddEdge("analyze", "process")
result := dag.Run(ctx, "复杂任务")
```

## 14 个 Capable 接口

| 接口 | 能力 | 典型用途 |
|------|------|----------|
| `MemoryCapable` | 记忆存储与检索 | 长期记忆、上下文保持 |
| `RAGCapable` | 检索增强生成 | 知识库问答、文档检索 |
| `ToolCapable` | 工具调用 | 执行外部操作 |
| `ContextCapable` | 上下文管理 | 会话管理、历史追踪 |
| `OrchestrationCapable` | 多 Agent 编排 | 任务分解、协作 |
| `LifecycleCapable` | 生命周期钩子 | 监控、日志、拦截 |
| `RetryCapable` | 重试策略 | 容错、稳定性 |
| `TimeoutCapable` | 超时控制 | 防止阻塞 |
| `CircuitBreakerCapable` | 熔断器 | 服务降级 |
| `MetricsCapable` | 指标收集 | 性能监控 |
| `TraceCapable` | 链路追踪 | 调试、分析 |
| `ConfigCapable` | 动态配置 | 运行时调整 |
| `HealthCheckCapable` | 健康检查 | 运维监控 |
| `InspectorCapable` | 检查器集成 | 可视化调试 |

## 架构优势

1. **零配置能力发现**：无需配置文件，通过类型断言即可发现 Agent 能力
2. **高度可扩展**：新增能力只需定义新接口，不影响现有代码
3. **组合优于继承**：通过接口组合而非继承获得能力，避免类爆炸
4. **生产级可靠性**：内置重试、熔断、超时等生产必需特性
5. **可观测性**：原生支持指标、追踪、日志，便于调试和监控

## 下一步

- 深入了解 [ReAct 循环](react-loop.md) 的工作原理
- 学习如何 [创建 Agent](../guides/create-agent.md)
- 查看 [API 参考](../api/agent.md) 了解详细接口定义
