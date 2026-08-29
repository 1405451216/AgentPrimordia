# AgentPrimordia v0.2.0 — Phase 2 Release Notes

> **发布日期**: 2026-05-29
> **版本**: 0.2.0
> **阶段**: Phase 2 — 均衡推进（架构统一 + 编排增强 + 新能力扩展）
> **状态**: 全部交付项已完成，全量测试通过

---

## 概述

AgentPrimordia v0.2.0 是框架的第二个主要版本。本版本在 v0.1.0 的基础上完成了三大目标：

1. **CodeCast 对齐 + 架构修复**（子阶段 2-A）：补齐了 Scope/FileLock 自动注入、System Prompt 模板、Agent 接口统一、工具安全增强、Memory 异步摘要等生产必需能力
2. **架构统一 + 编排增强**（子阶段 2-B）：统一消息总线、Run/StreamRun 去重、Session 分组管理、默认工具集、编排模式导出
3. **新能力扩展**（子阶段 2-C）：分布式 Agent 通信、DAG 工作流、Agent 发现协议、更多 LLM Provider、Web UI 管理面板、性能基准测试

**核心价值**：v0.2.0 使框架从"可用"升级为"好用"——统一了架构模式、增强了编排能力、扩展了分布式通信和运维能力，同时保持了零新增外部依赖的承诺。

---

## 交付统计

| 指标 | v0.1.0 | v0.2.0 | 增量 |
|------|:------:|:------:|:----:|
| Go 源文件 | 98 个 | ~130 个 | +32 |
| 代码总行数 | 17,661 行 | ~22,000 行 | +4,300 |
| 测试用例总数 | ~195 个 | ~300+ 个 | +105+ |
| 外部依赖 | 1 个 | 1 个 | 0 |
| Go 版本 | 1.26+ | 1.26+ | — |

---

## 新增功能

### 🔴 P0 — 生产必需（子阶段 2-A）

#### Task 18: Scope/FileLock 自动注入工具流

`FileScopePolicy` 和 `FileLockManager` 自动集成到 `Executor` 和 `FileSystem`，无需手动调用。

- `internal/tools/executor.go` — Executor 增加 ScopePolicy 和 FileLockManager 字段
- `internal/tools/builtin/filesystem.go` — write/edit 操作自动检查 Scope 和获取 FileLock
- `internal/tools/builtin/shell.go` — 命令执行前检查 Scope

```go
executor := tools.NewExecutor(registry).
    WithScopePolicy(scopePolicy).
    WithFileLock(fileLockManager)

// 执行工具时自动检查权限和获取文件锁
result, err := executor.Execute(ctx, &tools.FunctionCall{
    Name: "write_file",
    Arguments: `{"path": "/data/output.txt", "content": "hello"}`,
})
```

**测试**: 9 个用例，覆盖权限允许/拒绝/空策略、文件锁获取/释放等场景

---

#### Task 19: System Prompt 模板引擎

支持 `{{.Variable}}` 格式的模板变量替换，自动注入 Scope 规则到 System Prompt。

- `internal/agent/prompt.go` — 模板引擎实现

```go
// 使用内置默认模板
tmpl := agent.DefaultSystemPrompt().
    WithVar("AgentName", "CodeAssistant").
    WithScopeRules([]string{"/data/project", "/tmp/workspace"}).
    WithVar("TaskDescription", "重构代码并添加测试")

prompt, err := tmpl.Render()
// 输出包含 Agent 名称、文件权限规则、任务描述

// 自定义模板
tmpl = agent.NewPromptTemplate("你是 {{.Role}}，擅长 {{.Skill}}").
    WithVar("Role", "代码审查员").
    WithVar("Skill", "Go 语言")
```

**测试**: 9 个用例

---

#### Task 20: 统一 Agent 接口 + Pool 配置修复

定义 `Agent` 接口，引入 `AgentFactory` 工厂模式，修复 Pool 到 Agent 的配置传递断层。

- `internal/agent/types.go` — Agent 接口定义
- `internal/pool/types.go` — AgentFactory + AgentFactoryConfig

```go
// Agent 接口
type Agent interface {
    Run(ctx context.Context, input Message) (*Response, error)
    StreamRun(ctx context.Context, input Message) (<-chan StreamEvent, error)
    Stop()
    Stats() AgentStats
    Name() string
}

// AgentFactory 工厂模式
factory := func(cfg pool.AgentFactoryConfig) agent.Agent {
    return agent.NewReActAgent(cfg.Name, provider, toolkit)
}

p := pool.NewPool(pool.PoolConfig{MaxConcurrency: 5})
p.SetAgentFactory(factory)
```

**测试**: 10 个用例

---

#### Task 21: 工具安全增强

补齐工具安全防护，防止意外修改和资源耗尽。

- `edit_file` 唯一匹配：多处匹配时拒绝替换，要求提供更多上下文
- 文件大小限制：读取默认 4MB，写入默认 10MB
- 命令输出截断：超过 50,000 字符自动截断
- Web 内容截断：超过 50,000 字符自动截断
- FTS5 查询清洗：移除特殊字符和关键字，防止语法错误

```go
// edit_file 唯一匹配 — 多处匹配时返回错误
// old_string found 3 times in file, expected exactly 1 occurrence.
// Please provide more context to make the match unique.

// 文件大小限制
fs, _ := builtin.NewFileSystem(rootDir)
// readFile: 文件超过 4MB 返回错误
// writeFile: 内容超过 10MB 返回错误
```

**测试**: 11 个用例

---

#### Task 22: Memory 异步摘要 + 自动清理调度

自动生成记忆摘要和标签，自动调度过期清理。

- `internal/memory/summarizer.go` — 摘要提取器
- `internal/memory/sqlite.go` — StartAutoCleanup + ExtractSummaryAsync

```go
// 摘要提取器
summarizer := memory.NewSummarizer(provider).WithModel("gpt-4o-mini")
result, err := summarizer.ExtractSummary(ctx, content)
// result.Summary = "用户讨论了数据库优化方案"
// result.Topics = "数据库,性能优化,索引"

// 自动清理
stop := store.StartAutoCleanup(memory.CleanupConfig{
    MaxAgeDays:    30,
    Interval:      24 * time.Hour,
    PreserveRoles: []string{"tool"},
})
defer stop()

// 异步摘要提取
errCh := store.ExtractSummaryAsync(ctx, episodeID, summarizer)
```

**测试**: 10 个用例

---

### 🟡 P1 — 架构统一 + 编排增强（子阶段 2-B）

#### Task 23: 统一消息总线

合并 A2ABus + AgentBus 为单一 `MessageBus` 接口，支持 handler 回调和 channel 订阅双模式。

- `internal/agent/bus.go` — MessageBus 接口 + LocalMessageBus 实现

```go
bus := agent.NewLocalMessageBus()

// 方式一：handler 回调
bus.Register("agent-1", func(ctx context.Context, msg *agent.BusMessage) (*agent.BusMessage, error) {
    fmt.Printf("收到消息: %s\n", msg.Content)
    return &agent.BusMessage{Content: "已处理"}, nil
})

// 方式二：channel 订阅
ch := bus.Subscribe("agent-1")
go func() {
    for msg := range ch {
        fmt.Printf("通道收到: %s\n", msg.Content)
    }
}()

// 发送和广播
bus.Send(ctx, &agent.BusMessage{From: "agent-2", To: "agent-1", Content: "hello"})
bus.Broadcast(ctx, &agent.BusMessage{From: "agent-2", Content: "announcement"})
```

**迁移**: A2ABus 和 AgentBus 标记为 deprecated，内部委托给 LocalMessageBus，向后兼容

**测试**: 10 个用例

---

#### Task 24: Run/StreamRun 去重 + 编排 Hooks

抽象共享循环引擎 `reactLoopEngine`，为编排模式增加 before/after 钩子。

```go
// 编排 Hooks
hooks := agent.NewHookManager()
hooks.Register(agent.HookBeforePipelineStep, func(ctx context.Context, hctx *agent.HookContext) error {
    log.Printf("Pipeline 步骤开始: %s", hctx.AgentID)
    return nil
})
hooks.Register(agent.HookAfterPipelineStep, func(ctx context.Context, hctx *agent.HookContext) error {
    log.Printf("Pipeline 步骤完成: %s", hctx.AgentID)
    return nil
})

pipeline := agent.NewPipeline(steps...)
pipeline.SetHooks(hooks)
```

**新增 HookPoint**: `HookBeforePipelineStep` / `HookAfterPipelineStep` / `HookBeforeHandoff` / `HookAfterHandoff` / `HookBeforeParallelAgent` / `HookAfterParallelAgent` / `HookBeforeDAGNode` / `HookAfterDAGNode`

**测试**: 7 个用例

---

#### Task 25: Session 分组管理 + 按会话取消

Pool 支持按 Session 分组查询和取消任务。

- `internal/pool/types.go` — TaskConfig 增加 SessionID
- `internal/pool/dispatcher.go` — Session 级操作方法

```go
// 带会话 ID 的任务分发
p.Dispatch(pool.TaskConfig{
    ID:        "task-1",
    Prompt:    "分析代码",
    SessionID: "session-abc",
})

// 按会话查询
tasks := p.GetTasksBySession("session-abc")

// 按会话取消
p.CancelBySession("session-abc")
```

**测试**: 7 个用例

---

#### Task 26: 目录级搜索 + 默认工具集

新增目录递归搜索，提供开箱即用的工具配置。

- `internal/tools/builtin/filesystem.go` — searchDirectory
- `internal/tools/builtin/toolkit.go` — DefaultToolkit / MinimalToolkit

```go
// 默认工具集（包含 FS + Shell + Web）
registry, err := builtin.DefaultToolkit(builtin.ToolkitConfig{
    RootDir:     "/project",
    EnableFS:    true,
    EnableShell: true,
    EnableWeb:   true,
    ScopePolicy: scopePolicy,
    ScopeAgent:  "agent-1",
    FileLock:    fileLockMgr,
})

// 最小工具集（仅 FS + Shell）
registry, err := builtin.MinimalToolkit("/project")
```

**测试**: 8 个用例

---

#### Task 27: 编排模式导出 + Pipeline 条件步骤

导出编排 API，支持 Pipeline 条件分支。

- `internal/agent/orchestration.go` — PipelineStep.Condition
- `pkg/orchestration.go` — 编排公共 API

```go
// Pipeline 条件步骤
pipeline := agent.NewPipeline(
    agent.PipelineStep{
        Name:  "分析",
        Agent: analyzer,
        Input: "分析这段代码",
    },
    agent.PipelineStep{
        Name:      "修复",
        Agent:     fixer,
        Input:     "修复问题",
        Condition: func(ctx context.Context, prev *agent.StepResult) bool {
            return prev != nil && prev.Output != "无需修复"
        },
    },
)
```

**测试**: 5 个用例

---

### 🟢 P2 — 新能力扩展（子阶段 2-C）

#### Task 28: HTTP 传输层

基于 HTTP 的跨进程 Agent 通信传输层。

- `internal/agent/transport.go` — Transport 接口
- `internal/agent/http_transport.go` — HTTPTransport 实现

```go
// 启动传输服务
transport := agent.NewHTTPTransport()
transport.Start("localhost:0") // 随机端口
defer transport.Close()

addr := transport.Addr()

// 发送消息到远程 Agent
err := transport.Send(ctx, addr, &agent.BusMessage{
    From:    "agent-1",
    To:      "agent-2",
    Type:    agent.BusMsgTaskRequest,
    Content: "请处理这个任务",
})

// 接收远程消息
for msg := range transport.Receive() {
    fmt.Printf("收到: %s\n", msg.Content)
}
```

**测试**: 5 个用例

---

#### Agent 发现协议

支持进程内和跨进程的 Agent 发现与注册。

- `internal/agent/discovery.go` — Discovery 接口 + LocalDiscovery + HTTPDiscovery + DiscoveryServer

```go
// 进程内发现
local := agent.NewLocalDiscovery()
local.Register(ctx, &agent.AgentInfo{
    ID:           "agent-1",
    Name:         "worker",
    Address:      "localhost:8080",
    Capabilities: []string{"search", "compute"},
})

info, _ := local.Discover(ctx, "agent-1")

// 跨进程发现（HTTP）
server := agent.NewDiscoveryServer(local)
server.Start("localhost:9090")

remote := agent.NewHTTPDiscovery("http://localhost:9090")
remote.Register(ctx, &agent.AgentInfo{ID: "remote-agent", Name: "remote"})
agents, _ := remote.ListAgents(ctx)
```

**测试**: 覆盖注册/发现/心跳/注销等场景

---

#### DAG 工作流引擎

支持条件边、循环检测、并行执行的 DAG 工作流。

- `internal/agent/dag.go` — DAGWorkflow 实现

```go
dag := agent.NewDAGWorkflow()

// 添加节点
dag.AddNode(&agent.DAGNode{ID: "fetch", Agent: fetcher, Input: "获取数据"})
dag.AddNode(&agent.DAGNode{ID: "analyze", Agent: analyzer, Input: "分析数据"})
dag.AddNode(&agent.DAGNode{ID: "report", Agent: reporter, Input: "生成报告"})

// 添加边（含条件）
dag.AddEdge(agent.DAGEdge{From: "fetch", To: "analyze"})
dag.AddEdge(agent.DAGEdge{
    From: "analyze",
    To:   "report",
    Condition: func(ctx context.Context, result *agent.DAGNodeResult) bool {
        return result.Output != "无需报告"
    },
})

// 验证 DAG（检测循环）
if err := dag.Validate(); err != nil {
    log.Fatal(err)
}

// 执行工作流
result, err := dag.Run(ctx, "开始处理")
```

**测试**: 覆盖基本执行、条件跳过、循环检测、并行执行等场景

---

#### Task 29: 性能基准测试套件

Agent/Pool/Memory/Tools 四模块 benchmark，防止性能退化。

```bash
# 运行基准测试
go test -bench=. -benchmem ./internal/agent/...
go test -bench=. -benchmem ./internal/pool/...
go test -bench=. -benchmem ./internal/memory/...
go test -bench=. -benchmem ./internal/tools/...
```

**基准项**: `BenchmarkReActAgent_SimpleCompletion` / `BenchmarkPool_Dispatch_10Agents` / `BenchmarkMemory_Add` / `BenchmarkMemory_FTS5Search` / `BenchmarkTools_Filesystem_Read` 等 11 项

---

#### Task 30: 更多 LLM Provider

新增 Cohere 和 Mistral 两个 LLM Provider。

- `internal/llm/cohere_provider.go` — Cohere v2 API
- `internal/llm/mistral_provider.go` — Mistral AI (OpenAI 兼容)

```go
// Cohere Provider
cohere, _ := llm.NewCohereProvider(llm.Config{
    APIKey: "cohere-api-key",
    Model:  "command-r-plus",
})

// Mistral Provider
mistral, _ := llm.NewMistralProvider(llm.Config{
    APIKey: "mistral-api-key",
    Model:  "mistral-large-latest",
})
```

**测试**: 8 个用例

---

#### Task 31: Web UI 管理面板

提供 Web UI 查看 Agent 状态、任务列表和统计信息。

- `internal/admin/handler.go` — AdminHandler REST API + 内嵌 HTML
- `cmd/admin/main.go` — 管理面板入口

```go
// 启动管理面板
handler := admin.NewAdminHandler(pool)
http.ListenAndServe(":8080", handler)

// API 端点
// GET /api/agents     — 列出所有 Agent 状态
// GET /api/agents/{id} — 查看 Agent 详情
// GET /api/stats      — 池统计信息
// GET /api/tasks      — 任务列表
// GET /               — 管理面板首页（自动刷新）
```

**测试**: 6 个用例

---

## 变更说明

### 向后兼容变更

| 变更 | 影响 | 迁移方式 |
|------|------|---------|
| A2ABus/AgentBus 委托给 LocalMessageBus | 旧代码无需修改，仍可正常使用 | 建议新代码直接使用 `LocalMessageBus` |
| `interface{}` → `any` | Go 1.18+ 惯用法，无功能影响 | 无需操作 |
| LLM Provider 添加 `scanner.Err()` 检查 | 更严格的流式错误处理 | 无需操作 |
| AutoCleanup nil db 保护 | 防止 Close 后 panic | 无需操作 |

### Bug 修复

- **Memory AutoCleanup panic**: 修复 `store.Close()` 后 AutoCleanup goroutine 访问 nil db 导致 panic 的问题。现在 `autoCleanup` 方法在执行前检查 db 是否为 nil

---

## 废弃通知

| 废弃 API | 替代方案 | 移除计划 |
|----------|---------|---------|
| `A2ABus` | `LocalMessageBus` | v0.3.0 |
| `AgentBus` | `LocalMessageBus` | v0.3.0 |

`A2ABus` 和 `AgentBus` 在 v0.2.0 中仍可正常使用，内部已委托给 `LocalMessageBus`。建议新代码直接使用 `LocalMessageBus`。

---

## 从 v0.1.0 迁移指南

### 1. 使用 Agent 接口替代具体类型

```go
// v0.1.0 — 直接使用 ReActAgent
agent := agent.NewReActAgent("worker", provider, toolkit)

// v0.2.0 — 面向 Agent 接口编程
var a agent.Agent = agent.NewReActAgent("worker", provider, toolkit)
```

### 2. 使用 AgentFactory 配置 Pool

```go
// v0.1.0 — 使用 SetModel/SetToolkit
p := pool.NewPool(cfg)
p.SetModel(provider)
p.SetToolkit(registry)

// v0.2.0 — 使用 AgentFactory（推荐）
factory := func(cfg pool.AgentFactoryConfig) agent.Agent {
    return agent.NewReActAgent(cfg.Name, provider, toolkit)
}
p := pool.NewPool(pool.PoolConfig{MaxConcurrency: 5})
p.SetAgentFactory(factory)
```

### 3. 使用 LocalMessageBus 替代 A2ABus

```go
// v0.1.0
bus := agent.NewA2ABus()
bus.Register("agent-1", handler)

// v0.2.0
bus := agent.NewLocalMessageBus()
bus.Register("agent-1", handler)
```

### 4. 使用 DefaultToolkit 快速配置工具

```go
// v0.1.0 — 手动注册每个工具
reg := tools.NewRegistry()
fs, _ := builtin.NewFileSystem(rootDir)
reg.Register(fs)
shell := builtin.NewShell()
reg.Register(shell)

// v0.2.0 — 一行配置
reg, _ := builtin.DefaultToolkit(builtin.ToolkitConfig{
    RootDir: rootDir,
    EnableFS: true,
    EnableShell: true,
    ScopePolicy: scopePolicy,
    FileLock: fileLockMgr,
})
```

### 5. 使用 PromptTemplate 自动注入 Scope 规则

```go
// v0.1.0 — 手动拼接 System Prompt
systemPrompt := "你是 AI 助手。\n你只能操作以下文件：/data/project"

// v0.2.0 — 模板引擎自动注入
tmpl := agent.DefaultSystemPrompt().
    WithVar("AgentName", "Assistant").
    WithScopeRules([]string{"/data/project"})
prompt, _ := tmpl.Render()
```

---

## 已知限制

1. **TCP 传输层未实现**: 当前仅提供 HTTP 传输层，TCP 传输层待后续版本补充
2. **LLM Provider 覆盖率**: llm 模块单元测试覆盖率仍偏低，真实 API 调用需手动验证
3. **SummaryStrategy 未实现**: Context Window 仅实现 DefaultStrategy，LLM 摘要策略待后续版本
4. **DiscoveryServer 无认证**: Agent 发现协议的 HTTP 服务端未内置认证机制，生产环境需配合反向代理使用
5. **Admin 面板功能有限**: 当前仅提供状态查看，暂不支持通过 Web UI 下发任务

---

## 下一阶段规划（Phase 3 P2）

- [ ] TCP 传输层实现
- [ ] SummaryStrategy（LLM 摘要式上下文裁剪）
- [ ] DiscoveryServer 认证机制
- [ ] Admin 面板任务下发能力
- [ ] 分布式 Agent 端到端集成验证
- [ ] 更多 LLM Provider（Groq、Together AI 等）
- [ ] 插件化工具系统（动态加载第三方工具）
- [ ] Agent 评估框架（自动评估 Agent 输出质量）

---

*AgentPrimordia v0.2.0 — The Primordial Agent Framework for Go*
