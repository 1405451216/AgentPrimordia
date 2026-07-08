# Changelog

本文件记录 AgentPrimordia 框架的所有重要变更。格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/)。

## [v0.8.0] - 2026-07-07

### Added

- **MCP Go Server** (`internal/mcp/http_server.go`): 标准 MCP over HTTP 协议（tools/list, tools/call, initialize）
- **MCP TypeScript Client** (`sdk/typescript/src/mcp/client.ts`): 零依赖双传输（HTTP/SSE + stdio）JSON-RPC 客户端，7 tests
- **A2A 工具租赁** (`internal/agent/a2a/tool_lease.go`): 配额管理 + 过期回收，优先级抢占，15 tests
- **Lessee 客户端** (`internal/agent/a2a/lessee.go`): 本地租约全生命周期管理
- **零依赖可视化编辑器** (`sdk/typescript/src/react/visual-editor.tsx`): Pipeline/Handoff/DAG/GroupChat/Debate 五种模式
- **pgvector 独立模块** (`pgvector/store.go`): 向量 CRUD + KNN + HNSW/IVFFlat + JSONB，5 tests
- **K8s LLM 智能扩缩容** (`operator/autoscaler/llm_autoscaler.go`): 队列深度/延迟/Token 速率三维度调度，9 tests
- **Go WASM Edge Gateway** (`gateway/gateway.go`): KV 会话亲和，零 CGO，Cloudflare Workers/Vercel Edge 部署就绪，9 tests
- **WASM 运行时** (`wasm/runtime.go`): wazero 沙箱，模块编译缓存 + 资源限制，5 tests

### 依赖变更

- `github.com/jackc/pgx/v5` — pgvector 模块（无法用 stdlib 复现）
- `github.com/tetratelabs/wazero` — wasm 模块（纯 Go WASM 运行时）

## [Unreleased]

### Added

- **子包测试迁移**：将 `workflow_test.go` 和 `hooks_test.go` 从父包迁移到各自子包，新增 `core_test.go`、`tokencache_edge_test.go`、`gateway_test.go`，共 98 个新测试
- **子包覆盖率提升**：workflow/ 0%→73.9%，hooks/ 0%→82.1%，core/ 0%→100%，tokencache/ 55%→96.6%，gateway/ 0%→60.9%

### Changed

- **Phase 3 包拆分**：提取 workflow/、hooks/、core/、dag/、cost/、bufferpool/、tokencache/、zerocopy/、hitl/、context/ 子包，父包通过类型别名保持向后兼容
- **Deprecated 标注完善**：所有 `// Deprecated:` 注释补充 `// Removed in v2.0.` 标记

## [1.0.0] - 2026-06-30

### Added

- **全局版本统一 v1.0.0** — Go SDK (`pkg.Version`)、TypeScript SDK (`package.json`)、CLI (`ap version`)、脚手架模板全部对齐为 `v1.0.0`
- **API 稳定性承诺锁定** — Stable API 向后兼容，破坏性变更需大版本（v2.0）
- **API 参考文档全面重写** — `docs/api/` 下 7 个文件（agent / llm / tools / memory / pool / a2a / guardrail）对照源码逐行校验，修正接口签名、导入路径、类型定义
- **Go vs TypeScript 基准对比** (`docs/benchmarks/go-vs-typescript.md`): 双 SDK 性能基准报告

### Added (Go 性能优化 — perf-v11)

- **RAG RRF 融合算法** (`internal/memory/rag.go`): 新增 `HybridFusionMode`（Linear / RRF）和 `RAGFusionConfig`，
  支持 Reciprocal Rank Fusion 混合检索。RRF 基于排名而非原始分数融合，对量纲差异鲁棒。
  `NewRAGStoreWithFusionConfig()` 和 `RAGStore.SetFusionConfig()` 支持运行时切换融合模式。
- **BufferPool** (`internal/agent/bufferpool.go`): `sync.Pool` 复用 `bytes.Buffer`，
  减少 LLM 请求体构造和 SSE chunk 解析热路径上的内存分配。大 buffer（>4KB）归还时自动截断。
- **TokenCache** (`internal/agent/tokencache.go`): FNV-1a hash + `sync.Map` 的 token 估算缓存，
  面向长文档 chunk 和重复消息场景。当前保留供未来启用（`len()/4` 启发式已足够快）。
- **JSON Buffer Pool** (`internal/jsonutil/pool.go`): JSON 序列化/反序列化的 buffer 复用池。
- **pprof 端点** (`internal/health/pprof.go`): `ap.RegisterPProf(mux)` 和 `ap.PProfHandler()`
  导出至 `pkg/`，支持所有标准 profile 类型（heap / goroutine / cpu / block / mutex）。
- **`ap loop` CLI 子命令** (`cmd/ap/loop.go`): ReAct Loop 工程化工具
  - `ap loop trace` — 查看 Agent 执行追踪
  - `ap loop inspect` — 查看 Agent 当前状态
  - `ap loop resume` — 从检查点恢复运行
- **Fuzz 测试**: Sandbox 路径遍历（`sandbox_fuzz_test.go`）、RAG 检索（`rag_fuzz_test.go`）、
  工具执行器（`executor_fuzz_test.go`）安全模糊测试
- **供应链安全文档** (`docs/advanced/supply-chain-security.md`): govulncheck + npm audit + Trivy + cosign 签名 + SBOM 生成完整指南
- **PGO 性能调优文档** (`docs/advanced/pgo.md`): Profile-Guided Optimization 使用指南
- **Go vs TypeScript 基准对比** (`docs/benchmarks/go-vs-typescript.md`): 双 SDK 性能基准报告

### Added (TypeScript SDK — 100% Go Parity)

- **TypeScript SDK 基础设施补全 (Phase 24)**: 5 个模块实现 Go `internal/` 全覆盖
  - `audit/logger.ts` — 审计日志（`AuditLogger`, `InMemoryAuditOutput`, 合规报告生成）
  - `admin/handler.ts` — Bearer Token 认证管理 HTTP API + Web UI Dashboard
  - `debugger/server.ts` — Inspector（span/session trace）+ DebugServer（事件/快照）双 HTTP 服务
  - `persist/sqlite-checkpoint.ts` — SQLite 检查点存储（双接口：`CheckpointStore` + Go 兼容 `AgentState`）
  - `health/http.ts` — `/healthz`、`/readyz`、`/livez` Kubernetes 风格健康端点
- **TypeScript SDK Bug 修复 (Phase 11-23)**:
  - `ConcurrencyPool.release()` 竞态条件：改为直接交接模式，避免超额进入
  - `WorkerPool.drain()` 泄漏：增加 `running` 状态检查，drain 后停止派发
  - `StepExecutor` 耗时统计错误：修正 start/end 时间戳
  - `extractPattern` 非字符串崩溃：增加 `String()` 强制转换
  - `ZeroCopyPool` 不安全类型断言：移除 `as` 绕过 `readonly`，实现安全复用
- **TypeScript SDK 文档更新**:
  - `README.md` — 完整 24 Phase 模块清单 + Go 对等表 + 基础设施使用示例
  - `docs/api/index.md` — 全量 API 参考文档（含 Phase 24 基础设施端点）
  - `docs/index.md` — VitePress 首页更新为 9 大特性卡片

### Changed (Breaking)

- **`audit.NewLogger` 签名变更**: `func NewLogger(cfg LoggerConfig) *Logger` → `func NewLogger(cfg LoggerConfig) (*Logger, error)`
  - 原 `panic("audit: LoggerConfig.Output 不能为 nil")` 改为返回 `ErrOutputRequired`
  - 符合生产规范（构造器不应 panic），调用方需处理 error
  - 内部调用者已同步更新

### Changed

- **`Must*` 系列函数增加日志与文档警告** (v0.8.0 生产加固):
  - `agent.DAGBuilder.MustBuild()` — panic 前增加 `slog.Error` 日志
  - `memory.MustEpisode()` — panic 前增加 `slog.Error` 日志
  - `prompt.Registry.MustRegister()` — panic 前增加 `slog.Error` 日志
  - `prompt.Template.MustRender()` — panic 前增加 `slog.Error` 日志
  - 文档统一标注「生产建议：使用对应的 error 版本」
- **`pkg/agent.go` 版本号修正**: 从 `0.8.0` 修正为 `1.0.0`，与 README / Release Notes / 迁移文档一致
- **Dockerfile 基础镜像升级**: `golang:1.23-alpine` → `golang:1.26-alpine`，与 `go.mod` 声明的 `go 1.26` 对齐
- **`.gitignore` 补全**: 新增 `bin/` 和各类覆盖率产物（`llm_cover`、`pkg_cov`、`pkg_cover` 等）的忽略规则

### Fixed

- **Dockerfile 构建失败**: 原 `golang:1.23-alpine` 无法构建 `go 1.26` 项目，升级到 `1.26-alpine` 修复
- **版本号不一致**: `pkg/version.go` 中 `3.0.0` 与文档（v0.8.0）严重不一致，已修正
- **误提交的覆盖率文件**: `llm_cover`、`pkg_cov`、`pkg_cover` 已从仓库移除并加入 `.gitignore`

### Added

- **GitHub Issue Triage Bot** (Phase 18): `ecosystem/examples/github-issue-triage/`
  生产级 demo，展示 AgentPrimordia 在真实业务场景下的能力
  - 5 个预置 Issue，涵盖 bug/feature/question/duplicate 4 种分类
  - 3 个自定义工具（list_issues / read_issue / add_label）
  - httptest 模拟 GitHub API（生产可换成真实 API）
  - 支持 OpenAI / Qwen / DeepSeek / MockLLM 4 种模式
  - 无 API Key 时用 mock 模式自动跑通完整演示
- **Phase 18 实施计划**: `docs/plans/2026-06-12-issue-triage-bot.md`
- **公共 API 补全**: `pkg/llm.go` 新增 `QwenProvider / GLMProvider / DeepSeekProvider` 类型别名
  和 `NewQwenProvider / NewGLMProvider` 构造器，弥补 Phase 15 补遗中未实现的文档承诺
- **README 亮点 Demo 板块**: 在「快速开始」与「架构」之间插入 3 个 demo 展示
  （GitHub Issue Triage Bot / 链式 API 30 秒上手 / Pool 多 Agent 调度），
  配套 2 张 SVG 架构图（手写、可在 GitHub 直接渲染）
  - `docs/images/issue-triage-architecture.svg`
  - `docs/images/multi-agent-dispatch.svg`

- **Qwen Provider 工具调用与流式测试** (Phase 16-A): `qwen_provider_test.go` 新增 6 个集成测试
  - `TestQwenProvider_CallTools_Success / MultipleTools / NoToolCall`
  - `TestQwenProvider_Stream_Basic / ContextCancel / APIError`
- **GLM Provider 流式测试与行为锁定** (Phase 16-B): `glm_provider_test.go` 新增 4 个集成测试
  - `TestGLMProvider_CallTools_NotSupported` 锁定 `ErrNotSupported` 当前行为
  - `TestGLMProvider_Stream_Basic / ContextCancel / APIError`
- **DeepSeek Provider 集成测试** (Phase 16-C): 新建 `deepseek_provider_test.go`
  - 7 个测试覆盖 `deepseek-chat` / `deepseek-reasoner` / `deepseek-coder` 三种模型
  - 验证 OpenAI 兼容接口（`BaseURL=https://api.deepseek.com/v1`）的 Complete / CallTools / Stream 路径
- **Phase 16 实施计划** (Phase 16): `docs/plans/2026-06-12-llm-provider-tests.md`
- **FAQ 文档** (Phase 15 补遗): `agentprimordia/ecosystem/docs/faq.md` — 7 大分类 22 个问答
- **RAG Agent Cookbook** (Phase 15 补遗): `agentprimordia/ecosystem/docs/cookbook/rag-agent.md` — 含架构图、完整代码、三种 RAG 模式对比
- **`pkg/example_test.go`**: 8 个 Go Example 函数（NewAgent / Pool / DAG / Session / ResilientProvider 等）
  覆盖公共 API，可在 `go doc` 和 pkg.go.dev 上展示

### Fixed

- **文档 API 路径不一致** (Phase 15 补遗): `getting-started.md` 和 `best-practices.md` 大量使用 `internal/` 旧路径，
  统一改为 `ap.Xxx` 公共 API；删除不存在的 API 引用（`memory.NewMemory`, `debugger.NewDebugServer` 等）
- **`pkg/version.go` 版本号** (Phase 15 补遗): 从 `0.1.0` 修正为 `0.7.0`，与 README/CHANGELOG 一致
- **异步摘要结果丢失** (P2 M2): 扩展 `MemoryStore` 接口添加 `UpdateSummary` 方法，
  ReAct 异步摘要 goroutine 现在将结果写入记忆存储而非仅日志记录
- **Pool Task Map 无界增长** (P2 M8): 新增 `MaxRetainedTasks` 配置（默认 0=禁用），
  Dispatch 后自动清理已完成的终端任务，防止长期运行内存泄漏
- **编排循环缺少 ctx.Done() 检查** (P2 M7): `orchestrator.go` 的 executeSequential/Parallel/DAG 循环、
  重试循环及 `collaboration.go` 的 executeReview/executeBrainstorm 入口均已添加 ctx.Done() 检查，
  上下文取消时在 ~100ms 内返回
- **Metrics label 维度缺失** (P2 M13): 新增 `LabeledMetricsRecorder` 可选接口，
  react_loop 通过类型断言自动分发带标签指标（provider/model/tool_name/agent_name），
  Prometheus 输出现在包含三维标签，Dashboard PromQL 可正确聚合

### Testing

- **Anthropic 真实 API 集成测试** (Phase 17-A): `TestIntegration_Anthropic_Complete/Stream/CallTools`
  使用 `claude-haiku-4-5-20251001` 降低测试成本
- **GLM 真实 API 集成测试** (Phase 17-B): `TestIntegration_GLM_Complete/Stream`（CallTools 跳过，Phase 16-B 已锁定 `ErrNotSupported`）
- **Qwen/DeepSeek Stream 集成测试** (Phase 17-C): `TestIntegration_Qwen_Stream`, `TestIntegration_DeepSeek_Stream`
- **`pkg/` 公共 API 端到端测试** (Phase 17-D): `pkg/integration_test.go` — 4 个 e2e 测试
  验证 `ap.NewAgent / NewSession / WithMemory / StreamRun` 的真实跑通路径
- **跨平台集成测试脚本** (Phase 17-E): `scripts/test-integration.ps1`
  自动检测 API Key 并报告跳过情况，支持 Provider 过滤（`-Provider openai` 等）
- **异步摘要存储测试** (P2): `summary_store_test.go` — 验证 `UpdateSummary` 被正确调用
- **Pool 自动清理测试** (P2): `dispatcher_cleanup_test.go` — 验证 `MaxRetainedTasks` 阈值清理语义
- **编排 ctx.Done() 取消测试** (P2): `ctx_cancel_test.go` — 4 个测试覆盖顺序/并行/DAG 取消 + 无取消基线
- **Metrics label 维度测试** (P2): `metrics_labels_test.go` — 4 个测试验证 provider/model/tool_name/agent_name 标签输出

### Changed

- **README.md 文档链接** (Phase 15 补遗): 新增 FAQ / CLI 手册 / RAG Cookbook / 迁移指南的链接

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
