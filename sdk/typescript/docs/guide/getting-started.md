# Getting Started

## 5-Minute Quick Start

### 1. Install

```bash
npm install @agentprimordia/sdk
```

### 2. Create an Agent

```typescript
import { ReActAgent, MockProvider, ToolRegistry } from '@agentprimordia/sdk';

const provider = new MockProvider({ response: 'Hello! How can I help?' });
const registry = new ToolRegistry();

const agent = new ReActAgent({
  name: 'my-agent',
  model: provider,
  toolkit: registry,
  maxTurns: 5,
});

const response = await agent.run('Hi there!');
console.log(response.content); // "Hello! How can I help?"
```

### 3. Add Tools

```typescript
import { Tool } from '@agentprimordia/sdk';

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

registry.register(new WeatherTool());
```

### 4. Use OpenAI Provider

```typescript
import { OpenAIProvider } from '@agentprimordia/sdk';

const provider = new OpenAIProvider({
  apiKey: process.env.OPENAI_API_KEY!,
  model: 'gpt-4o',
});

const agent = new ReActAgent({
  name: 'openai-agent',
  model: provider,
  toolkit: registry,
  maxTurns: 10,
  systemPrompt: 'You are a helpful assistant with weather tools.',
});
```

### 5. Add Resilience

```typescript
import { ResilientProvider } from '@agentprimordia/sdk';

const resilient = new ResilientProvider(provider);
resilient.addFallback(new MockProvider({ response: 'Service temporarily unavailable' }));

const agent = new ReActAgent({
  name: 'resilient-agent',
  model: resilient,
  toolkit: registry,
  maxTurns: 10,
});
```

## Next Steps

- [Agent Lifecycle & Hooks](./agent.md)
- [Memory & Conversation Management](./memory.md)
- [Tool System](./tools.md)
- [API Reference](/api/)
