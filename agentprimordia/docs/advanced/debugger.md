# 调试与 Inspector

AgentPrimordia 提供 Inspector 组件，类似 LangSmith，用于追踪 Agent 执行过程、查看会话详情和可视化编排。

## Inspector 核心概念

```go
type Inspector struct {
    traces   []*TraceSpan
    sessions map[string]*SessionTrace
    maxSpans int
}

type TraceSpan struct {
    ID         string
    ParentID   string
    TraceID    string
    SessionID  string
    Name       string
    Kind       string   // agent, llm, tool, memory
    Status     string   // started, completed, failed
    StartTime  time.Time
    EndTime    time.Time
    Duration   time.Duration
    Attributes map[string]interface{}

    PromptTokens     int
    CompletionTokens int
    TotalTokens      int
}
```

## 启动 Inspector Server

```go
inspector := debugger.NewInspector(10000)

server := debugger.NewInspectorServer(inspector, debugger.ServerConfig{
    Addr: ":8081",
})

go server.Start()
```

## 查看追踪

浏览器访问 `http://localhost:8081`：

- 会话列表
- 单个会话的 Span 树
- Token 统计与成本估算
- 工具调用详情

## 与 ReActAgent 集成

```go
agent := NewReActAgent(cfg).WithTracer(inspector)
```

## 可视化编排

Inspector Server 同时提供 DAG 可视化编辑器，可将 `DAGWorkflow` 渲染为可交互图：

```go
visualizer := debugger.NewVisualizer(inspector)
visualizer.RenderDAG(dag)
```

## 下一步

- 查看 `cmd/example/inspector-demo/main.go`
- 了解 [OpenTelemetry 桥接](otel.md)
