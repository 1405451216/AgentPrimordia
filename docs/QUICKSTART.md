# AgentPrimordia 快速上手指南

> 5 分钟内创建并运行你的第一个 AI Agent。

## 前置条件

- Go 1.26+（`go version` 确认）
- 任意 LLM API Key（OpenAI / Anthropic / Ollama 等）

## 1. 安装

```bash
go get agentprimordia@latest
```

## 2. 创建第一个 Agent

创建 `main.go`：

```go
package main

import (
	"context"
	"fmt"
	"log"

	ap "agentprimordia/pkg"
)

func main() {
	// 创建 LLM Provider（以 OpenAI 为例）
	provider, err := ap.NewOpenAIProvider(ap.OpenAIConfig{
		APIKey: "sk-your-key-here",
		Model:  "gpt-4o",
	})
	if err != nil {
		log.Fatal(err)
	}

	// 创建 Agent
	agent, err := ap.NewAgent("my-first-agent", "You are a helpful assistant.", provider,
		ap.WithMaxTurns(10),
	)
	if err != nil {
		log.Fatal(err)
	}

	// 运行对话
	resp, err := agent.Run(context.Background(), ap.UserMessage("Hello! What can you do?"))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Agent says:", resp.Content)
}
```

## 3. 运行

```bash
export OPENAI_API_KEY="sk-your-key-here"
go run main.go
```

预期输出：
```
Agent says: Hello! I'm an AI assistant powered by AgentPrimordia. I can help you with...
```

## 4. 添加工具

让 Agent 能调用工具：

```go
import "agentprimordia/pkg"

// 创建工具注册表
registry := ap.NewRegistry()

// 注册内置文件系统工具
registry.Register(ap.NewFilesystemTool("/tmp/workspace"))

// 创建带工具的 Agent
agent, _ := ap.NewAgent("tool-agent", "You can read and write files.", provider,
    ap.WithMaxTurns(10),
    ap.WithToolkit(registry),
)
```

## 5. 添加记忆

让 Agent 记住对话历史：

```go
// 创建内存存储（生产环境用 SQLite）
mem := ap.NewInMemoryStore()

agent, _ := ap.NewAgent("memory-agent", "You have memory.", provider,
    ap.WithMemory(mem),
)
```

## 6. 流式输出

```go
ch, _ := agent.StreamRun(ctx, ap.UserMessage("Tell me a story"))
for event := range ch {
    fmt.Print(event.Content) // 逐字输出
}
```

## 7. 多 Agent 编排

```go
// Pipeline: Agent A → Agent B → Agent C
pipeline := ap.NewPipeline(30 * time.Second)
pipeline.AddStage(&ap.Stage{
    Name:    "research",
    Handler: researchAgent.Run,
})
pipeline.AddStage(&ap.Stage{
    Name:    "write",
    Handler: writerAgent.Run,
})
result, _ := pipeline.Execute(ctx, "Write about Go concurrency")
```

## 下一步

- 📖 [API 文档](https://pkg.go.dev/agentprimordia/pkg)
- 🧪 [示例代码](../ecosystem/examples/)
- 🏗️ [架构文档](architecture-mermaid.md)
- 🔧 [部署指南](DEPLOYMENT.md)
