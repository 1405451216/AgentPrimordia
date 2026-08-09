# 多 Agent 协作

本指南介绍如何使用 AgentPrimordia 实现多 Agent 协作。

## 选择编排模式

| 场景 | 推荐模式 | 原因 |
|------|----------|------|
| 流水线处理 | Sequential | 任务有明确的先后顺序 |
| 并行搜索 | Parallel | 多个独立任务可并行 |
| 复杂依赖 | DAG | 任务之间有复杂的依赖关系 |
| 讨论决策 | GroupChat | 需要多轮讨论达成共识 |
| 业务流程 | Workflow | 需要条件分支、循环等控制流 |

## Sequential 编排

### 基本用法

```go
import "agentprimordia.dev/agentprimordia/pkg/orchestration"

// 创建 Agent 链
agents := []agent.Agent{
    analyzerAgent,   // 1. 分析需求
    plannerAgent,    // 2. 制定计划
    executorAgent,   // 3. 执行任务
    reviewerAgent,   // 4. 审查结果
}

orch := orchestration.NewSequentialOrchestrator(agents)
result, err := orch.Run(ctx, "开发一个新功能")
```

### 数据传递

前一个 Agent 的输出自动作为后一个的输入：

```
输入 → Analyzer → Planner → Executor → Reviewer → 输出
```

### 错误处理

```go
orch := orchestration.NewSequentialOrchestrator(agents).
    WithErrorStrategy(orchestration.StopOnError)  // 默认：遇到错误停止
    // WithErrorStrategy(orchestration.ContinueOnError)  // 跳过错误继续
```

## Parallel 编排

### 基本用法

```go
agents := []agent.Agent{
    searchAgent1,
    searchAgent2,
    searchAgent3,
}

orch := orchestration.NewParallelOrchestrator(agents)
results, err := orch.Run(ctx, "搜索 Agent 框架")
// results 包含所有 Agent 的结果
```

### 并发控制

```go
orch := orchestration.NewParallelOrchestrator(agents).
    WithMaxConcurrency(5)  // 最多 5 个并发
```

### 结果合并

```go
orch := orchestration.NewParallelOrchestrator(agents).
    WithMerger(func(results []string) string {
        // 自定义合并逻辑
        return strings.Join(results, "\n---\n")
    })
```

## DAG 编排

### 基本用法

```go
dag := orchestration.NewDAGOrchestrator()

// 添加节点
dag.AddNode("collect", collectAgent)
dag.AddNode("analyze", analyzeAgent)
dag.AddNode("process", processAgent)
dag.AddNode("report", reportAgent)

// 添加边（依赖关系）
dag.AddEdge("collect", "analyze")
dag.AddNode("collect", "process")
dag.AddEdge("analyze", "report")
dag.AddEdge("process", "report")

result, err := dag.Run(ctx, "数据分析任务")
```

### 执行图

```
         ┌→ analyze ─┐
collect ─┤           ├→ report
         └→ process ─┘
```

### 拓扑排序

```go
order, err := dag.TopologicalSort()
// order: ["collect", "analyze", "process", "report"]
```

### 并行执行

DAG 自动识别可并行的节点：

```go
dag := orchestration.NewDAGOrchestrator().
    WithMaxConcurrency(10)
```

## GroupChat 编排

### 基本用法

```go
agents := []agent.Agent{
    moderatorAgent,  // 主持人
    expert1Agent,    // 专家 1
    expert2Agent,    // 专家 2
}

groupChat := orchestration.NewGroupChatOrchestrator(agents, orchestration.GroupChatConfig{
    MaxRounds:     10,
    StopCondition: "达成共识",
})

result, err := groupChat.Run(ctx, "讨论 AI 伦理问题")
```

### 轮次控制

```go
groupChat := orchestration.NewGroupChatOrchestrator(agents, orchestration.GroupChatConfig{
    MaxRounds: 5,  // 最多 5 轮讨论
})
```

### 自定义停止条件

```go
groupChat := orchestration.NewGroupChatOrchestrator(agents, orchestration.GroupChatConfig{
    StopConditionFunc: func(ctx context.Context, history []string) bool {
        // 检查是否达成共识
        return checkConsensus(history)
    },
})
```

## Workflow 编排

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
            Name: "route",
            Condition: func(ctx context.Context, state map[string]interface{}) string {
                complexity := state["complexity"].(string)
                if complexity == "high" {
                    return "complex"
                }
                return "simple"
            },
            Branches: map[string]orchestration.WorkflowStep{
                "complex": {Name: "complex_process", Agent: complexAgent},
                "simple":  {Name: "simple_process", Agent: simpleAgent},
            },
        },
        {Name: "finalize", Agent: finalizeAgent},
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
                score := state["score"].(float64)
                if score < 0.8 {
                    return "retry"
                }
                return "done"
            },
            Branches: map[string]orchestration.WorkflowStep{
                "retry": {Name: "generate", Agent: generateAgent},
                "done":  {},
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
                "completed": "reviewing",
                "failed":    "pending",
            },
        },
        "reviewing": {
            Agent: reviewAgent,
            Transitions: map[string]string{
                "passed":  "done",
                "failed":  "processing",
            },
        },
        "done": {},
    },
    InitialState: "pending",
})
```

## 编排组合

嵌套编排实现复杂流程：

```go
// 内层：顺序处理
phase1 := orchestration.NewSequentialOrchestrator([]agent.Agent{a1, a2})
phase2 := orchestration.NewSequentialOrchestrator([]agent.Agent{a3, a4})

// 外层：并行执行两个阶段
outer := orchestration.NewParallelOrchestrator([]agent.Agent{phase1, phase2})

result, err := outer.Run(ctx, "复杂任务")
```

## 共享状态

### 状态传递

```go
// 创建共享状态
state := orchestration.NewSharedState()
state.Set("user_id", "123")
state.Set("task_type", "analysis")

// 编排器使用共享状态
orch := orchestration.NewSequentialOrchestrator(agents).
    WithSharedState(state)
```

### 状态访问

```go
// Agent 中访问共享状态
func (a *MyAgent) Run(ctx context.Context, input string) (string, error) {
    state := orchestration.GetSharedState(ctx)
    userID := state.Get("user_id")
    // ...
}
```

## 监控与追踪

### Inspector 集成

```go
inspector := debugger.NewInspector()

orch := orchestration.NewDAGOrchestrator().
    WithInspector(inspector)

// 启动 Inspector UI
server := debugger.NewInspectorServer(inspector)
go http.ListenAndServe(":8080", server.Handler())
```

### 指标收集

```go
metrics := metrics.NewCollector()

orch := orchestration.NewParallelOrchestrator(agents).
    WithMetrics(metrics)

// 获取指标
fmt.Printf("总耗时: %v\n", metrics.GetDuration())
fmt.Printf("成功率: %f\n", metrics.GetSuccessRate())
```

## 错误处理

### 重试策略

```go
orch := orchestration.NewDAGOrchestrator().
    WithRetryPolicy(orchestration.RetryPolicy{
        MaxRetries: 3,
        Backoff:    orchestration.ExponentialBackoff,
        BaseDelay:  time.Second,
    })
```

### 超时控制

```go
orch := orchestration.NewWorkflow(config).
    WithTimeout(5 * time.Minute)
```

### 错误隔离

```go
orch := orchestration.NewParallelOrchestrator(agents).
    WithErrorStrategy(orchestration.IsolateError)  // 单个失败不影响其他
```

## 完整示例：代码审查流水线

```go
func createCodeReviewPipeline() (orchestration.Orchestrator, error) {
    // 1. 代码分析 Agent
    analyzer := agent.NewAgent(llm, toolMgr).
        WithSystemPrompt("你是代码分析专家，负责分析代码结构和复杂度。")
    
    // 2. 安全审查 Agent
    securityReviewer := agent.NewAgent(llm, toolMgr).
        WithSystemPrompt("你是安全专家，负责检查代码安全漏洞。")
    
    // 3. 性能审查 Agent
    perfReviewer := agent.NewAgent(llm, toolMgr).
        WithSystemPrompt("你是性能专家，负责检查性能问题。")
    
    // 4. 综合评审 Agent
    finalReviewer := agent.NewAgent(llm, toolMgr).
        WithSystemPrompt("你是首席评审官，综合所有审查意见给出最终结论。")
    
    // DAG 编排
    dag := orchestration.NewDAGOrchestrator()
    dag.AddNode("analyze", analyzer)
    dag.AddNode("security", securityReviewer)
    dag.AddNode("performance", perfReviewer)
    dag.AddNode("final", finalReviewer)
    
    dag.AddEdge("analyze", "security")
    dag.AddEdge("analyze", "performance")
    dag.AddEdge("security", "final")
    dag.AddEdge("performance", "final")
    
    return dag, nil
}
```

## 最佳实践

1. **选择合适的模式**：根据任务特点选择最合适的编排模式
2. **控制并发数**：防止资源耗尽
3. **设置超时**：防止长时间阻塞
4. **实现幂等**：支持重试和恢复
5. **错误隔离**：单个 Agent 失败不影响整体
6. **监控性能**：使用 Inspector 和指标收集
7. **共享状态**：使用 SharedState 传递上下文信息

## 下一步

- 查看 [多 Agent 指南](../guides/multi-agent.md) 了解更多实际案例
- 阅读 [编排 API](../api/orchestration.md) 了解完整接口定义
- 学习 [调试器](../advanced/debugger.md) 使用可视化工具
