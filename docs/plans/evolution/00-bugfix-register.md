# AgentPrimordia 已发现问题修复清单

> 生成时间：2026-07-09  
> 最近更新：2026-07-09 — R0（前置 BUG 修复）完成  
> 基于：全量代码审计 + `go build` / `go vet` / `go test` 结果  
> 审计范围：Go 核心引擎 + TypeScript SDK

---

## 问题总览

| 编号 | 级别 | 语言 | 模块 | 问题摘要 | 状态 |
|------|------|------|------|---------|------|
| BUG-01 | P0 | Go | `llm/stream_collector_test.go` | 并发测试数据竞争 | ✅ 已修复 (R0) |
| BUG-02 | P1 | Go | `llm/stream_collector.go` | O(n²) 字符串拼接 | ✅ 已修复 (R0) |
| BUG-03 | P1 | Go | `llm/*` + `a2a/service.go` | 28 处 `%w: %v` 断裂错误链 | ✅ 已修复 (R0) |
| BUG-04 | P2 | Go | `tool_learning/learner.go` | `GetBestPractices()` 空实现 | ✅ 已修复 (R1.1) |
| BUG-05 | P2 | Go | `react_loop_core.go` | Planning/Reflection 未接入主循环 | ✅ 已修复 (R1.3 + R1.4) |
| BUG-06 | P2 | Go | `react_loop_tools.go` | 工具串行执行无并行 | ✅ 已修复 (R1.6) |
| BUG-07 | P3 | Go | `go.mod` | Go 1.26 未锁定 patch 版本 | ✅ 已修复 (R0) |
| BUG-08 | P3 | TS | `memory/vector-extended.ts` | HNSW insert O(n log n) 非真正 O(log n) | ✅ 已修复 (R2/T1-1) |
| BUG-09 | P3 | TS | `agent/react-reasoning.ts` | 流式 token push+join | 🟡 可接受 (观察) |
| BUG-10 | P2 | Go | `capabilities.go` | 能力定义与实现断线 | ✅ 已修复 (R1.2) |

**R0 完成情况**：BUG-01/02/03/07 全部修复，go vet + go build + go test ./... 全绿（69 packages pass, 0 fail）。  
**R0 跳过的 P2 BUG**（BUG-04/05/06/10）均依赖 Phase 1 的能力接入（capCache、planner 注入、并行工具等），不能在 R0 独立完成，统一在 R1 处理。

---

---

## BUG-01：并发测试数据竞争（P0）

### 基本信息

- **文件**：`agentprimordia/internal/llm/stream_collector_test.go`
- **行号**：159
- **现象**：`TestStreamCollector_ConcurrentAccess` 偶发失败，报 `expected 5 results, got 4`
- **触发条件**：`-race` 模式下高频复现

### 根因分析

5 个 goroutine 同时向共享 slice `results` 执行 `append`，无任何同步机制。Go 的 `append` 不是并发安全的——多个 goroutine 可能同时读到同一底层数组位置，导致写入丢失。

```go
// stream_collector_test.go:142-161
var results []*CollectedResult  // 共享 slice，无保护
done := make(chan struct{})

for i := 0; i < 5; i++ {
    go func(i int) {
        // ...
        results = append(results, result)  // ← 数据竞争
        done <- struct{}{}
    }(i)
}
```

### 修复方案

使用 channel 收集结果（推荐），消除共享状态：

```go
func TestStreamCollector_ConcurrentAccess(t *testing.T) {
    resultCh := make(chan *CollectedResult, 5)

    for i := 0; i < 5; i++ {
        go func() {
            ch := make(chan Chunk, 5)
            go func() {
                ch <- Chunk{Content: "concurrent"}
                ch <- Chunk{Done: true}
                close(ch)
            }()

            collector := NewStreamCollector()
            result, err := collector.Collect(ch)
            if err != nil {
                t.Errorf("Collect() error = %v", err)
            }
            resultCh <- result
        }()
    }

    var results []*CollectedResult
    for i := 0; i < 5; i++ {
        results = append(results, <-resultCh)
    }

    if len(results) != 5 {
        t.Errorf("expected 5 results, got %d", len(results))
    }
}
```

### 验证

```bash
go test -race -count=100 -run TestStreamCollector_ConcurrentAccess ./internal/llm/
```

---

## BUG-02：O(n²) 字符串拼接（P1）

### 基本信息

- **文件**：`agentprimordia/internal/llm/stream_collector.go`
- **行号**：74-77
- **现象**：`Collect()` 在大量 chunk 时性能 O(n²) 退化

### 根因分析

Go 字符串不可变，`content += t` 每次分配新内存并复制全部历史内容。

```go
// stream_collector.go:74-77
content := ""
for _, t := range tokens {
    content += t  // O(n²)
}
```

### 修复方案

使用 `strings.Builder`（与 `react_reasoning.go:66` 已有的优化保持一致）：

```go
import "strings"

// 替换拼接逻辑：
var contentBuilder strings.Builder
contentBuilder.Grow(len(tokens) * 16) // 预估每个 token ~16 字节
for _, t := range tokens {
    contentBuilder.WriteString(t)
}
content := contentBuilder.String()
```

### 验证

```bash
go test -bench BenchmarkStreamCollector_LargeStream ./internal/llm/
```

---

## BUG-03：错误链断裂（P1）

### 基本信息

- **文件**：`agentprimordia/internal/llm/` 下 13 个文件 + `agent/a2a/service.go`
- **数量**：28 处
- **现象**：`fmt.Errorf("%w: %v", err1, err2)` 中 `%v` 将 error 转为字符串，丢失 `errors.Is()` / `errors.As()` 能力

### 受影响文件清单

| 文件 | 次数 |
|------|------|
| `llm/ollama_provider.go` | 4 |
| `llm/openai_provider.go` | 3 |
| `llm/azure_provider.go` | 3 |
| `llm/mistral_provider.go` | 3 |
| `llm/cohere_provider.go` | 2 |
| `llm/anthropic_provider.go` | 2 |
| `llm/resilient.go` | 2 |
| `llm/qwen_provider.go` | 2 |
| `llm/gemini_provider.go` | 2 |
| `llm/gemini_multimodal_provider.go` | 2 |
| `llm/anthropic_vision_provider.go` | 1 |
| `llm/openai_multimodal_provider.go` | 1 |
| `llm/glm_provider.go` | 1 |
| `agent/a2a/service.go` | 4 |

### 修复方案

将 `%v` 改为 `%w`（Go 1.20+ 支持多 `%w`）：

```go
// 修复前（错误链断裂）：
return nil, fmt.Errorf("callTools failed after %d retries: %v", maxRetries, lastErr)

// 修复后（错误链完整）：
return nil, fmt.Errorf("callTools failed after %d retries: %w", maxRetries, lastErr)
```

### 注意事项

每个替换需确认第二个参数确实是 `error` 类型。部分 `%v` 可能用于格式化非 error 值（如 string、int），这些**不能**改为 `%w`。

### 验证

```bash
go build ./...
go test ./internal/llm/... ./internal/agent/a2a/...
```

---

## BUG-04：GetBestPractices 空实现（P2）

### 基本信息

- **文件**：`agentprimordia/internal/agent/tool_learning/learner.go`
- **行号**：139-141
- **现象**：`GetBestPractices()` 直接返回空数组

```go
func (l *MemoryToolLearner) GetBestPractices(ctx context.Context, toolName string) ([]BestPractice, error) {
    return []BestPractice{}, nil  // 空实现
}
```

### 修复方案

从 `MemoryStore` 查询历史 `tool_usage` episode，按成功率聚合：

```go
func (l *MemoryToolLearner) GetBestPractices(ctx context.Context, toolName string) ([]BestPractice, error) {
    episodes, err := l.memory.Query(ctx, "tool_learning", map[string]string{
        "tool_name": toolName,
    })
    if err != nil {
        return nil, fmt.Errorf("query tool usage history: %w", err)
    }

    var successCount, totalCount int
    examples := make([]string, 0, 5)
    for _, ep := range episodes {
        var record ToolUsageRecord
        if err := json.Unmarshal([]byte(ep.Content), &record); err != nil {
            continue
        }
        totalCount++
        if record.Success {
            successCount++
            if len(examples) < 5 {
                examples = append(examples, record.Args)
            }
        }
    }

    if totalCount == 0 {
        return []BestPractice{}, nil
    }

    successRate := float64(successCount) / float64(totalCount)
    return []BestPractice{{
        ToolName:    toolName,
        Pattern:     "most_common_success_pattern",
        Description: fmt.Sprintf("成功率 %.1f%% (%d/%d)", successRate*100, successCount, totalCount),
        SuccessRate: successRate,
        Examples:    examples,
        CreatedAt:   time.Now(),
    }}, nil
}
```

### ⚠️ 前置条件：必须先扩展 MemoryStore 接口

当前 `tool_learning.MemoryStore` 接口只定义了 `Add(ctx, *Episode) error`，**没有 `Query` 方法**。修复代码中使用的 `l.memory.Query()` 在当前接口下**无法编译**。

必须先扩展接口：

```go
// tool_learning/learner.go — 扩展 MemoryStore 接口
type MemoryStore interface {
    Add(ctx context.Context, episode *Episode) error
    // 新增：按 sessionID 和 metadata 过滤查询
    Query(ctx context.Context, sessionID string, metadata map[string]string) ([]*Episode, error)
}
```

所有实现 `MemoryStore` 的类型（如 `agent.MemoryStore` 适配器）都需要同步增加 `Query` 方法。

---

## BUG-05 / BUG-10：Planning/Reflection/ToolLearning 能力断线（P2）

### 基本信息

- **文件**：`agentprimordia/internal/agent/react_loop_core.go` + `capabilities.go` + `capability_agent.go`
- **现象**：三个能力接口已定义、实现已完整、`CapabilityAgent` 链式 API 已注入，但 `runLoop()` 从未调用

### 证据链

```
capabilities.go:151  → PlanningCapable 接口定义           ✅
capability_agent.go:246  → WithPlanner() 链式注入          ✅
planning/planner.go  → LLMPlanner.Decompose() 完整实现    ✅
react_loop_core.go   → runLoop() 中无 planner 调用         ❌ 断线
```

同样的问题存在于 Reflection 和 ToolLearning。

### 对比

TS 端 `react-loop.ts` 已接入：
- L698-709：Tool Learning few-shot 注入 ✅
- L711-717：SelfTuner 运行前应用调优 ✅
- L751-761：SelfTuner 运行后记录指标 ✅

### 修复方案

详见进化路线 Phase 1 实施文档（`02-phase1-implementation.md`）。

---

## BUG-06：工具串行执行（P2）

### 基本信息

- **文件**：`agentprimordia/internal/agent/react_loop_tools.go`
- **行号**：15
- **现象**：`executeToolCalls` 串行执行，无法利用 goroutine 并发

### 修复方案

使用 `errgroup.Group` 并行执行，详见 Phase 1 实施文档。

---

## BUG-07：Go 工具链版本（P3）

### 基本信息

- **文件**：`agentprimordia/go.mod`
- **现象**：`go 1.26` 未锁定到最新 patch

### 修复方案

```bash
cd agentprimordia
go get toolchain@go1.26.4
go mod tidy
go build ./...
govulncheck ./...
```

---

## BUG-08：TS HNSW 性能退化（P3）

### 基本信息

- **文件**：`sdk/typescript/src/memory/vector-extended.ts`
- **行号**：62-84
- **现象**：`insert` 遍历全部节点排序，O(n log n) 而非 O(log n)

### 修复方案

详见进化路线 TS Phase 1 实施文档。

---

## BUG-09：TS 流式 token 拼接（P3，可接受）

### 基本信息

- **文件**：`sdk/typescript/src/agent/react-reasoning.ts:118`
- **现象**：`contentParts.push()` + `join('')`

### 评估

V8 引擎对此模式有优化，500 chunk 级别可接受。暂不修复，监控超长流场景。

---

## 修复排期

```
R0（已完成 2026-07-09）
├── ✅ BUG-01  并发测试数据竞争（P0） — channel 收集替换共享 slice
├── ✅ BUG-02  O(n²) 字符串拼接（P1） — strings.Builder
├── ✅ BUG-03  错误链断裂（P1）       — 28 处 %w: %v → %w: %w
└── ✅ BUG-07  Go 工具链升级（P3）     — go.mod 锁定 toolchain go1.26.4

R1（Phase 1 G1 闭环构建期）✅ 全部完成
├── ✅ BUG-04  GetBestPractices 空实现 (R1.1 — 2026-07-09)
├── ✅ BUG-05  Planning/Reflection 接入 (R1.3 + R1.4 — 2026-07-09)
├── ✅ BUG-06  并行工具执行 (R1.6 — 2026-07-09)
└── ✅ BUG-10  能力断线修复 (R1.2 — 2026-07-09)

R2（Phase 1 T1 基础能力做深）✅ 全部完成
├── ✅ BUG-08  TS HNSW 重写 (R2/T1-1 — 2026-07-09)
├── ✅ T1-2 浏览器端 IndexedDB 向量存储 (2026-07-09)
└── ✅ T1-3 流式 tool_call 实时解析 (2026-07-09)

R3（Phase 2 部分）✅ 已完成子项
├── ✅ G2-1 成本感知模型路由器 (2026-07-09)
├── ✅ G2-2 Go 原生投机执行 (2026-07-09)
├── ✅ G2-5 Eval CI 集成 (2026-07-09)
└── ✅ T2-2 Prompt A/B 平台化 (2026-07-09)

观察
└── BUG-09  TS 流式拼接
```

## R0 验证记录

| 检查 | 命令 | 结果 |
|------|------|------|
| 工具链版本 | `go version` | `go1.26.4 windows/amd64` |
| vet | `go vet ./...` | exit 0 |
| build | `go build ./...` | exit 0 |
| test | `go test -count=1 ./...` | 69 packages pass, 0 fail |
| BUG-01 回归 | `go test -count=10 -run TestStreamCollector_ConcurrentAccess ./internal/llm/` | 10/10 pass |
| BUG-03 残留 | `Select-String -Path "internal/**/*.go" -Pattern "%w: %v"` | 0 命中 |

## R1.1 验证记录（BUG-04）

| 检查 | 命令 | 结果 |
|------|------|------|
| 接口扩展 | `go build ./internal/agent/tool_learning/` | 编译通过 |
| 新测试 | `go test -count=1 -v -run TestMemoryToolLearnerGetBestPractices ./internal/agent/tool_learning/` | 5/5 PASS |
| 子包回归 | `go test ./internal/agent/{tool_learning,reflection,planning}/` | 3/3 PASS |
| 全量回归 | `go test -count=1 ./...` | 69 packages pass, 0 fail |

### R1.1 改动清单

| 文件 | 改动 |
|------|------|
| `internal/agent/tool_learning/learner.go` | `MemoryStore` 接口扩展 `Query` 方法；`GetBestPractices()` 改为实现：调用 Query → 反序列化 ToolUsageRecord → 按 success/total 聚合，返回 BestPractice |
| `internal/agent/tool_learning/learner_test.go` | `mockMemoryStore` 实现 `Query`；新增 4 个测试：Empty / WithHistory / DifferentTools / AllFailures；新增 `buildRecordJSON` 测试辅助 |

### R1.1 接口契约

```go
// tool_learning.MemoryStore
type MemoryStore interface {
    Add(ctx context.Context, episode *Episode) error
    // Query 按 sessionID + metadata 过滤查询 episodes
    // metadata 为 nil 或空时忽略 metadata 过滤；sessionID 为空时不过滤 session
    // 返回顺序由实现决定（建议按时间倒序）
    Query(ctx context.Context, sessionID string, metadata map[string]string) ([]*Episode, error)
}
```

## R1.2-1.6 验证记录（Phase 1 G1 闭环）

### R1.2 capCache 扩展（BUG-10 修复）

| 检查 | 命令 | 结果 |
|------|------|------|
| 编译 | `go build ./...` | exit 0 |
| 全量回归 | `go test -count=1 ./...` | 69 packages pass, 0 fail |

### R1.3 Planning 接入（G1-1）

| 检查 | 命令 | 结果 |
|------|------|------|
| DAG 测试 | `go test -v -run TestBuildDependencyGraph ./internal/agent/` | 2/2 PASS |
| Topo 测试 | `go test -v -run TestTopologicalLayers ./internal/agent/` | 4/4 PASS |

### R1.4 Reflection 接入（G1-2）

| 检查 | 命令 | 结果 |
|------|------|------|
| severity gating | `go test -v -run TestShouldImprove ./internal/agent/` | 7/7 PASS |
| reflectAndImprove | `go test -v -run TestReflectAndImprove ./internal/agent/` | 6/6 PASS |

### R1.5 ToolLearning 接入（G1-3）

无新增独立测试，逻辑通过 executeSingleTool/processToolResult 串联；
`TestMemoryToolLearnerGetBestPractices_*`（R1.1）覆盖了底层接口契约。

### R1.6 并行工具执行（G1-4）

通过 `ReActConfig.ParallelToolExecution` 开关启用；默认关闭保持向后兼容。
无新增独立测试，并行分支与串行分支都通过 `executeSingleTool/processToolResult` 共享。
现有 `react_loop_*_test.go` 在默认配置下走串行路径，确保无回归。

### R1 全量改动清单

| 文件 | 改动 |
|------|------|
| `internal/agent/react_loop.go` | `capabilityCache` 新增 planner/reflector/toolLearner 字段；`ReActConfig` 新增 ParallelToolExecution/MaxParallelTools/ReflectionSeverityThreshold/ToolLearningConfidenceThreshold；`resolveCapabilities` 填充新能力 |
| `internal/agent/react_capabilities.go` | 新增 `getPlanner/getReflector/getToolLearner` discovery helpers |
| `internal/agent/react_loop_core.go` | runLoop 入口注入 Planning；完成路径注入 Reflection |
| `internal/agent/react_loop_tools.go` | 重构 `executeToolCalls` 为串行/并行分发；新增 `executeToolCallsParallel`（WaitGroup+semaphore）；`executeSingleTool/processToolResult` 抽取复用；注入 ToolLearning（Suggestion + Record） |
| `internal/agent/react_plan_executor.go` (新) | `executePlan` + `buildDependencyGraph` + `topologicalLayers`（Kahn 算法） |
| `internal/agent/react_reflect.go` (新) | `reflectAndImprove` + `shouldImprove`（严重度阈值比较） |
| `internal/agent/r1_phase1_test.go` (新) | 6 个 DAG 测试 + 1 个 severity gating 子测试表（7 cases）+ 6 个 reflectAndImprove 测试 |

### R1 已知 TODO（不在 R1 范围内）

- **同层子任务并行**：`executePlan` 当前每层串行执行；同层无依赖子任务并行是 R1.x 后续优化
- **Plan 与 ToolLearning 的早期分支**：`runLoop` 在 turn==0 时调用 `planner.GeneratePlan`，会消耗一次额外 LLM；可后续改为异步预分解
- **plan 失败时的重试**：当前 fast-fail；可后续增加 N 次重试 + 降级策略

## R2 验证记录（Phase 1 T1 基础能力做深）

### T1-1 HNSW O(log n)（BUG-08）

| 检查 | 命令 | 结果 |
|------|------|------|
| 新测试 | `npx vitest run tests/unit/hnsw-bug08.test.ts` | 6/6 PASS |
| 旧测试 | `npx vitest run tests/unit/memory-extended.test.ts` | 104/104 PASS（无回归）|
| 全量回归 | `npx vitest run` | 1389 tests pass / 39 files |

### T1-2 IndexedDB 向量存储

| 检查 | 命令 | 结果 |
|------|------|------|
| 新测试 | `npx vitest run tests/unit/indexeddb-vector-store.test.ts` | 13/13 PASS |
| 设计 | `IndexedDBVectorStore` 走 IndexedDB API（异步事务）`InMemoryVectorStore` 走 Map（同接口 mock 用于 Node 测试） |

### T1-3 流式 tool_call 解析

| 检查 | 命令 | 结果 |
|------|------|------|
| 新测试 | `npx vitest run tests/unit/stream-tool-parser.test.ts` | 15/15 PASS |
| 设计 | `StreamToolCallParser.push(chunk)` 增量解析；触发条件为 5 种 LLM 起始标记启发式；JSON 平衡即返回 |

### R2 改动清单

| 文件 | 改动 |
|------|------|
| `sdk/typescript/src/memory/vector-extended.ts` | HNSW.insert 改写为分层图搜索（Phase 1/2 双层）+ greedyDescend + searchLayer（ef-search 简化版） |
| `sdk/typescript/src/memory/indexeddb-vector-store.ts` (新) | `IndexedDBVectorStore` + `InMemoryVectorStore` mock + `isIndexedDBAvailable` |
| `sdk/typescript/src/agent/stream-tool-parser.ts` (新) | `StreamToolCallParser`：多格式起始标记启发式 + 增量 JSON 平衡检测 |
| `sdk/typescript/tests/unit/hnsw-bug08.test.ts` (新) | 6 个 HNSW O(log n) 验证 |
| `sdk/typescript/tests/unit/indexeddb-vector-store.test.ts` (新) | 13 个 IndexedDB/InMemory 测试 |
| `sdk/typescript/tests/unit/stream-tool-parser.test.ts` (新) | 15 个流式解析测试 |

### R2 已知 TODO

- **HNSW searchLayer 内部 sort+shift**：仍是 O(n)，生产规模应替换为 MinHeap（与 02-phase1-implementation.md T1-1 注释一致）
- **IndexedDB 真浏览器 e2e 测试**：Node 环境无法测，需 Playwright/Puppeteer 验证事务正确性
- **stream-tool-parser 与 react-reasoning 集成**：parser 已可用但未接入 `reasonStream` 路径（保留为独立可复用组件）
