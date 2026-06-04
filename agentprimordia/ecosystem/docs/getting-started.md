# 🚀 快速入门指南

欢迎来到 AgentPrimordia！本指南将在 5 分钟内带你从零到运行第一个 AI Agent。

## 📋 前置要求

- **Go 1.22+**（推荐 Go 1.26+）
- **Git**（用于克隆项目）
- 可选：**API Key**（OpenAI / Gemini / 通义千问）

## 🔧 安装步骤

### 方式一：直接下载（推荐）

```bash
# 克隆仓库
git clone https://github.com/your-org/agentprimordia.git
cd agentprimordia

# 编译
go build ./...
```

### 方式二：Go module 引用

在你的 `go.mod` 中添加：

```
require agentprimordia v0.1.0
```

## ⚡ 第一个 Agent（无需 API Key）

让我们创建一个最简单的 Agent：

```go
package main

import (
    "context"
    "fmt"
    "log"

    "agentprimordia/cmd/example/demo"
    "agentprimordia/internal/agent"
)

func main() {
    // 使用内置的 Demo LLM（无需 API Key）
    demoLLM := demo.NewDemoLLM(
        "你好!我是AI助手,很高兴为你服务!",
    )

    // 创建 ReAct Agent
    simpleAgent := agent.NewReActAgent(agent.ReActConfig{
        Name:         "SimpleBot",
        SystemPrompt: "你是一个友好的AI助手",
        Model:        demoLLM,
        MaxTurns:     3,
    })

    // 运行 Agent
    resp, err := simpleAgent.Run(context.Background(), agent.UserMessage("你好"))
    if err != nil {
        log.Fatalf("❌ 运行失败: %v", err)
    }

    fmt.Println("✅ Agent 回复:", resp.Content)
}
```

运行：

```bash
go run examples/go/simple/main.go
```

## 🌐 连接真实 LLM Provider

### OpenAI (GPT-4o)

```go
import "agentprimordia/internal/llm"

provider, err := llm.NewOpenAIProvider(llm.Config{
    APIKey: "your-openai-api-key",
    Model:  "gpt-4o",
})
if err != nil {
    log.Fatal(err)
}

// 创建带真实 LLM 的 Agent
agent := agent.NewReActAgent(agent.ReActConfig{
    Name:         "GPTBot",
    SystemPrompt: "你是一个专业的AI助手",
    Model:        provider,
})
```

### Google Gemini

```go
import "agentprimordia/internal/llm"

provider, err := llm.NewGeminiProvider(llm.Config{
    APIKey: "your-gemini-api-key",
    Model:  "gemini-2.0-flash",
})
```

### 通义千问 (Qwen)

```go
import "agentprimordia/internal/llm"

provider, err := llm.NewQwenProvider(llm.Config{
    APIKey:  "your-qwen-api-key",
    BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",  // 阿里云兼容接口
    Model:   "qwen-plus",
})
```

## 🛠️ 添加工具能力

让 Agent 能够执行文件操作、Shell 命令等：

```go
import (
    "agentprimordia/internal/tools/builtin"
)

// 创建工具集
registry, err := builtin.DefaultToolkit(builtin.ToolkitConfig{
    RootDir:     "./workspace",   // 工作目录
    EnableFS:    true,            // 启用文件系统工具
    EnableShell: true,            // 启用 Shell 工具
    EnableWeb:   true,            // 启用 Web 工具
    EnableUtils: true,            // 启用计算器/日期时间工具
})
if err != nil {
    log.Fatal(err)
}

// 创建带工具的 Agent
agent := agent.NewReActAgent(agent.ReActConfig{
    Name:         "ToolBot",
    SystemPrompt: "你可以使用工具来帮助用户完成任务",
    Model:        provider,
    Tools:        registry,
})
```

## 💾 使用 Memory 系统

让 Agent 拥有记忆能力：

```go
import "agentprimordia/internal/memory"

// 方式1: SQLite 存储（持久化）
memoryStore, err := memory.NewMemory(memory.Config{
    Type: memory.BackendSQLite,
    Path: "./my_agent_memory.db",
})

// 方式2: 内存存储（测试用）
memoryStore, _ := memory.NewMemory(memory.Config{
    Type: memory.BackendMemory,
})

// 创建带记忆的 Agent
agent := agent.NewReActAgent(agent.ReActConfig{
    Name:         "MemoryBot",
    SystemPrompt: "你拥有长期记忆，可以记住对话内容",
    Model:        provider,
    Memory:       memoryStore,
})
```

## 🐛 调试与可视化

使用内置的调试工具：

```go
import "agentprimordia/internal/debugger"

// 启动调试 HTTP 服务器
debugServer := debugger.NewDebugServer(":8080")
go func() {
    if err := debugServer.Start(); err != nil {
        log.Fatal(err)
    }
}()

// 记录事件
debugServer.AddEvent("info", "Agent 启动成功")

// 浏览器访问 http://localhost:8080 查看调试界面
```

## 📚 下一步

- 📘 [API 完整参考](./api-reference.md) - 所有接口详细说明
- 📗 [最佳实践指南](./best-practices.md) - 生产环境建议
- 📕 [示例代码库](../examples/) - 10+ 示例
- 🎥 [视频教程](https://youtube.com/...) - 视频学习

## ❓ 常见问题

### Q: 需要付费的 API Key 吗？
A: 不需要！内置 Demo LLM 可以免费体验所有功能。

### Q: 支持 Windows 吗？
A: ✅ 完全支持！Zero CGO 设计确保跨平台兼容性。

### Q: 如何切换不同的 LLM？
A: 只需替换 `Model` 参数即可，一行代码搞定！

### Q: 性能如何？
A: 单 Agent 响应 <100ms（不含 LLM 调用时间），支持并发 1000+ Agents。

---

**🎉 恭喜！你已经掌握了 AgentPrimordia 的基础用法。继续探索更多功能吧！**
