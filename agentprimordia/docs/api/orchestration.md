# Orchestration API

> 多 Agent 编排模式：Pipeline / Handoff / DAG / GroupChat / Debate。

## 接口概览

```go
type Orchestrator interface {
    Execute(ctx context.Context, input any) (*Result, error)
}

// Pipeline — 顺序执行
func NewPipeline(stages ...Stage) *Pipeline

// Handoff — Agent间传递控制权
func NewHandoff(orchestrators ...Orchestrator) *Handoff

// DAG — 有向无环图执行
func NewDAG() *DAGBuilder

// GroupChat — 多 Agent 圆桌讨论
func NewGroupChat(members ...*GroupMember) *GroupChat

// Debate — 对抗式辩论
func NewDebate(pro, con Orchestrator, judge Orchestrator) *Debate
```

## 示例：Pipeline

```go
pipeline := ap.NewPipeline(
    ap.NewStage("extract", extractorAgent),
    ap.NewStage("transform", transformerAgent),
    ap.NewStage("load", loaderAgent),
)
result, _ := pipeline.Execute(ctx, rawData)
```

## 示例：DAG

```go
dag := ap.NewDAG().
    AddNode("A", taskA).
    AddNode("B", taskB).
    AddNode("C", taskC).
    AddEdge("A", "B").
    AddEdge("A", "C")  // B 和 C 并行
result, _ := dag.Execute(ctx, input)
```

## 错误处理

- 每个 Stage 可配置 `RetryPolicy`（最大重试次数、退避策略）
- DAG 节点失败时，下游节点可以选择跳过或终止
- 超时控制通过 `context.Context` 传递

## 可观测性

所有编排器自动暴露 Prometheus 指标：
- `ap_orchestration_stages_total` — 总 stage 执行次数
- `ap_orchestration_stage_duration_seconds` — stage 耗时直方图
- `ap_orchestration_errors_total` — 错误计数（按 stage 和 error_type 拆分）
