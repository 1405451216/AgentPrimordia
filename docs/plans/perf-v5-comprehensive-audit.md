# perf-v5: AgentPrimordia 综合性能优化实施计划

> 基于 6 个并行审计 + 直接审计的全项目 469 个 Go 文件（约 113K 行）的综合发现。
> 所有 12 项 Task 来自本计划的"实施路线"段落，按依赖关系和风险从低到高排序。
> 预计总工作量：约 8-10 小时（实施）+ 2-3 小时（验证）。

---

## 总体概览

| 维度 | 发现数 | 严重度分布 |
|------|------|------|
| LLM 模块 | 39 项 | 5 Critical / 10 High / 14 Medium / 10 Low |
| Agent 模块 | 22 项 | 2 Critical / 7 High / 9 Medium / 4 Low |
| Orchestration 模块 | 18 项 | 3 Critical / 7 High / 5 Medium / 3 Low |
| Memory 模块 | 9 项 | 0 Critical / 4 High / 3 Medium / 2 Low |
| Pool/Metrics/Events | 8 项 | 0 Critical / 3 High / 3 Medium / 2 Low |
| Tools/Executor | 5 项 | 2 Critical / 1 High / 1 Medium / 1 Low |
| 通用（fmt.Sprintf/锁/map/reflect） | ~20 项 | 0 Critical / 8 High / 8 Medium / 4 Low |
| **合计** | **~121 项** | **12 Critical / 40 High / 43 Medium / 26 Low** |

## 前置约束

- `go build ./...` 和 `go vet ./...` 零错误
- 所有修改模块 `go test` PASS（包含新增 benchmark）
- 中文注释
- 提交信息格式：`perf(v5): xxx`

---

## 实施路线

### Phase 1 — Critical 修复（先做安全/稳定，最高 ROI）

#### Task 1: Tool Executor 缺 panic recovery
- **文件**: `internal/tools/executor.go:112, 155-161`
- **类型**: panic recover
- **严重度**: Critical（任意工具 panic 杀死整个 agent 进程）
- **改动**:
  - `Execute` 加 `defer func() { if r := recover(); r != nil { ... } }()` 在 `tool.Execute(execCtx, args)` 外
  - `ExecuteBatch` 内 goroutine 加同样 recover
  - recover 错误转为 `NewErrorResult("tool panic: %v", r)` 并记录到日志

#### Task 2: Stream body 关闭一致性
- **文件**: 14 个 provider 的 SSE Stream 函数（`openai_provider.go:201, 215` 等）
- **类型**: 资源管理
- **严重度**: Critical（body 双重关闭 / 资源泄漏）
- **改动**:
  - error path 显式 `io.Copy(io.Discard, resp.Body)` + `Close()`，避免 defer 重复
  - 或在 goroutine 入口 `defer resp.Body.Close()`，外层只读 status

#### Task 3: 熔断器半开 race
- **文件**: `internal/llm/resilient.go:185-203`
- **类型**: 并发模型
- **严重度**: Critical（CAS 失败方被错误放过）
- **改动**:
  - CAS 失败方应返回 `ErrCircuitOpen`（当前错误返回 nil）
  - 允许多个并发 probe（`int32` 计数，最多 2-3 个）

#### Task 4: MCP transport timeout 缺失
- **文件**: `internal/tools/mcp_transport.go:34`
- **类型**: 资源管理
- **严重度**: Critical（goroutine 永久挂起）
- **改动**: `client: &http.Client{Timeout: 30 * time.Second}` + 共享 transport

#### Task 5: Supervisor/Handoff/Pipeline 锁内 JSON 序列化
- **文件**: `internal/orchestration/supervisor.go:584-595`, `internal/orchestration/handoff.go:345-356`, `internal/orchestration/orchestrator.go:301-311`, `internal/orchestration/collaboration.go:609-619`
- **类型**: 锁内 IO
- **严重度**: Critical（持锁时间过长，阻塞并发）
- **改动**: 锁内只快照 data，锁外 `json.MarshalIndent`

---

### Phase 2 — High 修复（连接池、Transport、JSON typed struct）

#### Task 6: 11 个 Provider 缺 Transport 配置
- **文件**: `internal/llm/{anthropic,azure,cohere,gemini,glm,mistral,qwen,ollama,openai_multimodal,anthropic_vision,gemini_multimodal}_provider.go`
- **类型**: 连接池调优
- **严重度**: High（每次请求 TCP+TLS 握手，+50-200ms 延迟）
- **改动**: 提取 `newDefaultLLMTransport()` 工厂函数（参考 OpenAI L90-101），所有 Provider 构造函数调用
- **预期收益**: 高并发吞吐 +30-50%，P99 延迟 -50ms

#### Task 7: 协作 prompt 构建用 strings.Builder
- **文件**: `internal/orchestration/collaboration.go:1004-1090`（5 个 buildXxxPrompt 函数）
- **类型**: 字符串分配
- **严重度**: High（每轮每 round 18+ 处 fmt.Sprintf 反射）
- **改动**:
  - `buildDebatePrompt`, `buildReviewPrompt`, `buildConsensusPrompt`, `buildVotingPrompt`, `buildDiscussionPrompt`, `buildBrainstormPrompt` 全部改用 `strings.Builder.Grow(1024)`
  - 改完后保持相同的字符串拼接顺序

#### Task 8: Cache fingerprint 缓存 + Stats 修复
- **文件**: `internal/llm/cache.go:108-119, 151-161`, `internal/llm/cache_enhanced.go:73, 98, 261`
- **类型**: 缓存命中率
- **严重度**: High
- **改动**:
  - fast-path miss 计入 misses
  - `PromptFingerprint` 用 `sync.Map` 缓存 query → fingerprint
  - entry.HitCount 改 `atomic.Int64`

#### Task 9: Cache LRU 改 container/list
- **文件**: `internal/llm/cache.go:182-189, 260-272`, `internal/llm/cache_enhanced.go:103-115`
- **类型**: 数据结构
- **严重度**: High（O(N) 重排/扫描）
- **改动**: `container/list` + `map[string]*list.Element` 实现 O(1) LRU 升级/淘汰

#### Task 10: 11 个 Provider 的 request body typed struct
- **文件**: 全部 `*_provider.go` 的 `Complete/Stream/CallTools` 的 `body := map[string]any{...}`
- **类型**: 序列化性能
- **严重度**: High（反射比 typed struct 慢 2-5×）
- **改动**:
  - 定义 `openaiChatRequest`, `anthropicRequest` 等 typed struct + json tag
  - 用 `json.Marshal(struct)` 替代 `json.Marshal(map)`
- **预期收益**: 序列化耗时 -50%，长 prompt (10k tokens) 节省 5-10ms/次

#### Task 11: HookStats 锁内 map 写改 atomic
- **文件**: `internal/agent/hooks.go:183-192`
- **类型**: 锁竞争
- **严重度**: High（每 turn 5+ 次 fire hook 写 map）
- **改动**:
  - `HookStats.ByPoint` 改 `[]atomic.Int64`（HookPoint 是有限 enum）
  - 删除 `s.mu.Lock/Unlock`

#### Task 12: HookManager.Fire 减少 slice 拷贝 + phaseOrder 包级 var
- **文件**: `internal/agent/hooks.go:298-306`
- **类型**: 内存分配
- **严重度**: High
- **改动**:
  - `phaseOrder` 提升为包级 `var phaseOrder = []HookPhase{...}`
  - 改用 `atomic.Pointer[[]Hook]` 快照，避免 `make([]Hook)` 每次分配

#### Task 13: CostTracker.checkBudgetLocked O(n) → O(1)
- **文件**: `internal/agent/cost_tracker.go:132-167`
- **类型**: 算法复杂度
- **严重度**: High（每 turn 调用，records 可上千）
- **改动**:
  - 维护 `atomic.Int64` / `atomic.Uint64` 累加字段（`TotalCostUSDBits` 存 math.Float64bits）
  - Record 时原子累加，CheckBudget 直接 Load 比较

#### Task 14: InMemoryStore.Search 锁内 ToLower
- **文件**: `internal/memory/memory.go:68-88`
- **类型**: 锁内计算
- **严重度**: High
- **改动**:
  - `lowerQuery` 提到 for 之前
  - ToLower 移到锁外：先 copy episode IDs 出来 → 锁外处理

#### Task 15: Pool dispatcher.pt.status atomic
- **文件**: `internal/pool/dispatcher.go:258-323`（5+ 处 `p.mu.Lock` + `pt.status = ...`）
- **类型**: 锁竞争
- **严重度**: High
- **改动**:
  - `pt.status` 改 `atomic.Int32`
  - 删除 80% 的 `p.mu.Lock/Unlock` 块

---

### Phase 3 — Medium 修复（同步原语、字符串拼接、缓存）

#### Task 16: 锁内 emitEvent/listener 回调移出锁外
- **文件**: `internal/agent/lifecycle.go:94-143`, `internal/agent/hooks.go:177-192`, `internal/orchestration/{supervisor,handoff,orchestrator,pipeline,debate,collaboration}.go`（emit 模式）
- **类型**: 锁内回调
- **严重度**: Medium
- **改动**: 锁内 `copy(listeners)/copy(hooks)`，锁外调用

#### Task 17: StepExecutor Condition 锁内用户回调
- **文件**: `internal/agent/dag.go:521-536`（`stateMu.Lock` 内调 `edge.Condition(ctx, srcResult)`）
- **类型**: 锁内 IO
- **严重度**: Medium
- **改动**: 锁外 evaluate，锁内只读 `srcResult`

#### Task 18: Stream 重复 string→[]byte
- **文件**: 8 个 provider 的 SSE 解析（`openai_provider.go:230, 240` 等）
- **类型**: 内存拷贝
- **严重度**: Medium
- **改动**: `json.NewDecoder(strings.NewReader(data)).Decode(&sseResp)` 一次完成

#### Task 19: Body / Scanner buffer sync.Pool
- **文件**: 7 个 provider 的 Stream 入口（`openai_provider.go:218` 等 64KB scanner buffer）
- **类型**: 内存分配
- **严重度**: Medium
- **改动**: `var scannerPool = sync.Pool{New: func() any { ... }}`

#### Task 20: Tool Executor 日志改 slog + 脱敏
- **文件**: `internal/tools/executor.go:30, 71, 89, 117, 124`
- **类型**: 日志性能 + 安全
- **严重度**: Medium
- **改动**: `*log.Logger` → `*slog.Logger`；敏感字段（password/token）截断或脱敏

#### Task 21: Event payload map 改 sync.Pool
- **文件**: `internal/agent/react_loop.go:325, 390, 437, 462, 481, 566, 583, 668, 684`
- **类型**: 内存分配
- **严重度**: Medium
- **改动**: 引入 `var eventPayloadPool = sync.Pool{...}` 缓存 `map[string]string` / `map[string]int`

#### Task 22: HITL fmt.Sprintf 改 strings.Builder
- **文件**: `internal/agent/react_loop.go:485, 486, 490, 498, 499, 506, 523, 550`
- **类型**: 字符串分配
- **严重度**: Medium
- **改动**: 6+ 处 fmt.Sprintf 改 strings.Builder 预拼

#### Task 23: 协作 mergeSuggestionsIntoOptions 预计算词频表
- **文件**: `internal/orchestration/collaboration.go:900-919`
- **类型**: map 分配
- **严重度**: Medium
- **改动**: 在入口处预计算所有 option 的 `map[string]int` 词频表，传给 similarityScore

#### Task 24: Agent Bus Broadcast 锁内分配
- **文件**: `internal/agent/bus.go:138-148`
- **类型**: 锁内分配
- **严重度**: Medium
- **改动**: `target` slice 用 sync.Pool 复用；`chsCopy` 改为按需新建

#### Task 25: Cache 慢路径 O(N) 向量搜索
- **文件**: `internal/llm/cache.go:140-149`
- **类型**: 算法复杂度
- **严重度**: Medium
- **改动**:
  - 简单方案：分桶（按 fingerprintToVector 第一维 hash）→ 只扫描同桶
  - 进阶方案：集成 HNSW

#### Task 26: Cache TTL 后台清理
- **文件**: `internal/llm/cache.go` (InMemoryCache, FingerprintCache)
- **类型**: 资源管理
- **严重度**: Medium
- **改动**: 启动 goroutine，每 5 分钟清理过期 entry

#### Task 27: Workflow vs DAG Metrics 分离
- **文件**: `internal/agent/workflow.go:1040-1050` 与 dag.go 共用 Metric 结构
- **类型**: 锁内分配
- **严重度**: Medium
- **改动**: `metrics.TotalDuration +=` 锁内，改 atomic.Int64 累加

#### Task 28: GroupChat RoundRobin/LastSpeakerSelector 改 atomic
- **文件**: `internal/agent/group_chat.go:129-138, 163-170`
- **类型**: 锁竞争
- **严重度**: Medium
- **改动**: `atomic.Uint64.Add` + 取模替代 sync.Mutex

#### Task 29: RoleBasedSelector 预 loweredKeywords
- **文件**: `internal/agent/group_chat.go:218-231`
- **类型**: 重复计算
- **严重度**: Medium（每轮 group chat 调）
- **改动**: RoleBasedConfig 构造时一次性 `ToLower` 所有 keywords

---

### Phase 4 — Low 修复（清理、可读性、少量性能）

#### Task 30: History 滑动窗口（默认行为）
- **文件**: `internal/agent/react_persist.go` 的 `trimContext`
- **类型**: 内存保护
- **严重度**: Low（默认 nil strategy 时 history 无界增长）
- **改动**: 默认滑动窗口 `defaultMaxHistoryMessages=100`，保留系统消息 + 最近 N 条

#### Task 31: 序列化导入清理
- **文件**: 全部 .go
- **类型**: dead code
- **严重度**: Low
- **改动**: 删除未使用的 import（`strconv`, `fmt` 等）

#### Task 32: 错误路径 fmt.Errorf 改 errors.New + %w
- **文件**: 多处 `fmt.Errorf("static: %w", err)` 反射开销
- **类型**: 错误分配
- **严重度**: Low
- **改动**: 静态部分用 `errors.New`；动态用 fmt.Errorf

#### Task 33: TODO/XXX/FIXME 清理
- **文件**: 多处
- **类型**: 代码质量
- **严重度**: Low
- **改动**: 实施 phase 3 时同步处理

#### Task 34: Benchmark 套件补充（关键缺口）
- **文件**: 各模块新增 `bench_test.go`
- **类型**: 性能基线
- **严重度**: Low
- **改动**:
  - agent: BenchmarkReActAgent_*, BenchmarkDAG_Run_*, BenchmarkBus_Publish_*
  - llm: BenchmarkCache_*, BenchmarkTokenBucket_*, BenchmarkPromptFingerprint
  - orchestration: BenchmarkEngine_Sequential_*, BenchmarkOrchestrator_*
  - tools: BenchmarkExecutor_Execute_*

---

## 优先级排序（按"实施难度 × 收益"）

### ⚡ Quick Win（< 1 小时，1-2 行可改）

| Task | 难度 | 预期收益 | 风险 |
|------|------|---------|------|
| T1 (Tool panic recover) | 极低 | 极高（稳定性） | 无 |
| T4 (MCP timeout) | 极低 | 极高（稳定性） | 无 |
| T11 (HookStats atomic) | 低 | 高 | 低 |
| T12 (phaseOrder 包级 var) | 极低 | 中 | 无 |
| T16 (emitEvent 移出锁) | 中 | 中 | 中 |
| T18 (SSE NewDecoder) | 低 | 中 | 低 |
| T20 (slog + 脱敏) | 低 | 中（可观测性+安全） | 无 |
| T28 (GroupChat atomic) | 极低 | 中 | 无 |
| T29 (RoleBased 预 lowered) | 极低 | 中 | 无 |
| T32 (errors.New) | 低 | 低 | 无 |
| T33 (TODO 清理) | 低 | 低 | 无 |

### 🏗️ 1-3 小时改动

| Task | 难度 | 预期收益 | 风险 |
|------|------|---------|------|
| T2 (Stream body close) | 中 | 高（稳定性） | 中 |
| T3 (熔断器 race) | 中 | 高 | 中 |
| T5 (锁内 JSON marshal) | 中 | 高 | 低 |
| T6 (Provider Transport) | 中 | 极高（吞吐） | 低 |
| T7 (协作 prompt Builder) | 中 | 高 | 无 |
| T8 (Cache fingerprint) | 中 | 中 | 低 |
| T13 (CostTracker O(1)) | 中 | 高 | 中 |
| T14 (Memory ToLower) | 低 | 中 | 无 |
| T15 (Pool status atomic) | 中 | 中 | 中 |
| T17 (DAG condition 锁外) | 中 | 中 | 中 |
| T19 (Scanner buffer pool) | 中 | 中 | 中 |
| T21 (event payload pool) | 中 | 中 | 无 |
| T22 (HITL Builder) | 中 | 中 | 无 |
| T23 (similarity 预计算) | 中 | 中 | 无 |
| T30 (history 滑动窗口) | 中 | 中 | 中 |

### 🏛️ 1 天+ 改动

| Task | 难度 | 预期收益 | 风险 |
|------|------|---------|------|
| T9 (LRU container/list) | 高 | 高 | 中 |
| T10 (Provider typed struct) | 高 | 极高（吞吐） | 高（API 兼容性） |
| T24 (Agent Bus Broadcast 优化) | 中 | 中 | 中 |
| T25 (Cache HNSW) | 高 | 高 | 高（外部依赖） |
| T26 (Cache TTL 清理) | 中 | 中 | 中 |
| T27 (Workflow atomic) | 低 | 中 | 低 |
| T34 (Benchmark 套件) | 中 | 基线 | 无 |

---

## 实施顺序

```
Week 1 (Critical + High 易改)
├── Day 1: T1, T2, T3, T4, T5        # Critical 修复
├── Day 2: T6 (Provider Transport)   # 极高收益
├── Day 3: T7 (协作 prompt) + T11 (HookStats) + T12 (phaseOrder)
└── Day 4: T13 (CostTracker) + T14 (Memory) + T15 (Pool)

Week 2 (Medium + 验证)
├── Day 5: T16-T19 (锁内回调/SSE/sync.Pool)
├── Day 6: T20-T23 (slog/event pool/HITL/similarity)
├── Day 7: T28-T30 (GroupChat/RoleBased/history window)
└── Day 8: 完整验证 + Benchmark 建立
```

---

## 预期成果

| 指标 | 预期改善 |
|------|---------|
| Tool panic crash | **0**（之前任意工具 panic 杀死进程） |
| 高并发 Agent 吞吐 | +30-50%（Transport + LRU + atomic） |
| 序列化延迟 | -50%（typed struct + sync.Pool） |
| 锁竞争 | -80%（mutating status 全部 atomic 化） |
| 长对话 LLM token 成本 | -40%（history 滑动窗口 100 条） |
| Benchmark 覆盖 | 6 个核心模块建立性能基线 |
| Goroutine 泄漏 | **0**（ctx 感知 + timeout 修复） |

---

## 验证

```bash
# 全量构建 + vet
go build ./...
go vet ./...

# 单元测试（每个 Phase 后跑一次）
go test -race -count=1 ./...

# 性能基线（每个 Phase 后跑一次，记录 ns/op 与 allocs/op）
go test -bench=. -benchmem -benchtime=10s ./internal/agent/ ./internal/llm/ ./internal/orchestration/ ./internal/tools/ ./internal/memory/ ./internal/pool/

# 内存基线
go test -bench=BenchmarkAgent -benchmem -memprofile=mem.out ./internal/agent/
go tool pprof -top -alloc_objects mem.out
```

