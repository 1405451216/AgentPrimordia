# Guardrail API Reference

## Engine

```go
func NewEngine() *Engine
func (e *Engine) AddRule(rule Rule)
func (e *Engine) Check(ctx context.Context, input string, checkpoint CheckPoint) *Report
```

## Rule Interface

```go
type Rule interface {
    Name() string
    Priority() int  // 数值越大越先执行
    Check(ctx context.Context, input string, checkpoint CheckPoint) (*Result, error)
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

```go
func NewInjectionRule() *InjectionRule           // Prompt 注入检测
func NewPIIRule() *PIIRule                       // PII 脱敏
func NewOutputRule() *OutputRule                 // 输出安全
func NewTopicRule(allowed []string) *TopicRule   // 主题约束
func NewTrieRule(words []string) *TrieRule       // 敏感词 Trie 匹配
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
