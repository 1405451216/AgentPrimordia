<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&style=for-the-badge" alt="Go Version">
  <img src="https://img.shields.io/badge/TypeScript-5.0+-3178C6?logo=typescript&style=for-the-badge" alt="TypeScript">
  <img src="https://img.shields.io/badge/License-Apache--2.0-blue?style=for-the-badge" alt="License">
  <img src="https://img.shields.io/badge/Zero_CGO-✓-brightgreen?style=for-the-badge" alt="Zero CGO">
  <img src="https://img.shields.io/badge/version-v1.0.0-blue?style=for-the-badge" alt="Version">
</p>

<h1 align="center">⚡ AgentPrimordia</h1>

<p align="center">
  <strong>万物之源，智能之始</strong><br>
  生产级 AI Agent 开发框架 — Go + TypeScript 双语言支持
</p>

<p align="center">
  <a href="#-quick-start">快速开始</a> · <a href="#-核心特性">核心特性</a> · <a href="#-架构设计">架构设计</a> · <a href="./DEVELOPMENT.md">开发文档</a> · <a href="#-typescript-sdk">TypeScript SDK</a>
</p>

---

## 💡 为什么选择 AgentPrimordia？

构建 AI Agent 应用时，你是否遇到这些痛点？

| 痛点 | AgentPrimordia 的答案 |
|------|----------------------|
| 🧩 LLM Provider 耦合，换个模型就要改代码 | **统一 Provider 接口**，10+ 内置 Provider，一行代码切换 |
| 💥 API 调用不稳定，偶尔超时或限流 | **ResilientProvider**：自动重试 + 降级链 + 熔断器 |
| 🔧 工具系统从零搭建，每个工具都要写胶水代码 | **Plugin Tool 接口**，7 个内置工具 + 权限确认机制 |
| 🧠 Agent 没有记忆，每次对话从零开始 | **Episodic Memory**：SQLite + FTS5 + 向量检索 + RAG |
| 🔄 多任务编排复杂，手动管理 goroutine | **Pool 调度器**：并发控制 + 超时 + 重试 + 事件通知 |
| 🔒 Agent 执行危险操作没有防线 | **Sandbox + ACL**：命令白名单 + 路径穿越检测 + 访问控制 |
| 📊 生产环境缺乏可观测性 | **Prometheus 指标 + Hook 系统 + Event Bus** |

**一句话**：AgentPrimordia 让你专注 Agent 的业务逻辑，基础设施我们包了。

---

## ✨ 核心特性

### 🧠 ReAct Loop 引擎

完整的 **Reason → Act → Observe** 循环，这是 Agent 智能行为的核心：

```
用户提问 → LLM 推理 → 选择工具 → 执行 → 观察结果 → 继续推理 → 最终回答
```

- 可配置最大轮数（默认 50）
- 支持流式输出（逐 token 返回）
- 上下文窗口自动裁剪
- 检查点保存与恢复

### 🛡️ 弹性调用（ResilientProvider）

生产环境的 LLM 调用从不稳定，AgentPrimordia 内建三重保护：

| 机制 | 说明 |
|------|------|
| **重试** | 指数退避重试（默认 3 次，最大 10s 退避） |
| **降级** | 主 Provider 失败后自动切换 Fallback Provider |
| **熔断** | 连续 5 次失败后熔断，30s 后半开探测恢复 |

```go
primary := NewOpenAIProvider(Config{APIKey: "...", Model: "gpt-4o"})
fallback := NewOpenAIProvider(Config{APIKey: "...", Model: "deepseek-chat", BaseURL: "..."})

resilient := NewResilientProvider(primary, DefaultResilientConfig())
resilient.AddFallback(fallback)  // 主模型挂了自动切备用
```

### 🧩 多 LLM Provider 支持

| Provider | 用途 |
|----------|------|
| **OpenAI** | GPT-4o / GPT-4o-mini，及所有 OpenAI 兼容 API |
| **Anthropic** | Claude 系列 |
| **Gemini** | Google Gemini |
| **Ollama** | 本地模型（Llama、Qwen 等） |
| **Azure OpenAI** | Azure 部署的 OpenAI 模型 |
| **Qwen** | 通义千问（DashScope 兼容） |
| **GLM** | 智谱 GLM |
| **Cohere** | Cohere v2 API |
| **Mistral** | Mistral AI |
| **DeepSeek** | 通过 OpenAI 兼容模式使用 |
| **ResilientProvider** | 任意 Provider 的弹性包装 |

### 🔧 工具系统

7 个开箱即用的内置工具：

| 工具 | 能力 |
|------|------|
| **FileSystem** | 文件/目录的读写、搜索、列表 |
| **Shell** | 执行 Shell 命令 |
| **Web** | HTTP GET/POST 请求 |
| **API** | REST API 调用（白名单、超时） |
| **Database** | SQL 数据库查询 |
| **CodeExecution** | 代码执行（沙箱） |
| **Knowledge** | RAG on_demand 模式的知识检索 |

自定义工具只需实现 4 个方法：

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage
    Execute(ctx context.Context, args json.RawMessage) (*Result, error)
}
```

### 🧠 记忆系统

**三层记忆架构**：

| 层级 | 技术 | 能力 |
|------|------|------|
| **Episodic Memory** | SQLite + FTS5 | 全文搜索、标签过滤、重要性评分、时间线视图 |
| **Vector Store** | 内存 + 余弦相似度 | 语义搜索 |
| **RAG** | FTS + Vector 混合检索 | 加权融合（FTS×0.4 + Vec×0.6），知识库上下文注入 |

```go
// 混合检索：关键词 + 语义双通道
results, _ := ragStore.HybridSearch(ctx, "Go 并发模型", 5)
// 自动融合 FTS 和向量搜索结果，按相关度排序
```

### 🔄 多 Agent 并发调度

```go
pool := NewPool(PoolConfig{MaxConcurrency: 5})
results, _ := pool.Dispatch(ctx, []TaskConfig{
    {ID: "t1", Title: "数据分析", Prompt: "分析销售趋势"},
    {ID: "t2", Title: "报告生成", Prompt: "生成月度报告"},
    {ID: "t3", Title: "邮件草拟", Prompt: "撰写客户回访邮件"},
})
// 3 个任务并发执行，自动管理 Agent 生命周期
```

### 🔒 安全沙箱

```go
acl := NewACL()
acl.Allow("agent-1", "/workspace/data", AccessAll)
acl.Deny("agent-1", "/workspace/.env")

sandbox := NewSandbox(acl)
sandbox.AllowCommand("ls")
sandbox.BlockCommand("rm -rf")
sandbox.ValidatePath("agent-1", "/etc/../../../etc/passwd", AccessRead)
// → ErrPathTraversal: 路径穿越攻击已拦截
```

### 🪝 20+ 个生命周期 Hook

在 Agent 运行的每个关键节点插入你的逻辑：

```go
hooks := NewHookManager()
hooks.Register(HookBeforeTool, func(ctx context.Context, hctx *HookContext) error {
    return auditLog.Record(hctx.ToolCall)  // 审计日志
})
hooks.Register(HookOnError, func(ctx context.Context, hctx *HookContext) error {
    return alerting.Notify(hctx.Error)     // 错误告警
})
hooks.Register(HookAfterLLM, func(ctx context.Context, hctx *HookContext) error {
    return costTracker.Record(hctx.Response.Usage)  // 成本追踪
})
```

### 📊 Prometheus 指标

零配置即可获得生产级可观测性：

```
ap_llm_total_calls 1024
ap_llm_latency_ms_bucket{le="100"} 856
ap_tool_total_calls 512
ap_active_agents 3
```

### 🚨 35 个结构化错误码

每个错误都有唯一编码，便于程序化处理和告警：

```go
resp, err := agent.Run(ctx, msg)
if err != nil {
    switch GetCodeFromError(err) {
    case "AGENT_003": log.Println("超出最大轮数")
    case "LLM_003":  log.Println("熔断器开启，Provider 不健康")
    case "SEC_004":   log.Println("检测到路径穿越攻击")
    }
}
```

---

## 🎉 v1.0.0 正式发布 (2026-06-30)

**Go SDK / TypeScript SDK / CLI 全局统一为 v1.0.0，API 稳定性承诺锁定。**

- 🔒 **API 稳定性承诺** — Stable API 向后兼容，破坏性变更需大版本（v2.0）
- 📊 **全局版本统一** — Go `pkg.Version`、TypeScript `package.json`、CLI `ap version` 全部对齐 v1.0.0
- 📖 **文档全面更新** — API 参考文档、CODE_WIKI、README 同步至 v1.0.0
- ✅ **生产就绪** — 47 包 2900+ Go 测试用例 + 154 TypeScript 测试用例全部通过

---

## 🆕 v1.0.0 性能优化亮点

### 代码审查与质量

- 🔍 **全量代码审查**: 覆盖 Go 47 包 + TypeScript 6 文件，发现并修复 24 个问题（3 高 / 8 中 / 13 低优先级）

### 阶段一：并发调度优化

- ⚡ **动态信号量**: Pool 调度器从固定容量 channel 升级为 `sync.Cond` 动态信号量，AutoScaler 实时生效
- 🧵 **协程池优化**: `GoroutinePool.Wait()` 从忙等待改为 `sync.Cond` 通知机制，消除 CPU 空转
- 🔗 **连接池复用**: LLM Provider 统一通过 `NewDefaultLLMClient` 共享 HTTP 连接池
- ⏱️ **超时防护**: 上下文压缩 LLM 调用添加 30s 超时，防止无限阻塞

### 阶段二：内存优化

- 🔄 **HookContext Pool**: ReAct 循环热点路径引入 `sync.Pool` 复用 HookContext，减少 GC 压力
- 📦 **bytes.Buffer Pool**: 通用 buffer 池化工具，benchmark 显示 2.2x 加速、0 allocs/op
- 🧪 **Token 缓存评估**: 实测 `len(text)/4` (0.4ns) 比 sync.Map 缓存 (55ns) 快 100+ 倍，决策不启用缓存

### 阶段三：LLM 层优化

- 🌊 **SSE 流式背压**: OpenAI Provider 添加 timer-based 背压控制（5s 超时 + 10 连续丢弃后中断），防止慢消费者阻塞流
- 📊 **Token 缓存基础设施**: FNV-1a 哈希 + sync.Map 缓存，为未来大规模场景预留

### 阶段四：向量搜索升级

- 🔀 **RRF 融合算法**: 引入 Reciprocal Rank Fusion (Cormack et al., 2009)，解决 Linear 加权量纲不可比问题
- ⚙️ **可配置融合策略**: `RAGFusionConfig` 支持 Linear/RRF 模式切换、权重调优、over-fetch 召回
- 🏷️ **双命中加成**: RRF 模式下同时命中 FTS + Vector 的结果获得 2x 分数加成
- 🛡️ **类型安全**: TypeScript SDK `role` 字段从 `string` 强化为联合类型
- ✅ **测试覆盖**: Go 47 包 2900+ 用例 + TS 6 文件 154 用例，全部 PASS

## 🆕 v1.0.0 亮点

- 🚀 **开发者体验重构**: `ap.NewAgent()` 简化入口，3 行创建带记忆 / RAG / Hook 的 Agent
- 🔌 **`WithRAGMemory()` 一步 RAG**: 自动完成 EmbeddingAdapter + RAGStore + RAGProvider 组装
- 🔀 **RRF 融合算法**: Reciprocal Rank Fusion 混合检索，支持运行时切换 Linear / RRF 模式
- 🧪 **`testutil` 测试包**: `MockProvider` + `NewTestAgent()`，无需手写 Mock
- ⚡ **性能优化**: BufferPool、TokenCache、JSON Pool、pprof 端点、SSE 背压
- ✅ **向后兼容**: 旧 `ap.NewReActAgent()` API 仍然可用
- 🔒 **API 稳定性承诺**: Stable API 向后兼容，破坏性变更需大版本（v2.0）

## v0.7.0 亮点

- 🔒 **安全加固**: symlink 逃逸修复、熔断器逻辑修复、YAML 注入防护、License 统一
- ☸️ **Operator 完善**: Service 暴露、HPA 自动扩缩、真实 Pod 指标采集
- 📦 **TypeScript SDK 扩展**: Pipeline/Handoff/ParallelRun 编排、A2A 消息总线、MCP 类型、SQLite 持久化
- 🔄 **CI/CD**: 安全扫描(govulncheck/Trivy)、多平台测试、Release 签名+SBOM
- 📚 **文档更新**: 架构图重写、CHANGELOG 回填 v0.3-v0.6、开发文档同步

---

## 🚀 Quick Start

### 安装

```bash
go get agentprimordia
```

### 30 秒创建你的第一个 Agent

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    ap "agentprimordia/pkg"
)

func main() {
    // 1. 创建 LLM Provider
    provider, err := ap.NewOpenAIProvider(ap.Config{
        APIKey: os.Getenv("OPENAI_API_KEY"),
        Model:  "gpt-4o-mini",
    })
    if err != nil {
        log.Fatal(err)
    }

    // 2. 创建工具注册表
    registry := ap.NewToolRegistry()

    // 3. 创建 Agent（推荐入口：ap.NewAgent）
    agent, err := ap.NewAgent("my-agent", "You are a helpful assistant.",
        provider,
        ap.WithMaxTurns(10),
        ap.WithToolkit(registry),
    )
    if err != nil {
        log.Fatal(err)
    }

    // 4. 运行！
    resp, err := agent.Run(context.Background(), ap.UserMessage("Hello!"))
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(resp.Content)
    fmt.Printf("Turns: %d, Tokens: %d\n",
        resp.Metrics.TotalTurns,
        resp.Usage.TotalTokens)
}
```

### 5 分钟打造带工具和记忆的 Agent

```go
// 工具
registry := ap.NewToolRegistry()
fsTool, _ := ap.NewFileSystem(".")
registry.Register(fsTool)

// 记忆
memory, _ := ap.WithInMemory()
defer memory.Close()

// Hook：审计每个工具调用
hooks := ap.NewHookManager()
hooks.Register(ap.HookBeforeTool, func(ctx context.Context, hctx *ap.HookContext) error {
    slog.Info("工具调用", "tool", hctx.ToolCall.Name)
    return nil
})

// Agent
agent, err := ap.NewAgent("smart-agent", "You are a file analysis assistant.",
    resilientProvider,
    ap.WithMaxTurns(5),
    ap.WithToolkit(registry),
    ap.WithMemory(memory),
    ap.WithHooks(hooks),
)
if err != nil {
    log.Fatal(err)
}

// 运行
resp, _ := agent.Run(ctx, ap.UserMessage("分析当前目录的文件结构"))

// 回忆
episodes, _ := memory.Search(ctx, "文件", nil)
fmt.Printf("记忆了 %d 条交互记录\n", len(episodes))
```

### 10 分钟构建多 Agent 并发系统

```go
pool := ap.NewPool(ap.PoolConfig{
    MaxConcurrency: 3,
    Timeout:        30 * time.Second,
    DefaultAgent: ap.ReActAgentConfig{
        SystemPrompt: "You are a task assistant.",
        MaxTurns:     3,
    },
})
defer pool.Close()

pool.SetModel(resilientProvider)
pool.SetToolkit(registry)

// 监听事件
go func() {
    for event := range pool.EventChannel() {
        slog.Info("Pool事件", "type", event.Type, "task", event.TaskID)
    }
}()

// 提交任务
results, _ := pool.Dispatch(ctx, []ap.TaskConfig{
    {ID: "1", Title: "代码审查", Prompt: "Review the pull request"},
    {ID: "2", Title: "文档生成", Prompt: "Generate API documentation"},
    {ID: "3", Title: "测试编写", Prompt: "Write unit tests for auth module"},
})

for _, r := range results {
    icon := "✅"
    if r.Error != nil { icon = "❌" }
    fmt.Printf("%s [%s] %s (%v)\n", icon, r.TaskID, r.Task.Title, r.Duration)
}
```

---

## 🏗️ 架构设计

### 分层架构

```
┌─────────────────────────────────────────────────┐
│                  pkg/ (公共 API)                 │
├─────────────────────────────────────────────────┤
│  pool/   │  agent/   │  security/  │ concurrency│
│ (调度)   │ (ReAct)   │  (安全)     │  (并发)    │
├──────────┼───────────┼────────────┼────────────┤
│  tools/  │  memory/  │  events/   │  metrics/  │
│ (工具)   │ (记忆)    │  (事件)    │  (指标)    │
├──────────┴───────────┴────────────┴────────────┤
│              llm/ (Provider 抽象层)              │
├─────────────────────────────────────────────────┤
│            persist/ (检查点持久化)               │
└─────────────────────────────────────────────────┘
```

**设计原则**：
- 🔌 **接口驱动** — 所有子系统通过 interface 解耦
- 🧱 **组合优于继承** — 能力通过配置组合
- 🔒 **弹性优先** — 内建重试、降级、熔断
- 🪶 **零 CGO 依赖** — 纯 Go SQLite 驱动

### ReAct Loop 数据流

```
                ┌──────────┐
                │ 用户输入  │
                └────┬─────┘
                     │
                     ▼
              ┌──────────────┐
              │  RAG 上下文   │ ◄── 知识库检索
              │   注入       │
              └──────┬───────┘
                     │
                     ▼
              ┌──────────────┐
              │ 上下文窗口    │ ◄── 自动裁剪长对话
              │   裁剪       │
              └──────┬───────┘
                     │
                     ▼
     ┌───────────────────────────────┐
     │         LLM 推理              │
     │   (Complete / CallTools)      │
     └───────────┬───────────────────┘
                 │
         ┌───────┴───────┐
         │               │
    有工具调用         无工具调用
         │               │
         ▼               ▼
  ┌─────────────┐  ┌──────────┐
  │  执行工具    │  │ 最终回答  │
  │  (Execute)  │  │ Response │
  └──────┬──────┘  └──────────┘
         │
         ▼
  ┌─────────────┐
  │ 结果加入历史 │
  └──────┬──────┘
         │
         └──────► 回到 RAG 上下文注入
```

---

## 🟦 TypeScript SDK

为 Node.js 开发者提供完整的 TypeScript SDK：

```bash
npm install @agentprimordia/sdk
```

```typescript
import {
    ReActAgent,
    OpenAIProvider,
    ResilientProvider,
    ToolRegistry,
    InMemoryStore,
    VectorStore,
    HookManager,
    AgentPool,
    ACL,
    Sandbox,
    Bus,
    Pipeline,
    ParallelRun,
    Handoff,
    A2ABus,
    MCPClient,
    SqliteStore,
    VERSION,
} from '@agentprimordia/sdk';

// 创建弹性 Provider
const primary = new OpenAIProvider({ apiKey: process.env.OPENAI_API_KEY! });
const fallback = new OpenAIProvider({
    apiKey: process.env.DEEPSEEK_API_KEY!,
    baseURL: 'https://api.deepseek.com/v1',
});
const resilient = new ResilientProvider(primary);
resilient.addFallback(fallback);

// 创建 Agent
const agent = new ReActAgent({
    name: 'production-agent',
    model: resilient,
    toolkit: new ToolRegistry(),
    memory: new InMemoryStore(),
    hooks: new HookManager(),
    maxTurns: 10,
});

// 运行
const response = await agent.run('Analyze the market trends');
console.log(`SDK v${VERSION}: ${response.content}`);
```

---

## 📦 项目结构

```
agentprimordia/
├── cmd/
│   ├── ap/              # CLI 工具 (loop / run / pool / console)
│   └── example/
│       ├── hello-agent/     # 30 秒入门
│       ├── multi-agent/     # 多 Agent 并发
│       └── production/      # 生产级示例（RAG + 多模型 + 可观测性）
├── internal/
│   ├── admin/           # Admin HTTP API (调试/管理接口)
│   ├── agent/           # ReAct Loop 引擎 + Hook + 生命周期
│   │   ├── a2a/         # Agent2Agent 协议 (JSON-RPC / SSE / 任务管理)
│   │   ├── planning/    # 任务规划器
│   │   ├── reflection/  # Agent 自反思
│   │   └── tool_learning/ # 工具学习/自动发现
│   ├── concurrency/     # 文件锁 + 动态协程池 (sync.Cond 信号通知)
│   ├── config/          # 配置热加载
│   ├── debugger/        # 调试器 / Inspector
│   ├── events/          # 内部事件总线
│   ├── guardrail/       # 输入/输出护栏 (注入检测 / PII / 主题过滤)
│   ├── llm/             # Provider 接口 + 10+ 内置实现 + 弹性层
│   ├── memory/          # SQLite + FTS5 + VectorStore + RAG
│   ├── metrics/         # Prometheus 指标采集
│   ├── orchestration/   # 编排模式 (Pipeline / Handoff / DAG / GroupChat)
│   ├── otel/            # OpenTelemetry 桥接与导出
│   ├── persist/         # 状态持久化与 Checkpoint
│   ├── pool/            # 多 Agent 并发调度器 + AutoScaler 动态扩缩
│   ├── prompt/          # 提示词模板与少样本管理
│   ├── security/        # ACL + Sandbox + 路径校验
│   └── tools/           # 工具注册 + 执行 + MCP + 内置工具
│       ├── builtin/     # FileSystem / Shell / Web / API / DB / Code
│       └── mcp/         # MCP 协议适配器
├── pkg/                 # 公共 API 重导出
├── ecosystem/           # 插件生态 / 示例 / 模板
├── operator/            # Kubernetes Operator (CRD + HPA)
├── pgvector/            # pgvector 向量存储扩展
├── bench/               # 性能基准测试套件
└── sdk/typescript/      # TypeScript SDK
```

---

## 📊 质量与性能指标

### 测试覆盖

| 指标 | Go | TypeScript | 合计 |
|------|-----|-----------|------|
| 测试包数 | 47 | 6 | 53 |
| 测试用例 | 2,900+ | 154 | 3,054+ |
| 通过率 | 100% | 100% | 100% |
| 总耗时 | ~125s | 0.6s | ~126s |

### 并发与内存性能

| 组件 | 机制 | 优化效果 |
|------|------|----------|
| Pool 调度器 | `sync.Cond` 动态信号量 | AutoScaler 实时生效，无忙等待 |
| GoroutinePool | `sync.Cond` 通知 | Wait() CPU 占用从 ~100% → ~0% |
| LLM Provider | 共享 HTTP 连接池 | 减少 TCP 连接数，复用 Keep-Alive |
| 上下文压缩 | 30s 超时控制 | 防止 LLM 调用无限阻塞 |
| HookContext | `sync.Pool` 复用 | ReAct 热点路径减少 GC 压力 |
| bytes.Buffer | `sync.Pool` 池化 | 2.2x 加速，0 allocs/op |
| SSE 流式 | Timer-based 背压 | 5s 超时 + 10 丢弃中断，防止流阻塞 |
| Token 估算 | `len(text)/4` 直接计算 | 0.4ns/op，比 sync.Map 缓存快 100+ 倍 |

### 向量搜索性能

| 融合模式 | 算法 | 适用场景 |
|----------|------|----------|
| Linear | 加权融合 (FTS×0.4 + Vec×0.6) | 快速原型、小规模数据 |
| RRF | Reciprocal Rank Fusion (k=60) | 生产环境、大规模数据 |

- **Over-fetch 召回**: 预取 `topK + OverFetchSize` 候选，提升融合质量
- **双命中加成**: RRF 模式下 FTS + Vector 同时命中的结果获得 2x 分数加成
- **可配置策略**: 运行时动态切换融合模式，无需重启

### 架构合规

| 检查项 | 状态 |
|--------|------|
| 接口驱动设计 | 通过 |
| 依赖方向规则 | 通过 |
| 零 CGO 依赖 | 通过 |
| 第三方依赖白名单 | 通过 |
| 并发安全 (mutex/atomic/pool) | 通过 |
| 错误处理规范 | 通过 |
| 结构化错误码 | 35 个可用 |

---

## 🗺️ 优化路线图

## ✅ 已完成 (v1.0)

- [x] **代码审查**: 全量审查 Go 47 包 + TypeScript 6 文件，修复 24 个问题
- [x] **并发调度优化**: `sync.Cond` 动态信号量 + 协程池通知机制
- [x] **内存优化**: HookContext/Buffer `sync.Pool` 池化，benchmark 2.2x 加速
- [x] **LLM 层优化**: SSE 流式背压控制 + Token 缓存评估
- [x] **向量搜索升级**: RRF 融合算法 + `RAGFusionConfig` 可配置化
- [x] **版本统一**: Go SDK / TypeScript SDK / CLI 全局统一为 v1.0.0
- [x] **API 稳定性承诺**: Stable API 向后兼容锁定

### 近期 (v1.1)

- [ ] **高并发压测**: 编写 `bench/` 套件，覆盖 Pool 1000+ 并发任务、GoroutinePool 10K goroutine 场景
- [ ] **LLM 请求批量合并**: 实现 Request Batching 减少 API 调用次数
- [ ] **RRF 生产调优**: 基于真实负载调整 RRF k 值与 over-fetch 比例

### 中期 (v1.2)

- [ ] **向量搜索扩展**: 大规模数据场景迁移到 pgvector 或 Milvus 后端
- [ ] **TypeScript SDK 性能**: 添加 Node.js 性能基准测试
- [ ] **分布式追踪**: 完善 OpenTelemetry Span 链路
- [ ] **eBPF 可观测性**: 内核级 Agent 行为监控

### 长期 (v2.0)

- [ ] **WASM 沙箱**: 基于 WasmEdge 的代码执行隔离
- [ ] **多租户隔离**: 命名空间级别的资源隔离
- [ ] **Agent 市场**: 可插拔 Agent 模板生态
- [ ] **生产就绪**: SLA 保障、混沌工程验证

---

## 🧪 运行示例

```bash
# Hello Agent — 最简示例
make run-hello

# Multi-Agent — 并发调度
make run-multi

# Production — RAG + 弹性调用 + 事件系统
make run-production
```

---

## 📄 完整文档

- **[开发文档 (DEVELOPMENT.md)](./DEVELOPMENT.md)** — 架构详解、扩展指南、API 参考
- **[许可证 (LICENSE)](./LICENSE)** — Apache License 2.0

---

## 🌟 核心优势总结

| | AgentPrimordia | LangChain | AutoGen |
|---|---|---|---|
| **语言** | Go + TypeScript | Python | Python |
| **CGO 依赖** | ❌ 零 CGO | — | — |
| **弹性调用** | ✅ 内建重试+降级+熔断 | 需自行实现 | 需自行实现 |
| **记忆系统** | ✅ SQLite+FTS+Vector+RAG | 需外接 | 基础支持 |
| **安全沙箱** | ✅ ACL+Sandbox+路径穿越检测 | ❌ | ❌ |
| **Prometheus** | ✅ 内建 | 需自行集成 | 需自行集成 |
| **结构化错误码** | ✅ 35 个错误码 | ❌ | ❌ |
| **TypeScript SDK** | ✅ 官方支持 | 社区 | ❌ |
| **Operator HPA** | ✅ | ❌ | ❌ |
| **单二进制部署** | ✅ | ❌ | ❌ |

---

<p align="center">
  <strong>万物之源，智能之始</strong><br>
  用 AgentPrimordia 构建你自己的 AI Agent
</p>
