# 工具学习（Tool Learning）

Tool Learning 模块让 Agent 从工具使用经验中自动学习，积累最佳实践并提供参数改进建议。

## 核心接口

```go
type ToolLearner interface {
    // RecordSuccess 记录工具成功使用经验
    RecordSuccess(ctx context.Context, toolName string, args, result string) error
    // RecordFailure 记录工具失败经验
    RecordFailure(ctx context.Context, toolName string, args, errorMsg string) error
    // GetBestPractices 获取工具最佳实践
    GetBestPractices(ctx context.Context, toolName string) ([]BestPractice, error)
    // SuggestImprovement 基于历史经验建议改进
    SuggestImprovement(ctx context.Context, toolName string, args string) (*Suggestion, error)
}
```

## 数据结构

- **BestPractice**：最佳实践，包含工具名、模式、描述、成功率和示例
- **Suggestion**：改进建议，包含建议内容和置信度（0-1）

## 工作原理

1. Agent 每次调用工具后，自动记录成功/失败经验到 MemoryStore
2. 累积足够样本后，提取高频成功模式作为 BestPractice
3. 当 Agent 再次调用同一工具时，`SuggestImprovement` 基于历史失败模式给出参数建议

## 与 ReAct 循环的集成

通过配置置信度阈值控制建议触发：

```go
cfg := ap.ReActConfig{
    ToolLearningConfidenceThreshold: 0.7, // 置信度 >= 0.7 才触发建议
}
agent := ap.NewReActAgent(cfg).WithToolLearning(learner)
```
