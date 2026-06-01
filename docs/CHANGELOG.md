# Changelog

本文件记录 AgentPrimordia 框架的所有重要变更。格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/)。

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
