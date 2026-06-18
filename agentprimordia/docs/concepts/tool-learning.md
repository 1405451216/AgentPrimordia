# 工具学习

Tool Learning 模块让 Agent 能够记录工具使用的成功/失败经验，形成最佳实践并用于后续调参建议。

## 核心接口

```go
type ToolLearner interface {
    RecordSuccess(ctx context.Context, toolName string, args, result string) error
    RecordFailure(ctx context.Context, toolName string, args, errorMsg string) error
    GetBestPractices(ctx context.Context, toolName string) ([]BestPractice, error)
    SuggestImprovement(ctx context.Context, toolName string, args string) (*Suggestion, error)
}
```

## 数据结构

```go
type BestPractice struct {
    ToolName    string    // 工具名
    Pattern     string    // 成功模式
    Description string    // 描述
    SuccessRate float64   // 成功率
    Examples    []string  // 示例
    CreatedAt   time.Time // 创建时间
}

type Suggestion struct {
    OriginalArgs string  // 原始参数
    ImprovedArgs string  // 建议参数
    Reason       string  // 理由
    Confidence   float64 // 置信度
}
```

## 快速开始

```go
learner := tool_learning.NewMemoryToolLearner(memoryStore)

// 记录成功
_ = learner.RecordSuccess(ctx, "search", `{"query":"Go 并发"}`, "找到 10 条结果")

// 记录失败
_ = learner.RecordFailure(ctx, "search", `{"query":""}`, "query 不能为空")

// 获取最佳实践
practices, _ := learner.GetBestPractices(ctx, "search")
for _, p := range practices {
    fmt.Printf("模式: %s, 成功率: %.2f\n", p.Pattern, p.SuccessRate)
}

// 获取改进建议
sug, _ := learner.SuggestImprovement(ctx, "search", `{"query":""}`)
fmt.Println("建议:", sug.ImprovedArgs)
```

## 与 ReActAgent 集成

```go
agent := NewReActAgent(cfg).
    WithToolkit(toolkit).
    WithToolLearner(learner)
```

引擎在工具调用成功/失败时自动调用 learner 记录经验，并在构造工具参数前查询最佳实践。

## 使用场景

- 搜索引擎 Agent 学习更优的查询词
- API 调用 Agent 学习参数组合
- 代码工具 Agent 积累正确用法

## 下一步

- 了解 [记忆系统](memory.md)
- 查看 [添加工具指南](../guides/add-tools.md)
