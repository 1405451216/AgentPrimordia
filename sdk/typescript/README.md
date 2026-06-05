# @agentprimordia/sdk

TypeScript SDK for AgentPrimordia — Universal AI Agent Development Framework.

## Installation

```bash
npm install @agentprimordia/sdk
```

## Quick Start

```typescript
import { ReActAgent, MockProvider, ToolRegistry } from '@agentprimordia/sdk';

const provider = new MockProvider({ response: 'Hello!' });
const registry = new ToolRegistry();

const agent = new ReActAgent({
  name: 'my-agent',
  model: provider,
  toolkit: registry,
  maxTurns: 5,
});

const response = await agent.run('Hi');
console.log(response.content);
```

## API Overview

### Agent
- `ReActAgent` — ReAct loop agent with hook system
- `HookManager` — Register lifecycle hooks (before_run, after_turn, etc.)
- `Lifecycle` — Agent status management (idle, running, paused, completed, error)

### LLM Providers
- `MockProvider` — Test provider with configurable responses
- `OpenAIProvider` — OpenAI API integration (gpt-4o, gpt-4o-mini)
- `ResilientProvider` — Retry + circuit breaker + fallback wrapper

### Tools
- `ToolRegistry` — Register and execute tools
- `FileScopePolicy` — File access scope management for multi-agent setups

### Memory
- `InMemoryStore` — In-memory episodic memory store
- `VectorStore` — Cosine similarity vector search

### Events
- `Bus` — Pub/sub event bus for agent lifecycle events

### Security
- `ACL` — Access control list (allow/deny rules)
- `Sandbox` — Command execution and path access control

### Metrics
- `MetricsCollector` — Track LLM/tool call latencies and error rates

### Concurrency
- `AgentPool` — Concurrent agent task execution with configurable max concurrency

## Adding Tools

```typescript
import { Tool, ToolRegistry } from '@agentprimordia/sdk';

class WeatherTool implements Tool {
  name = 'get_weather';
  description = 'Get current weather for a city';
  parameters = {
    type: 'object' as const,
    properties: {
      city: { type: 'string', description: 'City name' },
    },
    required: ['city'],
  };

  async execute(args: { city: string }): Promise<string> {
    return `Weather in ${args.city}: 22°C, sunny`;
  }
}

const registry = new ToolRegistry();
registry.register(new WeatherTool());
```

## Using Hooks

```typescript
import { ReActAgent, HookManager, MockProvider, ToolRegistry } from '@agentprimordia/sdk';

const hooks = new HookManager();
hooks.register('before_llm', (ctx) => {
  console.log(`Calling LLM at turn ${ctx.turn}`);
});
hooks.register('after_tool', (ctx) => {
  console.log(`Tool ${ctx.toolCall?.name} completed`);
});

const agent = new ReActAgent({
  name: 'hooked-agent',
  model: new MockProvider({ response: 'Done!' }),
  toolkit: new ToolRegistry(),
  maxTurns: 3,
  hooks,
});
```

## OpenAI Provider

```typescript
import { OpenAIProvider } from '@agentprimordia/sdk';

const provider = new OpenAIProvider({
  apiKey: process.env.OPENAI_API_KEY!,
  model: 'gpt-4o',
});
```

## Resilient Provider

```typescript
import { OpenAIProvider, ResilientProvider, MockProvider } from '@agentprimordia/sdk';

const primary = new OpenAIProvider({ apiKey: process.env.OPENAI_API_KEY! });
const fallback = new MockProvider({ response: 'Fallback response' });

const resilient = new ResilientProvider(primary);
resilient.addFallback(fallback);
```

## Documentation

- [Getting Started](./docs/guide/getting-started.md)
- [Agent Lifecycle](./docs/guide/agent.md)
- [Memory System](./docs/guide/memory.md)
- [Tool System](./docs/guide/tools.md)
- [API Reference](./docs/api/index.md)

## License

Apache-2.0
