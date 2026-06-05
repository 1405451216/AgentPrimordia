# Changelog

本文件记录 AgentPrimordia 框架的所有重要变更。格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/)。

## [0.7.0] - 2026-06-05

### Added

- **公共 API 稳定性策略** (Phase 6.5.1, 7.1): pkg/ 顶部 4 级 `// Stability:` 标注
  (Stable / Experimental / Deprecated / Internal); 详见 `docs/specs/2026-06-04-semver-policy.md`
- **SemVer 策略 spec** (Phase 7.1): `docs/specs/2026-06-04-semver-policy.md`
  定义 v0.x → v1.0 → v2.0 升级窗口、CHANGELOG 规范、godoc 标注模板
- **协议式微内核** (Phase 6.5.4): 12 个 `*Capable` 接口 + `WithXxx` 链式 API;
  取代 `ReActConfig` 中 14 个能力字段
- **LLM Provider 模板** (Phase 6.5.3): `internal/llm/provider_template.go`
  启动期拒绝构造,防误用
- **`ap init` 脚手架可编译** (Phase 6.5.5, 6.5.9): 生成 go.mod + replace 指向 `..`
- **`TemplateProvider` 误用防护** (Phase 6.5.3): `ErrTemplateNotImplemented` sentinel 错误
- **生态 README** (Phase 6.5.8): `ecosystem/README.md` 显式核心/生态边界
- **CONTRIBUTING 模块边界同步** (Phase 6.5.6): 把 AGENTS.md 关键规则纳入仓库
- **SqliteStore**: TypeScript SDK SQLite 持久化层 (better-sqlite3)
- **CI 安全扫描**: govulncheck + npm audit + Trivy
- **CI 多平台测试**: ubuntu/macos/windows 矩阵
- **CI API 兼容性检查**: apidiff 对 PR 检查公共 API 变更
- **Docker 安全加固**: 非root用户 + HEALTHCHECK
- **Release 签名**: cosign 签名 + syft SBOM 生成
- **Operator Service**: `{name}-metrics` Service 暴露 metrics sidecar
- **Operator HPA**: HorizontalPodAutoscaler + `concurrent_tasks_per_pod` Pods 指标
- **Operator Pod 指标**: 从真实 Pod 状态聚合 ActiveReplicas/CompletedTasks/FailedTasks
- **TypeScript 编排**: Pipeline（条件步骤）+ ParallelRun + Handoff
- **TypeScript A2A**: A2ABus 跨 Agent 消息通信
- **TypeScript MCP**: MCPClient 占位 + 完整类型定义
- **E2E 集成测试**: ReActAgent 真实 API 调用测试 (//go:build integration)
- **架构文档重写**: architecture-mermaid.md 从 CodeCast 改为 AP 架构（6 张图）
- **CHANGELOG 回填**: v0.3.0 - v0.6.0 完整条目
- **TypeScript 上下文窗口管理**: `maxMessages` 配置 + `trimMessages()` 保留 system prompt
- **TypeScript AgentPool 并发安全**: 索引分派替代 queue.shift()
- **TypeScript tsconfig 对齐**: module/moduleResolution 改为 Node16

### Changed

- **ReActConfig 14 个能力字段**: 标 `// Deprecated:` + `// Removed in v2.0.`,
  4 阶段废弃时间表 (v0.7 → v1.0 panic → v2.0 移除)
- **pkg/agent.go ReActConfig**: export 级 Stability 标注 + 迁移指南指针
- **pkg/llm.go**: 多模态 / LLM 缓存 / MCP / Plugin 区加 export 级 Experimental 标注
- **pkg/tools.go**: MCP 客户端 / 插件加 export 级 Experimental 标注
- **Operator ConfigMap**: yaml.Marshal 替代 fmt.Sprintf 安全生成 YAML
- **Operator 镜像**: imageOrDefault() 方法，spec.Image > DefaultImage > 硬编码默认值
- **Makefile**: `make test` 自动检测 CGO，Windows 无 gcc 也能跑
- **统一 License**: Apache-2.0 (Copyright 2026 AgentPrimordia Contributors)

### Deprecated

- `ReActConfig.Memory / Toolkit / RAG / Hooks / Tracer / Cache` 等 14 个字段
  迁移到 `NewReActAgent(...).WithXxx()` 链式 API
  详见 `ecosystem/docs/migration/v0-deprecations.md`

### Fixed

- `ap init` 生成的项目缺 `go.mod`, 此前无法编译 (Phase 6.5.5)
- `ap init` 生成的 `go.mod` replace 路径错 (`../agentprimordia` → `..`) (Phase 6.5.9)
- `cmd/ap/scaffold/main.go` 孤儿文件被 `//go:embed` 包含 (Phase 6.5.9)
- 6 个生态插件补单元测试: git / http / json / sql (Phase 6.5.4)
- **CRITICAL**: filesystem.go EvalSymlinks 失败静默放行 symlink 逃逸
- **CRITICAL**: react_loop.go searchRAG nil 解引用 panic
- **CRITICAL**: resilient.go HalfOpen 熔断器逻辑反转
- **CRITICAL**: License 不一致 (MIT vs Apache-2.0)
- StreamRun goroutine 错误丢失
- json.Marshal 错误静默忽略
- Operator 缺失 Finalizer
- Operator 镜像硬编码
- ConfigMap YAML 不安全生成 (fmt.Sprintf 注入风险)
- TypeScript SSE 流读取无超时
- TypeScript ReAct 连续工具失败无退出机制
- CLI config.go YAML 解析坏了
- CI YAML 语法错误（未引号冒号）
- Windows `go test -race` 失败（CI 跳过 + Makefile 自动检测）

## [0.6.0] - 2026-06-03

### Added

- **Pre-commit hook**: 格式化 + lint 强制（Phase 8.4）
- **Agent 模板系统**: 3 个新 agent 模板 + template-lock.json（Phase 8.2）
- **Operator CRD 增强**: metrics/tracing 字段 + DeepCopy 方法（Phase 8.3）
- **Tier 3 软门**: 内存/调试/编排等核心包 ≥50% warning（Phase 8.1）

### Changed

- 覆盖率网关从单一门槛改为 Tier 1/2/3 阶梯式（Phase 7.2 → 8.1）

### Fixed

- Debugger + memory 模块测试覆盖率提升（Phase 8.1）

## [0.5.0] - 2026-06-02

### Added

- **Phase 7: SemVer 策略**: `docs/specs/2026-06-04-semver-policy.md`
- **export 级稳定性标注**: pkg/ 顶部 4 级 Stability 标注（Phase 7.1）
- **阶梯覆盖率网关**: Tier 1 ≥80%, Tier 2 ≥65%, Tier 3 ≥50%（Phase 7.2）
- **go.work**: 多模块工作空间 + examples README（Phase 7.3）
- **Phase 6.5 治理后记**: 文档对齐（Phase 7.4）

## [0.4.0] - 2026-06-01

### Added

- **协议式微内核**: 12 个 `*Capable` 接口 + `WithXxx` 链式 API（Phase 6.5.4）
- **LLM Provider 模板**: 启动期拒绝构造防误用（Phase 6.5.3）
- **ap init 可编译**: 生成 go.mod + replace 指向 `..`（Phase 6.5.5, 6.5.9）
- **生态 README**: 显式核心/生态边界（Phase 6.5.8）
- **CONTRIBUTING 模块边界同步**: AGENTS.md 关键规则纳入仓库（Phase 6.5.6）

### Changed

- **ReActConfig 14 个能力字段**: 标 `// Deprecated:` + `// Removed in v2.0.`（Phase 6.5.1）
- **pkg/agent.go ReActConfig**: export 级 Stability 标注 + 迁移指南指针
- **pkg/llm.go**: 多模态/LLM 缓存/MCP/Plugin 区加 Experimental 标注
- **pkg/tools.go**: MCP 客户端/插件加 Experimental 标注

### Fixed

- `ap init` 生成的项目缺 go.mod，此前无法编译（Phase 6.5.5）
- `ap init` 生成的 go.mod replace 路径错（Phase 6.5.9）
- `cmd/ap/scaffold/main.go` 孤儿文件被 `//go:embed` 包含（Phase 6.5.9）

### Deprecated

- `ReActConfig.Memory / Toolkit / RAG / Hooks / Tracer / Cache` 等 14 个字段
  迁移到 `NewReActAgent(...).WithXxx()` 链式 API

## [0.3.0] - 2026-05-30

### Added

- **微内核架构**: 能力接口 `*Capable` + 链式 API `WithXxx()`（Phase 6）
- **PluginLoader**: 插件化工具系统，动态加载第三方工具（Phase 6）
- **Provider 模板**: 生态贡献模板（Phase 6）
- **WorkflowExecution 引擎**: 导出至 pkg/（Phase 6 prerequisite）
- **SummaryEngine / SummaryStrategy / WindowSummaryStrategy**: 导出至 pkg/agent.go
- **CostTracker / ModelPricing**: 成本追踪导出至 pkg
- **ContentPart / ContentType**: 多模态内容常量导出

### Changed

- docs/examples 迁移至 ecosystem/ 目录（Phase 6）
- 代码质量改进: agent, llm, memory, orchestration, tools 模块重构

### Removed

- CodeCast-desktop 目录（已独立为 CodeCast 项目）

## [0.2.0] - 2026-05-29

### Added

- **统一消息总线**: `LocalMessageBus` 合并 A2ABus + AgentBus，支持 handler 回调和 channel 订阅双模式
- **编排 Hooks**: Pipeline/Handoff/ParallelRun 支持 before/after 钩子
- **Pipeline 条件步骤**: `PipelineStep.Condition` 支持条件跳过
- **Session 分组管理**: `TaskConfig.SessionID` + `Pool.GetTasksBySession`/`CancelBySession`
- **目录级搜索**: FileSystem 新增 `search_directory` 递归搜索
- **默认工具集**: `DefaultToolkit`/`MinimalToolkit` 快速配置
- **HTTP 传输层**: `HTTPTransport` 跨进程 Agent 通信
- **Agent 发现协议**: `LocalDiscovery` + `HTTPDiscovery` + `DiscoveryServer`
- **DAG 工作流引擎**: `DAGWorkflow` 支持条件边、循环检测、并行执行
- **Memory 运维增强**: `RecordToolUse`/`ClearAll`/`ExportMemories`/`ImportMemories`
- **Cohere Provider**: Cohere v2 API 支持
- **Mistral Provider**: Mistral AI (OpenAI 兼容) 支持
- **Web UI 管理面板**: `AdminHandler` REST API + 内嵌 HTML
- **性能基准测试**: Agent/Pool/Memory/Tools 四模块 benchmark
- **Run/StreamRun 去重**: `reactLoopEngine` 统一引擎
- **System Prompt 模板引擎**: `PromptTemplate` 支持变量替换和 Scope 规则注入
- **Scope/FileLock 自动注入**: Executor 和 FileSystem 自动集成权限检查和文件锁
- **工具安全增强**: edit_file 唯一匹配、文件大小限制、命令输出截断、FTS5 查询清洗
- **Memory 异步摘要**: `Summarizer` + `ExtractSummaryAsync` + `StartAutoCleanup`
- **统一 Agent 接口**: `Agent` 接口 + `AgentFactory` 工厂模式
- **编排模式导出**: `pkg/orchestration.go` 导出 Pipeline/Handoff/MessageBus 等公共 API

### Changed

- A2ABus/AgentBus 内部委托给 `LocalMessageBus` (向后兼容)
- 所有 `interface{}` 替换为 `any` (Go 1.18+ 惯用法)
- 4 个 LLM Provider 添加 `scanner.Err()` 检查
- AutoCleanup 添加 nil db 保护

### Fixed

- Memory AutoCleanup 在 `store.Close()` 后 panic 的问题

### Deprecated

- A2ABus (请使用 `LocalMessageBus`)
- AgentBus (请使用 `LocalMessageBus`)

## [0.1.0] - 2026-05-29

### Added

- **ReActLoop 引擎**: 思考-行动-观察循环，支持 hooks + lifecycle
- **AgentPool 调度**: 信号量并发控制 + EventBus
- **内置工具集**: FileSystem / Shell / Web / Knowledge
- **Memory Store**: SQLite FTS5 全文搜索 + RAG + 向量存储
- **OpenAI Compatible HTTP Provider**: 兼容 OpenAI / DeepSeek / Moonshot / GLM / Ollama
- **Anthropic Provider**: Claude 系列模型支持
- **Azure Provider**: Azure OpenAI 支持
- **Gemini Provider**: Google Gemini 支持
- **Ollama Provider**: 本地 Ollama 支持
- **Resilient Provider**: 指数退避重试 + Fallback 链 + 三态熔断器
- **FileLock Manager**: 文件级并发写锁
- **Scope Policy 权限系统**: Agent 文件操作权限控制
- **Enhanced Memory Store**: 标签/重要性评分/时间线/自动清理
- **Context Window Manager**: 自动上下文窗口管理
- **Metrics 可观测性**: Prometheus 格式指标输出
- **Checkpoint 持久化**: Agent 执行状态保存和恢复
- **安全沙箱**: 命令白名单 + 路径限制
- **事件总线**: Channel-based pub/sub
- **A2A 协作**: Agent-to-Agent 通信
- **编排模式**: Pipeline / Handoff / Parallel / Stream
- **MCP 协议**: Model Context Protocol 支持
- **示例应用**: hello-agent / multi-agent / production / with-tools
- **TypeScript SDK**: 完整的 TS SDK + 类型定义
