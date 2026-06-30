# Agent API

Agent API 参考文档。

## Agent 接口

```go
// Agent 是所有 Agent 实现的核心接口
// 编排模式（Pipeline/Handoff/Parallel）和 Pool 均面向此接口编程
type Agent interface {
    // Run 执行同步推理，接收 Message 输入并返回 Response
    Run(ctx context.Context, input Message) (*Response, error)

    // StreamRun 执行流式推理，返回 StreamEvent 通道
    StreamRun(ctx context.Context, input Message) (<-chan StreamEvent, error)

    // Stop 停止当前运行
    Stop()

    // Stats 返回运行统计
    Stats() AgentStats

    // Name 返回 Agent 名称
    Name() string
}
```

## NewAgent

创建 Agent 的推荐入口（v0.7.0 起）。只暴露核心字段（名称、系统提示词、模型），能力通过 Functional Options 注入。

```go
func NewAgent(name, systemPrompt string, model llm.Provider, opts ...Option) (*CapabilityAgent, error)
```

**参数：**
- `name`: Agent 名称（不能为空）
- `systemPrompt`: 系统提示词（可为空）
- `model`: LLM Provider（不能为 nil）
- `opts`: Functional Options（`WithMaxTurns`、`WithMemory`、`WithToolkit` 等）

**返回：**
- `*CapabilityAgent`: 具备链式 API 的 Agent 实例
- `error`: 配置校验错误

**示例：**

=== "Go"

    ```go
    agent, err := ap.NewAgent("my-agent", "你是一个智能助手", provider,
        ap.WithMaxTurns(10),
        ap.WithMemory(memStore),
        ap.WithToolkit(registry),
    )
    if err != nil {
        log.Fatal(err)
    }

    resp, err := agent.Run(ctx, ap.UserMessage("你好"))
    ```

=== "TypeScript"

    ```typescript
    const agent = new ReActAgent({
      name: 'my-agent',
      systemPrompt: '你是一个智能助手',
      model: provider,
      maxTurns: 10,
      memory: memStore,
      toolkit: registry,
    });

    const resp = await agent.run('你好');
    ```

## NewReActAgent

传统入口（仍然兼容），通过 `ReActConfig` 配置核心参数，再通过链式 API 注入能力。

```go
func NewReActAgent(cfg ReActConfig) *CapabilityAgent
```

**ReActConfig 结构：**

```go
type ReActConfig struct {
    Name           string          // Agent 名称
    SystemPrompt   string          // 系统提示词
    PromptTemplate *PromptTemplate // 提示词模板（可选）
    Model          llm.Provider    // LLM Provider
    MaxTurns       int             // 最大迭代次数（默认 50）
    Temperature    float64         // 温度参数
    SessionID      string          // 会话 ID
    Lifecycle      *Lifecycle      // 生命周期管理器（默认自动创建）
    Logger         *slog.Logger    // 日志（默认 slog.Default()）
}
```

**示例：**

```go
agent := ap.NewReActAgent(ap.ReActConfig{
    Name:         "my-agent",
    SystemPrompt: "你是一个助手",
    Model:        provider,
    MaxTurns:     10,
}).WithMemory(mem).WithToolkit(registry)
```

## Functional Options

`NewAgent` 接受以下 Option 函数注入能力：

### 标量配置

| Option | 说明 |
|--------|------|
| `WithMaxTurns(n int)` | 设置最大迭代次数（默认 50） |
| `WithTemperature(t float64)` | 设置 LLM 温度参数 |
| `WithSessionID(id string)` | 设置会话 ID，用于跨轮记忆关联 |
| `WithPromptTemplate(t *PromptTemplate)` | 设置提示词模板 |

### 能力注入

| Option | 说明 |
|--------|------|
| `WithMemory(m MemoryStore)` | 注入记忆存储 |
| `WithToolkit(r *tools.Registry)` | 注入工具注册表 |
| `WithHooks(h Hooks)` | 注入 Hook 管理器 |
| `WithRAG(cfg RAGConfig)` | 注入 RAG 检索配置 |
| `WithTracer(t Tracer)` | 注入分布式追踪器 |
| `WithCostTracker(ct *CostTracker)` | 注入成本追踪器 |
| `WithContextWindow(cw ContextWindowStrategy)` | 注入上下文窗口裁剪策略 |
| `WithEvents(ep EventPublisher)` | 注入事件发布器 |
| `WithMetrics(m MetricsRecorder)` | 注入指标记录器 |
| `WithCheckpointStore(cs persist.CheckpointStore)` | 注入检查点存储 |
| `WithSummarizer(s memory.SummaryExtractor)` | 注入摘要提取器 |
| `WithFileScope(scopes []string)` | 注入文件作用域限制 |
| `WithCache(cache llm.LLMCache)` | 注入 LLM 缓存 |
| `WithHITL(cfg HITLConfig)` | 注入人机协作配置 |

## CapabilityAgent 链式 API

`CapabilityAgent` 实现所有 Capable 接口，提供链式 API 按需组合能力：

```go
agent := ap.NewReActAgent(ap.ReActConfig{
    Name: "capable-agent", Model: provider, MaxTurns: 10,
}).
    WithMemory(mem).              // 添加记忆
    WithRAG(ragCfg).              // 添加 RAG
    WithToolkit(registry).        // 添加工具
    WithHooks(hooks).             // 添加钩子
    WithEvents(eventBus).         // 添加事件
    WithMetrics(metricsRecorder). // 添加指标
    WithTracer(tracer).           // 添加追踪
    WithCostTracker(costTracker)  // 添加成本追踪
```

## 核心类型

### Message

```go
type Message struct {
    Role         Role          `json:"role"`           // system / user / assistant / tool
    Content      string        `json:"content"`
    ContentParts []ContentPart `json:"content_parts,omitempty"` // 多模态内容
    ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
    Metadata     Metadata      `json:"metadata,omitempty"`
}

// 消息构造辅助函数
func UserMessage(content string) Message
func SystemMessage(content string) Message
```

### Response

```go
type Response struct {
    RequestID string     `json:"request_id"`
    Content   string     `json:"content"`
    ToolCalls []ToolCall `json:"tool_calls,omitempty"`
    Usage     Usage      `json:"usage"`
    Metrics   Metrics    `json:"metrics"`
    Error     error      `json:"-"`
}

// ErrorCode 返回结构化错误码
func (r *Response) ErrorCode() string
```

### Usage

```go
type Usage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}
```

### ToolCall

```go
type ToolCall struct {
    ID   string `json:"id"`
    Name string `json:"name"`
    Args string `json:"args"` // JSON-encoded arguments
}
```

### StreamEvent

```go
type StreamEvent struct {
    Type    StreamEventType `json:"type"`    // token / thought / tool_call / tool_result / complete / error
    Content string           `json:"content"`
    Data    any              `json:"data,omitempty"`
}
```

## 协议式微内核（17 个 Capable 接口）

引擎通过类型断言自动发现 Agent 实现了哪些 Capable 接口，从而启用对应能力：

```go
type MemoryCapable interface { GetMemoryStore() MemoryStore }
type RAGCapable interface { GetRAGConfig() *RAGConfig }
type HITLCapable interface { GetHITLConfig() *HITLConfig }
type HookCapable interface { GetHooks() Hooks }
type TraceCapable interface { GetTracer() Tracer }
type CostCapable interface { GetCostTracker() *CostTracker }
type ContextWindowCapable interface { GetContextWindowStrategy() ContextWindowStrategy }
type EventCapable interface { GetEventPublisher() EventPublisher }
type MetricsCapable interface { GetMetricsRecorder() MetricsRecorder }
type CheckpointCapable interface { GetCheckpointStore() persist.CheckpointStore }
type SummarizerCapable interface { GetSummarizer() memory.SummaryExtractor }
type FileScopeCapable interface { GetFileScope() []string }
type CacheCapable interface { GetCache() llm.LLMCache }
type ToolkitCapable interface { GetToolkit() *tools.Registry }
type PlanningCapable interface { GetPlanner() planning.Planner }
type ReflectionCapable interface { GetReflector() reflection.Reflector }
type ToolLearningCapable interface { GetToolLearner() tool_learning.ToolLearner }
```

## RAG 配置

```go
type RAGConfig struct {
    Provider         RAGProvider  // RAG 检索提供者
    Mode             RAGMode      // 注入模式: auto / first / on_demand
    TopK             int          // 返回结果数（默认 5）
    MinScore         float32      // 最低相关度阈值（默认 0.3）
    ContextTemplate  string       // 上下文注入模板
}

type RAGProvider interface {
    Search(ctx context.Context, query string, topK int) ([]*RAGDocument, error)
}

type RAGDocument struct {
    ID      string  `json:"id"`
    Content string  `json:"content"`
    Score   float32 `json:"score"`
    Source  string  `json:"source,omitempty"`  // "fts" / "vector"
    Role    string  `json:"role,omitempty"`
}
```

## Hook 系统

20+ 钩子点，按阶段执行：

| 类别 | 钩子点 |
|------|--------|
| 生命周期 | `before_run`, `after_run`, `before_shutdown`, `after_shutdown` |
| 执行 | `before_turn`, `after_turn`, `on_complete`, `on_error` |
| LLM | `before_llm`, `after_llm` |
| 工具 | `before_tool`, `after_tool`, `before_tool_parse`, `after_tool_parse` |
| 记忆 | `before_rag`, `after_rag`, `before_memory_read/write`, `after_memory_read/write` |
| 流式 | `on_stream`, `on_stream_start`, `on_stream_end` |
| 编排 | `before_pipeline_step`, `before_handoff`, `before_dag_node` 等 |
| 上下文 | `context_window_update`, `context_window_full` |

```go
hooks := ap.NewHooks()
hooks.Register(ap.HookBeforeTurn, func(ctx *ap.HookContext) error {
    log.Printf("Turn %d 开始", ctx.Turn)
    return nil
})

agent := ap.NewReActAgent(cfg).WithHooks(hooks)
```

## 编排模式

| 模式 | 类型 | 说明 |
|------|------|------|
| **Pipeline** | `Pipeline` | 顺序执行，前一个输出作为后一个输入 |
| **Handoff** | `Handoff` | Agent 间动态交接，Router 函数路由 |
| **Parallel** | `ParallelRun()` | 并行执行，同一输入发给多个 Agent |
| **DAG** | `DAGWorkflow` | 有向无环图，支持条件边、重试、并行 |
| **GroupChat** | `GroupChat` | 多 Agent 对话，支持多种发言选择器 |

### Pipeline 示例

```go
pipeline := ap.NewPipeline(agent1, agent2, agent3)
resp, err := pipeline.Run(ctx, ap.UserMessage("分析数据"))
```

### DAG 示例

```go
dag, _ := ap.NewDAGBuilder("data-analysis").
    Node("collect", collectFn).
    Node("analyze", analyzeFn).
    Node("report", reportFn).
    Edge("collect", "analyze").
    Edge("analyze", "report").
    Build()

result, _ := dag.Run(ctx, "分析销售数据")
```

## 生命周期

```
Idle → Running → [Paused | WaitingForInput | Completed | Failed | Cancelled]
Paused → Running | Cancelled
WaitingForInput → Running | Cancelled | Failed
Completed/Failed/Cancelled → Idle (Reset)
```

```go
agent.Pause()                 // 暂停
agent.Resume()                // 恢复
agent.Stop()                  // 停止
agent.GracefulShutdown(ctx)   // 优雅关闭
agent.ResumeFromCheckpoint(ctx) // 从检查点恢复
```

## AgentStats

```go
type AgentStats struct {
    Status      AgentStatus       // 当前状态
    Turns       int               // 总轮次
    ToolsCalled map[string]int    // 工具调用次数
    StartTime   time.Time         // 启动时间
    Duration    time.Duration     // 运行时长
}
```

## 完整示例

=== "Go"

    ```go
    package main

    import (
        "context"
        "log"
        "os"

        ap "agentprimordia/pkg"
    )

    func main() {
        provider := ap.NewOpenAIProvider(ap.Config{
            APIKey: os.Getenv("OPENAI_API_KEY"),
            Model:  "gpt-4o",
        })

        // 创建 Agent（推荐入口）
        agent, err := ap.NewAgent("my-agent", "你是一个智能助手", provider,
            ap.WithMaxTurns(10),
        )
        if err != nil {
            log.Fatal(err)
        }

        // 运行
        resp, err := agent.Run(context.Background(), ap.UserMessage("你好！"))
        if err != nil {
            log.Fatal(err)
        }

        log.Printf("回复: %s", resp.Content)
        log.Printf("Token: %d", resp.Usage.TotalTokens)
    }
    ```

=== "TypeScript"

    ```typescript
    import { ReActAgent, OpenAIProvider } from '@agentprimordia/sdk';

    const agent = new ReActAgent({
      name: 'my-agent',
      systemPrompt: '你是一个智能助手',
      model: new OpenAIProvider({
        apiKey: process.env.OPENAI_API_KEY!,
        model: 'gpt-4o',
      }),
      maxTurns: 10,
    });

    const resp = await agent.run('你好！');
    console.log(`回复: ${resp.content}`);
    console.log(`Token: ${resp.usage.totalTokens}`);
    ```
