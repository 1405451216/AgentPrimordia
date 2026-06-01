# AgentPrimordia Phase 2: 均衡推进 — 设计规格

> **日期**: 2026-05-29
> **状态**: Ready to Execute
> **前置条件**: Phase 1-D (Tasks 10-17) 已完成，~195 tests 全通过
> **关联**: [Phase 1-D 设计规格](2026-05-28-phase1d-cast-alignment-design.md) | [v0.1.0 发布说明](../RELEASE-NOTES-v0.1.0.md)

---

## 1. 背景与目标

### 1.1 Phase 1-D 回顾

Phase 1-D 完成了 8 个 Task，补齐了 CodeCast 的核心能力（OpenAI Provider、FileLock、Scope、Enhanced Memory、Context Window、Resilient Provider、Metrics、Checkpoint），框架已具备完整的 Agent 开发能力。

但深入分析发现两类遗留问题：

1. **集成断层**：FileLock/Scope 已实现但未接入工具执行流，System Prompt 无模板引擎无法自动注入权限规则
2. **架构债务**：双消息总线并存、缺少 Agent 接口抽象、Pool 配置断层、Run/StreamRun 代码重复

### 1.2 Phase 2 目标

按 P0→P1→P2 均衡推进，分 3 个子阶段交付：

- **2-A（P0）**：CodeCast 对齐 + 架构修复 — 让框架能真正支撑 CodeCast 运行
- **2-B（P1）**：架构统一 + 编排增强 — 统一架构模式，增强编排能力
- **2-C（P2）**：新能力扩展 — 分布式通信、性能基准、更多 Provider

---

## 2. 差距分析摘要

### 2.1 CodeCast 未覆盖差距

| # | 差距 | 来源 | 优先级 | 影响 |
|:-:|------|:----:|:------:|------|
| G1 | Scope/FileLock 未集成到工具执行流 | agent_tools.go L55-67, L128-141 | **P0** | 多 Agent 并发写文件无保护 |
| G2 | System Prompt 无模板引擎 | agent_engine.go L25-52 | **P0** | 无法自动注入 FilesScope 规则 |
| G3 | edit_file 替换所有匹配而非唯一匹配 | agent_tools.go L190-193 | **P0** | 可能导致意外大范围修改 |
| G4 | 文件读写无大小限制 | agent_tools.go L103, L135 | **P0** | 可能 OOM |
| G5 | 命令输出无截断 | agent_tools.go L257-259 | **P0** | 极长输出消耗 token |
| G6 | Memory 无异步摘要提取 | memory.go L286-373 | **P0** | 搜索质量低下 |
| G7 | Memory 无自动清理调度 | memory.go L265-284 | **P0** | 长期运行记忆无限增长 |
| G8 | Session 分组与按会话取消 | agent.go L250-283 | **P1** | 多会话场景无法管理 |
| G9 | 目录级搜索 | agent_tools.go L296-311 | **P1** | 仅单文件搜索 |
| G10 | 默认工具集 | agent_engine.go L57-186 | **P1** | 用户需自行组装工具 |
| G11 | FTS5 查询清洗 | memory.go L110-115 | **P1** | 用户输入可能导致语法错误 |
| G12 | RecordToolUse | memory.go L209-219 | **P2** | 工具调用无特殊记录 |
| G13 | ClearAll / ExportMemories | memory.go L198-204, L505-548 | **P2** | 运维便捷性缺失 |

### 2.2 架构债务

| # | 问题 | 位置 | 优先级 | 影响 |
|:-:|------|:----:|:------:|------|
| A1 | 缺少 Agent 接口抽象 | agent/react_loop.go | **P0** | 编排模式直接依赖具体类型 |
| A2 | Pool 配置断层 | pool/types.go L69-72 | **P0** | 丢失 Memory/Hooks/RAG 等配置 |
| A3 | 双消息总线并存 | a2a.go + collaboration.go | **P1** | 概念重叠，维护成本高 |
| A4 | Run/StreamRun 代码重复 | react_loop.go L323-L850 | **P1** | 维护困难 |
| A5 | 编排模式未导出 | pkg/ | **P1** | 用户无法使用 Pipeline/Handoff |
| A6 | 编排缺少条件分支和 Hooks | orchestration.go | **P1** | 仅线性顺序 |

---

## 3. 详细设计

### 3.1 Task 18: Scope/FileLock 自动注入工具流

**来源**: G1 — CodeCast 的 `canWriteFile` 和 `AcquireFileLock` 直接嵌入工具执行
**目标**: `FileScopePolicy` 和 `FileLockManager` 自动集成到 `Executor` 和 `FileSystem` 工具

**目标文件**:
- 修改: `internal/tools/executor.go` — Executor 增加 ScopePolicy 和 FileLockManager 字段
- 修改: `internal/tools/builtin/filesystem.go` — write/edit 操作自动检查 Scope 和获取 FileLock
- 修改: `internal/tools/builtin/shell.go` — 命令执行前检查 Scope
- 新增: `internal/tools/executor_test.go` — 集成测试

**设计要点**:

```go
// executor.go — Executor 增加可选依赖
type Executor struct {
    registry    *Registry
    scopePolicy ScopePolicy       // 新增：可选权限策略
    fileLock    *concurrency.FileLockManager  // 新增：可选文件锁
    logger      *slog.Logger
    timeout     time.Duration
}

// WithScopePolicy 注入权限策略
func (e *Executor) WithScopePolicy(policy ScopePolicy) *Executor

// WithFileLock 注入文件锁管理器
func (e *Executor) WithFileLock(fl *concurrency.FileLockManager) *Executor

// Execute 增加权限检查步骤
func (e *Executor) Execute(ctx context.Context, tc *FunctionCall) (*Result, error) {
    // 1. 查找工具
    // 2. 权限检查（新增）：if scopePolicy != nil, check Allow
    // 3. 执行工具
    // 4. 返回结果
}
```

```go
// filesystem.go — write/edit 自动使用 FileLock + ScopePolicy
type FileSystem struct {
    rootDir     string
    scopePolicy tools.ScopePolicy              // 新增
    fileLock    *concurrency.FileLockManager   // 新增
    maxReadSize  int64                         // 新增：默认 4MB
    maxWriteSize int64                         // 新增：默认 10MB
}

func (fs *FileSystem) writeFile(ctx context.Context, args json.RawMessage) (*tools.Result, error) {
    // 1. 解析参数获取 path
    // 2. ScopePolicy 检查（新增）
    // 3. FileLock 获取（新增）
    // 4. 文件大小检查（新增）
    // 5. 写入文件
    // 6. FileLock 释放（新增，defer）
}
```

**测试用例**:
```
TestExecutor_WithScopePolicy_Allowed       // 有权限时正常执行
TestExecutor_WithScopePolicy_Denied        // 无权限时拒绝执行
TestExecutor_WithScopePolicy_Nil           // 无策略时不检查
TestFileSystem_Write_WithFileLock          // 写文件自动获取/释放锁
TestFileSystem_Write_ScopeDenied           // 写文件越权被拒绝
TestFileSystem_Edit_WithFileLock           // 编辑文件自动获取/释放锁
TestFileSystem_Edit_ScopeDenied            // 编辑文件越权被拒绝
TestFileSystem_Read_ScopeDenied            // 读文件越权被拒绝
TestShell_Execute_ScopeDenied              // 命令执行越权被拒绝
```

---

### 3.2 Task 19: System Prompt 模板引擎

**来源**: G2 — CodeCast 的 `agentSystemPrompt` 自动注入任务描述和 FilesScope 规则
**目标**: 支持模板变量替换，自动注入 Scope 规则到 System Prompt

**目标文件**:
- 新增: `internal/agent/prompt.go` — 模板引擎
- 新增: `internal/agent/prompt_test.go` — 测试
- 修改: `internal/agent/react_loop.go` — Run 时自动渲染模板

**设计要点**:

```go
// prompt.go
package agent

// PromptTemplate 支持 {{.Variable}} 格式的模板
type PromptTemplate struct {
    template string
    vars     map[string]string
}

// NewPromptTemplate 创建模板
func NewPromptTemplate(tmpl string) *PromptTemplate

// WithVar 设置模板变量
func (t *PromptTemplate) WithVar(key, value string) *PromptTemplate

// WithScopeRules 自动注入 FilesScope 规则文本
func (t *PromptTemplate) WithScopeRules(scopes []string) *PromptTemplate

// Render 渲染模板，返回最终 prompt
func (t *PromptTemplate) Render() (string, error)

// DefaultSystemPrompt 返回内置的默认 System Prompt 模板
func DefaultSystemPrompt() *PromptTemplate
```

**内置模板**:

```
你是一个 AI 助手，名为 {{.AgentName}}。

{{if .ScopeRules}}
## 文件操作权限
你只能操作以下文件或目录：
{{.ScopeRules}}

如果用户要求你操作范围外的文件，请拒绝并说明原因。
{{end}}

{{if .TaskDescription}}
## 任务描述
{{.TaskDescription}}
{{end}}

请逐步思考和行动，使用可用的工具来完成任务。
```

**集成点**:
- `ReActConfig` 增加 `PromptTemplate *PromptTemplate` 字段
- `Run` 方法启动时：如果 PromptTemplate 非空，渲染后赋值给 SystemPrompt
- `Pool.Dispatch` 创建 Agent 时：自动从 TaskConfig.FilesScope 生成 ScopeRules

**测试用例**:
```
TestPromptTemplate_SimpleVar              // 简单变量替换
TestPromptTemplate_MultipleVars           // 多变量替换
TestPromptTemplate_ScopeRules             // Scope 规则注入
TestPromptTemplate_TaskDescription        // 任务描述注入
TestPromptTemplate_ConditionalBlock       // 条件块渲染
TestPromptTemplate_MissingVar             // 缺失变量处理
TestPromptTemplate_DefaultTemplate        // 默认模板渲染
TestPromptTemplate_EmptyTemplate          // 空模板处理
TestReActAgent_WithPromptTemplate         // ReActAgent 集成测试
```

---

### 3.3 Task 20: 统一 Agent 接口 + Pool 配置修复

**来源**: A1 + A2 — 缺少 Agent 接口抽象，Pool 配置断层
**目标**: 定义 Agent 接口，修复 Pool 到 Agent 的配置传递

**目标文件**:
- 修改: `internal/agent/types.go` — 新增 Agent 接口
- 修改: `internal/agent/react_loop.go` — ReActAgent 实现 Agent 接口
- 修改: `internal/pool/types.go` — 用 AgentFactory 替代 ReActAgentConfig
- 修改: `internal/pool/dispatcher.go` — 使用 AgentFactory 创建 Agent
- 修改: `internal/agent/orchestration.go` — Pipeline/Handoff/Parallel 面向 Agent 接口
- 修改: `internal/agent/a2a.go` — A2AAgent 面向 Agent 接口
- 修改: `pkg/agent.go` — 导出 Agent 接口

**设计要点**:

```go
// agent/types.go — 新增 Agent 接口
type Agent interface {
    // Run 执行同步推理
    Run(ctx context.Context, input string, opts ...Option) (*Response, error)
    // StreamRun 执行流式推理
    StreamRun(ctx context.Context, input string) (<-chan StreamEvent, error)
    // Stop 停止运行
    Stop()
    // Stats 返回运行统计
    Stats() AgentStats
    // Name 返回 Agent 名称
    Name() string
}
```

```go
// pool/types.go — AgentFactory 替代 ReActAgentConfig
type AgentFactory func(config AgentFactoryConfig) agent.Agent

type AgentFactoryConfig struct {
    Name        string
    SystemPrompt string
    PromptTemplate *agent.PromptTemplate  // 新增
    MaxTurns    int
    Temperature float64
    FilesScope  []string                  // 新增
    SessionID   string                    // 新增
    Metadata    map[string]string         // 新增
}
```

```go
// pool/dispatcher.go — Pool 使用 AgentFactory
type Pool struct {
    // ... 现有字段 ...
    agentFactory AgentFactory  // 替代 model + toolkit + defaultAgent
    memory       memory.MemoryStore       // 新增：共享 Memory
    scopePolicy  tools.ScopePolicy        // 新增：共享 ScopePolicy
    fileLock     *concurrency.FileLockManager  // 新增：共享 FileLock
    metrics      metrics.MetricsRecorder  // 新增：共享 Metrics
}
```

**迁移策略**:
- `ReActAgent` 实现 `Agent` 接口（零破坏性，现有代码无需修改）
- `Pool` 新增 `WithAgentFactory` 方法，兼容旧的 `SetModel`/`SetToolkit` 模式
- `Pipeline`/`Handoff`/`ParallelRun` 参数从 `*ReActAgent` 改为 `Agent` 接口

**测试用例**:
```
TestAgentInterface_ReActAgent_Implements   // ReActAgent 实现 Agent 接口
TestAgentFactory_DefaultFactory            // 默认工厂创建 ReActAgent
TestAgentFactory_WithScopePolicy           // 工厂注入 ScopePolicy
TestAgentFactory_WithMemory                // 工厂注入 Memory
TestAgentFactory_WithFileLock              // 工厂注入 FileLock
TestPool_WithAgentFactory                  // Pool 使用工厂创建 Agent
TestPool_AgentReceivesFullConfig           // Agent 接收完整配置
TestPipeline_WithAgentInterface            // Pipeline 面向接口编程
TestHandoff_WithAgentInterface             // Handoff 面向接口编程
TestParallelRun_WithAgentInterface         // ParallelRun 面向接口编程
```

---

### 3.4 Task 21: 工具安全增强

**来源**: G3 + G4 + G5 — edit_file 唯一匹配、文件大小限制、输出截断
**目标**: 补齐工具安全防护，防止意外修改和资源耗尽

**目标文件**:
- 修改: `internal/tools/builtin/filesystem.go` — edit 唯一匹配 + 大小限制
- 修改: `internal/tools/builtin/shell.go` — 输出截断
- 修改: `internal/tools/builtin/web.go` — 内容截断
- 修改: `internal/memory/sqlite.go` — FTS5 查询清洗
- 新增测试用例到现有测试文件

**设计要点**:

```go
// filesystem.go — edit 唯一匹配
func (fs *FileSystem) editFile(ctx context.Context, args json.RawMessage) (*tools.Result, error) {
    // ... 解析参数 ...
    content, _ := os.ReadFile(path)
    count := strings.Count(content, oldString)
    if count == 0 {
        return tools.NewErrorResult("old_string not found in file"), nil
    }
    if count > 1 {
        return tools.NewErrorResult(fmt.Sprintf(
            "old_string found %d times in file, expected exactly 1 occurrence. "+
            "Please provide more context to make the match unique.", count)), nil
    }
    // 唯一匹配时才替换
    newContent := strings.Replace(content, oldString, newString, 1)
}

// filesystem.go — 文件大小限制
const (
    defaultMaxReadSize  = 4 * 1024 * 1024   // 4MB
    defaultMaxWriteSize = 10 * 1024 * 1024   // 10MB
)

// shell.go — 输出截断
const defaultMaxOutputSize = 50000 // 50KB

func (s *Shell) Execute(ctx context.Context, args json.RawMessage) (*tools.Result, error) {
    // ... 执行命令 ...
    if len(output) > defaultMaxOutputSize {
        output = output[:defaultMaxOutputSize] + "\n... [输出已截断，总长度超过 50000 字符]"
    }
}

// memory/sqlite.go — FTS5 查询清洗
func sanitizeFTSQuery(query string) string {
    // 移除 FTS5 特殊字符: 引号、星号、括号、AND/OR/NOT
    re := regexp.MustCompile(`["*(){}]`)
    query = re.ReplaceAllString(query, "")
    // 转义关键字
    keywords := []string{"AND", "OR", "NOT", "NEAR"}
    for _, kw := range keywords {
        query = strings.ReplaceAll(query, kw, "")
    }
    return strings.TrimSpace(query)
}
```

**测试用例**:
```
TestFileSystem_Edit_UniqueMatch            // 唯一匹配正常替换
TestFileSystem_Edit_NoMatch                // 未找到匹配报错
TestFileSystem_Edit_MultipleMatches        // 多处匹配报错
TestFileSystem_Read_MaxSize                // 超过大小限制报错
TestFileSystem_Write_MaxSize               // 超过大小限制报错
TestShell_OutputTruncation                 // 长输出自动截断
TestShell_OutputUnderLimit                 // 短输出不截断
TestWeb_ContentTruncation                  // 长内容自动截断
TestMemory_SanitizeFTSQuery_SpecialChars   // 特殊字符清洗
TestMemory_SanitizeFTSQuery_Keywords       // 关键字清洗
TestMemory_SanitizeFTSQuery_Normal         // 正常查询不修改
```

---

### 3.5 Task 22: Memory 异步摘要 + 自动清理调度

**来源**: G6 + G7 — CodeCast 的 `ExtractSummaryAsync` 和 `StartAutoCleanup`
**目标**: 自动生成记忆摘要和标签，自动调度过期清理

**目标文件**:
- 修改: `internal/memory/sqlite.go` — 新增 StartAutoCleanup 和 ExtractSummaryAsync
- 新增: `internal/memory/summarizer.go` — 摘要提取器
- 新增: `internal/memory/summarizer_test.go` — 测试
- 修改: `internal/memory/memory_test.go` — 补充测试

**设计要点**:

```go
// summarizer.go — 摘要提取器
type Summarizer struct {
    provider   llm.Provider
    model      string  // 可选：轻量模型名称
    maxRetries int
}

type SummaryResult struct {
    Summary string
    Topics  string
}

func NewSummarizer(provider llm.Provider) *Summarizer

// ExtractSummary 从内容中提取摘要和标签
func (s *Summarizer) ExtractSummary(ctx context.Context, content string) (*SummaryResult, error)

// WithModel 设置摘要使用的模型（如 flash/mini 版本）
func (s *Summarizer) WithModel(model string) *Summarizer
```

```go
// sqlite.go — 自动清理调度
type CleanupConfig struct {
    MaxAgeDays    int           // 默认 30 天
    Interval      time.Duration // 默认 24 小时
    PreserveRoles []string      // 默认 ["tool"]，保留工具记录
}

// StartAutoCleanup 启动后台清理 goroutine
func (s *SQLiteStore) StartAutoCleanup(cfg CleanupConfig) (stop func())

// ExtractSummaryAsync 异步提取摘要（不阻塞调用方）
func (s *SQLiteStore) ExtractSummaryAsync(id string, summarizer *Summarizer) <-chan error
```

**集成点**:
- `ReActConfig` 增加 `Summarizer *memory.Summarizer` 字段
- ReActAgent 每 turn 结束后，如果 Summarizer 非空，异步提取摘要
- Pool 创建 Memory 时，可选启动 AutoCleanup

**测试用例**:
```
TestSummarizer_ExtractSummary              // 正常提取摘要
TestSummarizer_ExtractSummary_EmptyContent // 空内容处理
TestSummarizer_ExtractSummary_Error        // LLM 调用失败处理
TestSummarizer_WithModel                   // 自定义模型
TestSQLiteStore_StartAutoCleanup           // 自动清理启动和停止
TestSQLiteStore_StartAutoCleanup_Expired   // 过期记录被清理
TestSQLiteStore_StartAutoCleanup_PreserveTool // 工具记录被保留
TestSQLiteStore_ExtractSummaryAsync        // 异步摘要提取
TestSQLiteStore_ExtractSummaryAsync_Error  // 异步摘要失败不阻塞
TestReActAgent_WithSummarizer              // ReActAgent 集成测试
```

---

### 3.6 Task 23: 统一消息总线

**来源**: A3 — A2ABus 和 AgentBus 功能重叠
**目标**: 合并为单一 MessageBus 接口，支持进程内和跨进程扩展

**目标文件**:
- 新增: `internal/agent/bus.go` — 统一 MessageBus 接口和实现
- 新增: `internal/agent/bus_test.go` — 测试
- 修改: `internal/agent/a2a.go` — A2AAgent 改用 MessageBus
- 修改: `internal/agent/collaboration.go` — Collaborator 改用 MessageBus
- 修改: `pkg/agent.go` — 导出 MessageBus

**设计要点**:

```go
// bus.go — 统一消息总线
type MessageBus interface {
    // Send 发送消息到指定 Agent
    Send(ctx context.Context, msg *BusMessage) error
    // Broadcast 广播消息到所有 Agent
    Broadcast(ctx context.Context, msg *BusMessage) error
    // Subscribe 订阅消息
    Subscribe(agentID string, handler MessageHandler) (unsub func())
    // Register 注册 Agent
    Register(agentID string, handler MessageHandler) error
    // Unregister 注销 Agent
    Unregister(agentID string) error
    // ListAgents 列出已注册 Agent
    ListAgents() []string
}

type BusMessage struct {
    ID        string            `json:"id"`
    From      string            `json:"from"`
    To        string            `json:"to,omitempty"`
    Type      BusMessageType    `json:"type"`
    Content   string            `json:"content"`
    Metadata  map[string]string `json:"metadata,omitempty"`
    Timestamp time.Time         `json:"timestamp"`
}

type BusMessageType string
const (
    BusMsgTaskRequest  BusMessageType = "task_request"
    BusMsgTaskResult   BusMessageType = "task_result"
    BusMsgQuery        BusMessageType = "query"
    BusMsgResponse     BusMessageType = "response"
    BusMsgHandoff      BusMessageType = "handoff"
    BusMsgBroadcast    BusMessageType = "broadcast"
    BusMsgStatusUpdate BusMessageType = "status_update"
)

// LocalMessageBus 进程内实现（合并 A2ABus + AgentBus 能力）
type LocalMessageBus struct {
    mu       sync.RWMutex
    agents   map[string]MessageHandler
    channels map[string][]chan *BusMessage
}
```

**迁移策略**:
- `A2ABus` 和 `AgentBus` 标记为 deprecated，内部委托给 `LocalMessageBus`
- `A2AMessage` 和 `AgentMessage` 统一为 `BusMessage`
- 新代码直接使用 `MessageBus` 接口

**测试用例**:
```
TestLocalMessageBus_RegisterAndUnregister   // 注册和注销
TestLocalMessageBus_Send                    // 点对点发送
TestLocalMessageBus_SendToUnregistered      // 发送到未注册 Agent
TestLocalMessageBus_Broadcast               // 广播消息
TestLocalMessageBus_Subscribe               // 订阅消息
TestLocalMessageBus_SubscribeFilter         // 订阅过滤
TestLocalMessageBus_ConcurrentAccess        // 并发安全
TestLocalMessageBus_ListAgents              // 列出 Agent
TestA2AAgent_WithMessageBus                 // A2AAgent 使用新总线
TestCollaborator_WithMessageBus             // Collaborator 使用新总线
```

---

### 3.7 Task 24: Run/StreamRun 去重 + 编排 Hooks

**来源**: A4 + A6 — Run 和 StreamRun 代码重复，编排缺少 Hooks
**目标**: 抽象共享循环引擎，为编排模式增加 Hooks

**目标文件**:
- 修改: `internal/agent/react_loop.go` — 抽象 reactLoopEngine
- 修改: `internal/agent/orchestration.go` — 增加 Pipeline/Handoff Hooks
- 修改: `internal/agent/hooks.go` — 新增编排级 HookPoint

**设计要点**:

```go
// react_loop.go — 抽象共享引擎
type loopOutputMode int
const (
    outputSync   loopOutputMode = iota
    outputStream
)

// reactLoopEngine 共享的 ReAct 循环引擎
func (a *ReActAgent) reactLoopEngine(ctx context.Context, input string, mode loopOutputMode, streamCh chan<- StreamEvent) (*Response, error) {
    // 合并 Run 和 StreamRun 的核心逻辑
    // mode == outputSync: 直接返回 Response
    // mode == outputStream: 发送 StreamEvent 到 streamCh
}

// Run 调用共享引擎
func (a *ReActAgent) Run(ctx context.Context, input string, opts ...Option) (*Response, error) {
    return a.reactLoopEngine(ctx, input, outputSync, nil)
}

// StreamRun 调用共享引擎
func (a *ReActAgent) StreamRun(ctx context.Context, input string) (<-chan StreamEvent, error) {
    ch := make(chan StreamEvent, 100)
    go func() {
        _, _ = a.reactLoopEngine(ctx, input, outputStream, ch)
        close(ch)
    }()
    return ch, nil
}
```

```go
// hooks.go — 新增编排级 HookPoint
const (
    // ... 现有 HookPoint ...
    HookBeforePipelineStep  HookPoint = "before_pipeline_step"
    HookAfterPipelineStep   HookPoint = "after_pipeline_step"
    HookBeforeHandoff       HookPoint = "before_handoff"
    HookAfterHandoff        HookPoint = "after_handoff"
    HookBeforeParallelAgent HookPoint = "before_parallel_agent"
    HookAfterParallelAgent  HookPoint = "after_parallel_agent"
)
```

**测试用例**:
```
TestReActAgent_Run_SameResultAsBefore       // Run 行为不变
TestReActAgent_StreamRun_SameResultAsBefore // StreamRun 行为不变
TestReActAgent_EngineSyncMode              // 引擎同步模式
TestReActAgent_EngineStreamMode            // 引擎流式模式
TestPipeline_Hooks_Fired                   // Pipeline Hook 触发
TestHandoff_Hooks_Fired                    // Handoff Hook 触发
TestParallelRun_Hooks_Fired                // Parallel Hook 触发
```

---

### 3.8 Task 25: Session 分组管理 + 按会话取消

**来源**: G8 — CodeCast 的 `GetAgentsBySession` + `CancelBySession`
**目标**: Pool 支持按 Session 分组查询和取消

**目标文件**:
- 修改: `internal/pool/types.go` — TaskConfig 增加 SessionID，Pool 新增方法
- 修改: `internal/pool/dispatcher.go` — 实现 Session 级操作
- 修改: `internal/pool/pool_test.go` — 补充测试

**设计要点**:

```go
// types.go — TaskConfig 增加 SessionID
type TaskConfig struct {
    ID          string
    Title       string
    Prompt      string
    SessionID   string            // 新增：会话分组
    Tools       []string
    FilesScope  []string
    MaxTurns    int
    Metadata    map[string]string
}

// dispatcher.go — Pool 新增 Session 级方法
func (p *Pool) GetTasksBySession(sessionID string) []TaskResult
func (p *Pool) CancelBySession(sessionID string) error
func (p *Pool) GetTask(id string) (TaskResult, bool)  // 新增：按 ID 查询
```

**测试用例**:
```
TestPool_GetTasksBySession                 // 按会话查询任务
TestPool_GetTasksBySession_Empty           // 不存在的会话返回空
TestPool_CancelBySession                   // 按会话取消任务
TestPool_CancelBySession_NoTasks           // 不存在的会话无操作
TestPool_GetTask                           // 按 ID 查询任务
TestPool_GetTask_NotFound                  // 不存在的任务
TestPool_Dispatch_WithSessionID            // 带会话 ID 的任务分发
```

---

### 3.9 Task 26: 目录级搜索 + 默认工具集

**来源**: G9 + G10 — 缺少 grep -r 级搜索，缺少开箱即用的工具配置
**目标**: 新增目录递归搜索，提供 DefaultToolkit 快捷方法

**目标文件**:
- 修改: `internal/tools/builtin/filesystem.go` — 新增 searchDirectory
- 新增: `internal/tools/toolkit.go` — 默认工具集配置
- 新增: `internal/tools/toolkit_test.go` — 测试

**设计要点**:

```go
// filesystem.go — 目录级搜索
type searchDirectoryParams struct {
    Path       string `json:"path"`
    Query      string `json:"query"`
    Include    string `json:"include,omitempty"`  // 如 "*.go", "*.ts"
    MaxResults int    `json:"max_results,omitempty"`
}

func (fs *FileSystem) searchDirectory(ctx context.Context, args json.RawMessage) (*tools.Result, error) {
    // 递归遍历目录，按 include 模式过滤文件
    // 在每个文件中搜索 query
    // 返回匹配结果（文件名:行号:内容）
}
```

```go
// toolkit.go — 默认工具集
type ToolkitConfig struct {
    RootDir     string
    EnableFS    bool  // 默认 true
    EnableShell bool  // 默认 true
    EnableWeb   bool  // 默认 true
    EnableSearch bool // 默认 true
    ScopePolicy ScopePolicy
    FileLock    *concurrency.FileLockManager
}

// DefaultToolkit 创建包含所有内置工具的 Registry
func DefaultToolkit(cfg ToolkitConfig) (*Registry, error)

// MinimalToolkit 创建仅包含 FS + Shell 的 Registry
func MinimalToolkit(rootDir string) (*Registry, error)
```

**测试用例**:
```
TestFileSystem_SearchDirectory_Basic        // 目录级搜索基本功能
TestFileSystem_SearchDirectory_WithInclude  // 按文件模式过滤
TestFileSystem_SearchDirectory_NoMatch      // 无匹配结果
TestFileSystem_SearchDirectory_DeepNested   // 深层嵌套目录
TestDefaultToolkit_AllTools                 // 默认工具集包含所有工具
TestDefaultToolkit_WithScopePolicy          // 工具集注入 ScopePolicy
TestDefaultToolkit_WithFileLock             // 工具集注入 FileLock
TestMinimalToolkit                          // 最小工具集
```

---

### 3.10 Task 27: 编排模式导出 + 嵌套组合

**来源**: A5 + A6 — 编排模式未导出，缺少条件分支
**目标**: 导出编排 API，支持 Pipeline 条件分支和嵌套组合

**目标文件**:
- 修改: `internal/agent/orchestration.go` — Pipeline 增加条件步骤
- 修改: `pkg/agent.go` — 导出编排类型
- 新增: `pkg/orchestration.go` — 编排公共 API
- 新增: `internal/agent/orchestration_test.go` — 补充测试

**设计要点**:

```go
// orchestration.go — Pipeline 条件步骤
type PipelineStep struct {
    Agent    Agent
    Input    string
    Name     string
    // 新增：条件执行
    Condition func(ctx context.Context, prevResult *StepResult) bool
}

// Pipeline 条件步骤：如果 Condition 返回 false，跳过此步骤
```

```go
// pkg/orchestration.go — 编排公共 API
type Pipeline = agent.Pipeline
type PipelineStep = agent.PipelineStep
type PipelineResult = agent.PipelineResult
type Handoff = agent.Handoff
type HandoffConfig = agent.HandoffConfig
type HandoffResult = agent.HandoffResult
type MessageBus = agent.MessageBus
type BusMessage = agent.BusMessage

var NewPipeline = agent.NewPipeline
var NewHandoff = agent.NewHandoff
var NewLocalMessageBus = agent.NewLocalMessageBus
var ParallelRun = agent.ParallelRun
```

**测试用例**:
```
TestPipeline_ConditionalStep_Skip           // 条件不满足时跳过步骤
TestPipeline_ConditionalStep_Execute        // 条件满足时执行步骤
TestPipeline_NestedPipeline                 // 嵌套 Pipeline
TestPipeline_WithTimeout                    // 步骤级超时
TestHandoff_WithLLMRouter                   // LLM 驱动的路由
TestOrchestration_ExportedTypes             // 导出类型可用
```

---

### 3.11 Task 28: 分布式 Agent 通信（HTTP 传输层）

**来源**: Phase 2 新功能
**目标**: 支持跨进程 Agent 通信，基于 HTTP 传输

**目标文件**:
- 新增: `internal/agent/transport.go` — Transport 接口
- 新增: `internal/agent/http_transport.go` — HTTP 实现
- 新增: `internal/agent/http_transport_test.go` — 测试

**设计要点**:

```go
// transport.go — 传输层抽象
type Transport interface {
    // Send 发送消息到远程 Agent
    Send(ctx context.Context, target string, msg *BusMessage) error
    // Receive 接收远程消息
    Receive() <-chan *BusMessage
    // Start 启动传输层
    Start(addr string) error
    // Close 关闭传输层
    Close() error
}

// http_transport.go — HTTP 实现
type HTTPTransport struct {
    client   *http.Client
    server   *http.Server
    inbound  chan *BusMessage
    handlers map[string]MessageHandler
}

func NewHTTPTransport() *HTTPTransport
```

**测试用例**:
```
TestHTTPTransport_SendAndReceive            // 发送和接收消息
TestHTTPTransport_StartAndClose             // 启动和关闭
TestHTTPTransport_ConcurrentMessages        // 并发消息处理
TestHTTPTransport_Timeout                   // 超时处理
TestHTTPTransport_LargeMessage              // 大消息传输
```

---

### 3.12 Task 29: 性能基准测试套件

**来源**: Phase 2 新功能
**目标**: 建立性能基准，防止性能退化

**目标文件**:
- 新增: `internal/agent/bench_test.go` — Agent 基准
- 新增: `internal/pool/bench_test.go` — Pool 基准
- 新增: `internal/memory/bench_test.go` — Memory 基准
- 新增: `internal/tools/bench_test.go` — Tools 基准

**设计要点**:

```go
// bench_test.go — 使用 Go 标准 testing.B
func BenchmarkReActAgent_SimpleCompletion(b *testing.B)
func BenchmarkReActAgent_SingleToolCall(b *testing.B)
func BenchmarkReActAgent_MaxTurns(b *testing.B)
func BenchmarkPool_Dispatch_10Agents(b *testing.B)
func BenchmarkPool_Dispatch_100Agents(b *testing.B)
func BenchmarkMemory_Add(b *testing.B)
func BenchmarkMemory_Search(b *testing.B)
func BenchmarkMemory_FTS5Search(b *testing.B)
func BenchmarkTools_Filesystem_Read(b *testing.B)
func BenchmarkTools_Filesystem_Write(b *testing.B)
func BenchmarkTools_Shell_Execute(b *testing.B)
```

---

### 3.13 Task 30: 更多 LLM Provider

**来源**: Phase 2 新功能
**目标**: 支持 Cohere 和 Mistral 等 LLM Provider

**目标文件**:
- 新增: `internal/llm/cohere_provider.go`
- 新增: `internal/llm/mistral_provider.go`
- 新增: `internal/llm/cohere_test.go`
- 新增: `internal/llm/mistral_test.go`

**设计要点**: 复用 OpenAI Provider 的 HTTP 调用模式，适配各 Provider 的 API 差异

---

### 3.14 Task 31: Web UI 管理面板

**来源**: Phase 2 新功能
**目标**: 提供 Web UI 查看 Agent 状态、Memory、Metrics

**目标文件**:
- 新增: `cmd/admin/main.go` — 管理面板入口
- 新增: `internal/admin/handler.go` — HTTP handler
- 新增: `internal/admin/handler_test.go` — 测试

**设计要点**:

```go
// admin/handler.go
type AdminHandler struct {
    pool   *pool.Pool
    memory memory.MemoryStore
    metrics *metrics.AgentMetrics
}

// GET /api/agents        — 列出所有 Agent 状态
// GET /api/agents/:id    — 查看 Agent 详情
// GET /api/memory        — 查看记忆列表
// GET /api/memory/search — 搜索记忆
// GET /api/metrics       — Prometheus 格式指标
// GET /                  — 内嵌的静态管理页面
```

---

## 4. 文件结构变更

```
agentprimordia/
├── internal/
│   ├── agent/
│   │   ├── types.go               ← 修改: 新增 Agent 接口
│   │   ├── react_loop.go          ← 修改: 抽象引擎 + PromptTemplate
│   │   ├── prompt.go              ← 🆕 Task 19
│   │   ├── bus.go                 ← 🆕 Task 23
│   │   ├── transport.go           ← 🆕 Task 28
│   │   ├── http_transport.go      ← 🆕 Task 28
│   │   ├── orchestration.go       ← 修改: 条件步骤 + Hooks
│   │   ├── a2a.go                 ← 修改: 改用 MessageBus
│   │   ├── collaboration.go       ← 修改: 改用 MessageBus
│   │   └── bench_test.go          ← 🆕 Task 29
│   │
│   ├── memory/
│   │   ├── sqlite.go              ← 修改: 自动清理 + FTS5 清洗
│   │   ├── summarizer.go          ← 🆕 Task 22
│   │   └── bench_test.go          ← 🆕 Task 29
│   │
│   ├── pool/
│   │   ├── types.go               ← 修改: AgentFactory + SessionID
│   │   ├── dispatcher.go          ← 修改: Session 级操作
│   │   └── bench_test.go          ← 🆕 Task 29
│   │
│   ├── tools/
│   │   ├── executor.go            ← 修改: ScopePolicy + FileLock
│   │   ├── toolkit.go             ← 🆕 Task 26
│   │   ├── builtin/
│   │   │   ├── filesystem.go      ← 修改: 安全增强 + 目录搜索
│   │   │   ├── shell.go           ← 修改: 输出截断
│   │   │   └── web.go             ← 修改: 内容截断
│   │   └── bench_test.go          ← 🆕 Task 29
│   │
│   ├── llm/
│   │   ├── cohere_provider.go     ← 🆕 Task 30
│   │   └── mistral_provider.go    ← 🆕 Task 30
│   │
│   └── admin/                     ← 🆕 Task 31
│       ├── handler.go
│       └── handler_test.go
│
├── cmd/
│   └── admin/                     ← 🆕 Task 31
│       └── main.go
│
├── pkg/
│   ├── agent.go                   ← 修改: 导出 Agent 接口 + MessageBus
│   ├── orchestration.go           ← 🆕 Task 27
│   └── pool.go                    ← 修改: 导出 AgentFactory
│
└── docs/
    └── specs/
        └── 2026-05-29-phase2-design.md  ← 本文档
```

---

## 5. 执行顺序

```
子阶段 2-A (P0): CodeCast 对齐 + 架构修复
  Task 18: Scope/FileLock 自动注入    ← 多 Agent 并发安全保障
  Task 19: System Prompt 模板引擎     ← 权限规则自动注入
  Task 20: 统一 Agent 接口            ← 架构基础，后续 Task 依赖
  Task 21: 工具安全增强               ← 防止意外修改和资源耗尽
  Task 22: Memory 异步摘要 + 清理     ← 记忆质量保障

子阶段 2-B (P1): 架构统一 + 编排增强
  Task 23: 统一消息总线               ← 消除架构债务
  Task 24: Run/StreamRun 去重         ← 代码质量
  Task 25: Session 分组管理           ← 多会话支持
  Task 26: 目录级搜索 + 默认工具集    ← 开箱即用
  Task 27: 编排模式导出               ← 公共 API 完善

子阶段 2-C (P2): 新能力扩展
  Task 28: 分布式 Agent 通信          ← 跨进程协作
  Task 29: 性能基准测试               ← 防止性能退化
  Task 30: 更多 LLM Provider          ← 生态扩展
  Task 31: Web UI 管理面板            ← 可视化运维
```

---

## 6. 成功指标

- [ ] 子阶段 2-A 完成后：CodeCast 可基于 AP 运行完整工作流
- [ ] 子阶段 2-B 完成后：公共 API 完整，编排能力增强
- [ ] 子阶段 2-C 完成后：框架具备分布式和可观测性能力
- [ ] 全量测试 `go test ./...` 零失败（目标 ~300 tests）
- [ ] 性能基准测试无退化
- [ ] 零新增外部依赖（除已有的 modernc.org/sqlite）

---

*Spec Version: 1.0 | Created: 2026-05-29*
