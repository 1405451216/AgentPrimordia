# 编排层统一执行引擎设计

> 状态：已确认待实现  
> 阶段：AgentPrimordia 性能优化 - 第一阶段（编排层）

## 1. 背景与动机

当前 `internal/orchestration/orchestrator.go` 中，`Sequential`、`Parallel`、`DAG` 三种模式各自实现执行逻辑，导致以下性能与维护问题：

- **Parallel 模式**：为每个 step 创建一个 goroutine，无并发上限，step 数量大时会造成 goroutine 爆炸。
- **Parallel 模式重试**：重试时会再次向 `resultsCh` 写入结果，导致 `result.Steps` 被后续结果覆盖。
- **DAG 模式**：按拓扑顺序串行执行，同一层级中没有依赖关系的节点未能并行化。
- **重试机制**：`time.Sleep` 直接阻塞执行 goroutine，降低有效并发。
- **代码重复**：三种模式的并发控制、重试、取消逻辑分散，难以统一优化。

本设计提出在编排层内部引入统一的执行引擎，将“编排策略”与“执行机制”解耦。

## 2. 目标

- 统一 `Sequential / Parallel / DAG` 的执行路径，收敛并发控制、重试、取消逻辑。
- 使用固定 worker 池替代无限制 goroutine，降低高并发场景下的内存与调度开销。
- 支持 DAG 同层节点并行执行，减少端到端延迟。
- 将重试逻辑从 worker 中移出，避免阻塞执行 goroutine。
- 保持 `Orchestrator` 公共 API 不变，实现内部重构的透明替换。

## 3. 非目标

- 不新增对外公共接口或修改 `OrchestratorConfig`、`AgentStep`、`ExecutionResult` 等已有结构。
- 不改动 `Pipeline`、`Handoff`、`Collaboration` 等其它编排组件（可在后续阶段借鉴本引擎）。
- 不引入分布式调度或持久化任务队列，仍保持单进程内存内执行。

## 4. 架构 overview

```text
Orchestrator.Execute
        │
        ▼
ExecutionEngine.Run(ctx, mode, steps, edges, initialInput)
        │
        ├── PlanBuilder.Build(mode, steps, edges) ──► ExecutionPlan
        │
        └── Scheduler.Run(ctx, plan, initialInput)
                │
                ├── 派发就绪 step 到 WorkerPool
                │
                └── 循环读取 results 通道
                        ├── 更新依赖图
                        ├── 触发下游就绪 step
                        └── 失败时重试或中止
```

## 5. 新增文件与模块边界

所有新增类型位于 `internal/orchestration/` 包内，均为内部实现。

| 文件 | 职责 |
|---|---|
| `engine.go` | `ExecutionEngine` 对外入口，封装 PlanBuilder + Scheduler + WorkerPool 生命周期 |
| `plan.go` | `ExecutionPlan`、`StepNode`、`DependencyGraph` 数据结构 |
| `plan_builder.go` | `PlanBuilder`，将不同 mode 转换为统一的 `ExecutionPlan` |
| `scheduler.go` | `Scheduler`，单 goroutine 维护执行状态与派发 |
| `worker_pool.go` | `WorkerPool`，固定数量 worker goroutine 执行 step |
| `step_executor.go` | `StepExecutor` 接口与默认实现（封装 `Agent.Run`） |
| `result_collector.go` | 收集 step 结果并生成 `ExecutionResult` / `ExecutionMetrics` |

## 6. 核心接口与数据结构

```go
// StepExecutor 执行单个 step。
type StepExecutor interface {
    Execute(ctx context.Context, step *AgentStep, input map[string]any) *StepResult
}

// ExecutionPlan 是统一执行计划。
type ExecutionPlan struct {
    Mode     OrchestratorMode
    Steps    []*StepNode
    DepGraph *DependencyGraph
}

// StepNode 是计划中的执行节点。
type StepNode struct {
    Step   *AgentStep
    Status StepStatus
    Result *StepResult
}

// DependencyGraph 维护 step 间的依赖关系。
type DependencyGraph struct {
    inDegree map[string]int      // stepID -> 未完成的依赖数
    outEdges map[string][]string // stepID -> 下游 stepID 列表
}

// Scheduler 负责状态流转与派发。
type Scheduler struct {
    plan           *ExecutionPlan
    pool           *WorkerPool
    maxConcurrency int
    retryPolicy    RetryPolicy
    results        chan *StepResult
    done           chan struct{}
}

// WorkerPool 使用固定 worker goroutine 执行 step。
type WorkerPool struct {
    workers  int
    tasks    chan workerTask
    executor StepExecutor
    wg       sync.WaitGroup
}

type workerTask struct {
    ctx   context.Context
    node  *StepNode
    input map[string]any
}
```

## 7. 关键流程

### 7.1 计划构建

- `Sequential`：每个 step 依赖前一个 step，形成链式图。
- `Parallel`：所有 step 入度为 0，互相独立。
- `DAG`：根据 `dagEdges` 构建依赖图，并通过拓扑排序验证无环。

### 7.2 Scheduler 主循环

1. 初始化所有 `StepNode` 为 `StepPending`。
2. 收集入度为 0 的节点作为初始就绪集合。
3. 只要存在就绪节点且运行中节点数 < `maxConcurrency`，即向 `WorkerPool` 提交任务。
4. 阻塞读取 `results` 通道：
   - 成功：标记 `StepCompleted`，合并输出，将下游节点入度减 1；若下游入度变为 0，加入就绪集合。
   - 失败：根据重试策略决定重新入队或标记 `StepFailed`；若策略为 `fail_fast`，停止派发新任务。
5. 当所有节点完成或失败，且运行中节点数为 0 时，生成 `ExecutionResult` 并返回。

### 7.3 WorkerPool 执行

- `NewWorkerPool(workers, executor)` 启动固定数量 goroutine。
- 每个 worker 从 `tasks` 通道读取任务，调用 `executor.Execute`。
- 执行完成后将结果写入 `results` 通道，不处理重试或状态更新。
- `Stop()` 关闭 `tasks` 通道并 `wg.Wait()` 等待 worker 退出。

## 8. 错误处理与重试

| 策略 | 行为 |
|---|---|
| `fail_fast` | 任意 step 失败后，scheduler 停止派发新任务，等待已运行 step 结束，返回失败 |
| `continue_on_error` | step 失败后标记为 `StepFailed`，其下游 step 标记为 `StepSkipped`，其余分支继续执行 |
| `retry` | scheduler 在失败后按 backoff 重新入队；重试次数达到上限后再按上述策略处理 |

重试逻辑在 scheduler 内完成，不阻塞 worker goroutine。backoff 可通过 `time.After` 或简单延迟实现。

## 9. 与现有 Orchestrator 的集成

`Orchestrator.Execute` 内部改为委托给 `ExecutionEngine`：

```go
func (o *Orchestrator) Execute(ctx context.Context, initialInput map[string]any) (*ExecutionResult, error) {
    engine := NewExecutionEngine(ExecutionEngineConfig{
        MaxConcurrency: o.getMaxConcurrency(),
        RetryPolicy:    o.getRetryPolicy(),
    })
    return engine.Run(ctx, o.config.Mode, o.steps, o.dagEdges, initialInput)
}
```

`executeSequential`、`executeParallel`、`executeDAG` 可保留为私有方法作为兼容层，内部统一调用 `ExecutionEngine`，降低代码 diff 风险。

## 10. 并发与资源控制

- Scheduler 为单 goroutine，所有 `StepNode` 状态变更串行发生，无需对 `ExecutionPlan` 加锁。
- WorkerPool 固定 worker 数等于 `MaxConcurrency`，避免 goroutine 数量随 step 数增长。
- Context 取消时，scheduler 停止派发；worker 通过 `ctx.Done()` 感知并提前退出。

## 11. 测试策略

| 测试类型 | 覆盖点 |
|---|---|
| 单元测试 | `DependencyGraph` 构建、入度更新、下游触发 |
| 单元测试 | `Scheduler` 状态机：`pending → running → completed/failed` |
| 单元测试 | `WorkerPool` 固定并发、优雅停止、context 取消 |
| 集成测试 | `ExecutionEngine` 对 Sequential/Parallel/DAG 的正确性 |
| 基准测试 | 100/1000 step 的 Parallel/DAG 调度开销、goroutine 数量、内存分配 |
| 回归测试 | 与现有 `orchestrator_test.go` 用例结果保持一致 |

## 12. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 重构引入执行顺序或结果归并 bug | 保留完整回归测试，逐步替换并对比输出 |
| WorkerPool 固定并发在某些场景下降低吞吐 | `MaxConcurrency` 可配置，默认值保持与当前行为一致 |
| Scheduler 单 goroutine 在超大规模图下成为瓶颈 | 当前场景 step 数为千级以内，单 goroutine 状态更新开销可忽略；未来可再拆分 |

## 13. 后续扩展

- 将统一执行引擎复用到 `Pipeline`、`Collaboration`、`Handoff` 等其它编排组件。
- 在 engine 层接入 `metrics` 与 `otel`，输出每 step 耗时、队列等待时间、并发利用率等指标。
- 支持更复杂的动态图（条件分支、循环），可向 `PlanBuilder` 扩展新的 plan 类型。
