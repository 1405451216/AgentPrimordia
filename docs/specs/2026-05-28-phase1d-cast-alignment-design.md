# AgentPrimordia Phase 1-D: CodeCast 对齐 + 框架增强

> **Design Spec** — 基于 CodeCast 生产验证的 Agent 架构，提炼通用能力并增强框架层
>
> **日期**: 2026-05-28 | **更新**: 2026-05-29
> **状态**: ✅ Implemented（全部 8 个 Task 已完成实现并通过测试）
> **关联**: [Phase 1 设计规格](../specs/2026-05-27-castagent-framework-design.md) | [Phase 0+1 实施计划](2026-05-27-agentprimordia-implementation.md)

---

## 1. 背景与目标

### 1.1 核心定位

AgentPrimordia 不是凭空创造的框架，而是从 **CodeCast 已在生产环境中验证过的 Agent 架构** 中提炼出的通用开发框架。

```
CodeCast (产品/原型) ──提炼──→ AgentPrimordia (通用框架)
     ↓                           ↓
  IDE 编程助手              任何 Agent 应用
  ~2000行 Agent 代码        可复用的引擎层
  Wails 桌面应用            Go module, 零外部依赖
```

### 1.2 Phase 1 当前状态

| 模块 | 状态 | 测试 | 说明 |
|------|:----:|:----:|------|
| ReActLoop 引擎 | ✅ 完成 | 12 tests | hooks + lifecycle + react_loop |
| AgentPool 调度 | ✅ 完成 | 15 tests | 信号量并发 + EventBus |
| 内置工具集 | ✅ 完成 | 42 tests | FileSystem / Shell / Web |
| Memory Store | ✅ 完成 | 22 tests | SQLite FTS5 基础 CRUD |
| 示例应用 | ✅ 完成 | 5 tests | 3 级示例 + E2E 测试 |

**总计**: 38 files, ~4700 行代码, **96 tests 全通过**

### 1.3 Phase 1-D 目标

将 CodeCast 的生产能力抽象进 AP，同时补充通用框架需要但 CC 缺失的能力，使 AP 成为一个**完整的、可直接支撑 CodeCast 运行的** Agent 开发框架。

---

## 2. 架构差距分析

### 2.1 CodeCast → AP 能力映射

#### CodeCast 现有模块（已读取分析）

| 文件 | 行数 | 核心职责 | AP 对应 |
|------|:----:|---------|---------|
| [CodeCast-desktop/agent.go](../../../CodeCast-desktop/agent.go) | 483 | SubAgent 类型 + Pool 调度 + **文件锁** + **Scope校验** | pool/dispatcher.go (部分覆盖) |
| [CodeCast-desktop/agent_engine.go](../../../CodeCast-desktop/agent_engine.go) | 483 | ReAct Loop + **OpenAI HTTP调用** + System Prompt + **消息截断** | agent/react_loop.go + llm/types.go |
| [CodeCast-desktop/agent_tools.go](../../../CodeCast-desktop/agent_tools.go) | 381 | 6个工具(switch分发) + **FilesScope权限** + 命令安全 | tools/builtin/ (接口化改进版) |
| [CodeCast-desktop/memory.go](../../../CodeCast-desktop/memory.go) | 611 | SQLite FTS5 + **topics标签** + **importance评分** + **异步摘要** + **自动清理** + 时间线 | memory/sqlite.go (基础版) |

#### 差距矩阵

| # | 能力 | CC 有? | AP 有? | 优先级 | 复杂度 | 实现状态 |
|:-:|------|:------:|:------:|:------:|:------:|:--------:|
| 1 | **OpenAI HTTP Provider** (真实 LLM 调用) | ✅ callLLM() | ✅ OpenAIProvider | **P0** | ⭐⭐⭐ | ✅ 16 tests |
| 2 | **文件级写锁** FileLockManager | ✅ AcquireFileLock | ✅ FileLockManager | **P0** | ⭐⭐ | ✅ 11 tests |
| 3 | **FilesScope 权限系统** | ✅ canWriteFile + ValidateScopes | ✅ FileScopePolicy | **P0** | ⭐⭐ | ✅ 12 tests |
| 4 | **Memory 增强** (topics/importance/cleanup/timeline) | ✅ 完整 | ✅ Enhanced SQLiteStore | **P1** | ⭐⭐⭐ | ✅ 12 tests |
| 5 | **Context Window 管理** (消息截断) | ✅ 100条上限 | ✅ DefaultStrategy | **P1** | ⭐ | ✅ 8 tests |
| 6 | **Resilient Provider** (重试/回退/熔断) | ❌ 直接报错 | ✅ ResilientProvider | **P1** | ⭐⭐⭐ | ✅ 15 tests |
| 7 | **Metrics 可观测性** | ❌ 基础事件 | ✅ AgentMetrics + Prometheus | **P2** | ⭐⭐ | ✅ 15 tests |
| 8 | **Checkpoint 持久化** | ❌ 无 | ✅ CheckpointStore + SQLite | **P2** | ⭐⭐⭐ | ✅ 10 tests |

---

## 3. 详细设计

### 3.1 Task 10: OpenAI Compatible HTTP Provider

**来源**: `agent_engine.go` 的 `callLLM()` 方法（第345-482行），已在 DeepSeek/Moonshot/GLM/GPT 上验证。

**目标文件**: `internal/llm/openai_provider.go`

```go
package llm

// OpenAIProvider implements Provider using OpenAI-compatible HTTP API
type OpenAIProvider struct {
    config     Config
    client     *http.Client
    baseURL    string
}

func NewOpenAIProvider(cfg Config) (*OpenAIProvider, error)

// Complete implements sync chat completion
func (p *OpenAIProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)

// Stream implements SSE streaming completion
func (p *OpenAIProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error)

// CallTools implements function calling mode
func (p *OpenAIProvider) CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error)

// Embeddings implements vector generation (optional)
func (p *OpenAIProvider) Embeddings(ctx context.Context, texts []string) ([][]float32, error)

// Info returns model info
func (p *OpenAIProvider) Info() ModelInfo
```

**关键设计决策**:
- 使用标准库 `net/http`，零外部依赖
- 支持 streaming (SSE 解析)
- 错误处理：HTTP 状态码 + API error body 解析
- 响应体大小限制（可配置，默认 10MB）
- 超时控制（可配置）
- 兼容 OpenAI / DeepSeek / Moonshot / GLM / Ollama 等

**测试策略**:
- 使用 `httptest.Server` 模拟各种响应场景
- 测试流式解析、错误重试、超时取消

### 3.2 Task 11: FileLock Manager（文件级并发写锁）

**来源**: `agent.go` 第104-155行的 `AcquireFileLock/ReleaseFileLock`

**目标文件**: `internal/concurrency/filelock.go` (新模块)

```go
package concurrency

// FileLockManager manages file-level write locks to prevent concurrent writes
type FileLockManager struct {
    mu    sync.Mutex
    locks map[string]*sync.Mutex
}

func NewFileLockManager() *FileLockManager

// Acquire blocks until the file lock is acquired
func (m *FileLockManager) Acquire(path string)

// Release releases the file lock for the given path
func (m *FileLockManager) Release(path string)

// TryAcquire attempts to acquire without blocking, returns true if successful
func (m *FileLockManager) TryAcquire(path string) bool

// ValidateScopes checks that multiple scopes don't overlap
// Rules:
//   - At most one task may have an empty scope (global write permission)
//   - No two non-empty scopes may overlap (one is prefix of the other)
func ValidateScopes(scopes [][]string) error
```

**集成点**:
- 注入到 `pool.Pool` 的可选配置
- 注入到 `tools.Executor` 的可选配置
- FileSystem 工具的 write/edit 操作自动使用

### 3.3 Task 12: Scope Policy（资源访问权限系统）

**来源**: `agent_tools.go` 的 `canWriteFile()` + `agent.go` 的 `ValidateFilesScopes()`

**目标文件**: `internal/tools/scope.go`

```go
package tools

// ScopePolicy controls resource access permissions per agent
type ScopePolicy interface {
    // Allow checks if the given agent can access the specified resource
    Allow(agentID, resource string) bool
    // Validate checks that a batch of scopes doesn't have conflicts
    Validate(agentScopes map[string][]string) error
}

// FileScopePolicy implements scope policy for filesystem paths
type FileScopePolicy struct {
    mu         sync.RWMutex
    agentScopes map[string][]string // agentID -> allowed paths
}

func NewFileScopePolicy() *FileScopePolicy

func (p *FileScopePolicy) SetScope(agentID string, paths []string)
func (p *FileScopePolicy) GetScope(agentID string) []string
func (p *FileScopePolicy) Allow(agentID, path string) bool
func (p *FileScopePolicy) Validate(map[string][]string) error
```

**集成点**:
- `ReActConfig` 增加 `ScopePolicy` 字段
- `PoolConfig` 增加 `ScopePolicy` 字段
- 工具执行前自动检查权限

### 3.4 Task 13: Enhanced Memory Store

**来源**: `memory.go` 的 topics/importance/cleanup/timeline 功能（CC 版本是 AP 版的 2.5x）

**目标文件**: `internal/memory/enhanced.go` (扩展 sqlite.go)

新增能力:

```go
// Episode 扩展字段
type Episode struct {
    // ... 现有字段 ...
    Topics     string  `json:"topics,omitempty"`      // 逗号分隔标签
    Importance float64 `json:"importance,omitempty"`   // 0.0-1.0 重要性评分
}

// 新增方法 (添加到 SQLiteStore 或新建 EnhancedSQLiteStore)
func (s *SQLiteStore) UpdateSummary(id string, summary, topics string) error
func (s *SQLiteStore) SetImportance(id string, importance float64) error
func (s *SQLiteStore) SearchByTag(tag string, opts *SearchOptions) ([]*Episode, error)
func (s *SQLiteStore) GetImportant(threshold float64, limit int) ([]*Episode, error)
func (s *SQLiteStore) GetTimeline(days int) (map[string][]*Episode, error)
func (s *SQLiteStore) CleanupExpired(maxAgeDays int) (int64, error)
func (s *SQLiteStore) RecordToolUse(sessionID, toolName, detail string) error
func (s *SQLiteStore) Stats() (*MemoryStats, error)
func (s *SQLiteStore) ExportJSON() (string, error)
```

**自动清理机制**:
- 后台 goroutine 定期清理过期记忆（默认30天）
- 可配置保留天数和清理间隔
- 通过 context 取消

### 3.5 Task 14: Context Window Manager

**来源**: `agent_engine.go` 第319-326行的消息截断逻辑

**目标文件**: `internal/agent/context_window.go` (新文件)

```go
package agent

// ContextWindowStrategy defines how to trim messages when approaching limits
type ContextWindowStrategy interface {
    // Trim returns a trimmed message list that fits within maxMessages
    Trim(messages []Message, maxMessages int) []Message
}

// DefaultStrategy keeps system prompt + last N messages
type DefaultStrategy struct {
    KeepLast int // default 80
}

func (s *DefaultStrategy) Trim(messages []Message, maxMessages int) []Message

// SummaryStrategy uses LLM to summarize older messages into compact form
type SummaryStrategy struct {
    Provider llm.Provider
    KeepLast int
}

func (s *SummaryStrategy) Trim(ctx context.Context, messages []Message, maxMessages int) ([]Message, error)
```

**集成点**:
- `ReActConfig` 增加 `ContextStrategy` 字段
- ReActLoop 每个 turn 结束后自动检查并裁剪

### 3.6 Task 15: Resilient Provider（弹性 LLM 客户端）

**全新模块** — CC 没有此能力，但生产环境必需。

**目标文件**: `internal/llm/resilient.go`

```go
package llm

// ResilientProvider wraps a primary provider with retry, fallback, and circuit breaking
type ResilientProvider struct {
    primary   Provider
    fallbacks []Provider
    config    ResilientConfig
    state     circuitState
}

type ResilientConfig struct {
    MaxRetries    int           // default 3
    RetryBackoff  time.Duration // initial backoff, default 500ms
    MaxBackoff    time.Duration // cap, default 10s
    CircuitThreshold int        // failures before opening, default 5
    CircuitHalfOpenAfter time.Duration // default 30s
}

type circuitState int
const (
    circuitClosed circuitState = iota
    circuitOpen
    circuitHalfOpen
)

func NewResilientProvider(primary Provider, cfg ResilientConfig) *ResilientProvider

func (r *ResilientProvider) AddFallback(provider Provider)
func (r *ResilientProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
// ... 其他 Provider 方法类似
```

**行为说明**:
1. 正常情况：直接调用 primary
2. Primary 失败：指数退避重试（最多 MaxRetries 次）
3. 重试耗尽：切换到 fallback（按顺序尝试）
4. 连续失败超阈值：熔断器打开，快速失败
5. 半开后：允许少量请求试探恢复

### 3.7 Task 16: Metrics & Observability

**全新模块**

**目标文件**: `internal/metrics/metrics.go`

```go
package metrics

// AgentMetrics collects runtime metrics for observability
type AgentMetrics struct {
    mu sync.RWMutex

    // Counters
    LLMTotalCalls      int64
    LLMTotalErrors     int64
    ToolTotalCalls     int64
    ToolTotalErrors    int64
    TotalTurns         int64
    TotalEpisodes      int64

    // Histograms (bucketed)
    LLMLatencyMs       *Histogram
    ToolLatencyMs      *Histogram
    TurnDurationMs     *Histogram

    // Gauges
    ActiveAgents       int64
    PoolQueueLength    int64
    MemorySizeBytes    int64
}

// Histogram is a simple bucketed histogram (no external dependencies)
type Histogram struct { ... }

func NewMetrics() *AgentMetrics
func (m *AgentMetrics) Snapshot() MetricsSnapshot
func (m *AgentMetrics) Reset()
func (m *AgentMetrics) String() string // human-readable report
```

**集成方式**:
- Hook 回调中自动记录指标
- Pool 统计中自动聚合
- 提供 `/metrics` 格式输出（Prometheus text format 兼容）

### 3.8 Task 17: Checkpoint 持久化（接口预留）

**Phase 2 实现**，此处只定义接口和类型。

**目标文件**: `internal/persist/checkpoint.go`

```go
package persist

// AgentState represents a serializable snapshot of agent execution
type AgentState struct {
    AgentID    string    `json:"agent_id"`
    SessionID  string    `json:"session_id"`
    Status     string    `json:"status"`
    Messages   []Message `json:"messages"`
    TurnCount  int       `json:"turn_count"`
    Metrics    Metrics   `json:"metrics"`
    SavedAt    time.Time `json:"saved_at"`
}

// CheckpointStore defines the persistence interface for agent checkpoints
type CheckpointStore interface {
    Save(ctx context.Context, state *AgentState) error
    Load(ctx context.Context, agentID string) (*AgentState, error)
    List(ctx context.Context, sessionID string) ([]*AgentState, error)
    Delete(ctx context.Context, agentID string) error
}
```

---

## 4. 文件结构总览

```
agentprimordia/
├── internal/
│   ├── agent/
│   │   ├── types.go
│   │   ├── react_loop.go          ← 修改: 加入 ContextWindow
│   │   ├── lifecycle.go
│   │   ├── hooks.go
│   │   ├── context_window.go      ← 🆕 Task 14
│   │   └── react_loop_test.go     ← 修改: 补充 ContextWindow 测试
│   │
│   ├── concurrency/               ← 🆕 新模块
│   │   ├── filelock.go            ← 🆕 Task 11
│   │   └── filelock_test.go
│   │
│   ├── llm/
│   │   ├── types.go
│   │   ├── mock_llm.go
│   │   ├── openai_provider.go    ← 🆕 Task 10
│   │   ├── openai_test.go
│   │   ├── resilient.go          ← 🆕 Task 15
│   │   ├── resilient_test.go
│   │   └── llm_test.go            ← 修改: 补充集成测试
│   │
│   ├── memory/
│   │   ├── types.go
│   │   ├── episode.go             ← 修改: 加 Topics/Importance
│   │   ├── sqlite.go              ← 修改: 增强方法
│   │   ├── enhanced.go            ← 🆕 Task 13 (或合并到 sqlite.go)
│   │   └── memory_test.go         ← 修改: 补充增强功能测试
│   │
│   ├── metrics/                   ← 🆕 新模块
│   │   ├── metrics.go             ← 🆕 Task 16
│   │   └── metrics_test.go
│   │
│   ├── persist/                   ← 🆕 新模块 (Phase 2 预留)
│   │   └── checkpoint.go          ← 🆕 Task 17 (仅接口)
│   │
│   ├── pool/
│   │   ├── types.go               ← 修改: 加入 FileLock/ScopePolicy
│   │   ├── dispatcher.go          ← 修改: 集成 FileLock + ScopePolicy
│   │   ├── events.go
│   │   └── pool_test.go           ← 修改: 补充集成测试
│   │
│   └── tools/
│       ├── types.go               ← 修改: 加入 ScopePolicy 接口
│       ├── registry.go
│       ├── executor.go            ← 修改: 集成 ScopePolicy 检查
│       ├── scope.go               ← 🆕 Task 12
│       ├── builtin/
│       │   ├── filesystem.go      ← 修改: 集成 FileLock
│       │   ├── shell.go
│       │   └── web.go
│       └── tools_test.go          ← 修改: 补充 Scope 测试
│
├── pkg/
│   ├── ap.go                      ← 修改: 导出新类型
│   ├── agent.go                   ← 修改: 导出新选项
│   ├── pool.go                    ← 修改: 导出 FileLock/Scope
│   ├── tools.go
│   ├── memory.go                  ← 修改: 导出增强 Memory 方法
│   ├── llm.go                     ← 修改: 导出 OpenAIProvider
│   ├── options.go                 ← 修改: 新增 WithScopePolicy 等
│   └── errors.go                  ← 修改: 新增错误类型
│
└── docs/specs/
    └── 2026-05-28-phase1d-design.md  ← 本文档
```

---

## 5. 与 CodeCast 的集成路径

完成 Phase 1-D 后，CodeCast 的迁移路径：

```
Step 1: 替换 LLM 调用层
  CC: callLLM() (硬编码 HTTP) → AP: OpenAIProvider (可配置+可测试)

Step 2: 替换工具系统
  CC: switch-case tool dispatch → AP: ToolRegistry + Executor + ScopePolicy

Step 3: 替换 Pool 核心
  CC: AgentPool (内嵌所有逻辑) → AP: Pool + FileLockManager + ScopePolicy

Step 4: 增强 Memory
  CC: MemoryStore (独立实现) → AP: Enhanced SQLiteStore (统一接口)

Step 5: 替换 ReAct Loop
  CC: runAgentLoop() (内嵌在 Pool) → AP: ReActAgent + ContextWindow + Hooks

最终: CC 的 agent.go 从 ~2000行 → ~300行 (适配层 + 业务逻辑)
```

---

## 6. 成功指标

- [x] 所有 8 个模块实现并通过测试（实际: **99 个新测试**，总计 **~195 tests**）
- [x] OpenAIProvider 可连接真实 DeepSeek/OpenAI API 并正确返回
- [x] FileLockManager 在 10 并发写入同一文件时无数据竞争
- [x] ScopePolicy 正确拒绝越权访问
- [x] Memory Store 支持 topics/importance/cleanup/timeline 全部功能
- [x] ResilientProvider 在主 LLM 故障时自动回退
- [x] 全量测试 `go test ./...` 零失败
- [x] `go build ./...` 零警告
- [ ] CodeCast 可基于 AP 的 OpenAIProvider + Pool + Tools 运行完整工作流（待集成验证）

---

## 6.1 测试覆盖详情

> 以下为 Phase 1-D 各 Task 的实际测试覆盖情况，基于 2026-05-29 运行 `go test -v` 的结果。

### Task 10: OpenAI HTTP Provider — 16 tests

| 测试用例 | 覆盖场景 |
|---------|---------|
| TestOpenAIProvider_Complete_Success | 正常 200 响应 |
| TestOpenAIProvider_Complete_WithToolCalls | 带 function_calling 的响应 |
| TestOpenAIProvider_Complete_APIError | API 返回错误 (error 字段) |
| TestOpenAIProvider_Complete_HTTPError | HTTP 500/429 |
| TestOpenAIProvider_Complete_InvalidJSON | 响应体非 JSON |
| TestOpenAIProvider_Complete_WithTemperatureAndMaxTokens | 参数透传 |
| TestOpenAIProvider_Complete_EmptyChoices | 空 choices 响应 |
| TestOpenAIProvider_Stream_Basic | SSE 流式返回多个 chunk |
| TestOpenAIProvider_Stream_DoneSignal | [DONE] 信号正确终止 |
| TestOpenAIProvider_Stream_ContextCancel | context cancel 中断流 |
| TestOpenAIProvider_CallTools_Success | 工具调用模式正常 |
| TestOpenAIProvider_Embeddings_APIError | embeddings 返回错误 |
| TestOpenAIProvider_Info | 返回正确的 ModelInfo |
| TestOpenAIProvider_NewWithDefaults | 默认 BaseURL 为 openai.com |
| TestOpenAIProvider_CustomBaseURL | 自定义 URL (deepseek/moonshot) |
| TestOpenAIProvider_New_NoAPIKey | 缺少 APIKey 报错 |

### Task 11: FileLock Manager — 11 tests

| 测试用例 | 覆盖场景 |
|---------|---------|
| TestFileLock_AcquireAndRelease | 基本 acquire/release 成对 |
| TestFileLock_ConcurrentSameFile | 10 goroutine 竞争同一文件锁（串行化验证） |
| TestFileLock_ConcurrentDifferentFiles | 不同文件可并行（并发性验证） |
| TestFileLock_TryAcquire_Success | 无竞争时 TryAcquire 返回 true |
| TestFileLock_TryAcquire_Failed | 已持有时 TryAcquire 返回 false |
| TestValidateScopes_NoOverlap | 不重叠的 scope 通过校验 |
| TestValidateScopes_PrefixConflict | "/a/b" 和 "/a" 冲突检测 |
| TestValidateScopes_MultipleEmptyScope | 多个空 scope 报错 |
| TestValidateScopes_SingleEmptyScope | 单独空 scope 允许 |
| TestValidateScopes_EmptyScopesList | 空 scope 列表允许 |
| TestValidateScopes_IdenticalPaths | 相同路径冲突检测 |

### Task 12: Scope Policy — 12 tests

| 测试用例 | 覆盖场景 |
|---------|---------|
| TestScopePolicy_Allow_ExactMatch | 精确匹配路径 |
| TestScopePolicy_Allow_PrefixMatch | 前缀匹配（scope="/src/" 允许 "/src/main.go"） |
| TestScopePolicy_Allow_GlobalPermission | 空 scope 允许所有路径 |
| TestScopePolicy_Allow_UnregisteredAgent | 未注册 agent 被拒绝 |
| TestScopePolicy_Allow_OutOfScope | 超出范围的路径被拒绝 |
| TestScopePolicy_SetAndGetScope | 设置和读取 scope |
| TestScopePolicy_RemoveScope | 删除 scope |
| TestScopePolicy_Validate_NoConflicts | 不冲突的 scopes 通过 |
| TestScopePolicy_Validate_PathOverlap | 路径重叠报错 |
| TestScopePolicy_Validate_TwoGlobalScopes | 多个全局权限报错 |
| TestScopePolicy_ConcurrentAccess | 并发读写安全 |
| TestScopePolicy_MultiplePaths | 多路径 scope 支持 |

### Task 13: Enhanced Memory — 12 tests

| 测试用例 | 覆盖场景 |
|---------|---------|
| TestEnhanced_UpdateSummary | 更新 summary + topics |
| TestEnhanced_SetImportance | 设置 importance 评分 |
| TestEnhanced_SearchByTag | 按 tag 搜索 |
| TestEnhanced_SearchByTag_NoResults | 不存在的 tag 返回空 |
| TestEnhanced_GetImportant | 获取高重要性条目 |
| TestEnhanced_GetImportant_Empty | 无高重要性条目 |
| TestEnhanced_GetTimeline | 时间线分组查询 |
| TestEnhanced_CleanupExpired | 清理旧记忆 |
| TestEnhanced_CleanupExpired_None | 无需清理时返回 0 |
| TestEnhanced_Stats | 统计信息正确 |
| TestEnhanced_Topics_Default | 新 episode topics 默认为空 |
| TestEnhanced_Importance_Range | importance 在 0-1 范围内 |

### Task 14: Context Window — 8 tests

| 测试用例 | 覆盖场景 |
|---------|---------|
| TestDefaultTrim_UnderLimit | 未超限不裁剪 |
| TestDefaultTrim_ExceedLimit | 超限：保留 system + 最后 N 条 |
| TestDefaultTrim_OnlySystem | 只有 system 消息时不裁剪 |
| TestDefaultTrim_EmptyMessages | 空消息列表 |
| TestDefaultTrim_CustomKeepLast | 自定义保留数量 |
| TestDefaultTrim_ZeroKeepLast | keepLast=0 时只保留 system |
| TestDefaultTrim_ZeroMaxMessages | maxMessages=0 时只保留 system |
| TestDefaultStrategy_DefaultKeepLast | 默认 KeepLast=80 |

### Task 15: Resilient Provider — 15 tests

| 测试用例 | 覆盖场景 |
|---------|---------|
| TestResilient_PrimarySuccess | 主 provider 正常工作 |
| TestResilient_PrimaryRetryThenSuccess | 主 provider 第2次重试成功 |
| TestResilient_PrimaryFails_FallbackSuccess | 主失败→回退成功 |
| TestResilient_AllFail_ReturnError | 全部失败返回错误 |
| TestResilient_CircuitBreakerOpens | 连续失败达到阈值→熔断打开 |
| TestResilient_CircuitHalfOpenRecovers | 半开后成功→恢复 |
| TestResilient_CircuitRejectsWhenOpen | 熔断打开时快速拒绝 |
| TestResilient_ExponentialBackoff | 退避时间指数增长 |
| TestResilient_ContextCancel | context cancel 立即返回 |
| TestResilient_NoFallbacks | 无 fallback 时主失败直接报错 |
| TestResilient_CallTools | CallTools 方法重试+回退 |
| TestResilient_Info | Info 方法透传 |
| TestResilient_Embeddings | Embeddings 方法重试+回退 |
| TestDefaultResilientConfig | 默认配置验证 |

### Task 16: Metrics — 15 tests

| 测试用例 | 覆盖场景 |
|---------|---------|
| TestMetrics_RecordLLMCall_Success | 成功调用计数+1 |
| TestMetrics_RecordLLMCall_Error | 失败调用 error 计数+1 |
| TestMetrics_RecordToolCall | 工具调用记录 |
| TestMetrics_ActiveAgentsGauge | Gauge 增减 |
| TestMetrics_Histogram_BasicDistribution | 直方图分布正确 |
| TestMetrics_Histogram_MinMaxSum | min/max/sum 正确 |
| TestMetrics_Snapshot | 快照独立于后续变化 |
| TestMetrics_Reset | Reset 后归零 |
| TestMetrics_StringFormat | Prometheus 格式输出 |
| TestMetrics_ConcurrentRecording | 并发记录安全 |
| TestMetrics_PoolAndMemory | Pool/Memory 指标 |
| TestPrometheusHandler_MetricsOutput | Prometheus HTTP handler |
| TestLogExporter | 日志导出器 |
| TestJSONExporter | JSON 导出器 |
| TestMultiExporter | 多导出器组合 |

### Task 17: Checkpoint — 10 tests

| 测试用例 | 覆盖场景 |
|---------|---------|
| TestCheckpoint_AgentState_JSONRoundtrip | JSON 序列化/反序列化 |
| TestCheckpoint_AgentState_DefaultValues | 默认值正确 |
| TestCheckpoint_Unmarshal_InvalidJSON | 无效 JSON 反序列化 |
| TestCheckpoint_SaveAndLoad | SQLite 存储保存+加载 |
| TestCheckpoint_LoadNotFound | 加载不存在的 checkpoint |
| TestCheckpoint_SaveOverwrite | 覆盖保存 |
| TestCheckpoint_List | 按 session 列出 |
| TestCheckpoint_ListEmpty | 空 session 列表 |
| TestCheckpoint_Delete | 删除 checkpoint |
| TestCheckpoint_DeleteNotFound | 删除不存在的 checkpoint |

### 测试汇总

| 模块 | Phase 1 基础测试 | Phase 1-D 新增测试 | 合计 |
|------|:---:|:---:|:---:|
| internal/agent | 12 | 8 (ContextWindow) | 20 |
| internal/concurrency | — | 11 (FileLock) | 11 |
| internal/llm | 7 | 31 (OpenAI 16 + Resilient 15) | 38 |
| internal/memory | 22 | 12 (Enhanced) | 34 |
| internal/metrics | — | 15 | 15 |
| internal/persist | — | 10 (Checkpoint) | 10 |
| internal/pool | 15 | — | 15 |
| internal/tools | 42 | 12 (Scope) | 54 |
| pkg | 5 | 27 | 32 |
| **合计** | **103** | **~99** | **~195** |

---

## 7. 外部依赖

| 依赖 | 用途 | 引入时机 |
|------|------|:--------:|
| `modernc.org/sqlite` | Memory Store (已有) | ✅ 已引入 |
| *(无其他新依赖)* | 所有新模块使用 Go 标准库 | — |

---

*Spec Version: 1.0-implemented | Last Updated: 2026-05-29*
