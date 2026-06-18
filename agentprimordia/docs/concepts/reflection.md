# 自反思

Reflection 模块让 Agent 能够对自身输出进行反思、批评和改进，提升输出质量并自动纠错。

## 核心接口

```go
type Reflector interface {
    Reflect(ctx context.Context, input, output string) (*Reflection, error)
    Critique(ctx context.Context, output string) (*Critique, error)
    Improve(ctx context.Context, output string, feedback *Critique) (string, error)
}
```

## 反思结果

```go
type Reflection struct {
    Strengths   []string // 优点
    Weaknesses  []string // 不足
    Suggestions []string // 改进建议
    Confidence  float64  // 置信度
}

type Critique struct {
    Issues      []Issue      // 问题列表
    Severity    Severity     // 严重程度
    Corrections []Correction // 纠正建议
}
```

## 快速开始

```go
reflector := reflection.NewLLMReflector(provider)

feedback, err := reflector.Critique(ctx, draftOutput)
if err != nil {
    panic(err)
}

if feedback.Severity == reflection.SeverityHigh {
    improved, err := reflector.Improve(ctx, draftOutput, feedback)
    fmt.Println("改进后:", improved)
}
```

## 与 ReActAgent 集成

通过 Hook 在 Action 后触发反思：

```go
hooks := agent.Hooks{
    agent.HookAfterAct: func(ctx context.Context, hctx *agent.HookContext) error {
        feedback, err := reflector.Critique(ctx, hctx.LastResponse.Content)
        if err == nil && feedback.Severity == reflection.SeverityHigh {
            // 触发重试或改进逻辑
        }
        return nil
    },
}

agent := NewReActAgent(cfg).WithReflector(reflector).WithHooks(hooks)
```

## 使用场景

- 代码审查 Agent 自动发现 Bug
- 写作 Agent 自我润色
- 决策 Agent 多轮自检

## 下一步

- 了解 [任务规划](planning.md) 与自反思的组合
- 查看 [工具学习](tool-learning.md)
