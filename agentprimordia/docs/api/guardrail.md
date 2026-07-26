# Guardrail API

Guardrail 提供输入/输出护栏引擎，在 Agent 处理前后执行安全规则检查。

## 核心类型

```go
// Engine 护栏引擎，管理规则集并执行检查
type Engine struct { /* ... */ }

// Rule 规则接口
type Rule interface {
    Name() string
    Priority() int
    Check(ctx context.Context, input string, checkpoint CheckPoint) (*Result, error)
}

// CheckPoint 检查点
const (
    CheckInput  CheckPoint = "input"
    CheckOutput CheckPoint = "output"
)

// Action 检查动作
const (
    ActionPass     Action = "pass"
    ActionReject   Action = "reject"
    ActionSanitize Action = "sanitize"
    ActionFlag     Action = "flag"
)
```

## 使用方式

```go
import ap "agentprimordia/pkg"

engine := ap.NewGuardrailEngine()
engine.AddRule(ap.NewInjectionRule())   // Prompt 注入检测
engine.AddRule(ap.NewPIIRule())         // PII 脱敏
engine.AddRule(ap.NewTopicRule(allowed)) // 主题过滤

report := engine.Check(ctx, userInput, ap.CheckInput)
if !report.Passed {
    // 拒绝或清理输入
}
```

## 内置规则

| 规则 | 优先级 | 说明 |
|------|--------|------|
| InjectionRule | 1000 | Prompt 注入检测 |
| PIIRule | 500 | 个人敏感信息脱敏 |
| OutputRule | 500 | 输出安全检查 |
| TopicRule | 100 | 主题约束过滤 |
| TrieRule | 100 | 基于 Trie 树的敏感词匹配 |

## 与 Agent 集成

通过 Hook 机制自动注入 ReAct 循环：

```go
agent := ap.NewReActAgent(cfg).WithGuardrail(engine)
```
