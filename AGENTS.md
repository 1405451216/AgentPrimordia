# AgentPrimordia 工作区规则

> 此规则优先级最高，不可跳过，不可以"任务简单"为由省略

## 1. 项目定位

AgentPrimordia 是从 CodeCast 生产验证的 Agent 架构中提炼出的 **通用 Go Agent 开发框架**。
- 不是 CodeCast 应用本身
- 核心价值：ReActLoop、AgentPool、Tool System、Memory Store、LLM Abstraction
- 目标用户：任何想用 Go 构建 AI Agent 应用的开发者

## 2. 技术栈约束

- **语言**: Go 1.22+
- **外部依赖**: 仅允许 `modernc.org/sqlite`（纯 Go SQLite 驱动，无 CGO）
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

> **2026-06 更新**: Phase 6 起 `agent/` 实际处于依赖顶层（依赖 llm/memory/persist/tools），
> 旧的"不依赖 pool/memory"描述已不准。详见 `docs/plans/2026-06-04-phase6-implementation.md` §模块边界更新。

```
internal/
├── agent/      — ReActLoop 引擎 + 协议式微内核（顶层，依赖 llm/memory/persist/tools）
├── pool/       — 多 Agent 调度（依赖 agent, tools）
├── tools/      — 工具系统（独立模块，被 agent/pool 依赖）
├── memory/     — 记忆存储（独立模块，被 agent 依赖）
├── llm/        — LLM 抽象层（最底层，被 agent 依赖）
└── persist/    — 状态持久化（独立模块，被 agent 依赖）
```

实际依赖图：

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
```

- `internal/*` 之间：`agent/` 处于顶层，可引用下层；下层（llm/memory/persist/tools）不能反向引用 `agent/`
- `pkg/` 只做类型导出和 re-export，不含业务逻辑
- `ecosystem/` 与 `internal/` 互不依赖：`ecosystem/plugins/*` 等通过 `tools.Plugin` 协议与核心解耦

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

`CodeCast-desktop/` 目录下的 4 个文件是架构参考：
- `agent.go` — Pool + FileLock + Scope 校验的实现参考
- `agent_engine.go` — ReAct Loop + OpenAI HTTP 调用的实现参考
- `agent_tools.go` — 工具分发 + FilesScope 权限的实现参考
- `memory.go` — 增强 Memory (topics/importance/cleanup) 的实现参考

**提取模式而非复制代码**：理解设计意图后在 AP 中用更通用的方式重新实现。
