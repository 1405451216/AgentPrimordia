# 配置护栏

本文介绍如何在 AgentPrimordia 中配置输入/输出护栏。

## 基础配置

```go
engine := guardrail.NewEngine()

// 提示注入检测
engine.AddRule(guardrail.NewInjectionRule())

// PII 检测并掩码
engine.AddRule(guardrail.NewPIIRule(guardrail.PIIMask))

// 主题白名单
engine.AddRule(guardrail.NewTopicRule(
    []string{"技术", "产品", "运维"}, // 允许主题
    nil,                               // 无黑名单
))
```

## 输入点拦截

```go
hooks := agent.Hooks{
    agent.HookBeforeThink: func(ctx context.Context, hctx *agent.HookContext) error {
        report, _ := engine.Check(hctx.Input.Content, guardrail.CheckInput)
        if !report.Passed {
            return fmt.Errorf("输入被护栏拦截: %s", report.Results[0].Message)
        }
        return nil
    },
}

agent := NewReActAgent(cfg).WithHooks(hooks)
```

## 输出点检查

```go
hooks := agent.Hooks{
    agent.HookAfterAct: func(ctx context.Context, hctx *agent.HookContext) error {
        report, _ := engine.Check(hctx.LastResponse.Content, guardrail.CheckOutput)
        if report.Action == guardrail.ActionSanitize {
            hctx.LastResponse.Content = report.Results[0].Sanitized
        }
        return nil
    },
}
```

## 组合多个规则

```go
engine.AddRule(
    guardrail.NewInjectionRule(),
    guardrail.NewPIIRule(guardrail.PIIFlag),
    guardrail.NewTopicRule(allowedTopics, blockedTopics),
    guardrail.NewTrieRule([]string{"敏感词1", "敏感词2"}),
)
```

## 自定义严重级别

```go
rule := guardrail.NewInjectionRule()
// 可通过 WithSeverity 设置默认严重级别
```

## 下一步

- 阅读 [输入输出护栏概念](../concepts/guardrail.md)
- 查看 [Guardrail API 参考](../api/guardrail.md)
