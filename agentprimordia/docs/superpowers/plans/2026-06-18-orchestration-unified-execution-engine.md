# 编排层统一执行引擎实现计划

> **状态：已完成** ✅
> **完成日期：2026-06-22**

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `internal/orchestration` 中引入统一执行引擎，将 `Sequential / Parallel / DAG` 三种模式的并发控制、重试、取消逻辑收敛到同一套 Scheduler + WorkerPool 实现，从而消除 goroutine 爆炸、修复 retry 覆盖 bug、提升 DAG 同层并行度，并保持 `Orchestrator` 公共 API 不变。

**Architecture:** 新增 `ExecutionPlan`、`DependencyGraph`、`PlanBuilder`、`StepExecutor`、`WorkerPool`、`Scheduler`、`ExecutionEngine` 等内部类型；`Orchestrator.Execute` 委托给 `ExecutionEngine.Run`；Scheduler 单 goroutine 维护状态，WorkerPool 固定 goroutine 执行 step。

**Tech Stack:** Go 1.26+，标准库（context、sync、time），项目已有类型（`AgentStep`、`StepResult`、`ExecutionResult` 等）。

---

## 文件结构

| 文件 | 操作 | 说明 |
|---|---|---|
| `internal/orchestration/plan.go` | 创建 | `StepNode`、`DependencyGraph` |
| `internal/orchestration/plan_builder.go` | 创建 | `ExecutionPlan`、`BuildExecutionPlan` |
| `internal/orchestration/plan_builder_test.go` | 创建 | PlanBuilder 单元测试 |
| `internal/orchestration/step_executor.go` | 创建 | `StepExecutor` 接口与默认实现 |
| `internal/orchestration/step_executor_test.go` | 创建 | StepExecutor 单元测试 |
| `internal/orchestration/worker_pool.go` | 创建 | `WorkerPool` |
| `internal/orchestration/worker_pool_test.go` | 创建 | WorkerPool 单元测试 |
| `internal/orchestration/scheduler.go` | 创建 | `Scheduler` |
| `internal/orchestration/scheduler_test.go` | 创建 | Scheduler 单元测试 |
| `internal/orchestration/engine.go` | 创建 | `ExecutionEngine` |
| `internal/orchestration/engine_test.go` | 创建 | ExecutionEngine 集成测试 |
| `internal/orchestration/orchestrator.go` | 修改 | `Execute` 委托给 engine；提取辅助函数为包级函数 |
| `internal/orchestration/orchestrator_test.go` | 修改 | 保留并补充回归测试 |
| `internal/orchestration/bench_engine_test.go` | 创建 | 新引擎基准测试 |

---

## Task 1: DependencyGraph 与 StepNode

**Files:**
- Create: `internal/orchestration/plan.go`
- Test: `internal/orchestration/plan_test.go`

- [ ] **Step 1: 编写失败测试**

```go
package orchestration

import "testing"

func TestDependencyGraph_ReadyAndComplete(t *testing.T) {
    steps := []*AgentStep{
        {ID: "a"},
        {ID: "b"},
        {ID: "c"},
    }
    edges := []DAGEdge{
        {From: "a", To: "b"},
        {From: "b", To: "c"},
    }
    g, err := NewDependencyGraph(steps, edges)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !g.Ready("a") {
        t.Errorf("a should be ready")
    }
    if g.Ready("b") || g.Ready("c") {
        t.Errorf("b and c should not be ready yet")
    }

    ready := g.Complete("a")
    if len(ready) != 1 || ready[0] != "b" {
        t.Errorf("completing a should make b ready, got %v", ready)
    }
    if !g.Ready("b") {
        t.Errorf("b should now be ready")
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/orchestration/ -run TestDependencyGraph_ReadyAndComplete -v`
Expected: FAIL (`NewDependencyGraph` / `DependencyGraph` 未定义)

- [ ] **Step 3: 实现最小代码**

```go
package orchestration

type StepNode struct {
    Step   *AgentStep
    Status StepStatus
    Result *StepResult
}

type DependencyGraph struct {
    inDegree map[string]int
    outEdges map[string][]string
    inEdges  map[string][]string // stepID -> 上游 stepID 列表
}

func NewDependencyGraph(steps []*AgentStep, edges []DAGEdge) (*DependencyGraph, error) {
    g := &DependencyGraph{
        inDegree: make(map[string]int, len(steps)),
        outEdges: make(map[string][]string, len(steps)),
        inEdges:  make(map[string][]string, len(steps)),
    }
    for _, s := range steps {
        g.inDegree[s.ID] = 0
    }
    for _, e := range edges {
        if _, ok := g.inDegree[e.From]; !ok {
            return nil, fmt.Errorf("unknown step %q in edge", e.From)
        }
        if _, ok := g.inDegree[e.To]; !ok {
            return nil, fmt.Errorf("unknown step %q in edge", e.To)
        }
        g.inDegree[e.To]++
        g.outEdges[e.From] = append(g.outEdges[e.From], e.To)
        g.inEdges[e.To] = append(g.inEdges[e.To], e.From)
    }
    return g, nil
}

func (g *DependencyGraph) Ready(stepID string) bool {
    return g.inDegree[stepID] == 0
}

func (g *DependencyGraph) Complete(stepID string) []string {
    newlyReady := make([]string, 0)
    for _, next := range g.outEdges[stepID] {
        g.inDegree[next]--
        if g.inDegree[next] == 0 {
            newlyReady = append(newlyReady, next)
        }
    }
    return newlyReady
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/orchestration/ -run TestDependencyGraph_ReadyAndComplete -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/orchestration/plan.go internal/orchestration/plan_test.go
git commit -m "feat(orchestration): add DependencyGraph and StepNode for unified execution engine"
```

---

## Task 2: PlanBuilder

**Files:**
- Create: `internal/orchestration/plan_builder.go`
- Test: `internal/orchestration/plan_builder_test.go`

- [ ] **Step 1: 编写失败测试**

```go
package orchestration

import "testing"

func TestBuildExecutionPlan_Sequential(t *testing.T) {
    steps := []*AgentStep{
        {ID: "s1", Name: "step1"},
        {ID: "s2", Name: "step2"},
        {ID: "s3", Name: "step3"},
    }
    plan, err := BuildExecutionPlan(SequentialMode, steps, nil)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if plan.Mode != SequentialMode {
        t.Errorf("mode mismatch")
    }
    if !plan.DepGraph.Ready("s1") {
        t.Errorf("s1 should be ready")
    }
    if plan.DepGraph.Ready("s2") || plan.DepGraph.Ready("s3") {
        t.Errorf("s2/s3 should not be ready initially")
    }
}

func TestBuildExecutionPlan_Parallel(t *testing.T) {
    steps := []*AgentStep{
        {ID: "p1"},
        {ID: "p2"},
    }
    plan, err := BuildExecutionPlan(ParallelMode, steps, nil)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !plan.DepGraph.Ready("p1") || !plan.DepGraph.Ready("p2") {
        t.Errorf("all parallel steps should be ready")
    }
}

func TestBuildExecutionPlan_DAG(t *testing.T) {
    steps := []*AgentStep{
        {ID: "a"},
        {ID: "b"},
        {ID: "c"},
    }
    edges := []DAGEdge{{From: "a", To: "c"}, {From: "b", To: "c"}}
    plan, err := BuildExecutionPlan(DAGMode, steps, edges)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !plan.DepGraph.Ready("a") || !plan.DepGraph.Ready("b") {
        t.Errorf("a and b should be ready")
    }
    if plan.DepGraph.Ready("c") {
        t.Errorf("c should wait for a and b")
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/orchestration/ -run TestBuildExecutionPlan -v`
Expected: FAIL (`BuildExecutionPlan` 未定义)

- [ ] **Step 3: 实现最小代码**

```go
package orchestration

import "fmt"

type ExecutionPlan struct {
    Mode     OrchestratorMode
    Steps    []*StepNode
    DepGraph *DependencyGraph
}

func BuildExecutionPlan(mode OrchestratorMode, steps []*AgentStep, edges []DAGEdge) (*ExecutionPlan, error) {
    switch mode {
    case SequentialMode:
        return buildSequentialPlan(steps)
    case ParallelMode:
        return buildParallelPlan(steps)
    case DAGMode:
        return buildDAGPlan(steps, edges)
    default:
        return nil, fmt.Errorf("unsupported orchestrator mode: %s", mode)
    }
}

func buildSequentialPlan(steps []*AgentStep) (*ExecutionPlan, error) {
    edges := make([]DAGEdge, 0, len(steps)-1)
    for i := 1; i < len(steps); i++ {
        edges = append(edges, DAGEdge{From: steps[i-1].ID, To: steps[i].ID})
    }
    dg, err := NewDependencyGraph(steps, edges)
    if err != nil {
        return nil, err
    }
    return &ExecutionPlan{Mode: SequentialMode, Steps: nodesFromSteps(steps), DepGraph: dg}, nil
}

func buildParallelPlan(steps []*AgentStep) (*ExecutionPlan, error) {
    dg, err := NewDependencyGraph(steps, nil)
    if err != nil {
        return nil, err
    }
    return &ExecutionPlan{Mode: ParallelMode, Steps: nodesFromSteps(steps), DepGraph: dg}, nil
}

func buildDAGPlan(steps []*AgentStep, edges []DAGEdge) (*ExecutionPlan, error) {
    if _, err := topologicalSort(steps, edges); err != nil {
        return nil, fmt.Errorf("DAG validation failed: %w", err)
    }
    dg, err := NewDependencyGraph(steps, edges)
    if err != nil {
        return nil, err
    }
    return &ExecutionPlan{Mode: DAGMode, Steps: nodesFromSteps(steps), DepGraph: dg}, nil
}

func nodesFromSteps(steps []*AgentStep) []*StepNode {
    nodes := make([]*StepNode, len(steps))
    for i, s := range steps {
        nodes[i] = &StepNode{Step: s, Status: StepPending}
    }
    return nodes
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/orchestration/ -run TestBuildExecutionPlan -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/orchestration/plan_builder.go internal/orchestration/plan_builder_test.go
git commit -m "feat(orchestration): add PlanBuilder for sequential/parallel/DAG plans"
```

---

## Task 3: StepExecutor

**Files:**
- Create: `internal/orchestration/step_executor.go`
- Test: `internal/orchestration/step_executor_test.go`

- [ ] **Step 1: 编写失败测试**

```go
package orchestration

import (
    "context"
    "testing"

    "agentprimordia/internal/agent"
    "agentprimordia/internal/llm"
)

func TestDefaultStepExecutor_ExecutesAgent(t *testing.T) {
    mockLLM := llm.NewMockLLM(nil)
    mockLLM.WithResponse("hello")
    ag := agent.New(agent.Config{Name: "test"}, mockLLM)

    step := &AgentStep{ID: "s1", Name: "s1", Agent: ag, Prompt: "say hello"}
    exec := NewDefaultStepExecutor(nil)
    result := exec.Execute(context.Background(), step, nil)

    if result.Status != StepCompleted {
        t.Errorf("expected completed, got %s, error: %v", result.Status, result.Error)
    }
    if result.Output["content"] != "hello" {
        t.Errorf("unexpected output: %v", result.Output)
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/orchestration/ -run TestDefaultStepExecutor_ExecutesAgent -v`
Expected: FAIL (`StepExecutor` 未定义)

- [ ] **Step 3: 实现最小代码**

```go
package orchestration

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"
    "time"

    "agentprimordia/internal/agent"
)

// StepExecutor 执行单个 step。
type StepExecutor interface {
    Execute(ctx context.Context, step *AgentStep, input map[string]any) *StepResult
}

type defaultStepExecutor struct {
    eventCh chan<- *OrchestrationEvent
}

// NewDefaultStepExecutor 创建默认 step 执行器。
// eventCh 可为 nil，表示不发送事件。
func NewDefaultStepExecutor(eventCh chan<- *OrchestrationEvent) StepExecutor {
    return &defaultStepExecutor{eventCh: eventCh}
}

func (e *defaultStepExecutor) Execute(ctx context.Context, step *AgentStep, input map[string]any) *StepResult {
    startTime := time.Now()
    result := &StepResult{
        StepID:    step.ID,
        StepName:  step.Name,
        Status:    StepRunning,
        StartTime: startTime,
        Output:    make(map[string]any),
    }

    e.emitEvent("step_started", step.ID, map[string]any{"name": step.Name})

    if !stepConditionSatisfied(step, input) {
        result.Status = StepSkipped
        result.EndTime = time.Now()
        result.Duration = result.EndTime.Sub(startTime)
        return result
    }

    prompt := step.Prompt
    if prompt == "" {
        prompt = buildPromptFromInputs(input, step.InputFrom)
    }

    resp, err := step.Agent.Run(ctx, agent.UserMessage(prompt))

    result.EndTime = time.Now()
    result.Duration = result.EndTime.Sub(startTime)
    result.Response = resp

    if err != nil {
        result.Status = StepFailed
        result.Error = err
        e.emitEvent("step_failed", step.ID, map[string]any{"error": err.Error()})
        return result
    }

    result.Status = StepCompleted
    if resp.Content != "" {
        result.Output["content"] = resp.Content
    }
    if step.OutputKey != "" {
        result.Output[step.OutputKey] = resp.Content
    }
    if resp.Metrics.TotalTurns > 0 {
        result.Output["turns"] = resp.Metrics.TotalTurns
    }

    e.emitEvent("step_completed", step.ID, map[string]any{
        "duration": result.Duration,
        "turns":    resp.Metrics.TotalTurns,
    })
    return result
}

func (e *defaultStepExecutor) emitEvent(typ, stepID string, data any) {
    if e.eventCh == nil {
        return
    }
    select {
    case e.eventCh <- &OrchestrationEvent{Type: typ, Timestamp: time.Now(), StepID: stepID, Data: data}:
    default:
    }
}

// stepConditionSatisfied 检查 step 执行条件。
func stepConditionSatisfied(step *AgentStep, input map[string]any) bool {
    if step.Condition.Type == "" || step.Condition.Type == "always" {
        return true
    }

    value, exists := input[step.Condition.Field]
    if !exists {
        return false
    }

    switch step.Condition.Operator {
    case "==":
        return fmt.Sprintf("%v", value) == fmt.Sprintf("%v", step.Condition.Value)
    case "!=":
        return fmt.Sprintf("%v", value) != fmt.Sprintf("%v", step.Condition.Value)
    case "contains":
        strValue := fmt.Sprintf("%v", value)
        return strings.Contains(strings.ToLower(strValue), strings.ToLower(fmt.Sprintf("%v", step.Condition.Value)))
    case "empty":
        return value == nil || fmt.Sprintf("%v", value) == ""
    case "not_empty":
        return value != nil && fmt.Sprintf("%v", value) != ""
    default:
        return true
    }
}

// buildPromptFromInputs 从输入构建提示词。
func buildPromptFromInputs(input map[string]any, inputKeys []string) string {
    if len(inputKeys) == 0 {
        data, _ := json.MarshalIndent(input, "", "  ")
        return fmt.Sprintf("请基于以下上下文信息进行处理:\n\n%s", string(data))
    }

    var parts []string
    for _, key := range inputKeys {
        if val, ok := input[key]; ok {
            parts = append(parts, fmt.Sprintf("[%s]:\n%v", key, val))
        }
    }

    return fmt.Sprintf("请基于以下信息进行处理:\n\n%s", strings.Join(parts, "\n\n"))
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/orchestration/ -run TestDefaultStepExecutor_ExecutesAgent -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/orchestration/step_executor.go internal/orchestration/step_executor_test.go
git commit -m "feat(orchestration): add StepExecutor with event emission"
```

---

## Task 4: WorkerPool

**Files:**
- Create: `internal/orchestration/worker_pool.go`
- Test: `internal/orchestration/worker_pool_test.go`

- [ ] **Step 1: 编写失败测试**

```go
package orchestration

import (
    "context"
    "fmt"
    "sync/atomic"
    "testing"
    "time"
)

func TestWorkerPool_FixedConcurrency(t *testing.T) {
    var running int64
    var maxRunning int64
    exec := StepExecutorFunc(func(ctx context.Context, step *AgentStep, input map[string]any) *StepResult {
        cur := atomic.AddInt64(&running, 1)
        if cur > atomic.LoadInt64(&maxRunning) {
            atomic.StoreInt64(&maxRunning, cur)
        }
        time.Sleep(50 * time.Millisecond)
        atomic.AddInt64(&running, -1)
        return &StepResult{StepID: step.ID, Status: StepCompleted}
    })

    pool := NewWorkerPool(2, exec)
    defer pool.Stop()

    resultCh := make(chan *StepResult, 5)
    for i := 0; i < 5; i++ {
        pool.Submit(context.Background(), &StepNode{Step: &AgentStep{ID: fmt.Sprintf("s%d", i)}}, nil, resultCh)
    }

    for i := 0; i < 5; i++ {
        <-resultCh
    }

    if atomic.LoadInt64(&maxRunning) > 2 {
        t.Errorf("max concurrent should be 2, got %d", maxRunning)
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/orchestration/ -run TestWorkerPool_FixedConcurrency -v`
Expected: FAIL (`WorkerPool` 未定义)

- [ ] **Step 3: 实现最小代码**

```go
package orchestration

import (
    "context"
    "sync"
)

// WorkerPool 使用固定 worker goroutine 执行 step。
type WorkerPool struct {
    workers  int
    tasks    chan workerTask
    executor StepExecutor
    wg       sync.WaitGroup
}

type workerTask struct {
    ctx      context.Context
    node     *StepNode
    input    map[string]any
    resultCh chan<- *StepResult
}

// NewWorkerPool 创建 worker 池。
func NewWorkerPool(workers int, executor StepExecutor) *WorkerPool {
    if workers <= 0 {
        workers = 1
    }
    p := &WorkerPool{
        workers:  workers,
        tasks:    make(chan workerTask),
        executor: executor,
    }
    p.start()
    return p
}

func (p *WorkerPool) start() {
    for i := 0; i < p.workers; i++ {
        p.wg.Add(1)
        go func() {
            defer p.wg.Done()
            for task := range p.tasks {
                result := p.executor.Execute(task.ctx, task.node.Step, task.input)
                task.resultCh <- result
            }
        }()
    }
}

// Submit 提交任务到 worker 池。
func (p *WorkerPool) Submit(ctx context.Context, node *StepNode, input map[string]any, resultCh chan<- *StepResult) {
    p.tasks <- workerTask{ctx: ctx, node: node, input: input, resultCh: resultCh}
}

// Stop 关闭任务通道并等待所有 worker 退出。
func (p *WorkerPool) Stop() {
    close(p.tasks)
    p.wg.Wait()
}

// StepExecutorFunc 允许用函数实现 StepExecutor。
type StepExecutorFunc func(ctx context.Context, step *AgentStep, input map[string]any) *StepResult

func (f StepExecutorFunc) Execute(ctx context.Context, step *AgentStep, input map[string]any) *StepResult {
    return f(ctx, step, input)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/orchestration/ -run TestWorkerPool_FixedConcurrency -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/orchestration/worker_pool.go internal/orchestration/worker_pool_test.go
git commit -m "feat(orchestration): add fixed-size WorkerPool for step execution"
```

---

## Task 5: Scheduler

**Files:**
- Create: `internal/orchestration/scheduler.go`
- Test: `internal/orchestration/scheduler_test.go`

- [ ] **Step 1: 编写失败测试**

```go
package orchestration

import (
    "context"
    "fmt"
    "testing"
    "time"
)

func TestScheduler_RunsAllSteps(t *testing.T) {
    steps := []*AgentStep{
        {ID: "a", Name: "a"},
        {ID: "b", Name: "b"},
        {ID: "c", Name: "c"},
    }
    plan, _ := BuildExecutionPlan(ParallelMode, steps, nil)

    exec := StepExecutorFunc(func(ctx context.Context, step *AgentStep, input map[string]any) *StepResult {
        return &StepResult{StepID: step.ID, Status: StepCompleted, Output: map[string]any{"k": step.ID}}
    })
    pool := NewWorkerPool(2, exec)
    defer pool.Stop()

    scheduler := NewScheduler(plan, pool, SchedulerConfig{MaxConcurrency: 2})
    results, err := scheduler.Run(context.Background(), nil)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(results) != 3 {
        t.Errorf("expected 3 results, got %d", len(results))
    }
    if results["a"].Status != StepCompleted {
        t.Errorf("a not completed")
    }
}

func TestScheduler_RespectsDependencies(t *testing.T) {
    steps := []*AgentStep{
        {ID: "a"},
        {ID: "b"},
        {ID: "c"},
    }
    edges := []DAGEdge{{From: "a", To: "c"}, {From: "b", To: "c"}}
    plan, _ := BuildExecutionPlan(DAGMode, steps, edges)

    exec := StepExecutorFunc(func(ctx context.Context, step *AgentStep, input map[string]any) *StepResult {
        return &StepResult{StepID: step.ID, Status: StepCompleted}
    })
    pool := NewWorkerPool(2, exec)
    defer pool.Stop()

    scheduler := NewScheduler(plan, pool, SchedulerConfig{MaxConcurrency: 2})
    results, err := scheduler.Run(context.Background(), nil)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if results["c"].Status != StepCompleted {
        t.Errorf("c should be completed after a and b")
    }
}

func TestScheduler_RetryThenSuccess(t *testing.T) {
    steps := []*AgentStep{{ID: "r1"}}
    plan, _ := BuildExecutionPlan(ParallelMode, steps, nil)

    attempts := 0
    exec := StepExecutorFunc(func(ctx context.Context, step *AgentStep, input map[string]any) *StepResult {
        attempts++
        if attempts < 3 {
            return &StepResult{StepID: step.ID, Status: StepFailed, Error: fmt.Errorf("fail")}
        }
        return &StepResult{StepID: step.ID, Status: StepCompleted}
    })
    pool := NewWorkerPool(1, exec)
    defer pool.Stop()

    scheduler := NewScheduler(plan, pool, SchedulerConfig{
        MaxConcurrency: 1,
        RetryPolicy:    RetryPolicy{MaxRetries: 3, Backoff: 10 * time.Millisecond},
    })
    results, err := scheduler.Run(context.Background(), nil)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if results["r1"].Status != StepCompleted {
        t.Errorf("expected success after retry, got %s", results["r1"].Status)
    }
    if attempts != 3 {
        t.Errorf("expected 3 attempts, got %d", attempts)
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/orchestration/ -run TestScheduler -v`
Expected: FAIL (`Scheduler` 未定义)

- [ ] **Step 3: 实现最小代码**

```go
package orchestration

import (
    "context"
    "fmt"
    "time"
)

// SchedulerConfig 调度器配置。
type SchedulerConfig struct {
    MaxConcurrency int
    RetryPolicy    RetryPolicy
    FailFast       bool
}

// Scheduler 负责按依赖关系和并发限制派发 step。
type Scheduler struct {
    plan     *ExecutionPlan
    pool     *WorkerPool
    cfg      SchedulerConfig
}

// NewScheduler 创建 Scheduler。
func NewScheduler(plan *ExecutionPlan, pool *WorkerPool, cfg SchedulerConfig) *Scheduler {
    if cfg.MaxConcurrency <= 0 {
        cfg.MaxConcurrency = 1
    }
    return &Scheduler{plan: plan, pool: pool, cfg: cfg}
}

// Run 执行计划，返回 stepID -> StepResult 的映射。
func (s *Scheduler) Run(ctx context.Context, initialInput map[string]any) (map[string]*StepResult, error) {
    resultsCh := make(chan *StepResult, len(s.plan.Steps))
    defer close(resultsCh)

    nodeByID := make(map[string]*StepNode, len(s.plan.Steps))
    for _, n := range s.plan.Steps {
        nodeByID[n.Step.ID] = n
    }

    running := 0
    completed := 0
    failed := false
    outputs := make(map[string]map[string]any)
    retryCount := make(map[string]int)
    pendingRetry := make(map[string]time.Time)

    ready := make([]*StepNode, 0)
    for _, n := range s.plan.Steps {
        if s.plan.DepGraph.Ready(n.Step.ID) {
            ready = append(ready, n)
        }
    }

    retryTimer := time.NewTimer(time.Hour)
    defer retryTimer.Stop()
    retryTimer.Stop()

    for completed+running < len(s.plan.Steps) {
        // 将已到重试时间的任务加入就绪队列
        now := time.Now()
        for id, when := range pendingRetry {
            if now.After(when) || now.Equal(when) {
                ready = append(ready, nodeByID[id])
                delete(pendingRetry, id)
            }
        }

        // 派发就绪任务
        for len(ready) > 0 && running < s.cfg.MaxConcurrency && !failed {
            node := ready[0]
            ready = ready[1:]
            node.Status = StepRunning
            running++
            input := buildStepInput(node.Step, initialInput, outputs, s.plan.DepGraph)
            s.pool.Submit(ctx, node, input, resultsCh)
        }

        if running == 0 && len(pendingRetry) == 0 && len(ready) == 0 {
            break
        }

        // 计算下次重试时间，用于设置 timer
        var nextRetry time.Time
        hasPending := false
        for _, when := range pendingRetry {
            if !hasPending || when.Before(nextRetry) {
                nextRetry = when
                hasPending = true
            }
        }
        if hasPending {
            retryTimer.Reset(time.Until(nextRetry))
        } else {
            retryTimer.Stop()
        }

        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-retryTimer.C:
            // 下一轮循环会把到期重试加入 ready
        case result := <-resultsCh:
            running--
            node := nodeByID[result.StepID]
            node.Result = result
            node.Status = result.Status

            if result.Status == StepCompleted {
                completed++
                outputs[node.Step.ID] = result.Output
                newlyReady := s.plan.DepGraph.Complete(node.Step.ID)
                for _, id := range newlyReady {
                    ready = append(ready, nodeByID[id])
                }
            } else if result.Status == StepFailed {
                maxRetries := s.cfg.RetryPolicy.MaxRetries
                if node.Step.RetryPolicy != nil && node.Step.RetryPolicy.MaxRetries > 0 {
                    maxRetries = node.Step.RetryPolicy.MaxRetries
                }
                if maxRetries <= 0 {
                    maxRetries = defaultMaxRetries
                }
                if retryCount[node.Step.ID] < maxRetries {
                    retryCount[node.Step.ID]++
                    node.Result.RetryCount = retryCount[node.Step.ID]
                    backoff := s.cfg.RetryPolicy.Backoff
                    if node.Step.RetryPolicy != nil && node.Step.RetryPolicy.Backoff > 0 {
                        backoff = node.Step.RetryPolicy.Backoff
                    }
                    if backoff <= 0 {
                        backoff = defaultBackoff
                    }
                    // 指数退避：attempt * baseBackoff
                    backoff = backoff * time.Duration(retryCount[node.Step.ID])
                    pendingRetry[node.Step.ID] = time.Now().Add(backoff)
                    continue
                }

                completed++
                if s.cfg.FailFast {
                    failed = true
                } else {
                    // continue-on-error: 仍然解锁下游，但下游会收到 StepSkipped
                    newlyReady := s.plan.DepGraph.Complete(node.Step.ID)
                    for _, id := range newlyReady {
                        ready = append(ready, nodeByID[id])
                    }
                }
            }
        }
    }

    results := make(map[string]*StepResult, len(s.plan.Steps))
    for _, n := range s.plan.Steps {
        if n.Result == nil {
            n.Status = StepSkipped
            n.Result = &StepResult{StepID: n.Step.ID, StepName: n.Step.Name, Status: StepSkipped}
        }
        results[n.Step.ID] = n.Result
    }

    if failed {
        return results, fmt.Errorf("one or more steps failed")
    }
    return results, nil
}

func buildStepInput(step *AgentStep, initialInput map[string]any, outputs map[string]map[string]any, g *DependencyGraph) map[string]any {
    input := make(map[string]any, len(initialInput)+len(step.InputFrom))
    for k, v := range initialInput {
        input[k] = v
    }
    // 按依赖边合并上游输出
    for _, depID := range g.inEdges[step.ID] {
        out, ok := outputs[depID]
        if !ok {
            continue
        }
        for _, key := range step.InputFrom {
            if v, ok := out[key]; ok {
                input[key] = v
            }
        }
        // 若未指定 InputFrom，默认合并所有上游输出
        if len(step.InputFrom) == 0 {
            for k, v := range out {
                input[k] = v
            }
        }
    }
    return input
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/orchestration/ -run TestScheduler -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/orchestration/scheduler.go internal/orchestration/scheduler_test.go
git commit -m "feat(orchestration): add Scheduler with dependency-aware dispatch"
```

---

## Task 6: ExecutionEngine

**Files:**
- Create: `internal/orchestration/engine.go`
- Test: `internal/orchestration/engine_test.go`

- [ ] **Step 1: 编写失败测试**

```go
package orchestration

import (
    "context"
    "testing"

    "agentprimordia/internal/agent"
    "agentprimordia/internal/llm"
)

func TestExecutionEngine_Parallel(t *testing.T) {
    mockLLM := llm.NewMockLLM(nil)
    for i := 0; i < 3; i++ {
        mockLLM.WithResponse("ok")
    }
    ag := agent.New(agent.Config{Name: "test"}, mockLLM)

    steps := []*AgentStep{
        {ID: "p1", Name: "p1", Agent: ag, Prompt: "go"},
        {ID: "p2", Name: "p2", Agent: ag, Prompt: "go"},
        {ID: "p3", Name: "p3", Agent: ag, Prompt: "go"},
    }

    engine := NewExecutionEngine(ExecutionEngineConfig{MaxConcurrency: 2})
    result, err := engine.Run(context.Background(), ParallelMode, steps, nil, nil)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result.Status != StatusCompleted {
        t.Errorf("expected completed, got %s", result.Status)
    }
    if len(result.Steps) != 3 {
        t.Errorf("expected 3 steps, got %d", len(result.Steps))
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/orchestration/ -run TestExecutionEngine_Parallel -v`
Expected: FAIL (`ExecutionEngine` 未定义)

- [ ] **Step 3: 实现最小代码**

```go
package orchestration

import (
    "context"
    "time"
)

// ExecutionEngineConfig 执行引擎配置。
type ExecutionEngineConfig struct {
    MaxConcurrency int
    RetryPolicy    RetryPolicy
    FailFast       bool
    EventCh        chan<- *OrchestrationEvent
}

// ExecutionEngine 统一执行引擎。
type ExecutionEngine struct {
    cfg ExecutionEngineConfig
}

// NewExecutionEngine 创建执行引擎。
func NewExecutionEngine(cfg ExecutionEngineConfig) *ExecutionEngine {
    if cfg.MaxConcurrency <= 0 {
        cfg.MaxConcurrency = defaultWorkerMaxConcurrency
    }
    return &ExecutionEngine{cfg: cfg}
}

// Run 执行编排计划。
func (e *ExecutionEngine) Run(ctx context.Context, mode OrchestratorMode, steps []*AgentStep, edges []DAGEdge, initialInput map[string]any) (*ExecutionResult, error) {
    startTime := time.Now()

    plan, err := BuildExecutionPlan(mode, steps, edges)
    if err != nil {
        return nil, err
    }

    executor := NewDefaultStepExecutor(e.cfg.EventCh)
    pool := NewWorkerPool(e.cfg.MaxConcurrency, executor)
    defer pool.Stop()

    scheduler := NewScheduler(plan, pool, SchedulerConfig{
        MaxConcurrency: e.cfg.MaxConcurrency,
        RetryPolicy:    e.cfg.RetryPolicy,
        FailFast:       e.cfg.FailFast,
    })

    stepResults, err := scheduler.Run(ctx, initialInput)
    if err != nil && len(stepResults) == 0 {
        return nil, err
    }

    return buildExecutionResult(plan, stepResults, initialInput, startTime, err), nil
}

func buildExecutionResult(plan *ExecutionPlan, stepResults map[string]*StepResult, initialInput map[string]any, startTime time.Time, runErr error) *ExecutionResult {
    result := &ExecutionResult{
        Mode:        plan.Mode,
        Status:      StatusCompleted,
        StartTime:   startTime,
        Steps:       stepResults,
        FinalOutput: make(map[string]any),
    }

    if initialInput != nil {
        for k, v := range initialInput {
            result.FinalOutput[k] = v
        }
    }
    for _, sr := range stepResults {
        if sr.Status == StepCompleted && sr.Output != nil {
            for k, v := range sr.Output {
                result.FinalOutput[k] = v
            }
        }
    }

    result.Error = runErr
    if runErr != nil {
        hasCompleted := false
        hasFailed := false
        for _, sr := range stepResults {
            switch sr.Status {
            case StepCompleted:
                hasCompleted = true
            case StepFailed:
                hasFailed = true
            }
        }
        if hasCompleted && hasFailed {
            result.Status = StatusPartial
        } else if hasFailed {
            result.Status = StatusFailed
        } else {
            result.Status = StatusCompleted
        }
    }

    result.EndTime = time.Now()
    result.Duration = result.EndTime.Sub(result.StartTime)
    result.Metrics = computeExecutionMetrics(result)
    return result
}

func computeExecutionMetrics(result *ExecutionResult) ExecutionMetrics {
    metrics := ExecutionMetrics{TotalSteps: len(result.Steps)}
    var totalDuration time.Duration
    var durations []time.Duration
    for _, sr := range result.Steps {
        switch sr.Status {
        case StepCompleted:
            metrics.CompletedSteps++
        case StepFailed:
            metrics.FailedSteps++
        case StepSkipped:
            metrics.SkippedSteps++
        }
        if sr.Duration > 0 {
            totalDuration += sr.Duration
            durations = append(durations, sr.Duration)
        }
    }
    if len(durations) > 0 {
        metrics.AvgStepDuration = totalDuration / time.Duration(len(durations))
        metrics.MaxStepDuration = durations[0]
        metrics.MinStepDuration = durations[0]
        for _, d := range durations {
            if d > metrics.MaxStepDuration {
                metrics.MaxStepDuration = d
            }
            if d < metrics.MinStepDuration {
                metrics.MinStepDuration = d
            }
        }
    }
    metrics.TotalDuration = totalDuration
    if result.Mode == ParallelMode {
        metrics.ConcurrencyUsed = len(result.Steps)
    } else {
        metrics.ConcurrencyUsed = 1
    }
    return metrics
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/orchestration/ -run TestExecutionEngine_Parallel -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/orchestration/engine.go internal/orchestration/engine_test.go
git commit -m "feat(orchestration): add ExecutionEngine orchestrating plan/builder/pool/scheduler"
```

---

## Task 7: 集成到 Orchestrator 并提取辅助函数

**Files:**
- Modify: `internal/orchestration/orchestrator.go`
- Test: `internal/orchestration/orchestrator_test.go`

- [ ] **Step 1: 编写回归测试（验证旧行为不变）**

在 `internal/orchestration/orchestrator_test.go` 中补充：

```go
func TestOrchestrator_Parallel_ResultsNotOverwritten(t *testing.T) {
    mockLLM := llm.NewMockLLM(nil)
    mockLLM.WithResponse("one")
    mockLLM.WithResponse("two")
    ag := agent.New(agent.Config{Name: "test"}, mockLLM)

    orch := NewOrchestrator(OrchestratorConfig{Mode: ParallelMode})
    _ = orch.AddStep(&AgentStep{ID: "s1", Name: "s1", Agent: ag})
    _ = orch.AddStep(&AgentStep{ID: "s2", Name: "s2", Agent: ag})

    result, err := orch.Execute(context.Background(), nil)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result.Steps["s1"].Response.Content != "one" {
        t.Errorf("s1 overwritten: %v", result.Steps["s1"].Response)
    }
    if result.Steps["s2"].Response.Content != "two" {
        t.Errorf("s2 overwritten: %v", result.Steps["s2"].Response)
    }
}
```

- [ ] **Step 2: 运行测试确认失败（或确认旧实现有此 bug）**

Run: `go test ./internal/orchestration/ -run TestOrchestrator_Parallel_ResultsNotOverwritten -v`
Expected: 在旧实现下 FAIL（结果覆盖）

- [ ] **Step 3: 修改 orchestrator.go**

将 `Orchestrator.Execute` 改为委托给 `ExecutionEngine`。旧的 `executeSequential`、`executeParallel`、`executeDAG` 以及 `executeStep` 方法保留但不再被调用（后续清理阶段再删除或重构为使用 `StepExecutor`），避免单文件 diff 过大：

```go
func (o *Orchestrator) Execute(ctx context.Context, initialInput map[string]any) (*ExecutionResult, error) {
    startTime := time.Now()

    result := &ExecutionResult{
        OrchestratorID: o.config.Name,
        Mode:           o.config.Mode,
        Status:         StatusRunning,
        StartTime:      startTime,
        Steps:          make(map[string]*StepResult),
        FinalOutput:    make(map[string]any),
    }

    o.emitEvent("execution_started", "", map[string]any{
        "mode":  o.config.Mode,
        "steps": len(o.steps),
        "input": initialInput,
    })

    maxConcurrency := len(o.steps)
    if maxConcurrency <= 0 {
        maxConcurrency = defaultWorkerMaxConcurrency
    }
    engine := NewExecutionEngine(ExecutionEngineConfig{
        MaxConcurrency: maxConcurrency,
        RetryPolicy:    RetryPolicy{MaxRetries: o.config.MaxRetries, Backoff: defaultBackoff},
        FailFast:       true,
        EventCh:        o.eventCh,
    })

    execResult, err := engine.Run(ctx, o.config.Mode, o.steps, o.dagEdges, initialInput)
    if execResult != nil {
        // 复制引擎结果，但保留 orchestrator 元信息
        *result = *execResult
        result.OrchestratorID = o.config.Name
    }
    result.Error = err

    o.emitEvent("execution_completed", "", map[string]any{
        "status":   result.Status,
        "duration": result.Duration,
        "error":    err,
    })

    return result, err
}
```

将 `calculateMetrics` 从 `Orchestrator` 的方法改为包级函数（因为 `buildExecutionResult` 需要调用它，且它不依赖 `Orchestrator` 状态）。然后更新 `engine.go` 中的 `buildExecutionResult`，删除本地 `computeExecutionMetrics`，改为调用包级 `calculateMetrics`：

```go
// orchestrator.go
func calculateMetrics(result *ExecutionResult) ExecutionMetrics {
    metrics := ExecutionMetrics{
        TotalSteps: len(result.Steps),
    }
    var totalDuration time.Duration
    var durations []time.Duration
    for _, sr := range result.Steps {
        switch sr.Status {
        case StepCompleted:
            metrics.CompletedSteps++
        case StepFailed:
            metrics.FailedSteps++
        case StepSkipped:
            metrics.SkippedSteps++
        }
        if sr.Duration > 0 {
            totalDuration += sr.Duration
            durations = append(durations, sr.Duration)
            metrics.TotalRetries += sr.RetryCount
        }
    }
    if len(durations) > 0 {
        metrics.AvgStepDuration = totalDuration / time.Duration(len(durations))
        metrics.MaxStepDuration = durations[0]
        metrics.MinStepDuration = durations[0]
        for _, d := range durations {
            if d > metrics.MaxStepDuration {
                metrics.MaxStepDuration = d
            }
            if d < metrics.MinStepDuration {
                metrics.MinStepDuration = d
            }
        }
    }
    metrics.TotalDuration = totalDuration
    switch result.Mode {
    case ParallelMode:
        metrics.ConcurrencyUsed = len(result.Steps)
    default:
        metrics.ConcurrencyUsed = 1
    }
    return metrics
}

// Orchestrator 原方法改为调用包级函数
func (o *Orchestrator) calculateMetrics(result *ExecutionResult) ExecutionMetrics {
    return calculateMetrics(result)
}
```

```go
// engine.go
result.Metrics = calculateMetrics(result)
// 删除 engine.go 中的 computeExecutionMetrics 函数
```

- [ ] **Step 4: 运行所有 orchestration 测试**

Run: `go test ./internal/orchestration/ -v`
Expected: PASS（包括新回归测试）

- [ ] **Step 5: 提交**

```bash
git add internal/orchestration/orchestrator.go internal/orchestration/orchestrator_test.go
git commit -m "refactor(orchestration): integrate ExecutionEngine into Orchestrator"
```

---

## Task 8: 基准测试

**Files:**
- Create: `internal/orchestration/bench_engine_test.go`

- [ ] **Step 1: 编写基准测试**

```go
package orchestration

import (
    "context"
    "fmt"
    "testing"

    "agentprimordia/internal/agent"
    "agentprimordia/internal/llm"
)

func BenchmarkExecutionEngine_Parallel_100(b *testing.B) {
    for i := 0; i < b.N; i++ {
        b.StopTimer()
        mockLLM := llm.NewMockLLM(nil)
        for j := 0; j < 100; j++ {
            mockLLM.WithResponse("ok")
        }
        ag := agent.New(agent.Config{Name: "bench"}, mockLLM)
        steps := make([]*AgentStep, 100)
        for j := 0; j < 100; j++ {
            steps[j] = &AgentStep{ID: fmt.Sprintf("s%d", j), Name: fmt.Sprintf("s%d", j), Agent: ag, Prompt: "go"}
        }
        engine := NewExecutionEngine(ExecutionEngineConfig{MaxConcurrency: 10})
        b.StartTimer()

        _, err := engine.Run(context.Background(), ParallelMode, steps, nil, nil)
        if err != nil {
            b.Fatalf("unexpected error: %v", err)
        }
    }
}
```

- [ ] **Step 2: 运行基准测试**

Run: `go test ./internal/orchestration/ -bench BenchmarkExecutionEngine_Parallel_100 -benchmem`
Expected: 运行成功，输出内存分配与耗时

- [ ] **Step 3: 提交**

```bash
git add internal/orchestration/bench_engine_test.go
git commit -m "test(orchestration): add ExecutionEngine benchmark"
```

---

## Task 9: 全量回归与清理

**Files:**
- All modified files

- [ ] **Step 1: 运行格式化与静态检查**

```bash
go fmt ./internal/orchestration/...
go vet ./internal/orchestration/...
```
Expected: 无错误

- [ ] **Step 2: 运行全量测试**

```bash
go test ./internal/... ./pkg/...
```
Expected: PASS

- [ ] **Step 3: 检查是否遗留旧 execute 方法**

确认 `orchestrator.go` 中 `executeSequential`、`executeParallel`、`executeDAG` 不再被调用，或已改为空壳。

- [ ] **Step 4: 提交**

```bash
git add -A
git commit -m "chore(orchestration): format, vet, and verify full test suite"
```

---

## Self-Review

| Spec 要求 | 对应 Task |
|---|---|
| 统一 Sequential/Parallel/DAG 执行路径 | Task 2 + Task 6 + Task 7 |
| 固定 worker 池替代无限制 goroutine | Task 4 |
| DAG 同层节点并行 | Task 2（PlanBuilder）+ Task 5（Scheduler 按依赖图触发） |
| 重试不阻塞 worker goroutine | Task 5（Scheduler 内处理重试，WorkerPool 只执行） |
| 保持公共 API 不变 | Task 7（Orchestrator.Execute 签名不变） |
| 事件兼容 | Task 3 + Task 7（StepExecutor 与 Orchestrator 共用 eventCh） |

**Placeholder 扫描：** 无 TBD/TODO，所有步骤包含具体代码、命令与预期输出。

**类型一致性检查：**
- `DependencyGraph.Ready/Complete` 在 Task 1 定义，Task 2/5 使用。
- `StepExecutor` 接口在 Task 3 定义，Task 4/6 使用。
- `ExecutionEngineConfig` 在 Task 6 定义，Task 7 使用。
- `SchedulerConfig` 在 Task 5 定义，Task 6 使用。

**实现时需注意：**
- `buildStepInput` 通过 `DependencyGraph.inEdges` 精确合并直接上游输出；若多个上游输出同名键，后遍历者覆盖前者，与原 `executeDAG` 行为一致。
- 重试使用 `pendingRetry` + `retryTimer`，不阻塞 scheduler 主循环，backoff 当前为固定值，后续可扩展为指数退避与 jitter。
