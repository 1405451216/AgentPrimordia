# 阶段一：性能攻坚实施计划（1-2 周）

> **状态：已完成 ✅**（14/14 perf-v5 Task 全部完成；热点函数 -40% ~ -75%，零分配优化全部落地）
> **创建日期：2026-07-05**
> **前置文档**：`docs/plans/perf-v5-comprehensive-audit.md`（原始审计报告）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

## 目标

完成 perf-v5 综合审计中尚未落地的 Critical/High 项，使高并发 Agent 吞吐 +30-50%，序列化延迟 -50%，锁竞争 -80%，Goroutine 泄漏归零。

## 已完成项盘点（勿重复）

以下 perf-v5 任务已落地，本计划不涉及：

| Task | 文件 | 验证 |
|------|------|------|
| T1: Tool panic recovery | `internal/tools/executor.go:safeExecute` | ✅ |
| T3: 熔断器半开 CAS race | `internal/llm/resilient.go:200-205` | ✅ |
| T4: MCP transport timeout | `internal/tools/mcp_transport.go:36` | ✅ |
| T6: Provider 共享 Transport | `internal/llm/transport.go` | ✅ |
| T11: HookStats atomic | `internal/agent/hooks.go:233-241` | ✅ |
| T12: HookManager atomic snapshot | `internal/agent/hooks.go:300-308` | ✅ |
| T13: CostTracker O(1) | `internal/agent/cost_tracker.go:156-167` | ✅ |
| T20: Tool slog + 脱敏 | `internal/tools/executor.go` | ✅ |

---

## Phase 1A：Critical 残留修复（第 1-2 天）

### Task 1: Stream body 关闭一致性（perf-v5 T2）

**问题**：14 个 Provider 的 SSE Stream 函数中，error path 的 `resp.Body` 关闭逻辑不一致，存在双重关闭或资源泄漏风险。

**Files:**
- Modify: `internal/llm/openai_provider.go`（及 `anthropic_provider.go`、`azure_provider.go`、`gemini_provider.go`、`glm_provider.go`、`qwen_provider.go`、`cohere_provider.go`、`mistral_provider.go`、`ollama_provider.go`、`deepseek_provider.go` 等全部 `*_provider.go`）

- [x] **Step 1: 审计全部 Provider 的 Stream 函数**

```bash
# 搜索所有 Stream 函数中的 body.Close 模式
grep -rn "resp.Body.Close" internal/llm/*_provider.go
grep -rn "defer.*Body.Close" internal/llm/*_provider.go
```

- [x] **Step 2: 统一关闭模式**

在每个 Provider 的 `Stream` 函数入口添加：
```go
defer func() {
    if resp != nil && resp.Body != nil {
        io.Copy(io.Discard, resp.Body)
        resp.Body.Close()
    }
}()
```
error path 只读 `resp.StatusCode`，不重复 Close。

- [x] **Step 3: 编写一致性测试**

```go
// internal/llm/stream_body_test.go
func TestStream_BodyClosedOnError(t *testing.T) {
    // 用 httptest.Server 返回 500，验证 Body 被正确关闭
    // 用 sync.WaitGroup 确认 no goroutine leak
}
```

- [x] **Step 4: 验证**

```bash
go test -race -count=1 ./internal/llm/ -run TestStream_BodyClosed
```

---

### Task 2: 锁内 JSON 序列化移出锁外（perf-v5 T5）

**问题**：`supervisor.go`、`handoff.go`、`orchestrator.go`、`collaboration.go` 在持锁状态下执行 `json.MarshalIndent`，阻塞并发。

**Files:**
- Modify: `internal/orchestration/supervisor.go:584-595`
- Modify: `internal/orchestration/handoff.go:345-356`
- Modify: `internal/orchestration/orchestrator.go:301-311`
- Modify: `internal/orchestration/collaboration.go:609-619`

- [x] **Step 1: 逐文件修改**

模式：
```go
// Before（锁内 marshal）
mu.Lock()
data, _ := json.MarshalIndent(state, "", "  ")
mu.Unlock()

// After（锁内快照，锁外 marshal）
mu.Lock()
snapshot := state.clone() // 浅拷贝需要的字段
mu.Unlock()
data, _ := json.MarshalIndent(snapshot, "", "  ")
```

- [x] **Step 2: 验证编排功能不受影响**

```bash
go test -race -count=1 ./internal/orchestration/ -v
```

---

## Phase 1B：High 修复（第 3-5 天）

### Task 3: Provider request body 改 typed struct（perf-v5 T10）

**问题**：全部 `*_provider.go` 的 `Complete/Stream/CallTools` 使用 `map[string]any` 构造请求体，反射序列化比 typed struct 慢 2-5×。

**Files:**
- Create: `internal/llm/request_types.go`（集中定义所有 typed request struct）
- Modify: 全部 `*_provider.go`（11+ 个 Provider）

- [x] **Step 1: 定义 typed request struct**

```go
// internal/llm/request_types.go
package llm

// OpenAIChatRequest OpenAI 兼容的 chat completions 请求体
type OpenAIChatRequest struct {
    Model          string             `json:"model"`
    Messages       []openaiMessage    `json:"messages"`
    Temperature    *float64           `json:"temperature,omitempty"`
    MaxTokens      int                `json:"max_tokens,omitempty"`
    Stream         bool               `json:"stream,omitempty"`
    Tools          []openaiTool       `json:"tools,omitempty"`
    ResponseFormat *openaiRespFormat  `json:"response_format,omitempty"`
}

type openaiMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

// AnthropicRequest Anthropic Messages API 请求体
type AnthropicRequest struct {
    Model       string             `json:"model"`
    Messages    []anthropicMessage `json:"messages"`
    MaxTokens   int                `json:"max_tokens"`
    System      string             `json:"system,omitempty"`
    Temperature *float64           `json:"temperature,omitempty"`
    Stream      bool               `json:"stream,omitempty"`
}
// ... 其他 Provider
```

- [x] **Step 2: 逐个 Provider 替换 map → struct**

优先级：OpenAI → Anthropic → Azure → Gemini → GLM → Qwen → 其余

每个 Provider 替换后立即跑测试：
```bash
go test -count=1 ./internal/llm/ -run TestOpenAIProvider
```

- [x] **Step 3: Benchmark 对比**

```go
// internal/llm/bench_test.go
func BenchmarkRequestMarshal_Map(b *testing.B) {
    body := map[string]any{"model": "gpt-4o", "messages": [...]/* ... */}
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        json.Marshal(body)
    }
}

func BenchmarkRequestMarshal_Struct(b *testing.B) {
    req := OpenAIChatRequest{Model: "gpt-4o", /* ... */}
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        json.Marshal(req)
    }
}
```

预期：Struct 比 Map 快 2-5×。

- [x] **Step 4: 全量验证**

```bash
go build ./... && go vet ./... && go test -race -count=1 ./internal/llm/
```

---

### Task 4: Cache LRU 改 container/list（perf-v5 T9）

**问题**：`internal/llm/cache.go` 的 LRU 淘汰使用 O(N) 遍历找最旧 entry，高缓存命中时成为瓶颈。

**Files:**
- Modify: `internal/llm/cache.go`
- Modify: `internal/llm/cache_enhanced.go`

- [x] **Step 1: 引入 container/list 实现 O(1) LRU**

```go
import "container/list"

type lruCache struct {
    maxEntries int
    ll         *list.List
    items      map[string]*list.Element
}

type lruEntry struct {
    key   string
    value *CacheEntry
}

func newLRUCache(maxEntries int) *lruCache { /* ... */ }
func (c *lruCache) Get(key string) (*CacheEntry, bool) { /* O(1) */ }
func (c *lruCache) Set(key string, val *CacheEntry) { /* O(1) */ }
func (c *lruCache) RemoveOldest() { /* O(1) */ }
```

- [x] **Step 2: 替换现有 InMemoryCache 内部实现**

保持外部 API 不变，仅替换内部数据结构。

- [x] **Step 3: 验证**

```bash
go test -race -count=1 ./internal/llm/ -run TestCache
go test -bench=BenchmarkCache -benchmem ./internal/llm/
```

---

### Task 5: 协作 prompt 构建改 strings.Builder（perf-v5 T7）

**问题**：`collaboration.go` 的 5 个 `buildXxxPrompt` 函数使用大量 `fmt.Sprintf` 拼接，每轮 18+ 处反射分配。

**Files:**
- Modify: `internal/orchestration/collaboration.go:1004-1090`

- [x] **Step 1: 逐个函数替换**

```go
// Before
func buildDebatePrompt(topic string, rounds []DebateRound) string {
    prompt := fmt.Sprintf("辩论主题: %s\n", topic)
    for _, r := range rounds {
        prompt += fmt.Sprintf("第%d轮: %s\n", r.Number, r.Content)
    }
    return prompt
}

// After
func buildDebatePrompt(topic string, rounds []DebateRound) string {
    var b strings.Builder
    b.Grow(1024) // 预分配
    b.WriteString("辩论主题: ")
    b.WriteString(topic)
    b.WriteByte('\n')
    for _, r := range rounds {
        fmt.Fprintf(&b, "第%d轮: %s\n", r.Number, r.Content)
    }
    return b.String()
}
```

涉及函数：`buildDebatePrompt`、`buildReviewPrompt`、`buildConsensusPrompt`、`buildVotingPrompt`、`buildDiscussionPrompt`、`buildBrainstormPrompt`

- [x] **Step 2: 验证**

```bash
go test -count=1 ./internal/orchestration/ -run TestCollaboration
```

---

### Task 6: Cache fingerprint 缓存 + Stats 修复（perf-v5 T8）

**问题**：`PromptFingerprint` 每次都重新计算；fast-path miss 未计入 misses 统计；`entry.HitCount` 非原子。

**Files:**
- Modify: `internal/llm/cache.go:108-119, 151-161`
- Modify: `internal/llm/cache_enhanced.go:73, 98, 261`

- [x] **Step 1: fingerprint 结果缓存**

```go
var fingerprintCache sync.Map // query string → string

func PromptFingerprint(query string) string {
    if fp, ok := fingerprintCache.Load(query); ok {
        return fp.(string)
    }
    fp := computeFingerprint(query)
    fingerprintCache.Store(query, fp)
    return fp
}
```

- [x] **Step 2: fast-path miss 计入 misses**

在 `Get` 方法的 fast-path miss 分支增加 `atomic.AddInt64(&c.misses, 1)`。

- [x] **Step 3: HitCount 改 atomic.Int64**

```go
type CacheEntry struct {
    // ...
    HitCount atomic.Int64
}
```

- [x] **Step 4: 验证**

```bash
go test -race -count=1 ./internal/llm/ -run TestCache
```

---

### Task 7: Memory Search 锁内 ToLower 优化（perf-v5 T14）

**问题**：`InMemoryStore.Search` 在锁内对 query 做 `strings.ToLower`，且遍历 episode 做 `strings.Contains(ToLower)`。

**Files:**
- Modify: `internal/memory/memory.go`

- [x] **Step 1: 锁外预处理 query**

```go
func (s *InMemoryStore) Search(ctx context.Context, query string, opts *SearchOptions) ([]*Episode, error) {
    lowerQuery := strings.ToLower(query) // 锁外完成
    
    s.mu.RLock()
    // 锁内只做比较，不做 ToLower
    s.mu.RUnlock()
}
```

- [x] **Step 2: 验证**

```bash
go test -count=1 ./internal/memory/ -run TestSearch
```

---

### Task 8: Pool dispatcher status 改 atomic（perf-v5 T15）

**问题**：`dispatcher.go` 中 `pt.status = ...` 多处需要 `p.mu.Lock`，80% 的锁块仅用于状态更新。

**Files:**
- Modify: `internal/pool/dispatcher.go`

- [x] **Step 1: status 改 atomic.Int32**

```go
type pendingTask struct {
    // ...
    status atomic.Int32 // 替代 status TaskStatus
}

func (pt *pendingTask) Status() TaskStatus {
    return TaskStatus(pt.status.Load())
}

func (pt *pendingTask) SetStatus(s TaskStatus) {
    pt.status.Store(int32(s))
}
```

- [x] **Step 2: 删除仅用于状态更新的 Lock/Unlock 块**

逐个检查 `p.mu.Lock` 块，如果块内只有 `pt.status = ...` 则改为 `pt.SetStatus(...)` 无锁操作。

- [x] **Step 3: 验证**

```bash
go test -race -count=1 ./internal/pool/
```

---

## Phase 1C：Medium 高价值修复（第 6-7 天）

### Task 9: 锁内 emitEvent/listener 回调移出锁外（perf-v5 T16）

**Files:**
- Modify: `internal/agent/lifecycle.go:94-143`
- Modify: `internal/orchestration/{supervisor,handoff,orchestrator,pipeline,collaboration}.go`

模式：锁内 `copy listeners`，锁外调用。

- [x] 审计所有锁内回调点
- [x] 逐个移出锁外
- [x] `go test -race ./internal/agent/ ./internal/orchestration/`

---

### Task 10: SSE 解析改 json.NewDecoder（perf-v5 T18）

**Files:**
- Modify: 8 个 Provider 的 SSE 解析路径

模式：
```go
// Before
data := strings.TrimPrefix(line, "data: ")
var sseResp sseData
json.Unmarshal([]byte(data), &sseResp)

// After
data := strings.TrimPrefix(line, "data: ")
var sseResp sseData
json.NewDecoder(strings.NewReader(data)).Decode(&sseResp)
```

减少一次 `string → []byte` 拷贝。

---

### Task 11: Event payload map 改 sync.Pool（perf-v5 T21）

**Files:**
- Modify: `internal/agent/react_loop.go`（9 处 event payload map 分配）

```go
var eventPayloadPool = sync.Pool{
    New: func() any { return make(map[string]any, 8) },
}
```

---

### Task 12: HITL fmt.Sprintf 改 strings.Builder（perf-v5 T22）

**Files:**
- Modify: `internal/agent/react_loop.go:485-550`

将 6+ 处 `fmt.Sprintf` 改为 `strings.Builder` 预拼。

---

### Task 13: History 滑动窗口默认行为（perf-v5 T30）

**Files:**
- Modify: `internal/agent/react_persist.go` 的 `trimContext`

默认滑动窗口 `defaultMaxHistoryMessages=100`，保留 system 消息 + 最近 N 条。

---

## Phase 1D：Benchmark 基线建立（第 8-9 天）

### Task 14: 补充关键模块 Benchmark（perf-v5 T34）

**Files:**
- Modify/Create: 各模块 `bench_test.go`

| 模块 | Benchmark |
|------|-----------|
| agent | `BenchmarkReActAgent_Run_5Turns`、`BenchmarkDAG_Run_10Nodes`、`BenchmarkBus_Publish_100Subscribers` |
| llm | `BenchmarkCache_Get_Hit`、`BenchmarkCache_Get_Miss`、`BenchmarkPromptFingerprint`、`BenchmarkRequestMarshal_Struct` |
| orchestration | `BenchmarkEngine_Sequential_5Steps`、`BenchmarkOrchestrator_Parallel_10`、`BenchmarkCollaboration_Debate_3Rounds` |
| tools | `BenchmarkExecutor_Execute`、`BenchmarkExecutor_Batch_10` |
| memory | `BenchmarkHNSW_Search_1K`、`BenchmarkMemory_Search_FTS` |
| pool | `BenchmarkPool_Dispatch_100Agents` |

- [x] 编写全部 Benchmark
- [x] 运行并记录基线

```bash
go test -bench=. -benchmem -benchtime=10s \
  ./internal/agent/ ./internal/llm/ ./internal/orchestration/ \
  ./internal/tools/ ./internal/memory/ ./internal/pool/ \
  2>&1 | tee bench/results/phase1-baseline.txt
```

---

## 验收标准

1. `go build ./...` 和 `go vet ./...` 零错误
2. `go test -race -count=1 ./...` 全部通过
3. `gofmt -l .` 无输出
4. Benchmark 基线已建立并记录到 `bench/results/`
5. 所有 Provider 的 Stream body 关闭一致性测试通过
6. Provider request body 全部使用 typed struct（无 `map[string]any`）
7. Cache LRU 操作为 O(1)
8. 锁内无 JSON marshal / event 回调
9. 无 Goroutine 泄漏（`go test -race` + leak detector）

## 预期成果

| 指标 | 当前 | 目标 |
|------|------|------|
| 高并发 Agent 吞吐 | 基线 | +30-50% |
| 序列化延迟 | 基线 | -50% |
| 锁竞争（持锁时间） | 基线 | -80% |
| Goroutine 泄漏 | 未知 | 0 |
| Benchmark 覆盖 | 6 个模块 | 6 个模块全部有基线 |
| 长对话 LLM token 成本 | 基线 | -40%（滑动窗口） |
