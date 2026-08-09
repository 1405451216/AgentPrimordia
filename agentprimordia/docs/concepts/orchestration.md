# 多 Agent 编排

AgentPrimordia 提供了强大的多 Agent 编排能力，支持顺序、并行、DAG、群聊和工作流五种模式。

## 编排模式概览

| 模式 | 适用场景 | 特点 |
|------|----------|------|
| **Sequential** | 任务有明确的先后顺序 | 简单、可预测 |
| **Parallel** | 多个独立任务可并行执行 | 高性能、节省时间 |
| **DAG** | 复杂任务有依赖关系 | 灵活、支持分支合并 |
| **GroupChat** | 多 Agent 讨论协作 | 动态、交互式 |
| **Workflow** | 复杂业务流程 | 支持条件、循环、状态机 |

## Sequential 编排

Agent 按顺序依次执行，前一个的输出作为后一个的输入：

```go
orch := orchestration.NewSequentialOrchestrator([]agent.Agent{
    analyzerAgent,   // 1. 分析需求
    plannerAgent,    // 2. 制定计划
    executorAgent,   // 3. 执行任务
    reviewerAgent,   // 4. 审查结果
})

result, err := orch.Run(ctx, "开发一个新功能")
```

### 执行流程

```
输入 → Agent1 → Agent2 → Agent3 → ... → 输出
```

## Parallel 编排

多个 Agent 并行执行，结果合并：

```go
orch := orchestration.NewParallelOrchestrator([]agent.Agent{
    searchAgent1,  // 搜索引擎 1
    searchAgent2,  // 搜索引擎 2
    searchAgent3,  // 搜索引擎 3
})

results, err := orch.Run(ctx, "搜索 Agent 框架")
// results 包含所有 Agent 的结果
```

### 执行流程

```
      ┌→ Agent1 ─┐
输入 ─┼→ Agent2 ─┼→ 合并 → 输出
      └→ Agent3 ─┘
```

### 并发控制

限制最大并发数：

```go
orch := orchestration.NewParallelOrchestrator(agents).
    WithMaxConcurrency(5)  // 最多 5 个并发
```

## DAG 编排

有向无环图编排，支持复杂的依赖关系：

```go
dag := orchestration.NewDAGOrchestrator()

// 添加节点
dag.AddNode("collect", collectAgent)
dag.AddNode("analyze", analyzeAgent)
dag.AddNode("process", processAgent)
dag.AddNode("report", reportAgent)

// 添加边（依赖关系）
dag.AddEdge("collect", "analyze")
dag.AddEdge("collect", "process")
dag.AddEdge("analyze", "report")
dag.AddEdge("process", "report")

result, err := dag.Run(ctx, "数据分析任务")
```

### 执行流程

```
         ┌→ analyze ─┐
collect ─┤           ├→ report
         └→ process ─┘
```

### 拓扑排序

DAG 自动进行拓扑排序，确定执行顺序：

```go
order, err := dag.TopologicalSort()
// order: ["collect", "analyze", "process", "report"]
```

## GroupChat 编排

多 Agent 讨论协作，适合需要多轮对话的场景：

```go
groupChat := orchestration.NewGroupChatOrchestrator(
    []agent.Agent{moderatorAgent, expert1Agent, expert2Agent},
    orchestration.GroupChatConfig{
        MaxRounds:     10,     // 最多 10 轮
        StopCondition: "共识达成",  // 停止条件
    },
)

result, err := groupChat.Run(ctx, "讨论 AI 伦理问题")
```

### 执行流程

```
Round 1: Moderator → Expert1 → Expert2
Round 2: Moderator → Expert1 → Expert2
...
Round N: 达成共识，停止
```

## Workflow 编排

工作流编排支持条件分支、循环和状态机：

### 线性工作流

```go
workflow := orchestration.NewWorkflow(orchestration.WorkflowConfig{
    Steps: []orchestration.WorkflowStep{
        {Name: "step1", Agent: agent1},
        {Name: "step2", Agent: agent2},
        {Name: "step3", Agent: agent3},
    },
})
```

### 条件分支

```go
workflow := orchestration.NewWorkflow(orchestration.WorkflowConfig{
    Steps: []orchestration.WorkflowStep{
        {Name: "analyze", Agent: analyzeAgent},
        {
            Name: "decision",
            Condition: func(ctx context.Context, state map[string]interface{}) string {
                if state["complexity"].(string) == "high" {
                    return "complex_path"
                }
                return "simple_path"
            },
            Branches: map[string]orchestration.WorkflowStep{
                "complex_path": {Name: "complex", Agent: complexAgent},
                "simple_path":  {Name: "simple", Agent: simpleAgent},
            },
        },
    },
})
```

### 循环

```go
workflow := orchestration.NewWorkflow(orchestration.WorkflowConfig{
    Steps: []orchestration.WorkflowStep{
        {Name: "generate", Agent: generateAgent},
        {
            Name: "review",
            Condition: func(ctx context.Context, state map[string]interface{}) string {
                if state["score"].(float64) < 0.8 {
                    return "retry"  // 重新生成
                }
                return "done"
            },
            Branches: map[string]orchestration.WorkflowStep{
                "retry": {Name: "generate", Agent: generateAgent},
                "done":  {},  // 空步骤，结束循环
            },
        },
    },
})
```

### 状态机

```go
workflow := orchestration.NewWorkflow(orchestration.WorkflowConfig{
    States: map[string]orchestration.State{
        "pending": {
            Agent: pendingAgent,
            Transitions: map[string]string{
                "approved": "processing",
                "rejected": "done",
            },
        },
        "processing": {
            Agent: processAgent,
            Transitions: map[string]string{
                "completed": "done",
                "failed":    "pending",
            },
        },
        "done": {},  // 终态
    },
    InitialState: "pending",
})
```

## 编排组合

可以将多个编排器组合使用：

```go
// 外层并行，内层顺序
outer := orchestration.NewParallelOrchestrator([]agent.Agent{
    orchestration.NewSequentialOrchestrator([]agent.Agent{a1, a2}),
    orchestration.NewSequentialOrchestrator([]agent.Agent{a3, a4}),
})
```

## 错误处理

编排中的错误处理策略：

### 失败继续

```go
orch := orchestration.NewParallelOrchestrator(agents).
    WithErrorStrategy(orchestration.ContinueOnError)
```

### 失败停止

```go
orch := orchestration.NewSequentialOrchestrator(agents).
    WithErrorStrategy(orchestration.StopOnError)  // 默认
```

### 重试

```go
orch := orchestration.NewDAGOrchestrator().
    WithRetryPolicy(orchestration.RetryPolicy{
        MaxRetries: 3,
        Backoff:    orchestration.ExponentialBackoff,
    })
```

## 监控与追踪

### 指标收集

```go
orch := orchestration.NewDAGOrchestrator().
    WithMetrics(metricsCollector)
```

### 链路追踪

```go
orch := orchestration.NewWorkflow(...).
    WithTracing(tracer)
```

### Inspector 集成

```go
inspector := debugger.NewInspector()
orch := orchestration.NewDAGOrchestrator().
    WithInspector(inspector)
```

## 最佳实践

1. **选择合适的编排模式**：根据任务特点选择最合适的模式
2. **控制并发数**：防止资源耗尽
3. **设置超时**：防止长时间阻塞
4. **实现幂等**：支持重试和恢复
5. **监控性能**：及时发现瓶颈
6. **错误隔离**：单个 Agent 失败不影响整体

## 下一步

- 学习如何 [实现多 Agent 协作](../guides/multi-agent.md)
- 查看 [多 Agent 指南](../guides/multi-agent.md) 了解实际应用
- 阅读 [API 参考](../api/orchestration.md) 了解详细接口定义
