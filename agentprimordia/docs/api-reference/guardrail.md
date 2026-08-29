# Guardrail API Reference

## Engine

```go
func NewEngine() *Engine
func (e *Engine) AddRule(rule Rule)
func (e *Engine) Check(input string, point CheckPoint) (*Report, error)

// 便捷入口
func (e *Engine) CheckInput(input string) (*Report, error)
func (e *Engine) CheckOutput(output string) (*Report, error)
```

## Rule Interface

```go
type Rule interface {
    Name() string
    Priority() int  // 数值越大越先执行
    Check(input string, point CheckPoint) (*Result, error)
}
```

## Result

```go
type Result struct {
    RuleName  string
    Action    Action    // pass / reject / sanitize / flag
    Severity  Severity  // low / medium / high / critical
    Message   string
    Sanitized string         // 清理后的文本（Action=sanitize 时有效）
    Metadata  map[string]any
}
```

## Report

```go
type Report struct {
    Passed  bool
    Results []Result
    Action  Action  // 最终聚合动作
}
```

## 内置规则构造器

各规则通过 Config 结构注入动作/严重度/优先级（不设则用各规则默认值）：

```go
func NewPromptInjectionRule(config PromptInjectionConfig) *PromptInjectionRule // Prompt 注入检测（默认 PriorityCritical）
func NewPIIRule(config PIIRuleConfig) *PIIRule                                 // PII 脱敏（DetectPhone/DetectEmail/... 按需开启）
func NewOutputSafetyRule(config OutputSafetyConfig) *OutputSafetyRule          // 输出安全（可检测代码执行）
func NewTopicConstraintRule(config TopicConstraintConfig) *TopicConstraintRule // 主题约束（Mode 指定白/黑名单模式）
func NewSensitiveWordRule(config SensitiveWordConfig) *SensitiveWordRule       // 敏感词 Trie 匹配（Words 指定词表）
```

**示例：**

```go
engine := guardrail.NewEngine()
engine.AddRule(guardrail.NewPIIRule(guardrail.PIIRuleConfig{
    Action:      guardrail.ActionSanitize,
    Severity:    guardrail.SeverityHigh,
    DetectPhone: true,
    DetectEmail: true,
}))
report, err := engine.CheckInput("我的手机号是 13800138000")
```

## 优先级常量

```go
const (
    PriorityCritical = 1000  // 安全关键
    PriorityHigh     = 500   // 高优先级
    PriorityNormal   = 100   // 常规
    PriorityLow      = 0     // 最低
)
```
