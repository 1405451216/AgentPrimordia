# 任务规划（Planning）

Planning 模块为 Agent 提供任务分解和执行计划生成能力，是 ReAct 循环的高层补充。

## 核心接口

```go
type Planner interface {
    // Decompose 将复杂任务分解为子任务列表
    Decompose(ctx context.Context, task string) ([]SubTask, error)
    // GeneratePlan 生成执行计划（包含依赖关系）
    GeneratePlan(ctx context.Context, task string) (*Plan, error)
}
```

## 数据结构

- **SubTask**：子任务，包含 ID、描述、依赖列表（`DependsOn`）、状态和结果
- **Plan**：执行计划，包含目标、子任务列表和创建时间
- **TaskStatus**：`pending` → `running` → `completed` / `failed`

## 内置实现

### LLMPlanner

使用 LLM 进行智能任务规划：

```go
planner := planning.NewLLMPlanner(llmProvider)

// 分解任务
subtasks, err := planner.Decompose(ctx, "构建一个 Web 应用")

// 生成带依赖关系的计划
plan, err := planner.GeneratePlan(ctx, "部署微服务架构")
```

LLMPlanner 通过结构化提示词引导 LLM 输出 JSON 格式的子任务列表，自动解析依赖关系。

## 与 ReAct 循环的集成

通过链式 API `WithPlanning()` 注入 Agent：

```go
agent := ap.NewReActAgent(cfg).WithPlanning(planner)
```

注入后，Agent 在处理复杂任务时会先调用 Planner 进行分解，再逐步执行子任务。
