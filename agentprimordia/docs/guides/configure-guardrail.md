# 配置护栏（Configure Guardrail）

本指南介绍如何为 Agent 配置输入/输出护栏规则。

## 基本配置

```go
import ap "agentprimordia/pkg"

// 创建护栏引擎
engine := ap.NewGuardrailEngine()

// 添加 Prompt 注入检测（优先级最高）
engine.AddRule(ap.NewInjectionRule())

// 添加 PII 脱敏规则
engine.AddRule(ap.NewPIIRule())

// 注入 Agent
agent := ap.NewReActAgent(cfg).WithGuardrail(engine)
```

## 自定义规则

实现 `Rule` 接口即可创建自定义规则：

```go
type MyRule struct{}

func (r *MyRule) Name() string { return "my-rule" }
func (r *MyRule) Priority() int { return 200 }

func (r *MyRule) Check(ctx context.Context, input string, cp ap.CheckPoint) (*ap.Result, error) {
    if containsForbidden(input) {
        return &ap.Result{
            RuleName: "my-rule",
            Action:   ap.ActionReject,
            Severity: ap.SeverityHigh,
            Message:  "包含禁止内容",
        }, nil
    }
    return &ap.Result{RuleName: "my-rule", Action: ap.ActionPass}, nil
}
```

## 主题约束

限制 Agent 只处理特定主题：

```go
topicRule := ap.NewTopicRule([]string{"编程", "技术", "科学"})
engine.AddRule(topicRule)
```

## 敏感词过滤

基于 Trie 树的高效敏感词匹配：

```go
trieRule := ap.NewTrieRule([]string{"敏感词1", "敏感词2"})
engine.AddRule(trieRule)
```

## 检查点说明

- `CheckInput`：在 Agent 处理用户输入前检查
- `CheckOutput`：在 Agent 返回响应前检查

规则按优先级降序执行，遇到 `ActionReject` 立即终止。
