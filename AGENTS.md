# AgentPrimordia 工作区规则

> 此规则优先级最高，不可跳过，不可以"任务简单"为由省略

## 1. 项目定位

AgentPrimordia 是从 CodeCast 生产验证的 Agent 架构中提炼出的 **通用 Go Agent 开发框架**。
- 不是 CodeCast 应用本身
- 核心价值：ReActLoop、AgentPool、Tool System、Memory Store、LLM Abstraction
- 目标用户：任何想用 Go 构建 AI Agent 应用的开发者

## 2. 技术栈约束

- **语言**: Go 1.26+（`go.mod` 已声明 `go 1.26`）
- **外部依赖**: 仅允许 `modernc.org/sqlite`（纯 Go SQLite 驱动，无 CGO）与 `gopkg.in/yaml.v3`
- **所有新模块使用 Go 标准库**（net/http, database/sql, os/exec 等）
- **不引入**：任何 Web 框架、ORM、配置库等第三方包

## 3. 代码规范

- **TDD 强制**: 所有功能必须先写测试（Red → Green → Refactor）
- **接口优先**: LLM、Tools、Memory、Pool 全部通过接口解耦
- **并发安全**: 共享状态必须用 sync.RWMutex / sync.Mutex / channel 保护
- **错误处理**: 使用 `pkg/errors.go` 中定义的错误变量
- **中文注释**: 代码注释使用中文
- **代码风格**: 与现有代码保持一致（参考 internal/agent/、internal/pool/）

## 4. 模块边界

> **2026-06 更新**: 随着 Phase 6 及后续阶段演进，项目模块已从最初的 6 个核心包扩展为更大的 monorepo 结构。
> 当前 `agent/` 仍是 ReAct 循环与协议式微内核的顶层入口，实际依赖关系以本节为准。
> 历史 Phase 实施记录见 [`docs/CHANGELOG.md`](docs/CHANGELOG.md)。

### 4.1 当前模块结构

```
internal/
├── admin/          — Admin HTTP API（调试/管理接口）
├── agent/          — ReActLoop 引擎 + 协议式微内核（顶层）
│   ├── a2a/        — Agent2Agent 协议实现（JSON-RPC / SSE / 任务管理）
│   ├── planning/   — 任务规划器
│   ├── reflection/ — Agent 自反思能力
│   └── tool_learning/ — 工具学习/自动发现
├── concurrency/    — 文件锁等并发原语
├── config/         — 配置热加载
├── debugger/       — 调试器 / Inspector / 可视化编辑器
├── events/         — 内部事件总线
├── guardrail/      — 输入/输出护栏（注入检测、PII、主题过滤等）
├── llm/            — LLM 抽象层与多家 Provider 实现
├── memory/         — 记忆存储（SQLite / InMemory / RAG / Vector）
├── metrics/        — Prometheus 指标收集
├── orchestration/  — 编排模式（Pipeline / Handoff / DAG / GroupChat / Debate）
├── otel/           — OpenTelemetry 桥接与导出
├── persist/        — 状态持久化与 Checkpoint
├── pool/           — 多 Agent 调度与会话管理
├── prompt/         — 提示词模板与少样本管理
├── security/       — ACL / Sandbox / 路径校验
└── tools/          — 工具系统（注册表、执行器、MCP、内置工具）
    └── builtin/    — filesystem / shell / web / api / database / code_execution

pkg/                — 公共 API 导出（类型别名 + 少量辅助构造器）
ecosystem/          — 示例、插件、模板、生态文档
operator/           — Kubernetes Operator（AgentDeployment CRD）
pgvector/           — pgvector 向量存储扩展
```

### 4.2 依赖方向规则

```
        ┌────────────────────────────────────────┐
        │           agent/  (顶层)               │
        │   引用 llm, memory, persist, tools    │
        └────┬───────┬───────┬───────────┬──────┘
             │       │       │           │
        ┌────▼─┐ ┌───▼──┐ ┌──▼───┐ ┌────▼────┐
        │ llm  │ │memory│ │persist│ │  tools  │
        └──────┘ └──────┘ └───────┘ └────┬────┘
                                          │
                                     ┌────▼────┐
                                     │  pool   │
                                     └─────────┘

        orchestration/、debugger/、metrics/、otel/、guardrail/、
        security/、events/、config/、prompt/ 等模块为横向支撑层，
        可消费 agent/llm/memory/tools/pool 等下层能力。
```

- **`agent/` 处于依赖顶层**：可引用 `llm/memory/persist/tools/pool/orchestration/security/metrics/otel/events/config/prompt/concurrency` 等下层/横向模块。
- **下层模块禁止反向引用上层**：`llm/memory/persist/tools` 不得 import `agent/`、`pool/`、`orchestration/`。
- **编排/调试/可观测等横向模块**：可引用 `agent/` 及以下模块，但不得被 `llm/memory/persist/tools` 反向引用。
- **`pkg/` 以类型导出和 re-export 为主**：允许少量不可或缺的公共错误/辅助构造器（如 `pkg/errors.go` 中的 `CodeError`），新增业务逻辑应优先放在 `internal/`。
- **`ecosystem/` 与 `internal/` 解耦**：`ecosystem/plugins/*`、`ecosystem/examples/*`、`ecosystem/templates/*` 应仅通过 `pkg/` 公共 API 与核心交互，不直接 import `internal/*`。
  - **当前状态**：部分示例与插件仍直接依赖 `internal/*`，属于已知的技术债务，需逐步迁移至公共 API。
- **`operator/`、`pgvector/`**：独立模块，与 `internal/` 通过 `pkg/` 或 CRD 解耦。

## 5. 测试要求

- 每个新功能必须有对应测试
- 使用 `t.TempDir()` 创建临时文件，不污染项目
- Shell/Web 工具测试用 `httptest.Server` 或模拟，不需要真实网络
- Memory 测试用 `WithInMemory()` 创建内存数据库
- MockLLM 用于 agent/pool 层测试；DemoLLM 用于示例应用

## 6. 提交粒度

- 每个 Task 完成后应可独立编译和通过测试
- 提交信息格式: `feat: xxx` / `fix: xxx` / `refactor: xxx`
- 不要在一个提交中混合多个 Task 的改动

## 7. 文档同步

- 用户文档: `agentprimordia/docs/` 下各模块文档
- 代码变更后如影响设计文档，需同步更新

## 8. CodeCast 参考代码

AgentPrimordia 最初从 CodeCast 生产架构中提炼核心模式，相关参考实现已融入当前代码库：

- **Pool + FileLock + Scope 校验** → `internal/pool/`、`internal/concurrency/`、`internal/tools/scope.go`
- **ReAct Loop + OpenAI HTTP 调用** → `internal/agent/react_loop.go`、`internal/llm/openai_provider.go`
- **工具分发 + FilesScope 权限** → `internal/tools/executor.go`、`internal/tools/builtin/filesystem.go`
- **增强 Memory (topics/importance/cleanup)** → `internal/memory/`、特别是 `sqlite.go` / `summarizer.go`

> 注：原 `CodeCast-desktop/` 参考目录已不再随仓库分发，上述对应关系用于追溯设计来源。

**提取模式而非复制代码**：理解 CodeCast 设计意图后，在 AP 中用更通用、接口化的方式重新实现。
