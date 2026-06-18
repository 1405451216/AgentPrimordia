# 任务规划

Planning 模块让 Agent 能够将复杂任务分解为可执行的子任务，并生成带依赖关系的执行计划。

## 核心接口

```go
type Planner interface {
    Decompose(ctx context.Context, task string) ([]SubTask, error)
    GeneratePlan(ctx context.Context, task string) (*Plan, error)
}
```

## 数据结构

```go
type SubTask struct {
    ID          string     // 子任务标识
    Description string     // 描述
    DependsOn   []string   // 依赖的子任务 ID
    Status      TaskStatus // 状态
    Result      string     // 执行结果
}

type Plan struct {
    Goal      string    // 目标
    SubTasks  []SubTask // 子任务列表
    CreatedAt time.Time // 创建时间
}
```

## 快速开始

```go
planner := planning.NewLLMPlanner(provider)

subtasks, err := planner.Decompose(ctx, "开发一个支持用户登录的 Web 应用")
if err != nil {
    panic(err)
}

for _, st := range subtasks {
    fmt.Printf("%s: %s (依赖: %v)\n", st.ID, st.Description, st.DependsOn)
}
```

## 与 ReActAgent 集成

```go
agent := NewReActAgent(cfg).WithPlanner(planner)
```

注入 PlanningCapable 后，ReAct 引擎在面对复杂输入时会先调用 planner 生成计划，再按步骤执行。

## 与 DAG 编排结合

```go
plan, _ := planner.GeneratePlan(ctx, task)

dag := NewDAGWorkflow()
for _, st := range plan.SubTasks {
    dag.AddNode(st.ID, agentForTask(st))
}
for _, st := range plan.SubTasks {
    for _, dep := range st.DependsOn {
        dag.AddEdge(dep, st.ID)
    }
}
result := dag.Run(ctx, plan.Goal)
```

## 下一步

- 了解 [多 Agent 编排](orchestration.md)
- 查看 [自反思](reflection.md)
