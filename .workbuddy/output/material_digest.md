# AgentPrimordia 架构设计 · 资料摘要

> 本文档做一件事：**精读主理人转交的全部原始资料，逐份、逐章节做出摘要**——后面任何人拿到这份摘要，都能通过章节号快速定位回原始文件的对应位置。

> 上游输入：主理人转交的全部原始资料（Markdown 文档，含项目规则、框架总览、代码知识库、版本发布说明、计划与评估文档）；
> 产出者：`knowledge-ingest-engineer`（知识摄入工程师 - 闻资料），经 G1 校验与人工审核通过后交付。

---

## 0. 元信息

```yaml
标题: AgentPrimordia - 资料摘要 v1.0
版本: v1.0
状态: Draft（待 G1 自动校验与人工审核）
创建日期: 2026-07-07
整理人: knowledge-ingest-engineer（闻资料）
审核人:
  - team-lead（主理人）

原始资料清单:
  - E:/codecast/AgentPrimordia/AGENTS.md: 项目规则与架构约束权威来源
  - E:/codecast/AgentPrimordia/README.md: 框架总览、特性、快速开始、架构示意
  - E:/codecast/AgentPrimordia/CODE_WIKI.md: 代码知识库（模块详解、接口、依赖图）
  - E:/codecast/AgentPrimordia/docs/2026-07-02-fix-optimize-evolve.md: 当前状态/技术债务/风险登记
  - E:/codecast/AgentPrimordia/docs/CHANGELOG.md: 版本变更日志
  - E:/codecast/AgentPrimordia/docs/RELEASE-NOTES-v0.1.0.md: Phase 1-D 发布说明
  - E:/codecast/AgentPrimordia/docs/RELEASE-NOTES-v0.2.0.md: Phase 2 发布说明
  - E:/codecast/AgentPrimordia/docs/RELEASE-NOTES-v0.7.0.md: 安全加固 + Operator 发布说明
  - E:/codecast/AgentPrimordia/docs/RELEASE-NOTES-v0.8.0.md: 开发者体验重构发布说明
  - E:/codecast/AgentPrimordia/docs/RELEASE-NOTES-v1.0.0.md: 正式发布说明
  - E:/codecast/AgentPrimordia/docs/TypeScript_vs_Go_Deep_Evaluation.md: Go/TS 深度对比评估
  - E:/codecast/AgentPrimordia/docs/api-reference.md: 公共 API 参考
  - E:/codecast/AgentPrimordia/docs/architecture-mermaid.md: Mermaid 架构图
  - E:/codecast/AgentPrimordia/docs/multi-agent-collaboration-prompt.md: 多智能体协同系统提示词
  - E:/codecast/AgentPrimordia/docs/concepts/interface-graph.md: 跨包接口关系图
  - E:/codecast/AgentPrimordia/docs/plans/2026-06-26-pre-commit-cleanup.md: pre-commit 清理计划
  - E:/codecast/AgentPrimordia/docs/plans/grpc-migration.md: A2A gRPC 迁移计划
  - E:/codecast/AgentPrimordia/docs/plans/perf-v5-comprehensive-audit.md: 性能优化审计计划
  - E:/codecast/AgentPrimordia/docs/architecture-drawio.drawio: 架构图示（drawio 源文件，未逐章摘要）
  - E:/codecast/AgentPrimordia/docs/images/issue-triage-architecture.svg: 架构图示（未逐章摘要）
  - E:/codecast/AgentPrimordia/docs/images/multi-agent-dispatch.svg: 架构图示（未逐章摘要）
```

| 版本 | 日期 | 作者 | 变更内容 |
| --- | --- | --- | --- |
| v1.0 | 2026-07-07 | knowledge-ingest-engineer | 初稿（Phase 1 资料摄入，G1 待校验） |

---

## 1. 资料清单

> 列出全部原始资料，每份标注解析状态。解析失败或跳过的必须注明原因。

| 编号 | 文件名 | 类型 | 来源 | 解析状态 | 说明 |
| --- | --- | --- | --- | --- | --- |
| D1 | AGENTS.md | md | 项目根 | 已解析 | 架构约束权威来源 |
| D2 | README.md | md | 项目根 | 已解析 | 框架总览、特性、架构示意 |
| D3 | CODE_WIKI.md | md | 项目根 | 已解析 | 代码知识库（约 68KB，按模块粒度摘要） |
| D4 | docs/2026-07-02-fix-optimize-evolve.md | md | docs/ | 已解析 | 当前状态 / 技术债务 / 风险登记 |
| D5 | docs/CHANGELOG.md | md | docs/ | 已解析 | 版本变更日志（v0.1.0 → v1.0.0） |
| D6 | docs/RELEASE-NOTES-v0.1.0.md | md | docs/ | 已解析 | Phase 1-D 发布说明 |
| D7 | docs/RELEASE-NOTES-v0.2.0.md | md | docs/ | 已解析 | Phase 2 发布说明 |
| D8 | docs/RELEASE-NOTES-v0.7.0.md | md | docs/ | 已解析 | 安全加固 + K8s Operator 发布说明 |
| D9 | docs/RELEASE-NOTES-v0.8.0.md | md | docs/ | 已解析 | 开发者体验重构发布说明 |
| D10 | docs/RELEASE-NOTES-v1.0.0.md | md | docs/ | 已解析 | 正式发布说明 |
| D11 | docs/TypeScript_vs_Go_Deep_Evaluation.md | md | docs/ | 已解析 | Go/TS 深度对比评估（约 67KB，按章节摘要） |
| D12 | docs/api-reference.md | md | docs/ | 已解析 | 公共 API 参考（约 28KB） |
| D13 | docs/architecture-mermaid.md | md | docs/ | 已解析 | Mermaid 架构图（7 张） |
| D14 | docs/multi-agent-collaboration-prompt.md | md | docs/ | 已解析 | 多智能体协同系统提示词（约 47KB，按章节摘要） |
| D15 | docs/concepts/interface-graph.md | md | docs/ | 已解析 | 跨包接口关系图 |
| D16 | docs/plans/2026-06-26-pre-commit-cleanup.md | md | docs/ | 已解析 | pre-commit 清理计划 |
| D17 | docs/plans/grpc-migration.md | md | docs/ | 已解析 | A2A gRPC/protobuf 迁移计划 |
| D18 | docs/plans/perf-v5-comprehensive-audit.md | md | docs/ | 已解析 | 性能优化综合审计计划 |
| — | docs/architecture-drawio.drawio | drawio | docs/ | 已登记未逐章摘要 | 架构图示源文件 |
| — | docs/images/issue-triage-architecture.svg | svg | docs/ | 已登记未逐章摘要 | 架构图示 |
| — | docs/images/multi-agent-dispatch.svg | svg | docs/ | 已登记未逐章摘要 | 架构图示 |

**类型枚举**：本文全部源资料均为 Markdown（md）；图示类为非 md（drawio / svg），按约定登记为「图示，未逐章摘要」，不计入逐章摘要范围。

---

## 2. 资料内容摘要

> 逐份文档按自身章节结构做摘要。每条摘要标注章节号（`D编号，§章节`），后面任何人想核实某个点，直接定位回原文对应位置即可。

### D1：AGENTS.md

> 项目规则与架构约束权威来源 — 来源：主理人转交（项目根目录）

| 章节 | 内容摘要 |
| --- | --- |
| D1, §1 项目定位 | AgentPrimordia 是从 CodeCast 生产验证的 Agent 架构中提炼出的**通用 Go Agent 开发框架**；不是 CodeCast 应用本身；核心价值为 ReActLoop、AgentPool、Tool System、Memory Store、LLM Abstraction；目标用户为任何想用 Go 构建 AI Agent 应用的开发者。（功能诉求） |
| D1, §2 技术栈约束 | 语言 Go 1.26+（go.mod 已声明 go 1.26）；默认原则：所有新模块仅使用 Go 标准库（net/http、database/sql、os/exec 等）；不引入任何 Web 框架、ORM、配置解析库、CLI 框架等第三方包。（技术约束） |
| D1, §2.1 已批准第三方依赖（白名单） | 因历史原因存在于 go.mod 的 3 类依赖：① `modernc.org/sqlite`（纯 Go SQLite 驱动，无 CGO，用于 internal/memory 与 ecosystem/plugins/kv 持久化）；② `gopkg.in/yaml.v3`（仅 cmd/ap 脚手架 YAML 模板渲染）；③ `google.golang.org/grpc` + `protobuf` + `genproto/googleapis/rpc`（**仅限 internal/agent/a2a/ 及其子包**，实现 Agent2Agent 协议）。（技术约束） |
| D1, §2.2 依赖扩展审批流程 | 新增白名单外依赖须满足之一：无法用标准库复现的硬性需求 / 位于 a2a/ 范围内 / 其他场景需 PR 说明理由并征得维护者同意；真实使用边界以 `go mod why -m 〈package〉` 输出为准，越界须调整或回滚。（技术约束 / 待决策项） |
| D1, §3 代码规范 | TDD 强制（Red→Green→Refactor）；接口优先（LLM/Tools/Memory/Pool 全部接口解耦）；并发安全（共享状态用 sync.RWMutex/Mutex/channel）；使用 pkg/errors.go 错误变量；中文注释；风格参考 internal/agent/、internal/pool/。（技术约束） |
| D1, §4 模块边界 | 2026-06 更新说明：项目模块已从最初 6 个核心包扩展为更大 monorepo；agent/ 仍是 ReAct 循环与协议式微内核顶层入口；历史演进见 docs/CHANGELOG.md。（现状） |
| D1, §4.1 当前模块结构 | internal/ 含 admin / agent（a2a、planning、reflection、tool_learning 子包）/ concurrency / config / debugger / events / guardrail / llm / memory / metrics / orchestration / otel / persist / pool / prompt / security / tools（builtin/）；另含 pkg/（公共 API）、ecosystem/、operator/（K8s Operator）、pgvector/。（现状 / 技术约束） |
| D1, §4.2 依赖方向规则 | agent/ 处于依赖顶层，可引用 llm/memory/persist/tools/pool/orchestration/security/metrics/otel/events/config/prompt/concurrency；下层模块（llm/memory/persist/tools）禁止反向引用 agent/、pool/、orchestration/；横向模块可引用 agent/ 及以下但不得被下层反向引用；pkg/ 以类型导出与 re-export 为主；ecosystem/ 与 internal/ 解耦（已知技术债务：部分示例/插件仍直接依赖 internal/）；operator/、pgvector/ 为独立模块。（技术约束） |
| D1, §5 测试要求 | 每功能须有对应测试；用 t.TempDir()；Shell/Web 工具测试用 httptest.Server 或模拟；Memory 测试用 WithInMemory()；MockLLM 用于 agent/pool 层测试，DemoLLM 用于示例。（技术约束） |
| D1, §6 提交粒度 | 每 Task 完成后应可独立编译与通过测试；提交信息格式 feat:/fix:/refactor:；不在一个提交中混合多个 Task 改动。（技术约束） |
| D1, §7 文档同步 | 用户文档位于 agentprimordia/docs/ 下各模块文档；代码变更影响设计文档须同步更新。（现状 / 注：实际仓库 docs 在 AgentPrimordia/docs/ 与 AgentPrimordia/agentprimordia/docs/ 两处均存在，见 §3 冲突 X6） |
| D1, §8 CodeCast 参考代码 | 提炼映射：Pool+FileLock+Scope → internal/pool/、internal/concurrency/、internal/tools/scope.go；ReAct Loop+OpenAI HTTP → internal/agent/react_loop.go、internal/llm/openai_provider.go；工具分发+FilesScope → internal/tools/executor.go、internal/tools/builtin/filesystem.go；增强 Memory（topics/importance/cleanup）→ internal/memory/（sqlite.go、summarizer.go）。原则：提取模式而非复制代码。（现状 / 设计来源） |

### D2：README.md

> 框架总览、核心特性、快速开始、架构示意 — 来源：主理人转交（项目根目录）

| 章节 | 内容摘要 |
| --- | --- |
| D2, §特性 | ReAct Loop 引擎（20+ 生命周期钩子）；多模式编排 Pipeline/Handoff/Parallel/DAG/GroupChat/A2A；工具系统 FileSystem/Shell/Web/Knowledge 内置 + MCP + 插件；三层记忆 SQLite FTS5 + Vector + RAG（RRF 融合）；10+ LLM Provider + Resilient（重试/降级/熔断）；Pool 并发调度（信号量/会话隔离/重试）；安全防护 ACL/Sandbox/Guardrails/PII/路径遍历 + symlink 逃逸防护；可观测 Prometheus/OTel/Grafana；K8s Operator（AgentDeployment CRD）；TypeScript SDK 100% Go 功能对等（24 模块）；CLI ap init/run/debug/loop/test/mcp/plugin/doctor/completion；最小外部依赖（仅 modernc.org/sqlite + gopkg.in/yaml.v3，无 CGO）。（功能诉求 / 特性） |
| D2, §v1.0.0 Highlights | 开发者体验重构（ap.NewAgent 3 行创建带记忆/RAG/Hook）；WithRAGMemory 一步 RAG；ap loop trace/inspect/resume；RAG RRF 融合（Linear/RRF 运行时切换）；性能优化 BufferPool/TokenCache/JSON Pool/pprof；供应链安全 govulncheck+npm audit+Trivy+cosign+SBOM；PGO 调优；Fuzz 测试；testutil；向后兼容；版本统一 v1.0.0。（现状 / 新增能力） |
| D2, §TypeScript SDK | sdk/typescript/ 与 Go 100% 功能对等，24 模块映射表；npm install @agentprimordia/sdk；HealthServer 暴露 /healthz、/readyz、/livez。（功能特性） |
| D2, §快速开始 | go build ./cmd/ap 编译 CLI；ap init/run 3 步创建 Agent；Hello Agent 5 行代码；含工具 Agent；多 Agent Pool（MaxConcurrency 5）；DAG 编排；MCP Server 集成；Resilient Provider（主+fallback 降级）。（使用说明） |
| D2, §亮点 Demo | GitHub Issue Triage Bot（生产级，5 预置 Issue 分类+标签+报告）；链式 API 30 秒上手；Pool 多 Agent 调度（10 任务 × 5 并发，P50 0.95s、P99 1.4s）。（示例） |
| D2, §架构 | ASCII 架构图：ReActLoop/Pool/DAG/Pipeline 引擎层 + Tool System（Builtin/MCP/Plugin）+ LLM Layer（多 Provider + ResilientProvider）+ Memory/EventBus/Metrics/Guardrails 支撑层。（架构示意） |
| D2, §项目结构 | cmd/（ap/admin/example）；internal/（agent/pool/tools/memory/llm/guardrail/debugger/prompt/config/metrics/otel/events/security/persist/concurrency）；operator/（独立 go.mod，含 api/v1、controller、cmd、manifest）；testutil/；pgvector/；deploy/grafana/；bench/；docs/；sdk/typescript/；pkg/。（现状） |
| D2, §CLI 命令 | ap init/run/debug/loop(test/mcp/plugin/doctor/completion) 完整子命令清单。（使用说明） |
| D2, §Vector DB 选型 | 低于 100K 文档 → InMemory（零依赖）；100K–1M → Qdrant（Go REST 客户端）；超过 1M → Milvus（分布式）；已有 PostgreSQL → pgvector（不引入新基础设施）。（技术约束 / 数据口径） |
| D2, §可观测性 | 内置 Prometheus 指标 + OpenTelemetry 桥接；3 个预置 Grafana Dashboard（Agent Runtime / LLM Operations / Cost Tracking）。（功能特性） |
| D2, §K8s 部署 | AgentDeployment CRD 示例（apiVersion: agent.primordia.dev/v1，replicas 3，service ClusterIP:8080，autoscaling min 1 / max 10 / targetConcurrentTasks 5）；Operator 自动创建 Service 并配置 HPA 基于 Pod 指标。（现状 / 架构示意，注 CRD apiVersion 与 D5/D8 不一致，见 §3 X4） |
| D2, §运行测试 | go test ./internal/... ./pkg/... -race；go test ./cmd/ap/；make test-integration（需 OPENAI_API_KEY）；go test -bench；golangci-lint run。（技术约束） |
| D2, §设计哲学 | 来自生产服务生产 / 接口优先 / 并发原生 / 最小外部依赖 / TDD 强制。（设计原则） |
| D2, §文档 | 链接清单：CHANGELOG、各 RELEASE-NOTES、architecture-mermaid、api-reference、TS SDK 文档、Cookbook、迁移指南等。（引用） |

### D3：CODE_WIKI.md

> 代码知识库（模块详解、关键接口、依赖图）— 来源：主理人转交（项目根目录，约 68KB，22 章）

| 章节 | 内容摘要 |
| --- | --- |
| D3, §1 项目概述 | 通用 Go AI Agent 框架，从 CodeCast 生产环境提炼；核心特性表（ReAct Loop / 协议式微内核 17 个 Capable 接口 / 多 Agent 编排 / 工具系统 / 三层记忆 / LLM 抽象 10+ Provider / Resilient / Pool / 安全 / 可观测 / K8s Operator / CLI / TS SDK）；数据流（用户输入 → ReAct Loop：RAG→LLM→Tool Execute→Thought/Result→历史循环 → 响应）；设计原则（接口驱动 / 组合优于继承 / 弹性优先 / 最小外部依赖 / 协议式微内核 / 链式 API / TDD）。（功能诉求 / 设计原则） |
| D3, §2 架构总览 | ASCII 架构图：ReActLoop/Pool/DAG/Pipeline + Tool System（Builtin/MCP/Plugin）+ LLM Layer（多 Provider + ResilientProvider）+ Memory/EventBus/Metrics/Guardrails 支撑层。（架构示意） |
| D3, §3 项目结构 | 详细 internal/ 文件树：agent/（a2a、react_loop.go、types.go、new_agent.go、capability_agent.go、capabilities.go、orchestration.go、dag*.go、group_chat.go、workflow.go、hooks.go、lifecycle.go、session.go、context_window.go、prompt.go、transport.go、http_transport.go、tcp_transport.go、bus.go、tracer.go、cost_tracker.go、hitl.go、multimodal.go、eval.go、discovery.go）；pool/；tools/（types/registry/executor/scope/plugin/mcp*/builtin/）；memory/（types/memory/sqlite/vector/rag*/summarizer/conversational_memory/qdrant_provider/milvus_provider/episode）；llm/（types 与 10+ Provider/Resilient/cache*/structured/multimodal/pricing/mock/template）；guardrail/；persist/；metrics/；events/；security/；otel/；concurrency/；config/；prompt/；debugger/；orchestration/；admin/；operator/；pgvector/；pkg/（类型别名+re-export）；sdk/typescript/；ecosystem/（examples/plugins/templates/docs/contributing）；testutil/。（现状） |
| D3, §4.1 agent | 核心职责：推理引擎、多 Agent 编排、生命周期管理；Agent 接口（Run/StreamRun/Stop/Stats/Name）；核心类型（ReActAgent/ReActConfig/CapabilityAgent/Message/Response/Thought/ToolCall/StreamEvent/AgentStatus/AgentStats）；协议式微内核（12 个 *Capable 接口示例，全文第 5 章列 17 个）；ReAct 循环流程（生命周期/预算检查/RAG 检索/上下文窗口裁剪/LLM/工具执行/HITL/Checkpoint）；编排模式表（Pipeline/Handoff/Parallel/DAG/GroupChat/Workflow）；生命周期状态机（Idle→Running→[Paused|WaitingForInput|Completed|Failed|Cancelled]）；Hook 系统（20+ 钩子点，按 Validation→PreProcessing→Execution→PostProcessing 阶段）。（模块职责 / 关键接口） |
| D3, §4.2 llm | Provider 接口（Complete/Stream/CallTools/Info）+ Embedder（Embeddings）；12 家 Provider 表（OpenAI/Anthropic/Gemini/Ollama/Azure/Qwen/GLM/Mistral/Cohere/Resilient/Cached/Multimodal，DeepSeek 合并到 OpenAI 兼容）；ResilientProvider（state: closed/open/halfOpen；重试指数退避+随机抖动、熔断、降级）；StructuredExtractor（Go struct→JSON Schema）。（模块职责 / 关键接口） |
| D3, §4.3 tools | Tool 接口（Name/Description/Parameters/Execute）；组件（Registry/Executor/ScopePolicy/FileScopePolicy/MCPClient/MCPRegistry/ToolPlugin/PluginLoader）；内置工具表（FileSystem/Shell/Web/API/Database/CodeExecution/KnowledgeSearch/Toolkit）；执行流程（Registry.Get → ScopePolicy.Allow → Permission.Confirm → WithTimeout → Execute → 指标）。（模块职责 / 关键接口） |
| D3, §4.4 memory | Memory 组合接口（7 子接口：Reader/Writer/Searcher/Lifecycle/Exporter/Query/ToolUse）；实现表（InMemoryStore/SQLiteStore/VectorStore/RAGStore/QdrantProvider/MilvusProvider）；RAG Pipeline（Embedding→Vector→FTS→RRF 融合→Rerank→TopK）；RRF 融合（LinearFusion 默认 / RFFFusion，RRFK=60）；Episode 结构（ID/SessionID/Role/Content/Summary/Topics/Importance/Metadata/CreatedAt）。（模块职责 / 关键接口） |
| D3, §4.5 pool | Pool 结构（semaphore 信号量 / tasks / agents / agentFactory）；PoolConfig（MaxConcurrency/Timeout/RetryPolicy/MaxRetainedTasks/DefaultAgent）；TaskConfig（ID/Title/Prompt/SessionID/FilesScope/MaxTurns/Metadata）；调度流程（Dispatch→每 task goroutine→semaphore→createAgent→Run→RetryPolicy→释放→Stats）。（模块职责 / 关键类型） |
| D3, §4.6 persist | CheckpointStore 接口（Save/Load/List/Delete）；AgentState 结构（AgentID/SessionID/Status/Messages/TurnCount/Metrics/SavedAt）；支持断点恢复。（模块职责 / 关键接口） |
| D3, §4.7 guardrail | Engine（rules []Rule）；Rule 接口（Name/Check(input,point)→Result）；CheckPoint（input/output）；Action（pass/reject/sanitize/flag）；Severity（low/medium/high/critical）；内置规则表（InjectionRule/OutputRule/PIIRule/TopicRule/TrieRule/Sanitizer）。（模块职责 / 关键接口） |
| D3, §4.8 metrics | MetricsRecorder 接口（RecordLLMCall/RecordToolCall/RecordTurn/RecordTokenUsage/IncActiveAgents/DecActiveAgents）。（模块职责 / 关键接口） |
| D3, §4.9 events | EventPublisher 接口（PublishAsync(eventType, source, payload)）。（模块职责 / 关键接口） |
| D3, §4.10 security | 核心职责：命令沙箱、ACL 访问控制（internal/security/sandbox.go）。（模块职责） |
| D3, §4.11 otel | Tracer 接口（Start/Span）、Span 接口（SpanContext/SetAttribute/SetStatus/End）；OTLP 导出。（模块职责 / 关键接口） |
| D3, §4.12 orchestration | 独立于 agent/orchestration.go 的编排引擎；OrchestratorMode（Sequential/Parallel/DAG）；OrchestratorConfig（MaxRetries 默认 3、Timeout 默认 5 分钟）；AgentStep（ID/Name/Agent/Prompt/InputFrom/OutputKey/Condition/RetryPolicy/Timeout/Priority）；组件（Orchestrator/Workflow/Handoff/Collaboration/Visualizer）。（模块职责 / 关键类型） |
| D3, §4.13 concurrency | FileLockManager（基于路径的文件锁，引用计数，Acquire/TryAcquire/Release）；Scope 验证（ValidateScopes，错误 ErrGlobalWriteConflict / ErrScopeOverlap）。（模块职责 / 关键接口） |
| D3, §4.14 config | 热重载 Watcher（监听配置文件变更，OnChange 回调）。（模块职责） |
| D3, §4.15 prompt | 模板引擎（NewTemplate.Execute(map)）；Few-shot 构建器（AddExample/BuildPrompt）。（模块职责） |
| D3, §4.16 debugger | HTTP 调试服务（ap debug 启动；ap loop trace/inspect/resume；端点 /debug/pprof/、/debug/agent/、/debug/tools/、/debug/memory/）；可视化 RenderHTML。（模块职责 / 使用说明） |
| D3, §4.17 admin | HTTP 管理 API（NewServer；端点 /api/agents、/api/agents/:id、/api/agents/:id/stop、/api/metrics、/api/health）。（模块职责） |
| D3, §5 协议式微内核 | 17 个 Capable 接口清单（Memory/RAG/HITL/Hook/Trace/Cost/ContextWindow/Event/Metrics/Checkpoint/Summarizer/FileScope/Cache/Toolkit/Planning/Reflection/ToolLearning）；CapabilityAgent 实现全部 Capable 并提供链式 API；自引用模式（a.self 默认指向自身，WithXxx 时更新为 CapabilityAgent，引擎通过类型断言发现能力）。（关键架构机制） |
| D3, §6 插件生态 | 官方插件表（http/sql/git/json/email/kv，均 0.1.0）；PluginLoader.Load / LoadWithConfig / plugins.LoadAll。（功能特性） |
| D3, §7 公共 API 层 pkg/ | 类型别名 + re-export（Agent/ReActAgent/CapabilityAgent/NewAgent、Provider/OpenAIProvider/NewOpenAIProvider、Tool/ToolRegistry/DefaultToolkit、Memory/WithInMemory、Pool/NewPool 等）；API 稳定性等级（Stable/Experimental/Deprecated/Internal）。（模块职责 / 关键接口） |
| D3, §8 依赖关系图 | agent/ 顶层引用 llm/memory/persist/tools；下层（llm/memory/persist/tools）不能反向引用 agent/；pool/ 依赖 agent/ 和 tools/；横切（guardrail/metrics/events/otel/security/concurrency）被 agent 集成；pkg/ 仅类型导出。（模块依赖方向） |
| D3, §9 关键接口总览 | 核心接口表（Agent/Provider/Embedder/Tool/ToolPlugin/Memory/MemoryStore/RAGProvider/CheckpointStore）；横切接口表（EventPublisher/MetricsRecorder/MessageBus/Transport/ContextWindowStrategy/LLMCache/Hooks/Lifecycle/Tracer）；17 个 Capable 接口指向第 5 章。（关键接口汇总） |
| D3, §10 错误码体系 | 位置 pkg/errors.go（正文仅标位置，未展开；完整错误码表见 D12 §错误码）。（关键接口 / 引用） |
| D3, §11-§21 用户与运维文档 | 依次为：快速上手（v0.8.0 简化入口 ap.NewAgent / 传统入口 / 渐进式能力）、构建与运行（CLI/项目/MCP/插件/K8s/Docker）、Docker 部署（Dockerfile/Compose）、测试体系（核心/CLI/集成/基准/Lint + 测试约定）、性能基准（perf-v11 优化、测试结果示例）、示例应用（链式/传统/生产级）、Provider 贡献指南（9 章节 + 模板）、TypeScript SDK（Go 对等表 + 使用示例）、版本迁移指南（v0.7→v0.8、v0.6→v0.7、迁移检查清单）、pgvector 适配器（特性）、相关文档索引。该部分属用户/运维类，非架构核心，已按章节登记但未逐行摄入。（使用说明 / 现状） |
| D3, §22 设计哲学 | 来自生产 / 接口优先 / 并发原生 / 最小外部依赖 / TDD 强制 / 协议式微内核 / 链式 API。（设计原则） |

### D4：docs/2026-07-02-fix-optimize-evolve.md

> 基于 2026-07-02 深度代码勘察的「修复·优化·进化」文档（草案，待 maintainer 确认）— 来源：docs/

| 章节 | 内容摘要 |
| --- | --- |
| D4, §0 TL;DR | 3 类共 12 项需修复：P0 安全/可发布 3 项（2.5h，本周）；P1 质量/债务 5 项（13.5h，下 1-2 sprint）；P2 进化/对等 4 项（130+h，季度）。最紧急：3 个 stdlib CVE 待 Go 1.26.4 升级。最高 ROI：复活 11 个 //go:build ignore 文件（+134 测试）。最大风险：TS 端「100% Go Parity」营销声明失真（实测 60-70%）。（现状问题 / 风险） |
| D4, §1 目的与方法 | 列出已验证技术债务/性能瓶颈/API 缺口，附文件:行号证据；grep 计数 + 文件读 + 覆盖率验证；区分「已验证」与「估算（⚠️）」。（方法） |
| D4, §2.1 公共 API 规模（实测） | Go pkg/：454 公共符号 / 24 文件 / 2261 行 / 21 模块；TS src/：777 公共符号 / 96 文件 / 28012 行 / 22 模块。（数据口径） |
| D4, §2.2 公共 API 稳定性（实测） | Stable 12 文件 213 符号 / Experimental 7 文件 165 符号 / 混合 3 文件 86 符号 / Deprecated 1 文件（agent.go）5 符号；结论：稳定性治理仍是优秀实践，无需调整。（现状） |
| D4, §2.3 测试现状（实测） | Go：51 包测试 + 200+ _test.go（51/51 通过，0 失败）；TS：33 测试文件 + 1646 it() 断言（1312 通过 / 17 跳过）。（数据口径） |
| D4, §2.4 Go 大文件 Top 10（实测 LoC） | a2a.pb.go 1479（自动生成）；collaboration.go 990（真要拆）；dispatcher.go 773（真要拆）；dag.go 761（真要拆）；visual_editor.go 720；cache.go 684；mcp.go 664；data_tools.go 642；hooks.go 632；filesystem.go 618。（现状问题） |
| D4, §2.5 TS 真实未实现项（实测 7 处，4 真桩） | vector-extended.ts HNSW 为 O(n) 全表扫描（无法用于生产大规模向量召回）；a2a/transport.ts TCP 无连接池复用；operator/crd.ts 自实现 objectToYAML（多行/嵌套易错）；tool-learning.ts:340 avgLatencyMs 硬编码 0（真 bug）；另 3 处为优化建议/文档注释。（现状问题） |
| D4, §3 P0（3 项） | P0.1 升级 Go 1.26.4 关闭 GO-2026-5039（net/textproto）、GO-2026-5037（crypto/x509）2 CVE，附可达触发链（dag.go→ollama_provider→email plugin→discovery）；P0.2 修 TS avgLatencyMs=0 bug；P0.3 删 2 个重复 ignore 测试文件。（待决策项 / 现状问题） |
| D4, §4 P1（5 项） | 复活 11 个 ignore 文件（+134 测试）；拆 collaboration.go(990)；拆 dispatcher.go(773)；提升关键包覆盖率；TS 公开声明修正（缓解合规风险）。（待决策项） |
| D4, §5 P2（4 项） | TS Operator 实现 controller（80h）；TS A2A gRPC 协议（40h）；TS HNSW 真实实现 + 补多模态 3 Provider（16h）；Go 端 3 大文件拆分（6h）。（待决策项 / 路线图） |
| D4, §7 执行路线图 | Week1 P0 全清；Week2-3 P1 拆分大文件；Week4-12 P2 调研驱动。（路线图） |
| D4, §8 验证清单 | 静态检查 / 全量测试 / 覆盖率回归 / 漏洞扫描 / TS SDK / 提交前，每项 PR 必跑。（技术约束） |
| D4, §9 风险登记 | R1 Go 1.26.4 升级兼容性（低/高）；R2 复活 ignore 文件引出 API 漂移（中/中）；R3 TS 修 bug 暴露 store 缺字段（中/低）；R4 拆大文件影响公共 API（低/高）；R5 TS Parity 声明引起社区反感（中/中，建议措辞「Core 100% / Edge 70%」）；R6 P2 占用资源影响 P0/P1（高/高）。（风险登记） |
| D4, §10 附录 | 测量脚本（PowerShell grep 计数，可复现）；证据索引（Go pkg/ 454、TS 777、TS 7 处 TODO、16 个 ignore 文件 155 测试、2 个 stdlib CVE、TS operator 是 Go 8.7%、GroupChat 无活跃测试）。（数据口径 / 证据） |

### D5：docs/CHANGELOG.md

> 版本变更日志 — 来源：docs/

| 章节 | 内容摘要 |
| --- | --- |
| D5, §[Unreleased] | 空（暂无未发布条目）。（现状） |
| D5, §[1.0.0] - 2026-06-30 | Added：全局版本统一 v1.0.0、API 稳定性承诺锁定、API 参考重写、Go vs TS 基准；perf-v11（RAG RRF 融合 / BufferPool / TokenCache / JSON Buffer Pool / pprof / ap loop 子命令 / Fuzz 测试 / 供应链安全文档 / PGO / 基准）；TS SDK Phase 24 基础设施补全（audit/admin/debugger/persist/health）。Changed(Breaking)：audit.NewLogger 签名改为返回 (*Logger, error)。Changed：Dockerfile golang:1.23-alpine→golang:1.26-alpine；版本号修正。Fixed：Dockerfile 构建失败、版本不一致、误提交覆盖率。Added：GitHub Issue Triage Bot、Qwen/GLM/DeepSeek Provider、文档。Fixed：异步摘要结果丢失、Pool Task Map 无界增长（新增 MaxRetainedTasks）、编排循环缺 ctx.Done() 检查、Metrics label 维度缺失。（现状 / 变更） |
| D5, §[0.7.0] - 2026-06-05 | Added：公共 API 稳定性策略（4 级 Stability 标注）、SemVer spec、协议式微内核（12 个 *Capable + WithXxx 链式 API）、LLM Provider 模板、Operator Service/HPA/Pod 指标、TS 编排/A2A/MCP、CI 安全扫描。Changed：ReActConfig 14 字段标 Deprecated（v2.0 移除）。Fixed（CRITICAL）：filesystem.go EvalSymlinks 失败静默放行 symlink 逃逸、react_loop.go 搜索 RAG nil 解引用 panic、resilient.go HalfOpen 熔断器逻辑反转、License 不一致、StreamRun goroutine 错误丢失、Operator 缺 Finalizer、ConfigMap YAML 不安全生成等。（现状 / 变更） |
| D5, §[0.6.0]~§[0.1.0] | 早期版本条目：pre-commit hook + Agent 模板系统（0.6.0）；各 phase 功能（0.5.0–0.1.0）。（历史） |

### D6：docs/RELEASE-NOTES-v0.1.0.md

> Phase 1-D 发布说明 — 来源：docs/

| 章节 | 内容摘要 |
| --- | --- |
| D6, §概述 | 首个完整可用版本，从 CodeCast 生产验证提炼通用能力，并补齐生产必需模块。（现状） |
| D6, §交付统计 | 98 Go 源文件 / 17661 行 / 7251 测试行（占比 41%）/ ~195 测试 / 全量通过 / 外部依赖 1 个（modernc.org/sqlite）/ Go 1.26+。（数据口径 / 注：外部依赖「1 个」口径与 D1 白名单 3 类不一致，见 §3 X5） |
| D6, §新增功能 P0（Task 10） | OpenAI 兼容 HTTP Provider：核心 openai_provider.go，含 Anthropic/Azure/Gemini/Ollama；能力含同步补全/SSE 流式/函数调用/向量生成/双层错误处理；零外部依赖（仅 net/http + encoding/json）。（功能特性） |
| D6, §新增功能 P0（Task 11） | FileLock Manager（internal/concurrency/filelock.go）：Acquire/Release/TryAcquire/ValidateScopes（全局写冲突 + 路径前缀重叠校验）。（功能特性） |
| D6, §新增功能 P1/P2 | 架构统一、编排增强、新能力扩展（章节标题，详情见 D5/D7）。（功能特性） |
| D6, §架构总览 / 外部依赖 / CodeCast 迁移路径 / 已知限制 / 下一阶段规划 | 架构总览图；外部依赖仅现代c SQLite；CodeCast 迁移路径；已知限制；下一阶段 Phase 2 规划。（现状 / 路线图） |

### D7：docs/RELEASE-NOTES-v0.2.0.md

> Phase 2 发布说明 — 来源：docs/

| 章节 | 内容摘要 |
| --- | --- |
| D7, §概述 | 三大目标：CodeCast 对齐+架构修复（2-A）/ 架构统一+编排增强（2-B）/ 新能力扩展（2-C）；从「可用」升级为「好用」。（现状） |
| D7, §交付统计 | ~130 Go 源文件 / ~22000 行 / ~300+ 测试 / 外部依赖 1 个 / Go 1.26+。（数据口径） |
| D7, §新增功能 P0（2-A, Task 18） | Scope/FileLock 自动注入工具流：Executor 增加 ScopePolicy 和 FileLockManager 字段；FileSystem write/edit 自动检查 Scope 并获取 FileLock；Shell 命令执行前检查 Scope。（功能特性 / 技术约束） |
| D7, §新增功能 P1（2-B） | 统一消息总线、Run/StreamRun 去重、Session 分组管理、默认工具集、编排模式导出。（功能特性） |
| D7, §新增功能 P2（2-C） | 分布式 Agent 通信、DAG 工作流、Agent 发现协议、更多 LLM Provider、Web UI 管理面板、性能基准测试。（功能特性） |
| D7, §迁移指南 / 已知限制 / 下一阶段规划 | v0.1.0→v0.2.0 迁移（Agent 接口替代具体类型、AgentFactory 配置 Pool、LocalMessageBus 替代 A2ABus、DefaultToolkit、PromptTemplate 注入 Scope）；已知限制；Phase 3 P2 规划。（现状 / 路线图） |

### D8：docs/RELEASE-NOTES-v0.7.0.md

> Phase 9/11/12 发布说明（安全加固 + K8s Operator 完成 + TS SDK 扩展）— 来源：docs/

| 章节 | 内容摘要 |
| --- | --- |
| D8, §概述 | 三大目标：安全加固（Phase 9）/ K8s Operator 完成（Phase 11）/ TS SDK 扩展（Phase 12）；从「功能完备」升级为「生产就绪」，零 CGO 依赖。（现状） |
| D8, §CRITICAL 安全加固 | 输入验证增强（LLM 参数/Memory Episode/Tool JSON Schema/Event payload 大小限制）；路径安全（symlink resolve、规范化、白名单严格匹配，拒绝穿越与编码绕过）；命令执行防护（参数化、白名单、30s 超时，拒绝注入）；依赖安全审计（govulncheck + pre-commit）。（技术约束 / 安全） |
| D8, §K8s Operator 完成 | AgentDeployment CRD（apiVersion: agent.agentprimordia.io/v1alpha1）、Controller 调谐循环、HPA 自动扩缩容、Metrics 暴露、Status 聚合（ActiveReplicas/CompletedTasks/FailedTasks）。（功能特性 / 注：CRD apiVersion 与 D2 不一致，见 §3 X4） |
| D8, §TS SDK 扩展 | 完整 TS SDK，覆盖 Agent 创建/工具注册/流式/记忆，35 个测试用例。（功能特性） |
| D8, §Fixed | symlink 逃逸静默放行、搜索 RAG nil panic、熔断器逻辑反转、License 不一致、StreamRun goroutine 错误丢失、json.Marshal 静默忽略、Operator 缺 Finalizer、镜像硬编码、ConfigMap YAML 不安全生成、TS SSE 无超时、TS ReAct 连续工具失败无退出、CLI config YAML 解析、CI YAML 语法、Windows race。（现状 / 修复） |

### D9：docs/RELEASE-NOTES-v0.8.0.md

> 开发者体验重构（DX）发布说明 — 来源：docs/

| 章节 | 内容摘要 |
| --- | --- |
| D9, §主题 | 聚焦「从零到第一个 Agent」的开发者体验，新增 3 个公共 API + 1 个 testutil 包，把 50+ 行样板压到 3 行。（现状） |
| D9, §新增公共 API：ap.NewAgent() | 简化入口 ap.NewAgent(name, systemPrompt, model, opts...)，替代 14 个 Deprecated 字段的 ReActConfig；新增 AgentOption / WithMaxTurns / WithTemperature / WithSessionID；旧 NewReActAgent 仍可用。（功能特性） |
| D9, §新增公共 API：WithRAGMemory | agent.WithRAGMemory(mem, emb) 一步 RAG（2 步替代 6 步手动组装）；内部 ragStoreAdapter 透明适配；默认 Mode=RAGModeAuto、TopK=5、MinScore=0.3。（功能特性） |
| D9, §新增公共 API：testutil | testutil.MockProvider（实现 llm.Provider + llm.Embedder）+ NewTestAgent() 快捷构造器，消除手写 40 行 MockLLM。（功能特性 / 测试） |
| D9, §内部 API 变化 | pkg/options.go 重命名（WithMaxTurns→WithMaxIterations、WithTemperature→WithTemp，旧为死代码）。（现状） |
| D9, §升级建议 | 推荐迁移到新 API；完全向后兼容。（现状） |

### D10：docs/RELEASE-NOTES-v1.0.0.md

> 正式发布说明（版本统一与 API 稳定性承诺）— 来源：docs/

| 章节 | 内容摘要 |
| --- | --- |
| D10, §主题 | v1.0.0 首个正式稳定版本；Go/TS/CLI/go.mod 模板全局统一 v1.0.0；API 稳定性承诺锁定。（现状） |
| D10, §核心变更 | 版本统一（Go pkg.Version / TS package.json / CLI / go.mod 模板 均 →1.0.0）；API 稳定性承诺（Stable 冻结向后兼容，破坏性变更仅 v2.0，Deprecated 至少保留一个大版本周期）。（现状 / 技术约束） |
| D10, §新增功能：RAG RRF 融合 | Reciprocal Rank Fusion（Cormack et al., 2009）；LinearFusion（默认，分数加权）/ RFFFusion（排名融合，RRFK=60）；双命中 2x 加成；Over-fetch 预取 topK+OverFetchSize。（功能特性） |
| D10, §新增功能：性能优化 | Pool sync.Cond 动态信号量（AutoScaler 实时生效无忙等待）；GoroutinePool Wait() CPU ~100%→~0%；LLM 共享 HTTP 连接池；HookContext sync.Pool；bytes.Buffer 池化 2.2x / 0 allocs；SSE Timer 背压（5s 超时）；Token 估算 len/4 直接计算。（性能） |
| D10, §新增功能：供应链安全 / PGO / CLI | govulncheck+npm audit+Trivy+cosign+SBOM+Fuzz；PGO 调优指南；ap loop trace/inspect/resume。（安全 / 工程） |
| D10, §测试覆盖 | Go 47 包 / 2900+ 用例 / 100%；TS 6 包 / 154 用例 / 100%。（数据口径） |
| D10, §升级指南 | 从 v0.8.0 升级（go get / npm install / ap version）；完全向后兼容，推荐迁移到 NewAgent。（现状） |

### D11：docs/TypeScript_vs_Go_Deep_Evaluation.md

> Go 与 TypeScript 运行时/并发/GC/性能/安全等 15 章深度对比评估 — 来源：docs/（约 67KB）

| 章节 | 内容摘要 |
| --- | --- |
| D11, §第一章 运行时内部机制 | V8 JIT（Ignition→Maglev→TurboFan 多级）vs Go AOT 编译为机器码；TS 存在去优化风险与预热期，Go 启动即优化机器码；Go 1.21+ 显式 PGO（default.pgo），V8 内部隐式。（技术评估） |
| D11, §第二章 并发模型 | Node.js libuv 事件循环 vs Go GMP 调度器；结论表：Go 并发 10/10，TS 5/10。（技术评估） |
| D11, §第三章 内存管理与 GC | V8 Orinoco（分代，老生代堆默认 1.5-4GB）vs Go GC（GOGC/GOMEMLIMIT 软上限）；设计哲学差异。（技术评估） |
| D11, §第四章 性能数据与可复现方法论 | 测试环境规范、基准测试定义、可复现方法论（Go/Node benchmark 脚本）。（方法） |
| D11, §第五章 错误处理哲学 | Go error 值传递 vs TS try-catch + 自定义错误；对架构可靠性的实际影响（Go 显式处理更可靠）。（技术评估） |
| D11, §第六章 性能调优工具链 | Go pprof（CPU/内存/goroutine/执行追踪）vs Node clinic.js/0x/Chrome DevTools。（技术评估） |
| D11, §第七章 FFI/互操作 | Go cgo vs Node N-API/WASM；WASM 互操作。（技术评估） |
| D11, §第八章 安全模型 | TS/Node 安全模型 vs Go 安全模型（govulncheck 漏洞扫描）。（技术评估 / 安全） |
| D11, §第九章 大规模工程实践 | Monorepo（go.work）/ 增量编译 / 团队协作模式。（工程） |
| D11, §第十章 成本分析 | 内存占用云成本 / AWS Lambda 计费 / EC2 容器 / 招聘成本；Go 在内存与冷启动成本占优。（技术评估） |
| D11, §第十一章 语言演进路线 | TS/Go 演进与泛型对比。（技术评估） |
| D11, §第十二章 真实迁移案例 | Hasura（Node→Go）、Uber、Stream、SafetyCulture、TS 编译器自身（TS→Go Project Corsa）。（参考） |
| D11, §第十三章 测试文化 | 框架对比 / 基准 / 模糊测试 / 竞态检测（Go 原生 race detector）。（技术评估） |
| D11, §第十四章 代码组织范式 | 模块系统 / 项目结构惯例 / 依赖注入。（工程） |
| D11, §第十五章 综合评估矩阵与结论 | 全维度评分卡（Go 在并发/内存/启动/云原生/编译/安全供应链占优；TS 在前端/类型表达/开发效率/人才占优）；选型决策矩阵；最终结论：**TypeScript 与 Go 互补而非竞争**——Go 不可替代领域为云原生基础设施/高并发后端/网络代理/CLI/Serverless，TS 不可替代领域为 Web 前端/全栈/实时通信。（技术评估 / 结论，供下游技术选型参考，本工程师不裁决） |

### D12：docs/api-reference.md

> 公共 API 参考（导入别名 ap）— 来源：docs/（约 28KB）

| 章节 | 内容摘要 |
| --- | --- |
| D12, §核心类型 | Agent 接口（Run/StreamRun/Stop/Stats/Name）；ReActAgent（核心引擎）；ReActConfig（仅标量字段，工具/记忆/RAG 等用 WithXxx 链式注入，v0.7.0 前 14 字段已废弃）；Message/Role/ToolCall/Thought/Response；StreamEvent（6 种）；PromptTemplate（{{.Variable}} 注入）。（关键类型） |
| D12, §消息总线 | MessageBus 接口（Send/Broadcast/Register/Unregister/ListAgents/Subscribe）；LocalMessageBus；BusMessage（8 类型：task_request/result、query/response、handoff、broadcast、status_update、notify）；Transport/HTTPTransport。（关键接口） |
| D12, §编排 | Pipeline（顺序，条件跳过）/ Handoff（Router 动态交接，MaxHandoffs）/ ParallelRun / DAG（DAGBuilder：Node/Edge/EdgeIf/Build/MustBuild）/ GroupChat（轮询/投票/LLM 路由）/ Debate / Supervisor。（关键类型） |
| D12, §RAG 知识库 | RAGConfig / RAGMode（Auto/First/OnDemand）/ RAGProvider / RAGDocument。（关键类型） |
| D12, §工具系统 | Tool / ToolRegistry / ToolExecutor；ToolPermission / ScopePolicy / FileScopePolicy（SetScope）；内置工具（FileSystem/Shell/Web/KnowledgeSearch）；ToolkitConfig / DefaultToolkit / MinimalToolkit。（关键接口 / 类型） |
| D12, §记忆存储 | Memory 组合接口（20+ 方法：Add/Search/Get/Delete/Count/List/UpdateSummary/SetImportance/SearchByTag/GetImportant/GetTimeline/CleanupExpired/Stats/RecordToolUse/ClearAll/Export/Import/Close）；SQLiteStore（WithInMemory 测试）；VectorStore / EmbeddingProvider（NewEmbeddingAdapter）；RAGStore（HybridSearch，RRF LinearFusion/RFFFusion，RRFK=60）；Episode / SearchOptions / ListOptions / MemoryStats / CleanupConfig；文档处理（DocumentLoader/TextFileLoader/Splitter/DocumentPipeline）。（关键接口 / 类型） |
| D12, §LLM 抽象 | Provider 接口（Complete/Stream/CallTools/Info）+ Embedder；提供者实现表（OpenAI/Anthropic/Gemini/Ollama/Azure/Qwen/GLM/Cohere/Mistral/Resilient，DeepSeek 用 NewOpenAIProvider + BaseURL）；Config / AzureConfig；ResilientProvider（默认 3 重试 / 500ms 退避 / 5 熔断阈值 / 30s 恢复）；请求/响应类型（CompletionRequest/Response、ToolCallRequest/Response、Chunk、ModelInfo、Usage 等）。（关键接口 / 类型） |
| D12, §Pool 调度 | Pool（MaxConcurrency/Timeout/RetryPolicy）；AgentFactory（SetAgentFactory 自定义创建）；PoolConfig / TaskConfig / TaskResult / PoolStats / PoolEvent / RetryPolicy / AgentFactoryConfig；任务状态（Queued/Running/Completed/Failed/Cancelled）。（关键类型） |
| D12, §钩子系统 | HookManager / HookPoint（19 个挂载点：BeforeRun/AfterRun/BeforeTurn/AfterTurn/BeforeLLM/AfterLLM/BeforeTool/AfterTool/OnError/OnComplete/BeforeRAG/AfterRAG/BeforePipelineStep/AfterPipelineStep/BeforeHandoff/AfterHandoff/BeforeParallelAgent/AfterParallelAgent 等）；Register / RegisterWithPriority。（关键接口） |
| D12, §适配器 | 6 个适配器：Memory→Agent（NewMemoryAdapter）、EventBus→Agent（NewEventBusAdapter）、Metrics→Agent（NewMetricsAdapter）、LLM→Memory（NewEmbeddingAdapter）、RAGStore→Agent（NewRAGProviderAdapter）、RAGStore→Tool（NewKnowledgeSearcherAdapter）；附完整组装示例。（关键接口 / 桥接） |
| D12, §事件系统 | Bus / Event / EventType（14 种：AgentStart/Stop/Panic/Error/Resume、TurnStart/End、ToolCall/Result、LLMCall/Response、PoolDispatch/Complete）。（关键接口） |
| D12, §安全 | ACL / ACLRule / AccessLevel（None/Read/Write/Execute/All）；Sandbox（AllowCommand/AllowPath）。（关键接口 / 安全） |
| D12, §并发控制 | FileLockManager（Acquire/Release）；ValidateScopes（返回 ErrScopeOverlap）。（关键接口） |
| D12, §指标 | AgentMetricsCollector / Histogram；导出器（PrometheusHandler/LogExporter/JSONExporter/MultiExporter/MetricsExporter）。（关键接口） |
| D12, §状态持久化 | CheckpointStore / SQLiteCheckpointStore（NewSQLiteCheckpointStore / InMemoryCheckpointStore）；AgentState。（关键接口 / 类型） |
| D12, §函数选项 | WithTimeout / WithMaxTurns / WithTemperature / WithCheckpoint / WithStreaming / WithMetadata。（关键 API） |
| D12, §错误码 | GetErrorCode(err) 提取结构化错误码表：AGENT_001~004、TOOL_001~004、LLM_001~008、POOL_001~003、CTX_001、MEM_001~008、SEC_001~004、EVT_001、PST_001、CON_001~002（共 32+ 项）。（关键接口 / 错误体系） |
| D12, §TypeScript SDK API | 安装 npm install @agentprimordia/sdk；核心 API 对照表（NewAgent/NewOpenAIProvider/NewResilientProvider/NewPool/NewDAGBuilder/NewMCPClient/DefaultToolkit/NewAuditLogger/NewAdminHandler/NewHealthServer 等）；基础设施 API（Phase 24：AuditLogger/AdminHandler/Inspector/DebugServer/HealthServer）。（关键 API / 注：100% 对等声明与 D4 实测不一致，见 §3 X1） |

### D13：docs/architecture-mermaid.md

> Mermaid 格式架构图（7 张）— 来源：docs/

| 章节 | 内容摘要 |
| --- | --- |
| D13, §1 系统架构 | mermaid graph：CLI（ap init/run/debug/test/mcp）→ Agent Layer（ReActAgent/ReAct Loop/Hook System/RAG Provider）→ LLM Layer / Memory Layer / Tool Layer / Orchestration；Operator（CRD/Controller/ConfigMap/Deployment）；TS SDK（ReActAgent/Provider/Memory/ToolRegistry/Pool）。（架构示意） |
| D13, §2 ReAct Loop 执行流 | flowchart：Think→Parse→Has tool_calls?→Select Tool→Permission granted?→Exec Tool in Sandbox→Observe→Record Episode→Hook→连续失败退出 / Max turns 完成。（执行流示意） |
| D13, §3 核心数据结构 | classDiagram：ReActAgent（provider/memory/tools/hooks/maxTurns/consecutiveFailures）、Provider（interface）、MemoryStore（interface）、Tool（interface）、AgentDeployment/AgentDeploymentSpec/AgentTemplateSpec。（类型关系） |
| D13, §4 K8s Operator 调谐 | sequenceDiagram：Watch AgentDeployment→finalizer→Ensure ConfigMap(ap.yaml)→Ensure Deployment(agent+metrics)→Get status→Update CRD status。（运维流示意） |
| D13, §5 Resilient Provider | stateDiagram：Closed→(failures≥threshold)→Open→(recoverAfter)→HalfOpen→Closed/HalfOpen probe 结果。（容错状态机） |
| D13, §6 多 Agent 编排模式 | graph：Sequential（S1→S2→S3）/ Parallel（P1/P2/P3）/ DAG（D1→D3，D2→D3，D1→D4，D2→D4）。（编排示意） |
| D13, §7 TS SDK Go Parity | graph：TS 24 模块（agent/llm/tools/memory/orchestration/pool/a2a/security/metrics/resilience/prompt/audit/admin/debugger/persist/health/communication）与 Go internal/ 1:1 对等；标注「Every Go module has a 1:1 TypeScript counterpart」。（架构示意 / 注：与 D4 实测不一致，见 §3 X1） |

### D14：docs/multi-agent-collaboration-prompt.md

> 多智能体协同编程系统提示词（用于驱动多 Agent 协作，含系统架构/智能体类型/质量门禁/异常处理）— 来源：docs/（约 47KB，15 章）

| 章节 | 内容摘要 |
| --- | --- |
| D14, §一.1 总体架构 | 分层解耦 + DAG 编排；所有智能体通过统一 MessageBus 通信；Orchestrator（Task Planner/Dispatcher/DAG Executor）负责任务调度；SharedStore 跨智能体共享记忆层（知识库/设计决策/代码规范/进度面板）。（概念架构，供下游业务/系统架构参考，本工程师不裁决） |
| D14, §一.2 通信拓扑 | 同步 BusMessage 点对点请求-响应（task_request→task_result）；异步 broadcast 推送状态/知识；共享状态 SharedStore；消息优先级 task_request > status_update > notify > broadcast。（通信设计） |
| D14, §一.3 编排模式选择 | 需求→设计→开发→测试 Sequential/DAG；多模块并行 Parallel；架构讨论 GroupChat/Debate；代码审查 Review；复杂依赖 DAG；条件分支 Workflow。（编排设计） |
| D14, §二 智能体类型定义 | 7 类：RequirementsAnalyst(P0)/Architect(P0)/Developer/Tester/Reviewer/Documenter/Coordinator；各含优先级、职责、输入、输出、工具权限、约束（如需求分析师不得直接写业务代码、需求变更须广播确认）。（角色设计 / 待下游业务架构参考） |
| D14, §三 任务分解策略 | 分解原则/流程/依赖关系类型（数据/控制/资源依赖）。（协作设计） |
| D14, §六 冲突解决机制 | 冲突类型与处理策略/解决流程/文件锁冲突处理（FileLock + Scope）。（协作设计） |
| D14, §九 跨智能体知识共享 | 共享记忆架构/知识发布协议/检索协议/一致性保障。（协作设计） |
| D14, §十 技术栈要求 | 后端 Go ≥1.26（零 CGO）、前端 TypeScript ≥5.4、AgentPrimordia ≥v1.0.0、SQLite(modernc)、YAML(gopkg.in/yaml.v3)；编码规范（命名/错误包装/godoc/TSDoc/Conventional Commits）；安全要求（输入验证/路径安全/命令执行 Sandbox/敏感数据/依赖审计）。（技术约束） |
| D14, §十一 质量保障 | 多层质量门禁（静态→单测≥80%→代码审查→集成 E2E→交付审查）；自反思机制（Critique→严重度→Improve→Lessons Learned）；交叉审查（critical/high 必须修复）；持续质量指标（覆盖率≥80%、Lint 0、漏洞 0、审查通过率≥95%、critical 修复≤30min、重试≤2、心跳丢失≤1%）。（质量门禁 / 待下游产品/平台架构参考） |
| D14, §十二 异常处理 | 智能体异常（超时/崩溃/死循环/权限不足）；系统级异常（LLM 不可用→ResilientProvider 降级、总线拥塞、存储故障、DAG 死锁）；降级策略（A/B/C/紧急 4 级）。（容错设计） |
| D14, §十三-§十五 协作示例/附录/使用指南 | 标准开发流程示例；附录能力矩阵/消息流转速查/DAG&GroupChat 伪代码；提示词适用场景与定制化建议。（使用说明） |

### D15：docs/concepts/interface-graph.md

> 跨包接口关系图（维护日期 2026-06-10，适用 v1.0.0+）— 来源：docs/concepts/

| 章节 | 内容摘要 |
| --- | --- |
| D15, §1 模块边界回顾 | agent/ 顶层（Agent/MemoryStore/RAGProvider/Tracer/Hooks/EventPublisher/MetricsRecorder/ContextWindowStrategy）；下层 llm/memory/persist/tools；pool 依赖 agent/tools；ecosystem 不 import internal。（模块边界） |
| D15, §2 关键接口映射表 | X(来自 Y)→Z(agent) 的适配方式：memory.Memory→agent.MemoryStore 直接用；memory.RAGStore→agent.RAGProvider 用 WithRAGMemory/NewRAGProviderAdapter；llm.Provider→memory.EmbeddingProvider 用 NewEmbeddingAdapter(1536)；persist.CheckpointStore→WithCheckpointStore；tools.Registry→WithToolkit 等。（接口桥接） |
| D15, §3 不需要适配的情况（90% 场景） | WithMemory/WithToolkit/WithHooks/WithTracer/WithCostTracker/WithContextWindow/WithEvents/WithMetrics/WithCheckpointStore/WithSummarizer/WithFileScope/WithCache/WithHITL 零适配直接注入。（接口桥接） |
| D15, §4 需要适配的唯一情况：RAG | 推荐 WithRAGMemory(memoryStore, llmProvider)（2 步）；或手动 6 步（NewEmbeddingAdapter→NewRAGStore→NewRAGProviderAdapter→RAGConfig→WithRAG）。（接口桥接） |
| D15, §5 接口定义清单 | 16 个接口（Agent/MemoryStore/RAGProvider/Tracer/Hooks/EventPublisher/MetricsRecorder/ContextWindowStrategy/Memory/EmbeddingProvider/SummaryExtractor/CheckpointStore/Provider/Embedder/LLMCache/Registry）的方法数与用途。（关键接口） |
| D15, §6 总结原则 | 同接口影子（依赖倒置，agent 层只暴露需要的方法）；RAG 需桥接（框架 WithRAGMemory 自动完成）；扩展自定义能力实现接口→WithXxx 注入。（设计原则） |

### D16：docs/plans/2026-06-26-pre-commit-cleanup.md

> v0.8.0 生产加固：pre-commit 既有问题修复计划 — 来源：docs/plans/

| 章节 | 内容摘要 |
| --- | --- |
| D16, §背景 | v0.8.0 提交时 pre-commit 检测 3 问题：gofmt 未过（约 130 个 .go 文件）/ Deprecated 标注不全（缺 Removed in vX.Y）/ 缺 docs/plans/*.md。（现状问题） |
| D16, §前置约束 | go build/vet 零错误；gofmt -l agentprimordia/ 空；pre-commit 全过（不再 --no-verify）。（技术约束） |
| D16, §实施路线 | Task1 gofmt -w（仅格式无逻辑变更）；Task2 修 Deprecated 标注（"Removed in v1.0.0"）；Task3 补 plan 文档（本文件）。（修复计划） |
| D16, §风险评估 | 生产影响无；兼容；可 git revert。（风险） |

### D17：docs/plans/grpc-migration.md

> A2A gRPC/protobuf 迁移实施计划（目标 v2）— 来源：docs/plans/

| 章节 | 内容摘要 |
| --- | --- |
| D17, §1 背景与动机 | 现状 A2A v1 = HTTP/1.1 + JSON-RPC 2.0 + SSE + encoding/json（4930 行 19 文件，下游用户 0，除 pkg/a2a.go 外无 import）；目标 v2 = HTTP/2 + gRPC + protobuf + server-streaming；收益：序列化 -70%、体积 -35~50%、吞吐 8x、流式首字节 -85%、类型安全断崖提升。（待决策项 / 计划，注：现状描述与 D1 §2.1 不一致，见 §3 X3） |
| D17, §2 范围与非范围 | In Scope：proto IDL（8 消息 + 5 RPC）、手写 pb.go、gRPC server/client、类型重写、认证迁移、流式事件、测试、pkg 重导出；Out of Scope：mTLS/interceptor 链/retry、流控 backpressure、双向流、跨语言客户端、服务网格。（计划范围） |
| D17, §3 协议设计（.proto IDL） | proto3 IDL：AgentCard/AgentCapabilities/SecurityScheme(AuthType: NONE/API_KEY/BEARER/MTLS)/AgentSkill/Task(TaskState enum)/TaskStatus/A2AMessage/Part(oneof Text/File/Data)/Artifact/TaskEvent；Service A2AService 5 RPC：FetchAgentCard/CreateTask/GetTask/CancelTask/StreamEvents(server-streaming)。（协议设计 / 关键接口） |
| D17, §4 实施步骤 | Phase A 基础设施（4-6h，加依赖/写 IDL/手写 pb.go/grpc server+client/集成测试/benchmark，旧 JSON-RPC 不动全 PASS）；Phase B 切换默认（2-3h，重写 server/client、类型 json→proto、pkg 重导出、文档）；Phase C 清理（3-4h，重写 19 测试、删 jsonrpc/sse、最终 bench）。（实施路线） |
| D17, §5 关键技术决策 | 手写 .pb.go vs protoc（决策手写，因无 protoc/buf 工具链，与 google.golang.org/protobuf protoimpl 兼容）；Part oneof；时间字段 google.protobuf.Timestamp；认证 HTTP header→gRPC metadata；错误 gRPC status code；流式 server-streaming；文件结构（proto/ + proto/gen/）；验证标准（Phase A/B/C 完成标准）；风险与回滚；时间线。（技术决策 / 待下游平台架构参考） |

### D18：docs/plans/perf-v5-comprehensive-audit.md

> perf-v5 综合性能优化实施计划 — 来源：docs/plans/

| 章节 | 内容摘要 |
| --- | --- |
| D18, §总体概览 | ~121 项发现（12 Critical / 40 High / 43 Medium / 26 Low）；按模块分布：LLM 39（5C/10H/14M/10L）、Agent 22、Orchestration 18、Memory 9、Pool+Metrics+Events 8、Tools/Executor 5、通用 ~20。（现状问题 / 性能） |
| D18, §前置约束 | go build/vet 零错误；修改模块 go test PASS（含新增 benchmark）；中文注释；提交前缀 perf(v5):。（技术约束） |
| D18, §实施路线 Phase 1 Critical | Task1 Tool Executor 缺 panic recovery（任意工具 panic 杀死整个 agent 进程，加 defer recover）；Task2 14 个 Provider SSE Stream body 关闭一致性（资源泄漏）；Task3 熔断器半开 race（resilient.go CAS 失败方应返回 ErrCircuitOpen）；Task4 MCP transport 缺 timeout（goroutine 永久挂起，加 30s + 共享 transport）；Task5 Supervisor/Handoff/Pipeline/Collaboration 锁内 JSON 序列化（持锁过长，锁内只快照锁外 marshal）。（性能 / 待决策项） |
| D18, §实施路线 Phase 2 High | Task6 11 个 Provider 缺 Transport 配置（提取 newDefaultLLMTransport，预期吞吐 +30~50%、P99 -50ms）；Task7 协作 prompt strings.Builder；Task8 Cache fingerprint 缓存 + Stats 修复；Task9 Cache LRU 改 container/list O(1)；Task10 11 个 Provider request body typed struct（序列化 -50%）；Task11 HookStats 锁内 map 改 atomic；Task12 HookManager.Fire 减少 slice 拷贝；Task13 CostTracker O(n)→O(1)；Task14 InMemoryStore 锁内 ToLower 移锁外；Task15 Pool dispatcher.pt.status atomic。（性能 / 待决策项） |
| D18, §实施路线 Phase 3/4 Medium/Low | Phase3：锁内回调移出锁外、Stream 重复拷贝、Buffer sync.Pool、日志改 slog+脱敏、Event payload Pool、HITL strings.Builder、mergeSuggestions 预计算、Bus Broadcast 锁内分配、Cache 慢路径分桶/HNSW、TTL 后台清理、Workflow vs DAG Metrics 分离、GroupChat selector atomic；Phase4：历史滑动窗口默认 100 条、序列化导入清理、errors.New+%w、TODO 清理、Benchmark 套件补充。（性能 / 待决策项） |
| D18, §优先级排序与验证 | Quick Win（〈1h 改动〉）优先；验证 build+vet+test+bench。（路线图） |

---

## 3. 冲突记录

> 不同资料对同一事实描述矛盾时，**并列保留两个版本**，不做裁决。

| 编号 | 冲突主题 | 版本 A | 出处 A | 版本 B | 出处 B | 差异说明 |
| --- | --- | --- | --- | --- | --- | --- |
| X1 | TypeScript SDK 与 Go 的功能对等程度 | 100% Go 功能对等（营销/文档声明） | D2 §特性、D3 §1 核心特性、D12 §TypeScript SDK API、D13 §7（标注「Every Go module has a 1:1 TypeScript counterpart」） | 实测 TS 端 60-70% 对等，存 7 处未实现（4 处真桩：HNSW O(n) 全表扫描、A2A TCP 无连接池、operator YAML 自实现、tool-learning avgLatencyMs=0 bug） | D4 §0 TL;DR、§2.5、§9 风险 R5 | 文档/营销声明「100% 对等」与 2026-07-02 实测「60-70% 对等」矛盾；D4 建议措辞改为「Core 100% / Edge 70%」。待主理人/下游裁决对外表述口径。 |
| X2 | Go 最低版本要求 | Go 1.26+（go.mod 声明 go 1.26） | D1 §2、D2 §特性（badge）、D6 §交付统计、D7 §交付统计 | 实测当前 Go 1.26.3，需升级到 1.26.4 关闭 2 个 stdlib CVE（GO-2026-5039/GO-2026-5037）；文档应改为「Go 1.26.4+」 | D4 §0 TL;DR、§2.1 范围、§3 P0.1 | 文档声明的「1.26+」与实测需「1.26.4」存在版本口径差异；属已识别待修复项（P0.1）。 |
| X3 | A2A 协议当前实现形态 | gRPC + protobuf 是 A2A 协议的「事实标准/已采用」实现（白名单仅限 a2a/ 使用 grpc+protobuf） | D1 §2.1（白名单说明）、D2 §特性（列 A2A Communication） | 当前 A2A 实际为 HTTP/1.1 + JSON-RPC 2.0 + SSE，gRPC+protobuf 是「计划中的迁移目标（v2）」，尚未落地 | D17 §1.1 现状（v1）、§1.2 目标（v2）、§4 Phase 路线 | D1 将 gRPC+protobuf 描述为已采用/白名单事实，D17 明确当前仍是 JSON-RPC+SSE，gRPC 为待实施迁移。待下游平台架构确认迁移计划优先级。 |
| X4 | K8s Operator CRD 的 apiVersion | agent.primordia.dev/v1 | D2 §K8s 部署（示例 yaml） | agent.agentprimordia.io/v1alpha1 | D5 §[0.7.0]、D8 §K8s Operator 完成 | README 部署示例与 CHANGELOG/v0.7.0 对 CRD apiVersion 命名不一致（可能为演进中的命名，v1alpha1 vs v1）；待确认最终 CRD group/version。 |
| X5 | 外部依赖数量口径 | 外部依赖 1 个（仅 modernc.org/sqlite） | D6 §交付统计、D8 §交付统计（v0.7.0 仍写「外部依赖 1 个」） | 白名单含 3 类第三方依赖：modernc.org/sqlite + gopkg.in/yaml.v3 + grpc/protobuf（仅限 a2a/） | D1 §2.1（白名单） | 早期发布说明称「1 个依赖」，AGENTS.md 白名单列出 3 类（yaml 与 grpc 为特定用途）。属统计口径差异（运行时核心 vs 全白名单），非事实矛盾。 |
| X6 | 用户文档所在路径 | 用户文档位于 agentprimordia/docs/ 下 | D1 §7 文档同步 | 实测仓库存在两处：AgentPrimordia/docs/（顶层，本次摄入范围）与 AgentPrimordia/agentprimordia/docs/（子目录，含 advanced/api/agent-reference 等） | D2 §文档、D3 §3 项目结构、文件系统实测 | AGENTS.md 描述的文档路径与实际仓库布局不完全一致；两处 docs 均存在，下游需注意文档引用路径。待确认权威文档根。 |

---

## 4. 硬指标清单

| 章节 | 硬指标 | 状态 |
| --- | --- | --- |
| §1 | 每份资料有解析状态，失败/跳过注明原因 | ✅ |
| §2 | 每份文档按章节逐条摘要，每条标注了 `D编号，§章节` | ✅ |
| §3 | 冲突信息并列保留，不做裁决 | ✅ |
| 全文 | 全文不得残留尖括号占位符、待填日期、尖括号数字占位符、示例前缀或 [待补充] 字样 | ✅ |
| D17/D18 | 计划类文档（迁移/性能）已标注为「待决策项 / 计划」，未越权裁决 | ✅ |

---

## 附录 A：生成流程

### 流程总览

| 步骤 | 动作 | 落入章节 |
| --- | --- | --- |
| Step0 | 读取模板 + 全部原始资料（D1–D18 及 3 个图示文件） | — |
| Step1 | 盘点资料清单，标注解析状态 | §1 |
| Step2 | 逐份打开资料，按自身章节结构逐条摘要 | §2 |
| Step3 | 交叉比对不同资料，发现并记录矛盾 | §3 |
| Step4 | 逐项核验硬指标 | §4 |

```mermaid
flowchart LR
    S0[读取模板与资料] --> S1[盘点资料清单]
    S1 --> S2[逐份精读逐章节摘要]
    S2 --> S3[交叉比对记录冲突]
    S3 --> S4[硬指标自检]
```

### 整理原则

1. **逐份精读，不跨文档归并**：摘要按文档自身章节结构组织，不做跨文档的主题重组（那是下游的事）
2. **出处即章节号**：每条摘要标注 `D编号，§章节`，直接映射回原文位置
3. **冲突保留**：矛盾信息并列保留两个版本，不擅自裁决
4. **事实驱动**：以原始资料中的事实为准，不添加主观推断；对 Go/TS 选型结论、质量门禁等仅做引用，不做业务/技术裁决

---

## 附录 B：解析 Skill

- `md`：Markdown 类项目规则、框架总览、代码知识库、发布说明、计划与评估文档（本次全部源资料均为 Markdown，使用 Read / Grep 提取标题与逐章内容）
- 图示（drawio / svg）：架构示意图，按约定登记为「图示，未逐章摘要」，不纳入逐章摘要范围
- 说明：本模板原枚举的 docx / pdf / pptx / xlsx 类型本次未出现；如后续补充此类格式资料，将分别对应 Word / PDF / PPT / Excel 解析能力
