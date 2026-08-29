# Orchestration API

> 多 Agent 编排模式：Pipeline / Handoff / DAG / GroupChat / Debate。

## 接口概览

```go
// Pipeline — 顺序执行
func NewPipeline(steps ...PipelineStep) *Pipeline
// PipelineStep{Name string, Agent Agent, Input string, Condition func(...)}
func (p *Pipeline) Run(ctx context.Context, initialInput string) (*PipelineResult, error)

// Handoff — Agent间传递控制权
func NewHandoff(config HandoffConfig) *Handoff
// HandoffConfig{Agents []Agent, Router func(ctx, input string) int, MaxHandoffs int}
func (h *Handoff) Run(ctx context.Context, input string) (*HandoffResult, error)

// DAG — 有向无环图执行
func NewDAGBuilder(name string) *DAGBuilder
// NodeHandler: func(ctx context.Context, input string) (string, error)
func (b *DAGBuilder) Node(id string, handler NodeHandler) *DAGBuilder
func (b *DAGBuilder) NodeWithAgent(id string, agent Agent) *DAGBuilder
func (b *DAGBuilder) Edge(from, to string) *DAGBuilder
func (b *DAGBuilder) Build() (*DAGWorkflow, error)
func (w *DAGWorkflow) Run(ctx context.Context, input string) (*DAGResult, error)

// GroupChat — 多 Agent 圆桌讨论
func NewGroupChat(cfg GroupChatConfig) (*GroupChat, error)
// GroupChatConfig{Agents []Agent, MaxRounds int, SelectSpeaker SpeakerSelector, Bus MessageBus}
func (g *GroupChat) Run(ctx context.Context, initialMessage Message) (*GroupChatResult, error)

// Debate — 对抗式辩论
func NewDebate(config DebateConfig) *Debate
// DebateConfig{MaxRounds int, Timeout time.Duration}
func (d *Debate) AddDebater(debater Debater) error
func (d *Debate) Execute(ctx context.Context, topic string) (*DebateResult, error)
```

## 示例：Pipeline

```go
pipeline := ap.NewPipeline(
    ap.PipelineStep{Name: "extract", Agent: extractorAgent},
    ap.PipelineStep{Name: "transform", Agent: transformerAgent},
    ap.PipelineStep{Name: "load", Agent: loaderAgent},
)
result, _ := pipeline.Run(ctx, rawData)
```

## 示例：DAG

```go
dag, err := ap.NewDAGBuilder("etl").
    Node("a", taskA).
    Node("b", taskB).
    Node("c", taskC).
    Edge("a", "b").
    Edge("a", "c"). // b 和 c 并行
    Build()
result, err := dag.Run(ctx, input)
```

## 错误处理

- Pipeline 步骤可配置 `Condition`（基于上一步结果决定是否执行/跳过）
- DAG 节点失败时内置重试，下游节点可以选择跳过或终止
- 超时控制通过 `context.Context` 传递

## 可观测性

所有编排器自动暴露 Prometheus 指标：
- `ap_orchestration_stages_total` — 总 stage 执行次数
- `ap_orchestration_stage_duration_seconds` — stage 耗时直方图
- `ap_orchestration_errors_total` — 错误计数（按 stage 和 error_type 拆分）
