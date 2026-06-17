# AP 全面性能优化计划

## 瓶颈总结

经过代码审计，发现以下核心性能瓶颈：

| 模块 | 瓶颈 | 严重度 |
|------|------|--------|
| Agent 引擎 | saveMemory 同步阻塞主循环；每轮重复类型断言；publishEvent 无意义 map 分配；executeTool 每次新建 Executor；流式拼接 O(n^2) | 高 |
| 消息转换 | convertToLLMMessages + BuildOpenAIMessages 双重转换；callToolsWithRetry 重复反解 toolDefs | 高 |
| Memory/SQLite | 全局 RWMutex 过度串行化；逐条 INSERT 无批量写入；importJSON 无事务 | 高 |
| LLM 调用层 | buildMessages 每轮重建 map 切片；HTTP Client 未复用优化；JSON 二次序列化 | 中 |
| Pool 调度 | updateStats O(n) 全量遍历；Stats() 双重锁；generateTaskID 分配 | 中 |
| 工具系统 | extractPathFromArgs 每次 JSON 解析；Definitions() 深度克隆开销 | 低 |
| 事件总线 | Publish 持锁写 channel；map[string]string payload 频繁分配 | 低 |

---

## Task 1: Agent 引擎 — saveMemory 异步化

**文件**: `internal/agent/react_persist.go`

将 `saveMemory` 中的 `mem.Add()` 改为异步 goroutine 写入，避免 SQLite 写入阻塞 ReAct 主循环。同时为异步摘要添加 goroutine 泄漏保护。

关键变更:
- `mem.Add()` 移入 goroutine，使用 `sync.WaitGroup` 在 agent 停止时等待完成
- 使用有界 channel 做写入队列，防止 goroutine 泄漏

---

## Task 1.5: Agent 引擎 — Executor 复用 + 流式拼接优化

**文件**: `internal/agent/react_llm.go`, `internal/agent/react_reasoning.go`

当前 `executeTool` 每次工具调用都 `tools.NewExecutor(a.getToolkit())` 分配新 Executor。`streamReasoning` 中 `fullContent += chunk.Content` 是 O(n^2)。

关键变更:
- 在 `ReActAgent` 上缓存 `*tools.Executor`，构造时创建或首次使用时懒初始化
- `streamReasoning` 改用 `strings.Builder` 拼接流式内容，O(n) 复杂度
- `streamReasoning` 内部不再重复调用 `tk.Definitions()`，复用外层传入的 `toolDefs`

---

## Task 2: Agent 引擎 — 缓存能力查找结果

**文件**: `internal/agent/react_loop.go`, `internal/agent/react_capabilities.go`

ReAct 循环中每轮都调用 `getTracer()`, `getCostTracker()`, `getMemoryStore()`, `getMetricsRecorder()` 等接口断言方法。这些在单次 `Run()` 期间不会变化。

关键变更:
- 在 `reactLoopEngine` 入口处一次性查找并缓存到 `loopConfig` 或局部变量
- `runLoop` 参数传入缓存的能力引用，消除每轮重复的类型断言

---

## Task 2.5: Agent 引擎 — 消除双重消息转换

**文件**: `internal/agent/react_convert.go`, `internal/agent/react_llm.go`, `internal/llm/provider_helpers.go`

当前数据流: `agent.Message` -> `convertToLLMMessages` -> `llm.ChatMessage` -> `BuildOpenAIMessages` -> `map[string]any`，同一份数据被转换两次。`callToolsWithRetry` 每轮还将 `[]map[string]any` 反解为 `[]llm.ToolDefinition`。

关键变更:
- 新增 `convertToOpenAIMessages(history []Message) []map[string]any` 一步到位
- `callToolsWithRetry` 直接使用 `[]llm.ToolDefinition` 而非 `[]map[string]any`，消除反解
- `streamReasoning` / `syncReasoning` 签名统一使用新的直接转换

---

## Task 3: Agent 引擎 — publishEvent 零分配优化

**文件**: `internal/agent/react_capabilities.go`, `internal/agent/react_loop.go`

当前 `publishEvent` 每次调用都创建 `map[string]string` 或 `map[string]int`，即使没有 EventPublisher 订阅者。

关键变更:
- 先检查 `getEventPublisher() == nil` 后立即 return（已有，但 payload 构造在调用前）
- 将 payload 构造延迟到 publishEvent 内部，或使用 `sync.Pool` 复用 map

---

## Task 4: Memory/SQLite — 写入路径批量优化

**文件**: `internal/memory/sqlite.go`

当前每次 `Add()` 都是独立的 `INSERT` 语句 + 全局互斥锁。在高吞吐场景（Pool 多 Agent 并发写入）下严重串行化。

关键变更:
- 引入 `BatchAdd(ctx, episodes)` 方法，使用 SQLite 事务批量写入
- `importJSON` 改用事务包裹，减少 fsync 次数
- 缩小锁粒度：写操作仅锁 WAL checkpoint 而非全局

---

## Task 5: Memory/SQLite — 读路径锁优化

**文件**: `internal/memory/sqlite.go`

当前所有读操作持有 `mu.RLock()`，但 SQLite WAL 模式本身已支持并发读。全局 RWMutex 导致读写互斥。

关键变更:
- 移除读操作（Search/Get/List/Count）的 `mu.RLock()` 包裹
- 仅保留写操作（Add/Delete/Update）的 `mu.Lock()`
- 依赖 SQLite 自身的 WAL + busy_timeout 处理并发

---

## Task 6: LLM 调用层 — HTTP Client 连接池优化

**文件**: `internal/llm/openai_provider.go`

当前 `NewOpenAIProvider` 创建 `http.Client{Timeout: 120s}` 但未配置 `Transport`，使用默认的 `http.DefaultTransport`。在高并发场景下应复用连接。

关键变更:
- 配置自定义 `http.Transport`：`MaxIdleConnsPerHost=10`, `IdleConnTimeout=90s`, `DisableKeepAlives=false`
- 使用 `sync.Pool` 复用 `bytes.Buffer` 减少 JSON 序列化时的分配

---

## Task 7: LLM 调用层 — buildMessages 分配优化

**文件**: `internal/llm/provider_helpers.go`

`BuildOpenAIMessages` 每次 LLM 调用都创建 `[]map[string]any` 切片，每个 map 都是独立堆分配。在长对话（20+ 轮）中分配开销显著。

关键变更:
- 预分配切片容量 `make([]map[string]any, len(msgs))`
- 对只有 role+content 的简单消息，使用预构建的 map 模板
- 考虑使用 `json.Encoder` 直接写入请求 body，避免中间 map 分配

---

## Task 8: Pool 调度 — Stats 增量维护

**文件**: `internal/pool/dispatcher.go`

`updateStats()` 遍历全部 tasks map (O(n))，而 `Stats()` 先调用 `updateStats()`（写锁），再获取读锁，存在双重锁开销。

关键变更:
- 使用 `atomic.Int64` 增量维护 running/queued/completed/failed 计数
- `Stats()` 直接读取原子变量，无需锁
- `generateTaskID` 改用 `strconv` + `sync.Pool` 避免 `fmt.Sprintf` 分配

---

## Task 9: 工具系统 — Registry 查找优化

**文件**: `internal/tools/registry.go`, `internal/tools/executor.go`

- `Registry.Get()` 每次获取 RLock，在高频率工具调用场景下有开销
- `extractPathFromArgs` 每次工具调用都做 JSON 解析
- `Definitions()` 每次深度克隆所有工具定义

关键变更:
- `Registry` 内部使用 `sync.Map` 替代 `map + RWMutex`（读多写少场景）
- `extractPathFromArgs` 使用 `json.Decoder` + 提前退出，只解析需要的字段
- `Definitions()` 缓存克隆结果，仅在 Register/Unregister 时失效

---

## Task 10: 事件总线 — 无锁发布优化

**文件**: `internal/events/bus.go`

`PublishAsync` 持有 RLock 遍历所有订阅者并尝试写 channel，当 channel 满时直接丢弃。

关键变更:
- 使用 `sync.RWMutex` + copy-on-write 快照模式：Publish 使用快照，Subscriber 变更时 copy
- 减少 hot path 上的锁持有时间

---

## Task 11: 基准测试验证

运行现有 bench 套件对比优化前后数据：
```bash
cd agentprimordia
go test -bench=BenchmarkAgentRun -benchmem -count=3 ./bench/suite/
go test -bench=BenchmarkConcurrent -benchmem -count=3 ./bench/suite/
go test -bench=BenchmarkMemoryLatency -benchmem -count=3 ./bench/suite/
```

补充关键微基准：
- `BenchmarkReActLoop_MemorySave` — saveMemory 异步化前后对比
- `BenchmarkSQLiteStore_BatchAdd` — 批量写入 vs 逐条写入
- `BenchmarkBuildOpenAIMessages` — 消息构建分配数

---

## Task 12: 编译验证 + 全量测试

```bash
cd agentprimordia
go build ./...
go test ./internal/agent/... ./internal/memory/... ./internal/llm/... ./internal/pool/... ./internal/tools/... ./internal/events/...
```

---

## 实施顺序

Task 1 -> Task 1.5 -> Task 2 -> Task 2.5 -> Task 3 (Agent 引擎)
-> Task 4 -> Task 5 (Memory)
-> Task 6 -> Task 7 (LLM)
-> Task 8 (Pool)
-> Task 9 (Tools)
-> Task 10 (Events)
-> Task 11 -> Task 12 (验证)
