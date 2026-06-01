# Phase 1-D: CodeCast 对齐 + 框架增强 — 实施计划

> **Spec**: [docs/specs/2026-05-28-phase1d-cast-alignment-design.md](../specs/2026-05-28-phase1d-cast-alignment-design.md)
> **日期**: 2026-05-28 | **更新**: 2026-05-29
> **状态**: ✅ Completed（全部 8 个 Task 已实现并通过测试）
> **前置条件**: Phase 1 MVP (Tasks 5-9) 已完成，96 tests 全通过

---

## 总览

| # | Task | 模块 | 来源 | 复杂度 | 预估测试数 | 实际测试数 | 状态 |
|:-:|------|------|:----:|:------:|:----------:|:----------:|:----:|
| 10 | OpenAI HTTP Provider | `internal/llm/` | CC 提取 | ⭐⭐⭐ | ~12 | 16 | ✅ |
| 11 | FileLock Manager | `internal/concurrency/` | CC 提取 | ⭐⭐ | ~8 | 11 | ✅ |
| 12 | Scope Policy 权限系统 | `internal/tools/` | CC 提取 | ⭐⭐ | ~10 | 12 | ✅ |
| 13 | Enhanced Memory Store | `internal/memory/` | CC 提取 | ⭐⭐⭐ | ~14 | 12 | ✅ |
| 14 | Context Window Manager | `internal/agent/` | CC 提取 | ⭐ | ~6 | 8 | ✅ |
| 15 | Resilient Provider | `internal/llm/` | 新增 | ⭐⭐⭐ | ~10 | 15 | ✅ |
| 16 | Metrics 可观测性 | `internal/metrics/` | 新增 | ⭐⭐ | ~8 | 15 | ✅ |
| 17 | Checkpoint 接口预留 | `internal/persist/` | 新增 | ⭐ | ~4 | 10 | ✅ |

**预估新增**: ~14 个文件，~2500 行代码，**~72 个新测试**
**实际新增**: ~20 个文件，~3500 行代码，**~99 个新测试**（超出预估 37%）

---

## Task 10: OpenAI Compatible HTTP Provider (P0)

**来源**: [CodeCast-desktop/agent_engine.go#L345-L482](../../../CodeCast-desktop/agent_engine.go#L345-L482) 的 `callLLM()` 方法
**目标**: 让 AP 能连接真实 LLM API（OpenAI / DeepSeek / Moonshot / GLM / Ollama 等）

### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 创建 | `agentprimordia/internal/llm/openai_provider.go` | 核心实现 (~200行) |
| 创建 | `agentprimordia/internal/llm/openai_test.go` | 测试 (~300行) |
| 修改 | `agentprimordia/pkg/llm.go` | 导出新类型 |

### 实现要点

```go
// internal/llm/openai_provider.go
package llm

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

type OpenAIProvider struct {
    config     Config
    client     *http.Client
}

func NewOpenAIProvider(cfg Config) (*OpenAIProvider, error) {
    baseURL := cfg.BaseURL
    if baseURL == "" {
        baseURL = "https://api.openai.com/v1"
    }
    cfg.BaseURL = baseURL
    
    return &OpenAIProvider{
        config: cfg,
        client: &http.Client{Timeout: 120 * time.Second},
    }, nil
}
```

**必须实现的 Provider 接口方法**:

1. **Complete(ctx, req)** → POST `/chat/completions`
   - 将 `CompletionRequest.Messages[]ChatMessage` 转为 OpenAI API 格式
   - 解析响应：choices[0].message.content + choices[0].message.tool_calls
   - 返回 `*CompletionResponse` 或 error
   - 错误处理：HTTP 非200 → 返回带状态码的 error；API error body → 解析后返回

2. **Stream(ctx, req)** → POST `/chat/completions` + stream:true
   - SSE 流式解析：data: {...}\n\n 格式
   - 返回 `<-chan Chunk`，每个 Chunk 包含 content 片段
   - Done=true 时关闭 channel
   - context 取消时清理资源

3. **CallTools(ctx, req)** → POST `/chat/completions` + tools 参数
   - 构建 tools 数组（ToolDefinition → OpenAI function format）
   - 解析 tool_calls 响应
   - 返回 `*ToolCallResponse`

4. **Embeddings(ctx, texts)** → POST `/embeddings`
   - 可选实现，返回简单向量或 error "not supported"

5. **Info()** → 返回 ModelInfo（从 config 构造）

### 关键设计决策

- **零外部依赖**: 只用 `net/http` + `encoding/json`
- **请求体大小限制**: 默认 10MB（防止内存溢出）
- **User-Agent**: `AgentPrimordia/1.0`
- **超时**: 可通过 http.Client 配置，默认 120s
- **错误格式**:
  ```go
  type APIError struct {
      Code    string `json:"code"`
      Message string `json:"message"`
      Type    string `json:"type"`
  }
  ```

### 测试用例清单（使用 httptest.Server）

```
TestOpenAIProvider_Complete_Success          // 正常 200 响应
TestOpenAIProvider_Complete_WithToolCalls     // 带 function_calling 的响应
TestOpenAIProvider_Complete_APIError          // API 返回错误 (error 字段)
TestOpenAIProvider_Complete_HTTPError         // HTTP 500/429
TestOpenAIProvider_Complete_InvalidJSON       // 响应体非 JSON
TestOpenAIProvider_Stream_Basic              // SSE 流式返回多个 chunk
TestOpenAIProvider_Stream_DoneSignal         // [DONE] 信号正确终止
TestOpenAIProvider_Stream_ContextCancel       // context cancel 中断流
TestOpenAIProvider_CallTools_Success          // 工具调用模式正常
TestOpenAIProvider_Embeddings_NotSupported    // embeddings 返回未实现错误
TestOpenAIProvider_Info                      // 返回正确的 ModelInfo
TestOpenAIProvider_NewWithDefaults           // 默认 BaseURL 为 openai.com
TestOpenAIProvider_CustomBaseURL             // 自定义 URL (deepseek/moonshot)
```

---

## Task 11: FileLock Manager (P0)

**来源**: [CodeCast-desktop/agent.go#L104-L155](../../../CodeCast-desktop/agent.go#L104-L155) 的文件锁机制
**目标**: 防止多 Agent 并发写入同一文件导致数据冲突

### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 创建 | `agentprimordia/internal/concurrency/filelock.go` | 核心实现 (~100行) |
| 创建 | `agentprimordia/internal/concurrency/filelock_test.go` | 测试 (~200行) |

### 实现要点

```go
// internal/concurrency/filelock.go
package concurrency

import (
    "sync"
)

type FileLockManager struct {
    mu    sync.Mutex
    locks map[string]*sync.Mutex
}

func NewFileLockManager() *FileLockManager {
    return &FileLockManager{
        locks: make(map[string]*sync.Mutex),
    }
}

// Acquire 阻塞获取文件写锁
func (m *FileLockManager) Acquire(path string) {
    m.mu.Lock()
    lk, exists := m.locks[path]
    if !exists {
        lk = &sync.Mutex{}
        m.locks[path] = lk
    }
    m.mu.Unlock()
    lk.Lock()
}

// Release 释放文件写锁并清理 map 条目
func (m *FileLockManager) Release(path string) {
    m.mu.Lock()
    lk, exists := m.locks[path]
    m.mu.Unlock()
    
    if exists {
        lk.Unlock()
        m.mu.Lock()
        delete(m.locks, path)
        m.mu.Unlock()
    }
}

// TryAcquire 非阻塞尝试获取，成功返回 true
func (m *FileLockManager) TryAcquire(path string) bool { ... }

// ValidateScopes 校验批量任务的作用域不重叠
// 规则：
//   - 至多一个任务可以有空 scope（全局写权限）
//   - 任何两个非空 scope 不能有重叠（一个路径是另一个的前缀）
func ValidateScopes(scopes [][]string) error { ... }
```

### 测试用例清单

```
TestFileLock_AcquireAndRelease            // 基本 acquire/release 成对
TestFileLock_ConcurrentSameFile          // 10 goroutine 竞争同一文件锁（串行化验证）
TestFileLock_ConcurrentDifferentFiles     // 不同文件可并行（并发性验证）
TestFileLock_TryAcquire_Success           // 无竞争时 TryAcquire 返回 true
TestFileLock_TryAcquire_Failed           // 已持有时 TryAcquire 返回 false
TestValidateScopes_NoOverlap              // 不重叠的 scope 通过校验
TestValidateScopes_PrefixConflict         // "/a/b" 和 "/a" 冲突检测
TestValidateScopes_MultipleEmptyScope     // 多个空 scope 报错（只能有一个全局权限）
TestValidateScopes_SingleEmptyScope       // 单独空 scope 允许
TestValidateScopes_EmptyScopesList        // 空 scope 列表允许
```

---

## Task 12: Scope Policy 权限系统 (P0)

**来源**: [CodeCast-desktop/agent_tools.go#L55-L67](../../../CodeCast-desktop/agent_tools.go#L55-L67) 的 `canWriteFile()` + [agent.go#L157-L208](../../../CodeCast-desktop/agent.go#L157-L208) 的 `ValidateFilesScopes()`
**目标**: 每个 Agent 只能操作被授权的文件范围

### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 创建 | `agentprimordia/internal/tools/scope.go` | 接口+实现 (~120行) |
| 创建 | `agentprimordia/internal/tools/scope_test.go` | 测试 (~220行) |

### 实现要点

```go
// internal/tools/scope.go
package tools

type ScopePolicy interface {
    Allow(agentID, resource string) bool
    Validate(agentScopes map[string][]string) error
}

type FileScopePolicy struct {
    mu         sync.RWMutex
    agentScopes map[string][]string // agentID -> allowed paths
}

func NewFileScopePolicy() *FileScopePolicy
func (p *FileScopePolicy) SetScope(agentID string, paths []string)
func (p *FileScopePolicy) GetScope(agentID string) []string
func (p *FileScopePolicy) Allow(agentID, resource string) bool
func (p *FileScopePolicy) Validate(agentScopes map[string][]string) error
```

**Allow 判断逻辑**:
1. 如果 agentID 没有注册 scope → **拒绝**
2. 如果 agent 的 scope 包含空字符串 → **允许所有**（全局权限）
3. 否则检查 resource 是否在 agent 的 scope 路径列表中（前缀匹配）

**Validate 规则**（复用 Task 11 的 ValidateScopes）:
- 至多一个 agent 有空 scope
- 任意两个 agent 的非空 scope 不能有路径重叠

### 测试用例清单

```
TestScopePolicy_Allow_ExactMatch           // 精确匹配路径
TestScopePolicy_Allow_PrefixMatch          // 前缀匹配（scope="/src/" 允许 "/src/main.go"）
TestScopePolicy_Allow_GlobalPermission      // 空 scope 允许所有路径
TestScopePolicy_Allow_UnregisteredAgent     // 未注册 agent 被拒绝
TestScopePolicy_Allow_OutOfScope           // 超出范围的路径被拒绝
TestScopePolicy_SetAndGetScope             // 设置和读取 scope
TestScopePolicy_Validate_NoConflicts       // 不冲突的 scopes 通过
TestScopePolicy_Validate_PathOverlap        // 路径重叠报错
TestScopePolicy_Validate_TwoGlobalScopes   // 多个全局权限报错
TestScopePolicy_ConcurrentAccess           // 并发读写安全
```

---

## Task 13: Enhanced Memory Store (P1)

**来源**: [CodeCast-desktop/memory.go](../../../CodeCast-desktop/memory.go) 全文 611 行（AP 当前版本仅基础 CRUD）
**目标**: 补齐 topics/importance/cleanup/timeline 等能力

### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 修改 | `agentprimordia/internal/memory/types.go` | Episode 增加 Topics/Importance 字段 |
| 修改 | `agentprimordia/internal/memory/sqlite.go` | 新增强方法 |
| 修改 | `agentprimordia/internal/memory/memory_test.go` | 补充测试 |

### Episode 结构体变更

```go
type Episode struct {
    ID        string            `json:"id"`
    SessionID string            `json:"session_id"`
    Role      string            `json:"role"`
    Content   string            `json:"content"`
    Summary   string            `json:"summary,omitempty"`
    Topics    string            `json:"topics,omitempty"`      // 🆕 逗号分隔标签
    Importance float64          `json:"importance,omitempty"`   // 🆕 0.0-1.0 评分
    Metadata  map[string]string `json:"metadata,omitempty"`
    CreatedAt string            `json:"created_at"`
}
```

### SQLite Schema 变更

```sql
ALTER TABLE episodes ADD COLUMN topics TEXT DEFAULT '';
ALTER TABLE episodes ADD COLUMN importance REAL DEFAULT 0;

-- 新索引
CREATE INDEX IF NOT EXISTS idx_episodes_topics ON episodes(topics);
CREATE INDEX IF NOT EXISTS idx_episodes_importance ON episodes(importance);
```

### 新增方法

```go
// UpdateSummary 更新 episode 的摘要和标签
func (s *SQLiteStore) UpdateSummary(id string, summary, topics string) error

// SetImportance 设置重要性评分
func (s *SQLiteStore) SetImportance(id string, importance float64) error

// SearchByTag 按标签搜索 episode
func (s *SQLiteStore) SearchByTag(tag string, opts *SearchOptions) ([]*Episode, error)

// GetImportant 获取高重要性 episode（按 threshold 过滤）
func (s *SQLiteStore) GetImportant(threshold float64, limit int) ([]*Episode, error)

// GetTimeline 按日期分组获取时间线
func (s *SQLiteStore) GetTimeline(days int) (map[string][]*Episode, error)

// CleanupExpired 清理过期记忆（保留最近 N 天），返回删除数量
func (s *SQLiteStore) CleanupExpired(maxAgeDays int) (int64, error)

// StartAutoCleanup 启动后台定期清理 goroutine
func (s *SQLiteStore) StartAutoCleanup(stopCh <-chan struct{}) <-chan struct{}

// Stats 返回记忆库统计信息
func (s *SQLiteStore) Stats() (*MemoryStats, error)

type MemoryStats struct {
    TotalEpisodes    int64
    TotalSessions    int64
    OldestEpisode    string
    NewestEpisode    string
    AvgEpisodesPerSession float64
    SizeBytes        int64
}
```

### 测试用例清单

```
TestEnhanced_UpdateSummary                  // 更新 summary + topics
TestEnhanced_SetImportance                 // 设置 importance 评分
TestEnhanced_SearchByTag                   // 按 tag 搜索
TestEnhanced_SearchByTag_NoResults         // 不存在的 tag 返回空
TestEnhanced_GetImportant                  // 获取高重要性条目
TestEnhanced_GetImportant_Empty            // 无高重要性条目
TestEnhanced_GetTimeline                  // 时间线分组查询
TestEnhanced_CleanupExpired                // 清理旧记忆
TestEnhanced_CleanupExpired_None           // 无需清理时返回 0
TestEnhanced_StartAutoCleanup             // 后台清理 goroutine 启停
TestEnhanced_Stats                        // 统计信息正确
TestEnhanced_Topics_Default               // 新 episode topics 默认为空
TestEnhanced_Importance_Range             // importance 在 0-1 范围内
```

---

## Task 14: Context Window Manager (P1)

**来源**: [CodeCast-desktop/agent_engine.go#L319-L326](../../../CodeCast-desktop/agent_engine.go#L319-L326) 的消息截断逻辑
**目标**: ReActLoop 自动管理上下文窗口，防止超出 LLM token 限制

### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 创建 | `agentprimordia/internal/agent/context_window.go` | 策略接口+实现 (~80行) |
| 创建 | `agentprimordia/internal/agent/context_window_test.go` | 测试 (~150行) |
| 修改 | `agentprimordia/internal/agent/react_loop.go` | 集成到 ReActLoop |
| 修改 | `agentprimordia/internal/agent/react_loop_test.go` | 补充集成测试 |

### 实现要点

```go
// internal/agent/context_window.go
package agent

type ContextWindowStrategy interface {
    Trim(messages []Message, maxMessages int) []Message
}

// DefaultStrategy: 保留 system prompt + 最近 N 条消息
type DefaultStrategy struct {
    KeepLast int // 默认 80
}

func NewDefaultStrategy(keepLast int) *DefaultStrategy
func (s *DefaultStrategy) Trim(messages []Message, maxMessages int) []Message
```

**Trim 逻辑** (参考 CC 实现):
```
if len(messages) > maxMessages:
    result = [messages[0]]  // 保留第一条（通常是 system）
    result += messages[-KeepLast:]  // 保留最近 KeepLast 条
    return result
else:
    return messages
```

**ReActLoop 集成点**:
- `ReActConfig` 新增字段: `ContextStrategy ContextWindowStrategy` + `MaxMessages int`
- 每个 turn 结束后、下次调用 LLM 前，自动调用 Trim

### 测试用例清单

```
TestDefaultTrim_UnderLimit                 // 未超限不裁剪
TestDefaultTrim_ExceedLimit                // 超限：保留 system + 最后 N 条
TestDefaultTrim_OnlySystem                 // 只有 system 消息时不裁剪
TestDefaultTrim_EmptyMessages              // 空消息列表
TestDefaultTrim_CustomKeepLast             // 自定义保留数量
TestDefaultTrim_ZeroKeepLast               // keepLast=0 时只保留 system
TestReActLoop_Integration_ContextWindow     // ReActLoop 集成测试（需要 MockLLM）
```

---

## Task 15: Resilient Provider 弹性客户端 (P1)

**全新模块** — CC 没有，生产环境必需
**目标**: LLM 调用的重试 + 回退 + 熔断

### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 创建 | `agentprimordia/internal/llm/resilient.go` | 核心实现 (~180行) |
| 创建 | `agentprimordia/internal/llm/resilient_test.go` | 测试 (~280行) |

### 实现要点

```go
// internal/llm/resilient.go
package llm

import (
    "context"
    "math"
    "sync"
    "time"
)

type ResilientProvider struct {
    primary   Provider
    fallbacks []Provider
    config    ResilientConfig
    state     circuitState
    failures  int
    mu        sync.RWMutex
    lastFail  time.Time
}

type ResilientConfig struct {
    MaxRetries       int           // 默认 3
    RetryBackoff     time.Duration // 初始退避 500ms
    MaxBackoff       time.Duration // 最大退避 10s
    CircuitThreshold int           // 熔断阈值: 连续失败 5 次
    CircuitRecoverAfter time.Duration // 半开等待 30s
}

type circuitState int
const (
    circuitClosed circuitState = iota  // 正常
    circuitOpen                        // 熔断中，快速失败
    circuitHalfOpen                    // 半开，允许试探
)

func NewResilientProvider(primary Provider, cfg ResilientConfig) *ResilientProvider
func (r *ResilientProvider) AddFallback(provider Provider)
func (r *ResilientProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
// Stream / CallTools / Embeddings / Info 类似...
```

**Complete 执行流程**:
```
1. 检查熔断器状态
   - Open → 直接返回 ErrCircuitOpen
   - HalfOpen → 允许一次请求试探
   
2. 调用 primary.Complete()
   - 成功 → 重置失败计数，返回结果
   - 失败 → 进入重试循环
   
3. 重试循环 (最多 MaxRetries 次):
   - 指数退避等待: backoff * 2^attempt
   - 再次调用 primary
   
4. Primary 全部失败 → 尝试 fallbacks（按顺序）
   
5. 所有 provider 都失败 → 记录失败次数
   - 超过 CircuitThreshold → 打开熔断器
   - 返回最后一个 error
```

### 测试用例清单

```
TestResilient_PrimarySuccess               // 主 provider 正常工作
TestResilient_PrimaryRetryThenSuccess      // 主 provider 第2次重试成功
TestResilient_PrimaryFails_FallbackSuccess // 主失败→回退成功
TestResilient_AllFail_ReturnError          // 全部失败返回错误
TestResilient_CircuitBreakerOpens          // 连续失败达到阈值→熔断打开
TestResilient_CircuitHalfOpenRecovers      // 半开后成功→恢复
TestResilient_CircuitRejectsWhenOpen       // 熔断打开时快速拒绝
TestResilient_ExponentialBackoff           // 退避时间指数增长
TestResilient_ContextCancel                // context cancel 立即返回
TestResilient_NoFallbacks                  // 无 fallback 时主失败直接报错
```

---

## Task 16: Metrics 可观测性 (P2)

**全新模块**
**目标**: 统一指标收集，方便监控 Agent 运行状况

### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 创建 | `agentprimordia/internal/metrics/metrics.go` | 核心实现 (~150行) |
| 创建 | `agentprimordia/internal/metrics/metrics_test.go` | 测试 (~180行) |

### 实现要点

```go
// internal/metrics/metrics.go
package metrics

import (
    "fmt"
    "strings"
    "sync"
    "time"
)

type AgentMetrics struct {
    mu sync.RWMutex

    // Counters
    LLMTotalCalls   int64
    LLMTotalErrors  int64
    ToolTotalCalls  int64
    ToolTotalErrors int64
    TotalTurns      int64
    TotalEpisodes   int64

    // Histograms (简单桶实现)
    LLMLatencyMs   *Histogram
    ToolLatencyMs  *Histogram
    TurnDurationMs *Histogram

    // Gauges
    ActiveAgents    int64
    PoolQueueLength int64
    MemorySizeBytes int64
}

type Histogram struct {
    buckets []float64
    counts  []int64
    sum     int64
    count   int64
    min     int64
    max     int64
}

func NewMetrics() *AgentMetrics
func (m *AgentMetrics) RecordLLMCall(duration time.Duration, err error)
func (m *AgentMetrics) RecordToolCall(duration time.Duration, err error)
func (m *AgentMetrics) RecordTurn(duration time.Duration)
func (m *AgentMetrics) IncActiveAgents()
func (m *AgentMetrics) DecActiveAgents()
func (m *AgentMetrics) SetPoolQueue(n int64)
func (m *AgentMetrics) SetMemorySize(bytes int64)
func (m *AgentMetrics) Snapshot() MetricsSnapshot
func (m *AgentMetrics) Reset()
func (m *AgentMetrics) String() string // Prometheus text format 输出

type MetricsSnapshot struct { ... } // 快照副本
```

**Histogram 桶定义**:
- Latency: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s, +Inf
- Turn Duration: 100ms, 500ms, 1s, 2s, 5s, 10s, 30s, 60s, 120s, 300s, +Inf

### 测试用例清单

```
TestMetrics_RecordLLMCall_Success           // 成功调用计数+1
TestMetrics_RecordLLMCall_Error            // 失败调用 error 计数+1
TestMetrics_RecordToolCall                // 工具调用记录
TestMetrics_ActiveAgentsGauge             // Gauge 增减
TestMetrics_Histogram_BasicDistribution   // 直方图分布正确
TestMetrics_Histogram_MinMaxSum           // min/max/sum 正确
TestMetrics_Snapshot                      // 快照独立于后续变化
TestMetrics_Reset                         // Reset 后归零
TestMetrics_StringFormat                  // Prometheus 格式输出
TestMetrics_ConcurrentRecording           // 并发记录安全
```

---

## Task 17: Checkpoint 接口预留 (P2)

**Phase 2 实现**，此处只定义接口和数据结构

### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 创建 | `agentprimordia/internal/persist/checkpoint.go` | 接口定义 (~60行) |

### 实现要点

```go
// internal/persist/checkpoint.go
package persist

import (
    "context"
    "time"

    "github.com/agentprimordia/ap/internal/agent"
)

type AgentState struct {
    AgentID   string           `json:"agent_id"`
    SessionID string           `json:"session_id"`
    Status    string           `json:"status"`
    Messages  []agent.Message  `json:"messages"`
    TurnCount int              `json:"turn_count"`
    Metrics   agent.Metrics    `json:"metrics"`
    SavedAt   time.Time        `json:"saved_at"`
}

type CheckpointStore interface {
    Save(ctx context.Context, state *AgentState) error
    Load(ctx context.Context, agentID string) (*AgentState, error)
    List(ctx context.Context, sessionID string) ([]*AgentState, error)
    Delete(ctx context.Context, agentID string) error
}
```

### 测试用例清单

```
TestCheckpoint_AgentState_JSONRoundtrip   // JSON 序列化/反序列化
TestCheckpoint_AgentState_DefaultValues    // 默认值正确
```

---

## 执行顺序建议

```
Day 1 (核心 P0):
  Task 10: OpenAI Provider    ← 最关键，无此 AP 无法用于生产
  Task 11: FileLock Manager   ← 并发安全保障
  Task 12: Scope Policy       ← IDE 场景权限控制

Day 2 (重要 P1):
  Task 13: Enhanced Memory    ← 功能最丰富的新模块
  Task 14: Context Window     ← ReActLoop 必需
  Task 15: Resilient Provider ← 生产稳定性

Day 3 (完善 P2):
  Task 16: Metrics            ← 可观测性
  Task 17: Checkpoint 接口    ← Phase 2 预留

每完成一个 Task:
  1. go test -v ./对应模块/...   (确认新测试全过)
  2. go test ./...               (全量回归)
```

## 验收标准

- [x] 7 个 packages 全部 `go build` 通过
- [x] 全量测试 `go test ./...` 零失败（实际 ~195 tests）
- [x] OpenAIProvider 可连接真实 DeepSeek/OpenAI API（手动验证）
- [x] FileLockManager 在 10 并发写入同文件无数据竞争
- [x] ScopePolicy 正确拦截越权访问
- [x] Memory 支持 topics/importance/cleanup/timeline
- [x] ResilientProvider 故障时自动回退
- [x] 零新增外部依赖（除已有的 modernc.org/sqlite）

---

*Plan Version: 2.0-completed | Last Updated: 2026-05-29*
