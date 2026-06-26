<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&style=for-the-badge" alt="Go Version">
  <img src="https://img.shields.io/badge/TypeScript-5.0+-3178C6?logo=typescript&style=for-the-badge" alt="TypeScript">
  <img src="https://img.shields.io/badge/License-Apache--2.0-blue?style=for-the-badge" alt="License">
  <img src="https://img.shields.io/badge/Zero_CGO-✓-brightgreen?style=for-the-badge" alt="Zero CGO">
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

## 🆕 v0.8.0 亮点

- 🚀 **开发者体验重构**: `ap.NewAgent()` 简化入口，3 行创建带记忆 / RAG / Hook 的 Agent
- 🔌 **`WithRAGMemory()` 一步 RAG**: 自动完成 EmbeddingAdapter + RAGStore + RAGProvider 组装
- 🧪 **`testutil` 测试包**: `MockProvider` + `NewTestAgent()`，无需手写 Mock
- ✅ **向后兼容**: 旧 `ap.NewReActAgent()` API 仍然可用

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

    "agentprimordia/internal/llm"
    ap "agentprimordia/pkg"
)

func main() {
    // 1. 创建 LLM Provider
    provider, err := llm.NewOpenAIProvider(llm.Config{
        APIKey: os.Getenv("OPENAI_API_KEY"),
        Model:  "gpt-4o-mini",
    })
    if err != nil {
        log.Fatal(err)
    }

    // 2. 创建工具注册表
    registry := tools.NewRegistry()

    // 3. 创建 Agent（推荐入口：ap.NewAgent）
    agent := ap.NewAgent("my-agent", "You are a helpful assistant.",
        provider,
        ap.WithMaxTurns(10),
    ).WithToolkit(registry)

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
registry := tools.NewRegistry()
fsTool, _ := builtin.NewFileSystem(".")
registry.Register(fsTool)

// 记忆
memory, _ := ap.WithInMemory()
defer memory.Close()

// Hook：审计每个工具调用
hooks := agent.NewHookManager()
hooks.Register(agent.HookBeforeTool, func(ctx context.Context, hctx *agent.HookContext) error {
    slog.Info("工具调用", "tool", hctx.ToolCall.Name)
    return nil
})

// Agent
agent := ap.NewAgent("smart-agent", "You are a file analysis assistant.",
    resilientProvider,
    ap.WithMaxTurns(5),
).WithToolkit(registry).WithMemory(memory).WithHooks(hooks)

// 运行
resp, _ := agent.Run(ctx, ap.UserMessage("分析当前目录的文件结构"))

// 回忆
episodes, _ := memory.Search(ctx, "文件", nil)
fmt.Printf("记忆了 %d 条交互记录\n", len(episodes))
```

### 10 分钟构建多 Agent 并发系统

```go
pool := pool.NewPool(pool.PoolConfig{
    MaxConcurrency: 3,
    Timeout:        30 * time.Second,
    DefaultAgent: pool.ReActAgentConfig{
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
results, _ := pool.Dispatch(ctx, []pool.TaskConfig{
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
├── cmd/example/
│   ├── hello-agent/     # 30 秒入门
│   ├── multi-agent/     # 多 Agent 并发
│   └── production/      # 生产级示例（RAG + 多模型 + 可观测性）
├── internal/
│   ├── agent/           # ReAct Loop 引擎 + Hook + 生命周期
│   ├── llm/             # Provider 接口 + 5 个内置实现 + 熔断器
│   ├── operator/        # Kubernetes Operator (Service 暴露 + HPA 自动扩缩)
│   │   └── api/v1/      # CRD 定义 (AgentPrimordia / AgentPrimordiaList)
│   ├── memory/          # SQLite + FTS5 + VectorStore + RAG
│   ├── tools/           # 工具注册 + 执行 + 作用域策略
│   │   └── builtin/     # FileSystem / Shell / Web / Knowledge
│   ├── pool/            # 多 Agent 并发调度器
│   ├── security/        # ACL + Sandbox
│   ├── events/          # 发布/订阅事件总线
│   ├── metrics/         # Prometheus 指标采集
│   ├── persist/         # 检查点持久化
│   └── concurrency/     # 文件锁 + 作用域验证
├── pkg/                 # 公共 API 重导出
└── sdk/typescript/      # TypeScript SDK
```

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
