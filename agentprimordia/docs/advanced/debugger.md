# 调试器（Debugger）

Debugger 模块提供类似 LangSmith 的 Agent 调试和追踪功能，帮助开发者理解 Agent 的执行过程。

## 核心组件

### Inspector

执行追踪器，记录 Agent 运行时的 Span 树：

```go
inspector := debugger.NewInspector()

// 开始追踪
span := inspector.StartSpan("agent-run", "agent", sessionID)
span.SetAttribute("model", "gpt-4o")

// 子 Span
childSpan := inspector.StartSpanWithParent("llm-call", "llm", sessionID, span.ID)
childSpan.End()
span.End()
```

### TraceSpan

每个 Span 包含：
- ID / ParentID / TraceID（支持父子关系）
- Kind：`agent` / `llm` / `tool` / `memory`
- Status：`started` / `completed` / `failed`
- Token 统计（PromptTokens / CompletionTokens）
- 自定义 Attributes 和 Events

### InspectorServer

内置 HTTP 服务，暴露调试 API：

| 端点 | 说明 |
|------|------|
| `GET /debug/stats` | 获取统计信息 |
| `GET /debug/traces` | 获取追踪列表 |
| `GET /debug/sessions/{id}` | 获取会话追踪详情 |

## 使用方式

```go
inspector := debugger.NewInspector()
server := debugger.NewInspectorServer(inspector, ":8081")
go server.Start()

// 将 inspector 注入 Agent（通过 Tracer 接口）
agent, err := ap.NewAgent("debug-agent", "你是助手", provider,
    ap.WithTracer(inspector),
)
```
