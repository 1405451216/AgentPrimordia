# 事件总线

Events 模块提供内部事件总线，实现 Agent、Pool、工具、LLM 等组件之间的解耦通信。

## 核心模型

```go
type EventType string

const (
    EventAgentStart   EventType = "agent.start"
    EventAgentStop    EventType = "agent.stop"
    EventAgentPanic   EventType = "agent.panic"
    EventAgentError   EventType = "agent.error"
    EventAgentResume  EventType = "agent.resume"
    EventTurnStart    EventType = "turn.start"
    EventTurnEnd      EventType = "turn.end"
    EventToolCall     EventType = "tool.call"
    EventToolResult   EventType = "tool.result"
    EventLLMCall      EventType = "llm.call"
    EventLLMResponse  EventType = "llm.response"
    EventPoolDispatch EventType = "pool.dispatch"
    EventPoolComplete EventType = "pool.complete"
    WildcardEvent     EventType = "*"
)

type Event struct {
    ID        string
    Type      EventType
    Source    string
    Timestamp time.Time
    Payload   any
}
```

## 快速开始

```go
bus := events.NewBus(events.WithBufferSize(64))

// 订阅工具调用事件
sub := bus.Subscribe(events.EventToolCall)
go func() {
    for ev := range sub.Ch {
        fmt.Printf("工具调用: %+v\n", ev.Payload)
    }
}()

// 发布事件
_ = bus.Publish(ctx, events.Event{
    Type:   events.EventToolCall,
    Source: "my-agent",
    Payload: map[string]any{"tool": "search"},
})
```

## 通配符订阅

```go
sub := bus.Subscribe(events.WildcardEvent)
for ev := range sub.Ch {
    fmt.Println(ev.Type, ev.Source)
}
```

## 与 ReActAgent 集成

```go
agent := NewReActAgent(cfg).WithEvents(bus)
```

注入 EventCapable 后，ReAct 引擎会在关键生命周期点自动发布事件。

## 性能特性

- Publish hot-path 无锁读取订阅者快照
- copy-on-write 保证订阅者列表一致性
- 支持自定义 buffer size

## 下一步

- 了解 [Pool 调度](pool.md)
- 查看 [Prometheus 指标](advanced/metrics.md)
