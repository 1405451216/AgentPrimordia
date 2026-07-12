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

### CapabilityAgent / newAgent

Protocol-based microkernel with chainable `WithXxx()` API.

```typescript
const agent = newAgent('my-agent', 'system prompt', provider)
  .WithToolkit(registry)
  .WithMemory(memory)
  .WithMaxTurns(50);
```

### Session

Session-scoped agent management with message history isolation.

**Methods:**
- `run(input: string): Promise<Response>`
- `getHistory(): Message[]`
- `clear(): void`

### PromptTemplate

System prompt with variable substitution and scope rule injection.

**Methods:**
- `render(vars: Record<string, string>): string`

### HITLManager

Human-in-the-loop tool confirmation.

**Methods:**
- `shouldInterrupt(toolName: string): boolean`
- `requestInterrupt(req: InterruptRequest): Promise<InterruptResponse>`

### CostTracker

LLM cost tracking with budget limits and per-model/per-provider breakdown.

**Methods:**
- `record(model, provider, inputTokens, outputTokens): number`
- `checkBudget(): boolean`
- `summary(): CostSummary`

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

### AnthropicProvider

**Constructor:** `new AnthropicProvider(config: ProviderConfig)`

### GeminiProvider

**Constructor:** `new GeminiProvider(config: ProviderConfig)`

### OllamaProvider

**Constructor:** `new OllamaProvider(config: ProviderConfig)`

### DeepSeekProvider / QwenProvider / GLMProvider / MistralProvider / CohereProvider

**Constructor:** `new XxxProvider(config: ProviderConfig)`

### AzureOpenAIProvider

**Constructor:** `new AzureOpenAIProvider(config: AzureConfig)`

### OpenAICompatibleProvider

**Constructor:** `new OpenAICompatibleProvider(config: ProviderConfig)`

### MockProvider

**Constructor:** `new MockProvider(opts: { response?, toolCalls?, error?, delay? })`

### ResilientProvider

**Constructor:** `new ResilientProvider(primary: Provider, opts?)`

**Methods:**
- `addFallback(provider: Provider): void`

### MultimodalAdapter

Image/audio/video content support for LLM providers.

**Methods:**
- `content(text, images?, audio?): MultimodalMessage`

### InMemoryCache / FingerprintCache / CachedProvider

LLM response caching with fingerprint-based cache keys.

### StructuredExtractor

Schema-based structured output extraction from LLM responses.

## Tools

### ToolRegistry

**Methods:**
- `register(tool: Tool): void`
- `get(name: string): Tool | undefined`
- `list(): Tool[]`
- `definitions(): ToolDefinition[]`
- `execute(call: ToolCall): Promise<ToolResult>`
- `remove(name: string): boolean`
- `size(): number`

### FileScopePolicy / ToolPermission / ScopedExecutor

File access scope and tool execution permission control.

### Built-in Tools

- `FileSystemTool` — File read/write/search
- `ShellTool` — Command execution with whitelist
- `WebTool` — HTTP fetch and search
- `APITool` — REST API calls
- `DatabaseTool` — SQL query execution
- `CodeExecutionTool` — Sandboxed code execution
- `KnowledgeTool` — Knowledge base query

### Document Loaders

- `JSONLoader` / `CSVLoader` / `HTMLLoader` / `MarkdownLoader`
- `PDFLoader` / `DOCXLoader`
- `TextSplitter` — Text chunking

## Memory

### InMemoryStore

Implements the `Memory` interface with in-memory storage.

### SqliteStore

SQLite persistent memory with FTS5 full-text search.

**Constructor:** `new SqliteStore(dbPath: string)`

### VectorStore

**Constructor:** `new VectorStore(dimensions?: number)`

**Methods:**
- `add(id, vector, metadata?): void`
- `search(query, topK?): VectorSearchResult[]`
- `delete(id): boolean`
- `get(id): metadata | undefined`
- `count(): number`

### HNSW

Hierarchical Navigable Small World vector index.

**Constructor:** `new HNSW(config: HNSWConfig)`

### MilvusProvider / QdrantProvider

External vector database providers.

### RAGStore / RAGPipeline / RAGReranker

RAG retrieval-augmented generation pipeline with hybrid search (FTS + Vector).

**RAGStore** supports two fusion modes:

- `'linear'` — Weighted score fusion (default)
- `'rrf'` — Reciprocal Rank Fusion, robust to score scale differences

```typescript
const store = new RAGStore(memory, embedder, {
  fusionMode: 'rrf',
  rrfK: 60,
  overFetchSize: 5,
});

// 运行时切换融合模式
store.setFusionConfig({ fusionMode: 'linear', ftsWeight: 0.4, vectorWeight: 0.6 });
```

**Retrieval flow:** Query → Embedding → Vector Search → FTS Search → RRF Fusion → Rerank → TopK → Context injection

### ConversationalMemory / SharedStore

Multi-agent shared memory with conversation context.

## Orchestration

### Pipeline / ParallelRun / Handoff

Basic orchestration patterns for sequential, parallel, and handoff workflows.

### DAGBuilder / DAGWorkflow

DAG workflow engine with cycle detection and conditional edges.

**Methods:**
- `addNode(id, agent): DAGBuilder`
- `addEdge(from, to, condition?): DAGBuilder`
- `build(): DAGWorkflow`
- `execute(ctx): Promise<DAGResult>`

### GroupChat / Debate / Supervisor

Multi-agent collaboration patterns.

### DynamicOrchestrator / Scheduler / StepExecutor / WorkerPool

Advanced orchestration with dynamic routing and task scheduling.

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

### GuardrailEngine

**Methods:**
- `checkInput(text): GuardrailResult`
- `checkOutput(text): GuardrailResult`

### PIIDetector / InjectionDetector / TopicFilter

Input/output content filtering and PII detection.

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

### MetricsRegistry / PrometheusExporter

Prometheus-format metrics export.

### OTelTracer / OTelBridge / OTLPExporter

OpenTelemetry distributed tracing.

### Debugger

Debug event collection and report generation.

**Methods:**
- `logEvent(type, data): void`
- `getEvents(): DebugEvent[]`
- `report(): string`

## Concurrency

### AgentPool

**Constructor:** `new AgentPool({ maxConcurrent?, scopePolicy? })`

**Methods:**
- `dispatch(tasks: PoolTask[]): Promise<PoolResult[]>`

### AutoScaler / Dispatcher

**Constructor:** `new AutoScaler(config: AutoScalerConfig)`

### ConcurrencyPool

Semaphore-based concurrency control with direct handoff release.

### FileLock

File-level concurrent write lock.

## A2A Communication

### A2ABus

**Methods:**
- `send(msg: AgentMessage): void`
- `subscribe(handler: MessageHandler): string`
- `unsubscribe(id: void`

### HTTPTransport / TCPTransport

Cross-process agent communication transports.

### AgentDiscovery / A2AAgentServer / A2AClient

Agent discovery protocol, server, and client.

## MCP Protocol

### MCPClient / MCPRegistry / MCPAdapter

Model Context Protocol client, server registry, and tool adapter.

## Resilience

### CircuitBreaker / Retry / ResilientWrapper

**Constructor:** `new CircuitBreaker(config: CircuitBreakerConfig)`

**Methods:**
- `execute(fn): Promise<T>`
- `state(): CircuitState`

## Infrastructure (Phase 24)

### AuditLogger

Compliance audit trail with query and report generation.

**Constructor:** `new AuditLogger({ output: AuditOutput })`

**Methods:**
- `log(event): Promise<void>`
- `query(filter: QueryFilter): Promise<AuditEvent[]>`
- `generateReport(start, end): Promise<ComplianceReport>`
- `exportReportJSON(report): string`

### InMemoryAuditOutput

Default in-memory audit output backend.

**Methods:**
- `write(event: AuditEvent): Promise<void>`
- `query(filter: QueryFilter): Promise<AuditEvent[]>`

### AdminHandler

Bearer Token authenticated management HTTP API with embedded Web UI.

**Constructor:** `new AdminHandler(config: AdminHandlerConfig)`

**Methods:**
- `handle(req, res): Promise<void>`

**Endpoints:**
- `GET /api/health` — Public health check
- `GET /api/agents` — List agents (auth required)
- `GET /api/agents/:id` — Get agent details (auth required)
- `GET /api/stats` — Pool statistics (auth required)
- `GET /api/tasks` — Task list (auth required)
- `GET /api/tools` — Tool registry (auth required)
- `GET /api/tools/:name` — Tool details (auth required)
- `GET /api/workflows` — Workflow list (auth required)
- `GET /api/workflows/:id` — Workflow details (auth required)
- `GET /api/system` — System info (auth required)
- `GET /api/logs/stream` — SSE log stream (auth required)
- `GET /` — Web UI dashboard

### Inspector / InspectorServer

Agent trace inspection with Web UI.

**Inspector Methods:**
- `recordSpan(span: OTelSpan): void`
- `recordSession(trace: SessionTrace): void`
- `getTraces(): OTelSpan[]`
- `getSessionTrace(sessionID): SessionTrace | undefined`
- `getStats(): { totalTraces, totalSessions }`

**InspectorServer Endpoints:**
- `GET /inspector` — Web UI
- `GET /api/inspector/traces` — All traces
- `GET /api/inspector/sessions` — Session list
- `GET /api/inspector/session/:id` — Session trace
- `GET /api/inspector/stats` — Inspector statistics

### DebugServer

Debug event and memory snapshot HTTP server with Web UI.

**Endpoints:**
- `GET /` — Web UI
- `GET /api/events` — Debug events
- `GET /api/snapshots` — Memory snapshots
- `GET /api/debug/report` — Plain text debug report

### SQLiteCheckpointStore

SQLite-based agent state persistence (implements `CheckpointStore`).

**Constructor:** `new SQLiteCheckpointStore(dbPath: string)`

**Static:** `SQLiteCheckpointStore.inMemory()` — In-memory instance for testing

**Methods:**
- `save(checkpoint: Checkpoint): Promise<void>`
- `load(id: string): Promise<Checkpoint | null>`
- `list(sessionID: string): Promise<Checkpoint[]>`
- `delete(id: string): Promise<void>`
- `saveState(state: AgentState): Promise<void>`
- `loadState(agentID: string): Promise<AgentState | null>`
- `listStates(sessionID: string): Promise<AgentState[]>`
- `deleteState(agentID: string): Promise<void>`
- `close(): void`

### HealthServer

HTTP health check endpoints for Kubernetes/container orchestration.

**Constructor:** `new HealthServer(config?: HealthServerConfig)`

**Methods:**
- `setReady(ready: boolean): void`
- `isReady(): boolean`
- `uptime(): number`
- `handle(req, res): Promise<void>`

**Endpoints:**
- `GET /healthz` — Liveness check (always 200 if process alive)
- `GET /readyz` — Readiness check (503 if not ready)
- `GET /livez` — Kubernetes liveness probe

## Utilities

### ZeroCopyPool / ZeroCopyMessage

Zero-copy message optimization for high-throughput scenarios.

### StringBuilder / ByteBufferPool

Buffer reuse utilities to reduce GC pressure.

### PricingCalculator

LLM cost calculator with default pricing table for major models.

### ConfigWatcher / StructuredLogger / EventBus

Advanced utilities for configuration hot-reload, structured logging, and event handling.
