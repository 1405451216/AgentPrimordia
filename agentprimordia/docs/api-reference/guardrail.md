# Guardrail API 参考

> `package guardrail` — 输入/输出护栏引擎与规则。

## Engine

```go
func NewEngine() *Engine
```

创建护栏引擎，支持 copy-on-write 快照无锁读取。

### 主要方法

| 方法 | 说明 |
|------|------|
| `AddRule(r Rule)` | 添加规则（nil 安全） |
| `Rules() []string` | 列出已注册规则名称 |
| `RuleCount() int` | 返回规则数量 |
| `Check(input string, point CheckPoint) (*Report, error)` | 执行检查 |
| `CheckInput(input string) (*Report, error)` | 输入检查快捷方法 |
| `CheckOutput(output string) (*Report, error)` | 输出检查快捷方法 |

## Rule 接口

```go
type Rule interface {
    Name() string
    Check(input string, point CheckPoint) (*Result, error)
}
```

## 核心类型

```go
type CheckPoint string

const (
    CheckInput  CheckPoint = "input"
    CheckOutput CheckPoint = "output"
)

type Action string

const (
    ActionPass     Action = "pass"      // 放行
    ActionReject   Action = "reject"    // 拒绝
    ActionSanitize Action = "sanitize"  // 清洗后放行
    ActionFlag     Action = "flag"      // 标记但放行
)

type Severity string

const (
    SeverityLow      Severity = "low"
    SeverityMedium   Severity = "medium"
    SeverityHigh     Severity = "high"
    SeverityCritical Severity = "critical"
)

type Result struct {
    RuleName  string
    Action    Action
    Severity  Severity
    Message   string
    Sanitized string
    Metadata  map[string]any
}

type Report struct {
    Passed  bool       // 是否通过（Pass / Flag 为 true）
    Results []Result   // 各规则结果（Reject 时提前终止）
    Action  Action     // 综合动作
}
```

## 内置规则

### PIIRule — PII 检测

```go
func NewPIIRule(config PIIRuleConfig) *PIIRule

type PIIRuleConfig struct {
    Action            Action
    Severity          Severity
    Priority          int       // 规则优先级，默认 PriorityHigh
    DetectPhone       bool      // 检测手机号
    DetectIDCard      bool      // 检测身份证
    DetectEmail       bool      // 检测邮箱
    DetectBankCard    bool      // 检测银行卡号
    DetectIPv4        bool      // 检测 IPv4 地址
    DetectPassport    bool      // 检测护照号
    DetectBankAccount bool      // 检测银行账号
    DetectSSN         bool      // 检测社会安全号
    DetectAPIKey      bool      // 检测 API Key
    DetectJWT         bool      // 检测 JWT
}
```

**示例：**

```go
engine.AddRule(guardrail.NewPIIRule(guardrail.PIIRuleConfig{
    Action:       guardrail.ActionSanitize,
    Severity:     guardrail.SeverityHigh,
    DetectPhone:  true,
    DetectEmail:  true,
    DetectIDCard: true,
}))
```

### PromptInjectionRule — 提示注入检测

```go
func NewPromptInjectionRule(config PromptInjectionConfig) *PromptInjectionRule

type PromptInjectionConfig struct {
    Action   Action
    Severity Severity
}
```

### SensitiveWordRule — 敏感词检测

```go
func NewSensitiveWordRule(config SensitiveWordConfig) *SensitiveWordRule

type SensitiveWordConfig struct {
    Words    []string  // 敏感词列表
    Action   Action
    Severity Severity
}
```

### TopicConstraintRule — 主题过滤

```go
func NewTopicConstraintRule(config TopicConstraintConfig) *TopicConstraintRule

type TopicConstraintConfig struct {
    Allow    []string  // 主题白名单
    Block    []string  // 主题黑名单
    Action   Action
    Severity Severity
}
```

### OutputSafetyRule — 输出安全

```go
func NewOutputSafetyRule(config OutputSafetyConfig) *OutputSafetyRule
```

### SanitizeRule — 内容清洗

```go
func NewSanitizeRule(cfg SanitizeConfig) *SanitizeRule
```

## 快速开始

=== "Go"

    ```go
    engine := guardrail.NewEngine()

    // 添加规则
    engine.AddRule(guardrail.NewPIIRule(guardrail.PIIRuleConfig{
        Action:       guardrail.ActionSanitize,
        Severity:     guardrail.SeverityHigh,
        DetectPhone:  true,
    }))
    engine.AddRule(guardrail.NewPromptInjectionRule(guardrail.PromptInjectionConfig{
        Action:   guardrail.ActionReject,
        Severity: guardrail.SeverityCritical,
    }))
    engine.AddRule(guardrail.NewSensitiveWordRule(guardrail.SensitiveWordConfig{
        Words:    []string{"违禁词"},
        Action:   guardrail.ActionReject,
        Severity: guardrail.SeverityHigh,
    }))

    // 检查输入
    report, err := engine.CheckInput("my phone is 13812345678")
    if !report.Passed {
        fmt.Println("拦截原因:", report.Results[0].Message)
    }
    ```

=== "TypeScript"

    ```typescript
    import { GuardrailEngine, PIIRule, PromptInjectionRule } from '@agentprimordia/sdk';

    const engine = new GuardrailEngine();

    engine.addRule(new PIIRule({
      action: 'sanitize',
      severity: 'high',
      detectPhone: true,
    }));

    engine.addRule(new PromptInjectionRule({
      action: 'reject',
      severity: 'critical',
    }));

    const report = engine.checkInput('my phone is 13812345678');
    if (!report.passed) {
      console.log('拦截原因:', report.results[0].message);
    }
    ```

## 与 Agent 集成

通过 Hook 在输入/输出点自动检查：

```go
hooks := ap.NewHooks()
hooks.Register(ap.HookBeforeTurn, func(ctx *ap.HookContext) error {
    report, _ := engine.CheckInput(ctx.Message.TextContent())
    if !report.Passed {
        return fmt.Errorf("输入被护栏拦截: %s", report.Results[0].Message)
    }
    return nil
})

agent := ap.NewReActAgent(cfg).WithHooks(hooks)
```

## 检查流程

```
输入 → CheckInput() → 遍历规则
    ↓
    Pass → 继续下一条
    Reject → 立即终止，返回拒绝
    Sanitize → 替换内容，继续
    Flag → 标记，继续
    ↓
全部通过 → Report{Passed: true}
```
