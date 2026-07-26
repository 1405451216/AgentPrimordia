# 自反思（Reflection）

Reflection 模块为 Agent 提供自我反思和纠错能力，实现"输出→批评→改进"的闭环。

## 核心接口

```go
type Reflector interface {
    // Reflect 对执行结果进行反思
    Reflect(ctx context.Context, input, output string) (*Reflection, error)
    // Critique 对输出进行批评和纠错
    Critique(ctx context.Context, output string) (*Critique, error)
    // Improve 基于反思结果改进输出
    Improve(ctx context.Context, output string, feedback *Critique) (string, error)
}
```

## 数据结构

- **Reflection**：反思结果，包含优势、弱项、建议和置信度
- **Critique**：批评结果，包含问题列表、严重程度和纠正建议
- **Severity**：`low` / `medium` / `high` / `critical`
- **Correction**：纠正建议，包含原文、修正和理由

## 与 ReAct 循环的集成

通过链式 API 注入 Agent，配合严重度阈值控制触发频率：

```go
cfg := ap.ReActConfig{
    ReflectionSeverityThreshold: "high", // 仅 high/critical 触发改进
}
agent := ap.NewReActAgent(cfg).WithReflection(reflector)
```

当 Critique 的严重程度达到阈值时，Agent 自动调用 `Improve()` 生成改进后的输出。
