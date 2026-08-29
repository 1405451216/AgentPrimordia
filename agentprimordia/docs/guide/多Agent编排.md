# 多 Agent 编排指南

> 协调多个 Agent 协作完成复杂任务。

## 编排模式

### Pipeline（管道）

Agent 顺序执行，前一个输出是后一个输入：

```go
pipeline := ap.NewPipeline(
    ap.PipelineStep{Name: "extract", Agent: extractAgent},
    ap.PipelineStep{Name: "transform", Agent: transformAgent},
    ap.PipelineStep{Name: "load", Agent: loadAgent},
)
result, _ := pipeline.Run(ctx, rawData)
```

### Handoff（交接）

Agent 完成工作后，由 Router 决定将控制权交给下一个 Agent：

```go
handoff := ap.NewHandoff(ap.HandoffConfig{
    Agents: []ap.Agent{writerAgent, editorAgent, publisherAgent},
    Router: func(ctx context.Context, input string) int {
        // 根据输入决定交给第几个 Agent（返回下标）
        if strings.Contains(input, "审校") {
            return 1
        }
        return 0
    },
    MaxHandoffs: 5,
})
result, _ := handoff.Run(ctx, draftContent)
```

### DAG（有向无环图）

支持并行分支与条件路由：

```go
dag, err := ap.NewDAGBuilder("report-pipeline").
    Node("analyze", analyze).
    Node("summary", summary).
    Node("report", report).
    Node("alert", alert).
    Edge("analyze", "summary").
    Edge("analyze", "alert").
    Edge("summary", "report").
    Build()
result, err := dag.Run(ctx, input)
```

### GroupChat（圆桌讨论）

多 Agent 讨论，多轮发言：

```go
chat, err := ap.NewGroupChat(ap.GroupChatConfig{
    Agents:    []ap.Agent{analystAgent, skepticAgent, optimizerAgent},
    MaxRounds: 6,
})
result, err := chat.Run(ctx, ap.UserMessage(topic))
```

### Debate（辩论）

正反双方多轮论辩，逐步达成共识：

```go
debate := ap.NewDebate(ap.DebateConfig{MaxRounds: 3})
_ = debate.AddDebater(proDebater)    // 实现 ap.Debater 接口
_ = debate.AddDebater(conDebater)
result, _ := debate.Execute(ctx, question)
```

## 错误与重试

- DAG 节点失败时内置重试，可经 `context.Context` 传递超时控制
- Pipeline 步骤可配置 `Condition`，基于上一步结果决定是否执行/跳过
- LLM 调用的重试/降级在 Provider 层处理：`ap.NewResilientProvider(primary, ap.DefaultResilientConfig())`

## 可观测性

所有编排器自动上报 Prometheus 指标：

```promql
# 各 stage 延迟 P99
histogram_quantile(0.99, ap_orchestration_stage_duration_seconds_bucket)

# 各编排模式执行速率
rate(ap_orchestration_executions_total{mode="handoff"}[5m])
```
