# 多 Agent 编排指南

> 协调多个 Agent 协作完成复杂任务。

## 编排模式

### Pipeline（管道）

Agent 顺序执行，前一个输出是后一个输入：

```go
pipeline := ap.NewPipeline(
    ap.NewStage("extract", extractAgent),
    ap.NewStage("transform", transformAgent),
    ap.NewStage("load", loadAgent),
)
result, _ := pipeline.Execute(ctx, rawData)
```

### Handoff（交接）

Agent 完成工作后，通过 Handoff 事件将控制权交给下一个 Agent：

```go
handoff := ap.NewHandoff(writerAgent, editorAgent, publisherAgent)
result, _ :=andoff.Execute(ctx, draftContent)
```

### DAG（有向无环图）

支持并行分支与条件路由：

```go
dag := ap.NewDAG().
    AddNode("analyze", analyze).
    AddNode("summary", summary).
    AddNode("report", report).
    AddNode("alert", alert).
    AddEdge("analyze", "summary").
    AddEdge("analyze", "alert").
    AddEdge("summary", "report")
result, _ := dag.Execute(ctx, input)
```

### GroupChat（圆桌讨论）

多 Agent 讨论，投票决策：

```go
chat := ap.NewGroupChat(
    ap.NewMember("analyst", analystAgent),
    ap.NewMember("skeptic", skepticAgent),
    ap.NewMember("optimizer", optimizerAgent),
)
result, _ := chat.Discuss(ctx, topic, ap.VotePolicyMajority)
```

### Debate（辩论）

正反双方 + 裁判模式：

```go
debate := ap.NewDebate(proAgent, conAgent, judgeAgent)
result, _ := debate.Debate(ctx, question)
```

## 错误与重试

```go
stage := ap.NewStage("analyze", agent,
    ap.WithRetry(3, ap.ExponentialBackoff),
    ap.WithFallback(fallbackAgent),
    ap.WithTimeout(60*time.Second),
)
```

## 可观测性

所有编排器自动上报 Prometheus 指标：

```promql
# 各 stage 延迟 P99
histogram_quantile(0.99, ap_orchestration_stage_duration_seconds_bucket)

# 各编排模式执行速率
rate(ap_orchestration_executions_total{mode="handoff"}[5m])
```
