# Agent

## ReActAgent

The core agent implementation using the ReAct (Reason-Act-Observe) loop pattern.

```typescript
import { ReActAgent, MockProvider, ToolRegistry } from '@agentprimordia/sdk';

const agent = new ReActAgent({
  name: 'my-agent',
  model: new MockProvider({ response: 'Done!' }),
  toolkit: new ToolRegistry(),
  maxTurns: 5,
  systemPrompt: 'You are a helpful assistant.',
});
```

### Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| name | string | required | Agent identifier |
| model | Provider | required | LLM provider |
| toolkit | ToolRegistry | required | Tool registry |
| maxTurns | number | 10 | Maximum reasoning turns |
| systemPrompt | string | undefined | System prompt prepended to messages |
| hooks | HookManager | undefined | Lifecycle hook manager |
| lifecycle | Lifecycle | undefined | Status management |

### Running

```typescript
// Simple run
const response = await agent.run('What is the weather?');
console.log(response.content);
console.log(response.metrics);

// Streaming
for await (const chunk of agent.stream('Tell me a story')) {
  process.stdout.write(chunk);
}
```

## Hooks

Register lifecycle hooks to observe or modify agent behavior.

```typescript
import { HookManager } from '@agentprimordia/sdk';

const hooks = new HookManager();

// Available hook points:
// before_run, after_run, before_turn, after_turn
// before_llm, after_llm, before_tool, after_tool
// on_error, on_complete

hooks.register('before_llm', (ctx) => {
  console.log(`Turn ${ctx.turn}: Calling LLM...`);
});

hooks.register('after_tool', (ctx) => {
  console.log(`Tool ${ctx.toolCall?.name} returned: ${ctx.toolResult?.content}`);
});

hooks.register('on_error', (ctx) => {
  console.error(`Error at turn ${ctx.turn}: ${ctx.error}`);
});
```

## Lifecycle

Control agent execution state.

```typescript
import { Lifecycle } from '@agentprimordia/sdk';

const lifecycle = new Lifecycle();

// Check status: idle | running | paused | completed | error
console.log(lifecycle.getStatus());

// Stop agent
lifecycle.stop();

// Pause/Resume
lifecycle.pause();
lifecycle.resume();
```
