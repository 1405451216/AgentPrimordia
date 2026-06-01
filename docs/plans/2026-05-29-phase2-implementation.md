# Phase 2: 均衡推进 — 实施计划

> **Spec**: [docs/specs/2026-05-29-phase2-design.md](../specs/2026-05-29-phase2-design.md)
> **日期**: 2026-05-29
> **状态**: Ready to Execute
> **前置条件**: Phase 1-D (Tasks 10-17) 已完成，~195 tests 全通过

---

## 总览

| # | Task | 子阶段 | 模块 | 来源 | 复杂度 | 预估测试数 |
|:-:|------|:------:|------|:----:|:------:|:----------:|
| 18 | Scope/FileLock 自动注入工具流 | 2-A | tools/ | CC 差距 | ⭐⭐ | ~9 |
| 19 | System Prompt 模板引擎 | 2-A | agent/ | CC 差距 | ⭐⭐ | ~9 |
| 20 | 统一 Agent 接口 + Pool 配置修复 | 2-A | agent/ + pool/ | 架构 | ⭐⭐⭐ | ~10 |
| 21 | 工具安全增强 | 2-A | tools/builtin | CC 差距 | ⭐⭐ | ~11 |
| 22 | Memory 异步摘要 + 自动清理调度 | 2-A | memory/ | CC 差距 | ⭐⭐⭐ | ~10 |
| 23 | 统一消息总线 | 2-B | agent/ | 架构 | ⭐⭐⭐ | ~10 |
| 24 | Run/StreamRun 去重 + 编排 Hooks | 2-B | agent/ | 架构 | ⭐⭐ | ~7 |
| 25 | Session 分组管理 + 按会话取消 | 2-B | pool/ | CC 差距 | ⭐⭐ | ~7 |
| 26 | 目录级搜索 + 默认工具集 | 2-B | tools/ | CC 差距 | ⭐⭐ | ~8 |
| 27 | 编排模式导出 + 嵌套组合 | 2-B | pkg/ + agent/ | 架构 | ⭐⭐ | ~5 |
| 28 | 分布式 Agent 通信（HTTP 传输层） | 2-C | agent/transport | 新功能 | ⭐⭐⭐ | ~5 |
| 29 | 性能基准测试套件 | 2-C | test/bench | 新功能 | ⭐⭐ | ~11 benchmarks |
| 30 | 更多 LLM Provider | 2-C | llm/ | 新功能 | ⭐⭐ | ~8 |
| 31 | Web UI 管理面板 | 2-C | cmd/admin | 新功能 | ⭐⭐⭐ | ~6 |

**预估新增**: ~15 个文件，~3000 行代码，**~105 个新测试**

---

## 子阶段 2-A: CodeCast 对齐 + 架构修复（P0）

### Task 18: Scope/FileLock 自动注入工具流

**来源**: CodeCast `agent_tools.go` L55-67 的 `canWriteFile` 和 L128-141 的 `AcquireFileLock`
**目标**: `FileScopePolicy` 和 `FileLockManager` 自动集成到 `Executor` 和 `FileSystem`

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 修改 | `internal/tools/executor.go` | 增加 ScopePolicy 和 FileLockManager 字段 |
| 修改 | `internal/tools/builtin/filesystem.go` | write/edit 自动检查 Scope + FileLock |
| 修改 | `internal/tools/builtin/shell.go` | 命令执行前检查 Scope |
| 修改 | `internal/tools/tools_test.go` | 补充集成测试 |

#### 实现步骤

1. **Executor 增加可选依赖**
   - 新增 `scopePolicy ScopePolicy` 和 `fileLock *concurrency.FileLockManager` 字段
   - 新增 `WithScopePolicy(policy)` 和 `WithFileLock(fl)` 方法
   - `Execute` 方法中增加权限检查步骤

2. **FileSystem 自动集成**
   - `FileSystem` 增加 `scopePolicy` 和 `fileLock` 字段
   - `writeFile`/`editFile` 执行前：ScopePolicy.Allow 检查 + FileLock.Acquire
   - defer FileLock.Release

3. **Shell 自动集成**
   - `Shell` 增加 `scopePolicy` 字段
   - `Execute` 执行前：ScopePolicy.Allow 检查（以 workdir 为资源路径）

#### 测试用例清单

```
TestExecutor_WithScopePolicy_Allowed
TestExecutor_WithScopePolicy_Denied
TestExecutor_WithScopePolicy_Nil
TestFileSystem_Write_WithFileLock
TestFileSystem_Write_ScopeDenied
TestFileSystem_Edit_WithFileLock
TestFileSystem_Edit_ScopeDenied
TestFileSystem_Read_ScopeDenied
TestShell_Execute_ScopeDenied
```

---

### Task 19: System Prompt 模板引擎

**来源**: CodeCast `agent_engine.go` L25-52 的 `agentSystemPrompt`
**目标**: 支持模板变量替换，自动注入 Scope 规则

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 新增 | `internal/agent/prompt.go` | 模板引擎实现 |
| 新增 | `internal/agent/prompt_test.go` | 测试 |
| 修改 | `internal/agent/react_loop.go` | Run 时自动渲染模板 |

#### 实现步骤

1. **PromptTemplate 实现**
   - 使用 `text/template` 标准库
   - 支持 `{{.AgentName}}`、`{{.ScopeRules}}`、`{{.TaskDescription}}` 变量
   - 支持 `{{if .ScopeRules}}...{{end}}` 条件块
   - `WithVar(key, value)` 设置自定义变量
   - `WithScopeRules(scopes)` 自动生成权限规则文本
   - `Render()` 渲染最终 prompt

2. **内置默认模板**
   - 包含 Agent 名称、权限规则、任务描述、行为指引

3. **ReActConfig 集成**
   - 新增 `PromptTemplate *PromptTemplate` 字段
   - Run 启动时：如果 PromptTemplate 非空，渲染后赋值给 SystemPrompt

#### 测试用例清单

```
TestPromptTemplate_SimpleVar
TestPromptTemplate_MultipleVars
TestPromptTemplate_ScopeRules
TestPromptTemplate_TaskDescription
TestPromptTemplate_ConditionalBlock
TestPromptTemplate_MissingVar
TestPromptTemplate_DefaultTemplate
TestPromptTemplate_EmptyTemplate
TestReActAgent_WithPromptTemplate
```

---

### Task 20: 统一 Agent 接口 + Pool 配置修复

**来源**: 架构问题 A1 + A2
**目标**: 定义 Agent 接口，修复 Pool 到 Agent 的配置传递

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 修改 | `internal/agent/types.go` | 新增 Agent 接口 |
| 修改 | `internal/agent/react_loop.go` | ReActAgent 实现 Agent 接口 |
| 修改 | `internal/agent/orchestration.go` | 面向 Agent 接口 |
| 修改 | `internal/agent/a2a.go` | A2AAgent 面向 Agent 接口 |
| 修改 | `internal/pool/types.go` | AgentFactory 替代 ReActAgentConfig |
| 修改 | `internal/pool/dispatcher.go` | 使用 AgentFactory |
| 修改 | `pkg/agent.go` | 导出 Agent 接口 |

#### 实现步骤

1. **定义 Agent 接口**
   ```go
   type Agent interface {
       Run(ctx context.Context, input string, opts ...Option) (*Response, error)
       StreamRun(ctx context.Context, input string) (<-chan StreamEvent, error)
       Stop()
       Stats() AgentStats
       Name() string
   }
   ```

2. **ReActAgent 实现验证**
   - 确认 ReActAgent 已实现 Agent 接口的所有方法
   - 如有缺失方法，添加实现

3. **AgentFactory 设计**
   ```go
   type AgentFactory func(config AgentFactoryConfig) Agent

   type AgentFactoryConfig struct {
       Name          string
       SystemPrompt  string
       PromptTemplate *PromptTemplate
       MaxTurns      int
       Temperature   float64
       FilesScope    []string
       SessionID     string
       Metadata      map[string]string
   }
   ```

4. **Pool 重构**
   - 新增 `agentFactory AgentFactory` 字段
   - 新增 `memory`/`scopePolicy`/`fileLock`/`metrics` 共享字段
   - `createAgentForTask` 使用 AgentFactory，传递完整配置
   - 兼容旧 API：`SetModel`/`SetToolkit` 自动创建默认 AgentFactory

5. **编排模式迁移**
   - Pipeline/Handoff/ParallelRun 参数从 `*ReActAgent` 改为 `Agent`

#### 测试用例清单

```
TestAgentInterface_ReActAgent_Implements
TestAgentFactory_DefaultFactory
TestAgentFactory_WithScopePolicy
TestAgentFactory_WithMemory
TestAgentFactory_WithFileLock
TestPool_WithAgentFactory
TestPool_AgentReceivesFullConfig
TestPipeline_WithAgentInterface
TestHandoff_WithAgentInterface
TestParallelRun_WithAgentInterface
```

---

### Task 21: 工具安全增强

**来源**: CodeCast 差距 G3 + G4 + G5
**目标**: edit_file 唯一匹配、文件大小限制、输出截断、FTS5 清洗

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 修改 | `internal/tools/builtin/filesystem.go` | edit 唯一匹配 + 大小限制 |
| 修改 | `internal/tools/builtin/shell.go` | 输出截断 |
| 修改 | `internal/tools/builtin/web.go` | 内容截断 |
| 修改 | `internal/memory/sqlite.go` | FTS5 查询清洗 |
| 修改 | `internal/tools/builtin/filesystem_test.go` | 补充测试 |
| 修改 | `internal/tools/builtin/shell_test.go` | 补充测试 |
| 修改 | `internal/memory/memory_test.go` | 补充测试 |

#### 实现步骤

1. **edit_file 唯一匹配**
   - `strings.Count(content, oldString)` 计算匹配数
   - count == 0 → 返回 "old_string not found" 错误
   - count > 1 → 返回 "found N times, expected 1" 错误
   - count == 1 → 正常替换

2. **文件大小限制**
   - `FileSystem` 新增 `maxReadSize`（默认 4MB）和 `maxWriteSize`（默认 10MB）
   - readFile: 检查文件大小，超过限制返回错误
   - writeFile: 检查写入内容大小，超过限制返回错误

3. **命令输出截断**
   - `Shell` 新增 `maxOutputSize`（默认 50000 字符）
   - 执行后检查输出长度，超过则截断并添加提示

4. **Web 内容截断**
   - `Web` 新增 `maxContentSize`（默认 50000 字符）
   - 获取内容后检查长度，超过则截断

5. **FTS5 查询清洗**
   - `sqlite.go` 新增 `sanitizeFTSQuery(query string) string`
   - 移除 FTS5 特殊字符：引号、星号、括号
   - 移除关键字：AND、OR、NOT、NEAR
   - Search 方法调用前自动清洗

#### 测试用例清单

```
TestFileSystem_Edit_UniqueMatch
TestFileSystem_Edit_NoMatch
TestFileSystem_Edit_MultipleMatches
TestFileSystem_Read_MaxSize
TestFileSystem_Write_MaxSize
TestShell_OutputTruncation
TestShell_OutputUnderLimit
TestWeb_ContentTruncation
TestMemory_SanitizeFTSQuery_SpecialChars
TestMemory_SanitizeFTSQuery_Keywords
TestMemory_SanitizeFTSQuery_Normal
```

---

### Task 22: Memory 异步摘要 + 自动清理调度

**来源**: CodeCast `memory.go` L286-373 的 `ExtractSummaryAsync` 和 L265-284 的 `StartAutoCleanup`
**目标**: 自动生成记忆摘要和标签，自动调度过期清理

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 新增 | `internal/memory/summarizer.go` | 摘要提取器 |
| 新增 | `internal/memory/summarizer_test.go` | 测试 |
| 修改 | `internal/memory/sqlite.go` | StartAutoCleanup + ExtractSummaryAsync |
| 修改 | `internal/agent/react_loop.go` | ReActConfig 增加 Summarizer |

#### 实现步骤

1. **Summarizer 实现**
   - `NewSummarizer(provider llm.Provider)` 构造
   - `ExtractSummary(ctx, content)` 调用 LLM 生成摘要和标签
   - `WithModel(model)` 指定轻量模型（如 flash/mini 版本）
   - Prompt: "请为以下内容生成简短摘要（1-2句）和标签（逗号分隔）"

2. **StartAutoCleanup**
   - `CleanupConfig{MaxAgeDays, Interval, PreserveRoles}`
   - 启动后台 goroutine，定期调用 CleanupExpired
   - 返回 `stop func()` 供调用方停止
   - PreserveRoles 默认包含 "tool"，清理时保留工具记录

3. **ExtractSummaryAsync**
   - 启动 goroutine 调用 Summarizer.ExtractSummary
   - 成功后调用 UpdateSummary 更新 Episode
   - 失败时记录日志，不阻塞主流程
   - 返回 `<-chan error` 供可选的错误监听

4. **ReActAgent 集成**
   - `ReActConfig` 新增 `Summarizer *memory.Summarizer` 字段
   - 每 turn 结束后，如果 Summarizer 非空，异步提取摘要

#### 测试用例清单

```
TestSummarizer_ExtractSummary
TestSummarizer_ExtractSummary_EmptyContent
TestSummarizer_ExtractSummary_Error
TestSummarizer_WithModel
TestSQLiteStore_StartAutoCleanup
TestSQLiteStore_StartAutoCleanup_Expired
TestSQLiteStore_StartAutoCleanup_PreserveTool
TestSQLiteStore_ExtractSummaryAsync
TestSQLiteStore_ExtractSummaryAsync_Error
TestReActAgent_WithSummarizer
```

---

## 子阶段 2-B: 架构统一 + 编排增强（P1）

### Task 23: 统一消息总线

**来源**: 架构问题 A3 — A2ABus 和 AgentBus 功能重叠
**目标**: 合并为单一 MessageBus 接口

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 新增 | `internal/agent/bus.go` | 统一 MessageBus 接口 + LocalMessageBus |
| 新增 | `internal/agent/bus_test.go` | 测试 |
| 修改 | `internal/agent/a2a.go` | A2AAgent 改用 MessageBus |
| 修改 | `internal/agent/collaboration.go` | Collaborator 改用 MessageBus |
| 修改 | `pkg/agent.go` | 导出 MessageBus |

#### 实现步骤

1. **定义 MessageBus 接口**
   - Send / Broadcast / Subscribe / Register / Unregister / ListAgents

2. **实现 LocalMessageBus**
   - 合并 A2ABus 的 Send/Broadcast 和 AgentBus 的 Subscribe 能力
   - 统一 BusMessage 类型（合并 A2AMessage 和 AgentMessage）

3. **迁移 A2AAgent**
   - 内部改用 MessageBus 接口
   - 保留 A2AAgent 便捷方法（SendTo/Broadcast/Run）

4. **迁移 Collaborator**
   - 内部改用 MessageBus 接口
   - 保留 CollaborationPattern 枚举

5. **旧 API 兼容**
   - A2ABus 和 AgentBus 标记为 deprecated
   - 内部委托给 LocalMessageBus

#### 测试用例清单

```
TestLocalMessageBus_RegisterAndUnregister
TestLocalMessageBus_Send
TestLocalMessageBus_SendToUnregistered
TestLocalMessageBus_Broadcast
TestLocalMessageBus_Subscribe
TestLocalMessageBus_SubscribeFilter
TestLocalMessageBus_ConcurrentAccess
TestLocalMessageBus_ListAgents
TestA2AAgent_WithMessageBus
TestCollaborator_WithMessageBus
```

---

### Task 24: Run/StreamRun 去重 + 编排 Hooks

**来源**: 架构问题 A4 + A6
**目标**: 抽象共享循环引擎，为编排模式增加 Hooks

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 修改 | `internal/agent/react_loop.go` | 抽象 reactLoopEngine |
| 修改 | `internal/agent/hooks.go` | 新增编排级 HookPoint |
| 修改 | `internal/agent/orchestration.go` | Pipeline/Handoff 触发 Hooks |
| 修改 | `internal/agent/react_loop_test.go` | 补充测试 |

#### 实现步骤

1. **抽象 reactLoopEngine**
   - 新增 `loopOutputMode` 枚举（outputSync / outputStream）
   - 提取 Run 和 StreamRun 的共享逻辑到 `reactLoopEngine`
   - Run 调用 `reactLoopEngine(ctx, input, outputSync, nil)`
   - StreamRun 调用 `reactLoopEngine(ctx, input, outputStream, ch)`

2. **新增编排级 HookPoint**
   - HookBeforePipelineStep / HookAfterPipelineStep
   - HookBeforeHandoff / HookAfterHandoff
   - HookBeforeParallelAgent / HookAfterParallelAgent

3. **编排模式触发 Hooks**
   - Pipeline 每步前后触发 Hook
   - Handoff 每次交接前后触发 Hook
   - ParallelRun 每个 Agent 前后触发 Hook

#### 测试用例清单

```
TestReActAgent_Run_SameResultAsBefore
TestReActAgent_StreamRun_SameResultAsBefore
TestReActAgent_EngineSyncMode
TestReActAgent_EngineStreamMode
TestPipeline_Hooks_Fired
TestHandoff_Hooks_Fired
TestParallelRun_Hooks_Fired
```

---

### Task 25: Session 分组管理 + 按会话取消

**来源**: CodeCast 差距 G8
**目标**: Pool 支持按 Session 分组查询和取消

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 修改 | `internal/pool/types.go` | TaskConfig 增加 SessionID |
| 修改 | `internal/pool/dispatcher.go` | Session 级操作方法 |
| 修改 | `internal/pool/pool_test.go` | 补充测试 |

#### 实现步骤

1. **TaskConfig 增加 SessionID**
   - 新增 `SessionID string` 字段

2. **Pool 新增方法**
   - `GetTasksBySession(sessionID string) []TaskResult`
   - `CancelBySession(sessionID string) error`
   - `GetTask(id string) (TaskResult, bool)` — 按 ID 查询

3. **内部实现**
   - `poolTask` 增加 `sessionID` 字段
   - `GetTasksBySession` 遍历 tasks map 按 sessionID 过滤
   - `CancelBySession` 批量取消匹配的任务

#### 测试用例清单

```
TestPool_GetTasksBySession
TestPool_GetTasksBySession_Empty
TestPool_CancelBySession
TestPool_CancelBySession_NoTasks
TestPool_GetTask
TestPool_GetTask_NotFound
TestPool_Dispatch_WithSessionID
```

---

### Task 26: 目录级搜索 + 默认工具集

**来源**: CodeCast 差距 G9 + G10
**目标**: 新增目录递归搜索，提供 DefaultToolkit

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 修改 | `internal/tools/builtin/filesystem.go` | 新增 searchDirectory |
| 新增 | `internal/tools/toolkit.go` | 默认工具集配置 |
| 新增 | `internal/tools/toolkit_test.go` | 测试 |

#### 实现步骤

1. **searchDirectory 实现**
   - 参数：path（目录）、query（搜索内容）、include（文件模式）、max_results
   - 使用 `filepath.WalkDir` 递归遍历
   - 按 include 模式过滤（`filepath.Match`）
   - 在每个文件中搜索 query，返回匹配行
   - 限制结果数量

2. **DefaultToolkit 实现**
   - `ToolkitConfig{RootDir, EnableFS, EnableShell, EnableWeb, EnableSearch, ScopePolicy, FileLock}`
   - `DefaultToolkit(cfg)` 创建包含所有内置工具的 Registry
   - `MinimalToolkit(rootDir)` 创建仅 FS + Shell 的 Registry
   - 自动注入 ScopePolicy 和 FileLock 到各工具

#### 测试用例清单

```
TestFileSystem_SearchDirectory_Basic
TestFileSystem_SearchDirectory_WithInclude
TestFileSystem_SearchDirectory_NoMatch
TestFileSystem_SearchDirectory_DeepNested
TestDefaultToolkit_AllTools
TestDefaultToolkit_WithScopePolicy
TestDefaultToolkit_WithFileLock
TestMinimalToolkit
```

---

### Task 27: 编排模式导出 + 嵌套组合

**来源**: 架构问题 A5 + A6
**目标**: 导出编排 API，支持 Pipeline 条件步骤

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 修改 | `internal/agent/orchestration.go` | Pipeline 条件步骤 |
| 新增 | `pkg/orchestration.go` | 编排公共 API |
| 修改 | `internal/agent/orchestration_test.go` | 补充测试 |

#### 实现步骤

1. **Pipeline 条件步骤**
   - `PipelineStep` 新增 `Condition func(ctx, prevResult) bool`
   - 执行时检查 Condition，返回 false 则跳过

2. **公共 API 导出**
   - `pkg/orchestration.go` 导出 Pipeline/Handoff/MessageBus 等类型
   - 导出构造函数

#### 测试用例清单

```
TestPipeline_ConditionalStep_Skip
TestPipeline_ConditionalStep_Execute
TestPipeline_NestedPipeline
TestPipeline_WithTimeout
TestHandoff_WithLLMRouter
TestOrchestration_ExportedTypes
```

---

## 子阶段 2-C: 新能力扩展（P2）

### Task 28: 分布式 Agent 通信（HTTP 传输层）

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 新增 | `internal/agent/transport.go` | Transport 接口 |
| 新增 | `internal/agent/http_transport.go` | HTTP 实现 |
| 新增 | `internal/agent/http_transport_test.go` | 测试 |

#### 实现步骤

1. **Transport 接口**: Send / Receive / Start / Close
2. **HTTPTransport**: 基于 `net/http` 的实现
   - POST `/api/message` 接收远程消息
   - JSON 序列化 BusMessage
   - 连接池 + 超时控制

#### 测试用例清单

```
TestHTTPTransport_SendAndReceive
TestHTTPTransport_StartAndClose
TestHTTPTransport_ConcurrentMessages
TestHTTPTransport_Timeout
TestHTTPTransport_LargeMessage
```

---

### Task 29: 性能基准测试套件

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 新增 | `internal/agent/bench_test.go` | Agent 基准 |
| 新增 | `internal/pool/bench_test.go` | Pool 基准 |
| 新增 | `internal/memory/bench_test.go` | Memory 基准 |
| 新增 | `internal/tools/bench_test.go` | Tools 基准 |

#### 基准清单

```
BenchmarkReActAgent_SimpleCompletion
BenchmarkReActAgent_SingleToolCall
BenchmarkReActAgent_MaxTurns
BenchmarkPool_Dispatch_10Agents
BenchmarkPool_Dispatch_100Agents
BenchmarkMemory_Add
BenchmarkMemory_Search
BenchmarkMemory_FTS5Search
BenchmarkTools_Filesystem_Read
BenchmarkTools_Filesystem_Write
BenchmarkTools_Shell_Execute
```

---

### Task 30: 更多 LLM Provider

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 新增 | `internal/llm/cohere_provider.go` | Cohere 实现 |
| 新增 | `internal/llm/mistral_provider.go` | Mistral 实现 |
| 新增 | `internal/llm/cohere_test.go` | 测试 |
| 新增 | `internal/llm/mistral_test.go` | 测试 |

#### 测试用例清单

```
TestCohereProvider_Complete_Success
TestCohereProvider_Complete_Error
TestCohereProvider_Stream_Basic
TestCohereProvider_CallTools_Success
TestMistralProvider_Complete_Success
TestMistralProvider_Complete_Error
TestMistralProvider_Stream_Basic
TestMistralProvider_CallTools_Success
```

---

### Task 31: Web UI 管理面板

#### 文件清单

| 操作 | 文件路径 | 说明 |
|------|---------|------|
| 新增 | `cmd/admin/main.go` | 管理面板入口 |
| 新增 | `internal/admin/handler.go` | HTTP handler |
| 新增 | `internal/admin/handler_test.go` | 测试 |

#### API 端点

```
GET /api/agents         — 列出所有 Agent 状态
GET /api/agents/:id     — 查看 Agent 详情
GET /api/memory         — 查看记忆列表
GET /api/memory/search  — 搜索记忆
GET /api/metrics        — Prometheus 格式指标
GET /                   — 管理面板首页
```

#### 测试用例清单

```
TestAdminHandler_ListAgents
TestAdminHandler_GetAgent
TestAdminHandler_ListMemory
TestAdminHandler_SearchMemory
TestAdminHandler_Metrics
TestAdminHandler_Index
```

---

## 执行顺序

```
子阶段 2-A (P0): CodeCast 对齐 + 架构修复
  Task 20: 统一 Agent 接口        ← 架构基础，后续 Task 依赖
  Task 18: Scope/FileLock 注入    ← 依赖 Task 20 的 Agent 接口
  Task 19: System Prompt 模板     ← 独立
  Task 21: 工具安全增强           ← 独立
  Task 22: Memory 异步摘要        ← 独立

子阶段 2-B (P1): 架构统一 + 编排增强
  Task 23: 统一消息总线           ← 独立
  Task 24: Run/StreamRun 去重     ← 独立
  Task 25: Session 分组管理       ← 依赖 Task 20 的 Pool 重构
  Task 26: 目录级搜索 + 工具集    ← 依赖 Task 18 的 Scope 注入
  Task 27: 编排模式导出           ← 依赖 Task 23 + Task 24

子阶段 2-C (P2): 新能力扩展
  Task 28: 分布式通信             ← 依赖 Task 23 的 MessageBus
  Task 29: 性能基准               ← 独立
  Task 30: 更多 Provider          ← 独立
  Task 31: Web UI                 ← 独立

每完成一个 Task:
  1. go test -v ./对应模块/...   (确认新测试全过)
  2. go test ./...               (全量回归)
  3. 更新设计文档状态
```

## 验收标准

- [ ] 子阶段 2-A: CodeCast 可基于 AP 运行完整工作流
- [ ] 子阶段 2-B: 公共 API 完整，编排能力增强
- [ ] 子阶段 2-C: 框架具备分布式和可观测性能力
- [ ] 全量测试 `go test ./...` 零失败（目标 ~300 tests）
- [ ] 性能基准测试无退化
- [ ] 零新增外部依赖（除已有的 modernc.org/sqlite）

---

*Plan Version: 1.0 | Created: 2026-05-29*
