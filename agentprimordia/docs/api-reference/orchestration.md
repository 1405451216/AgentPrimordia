# Orchestration API 参考

> `package ap` — 多 Agent 编排模式。

## Orchestrator 接口

```go
type Orchestrator interface {
    Execute(ctx context.Context, input any) (*Result, error)
}

type Result struct {
    Output any
    Steps  []StepResult
    Error  error
}
```

## Pipeline（管道）

```go
func NewPipeline(stages ...Stage) *Pipeline

type Stage struct {
    Name    string
    Agent   *Agent
    Retry   RetryPolicy
    Timeout time.Duration
}

// 示例
pipeline := ap.NewPipeline(
    ap.NewStage("extract", extractAgent, ap.WithRetry(3)),
    ap.NewStage("transform", transformAgent),
    ap.NewStage("load", loadAgent, ap.WithTimeout(30*time.Second)),
)
result, _ := pipeline.Execute(ctx, rawData)
```

## Handoff（交接）

```go
func NewHandoff(agents ...*Agent) *Handoff

// 示例
handoff := ap.NewHandoff(writerAgent, editorAgent, publisherAgent)
result, _ := handoff.Execute(ctx, draftContent)
```

## DAG（有向无环图）

```go
func NewDAG() *DAGBuilder

// 示例
dag := ap.NewDAG().
    AddNode("analyze", analyze).
    AddNode("summary", summary).
    AddNode("report", report).
    AddEdge("analyze", "summary").
    AddEdge("analyze", "report")
result, _ := dag.Execute(ctx, input)
```

## GroupChat（圆桌讨论）

```go
func NewGroupChat(members ...*GroupMember) *GroupChat

type GroupMember struct {
    Name string
    Agent *Agent
    Role string // 角色描述
}

// 示例
chat := ap.NewGroupChat(
    ap.NewMember("analyst", analystAgent, "数据分析师"),
    ap.NewMember("skeptic", skepticAgent, "质疑者"),
)
result, _ := chat.Discuss(ctx, topic, ap.VotePolicyMajority)
```

## Debate（辩论）

```go
func NewDebate(pro, con, judge *Agent) *Debate

// 示例
debate := ap.NewDebate(proAgent, conAgent, judgeAgent)
result, _ := debate.Debate(ctx, "AI 是否应该拥有权利？")
```

## 错误处理

```go
type StageError struct {
    Stage string
    Err   error
    Retry int
}

// 配置降级 Agent
stage := ap.NewStage("analyze", primaryAgent,
    ap.WithFallback(fallbackAgent),
    ap.WithRetry(3, ap.ExponentialBackoff),
)
```
