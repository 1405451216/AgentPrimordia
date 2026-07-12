# Quick Start

## TypeScript (npm)

```bash
npm install @agentprimordia/sdk
```

```typescript
import { ReActAgent, createAgent, MockProvider } from "@agentprimordia/sdk";

// 1. 使用 Mock Provider 快速体验
const provider = new MockProvider({ response: "Hello from AgentPrimordia!" });

// 2. 通过 Builder DSL 构建 Agent
const agent = createAgent("assistant")
  .withProvider(provider)
  .withSystemPrompt("You are a helpful assistant")
  .withMaxTurns(10)
  .build();

// 3. 运行对话
const result = await agent.run("Hello!");
console.log(result.content);
```

## Python

```bash
pip install agentprimordia
```

```python
from agentprimordia import AgentPrimordia, Tool, Session

client = AgentPrimordia(api_key="your-api-key")
agent = client.create_agent(name="assistant", model="gpt-4")
session = agent.chat("Hello!")
print(session.last_response.content)
```

## Go

```bash
go get github.com/agentprimordia/ap
```

```go
import "github.com/agentprimordia/pkg"

func main() {
    agent := pkg.NewAgent("assistant", "gpt-4")
    resp, _ := agent.Run(ctx, "Hello!")
    fmt.Println(resp.Content)
}
```

## 下一步

- 阅读 [Installation Guide](installation.md) 了解完整安装步骤
- 查看 [TypeScript API Reference](../api-reference/typescript.md) 获取详细 API 文档
- 访问 GitHub 上的 [examples/](https://github.com/agentprimordia/ap/tree/main/ecosystem/examples) 查看完整示例
