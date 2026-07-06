# Guardrail API 参考

> `package ap` — 输入/输出护栏中间件。

## Guardrail 接口

```go
type Guardrail interface {
    // 检查输入（Agent Run 前）
    CheckInput(ctx context.Context, input string) error
    // 检查输出（Agent Run 后）
    CheckOutput(ctx context.Context, output string) error
}
```

## 内置护栏

### 注入检测

```go
type InjectionDetector struct {
    Model    string  // 检测专用 LLM（如 claude-3-haik）
    Threshold float64 // 概率阈值
}

func NewInjectionDetector() *InjectionDetector
```

检测 prompt injection、越狱尝试。

### PII 过滤

```go
type PIIFilter struct {
    Action PIIAction // mask / reject / redact
}

type PIIAction string
const (
    PIIMask   PIIAction = "mask"   // 138****1234
    PIIReject PIIAction = "reject" // 拒绝请求
)

func NewPIIFilter(action PIIAction) *PIIFilter
```

自动识别手机号、邮箱、身份证号。

### 主题边界

```go
type TopicBoundary struct {
    AllowedTopics []string
    FallbackMessage string
}

func NewTopicBoundary(topics []string) *TopicBoundary
```

限制 Agent 只能讨论指定主题。

### 内容过滤

```go
type ContentFilter struct {
    BlockedWords []string
    BlockedPatterns []regexp.Regexp
}

func NewContentFilter() *ContentFilter
```

敏感词 / 正则匹配过滤。

## 护栏管道

组合多个护栏：

```go
guardrail := ap.NewGuardrailPipeline(
    ap.NewInjectionDetector(),
    ap.NewPIIFilter(ap.PIIMask),
    ap.NewTopicBoundary([]string{"产品", "技术"}),
)

agent := ap.NewAgent(ap.AgentConfig{
    Guardrail: guardrail,
})
```

## 错误类型

```go
type InjectionDetectedError struct{ Score float64 }
type PIILeakedError struct{ Field string }
type TopicViolatedError struct{ Topic string }
```

客户端根据错误类型提示用户。
