# API Reference

## Core

### ReActAgent

Main agent class implementing the ReAct loop.

**Methods:**
- `run(input: string): Promise<Response>` — Run agent with input
- `stream(input: string): AsyncIterable<string>` — Stream agent output
- `callLLM(): Promise<{content, toolCalls?}>` — Direct LLM call

### HookManager

**Methods:**
- `register(point: HookPoint, fn: HookFunc): void`
- `fire(ctx: HookContext): Promise<void>`
- `remove(point: HookPoint): void`
- `count(point: HookPoint): number`

### Lifecycle

**Methods:**
- `getStatus(): AgentStatus`
- `setStatus(s: AgentStatus): void`
- `stop(): void`
- `pause(): void`
- `resume(): void`

## LLM Providers

### Provider Interface

```typescript
interface Provider {
  complete(req: CompletionRequest): Promise<CompletionResponse>;
  stream?(req: CompletionRequest): AsyncIterable<Chunk>;
  callTools(req: ToolCallRequest): Promise<ToolCallResponse>;
  embeddings?(texts: string[]): Promise<number[][]>;
  info(): ModelInfo;
}
```

### OpenAIProvider

**Constructor:** `new OpenAIProvider(config: ProviderConfig)`

### MockProvider

**Constructor:** `new MockProvider(opts: { response?, toolCalls?, error?, delay? })`

### ResilientProvider

**Constructor:** `new ResilientProvider(primary: Provider, opts?)`

**Methods:**
- `addFallback(provider: Provider): void`

## Memory

### InMemoryStore

Implements the `Memory` interface with in-memory storage.

### VectorStore

**Constructor:** `new VectorStore(dimensions?: number)`

**Methods:**
- `add(id, vector, metadata?): void`
- `search(query, topK?): VectorSearchResult[]`
- `delete(id): boolean`
- `get(id): metadata | undefined`
- `count(): number`

## Security

### ACL

**Methods:**
- `allow(agentID, resource, level): void`
- `deny(agentID, resource): void`
- `check(agentID, resource, required): boolean`

### Sandbox

**Methods:**
- `allowCommand(cmd): void`
- `blockCommand(cmd): void`
- `canExecute(agentID, cmd): Error | null`
- `canAccess(agentID, resource, level): Error | null`
- `validatePath(agentID, path, level): Error | null`

## Events

### Bus

**Methods:**
- `subscribe(eventType, handler): string`
- `subscribeAll(handler): string`
- `unsubscribe(id): void`
- `publish(event): void`
- `close(): void`

## Metrics

### MetricsCollector

**Methods:**
- `recordLLMCall(durationMs, error?): void`
- `recordToolCall(durationMs, error?): void`
- `recordTurn(): void`
- `snapshot(): Record<string, unknown>`

## Concurrency

### AgentPool

**Constructor:** `new AgentPool({ maxConcurrent?, scopePolicy? })`

**Methods:**
- `dispatch(tasks: PoolTask[]): Promise<PoolResult[]>`
