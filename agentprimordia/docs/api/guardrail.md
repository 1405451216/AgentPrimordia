# Guardrail API

## Engine

```go
func NewEngine() *Engine
```

创建护栏引擎。

### 主要方法

| 方法 | 说明 |
|------|------|
| `AddRule(rule ...Rule)` | 添加规则 |
| `Check(input string, point CheckPoint) (*Report, error)` | 执行检查 |

## Rule 接口

```go
type Rule interface {
    Name() string
    Check(input string, point CheckPoint) (*Result, error)
}
```

## 内置规则

| 函数 | 说明 |
|------|------|
| `NewInjectionRule()` | 提示注入检测 |
| `NewPIIRule(action PIIAction)` | PII 检测 |
| `NewTopicRule(allow, block []string)` | 主题过滤 |
| `NewTrieRule(words []string)` | 敏感词 Trie |

## 类型

| 类型 | 说明 |
|------|------|
| `CheckPoint` | `input` / `output` |
| `Action` | `pass` / `reject` / `sanitize` / `flag` |
| `Severity` | `low` / `medium` / `high` / `critical` |
| `Result` | 单条规则结果 |
| `Report` | 综合检查报告 |
