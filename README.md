# AgentPrimordia

> 通用 Go Agent 开发框架 — 轻量、并发原生、生产验证

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8E.svg)](https://golang.org)

## 特性

- **ReAct Loop 引擎** — Reasoning + Acting 循环，20+ 生命周期钩子
- **多模式编排** — Pipeline / Handoff / Parallel / DAG / GroupChat / A2A
- **工具系统** — FileSystem / Shell / Web / Knowledge 内置，MCP 协议集成，插件扩展
- **三层记忆** — SQLite FTS5 + Vector Store + RAG Pipeline 混合检索
- **13 家 LLM Provider** — OpenAI / Anthropic / Gemini / Ollama / Azure / Qwen / GLM / Mistral / Cohere / DeepSeek
- **Resilient Provider** — 自动重试 / 降级 / 熔断
- **并发调度** — Pool 信号量调度，会话隔离，重试策略
- **安全防护** — ACL / Sandbox / Guardrails / PII 检测 / 路径遍历防护 + symlink 逃逸防护
- **可观测性** — Prometheus Metrics / OpenTelemetry / Grafana Dashboard
- **K8s Operator** — AgentDeployment CRD 声明式部署
- **CLI 工具** — `ap init / run / debug / test / mcp / plugin`
- **零外部依赖** — 纯 Go 标准库（仅 modernc.org/sqlite + gopkg.in/yaml.v3）

## v0.7.0 Highlights

- **安全加固** — symlink 逃逸修复、熔断器修复、YAML 注入防护
- **Operator** — Service 暴露、HPA 自动扩缩容、真实 Pod 指标
- **TypeScript SDK** — Pipeline/Handoff 编排、A2A 总线、MCP 类型、SQLite 持久化
- **CI/CD** — 安全扫描、多平台测试、Release 签名 + SBOM

## 快速开始

### 安装 CLI

```bash
go build -o ap ./cmd/ap/
```

### 3 步创建 Agent

```bash
ap init my-agent
cd my-agent
ap run
```

### Hello Agent（5 行代码）

```go
package main

import (
    "context"
    "fmt"
    "os"

    ap "agentprimordia/pkg"
)

func main() {
    agent := ap.NewAgent("HelloAgent", "你是一个智能助手",
        ap.NewOpenAIProvider(ap.Config{
            APIKey: os.Getenv("OPENAI_API_KEY"),
            Model:  "gpt-4o",
        }),
        ap.WithMaxTurns(10),
    )

    resp, _ := agent.Run(context.Background(), ap.UserMessage("你好"))
    fmt.Println(resp.Content)
}
```

### 含工具 Agent

```go
registry, _ := ap.DefaultToolkit(ap.ToolkitConfig{
    RootDir:     ".",
    EnableFS:    true,
    EnableShell: true,
    EnableWeb:   true,
})

memory, _ := ap.WithInMemory()
defer memory.Close()

agent := ap.NewAgent("CodingAssistant", "", provider,
    ap.WithMaxTurns(20),
).WithToolkit(registry).WithMemory(memory)
```

### 多 Agent 调度

```go
pool := ap.NewPool(ap.PoolConfig{
    MaxConcurrency: 5,
    DefaultAgent: ap.ReActAgentConfig{
        SystemPrompt: "你是任务处理助手",
        MaxTurns:     10,
    },
})
defer pool.Close()

pool.SetModel(provider)

results, _ := pool.Dispatch(ctx, []ap.TaskConfig{
    {ID: "task-1", Title: "代码分析", Prompt: "分析 main.go"},
    {ID: "task-2", Title: "运行测试", Prompt: "执行 go test"},
})
```

### DAG 编排

```go
dag := ap.NewDAG()
dag.AddNode("collect", collectAgent)
dag.AddNode("analyze", analyzeAgent)
dag.AddNode("report", reportAgent)
dag.AddEdge("collect", "analyze", nil)
dag.AddEdge("analyze", "report", nil)

result, _ := dag.Execute(ctx, ap.UserMessage("分析销售数据"))
```

### MCP Server 集成

```go
// 连接外部 MCP Server
client := ap.NewMCPClient("http://localhost:3001/mcp")
client.Initialize(ctx)
client.RegisterIntoRegistry(toolRegistry)

// 或通过 Registry 管理多个 MCP Server
mcpReg := ap.NewMCPRegistry()
mcpReg.Register(ap.MCPClientConfig{
    Name:      "filesystem",
    Command:   "npx",
    Args:      []string{"@modelcontextprotocol/server-filesystem", "/tmp"},
    AutoStart: true,
})
mcpReg.StartAll(ctx)
mcpReg.RegisterIntoRegistry(toolRegistry)
```

### Resilient Provider（重试 + 降级 + 熔断）

```go
primary := ap.NewOpenAIProvider(ap.Config{APIKey: key, Model: "gpt-4o"})
fallback := ap.NewGeminiProvider(ap.Config{APIKey: geminiKey, Model: "gemini-1.5-pro"})
local := ap.NewOllamaProvider(ap.Config{BaseURL: "http://localhost:11434", Model: "llama3"})

resilient := ap.NewResilientProvider(primary, ap.DefaultResilientConfig())
resilient.AddFallback(fallback)
resilient.AddFallback(local)
```

## 架构

```
┌──────────────────────────────────────────────────────────┐
│                    Your Application                       │
├──────────────────────────────────────────────────────────┤
│                   AgentPrimordia Framework                 │
│                                                            │
│  ┌──────────┐ ┌──────────┐ ┌────────┐ ┌──────────────┐  │
│  │ ReActLoop │ │   Pool   │ │  DAG   │ │  Pipeline    │  │
│  │ (Engine) │ │(Dispatch)│ │(Graph) │ │(Sequential)  │  │
│  └────┬─────┘ └────┬─────┘ └───┬────┘ └──────┬───────┘  │
│       └────────────┼───────────┼──────────────┘          │
│              ┌──────┴──────┐                               │
│              │ Tool System │                               │
│              │ ┌─────────┐ │                               │
│              │ │Built-in │ │  MCP  │  Plugin │             │
│              │ │FS/Shell/│ │  Client│  System │             │
│              │ │Web/Know │ │  Reg.  │         │             │
│              │ └─────────┘ │        │         │             │
│              └─────────────┘        │         │             │
│                                    │         │             │
│  ┌─────────────────────────────────────────────────────┐  │
│  │                  LLM Layer                          │  │
│  │  OpenAI │ Anthropic │ Gemini │ Ollama │ Azure │ ...│  │
│  │              ResilientProvider                       │  │
│  └─────────────────────────────────────────────────────┘  │
│                                                            │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────────┐  │
│  │ Memory   │ │ EventBus │ │ Metrics  │ │ Guardrails │  │
│  │SQLite+FTS│ │(Pub/Sub) │ │OTel/Prom │ │ACL/Sandbox │  │
│  │ +Vector  │ │          │ │          │ │  PII检测   │  │
│  │ +RAG     │ │          │ │          │ │            │  │
│  └──────────┘ └──────────┘ └──────────┘ └────────────┘  │
└──────────────────────────────────────────────────────────┘
```

## 项目结构

```
agentprimordia/
├── cmd/
│   ├── ap/                   # CLI 工具 (ap init/run/debug/test/mcp/plugin)
│   └── example/              # 示例应用
├── internal/
│   ├── agent/                # ReActLoop 引擎 + 编排 (DAG/Pipeline/Handoff/GroupChat)
│   ├── pool/                 # 多 Agent 并发调度
│   ├── tools/                # 工具系统 (Registry/MCP/Plugin/Builtin)
│   ├── memory/               # 记忆存储 (SQLite/Vector/RAG/Milvus/Qdrant/pgvector)
│   ├── llm/                  # LLM 抽象层 (13 Provider + Resilient)
│   ├── guardrail/            # 安全防护 (PII/Topic/Injection/Trie)
│   ├── metrics/              # Prometheus 指标
│   ├── otel/                 # OpenTelemetry 桥接
│   ├── events/               # 事件总线
│   ├── security/             # ACL + Sandbox
│   └── persist/              # 状态持久化
├── operator/                  # K8s Operator (独立 go.mod)
│   ├── api/v1/               # AgentDeployment CRD
│   ├── controller/           # Reconciler
│   ├── cmd/                  # Operator 入口
│   └── manifest/             # CRD + 部署清单 + 示例
├── deploy/grafana/            # Grafana Dashboard 模板
├── bench/                     # 性能基准测试套件
├── docs/                      # 文档 + Cookbook
├── sdk/typescript/            # TypeScript SDK
└── pkg/                       # 公共 API (类型别名 + re-export)
```

## CLI 命令

```bash
ap init my-agent              # 创建项目 (--template basic|with-tools|multi-agent)
ap run                        # 编译运行 (--watch 监视模式)
ap debug                      # 调试服务器 (http://localhost:6060)
ap test                       # 运行 eval 测试套件
ap mcp list                   # 列出 MCP Server
ap mcp add fs --command npx --args "@mcp/server-filesystem,/tmp"
ap mcp test fs                # 测试连通性
ap plugin install github.com/user/ap-plugin-xxx
ap plugin create ap-plugin-weather
```

## Vector DB 选型

| 规模 | 推荐 | 原因 |
|------|------|------|
| <100K 文档 | InMemory | 零依赖 |
| 100K-1M | Qdrant | Go REST 客户端，性能优 |
| >1M | Milvus | 分布式，水平扩展 |
| 已有 PostgreSQL | pgvector | 不引入新基础设施 |

## 可观测性

内置 Prometheus 指标 + OpenTelemetry 桥接，3 个预置 Grafana Dashboard：

```bash
# 导入 Dashboard
kubectl create configmap ap-dashboard \
  --from-file=deploy/grafana/dashboard-agent.json -n monitoring
```

| Dashboard | 内容 |
|-----------|------|
| Agent Runtime | 活跃数、轮次延迟、工具调用频率、错误率 |
| LLM Operations | 延迟 P50/P95/P99、Token 消耗、Provider 分布 |
| Cost Tracking | 成本趋势、按 Provider/Agent 分解 |

## K8s 部署

```yaml
apiVersion: agent.primordia.dev/v1
kind: AgentDeployment
metadata:
  name: code-reviewer
spec:
  replicas: 3
  template:
    provider: openai
    model: gpt-4o
    systemPrompt: "你是一个代码审查助手"
    tools:
      - name: filesystem
      - name: shell
        config:
          commandWhitelist: "go,git"
    memory:
      backend: sqlite
  service:
    type: ClusterIP
    port: 8080
  autoscaling:
    minReplicas: 1
    maxReplicas: 10
    targetConcurrentTasks: 5
```

Operator 自动创建 Service 暴露 Agent 并配置 HPA 基于 Pod 指标自动扩缩容。

```bash
kubectl apply -f operator/manifest/crd.yaml
kubectl apply -f operator/manifest/examples/basic-agent.yaml
kubectl get ad
kubectl get hpa   # 查看 HPA 状态
kubectl get svc   # 查看 Service
```

## 运行测试

```bash
# 核心测试
go test ./internal/... ./pkg/... -race

# CLI 测试
go test ./cmd/ap/

# 集成测试（需要 OPENAI_API_KEY）
make test-integration

# 基准测试
go test -bench=. -benchmem ./bench/suite/

# Lint
golangci-lint run
```

## 设计哲学

1. **来自生产，服务生产** — 核心模式从 CodeCast 生产环境提炼
2. **接口优先** — LLM / Tools / Memory 全部接口解耦，自由替换
3. **并发原生** — Goroutine + Channel 是一等公民
4. **零外部依赖** — 纯 Go 标准库（仅 modernc.org/sqlite + gopkg.in/yaml.v3）
5. **TDD 强制** — 每个功能先写测试，Red → Green → Refactor

## 文档

- [CHANGELOG](CHANGELOG.md)
- [v0.7.0 发布说明](RELEASE-NOTES-v0.7.0.md)
- [架构图](architecture-mermaid.md)
- [API 完整参考](api-reference.md)
- [开发文档](agentprimordia/DEVELOPMENT.md)
- [入门指南](agentprimordia/ecosystem/docs/getting-started.md)
- [CLI 开发手册](agentprimordia/ecosystem/docs/ap-guide.md)
- [最佳实践](agentprimordia/ecosystem/docs/best-practices.md)
- [FAQ](agentprimordia/ecosystem/docs/faq.md)
- [Vector DB 选型指南](agentprimordia/ecosystem/docs/vector-db-guide.md)
- [Cookbook: 客服机器人](agentprimordia/ecosystem/docs/cookbook/customer-support-bot.md)
- [Cookbook: 代码审查 Agent](agentprimordia/ecosystem/docs/cookbook/code-review-agent.md)
- [Cookbook: 数据分析 Agent](agentprimordia/ecosystem/docs/cookbook/data-analysis-agent.md)
- [Cookbook: RAG Agent](agentprimordia/ecosystem/docs/cookbook/rag-agent.md)
- [v0 → v1 迁移指南](agentprimordia/ecosystem/docs/migration/v0-deprecations.md)
- [Go 生态概览](agentprimordia/ecosystem/docs/go-ecosystem.md)
- [贡献 Provider](agentprimordia/ecosystem/contributing/PROVIDER.md)
- [贡献 Plugin](agentprimordia/ecosystem/contributing/PLUGIN.md)

## License

Apache-2.0 © AgentPrimordia Contributors
