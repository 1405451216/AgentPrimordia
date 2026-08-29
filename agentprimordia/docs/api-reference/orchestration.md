# Orchestration API 参考

> `package ap` — 多 Agent 编排模式。全部经 `agentprimordia/pkg` 别名暴露。

## Pipeline（管道）

顺序执行多个 Agent，前一个 Agent 的输出作为后一个的输入：

```go
func NewPipeline(steps ...PipelineStep) *Pipeline

type PipelineStep struct {
    Name      string
    Agent     Agent
    Input     string                                                    // 可选，覆盖该步输入
    Condition func(ctx context.Context, prevResult *StepResult) bool    // 可选，条件跳过
}

// 执行
func (p *Pipeline) Run(ctx context.Context, initialInput string) (*PipelineResult, error)
```

**示例：**

```go
pipeline := ap.NewPipeline(
    ap.PipelineStep{Name: "extract", Agent: extractAgent},
    ap.PipelineStep{Name: "transform", Agent: transformAgent},
    ap.PipelineStep{Name: "load", Agent: loadAgent},
)
result, err := pipeline.Run(ctx, rawData)
fmt.Println(result.Final)
```

## Handoff（交接）

按路由函数在多个 Agent 间动态交接：

```go
func NewHandoff(config HandoffConfig) *Handoff

type HandoffConfig struct {
    Agents      []Agent                                        // 参与交接的 Agent（按索引路由）
    Router      func(ctx context.Context, input string) int    // 返回下一个 Agent 的索引
    MaxHandoffs int                                            // 最大交接次数
}

func (h *Handoff) Run(ctx context.Context, input string) (*HandoffResult, error)
```

**示例：**

```go
handoff := ap.NewHandoff(ap.HandoffConfig{
    Agents: []ap.Agent{writerAgent, editorAgent, publisherAgent},
    Router: func(ctx context.Context, input string) int {
        // 按内容决定下一个 Agent；返回 -1 结束交接
        return 1
    },
    MaxHandoffs: 3,
})
result, err := handoff.Run(ctx, draftContent)
```

## DAG（有向无环图）

```go
func NewDAGBuilder(name string) *DAGBuilder

// 链式构建
func (b *DAGBuilder) Node(id string, handler NodeHandler) *DAGBuilder        // NodeHandler: func(ctx, input string) (string, error)
func (b *DAGBuilder) NodeWithAgent(id string, agent core.Agent) *DAGBuilder  // 直接挂 Agent
func (b *DAGBuilder) Edge(from, to string) *DAGBuilder
func (b *DAGBuilder) Build() (*DAGWorkflow, error)                           // 或 MustBuild()（panic on error）
```

**示例：**

```go
dag, err := ap.NewDAGBuilder("etl").
    Node("analyze", func(ctx context.Context, input string) (string, error) {
        resp, err := analyzeAgent.Run(ctx, ap.UserMessage(input))
        if err != nil {
            return "", err
        }
        return resp.Content, nil
    }).
    Node("report", reportHandler).
    Edge("analyze", "report").
    Build()
result, err := dag.Execute(ctx, input)
```

## GroupChat（圆桌讨论）

```go
func NewGroupChat(cfg GroupChatConfig) (*GroupChat, error)

type GroupChatConfig struct {
    Agents        []Agent
    MaxRounds     int             // 最大讨论轮次
    SelectSpeaker SpeakerSelector // 发言人选择策略
    Bus           MessageBus      // 消息总线
}

func (g *GroupChat) Run(ctx context.Context, initialMessage Message) (*GroupChatResult, error)
func (g *GroupChat) RunConsensus(ctx context.Context, question Message) (*ConsensusResult, error)
```

**示例：**

```go
chat, err := ap.NewGroupChat(ap.GroupChatConfig{
    Agents:    []ap.Agent{analystAgent, skepticAgent},
    MaxRounds: 5,
})
result, err := chat.Run(ctx, ap.UserMessage("评估这个方案的可行性"))
```

## Debate（辩论）

```go
func NewDebate(config DebateConfig) *Debate

type DebateConfig struct {
    MaxRounds int           `json:"max_rounds"` // 最大轮数（默认 3）
    Timeout   time.Duration `json:"timeout"`    // 总超时时间
}

func (d *Debate) AddDebater(debater Debater) error
func (d *Debate) Execute(ctx context.Context, topic string) (*DebateResult, error)
```

**示例：**

```go
debate := ap.NewDebate(ap.DebateConfig{MaxRounds: 3})
debate.AddDebater(proDebater)
debate.AddDebater(conDebater)
result, err := debate.Execute(ctx, "AI 是否应该拥有权利？")
```
