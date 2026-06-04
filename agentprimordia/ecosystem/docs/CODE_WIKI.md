# AgentPrimordia Code Wiki

> 万物之源，智能之始 — 生产级 Go AI Agent 开发框架

**版本**: v0.1.0 | **语言**: Go 1.26+ | **许可**: Apache-2.0 | **CGO**: 零依赖

---

## 目录

- [1. 项目概述](#1-项目概述)
- [2. 整体架构](#2-整体架构)
- [3. 目录结构](#3-目录结构)
- [4. 核心模块详解](#4-核心模块详解)
  - [4.1 agent — ReAct Loop 引擎](#41-agent--react-loop-引擎)
  - [4.2 llm — LLM 抽象层](#42-llm--llm-抽象层)
  - [4.3 tools — 工具系统](#43-tools--工具系统)
  - [4.4 memory — 记忆存储](#44-memory--记忆存储)
  - [4.5 pool — 多 Agent 调度](#45-pool--多-agent-调度)
  - [4.6 security — 安全沙箱](#46-security--安全沙箱)
  - [4.7 events — 事件总线](#47-events--事件总线)
  - [4.8 metrics — 指标采集](#48-metrics--指标采集)
  - [4.9 concurrency — 并发原语](#49-concurrency--并发原语)
  - [4.10 persist — 检查点持久化](#410-persist--检查点持久化)
  - [4.11 orchestration — 编排引擎](#411-orchestration--编排引擎)
  - [4.12 其他模块](#412-其他模块)
- [5. 协议式微内核架构](#5-协议式微内核架构)
- [6. 插件生态](#6-插件生态)
- [7. 公共 API 层 (pkg)](#7-公共-api-层-pkg)
- [8. 依赖关系图](#8-依赖关系图)
- [9. 关键接口总览](#9-关键接口总览)
- [10. 错误码体系](#10-错误码体系)
- [11. 构建与运行](#11-构建与运行)
- [12. 测试体系](#12-测试体系)
- [13. 示例应用](#13-示例应用)
- [14. Provider 贡献指南](#14-provider-贡献指南)
- [15. TypeScript SDK](#15-typescript-sdk)

---

## 1. 项目概述

AgentPrimordia 是从 CodeCast 生产验证的 Agent 架构中提炼出的**通用 Go Agent 开发框架**。核心价值：

| 能力 | 说明 |
|------|------|
| ReAct Loop | Reason → Act → Observe 循环引擎，Agent 智能行为的核心 |
| 协议式微内核 | 14 个 Capable 接口 + 链式 API，能力按需组合、自动发现 |
| 多 Agent 编排 | Pipeline / Handoff / Parallel / DAG 工作流 |
| 工具系统 | 注册、权限、作用域、MCP 协议、插件生态 |
| 记忆存储 | SQLite + FTS5 + 向量检索 + RAG |
| LLM 抽象 | OpenAI / Anthropic / Gemini / Ollama / Azure / Cohere / Mistral / GLM / Qwen |
| Pool 调度 | 并发任务分发、重试、会话管理 |
| 安全沙箱 | ACL + Sandbox + 路径穿越检测 |
| 可观测性 | Prometheus 指标 + Hook 系统 + Event Bus + OpenTelemetry |

**设计原则**：接口驱动、协议发现、组合优于继承、弹性优先、零 CGO 依赖。

---

## 2. 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                    pkg/ (公共 API 重导出)                         │
│  用户只需 import "agentprimordia/pkg" 即可使用全部功能             │
├──────────┬──────────┬──────────┬──────────┬────────────────────┤
│  pool/   │ agent/   │security/ │orchestr- │  guardrail/        │
│ (调度器) │(ReAct引擎)│ (沙箱)   │ ation/   │  (护栏)            │
├──────────┼──────────┼──────────┼──────────┼────────────────────┤
│  tools/  │ memory/  │ events/  │ metrics/ │  otel/             │
│ (工具)   │ (记忆)   │ (事件)   │ (指标)   │  (可观测)          │
├──────────┴──────────┴──────────┴──────────┼────────────────────┤
│              llm/ (Provider 抽象层)          │ persist/           │
│  OpenAI | Anthropic | Gemini | Ollama |    │ (检查点)           │
│  Azure | Cohere | Mistral | GLM | Qwen     │                    │
├────────────────────────────────────────────┼────────────────────┤
│           concurrency/ (并发原语)           │ config/            │
│           FileLock | Scope 验证             │ (热重载)           │
├────────────────────────────────────────────┴────────────────────┤
│              ecosystem/ (生态层 — 与核心源码隔离)                  │
│  plugins/ (官方插件) | examples/ (示例) | docs/ (文档)           │
│  contributing/ (贡献指南) | templates/ (项目脚手架)               │
└─────────────────────────────────────────────────────────────────┘
```

**分层规则**：上层可依赖下层，下层不能依赖上层。`ecosystem/` 依赖核心框架，核心框架不依赖 `ecosystem/`。

---

## 3. 目录结构

```
agentprimordia/
├── cmd/
│   ├── ap/                    # CLI 工具 (ap 命令)
│   │   ├── main.go            # 入口
│   │   ├── init.go            # ap init 初始化项目
│   │   ├── run.go             # ap run 运行 Agent
│   │   ├── config.go          # 配置管理
│   │   ├── debug.go           # 调试可视化
│   │   ├── mcp.go             # MCP 子命令
│   │   ├── plugin.go          # 插件管理
│   │   ├── test.go            # 测试子命令
│   │   └── scaffold/          # 项目脚手架模板 (go:embed 镜像)
│   ├── admin/                 # Admin API 服务
│   └── example/               # 内置示例应用
│       ├── hello-agent/       # 最简 Agent
│       ├── multi-agent/       # 多 Agent 并发
│       ├── production/        # 生产级示例
│       └── demo/              # DemoLLM (无需 API Key)
├── internal/
│   ├── agent/                 # ReAct Loop 引擎 (核心)
│   │   ├── react_loop.go      # ReAct 循环主逻辑 + ReActConfig + ReActAgent
│   │   ├── types.go           # 核心类型定义 (Agent/Message/Response/ToolCall)
│   │   ├── capabilities.go    # 14 个 Capable 接口协议 (协议式微内核)
│   │   ├── capability_agent.go# CapabilityAgent 包装器 + 链式 API 实现
│   │   ├── chain_api.go       # ReActAgent.WithXxx() 链式入口方法
│   │   ├── hooks.go           # Hook 系统 (30+ 钩子点, 4 执行阶段)
│   │   ├── lifecycle.go       # 生命周期状态机
│   │   ├── bus.go             # Agent 间消息总线
│   │   ├── dag.go             # DAG 工作流引擎
│   │   ├── dag_builder.go     # DAG 声明式构建器
│   │   ├── dag_delegate.go    # Agent 委派节点
│   │   ├── collaboration.go   # Agent 协作模式
│   │   ├── group_chat.go      # 群聊模式
│   │   ├── orchestration.go   # 编排模式 (Pipeline/Handoff/Parallel)
│   │   ├── context_window.go  # 上下文窗口裁剪
│   │   ├── context_compress.go# 上下文压缩
│   │   ├── cost_tracker.go    # 成本追踪
│   │   ├── hitl.go            # 人机协作 (HITL)
│   │   ├── prompt.go          # 提示词模板
│   │   ├── multimodal.go      # 多模态支持
│   │   ├── trace.go           # 分布式追踪
│   │   ├── transport.go       # 跨进程传输层接口
│   │   ├── http_transport.go  # HTTP 传输层
│   │   ├── tcp_transport.go   # TCP 传输层
│   │   ├── eval.go            # 评估框架
│   │   ├── agent_tool.go      # Agent 作为工具
│   │   └── a2a/               # Agent-to-Agent 协议
│   │       ├── server.go      # A2A 服务端
│   │       ├── client.go      # A2A 客户端
│   │       ├── bridge.go      # Agent 桥接
│   │       ├── jsonrpc.go     # JSON-RPC 2.0
│   │       ├── sse.go         # Server-Sent Events
│   │       ├── auth.go        # 认证
│   │       └── types.go       # A2A 类型定义
│   ├── llm/                   # LLM 抽象层
│   │   ├── types.go           # Provider 接口 + 核心类型
│   │   ├── config.go          # 通用配置
│   │   ├── openai_provider.go # OpenAI
│   │   ├── anthropic_provider.go  # Anthropic Claude
│   │   ├── gemini_provider.go     # Google Gemini
│   │   ├── ollama_provider.go     # Ollama 本地模型
│   │   ├── azure_provider.go      # Azure OpenAI
│   │   ├── cohere_provider.go     # Cohere
│   │   ├── mistral_provider.go    # Mistral AI
│   │   ├── glm_provider.go        # 智谱 GLM
│   │   ├── qwen_provider.go       # 通义千问
│   │   ├── resilient.go       # 弹性 Provider (重试+降级+熔断)
│   │   ├── cache.go           # LLM 响应缓存 (内存)
│   │   ├── cache_enhanced.go  # 增强缓存 (指纹+语义)
│   │   ├── cache_sqlite.go    # SQLite 持久化缓存
│   │   ├── structured.go      # 结构化输出提取
│   │   ├── multimodal_provider.go # 多模态统一抽象
│   │   ├── mock_llm.go        # MockLLM (测试用)
│   │   ├── pricing.go         # 模型定价
│   │   ├── provider_helpers.go    # 通用辅助函数
│   │   ├── provider_template.go   # Provider 模板代码
│   │   └── provider_template_test.go # 测试模板
│   ├── tools/                 # 工具系统
│   │   ├── types.go           # Tool 接口 + Result
│   │   ├── registry.go        # 工具注册中心
│   │   ├── executor.go        # 工具执行器 (权限+超时+文件锁)
│   │   ├── scope.go           # 作用域权限策略
│   │   ├── plugin.go          # 插件系统 (ToolPlugin + PluginLoader)
│   │   ├── mcp.go             # MCP 协议客户端
│   │   ├── api_tools.go       # API 调用工具 (HTTP/Git/Search)
│   │   ├── data_tools.go      # 数据处理工具 (JSON/CSV/SQLite)
│   │   └── builtin/           # 内置工具
│   │       ├── filesystem.go  # 文件系统操作
│   │       ├── shell.go       # Shell 命令执行
│   │       ├── web.go         # HTTP 请求
│   │       ├── knowledge.go   # 知识库搜索
│   │       ├── toolkit.go     # 工具包 (DefaultToolkit/MinimalToolkit)
│   │       └── utilities.go   # 工具辅助函数
│   ├── memory/                # 记忆存储
│   │   ├── types.go           # Memory 接口组合
│   │   ├── memory.go          # Memory 主实现
│   │   ├── sqlite.go          # SQLite + FTS5 存储
│   │   ├── vector.go          # 内存向量存储
│   │   ├── rag.go             # RAG 混合检索
│   │   ├── rag_pipeline.go    # RAG 管道
│   │   ├── rag_generator.go   # RAG 端到端生成
│   │   ├── rag_rerank.go      # RAG 重排序
│   │   ├── summarizer.go      # LLM 摘要提取
│   │   ├── conversational_memory.go # 对话记忆
│   │   ├── milvus_provider.go # Milvus 向量库
│   │   └── qdrant_provider.go # Qdrant 向量库
│   ├── pool/                  # 多 Agent 调度
│   │   ├── types.go           # Pool 类型定义
│   │   ├── dispatcher.go      # 调度器主逻辑
│   │   └── events.go          # Pool 事件
│   ├── security/              # 安全沙箱
│   │   └── sandbox.go         # ACL + Sandbox
│   ├── events/                # 事件总线
│   │   └── bus.go             # 发布/订阅
│   ├── metrics/               # 指标采集
│   │   ├── metrics.go         # Prometheus 兼容指标
│   │   └── exporter.go        # 指标导出
│   ├── concurrency/           # 并发原语
│   │   └── filelock.go        # 文件锁 + Scope 验证
│   ├── persist/               # 检查点持久化
│   │   ├── checkpoint.go      # CheckpointStore 接口
│   │   └── sqlite_checkpoint.go # SQLite 实现
│   ├── orchestration/         # 编排引擎
│   │   ├── orchestrator.go    # 编排器 (Sequential/Parallel/DAG)
│   │   ├── workflow.go        # 工作流定义
│   │   ├── handoff.go         # Agent 交接
│   │   ├── collaboration.go   # 协作模式
│   │   └── visualizer.go      # 编排可视化
│   ├── config/                # 配置管理
│   │   └── hot_reload.go      # 热重载
│   ├── guardrail/             # 安全护栏
│   │   ├── engine.go          # 护栏引擎
│   │   ├── hook.go            # 护栏 Hook 集成
│   │   ├── injection_rule.go  # 注入检测规则
│   │   ├── output_rule.go     # 输出过滤规则
│   │   ├── pii_rule.go        # PII 检测规则
│   │   ├── topic_rule.go      # 主题约束规则
│   │   ├── trie_rule.go       # Trie 匹配规则
│   │   └── sanitizer.go       # 输入净化器
│   ├── otel/                  # OpenTelemetry 集成
│   ├── admin/                 # Admin API
│   ├── debugger/              # 调试器
│   └── prompt/                # 提示词工程
├── ecosystem/                 # 生态层（与核心源码隔离）
│   ├── plugins/               # 官方插件生态
│   │   ├── registry.json      # 插件索引 (6 个插件元数据)
│   │   ├── plugins.go         # LoadAll() 便捷加载函数
│   │   ├── http/              # HTTP 客户端插件
│   │   │   └── plugin.go
│   │   ├── sql/               # SQLite 数据库插件
│   │   │   └── plugin.go
│   │   ├── git/               # Git 版本控制插件
│   │   │   └── plugin.go
│   │   ├── json/              # JSON/CSV 数据处理插件
│   │   │   └── plugin.go
│   │   ├── email/             # 邮件发送插件 (net/smtp)
│   │   │   ├── plugin.go
│   │   │   └── plugin_test.go
│   │   └── kv/                # 键值存储插件 (SQLite 后端)
│   │       ├── plugin.go
│   │       └── plugin_test.go
│   ├── examples/              # Go 示例集合
│   │   ├── chain-api/         # 链式 API：最简 Agent
│   │   ├── chain-capable/     # 链式 API：渐进式添加能力
│   │   ├── chain-rag/         # 链式 API：RAG + 事件 + 指标
│   │   ├── chain-plugins/     # 链式 API：插件生态
│   │   ├── chain-production/  # 链式 API：生产级 Agent
│   │   ├── basic/             # 基础用法
│   │   ├── simple/            # 简单示例
│   │   ├── with-tools/        # 带工具的 Agent
│   │   ├── builtin-tools/     # 内置工具演示
│   │   ├── multi-agent/       # 多 Agent
│   │   ├── multi-agent-collab/# 多 Agent 协作
│   │   ├── memory-backends/   # 记忆后端
│   │   ├── multimodal-vision/ # 视觉多模态
│   │   ├── multimodal-advanced/# 高级多模态
│   │   ├── gemini-provider/   # Gemini Provider
│   │   ├── qwen-provider/     # 千问 Provider
│   │   ├── resilient-provider/# 弹性 Provider
│   │   └── debug-tools/       # 调试工具
│   ├── docs/                  # 文档
│   │   ├── CODE_WIKI.md       # Code Wiki（本文件）
│   │   ├── getting-started.md # 快速入门
│   │   ├── api-reference.md   # API 参考
│   │   ├── best-practices.md  # 最佳实践
│   │   ├── ap-guide.md        # CLI 指南
│   │   ├── go-ecosystem.md    # Go 生态
│   │   ├── vector-db-guide.md # 向量库指南
│   │   └── cookbook/          # 实战菜谱
│   ├── contributing/          # 贡献指南
│   │   ├── PROVIDER.md        # Provider 贡献指南
│   │   └── PLUGIN.md          # 插件贡献指南
│   └── templates/             # 项目脚手架模板
│       ├── basic/
│       ├── multi-agent/
│       └── with-tools/
├── pkg/                       # 公共 API 重导出
│   ├── agent.go               # Agent + Capable 接口导出
│   ├── llm.go                 # LLM 类型导出
│   ├── tools.go               # Tools + PluginLoader 导出
│   ├── memory.go              # Memory 类型导出
│   ├── pool.go                # Pool 类型导出
│   ├── errors.go              # 35 个结构化错误码
│   ├── options.go             # 函数选项
│   ├── types.go               # 通用类型
│   ├── version.go             # 版本号
│   ├── adapters.go            # 适配器 (Memory→Agent 接口桥接)
│   ├── concurrency.go         # 并发原语导出
│   ├── events.go              # 事件导出
│   ├── guardrail.go           # 护栏导出
│   ├── hooks.go               # Hook 导出
│   ├── lifecycle.go           # 生命周期导出
│   ├── metrics.go             # 指标导出
│   ├── otel.go                # OTel 导出
│   ├── persist.go             # 持久化导出
│   ├── pipeline.go            # Pipeline 导出
│   └── security.go            # 安全导出
├── bench/                     # 性能基准测试
├── operator/                  # Kubernetes Operator
├── sdk/typescript/            # TypeScript SDK
├── deploy/grafana/            # Grafana 仪表盘
├── Makefile                   # 构建脚本
├── Dockerfile                 # Docker 构建
├── docker-compose.yml         # Docker Compose
└── go.mod                     # Go 模块定义
```

---

## 4. 核心模块详解

### 4.1 agent — ReAct Loop 引擎

**路径**: `internal/agent/`

Agent 模块是框架的核心，实现了 ReAct（Reasoning + Acting）循环引擎，并引入了协议式微内核架构。

#### 核心接口

```go
// Agent 是所有 Agent 实现的核心接口
type Agent interface {
    Run(ctx context.Context, input Message) (*Response, error)
    StreamRun(ctx context.Context, input Message) (<-chan StreamEvent, error)
    Stop()
    Stats() AgentStats
    Name() string
}
```

#### ReActConfig 配置结构

配置分为**核心配置（必填）**和**可选能力（推荐链式 API）**两组：

```go
type ReActConfig struct {
    // ===== 核心配置（必填） =====
    Name         string          // Agent 名称
    SystemPrompt string          // 系统提示词
    Model        llm.Provider    // LLM 提供者
    MaxTurns     int             // 最大推理轮次
    Temperature  float64         // 温度参数
    SessionID    string          // 会话 ID

    // ===== 可选能力（推荐使用链式 API 注入） =====
    // Deprecated: 使用 .WithXxx() 链式方法注入
    Toolkit         *tools.Registry
    Memory          MemoryStore
    EventPublisher  EventPublisher
    Metrics         MetricsRecorder
    ContextWindow   ContextWindowStrategy
    CheckpointStore persist.CheckpointStore
    RAG             *RAGConfig
    Hooks           Hooks
    Summarizer      memory.SummaryExtractor
    FileScope       []string
    HITL            *HITLConfig
    CostTracker     *CostTracker
    Tracer          Tracer
    Cache           llm.LLMCache
}
```

#### ReActAgent 结构

```go
type ReActAgent struct {
    config    ReActConfig
    lifecycle *Lifecycle
    hooks     Hooks
    logger    *slog.Logger
    stats     AgentStats
    hitlMgr   *HITLManager
    self      Agent  // 自引用，指向最外层包装器（协议式微内核）
}
```

`self` 字段是协议式微内核的关键：默认指向自身，WithXxx 链式调用时更新为 CapabilityAgent。引擎通过 `a.self.(XxxCapable)` 检测能力。

#### ReAct 循环流程

```
用户输入 → 构建系统提示词 → [循环开始]
  → RAG 上下文注入（可选，通过 RAGCapable 发现）
  → 上下文窗口裁剪（可选，通过 ContextWindowCapable 发现）
  → LLM 推理 (Complete / CallTools)
  → 有工具调用？ → 执行工具 → 结果加入历史 → 回到循环
  → 无工具调用？ → 返回最终 Response
[循环结束] → 超出 MaxTurns → 返回错误
```

#### 接口发现辅助方法

引擎通过 10 个 `getXxx()` 辅助方法访问能力，每个方法**优先通过 Capable 接口发现，回退到 config 字段**：

| 方法 | Capable 接口 | 回退字段 |
|------|-------------|---------|
| `getMemoryStore()` | MemoryCapable | config.Memory |
| `getRAGConfig()` | RAGCapable | config.RAG |
| `getEventPublisher()` | EventCapable | config.EventPublisher |
| `getMetricsRecorder()` | MetricsCapable | config.Metrics |
| `getTracer()` | TraceCapable | config.Tracer |
| `getCostTracker()` | CostCapable | config.CostTracker |
| `getCheckpointStore()` | CheckpointCapable | config.CheckpointStore |
| `getContextWindowStrategy()` | ContextWindowCapable | config.ContextWindow |
| `getSummarizer()` | SummarizerCapable | config.Summarizer |
| `getFileScope()` | FileScopeCapable | config.FileScope |

#### 关键类型

| 类型 | 文件 | 说明 |
|------|------|------|
| `ReActAgent` | `react_loop.go` | ReAct 循环 Agent 实现，核心引擎 |
| `CapabilityAgent` | `capability_agent.go` | 可组合能力的 Agent 包装器，实现所有 Capable 接口 |
| `Message` | `types.go` | 对话消息，含角色、内容、多模态片段、工具调用 |
| `Response` | `types.go` | Agent 响应，含内容、工具调用、用量、指标 |
| `Thought` | `types.go` | LLM 推理输出，含文本和工具调用列表 |
| `ToolCall` | `types.go` | LLM 发起的工具调用请求 |
| `StreamEvent` | `types.go` | 流式输出事件 (token/thought/tool_call/tool_result/complete/error) |
| `AgentStatus` | `types.go` | Agent 状态枚举 |

#### 生命周期管理 (`lifecycle.go`)

状态机管理合法状态转换：

```
Idle → Running → Paused → Running
                → WaitingForInput → Running
                → Completed → Idle
                → Failed → Idle
                → Cancelled → Idle
```

特性：状态转换守卫、状态钩子、超时自动失败、优雅关闭。

#### Hook 系统 (`hooks.go`)

30+ 钩子点，4 个执行阶段：

| 阶段 | 说明 | 典型用途 |
|------|------|---------|
| `PhaseValidation` | 护栏阶段 | Guardrails 安全检查 |
| `PhasePreProcessing` | 预处理 | 日志、指标收集 |
| `PhaseExecution` | 执行 | 业务逻辑 |
| `PhasePostProcessing` | 后处理 | 通知、缓存 |

#### DAG 工作流 (`dag.go`, `dag_builder.go`)

DAG 工作流引擎：拓扑排序执行、并行无依赖节点、条件分支、节点重试、Agent 委派、子工作流嵌套。

---

### 4.2 llm — LLM 抽象层

**路径**: `internal/llm/`

#### 核心接口

```go
type Provider interface {
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    Stream(ctx context.Context, req *CompletionRequest) (<-chan Chunk, error)
    CallTools(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error)
    Info() ModelInfo
}

type Embedder interface {
    Embeddings(ctx context.Context, texts []string) ([][]float32, error)
}
```

#### Provider 实现

| Provider | 文件 | 说明 |
|----------|------|------|
| `OpenAIProvider` | `openai_provider.go` | GPT-4o / GPT-4o-mini，及所有 OpenAI 兼容 API |
| `AnthropicProvider` | `anthropic_provider.go` | Claude 系列 |
| `GeminiProvider` | `gemini_provider.go` | Google Gemini |
| `OllamaProvider` | `ollama_provider.go` | 本地模型 (Llama, Qwen 等) |
| `AzureOpenAIProvider` | `azure_provider.go` | Azure 部署的 OpenAI |
| `CohereProvider` | `cohere_provider.go` | Cohere v2 API |
| `MistralProvider` | `mistral_provider.go` | Mistral AI |
| `GLMProvider` | `glm_provider.go` | 智谱 GLM |
| `QwenProvider` | `qwen_provider.go` | 通义千问 |
| `ResilientProvider` | `resilient.go` | 弹性包装器 (重试+降级+熔断) |

#### ResilientProvider 三重保护

| 机制 | 配置 | 默认值 |
|------|------|--------|
| 重试 | `MaxRetries` + `RetryBackoff` + `MaxBackoff` | 3 次, 500ms, 10s |
| 降级 | `AddFallback(provider)` | 无默认 |
| 熔断 | `CircuitThreshold` + `CircuitRecoverAfter` | 5 次失败, 30s 恢复 |

#### 缓存系统

| 缓存 | 文件 | 说明 |
|------|------|------|
| `InMemoryCache` | `cache.go` | 基于向量相似度的语义缓存 |
| `FingerprintCache` | `cache_enhanced.go` | 基于 Prompt 指纹的精确匹配缓存 |
| `HybridCache` | `cache_enhanced.go` | 混合缓存：先精确匹配再语义匹配 |
| SQLite 持久化缓存 | `cache_sqlite.go` | 基于 SQLite 的持久化缓存 |

#### 结构化输出 (`structured.go`)

从 LLM 响应中提取结构化数据：`StructuredExtractor`、`SchemaFromStruct`、预定义模板。

#### 贡献支持

| 文件 | 说明 |
|------|------|
| `CONTRIBUTING.md` | Provider 贡献指南（9 章节） |
| `provider_template.go` | 可编译的 Provider 模板代码 |
| `provider_template_test.go` | 测试模板 |

---

### 4.3 tools — 工具系统

**路径**: `internal/tools/`

#### 核心接口

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage
    Execute(ctx context.Context, args json.RawMessage) (*Result, error)
}

type ToolPlugin interface {
    Name() string
    Version() string
    Tools() []Tool
    Init(config map[string]any) error
    Close() error
}
```

#### 关键组件

| 组件 | 文件 | 说明 |
|------|------|------|
| `Registry` | `registry.go` | 工具注册中心：注册、查找、权限管理、分类 |
| `Executor` | `executor.go` | 工具执行器：权限检查、超时、文件锁、批量执行 |
| `FileScopePolicy` | `scope.go` | 基于文件路径的作用域权限策略 |
| `PluginLoader` | `plugin.go` | 插件加载器：Load/Unload/List/Get |
| `MCPClient` | `mcp.go` | MCP 协议客户端 |

#### PluginLoader 方法

| 方法 | 说明 |
|------|------|
| `Load(plugin)` | 加载插件（无配置） |
| `LoadWithConfig(plugin, config)` | 加载插件（带配置） |
| `Unload(name)` | 卸载插件 |
| `List()` | 列出已加载插件信息 |
| `Get(name)` | 获取指定插件 |

#### 内置工具 (`builtin/`)

| 工具 | 说明 |
|------|------|
| `FileSystem` | 文件读写、目录列表、搜索 |
| `Shell` | Shell 命令执行，支持超时和白名单 |
| `Web` | HTTP GET/POST 请求 |
| `Knowledge` | RAG on_demand 模式的知识检索 |

工具包：`DefaultToolkit(root)` = FileSystem + Shell + Web，`MinimalToolkit(root)` = FileSystem + Shell

---

### 4.4 memory — 记忆存储

**路径**: `internal/memory/`

#### 接口组合

```go
type Memory interface {
    MemoryReader     // Get, Search, List, Count, Stats
    MemoryWriter     // Add, Delete, UpdateSummary, SetImportance
    MemorySearcher   // SearchAdvanced, SearchByTag, GetImportant, GetTimeline
    MemoryLifecycle  // Close, CleanupExpired, ClearAll
    MemoryExporter   // ExportMemories, ImportMemories
    MemoryQuery      // GetMemoriesByTag/Session/Important/Timeline
    MemoryToolUse    // RecordToolUse
}
```

#### 存储后端

| 后端 | 说明 |
|------|------|
| `SQLiteStore` | SQLite + FTS5 全文搜索，支持自动清理 |
| `VectorStore` | 内存向量存储，余弦相似度搜索 |
| `MilvusProvider` | Milvus 分布式向量库 |
| `QdrantProvider` | Qdrant 向量数据库 |

#### RAG 系统

| 组件 | 说明 |
|------|------|
| `RAGStore` | 混合 RAG 检索：FTS + Vector 加权融合 |
| `RAGPipeline` | RAG 管道：检索→重排序→上下文组装 |
| `RetrievalAugmentedGenerator` | 端到端 RAG：检索→上下文组装→LLM 生成 |
| `Summarizer` | 基于 LLM 的摘要和标签提取 |

---

### 4.5 pool — 多 Agent 调度

**路径**: `internal/pool/`

#### 核心类型

| 类型 | 说明 |
|------|------|
| `Pool` | 多 Agent 并发调度器 |
| `PoolConfig` | 配置：最大并发数、超时、重试策略 |
| `TaskConfig` | 任务配置：ID、标题、提示词、会话、作用域 |
| `TaskResult` | 任务结果：响应、错误、耗时、状态 |
| `AgentFactory` | Agent 工厂函数 |

#### 调度流程

```
Pool.Dispatch(ctx, []TaskConfig)
  → goroutine 并发执行
    → 获取信号量 (并发控制)
    → createAgentForTask
    → Agent.Run(ctx, UserMessage(prompt))
    → 失败时按 RetryPolicy 重试
  → WaitGroup 等待全部完成
  → 返回 []TaskResult
```

---

### 4.6 security — 安全沙箱

**路径**: `internal/security/`

- **ACL**：访问控制列表，支持 Allow/Deny/Check，访问级别 Read/Write/Execute/All
- **Sandbox**：命令白名单/黑名单、Shell 元字符检测、路径穿越检测、ACL 访问控制

---

### 4.7 events — 事件总线

**路径**: `internal/events/`

发布/订阅模式：按类型订阅、通配符订阅、同步/异步发布、通道满自动丢弃。

---

### 4.8 metrics — 指标采集

**路径**: `internal/metrics/`

Prometheus 兼容指标：LLM 调用/错误计数、工具调用/错误计数、活跃 Agent 数、延迟分布直方图、Token 用量统计。

---

### 4.9 concurrency — 并发原语

**路径**: `internal/concurrency/`

- **FileLockManager**：基于路径的文件锁，阻塞/非阻塞获取，引用计数
- **Scope 验证**：全局写入冲突检测、作用域重叠检测

---

### 4.10 persist — 检查点持久化

**路径**: `internal/persist/`

```go
type CheckpointStore interface {
    Save(ctx context.Context, state *AgentState) error
    Load(ctx context.Context, agentID string) (*AgentState, error)
    List(ctx context.Context, sessionID string) ([]*AgentState, error)
    Delete(ctx context.Context, agentID string) error
}
```

SQLite 实现：`SQLiteCheckpointStore`

---

### 4.11 orchestration — 编排引擎

**路径**: `internal/orchestration/`

三种编排模式：Sequential（顺序）、Parallel（并行）、DAG（有向无环图）。

组件：Orchestrator、Workflow、Handoff、Collaboration、Visualizer。

---

### 4.12 其他模块

| 模块 | 路径 | 说明 |
|------|------|------|
| `config` | `internal/config/` | 配置热重载 |
| `guardrail` | `internal/guardrail/` | 安全护栏：注入检测、输出过滤、PII 检测、主题约束 |
| `otel` | `internal/otel/` | OpenTelemetry 集成 |
| `admin` | `internal/admin/` | Admin HTTP API |
| `debugger` | `internal/debugger/` | 调试 HTTP 服务 + 可视化 |
| `prompt` | `internal/prompt/` | 提示词模板、解析、Few-shot |

---

## 5. 协议式微内核架构

### 设计理念

借鉴 Go 标准库的**接口发现**模式（`io.Reader`、`http.Handler`），让能力成为接口而非配置字段：

> **不需要声明"我要用 RAG"，只要你的 Agent 实现了 `RAGCapable` 接口，ReAct 引擎就自动启用 RAG。**

### 14 个 Capable 接口

定义在 `internal/agent/capabilities.go`：

| 接口 | 方法 | 说明 |
|------|------|------|
| `MemoryCapable` | `GetMemoryStore() MemoryStore` | 记忆存储 |
| `RAGCapable` | `GetRAGConfig() *RAGConfig` | RAG 检索 |
| `HITLCapable` | `GetHITLConfig() *HITLConfig` | 人机协作 |
| `HookCapable` | `GetHooks() Hooks` | Hook 系统 |
| `TraceCapable` | `GetTracer() Tracer` | 分布式追踪 |
| `CostCapable` | `GetCostTracker() *CostTracker` | 成本追踪 |
| `ContextWindowCapable` | `GetContextWindowStrategy() ContextWindowStrategy` | 上下文裁剪 |
| `EventCapable` | `GetEventPublisher() EventPublisher` | 事件发布 |
| `MetricsCapable` | `GetMetricsRecorder() MetricsRecorder` | 指标记录 |
| `CheckpointCapable` | `GetCheckpointStore() persist.CheckpointStore` | 检查点 |
| `SummarizerCapable` | `GetSummarizer() memory.SummaryExtractor` | 摘要提取 |
| `FileScopeCapable` | `GetFileScope() []string` | 文件作用域 |
| `CacheCapable` | `GetCache() llm.LLMCache` | LLM 缓存 |
| `ToolingCapable` | (预留) | 工具能力 |

### CapabilityAgent 包装器

定义在 `internal/agent/capability_agent.go`：

- 实现 `Agent` 接口（委托给内部 ReActAgent）
- 实现所有 14 个 Capable 接口（未注入的返回 nil）
- 提供 14 个 `WithXxx()` 链式方法

### 链式 API

定义在 `internal/agent/chain_api.go`：

```go
// 最简 Agent（4 个必填字段）
agent := ap.NewReActAgent(ap.ReActConfig{
    Name: "my-agent", Model: provider, MaxTurns: 10,
})

// 渐进式添加能力
agent.WithMemory(mem).WithRAG(ragCfg).WithHooks(hooks)

// 生产级 Agent
agent.
    WithToolkit(registry).
    WithMemory(ap.NewMemoryAdapter(mem)).
    WithHooks(hooks).
    WithCostTracker(ct).
    WithEvents(ep).
    WithMetrics(mr).
    WithFileScope([]string{"./src"})
```

### 自引用模式

```go
// ReActAgent.self 默认指向自身
a.initSelf()  // a.self = a

// WithXxx 时更新为 CapabilityAgent
a.self = capAgent  // 引擎通过 a.self.(XxxCapable) 发现能力
```

### 第三方可扩展

任何用户都可以定义自己的 Capable 接口，无需修改框架核心：

```go
type AuditCapable interface {
    Auditor() Auditor
}

// 在 Hook 中检测
hooks.Register(HookAfterTool, func(ctx context.Context, hctx *HookContext) error {
    if audit, ok := agent.(AuditCapable); ok {
        audit.Auditor().Record(hctx.ToolCall)
    }
    return nil
})
```

---

## 6. 插件生态

**路径**: `ecosystem/plugins/`

### 插件索引

`ecosystem/plugins/registry.json` 包含 6 个官方插件的元数据：

| 插件 | 版本 | 分类 | 工具 | 类型 |
|------|------|------|------|------|
| http | 0.1.0 | network | `http_client` | 封装现有 |
| sql | 0.1.0 | database | `sqlite_processor` | 封装现有 |
| git | 0.1.0 | vcs | `git_tool` | 封装现有 |
| json | 0.1.0 | data | `json_processor` + `csv_processor` | 封装现有 |
| email | 0.1.0 | communication | `email_sender` | **全新** (net/smtp) |
| kv | 0.1.0 | database | `kv_store` | **全新** (SQLite) |

### 插件使用

```go
registry := ap.NewToolRegistry()
loader := ap.NewPluginLoader(registry)

// 无配置插件
loader.Load(jsonplugin.New())

// 带配置插件
loader.LoadWithConfig(kvplugin.New(), map[string]any{"db_path": "test.db"})
loader.LoadWithConfig(emailplugin.New(), map[string]any{
    "smtp_host": "smtp.example.com", "smtp_port": "587",
    "smtp_username": "user@example.com", "smtp_password": "pass",
})

// 一键加载全部官方插件
plugins.LoadAll(registry, configs)
```

### email 插件

基于 `net/smtp` 的邮件发送工具：
- 支持 to/cc/bcc
- 支持 text 和 html 内容类型
- Init 读取 SMTP 配置（host/port/username/password/from）
- 实现 `CategorizedTool` 接口（Category: "communication"）

### kv 插件

基于 SQLite 的键值存储工具：
- 支持 get/set/delete/list 操作
- 使用 UPSERT 语义
- 自动创建 `ap_kv_store` 表
- Init 读取 `db_path` 配置

---

## 7. 公共 API 层 (pkg)

**路径**: `pkg/`

用户唯一需要导入的包：

```go
import ap "agentprimordia/pkg"
```

| 文件 | 导出内容 |
|------|---------|
| `agent.go` | Agent, ReActAgent, ReActConfig, Message, Response, CapabilityAgent, 14 个 Capable 接口 |
| `llm.go` | Provider, OpenAI/Anthropic/Gemini/Ollama/Azure/Cohere/Mistral/GLM/Qwen Provider, 缓存, 结构化输出 |
| `tools.go` | Tool, Registry, Executor, PluginLoader, ToolPlugin, 内置工具, MCP |
| `memory.go` | Memory, Episode, SQLiteStore, VectorStore, RAGStore, Summarizer |
| `pool.go` | Pool, PoolConfig, TaskConfig, TaskResult, AgentFactory |
| `errors.go` | 35 个结构化错误码 + `GetErrorCode()` |
| `options.go` | 函数选项：WithTimeout, WithMaxTurns, WithTemperature 等 |
| `adapters.go` | 适配器：Memory→Agent 接口桥接 |
| `version.go` | `Version = "0.1.0"` |

---

## 8. 依赖关系图

```
                    ┌─────────┐
                    │  pkg/   │  (公共 API 重导出)
                    └────┬────┘
                         │
        ┌────────┬───────┼───────┬────────┐
        │        │       │       │        │
   ┌────▼──┐ ┌──▼───┐ ┌─▼──┐ ┌─▼──┐ ┌──▼───┐
   │ pool/ │ │agent/│ │sec/│ │evt/│ │guard/ │
   └──┬─┬──┘ └─┬─┬──┘ └─┬──┘ └────┘ └──┬───┘
      │ │      │ │       │              │
      │ │   ┌──┘ └──┐    │         ┌────┘
      │ │   │       │    │         │
   ┌──▼─▼───▼──┐ ┌─▼────▼──┐ ┌───▼────┐
   │  tools/   │ │ memory/ │ │guardrail│
   └────┬──────┘ └────┬────┘ └────────┘
        │             │
   ┌────▼─────┐  ┌────▼────┐
   │concurren/│  │  llm/   │  (最底层)
   └──────────┘  └────┬────┘
                     │
                ┌────▼────┐
                │ persist/ │
                └─────────┘

   ┌─────────────────────────────────────┐
   │    ecosystem/plugins/ (生态插件层)    │
   │  http | sql | git | json | email | kv│
   │  依赖 tools/ + 标准库               │
   ├─────────────────────────────────────┤
   │ ecosystem/examples/ | ecosystem/docs│
   │ ecosystem/contributing/ | templates │
   │ 依赖 pkg/ (公共 API)               │
   └─────────────────────────────────────┘
```

**外部依赖**：仅 `modernc.org/sqlite`（纯 Go SQLite 驱动，无 CGO）

---

## 9. 关键接口总览

| 接口 | 模块 | 说明 |
|------|------|------|
| `Agent` | agent | Agent 核心接口：Run/StreamRun/Stop/Stats/Name |
| `llm.Provider` | llm | LLM 提供者：Complete/Stream/CallTools/Info |
| `llm.Embedder` | llm | 嵌入接口：Embeddings |
| `Tool` | tools | 工具接口：Name/Description/Parameters/Execute |
| `ToolPlugin` | tools | 插件接口：Name/Version/Tools/Init/Close |
| `CategorizedTool` | tools | 分类工具接口：Category |
| `Memory` | memory | 记忆组合接口 (7 个子接口) |
| `MemoryStore` | agent | Agent 所需的记忆接口 (仅 Add) |
| `RAGProvider` | agent | RAG 检索接口：Search |
| `CheckpointStore` | persist | 检查点存储：Save/Load/List/Delete |
| `EventPublisher` | agent | 事件发布：PublishAsync |
| `MetricsRecorder` | agent | 指标记录：RecordLLMCall/RecordToolCall/... |
| `MessageBus` | agent | 消息总线：Send/Broadcast/Subscribe |
| `Transport` | agent | 传输层：Send/Receive/Start/Stop |
| `ContextWindowStrategy` | agent | 上下文窗口裁剪：Trim |
| `llm.LLMCache` | llm | LLM 缓存：Get/Set/Stats/Clear |
| `Hooks` | agent | Hook 管理器：Register/Fire |
| `Lifecycle` | agent | 生命周期：SetStatus/IsGracefulShutdown |
| `Tracer` | agent | 追踪器：Start/Span |
| **14 个 Capable 接口** | agent | 协议式微内核能力接口 |

---

## 10. 错误码体系

35 个结构化错误码，通过 `GetErrorCode(err)` 提取：

| 模块 | 错误码 | 说明 |
|------|--------|------|
| **Agent** | `AGENT_001` | Agent 已被停止 |
| | `AGENT_002` | Agent 已在运行中 |
| | `AGENT_003` | 超出最大推理轮次 |
| | `AGENT_004` | 未配置工具包 |
| **Tool** | `TOOL_001` | 工具未注册 |
| | `TOOL_002` | 工具执行失败 |
| | `TOOL_003` | 工具配置无效 |
| | `TOOL_004` | 工具确认被拒绝 |
| **LLM** | `LLM_001` | LLM 调用失败 |
| | `LLM_002` | 操作不被支持 |
| | `LLM_003` | 熔断器已打开 |
| | `LLM_004` | API Key 未提供 |
| | `LLM_005` | 空响应 |
| | `LLM_006` | 响应解析失败 |
| | `LLM_007` | 重试耗尽 |
| | `LLM_008` | 所有降级均失败 |
| **Pool** | `POOL_001` | Pool 已达最大容量 |
| | `POOL_002` | 任务未找到 |
| | `POOL_003` | 操作超时 |
| **Context** | `CTX_001` | 上下文已取消 |
| **Memory** | `MEM_001` ~ `MEM_008` | 记忆相关错误 |
| **Security** | `SEC_001` ~ `SEC_004` | 安全相关错误 |
| **Event** | `EVT_001` | 事件总线已关闭 |
| **Persist** | `PST_001` | 检查点未找到 |
| **Concurrency** | `CON_001` ~ `CON_002` | 并发冲突错误 |

---

## 11. 构建与运行

### 前置条件

- Go 1.26+
- 无需 CGO

### Makefile 命令

| 命令 | 说明 |
|------|------|
| `make build` | 编译所有包 |
| `make test` | 运行测试 (含竞态检测和覆盖率) |
| `make lint` | 运行 golangci-lint |
| `make clean` | 清理构建产物 |
| `make run-hello` | 运行 Hello Agent 示例 (`ecosystem/examples/chain-api/`) |
| `make run-multi` | 运行 Multi-Agent 示例 (`ecosystem/examples/multi-agent/`) |
| `make run-production` | 运行 Production 示例 (`ecosystem/examples/chain-production/`) |
| `make docker-build` | Docker 构建 |
| `make docker-run` | Docker Compose 启动 |

### CLI 工具

```bash
go install ./cmd/ap

ap init my-agent    # 初始化项目
ap run              # 运行 Agent
ap debug            # 调试
ap mcp              # MCP 管理
ap plugin           # 插件管理
```

---

## 12. 测试体系

### 策略

- **TDD 强制**：Red → Green → Refactor
- `t.TempDir()` 创建临时文件
- `httptest.Server` 模拟网络
- `WithInMemory()` 创建内存数据库
- `MockLLM` 用于 agent/pool 层测试
- `DemoLLM` 用于示例应用（无需 API Key）

### 命令

```bash
go test -v -race -coverprofile=coverage.out ./...
go test -bench=. -benchmem ./bench/suite/
```

---

## 13. 示例应用

**路径**: `ecosystem/examples/`

### 链式 API 示例（推荐）

| 示例 | 说明 |
|------|------|
| `chain-api/` | 最简 Agent — 4 个必填字段 |
| `chain-capable/` | 渐进式添加能力 — 工具 + 记忆 + Hook |
| `chain-rag/` | RAG + 事件 + 指标 |
| `chain-plugins/` | 插件生态 — JSON + KV + Email |
| `chain-production/` | 生产级 Agent — 全部能力 |

### 传统示例

| 示例 | 说明 |
|------|------|
| `basic/` | 基础用法 |
| `with-tools/` | 带工具的 Agent |
| `builtin-tools/` | 内置工具演示 |
| `multi-agent/` | 多 Agent 并发 |
| `multi-agent-collab/` | 多 Agent 协作 |
| `memory-backends/` | 记忆后端对比 |
| `multimodal-vision/` | 视觉多模态 |
| `resilient-provider/` | 弹性 Provider |
| `gemini-provider/` | Gemini Provider |
| `qwen-provider/` | 千问 Provider |

---

## 14. Provider 贡献指南

**路径**: `ecosystem/contributing/PROVIDER.md`（原 `internal/llm/CONTRIBUTING.md` 已迁移）

9 章节中文指南：

1. **概述** — 为什么要贡献 Provider
2. **快速开始** — 5 步创建新 Provider
3. **Provider 接口规范** — Complete/Stream/CallTools/Info 详解
4. **可选接口** — Embedder、MultimodalProvider
5. **配置模式** — 使用通用 Config 结构体
6. **测试要求** — 必须通过的测试
7. **命名规范** — 文件命名、结构体命名
8. **提交流程** — PR 检查清单
9. **常见问题** — 8 个 FAQ

模板代码：`internal/llm/provider_template.go`（可编译，TODO 标记清晰）

插件贡献指南：`ecosystem/contributing/PLUGIN.md`

---

## 15. TypeScript SDK

**路径**: `sdk/typescript/`

镜像 Go 框架核心能力的 TypeScript SDK：

```
sdk/typescript/src/
├── agent/react-loop.ts    # ReAct 循环
├── events/bus.ts          # 事件总线
├── llm/                   # Provider (OpenAI/Resilient)
├── memory/                # 记忆存储 + 向量
├── metrics/collector.ts   # 指标收集
├── pool/agent-pool.ts     # Agent Pool
├── security/sandbox.ts    # 安全沙箱
└── tools/                 # 工具注册 + 作用域
```

---

> **AgentPrimordia** — 万物之源，智能之始
