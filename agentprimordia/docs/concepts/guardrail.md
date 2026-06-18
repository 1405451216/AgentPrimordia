# 输入输出护栏

Guardrail 模块为 Agent 提供输入/输出内容安全检查，防止提示注入、敏感信息泄露、敏感词和主题越界。

## 核心模型

```go
// 检查点：输入或输出
type CheckPoint string
const (
    CheckInput  CheckPoint = "input"
    CheckOutput CheckPoint = "output"
)

// 动作：放行 / 拒绝 / 清洗 / 标记
type Action string
const (
    ActionPass     Action = "pass"
    ActionReject   Action = "reject"
    ActionSanitize Action = "sanitize"
    ActionFlag     Action = "flag"
)
```

## 内置规则

| 规则 | 检测内容 | 典型动作 |
|------|----------|----------|
| `InjectionRule` | 提示注入、越狱指令 | Reject |
| `PIIRule` | 身份证号、手机号、邮箱等 | Sanitize / Flag |
| `TopicRule` | 主题白名单/黑名单 | Reject |
| `TrieRule` | 敏感词 Trie 树 | Reject / Sanitize |

## 快速开始

```go
engine := guardrail.NewEngine()
engine.AddRule(guardrail.NewInjectionRule())
engine.AddRule(guardrail.NewPIIRule(guardrail.PIIMask))
engine.AddRule(guardrail.NewTopicRule([]string{"技术", "产品"}, nil))

report, err := engine.Check(ctx, "用户输入内容", guardrail.CheckInput)
if err != nil {
    panic(err)
}

if !report.Passed {
    fmt.Println("拒绝原因:", report.Results[0].Message)
}
```

## 自定义规则

实现 `Rule` 接口即可：

```go
type MyRule struct{}

func (r *MyRule) Name() string { return "my-rule" }

func (r *MyRule) Check(input string, point guardrail.CheckPoint) (*guardrail.Result, error) {
    if strings.Contains(input, "禁止词") {
        return &guardrail.Result{
            RuleName: r.Name(),
            Action:   guardrail.ActionReject,
            Severity: guardrail.SeverityHigh,
            Message:  "包含禁止词",
        }, nil
    }
    return &guardrail.Result{RuleName: r.Name(), Action: guardrail.ActionPass}, nil
}
```

## 与 ReActAgent 集成

通过 Hook 在输入/输出点检查：

```go
hooks := agent.Hooks{
    agent.HookBeforeThink: func(ctx context.Context, hctx *agent.HookContext) error {
        report, _ := engine.Check(ctx, hctx.Input.Content, guardrail.CheckInput)
        if !report.Passed {
            return fmt.Errorf("输入被护栏拦截: %s", report.Results[0].Message)
        }
        return nil
    },
}

agent := NewReActAgent(cfg).WithHooks(hooks)
```

## 性能优化

Guardrail Engine 使用 copy-on-write 快照，Check hot-path 无锁读取，适合高并发场景。

## 下一步

- 查看 [配置护栏指南](../guides/configure-guardrail.md)
- 阅读 [Guardrail API 参考](../api/guardrail.md)
