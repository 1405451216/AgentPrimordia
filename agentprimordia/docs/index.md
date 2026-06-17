# AgentPrimordia

**Go 语言生产级 AI Agent 开发框架**

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Documentation](https://img.shields.io/badge/docs-在线文档-green.svg)](https://docs.agentprimordia.dev)

---

## 什么是 AgentPrimordia？

AgentPrimordia 是一个用 Go 语言编写的生产级 AI Agent 开发框架，提供完整的工具来构建、部署和管理智能 Agent 系统。

### 核心特性

- **🚀 高性能**: Go 语言原生性能，启动速度快 20-30 倍，内存占用低 7-10 倍
- **🔧 协议式微内核**: 14 个 Capable 接口，能力自动发现
- **🛠️ 完整工具系统**: 内置工具 + 插件生态 + MCP 协议支持
- **🧠 三层记忆**: SQLite FTS5 + 向量存储 + RAG 管道
- **👥 多 Agent 编排**: 6 种编排模式（Sequential/Parallel/DAG/GroupChat/Workflow）
- **🔍 可观测性**: AP Inspector 实时追踪和调试
- **🎨 可视化编排**: Web UI 拖拽式 DAG 编排
- **📦 零外部依赖**: 仅依赖纯 Go SQLite，部署极简

## 快速开始

### 安装

```bash
# 安装 CLI 工具
go install github.com/AgentPrimordia/agentprimordia/cmd/ap@latest

# 创建新项目
ap init my-agent --template quickstart
cd my-agent

# 运行
go run main.go
```

### 5 分钟创建第一个 Agent

```go
package main

import (
    "context"
    "fmt"
    ap "agentprimordia/pkg"
)

func main() {
    // 创建 Agent
    agent := ap.NewReActAgent(ap.ReActConfig{
        Name:         "my-first-agent",
        SystemPrompt: "你是一个友好的助手",
        Model: ap.NewOpenAIProvider(ap.OpenAIConfig{
            APIKey: "your-api-key",
            Model:  "gpt-4o-mini",
        }),
        MaxTurns: 10,
    })

    // 运行
    resp, err := agent.Run(context.Background(), ap.UserMessage("你好！"))
    if err != nil {
        panic(err)
    }

    fmt.Println(resp.Content)
}
```

## 性能对比

| 指标 | AgentPrimordia (Go) | LangGraph (Python) | 提升 |
|------|---------------------|-------------------|------|
| 启动时间 | 0.12s | 2.8s | **23x** |
| 内存占用 | 45MB | 320MB | **7x** |
| 并发吞吐 | 852 req/s | 98 req/s | **8.7x** |
| 部署体积 | 15MB | 280MB | **18x** |

详细性能报告：[性能基准对比](docs/benchmarks/performance-comparison-2026.md)

## 核心概念

### 1. Agent 架构

Agent 是框架的核心单元，基于 ReAct（Reasoning + Acting）循环模式运行。

```go
// 创建 Agent
agent := ap.NewReActAgent(ap.ReActConfig{
    Name:         "assistant",
    SystemPrompt: "你是一个专业的助手",
    Model:        provider,
    MaxTurns:     10,
})

// 运行
response, err := agent.Run(ctx, ap.UserMessage("帮我分析这段代码"))
```

### 2. 工具系统

Agent 可以通过工具与外部世界交互。

```go
// 注册工具
agent.AddTool(ap.NewFunctionTool("search", "搜索信息", func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
    query := args["query"].(string)
    // 实现搜索逻辑
    return results, nil
}))
```

### 3. 记忆系统

三层记忆架构支持短期、长期和语义记忆。

```go
// 配置记忆
agent.WithMemory(ap.MemoryConfig{
    Backend: "sqlite",
    Path:    "./data/memory.db",
})
```

### 4. 多 Agent 编排

支持多种编排模式协调多个 Agent。

```go
// 创建编排器
orchestrator := ap.NewOrchestrator(ap.OrchestratorConfig{
    Mode: ap.DAGMode,
})

// 添加步骤
orchestrator.AddStep(ap.AgentStep{
    ID:    "research",
    Agent: researchAgent,
})

orchestrator.AddStep(ap.AgentStep{
    ID:        "write",
    Agent:     writeAgent,
    InputFrom: []string{"research"},
})

// 执行
result, err := orchestrator.Execute(ctx, input)
```

## 使用场景

### 🎮 游戏 AI NPC

```go
npc := ap.NewReActAgent(ap.ReActConfig{
    Name:         "game-npc",
    SystemPrompt: "你是一个友好的游戏NPC",
    MaxTurns:     5,
})

// 响应玩家
response, _ := npc.Run(ctx, ap.UserMessage("你好，请问任务在哪里？"))
```

### 💼 企业自动化

```go
// 文档处理 Agent
docAgent := ap.NewReActAgent(ap.ReActConfig{
    Name:         "doc-processor",
    SystemPrompt: "你是一个专业的文档处理助手",
})

docAgent.AddTool(ap.NewFunctionTool("extract", "提取文档内容", extractFunc))
docAgent.AddTool(ap.NewFunctionTool("summarize", "总结文档", summarizeFunc))
```

### 📊 数据分析

```go
// 数据分析 Agent
analyzer := ap.NewReActAgent(ap.ReActConfig{
    Name:         "data-analyst",
    SystemPrompt: "你是一个数据分析专家",
})

analyzer.AddTool(ap.NewFunctionTool("query_db", "查询数据库", queryDBFunc))
analyzer.AddTool(ap.NewFunctionTool("visualize", "生成图表", visualizeFunc))
```

## 生态系统

### 官方插件

- **HTTP 插件**: HTTP 请求工具
- **SQL 插件**: 数据库操作
- **Git 插件**: Git 操作
- **JSON 插件**: JSON 处理
- **Email 插件**: 邮件发送
- **KV 插件**: 键值存储

### 社区项目

- [AgentPrimordia Web](https://github.com/AgentPrimordia/web) - Web 界面
- [AgentPrimordia Examples](https://github.com/AgentPrimordia/examples) - 示例集合
- [AgentPrimordia Plugins](https://github.com/AgentPrimordia/plugins) - 插件市场

## 文档

- 📖 [完整文档](https://docs.agentprimordia.dev)
- 🚀 [快速开始](getting-started/quickstart.md)
- 📚 [API 参考](api/agent.md)
- 💡 [示例代码](examples/basic.md)

## 社区

- 💬 [GitHub Discussions](https://github.com/AgentPrimordia/agentprimordia/discussions)
- 🐦 [Twitter](https://twitter.com/AgentPrimordia)
- 📺 [视频教程](https://www.youtube.com/@AgentPrimordia)

## 贡献

欢迎贡献！请查看 [贡献指南](CONTRIBUTING.md) 了解详情。

## 许可证

MIT License - 详见 [LICENSE](LICENSE) 文件

---

**开始构建你的第一个 AI Agent 吧！** 🚀
