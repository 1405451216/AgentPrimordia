# TypeScript API Reference

## SDK 包导出

| 导出路径 | 说明 |
||------|------|
|| `@agentprimordia/sdk` | 主入口 |
|| `@agentprimordia/sdk/agent` | ReActAgent、Builder |
|| `@agentprimordia/sdk/llm` | LLM Providers |
|| `@agentprimordia/sdk/tools` | 工具系统 |
|| `@agentprimordia/sdk/orchestration` | 编排模式 |

## 核心类型

### Agent / ReAct

|| 名称 | 说明 |
||------|------|
|| `ReActAgent` | ReAct Loop 引擎 |
|| `createAgent(name)` | Builder DSL 入口 |
|| `StreamEvent` | 流式响应事件 |

### LLM Providers

|| Provider | 说明 |
||------|------|
|| `OpenAIProvider` | OpenAI API |
|| `AnthropicProvider` | Anthropic API |
|| `GeminiProvider` | Google Gemini |
|| `OllamaProvider` | Local Ollama |
|| `DeepSeekProvider` | DeepSeek API |
|| `MockProvider` | 测试用模拟 Provider |

### Memory

|| 类型 | 说明 |
||------|------|
|| `InMemoryStore` | 内存记忆存储 |
|| `SQLiteStore` | SQLite 持久化存储 |
|| `RAGProvider` | RAG 检索增强 |

### Tools

|| 类型 | 说明 |
||------|------|
|| `ToolRegistry` | 工具注册中心 |
|| `FileSystemTool` | 文件系统工具 |
|| `ShellTool` | Shell 工具 |
|| `WebTool` | Web 工具 |
|| `APITool` | API 调用工具 |

## 快速示例

```typescript
import { createAgent, OpenAIProvider } from "@agentprimordia/sdk";

const provider = new OpenAIProvider({ apiKey: process.env.OPENAI_API_KEY! });
const agent = createAgent("assistant")
  .withProvider(provider)
  .withSystemPrompt("You are a helpful coding assistant")
  .withMaxTurns(10)
  .build();

// 同步调用
const result = await agent.run("Write a quicksort function");
console.log(result.content);

// 流式调用
for await (const chunk of agent.stream("Explain recursion")) {
  process.stdout.write(chunk.content);
}
```

## 错误处理

```typescript
import { CodeError, ErrMaxTurnsExceeded } from "@agentprimordia/sdk";

try {
  await agent.run(input);
} catch (err) {
  if (err.code === "AGENT_003") {
    console.error("Max turns exceeded");
  }
}
```
