# TypeScript SDK 入门指南

## 安装

```bash
npm install @agentprimordia/sdk
```

**环境要求：**
- Node.js >= 18
- TypeScript >= 5.4
- 可选依赖：`react >= 18`（使用 React 集成时）、`better-sqlite3`（使用 SQLite 持久化时）

## 基本用法

### 导入 SDK 并创建 Agent

```typescript
import { createAgent, MockProvider } from "@agentprimordia/sdk";

// 使用 MockProvider 快速体验（无需 API Key）
const provider = new MockProvider({ response: "你好，我是 AgentPrimordia！" });

const agent = createAgent("assistant")
  .withProvider(provider)
  .withSystemPrompt("你是一个友好的 AI 助手")
  .withMaxTurns(10)
  .build();

const result = await agent.run("介绍一下你自己");
console.log(result.content);
```

### 配置 LLM Provider

**OpenAI：**

```typescript
import { OpenAIProvider } from "@agentprimordia/sdk";

const provider = new OpenAIProvider({
  apiKey: process.env.OPENAI_API_KEY!,
  model: "gpt-4",
});

const agent = createAgent("assistant")
  .withProvider(provider)
  .withSystemPrompt("你是一个友好的 AI 助手")
  .build();
```

**Anthropic：**

```typescript
import { AnthropicProvider } from "@agentprimordia/sdk";

const provider = new AnthropicProvider({
  apiKey: process.env.ANTHROPIC_API_KEY!,
  model: "claude-sonnet-4-20250514",
});
```

**弹性 Provider（自动重试 + 限流）：**

```typescript
import { ResilientProvider, OpenAIProvider } from "@agentprimordia/sdk";

const base = new OpenAIProvider({ apiKey: process.env.OPENAI_API_KEY! });
const provider = new ResilientProvider(base, {
  maxRetries: 3,
  backoffMs: 1000,
});
```

### 运行对话

```typescript
import { Session } from "@agentprimordia/sdk";

const session = new Session();

// 多轮对话
const r1 = await agent.run("什么是 Agent？", { session });
console.log(r1.content);

const r2 = await agent.run("能给个例子吗？", { session });
console.log(r2.content);
```

## 添加工具

### 注册自定义工具

```typescript
import { AgentTool } from "@agentprimordia/sdk";

// 定义一个天气查询工具
const weatherTool = new AgentTool({
  name: "get_weather",
  description: "获取指定城市的天气信息",
  parameters: {
    type: "object",
    properties: {
      city: { type: "string", description: "城市名称" },
    },
    required: ["city"],
  },
  execute: async ({ city }) => {
    // 实际项目中调用天气 API
    return { temperature: 25, condition: "晴", city };
  },
});

const agent = createAgent("weather-assistant")
  .withProvider(provider)
  .withTools([weatherTool])
  .withSystemPrompt("你是一个天气助手，可以查询城市天气。")
  .build();

const result = await agent.run("北京今天天气怎么样？");
console.log(result.content);
```

### 使用内置工具模块

```typescript
import { createAgent } from "@agentprimordia/sdk";
// 从 tools 子路径导入内置工具集
import { BuiltinTools } from "@agentprimordia/sdk/tools";

const agent = createAgent("tool-assistant")
  .withProvider(provider)
  .withTools(BuiltinTools.all())
  .build();
```

## 记忆系统

```typescript
import { InMemoryStore } from "@agentprimordia/sdk";

const memory = new InMemoryStore();

const agent = createAgent("memory-assistant")
  .withProvider(provider)
  .withMemory(memory)
  .build();

// Agent 会自动在对话中存储和检索记忆
await agent.run("我喜欢 TypeScript 和 Rust");
await agent.run("我喜欢什么编程语言？"); // 会从记忆中检索
```

## 多 Agent 编排

```typescript
import { createAgent, AgentPool } from "@agentprimordia/sdk";

const researcher = createAgent("researcher")
  .withProvider(provider)
  .withSystemPrompt("你是研究员，负责收集信息。")
  .build();

const writer = createAgent("writer")
  .withProvider(provider)
  .withSystemPrompt("你是撰稿人，负责撰写报告。")
  .build();

// 使用 Pool 并发调度多个 Agent
const pool = new AgentPool([researcher, writer]);
const results = await pool.map(["调研 AI 趋势", "撰写周报"]);
```

## 边缘运行时

TS SDK 支持多种边缘环境：

```typescript
// Cloudflare Workers
import { createAgent } from "@agentprimordia/sdk";
// Deno / Bun 同样支持
```

详见 [边缘部署指南](./guide/edge-deployment.md)。

## 下一步

- [跨语言开发指南](./cross-language-guide.md) — 了解 Go 与 TS SDK 的分工
- [功能差异分析](./gap-analysis.md) — 了解两端模块覆盖情况
- [API Reference](./api-reference/) — 完整 API 文档
