# 阶段三：架构进化与扩展性实施计划（3-6 周）

> **状态：大部分完成 🟢**（7/8 Task 已完成：Task 2 ✅ DAG/Hooks/Cost/Workflow 子包拆分、Task 3 ✅ bufferpool/tokencache/zerocopy/hitl/context 子包拆分、Task 4 ✅ GoroutinePool 集成、Task 5 ✅ Pool 指标导出、Task 6 ✅ LLM 批处理、Task 7 ✅ gRPC 默认、Task 8 ✅ 拦截器链；Task 1 react/ 子包拆分待实施——ReActAgent 与父包深度耦合，需先将 Lifecycle/PromptTemplate/Tracer 等类型迁入 core/）
> **创建日期：2026-07-05**
> **前置文档**：`docs/plans/grpc-migration.md`（已完成）、`docs/plans/agent-package-split.md`（已完成）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

## 目标

通过 Agent 包进一步拆分降低耦合、协程池集成提升调度效率、LLM 批处理降低 API 成本、A2A gRPC 成默认传输，使架构具备企业级横向扩展能力。

## 当前状态盘点

| 组件 | 状态 | 说明 |
|------|------|------|
| A2A gRPC | ✅ 已实现 | `a2a/grpc_server.go` + `grpc_client.go` + proto 生成，JSON-RPC 仍共存 |
| Agent 包拆分 | ✅ 大部分完成 | 已拆出 core/bufferpool/tokencache/cost/hitl/context/hooks/dag/workflow/zerocopy 子包；react/ 待后续提取 |
| GoroutinePool | ✅ 已实现 | `internal/concurrency/pool.go` 动态调优协程池 |
| GoroutinePool 集成 | ✅ 完成 | `pool/types.go` 有 GoroutinePoolConfig，`goroutine_pool_integration_test.go` 通过 |
| LLM 批处理 | ✅ 完成 | `llm/batch.go` + `pool/llm_batch_integration_test.go` 已落地 |
| 协程池动态调优 | ✅ 完成 | `concurrency.GoroutinePool` 支持 MinWorkers/MaxWorkers 动态扩缩容 |

---

## Phase 3A：Agent 包进一步拆分（第 1-2 周）

### Task 1: 拆分 ReAct 循环核心到 react/ 子包（待实施）

**问题**：`internal/agent/` 仍有 40+ 文件，核心 ReAct 逻辑（`react_loop*.go` 共 11 个文件约 3000 行）集中在一个包内，编译慢、测试覆盖率统计粗粒度。

**现状评估**（2026-07-08 核验）：

ReActAgent 结构体与父包深度耦合，共引用 9 个父包类型：

| 类型 | 定义位置 | 类型 | 迁移难度 |
|------|---------|------|----------|
| `Tracer` | `tracer.go` | interface | 低 — 迁入 core/ |
| `PromptTemplate` | `prompt.go` | struct | 低 — 迁入 core/ |
| `OutputGuard` | `capabilities.go` | func type | 低 — 迁入 core/ |
| `AuditEvent` | `capabilities.go` | struct | 低 — 迁入 core/ |
| `AuditLogger` | `capabilities.go` | interface | 低 — 迁入 core/ |
| `MemoryStore` | `react_loop.go` | interface | 低 — 迁入 core/ |
| `EventPublisher` | `react_loop.go` | interface | 低 — 迁入 core/ |
| `MetricsRecorder` | `react_loop.go` | interface | 低 — 迁入 core/ |
| `LabeledMetricsRecorder` | `react_loop.go` | interface | 低 — 迁入 core/ |

已迁入子包的类型（可直接 import）：`Lifecycle` → `lifecycle/`、`Hooks` → `hooks/`、`CostTracker` → `cost/`、`HITLConfig` → `hitl/`、`ContextWindowStrategy` → `context/`、`RAGConfig` → `core/`

**结论**：react/ 拆分可行，但需先执行 preparatory step 将上述 9 个类型迁入 `core/`。预计工作量 2-3 天。

**Files:**
- Modify: `internal/agent/core/core.go`（新增 9 个共享类型）
- Create: `internal/agent/react/` 目录
- Move: `react_loop.go`、`react_loop_core.go`、`react_loop_engine.go`、`react_loop_tools.go`、`react_lifecycle.go`、`react_persist.go`、`react_llm.go`、`react_rag.go`、`react_reasoning.go`、`react_convert.go`、`react_capabilities.go` → `react/`

- [ ] **Step 0: 迁移共享类型到 core/**（preparatory step）

将 9 个被 ReActAgent 引用的父包类型迁移到 `core/` 子包，父包保留类型别名：
- `Tracer`（interface）— from `tracer.go`
- `PromptTemplate`（struct）— from `prompt.go`
- `OutputGuard`（func type）— from `capabilities.go`
- `AuditEvent`（struct）— from `capabilities.go`
- `AuditLogger`（interface）— from `capabilities.go`
- `MemoryStore`（interface）— from `react_loop.go`
- `EventPublisher`（interface）— from `react_loop.go`
- `MetricsRecorder`（interface）— from `react_loop.go`
- `LabeledMetricsRecorder`（interface）— from `react_loop.go`

- [ ] **Step 1: 创建 react/ 子包目录结构**

```
internal/agent/react/
├── loop.go          ← react_loop.go
├── core.go          ← react_loop_core.go
├── engine.go        ← react_loop_engine.go
├── tools.go         ← react_loop_tools.go
├── lifecycle.go     ← react_lifecycle.go
├── persist.go       ← react_persist.go
├── llm.go           ← react_llm.go
├── rag.go           ← react_rag.go
├── reasoning.go     ← react_reasoning.go
├── convert.go       ← react_convert.go
├── capabilities.go  ← react_capabilities.go
└── loop_test.go     ← react_loop_test.go
```

- [ ] **Step 2: 迁移文件并更新包名**

```go
// 每个文件改 package agent → package react
// 导出需要被外部访问的类型和函数
```

- [ ] **Step 3: 在 agent/ 包中保留类型别名**

```go
// internal/agent/react_alias.go
package agent

import "agentprimordia/internal/agent/react"

// ReActAgent 类型别名，保持向后兼容
type ReActAgent = react.Agent
type ReActConfig = react.Config
```

- [ ] **Step 4: 验证编译和测试**

```bash
go build ./...
go test -count=1 ./internal/agent/... 
```

- [ ] **Step 5: 验证覆盖率统计粒度提升**

```bash
go test -cover ./internal/agent/react/
# 现在 react 子包有独立的覆盖率统计
```

---

### Task 2: 拆分 DAG/Workflow/Hooks/CostTracker ✅

**已完成：** hooks/ ✅ 父包别名化、cost/ ✅ 子包提取、dag/ ✅ 子包提取（dag.go/dag_builder.go/dag_delegate.go 迁移至 dag/ 子包，含完整测试）、workflow/ ✅ 子包提取（workflow.go/workflow_engine.go/workflow_evaluator.go/workflow_executor.go/workflow_lifecycle.go/visualize.go 迁移至 workflow/ 子包，父包保留类型别名）。

**Files:**
- Create: `internal/agent/dag/`
- Create: `internal/agent/workflow/`
- Create: `internal/agent/hooks/`
- Create: `internal/agent/cost/`

- [x] **Step 1: 拆分 dag/** ✅（dag.go/dag_builder.go/dag_delegate.go → dag/ 子包，父包保留类型别名）

- [x] **Step 2: 拆分 workflow/** ✅（workflow.go/workflow_engine.go/workflow_evaluator.go/workflow_executor.go/workflow_lifecycle.go/visualize.go → workflow/ 子包，父包保留类型别名）

- [x] **Step 3: 拆分 hooks/** ✅（hooks.go → hooks/ 子包，父包转为类型别名文件）

- [x] **Step 4: 拆分 cost/** ✅（cost_tracker.go → cost/ 子包，父包转为类型别名文件）

- [x] **Step 5: 更新所有引用** ✅（通过类型别名保持兼容，go build ./... 零错误）

- [x] **Step 6: 验证** ✅（`go build ./...` 零错误，`go test -count=1 -short ./internal/agent/dag/` 通过）

---

### Task 3: 拆分零散工具文件 ✅

**已完成：** bufferpool/ ✅、tokencache/ ✅、zerocopy/ ✅、hitl/ ✅、context/ ✅ 全部子包提取完成，父包保留类型别名。

**Files:**
- Create: `internal/agent/bufferpool/` ← `bufferpool.go` ✅
- Create: `internal/agent/tokencache/` ← `tokencache.go` ✅
- Create: `internal/agent/zerocopy/` ← `zerocopy.go` ✅
- Create: `internal/agent/hitl/` ← `hitl.go`（Human-in-the-Loop）✅
- Create: `internal/agent/context/` ← `context_compress.go`、`context_window.go` ✅
- Create: `internal/agent/core/` ← 共享类型包（Message/Response/Agent 接口等）✅

- [x] 逐个迁移并验证编译 ✅
- [x] 在 `agent/` 中保留类型别名 ✅

---

## Phase 3B：GoroutinePool 集成到 Pool 调度器（第 3 周）

### Task 4: Pool 调度器接入动态协程池

**问题**：`internal/pool/dispatcher.go` 使用固定 worker 池（`workerCount = p.config.MaxConcurrency`），无法根据负载动态扩缩容。

**Files:**
- Modify: `internal/pool/pool.go`
- Modify: `internal/pool/dispatcher.go`
- Create: `internal/pool/pool_integration_test.go`

- [x] **Step 1: 在 Pool 中注入 GoroutinePool**

```go
// internal/pool/pool.go
type Pool struct {
    // ... 现有字段
    workerPool *concurrency.GoroutinePool
}

func NewPool(cfg PoolConfig) *Pool {
    p := &Pool{
        // ...
    }
    
    // 创建动态协程池
    minWorkers := cfg.MinConcurrency
    if minWorkers < 1 {
        minWorkers = 1
    }
    maxWorkers := cfg.MaxConcurrency
    if maxWorkers < minWorkers {
        maxWorkers = minWorkers
    }
    
    p.workerPool = concurrency.NewGoroutinePool(concurrency.Config{
        MinWorkers:  minWorkers,
        MaxWorkers:  maxWorkers,
        QueueSize:   maxWorkers * 10,
        IdleTimeout: 30 * time.Second,
    })
    
    return p
}
```

- [x] **Step 2: 修改 dispatcher 使用 GoroutinePool.Submit**

```go
// internal/pool/dispatcher.go
// Before: 固定 worker goroutine
for w := 0; w < workerCount; w++ {
    go func() { for item := range taskCh { ... } }()
}

// After: 使用 GoroutinePool
for item := range taskCh {
    pool.Submit(func(ctx context.Context) error {
        // 执行任务
        return nil
    })
}
```

- [x] **Step 3: 优雅关闭集成**

```go
func (p *Pool) Close() error {
    if p.workerPool != nil {
        p.workerPool.Stop()
    }
    // ... 其他清理
}
```

- [x] **Step 4: 编写动态扩缩容测试**

```go
func TestPool_DynamicScaling(t *testing.T) {
    pool := NewPool(PoolConfig{
        MinConcurrency: 2,
        MaxConcurrency: 20,
    })
    defer pool.Close()
    
    // 提交 50 个任务，验证 worker 数动态增长
    // 等待空闲，验证 worker 数自动缩减
}
```

- [x] **Step 5: 验证** ✅（`goroutine_pool_integration_test.go` + `pool_integration_test.go` 通过）

---

### Task 5: Pool 指标导出

**Files:**
- Modify: `internal/pool/pool.go`

- [x] **Step 1: 导出协程池指标**

```go
type PoolStats struct {
    ActiveWorkers  int32
    TotalWorkers   int32
    QueueLength    int
    QueueCapacity  int
    TasksCompleted int64
    TasksFailed    int64
}

func (p *Pool) Stats() PoolStats {
    return PoolStats{
        ActiveWorkers: p.workerPool.ActiveWorkers(),
        TotalWorkers:  p.workerPool.TotalWorkers(),
        QueueLength:   p.workerPool.QueueLength(),
        // ...
    }
}
```

- [x] **Step 2: 在 metrics 中记录**（`Pool.GoroutinePoolStats()` 暴露给 Prometheus）

```go
// 记录到 Prometheus metrics
metrics.RecordPoolStats(pool.Stats())
```

---

## Phase 3C：LLM 请求批处理（第 4-5 周）

### Task 6: LLM BatchProcessor 实现

**问题**：多个并发 Agent 的 LLM 请求各自独立调用 API，无法合并为单次批处理调用，造成 API 成本浪费。

**Files:**
- Create: `internal/llm/batch.go`
- Create: `internal/llm/batch_test.go`
- Create: `internal/llm/bench_test.go`（补充 Benchmark）

- [x] **Step 1: 编写批处理器测试**

```go
// internal/llm/batch_test.go
func TestBatchProcessor_SingleRequest(t *testing.T) {
    mock := &mockBatchProvider{}
    batch := NewBatchProcessor(mock, BatchConfig{
        MaxBatchSize: 4,
        FlushTimeout: 100 * time.Millisecond,
    })
    defer batch.Close()
    
    resp, err := batch.Complete(context.Background(), &CompletionRequest{
        Messages: []Message{{Role: RoleUser, Content: "hello"}},
    })
    if err != nil {
        t.Fatalf("批处理失败: %v", err)
    }
    if resp.Content != "batch response" {
        t.Errorf("Content = %q", resp.Content)
    }
}

func TestBatchProcessor_MergeMultipleRequests(t *testing.T) {
    // 10 个并发请求，验证 API 调用次数 < 10
}

func TestBatchProcessor_FlushTimeout(t *testing.T) {
    // 单个请求，验证在 FlushTimeout 后自动刷新
}

func TestBatchProcessor_ContextCancel(t *testing.T) {
    // 验证 context 取消时请求被正确取消
}
```

- [x] **Step 2: 实现批处理器**

```go
// internal/llm/batch.go
package llm

type BatchConfig struct {
    MaxBatchSize int           // 最大批次大小
    FlushTimeout time.Duration // 刷新超时
}

type BatchProcessor struct {
    provider Provider
    cfg      BatchConfig
    queue    chan batchRequest
    ctx      context.Context
    cancel   context.CancelFunc
    wg       sync.WaitGroup
}

type batchRequest struct {
    req    *CompletionRequest
    respCh chan batchResponse
}

type batchResponse struct {
    resp *CompletionResponse
    err  error
}

func NewBatchProcessor(provider Provider, cfg BatchConfig) *BatchProcessor { /* ... */ }

func (bp *BatchProcessor) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
    br := batchRequest{
        req:    req,
        respCh: make(chan batchResponse, 1),
    }
    
    select {
    case bp.queue <- br:
    case <-ctx.Done():
        return nil, ctx.Err()
    }
    
    select {
    case resp := <-br.respCh:
        return resp.resp, resp.err
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}
```

- [x] **Step 3: 实现批处理合并逻辑**

```go
func (bp *BatchProcessor) processBatch(batch []batchRequest) {
    // 策略 1：如果 Provider 支持 batch API（如 OpenAI Batch API），合并为单次调用
    // 策略 2：不支持 batch API 时，逐个调用但共享连接池
    // 按优先级排序（高优先级先执行）
    
    for _, req := range batch {
        resp, err := bp.provider.Complete(bp.ctx, req.req)
        req.respCh <- batchResponse{resp: resp, err: err}
    }
}
```

- [x] **Step 4: 集成到 Pool 调度器**（`pool/llm_batch_integration_test.go` 验证 Pool + BatchProcessor 联动）

```go
type ReActAgent struct {
    // ... 现有字段
    batchProcessor *llm.BatchProcessor // 可选，nil 表示不使用批处理
}

func (a *ReActAgent) WithBatchProcessor(bp *llm.BatchProcessor) *ReActAgent {
    a.batchProcessor = bp
    return a
}
```

- [x] **Step 5: Benchmark**

```go
func BenchmarkBatchProcessor_vs_Direct(b *testing.B) {
    // 对比批处理 vs 直接调用的性能和 API 调用次数
}
```

- [x] **Step 6: 验证** ✅（`batch_test.go` + `llm_batch_integration_test.go` 通过）

---

## Phase 3D：A2A gRPC 成为默认传输（第 6 周）

### Task 7: gRPC 替代 JSON-RPC 成为默认 ✅

**问题**：A2A 包中 JSON-RPC 和 gRPC 双栈共存，增加维护成本。gRPC 在序列化延迟、消息体积、并发吞吐上全面优于 JSON-RPC。

**Files:**
- Modify: `internal/agent/a2a/server.go`（标记 JSON-RPC 为 Deprecated）✅
- Modify: `internal/agent/a2a/client.go`（标记 JSON-RPC 为 Deprecated）✅
- Modify: `pkg/a2a.go`（gRPC 成为默认导出）✅
- (无需改 `bridge.go`：bridge 是消息格式转换层，与传输无关)

- [x] **Step 1: 标记 JSON-RPC 为 Deprecated**

```go
// internal/agent/a2a/server.go
//
// Deprecated: 自 v1.x 起 gRPC 成为 A2A 的默认传输；新代码请使用 A2AGRPCServer。
type A2AServer struct { /* ... */ }

// Deprecated: 新代码请使用 NewA2AGRPCServer；本函数保留到 v2.0 移除。
func NewA2AServer(tm TaskManager, opts ...ServerOption) *A2AServer { /* ... */ }
```

`client.go` 中的 `A2AClient` / `NewA2AClient` 也已标记 Deprecated。

- [x] **Step 2: 更新 pkg/a2a.go 默认推荐 gRPC**

```go
// pkg/a2a.go — 文档开头直接推荐 gRPC，并保留 JSON-RPC 别名为 Deprecated
// 自 v1.x 起，**A2A 的默认传输是 gRPC**
//   srv  := ap.NewA2AGRPCServer(service)
//   cli, _ := ap.NewA2AGRPCClient("localhost:50051")

// A2AServer = a2a.A2AServer  // Deprecated: 自 v1.x 起 A2AGRPCServer 成为默认...
// A2AClient = a2a.A2AClient  // Deprecated: ...
// A2AJSONRPCRequest 等 JSON-RPC 类型别名全部 Deprecated
```

> 设计说明：保留 `A2AServer`/`A2AClient` 作为 JSON-RPC 别名（而非直接重定向到 gRPC），是为了不破坏旧示例代码；用户 IDE/编译器会通过 Deprecated 标记提醒迁移。

- [x] **Step 3: 更新文档和示例**

本 Task 的文档更新完成。`ecosystem/examples/a2a-grpc-demo` 已使用 gRPC API。JSON-RPC 旧示例保留但顶部添加迁移提示。

- [x] **Step 4: 验证** ✅

```bash
go build ./... && go vet ./... && go test -count=1 ./internal/agent/a2a/  # ok
```

---

### Task 8: A2A gRPC 拦截器链

**Files:**
- Create: `internal/agent/a2a/interceptors.go`

- [x] **Step 1: 实现通用拦截器**（`interceptors.go`：RecoveryInterceptor / LoggingInterceptor / MetricsInterceptor + Stream 变体）

```go
// LoggingInterceptor 日志拦截器
func LoggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
        start := time.Now()
        resp, err := handler(ctx, req)
        logger.Info("a2a rpc",
            "method", info.FullMethod,
            "duration", time.Since(start),
            "error", err,
        )
        return resp, err
    }
}

// MetricsInterceptor 指标拦截器
// RecoveryInterceptor panic 恢复拦截器
// AuthInterceptor 认证拦截器
```

- [x] **Step 2: 在 gRPC server 中注册拦截器链**（`ChainUnaryInterceptors` / `ChainStreamInterceptors` 组合器）

```go
func NewA2AGRPCServer(opts ...GRPCServerOption) *A2AGRPCServer {
    grpcOpts := []grpc.ServerOption{
        grpc.ChainUnaryInterceptor(
            RecoveryInterceptor(),
            LoggingInterceptor(slog.Default()),
            MetricsInterceptor(),
        ),
    }
    // ...
}
```

- [x] **Step 3: 验证** ✅

---

## 验收标准

1. `go build ./...` 和 `go vet ./...` 零错误
2. `go test -race -count=1 ./...` 全部通过
3. `internal/agent/` 包文件数从 40+ 降至 15 以下（核心逻辑迁入子包）
4. 每个子包有独立的覆盖率统计
5. Pool 调度器使用 GoroutinePool，支持动态扩缩容
6. LLM BatchProcessor 在 10 并发请求下 API 调用次数 < 10
7. A2A gRPC 为默认传输，JSON-RPC 标记 Deprecated
8. gRPC 拦截器链覆盖日志、指标、panic 恢复、认证
9. 所有公共 API 通过类型别名保持向后兼容
10. 覆盖率：新子包 ≥75%

## 预期成果

| 指标 | 当前 | 目标 |
|------|------|------|
| `internal/agent/` 文件数 | 40+ | < 15 |
| 编译时间（agent 包） | 基线 | -30%（拆分后增量编译） |
| Pool 调度器扩缩容 | 固定 worker | 动态（MinWorkers → MaxWorkers） |
| LLM API 调用次数（10 并发） | 10 | < 4（批处理合并） |
| A2A 序列化延迟 | ~500ns (JSON) | ~150ns (gRPC) |
| A2A 并发吞吐 | ~600 req/s | ~5000+ req/s |
| gRPC 拦截器 | 0 | 4（日志/指标/recovery/auth） |
