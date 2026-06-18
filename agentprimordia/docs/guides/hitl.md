# 人机协作（HITL）

HITL（Human-in-the-Loop）让 Agent 在执行敏感操作前暂停并等待人类确认。

## 核心概念

```go
type HITLConfig struct {
    ConfirmToolCalls bool          // 工具调用前确认
    ConfirmPatterns  []string      // 需要确认的正则模式
    Timeout          time.Duration // 等待确认超时
}
```

## 快速开始

```go
agent := NewReActAgent(cfg).WithHITL(HITLConfig{
    ConfirmToolCalls: true,
    Timeout:          60 * time.Second,
})
```

## 自定义确认条件

```go
agent := NewReActAgent(cfg).WithHITL(HITLConfig{
    ConfirmPatterns: []string{
        `(?i)delete`,
        `(?i)drop\s+table`,
    },
    Timeout: 5 * time.Minute,
})
```

## 确认流程

1. Agent 准备调用工具
2. HITLManager 检查是否需要确认
3. 触发 `HITLConfirmNeeded` 事件
4. 人类确认或拒绝
5. Agent 继续执行或终止

## 与事件总线集成

```go
bus.Subscribe(events.EventHITLConfirmNeeded)
// 在事件处理器中展示确认 UI
```

## 下一步

- 了解 [事件总线](../concepts/events.md)
- 查看 [安全最佳实践](../advanced/security.md)
