# @agentprimordia/sdk

TypeScript SDK for AgentPrimordia — Universal AI Agent Development Framework.

**100% feature parity with the Go framework.**

## Installation

```bash
npm install @agentprimordia/sdk
```

### Optional Peer Dependencies

```bash
# SQLite persistent storage (memory + checkpoints)
npm install better-sqlite3
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
- `CapabilityAgent` / `newAgent` — Protocol-based microkernel with `WithXxx()` chainable API
- `LifecycleManager` — Extended lifecycle with workflow state machine
- `Session` — Session-scoped agent management
- `PromptTemplate` — System prompt with variable substitution and scope rules
- `LLMReflector` — Self-critique and reflection
- `LLMPlanner` / `PlanExecutor` — Task decomposition and plan execution
- `MemoryToolLearner` — Best practice extraction from tool usage
- `ExactMatchEvaluator` / `LLMEvaluator` / `EvalSuite` — Agent evaluation
- `MermaidGenerator` / `DOTGenerator` / `VisualEditor` — Workflow visualization
- `SSEWriter` / `StreamPipeline` / `StreamCollector` — SSE streaming middleware
- `HITLManager` — Human-in-the-loop tool confirmation
- `CostTracker` — LLM cost tracking with budget limits

### LLM Providers
- `MockProvider` — Test provider with configurable responses
- `OpenAIProvider` — OpenAI API integration (gpt-4o, gpt-4o-mini)
- `AnthropicProvider` — Claude series model support
- `GeminiProvider` — Google Gemini support
- `OllamaProvider` — Local Ollama support
- `DeepSeekProvider` / `QwenProvider` / `GLMProvider` / `MistralProvider` / `CohereProvider` — Additional providers
- `AzureOpenAIProvider` / `OpenAICompatibleProvider` — Azure & OpenAI-compatible APIs
- `ResilientProvider` — Retry + circuit breaker + fallback wrapper
- `MultimodalAdapter` — Image/audio/video content support
- `InMemoryCache` / `FingerprintCache` / `CachedProvider` — LLM response caching
- `StructuredExtractor` — Schema-based structured output extraction
- `RateLimiter` / `BatchProcessor` — Rate limiting and batching

### Tools
- `ToolRegistry` — Register and execute tools
- `FileScopePolicy` / `ToolPermission` / `ScopedExecutor` — File access scope and permissions
- `FileSystemTool` / `ShellTool` / `WebTool` / `APITool` / `DatabaseTool` / `CodeExecutionTool` / `KnowledgeTool` — Built-in tools
- `JSONLoader` / `CSVLoader` / `HTMLLoader` / `MarkdownLoader` / `PDFLoader` / `DOCXLoader` — Document loaders
- `PluginLoader` — Dynamic plugin loading
- `ToolCache` / `TrieRule` — Tool caching and rule matching

### Memory
- `InMemoryStore` — In-memory episodic memory store
- `SqliteStore` — SQLite persistent memory with FTS5 full-text search
- `VectorStore` — Cosine similarity vector search
- `HNSW` — Hierarchical Navigable Small World vector index
- `MilvusProvider` / `QdrantProvider` — External vector database providers
- `RAGStore` / `RAGPipeline` / `RAGReranker` — RAG retrieval-augmented generation
- `Summarizer` / `MemoryCompressor` — Memory summarization and compression
- `ConversationalMemory` / `SharedStore` — Multi-agent shared memory

### Orchestration
- `Pipeline` / `ParallelRun` / `Handoff` — Basic orchestration patterns
- `DAGBuilder` / `DAGWorkflow` — DAG workflow engine with cycle detection
- `GroupChat` / `Debate` / `Supervisor` — Multi-agent collaboration
- `DynamicOrchestrator` / `Scheduler` / `StepExecutor` / `WorkerPool` — Advanced orchestration
- `CollaborationHub` — Agent collaboration center
- `PlanBuilder` — Plan-based workflow construction

### Concurrency & Pool
- `AgentPool` — Concurrent agent task execution
- `AutoScaler` / `Dispatcher` — Auto-scaling task dispatcher
- `ConcurrencyPool` — Semaphore-based concurrency control
- `FileLock` — File-level concurrent write lock

### Security
- `ACL` / `Sandbox` — Access control list and command execution sandbox
- `PIIDetector` / `InjectionDetector` / `TopicFilter` / `GuardrailEngine` — Input/output guardrails
- `InputSanitizer` / `CommandGuard` — Shell metacharacter and path traversal protection

### Observability
- `MetricsCollector` / `MetricsRegistry` — LLM/tool call metrics
- `PrometheusExporter` — Prometheus-format metrics export
- `OTelTracer` / `OTelBridge` / `OTLPExporter` — OpenTelemetry tracing
- `Debugger` — Debug event collection and report generation
- `Baggage` / `BaggagePropagator` — Distributed context propagation

### A2A Communication
- `A2ABus` — Agent-to-agent message bus
- `HTTPTransport` / `TCPTransport` — Cross-process agent communication
- `AgentDiscovery` — Agent discovery protocol
- `A2AAuth` / `A2AAgentServer` / `A2AClient` — A2A server and client

### MCP Protocol
- `MCPClient` — Model Context Protocol client
- `MCPRegistry` / `MCPAdapter` — MCP server registry and tool adapter
- `JSONRPCHandler` / `A2ABridge` — JSON-RPC and A2A bridge

### Resilience
- `CircuitBreaker` / `Retry` / `ResilientWrapper` / `FallbackHandler` — Fault tolerance

### Prompt Engineering
- `PromptEngine` / `FewShotPrompt` / `PromptParser` / `PromptRegistry` — Prompt management

### K8s Operator
- `basicAgentDeployment` / `multiAgentDeployment` / `withAutoscaling` — CRD generators
- `toYAML` — YAML serialization

### Infrastructure (Phase 24 — Go Parity)
- `AuditLogger` — Compliance audit trail with query and report generation
- `AdminHandler` — Bearer Token authenticated management HTTP API with Web UI
- `Inspector` / `InspectorServer` — Agent trace inspection with Web UI
- `DebugServer` — Debug event and memory snapshot HTTP server
- `SQLiteCheckpointStore` — SQLite-based agent state persistence
- `HealthServer` — `/healthz`, `/readyz`, `/livez` HTTP endpoints

### Utilities
- `ZeroCopyPool` / `ZeroCopyMessage` — Zero-copy message optimization
- `StringBuilder` / `ByteBufferPool` — Buffer reuse utilities
- `PricingCalculator` — LLM cost calculator
- `ConfigWatcher` / `StructuredLogger` / `EventBus` — Advanced utilities

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

## Infrastructure Endpoints

### Health Checks

```typescript
import { HealthServer } from '@agentprimordia/sdk';
import { createServer } from 'node:http';

const health = new HealthServer();
health.setReady(true);

const server = createServer((req, res) => {
  if (req.url?.startsWith('/health') || req.url?.startsWith('/ready') || req.url?.startsWith('/live')) {
    health.handle(req, res);
  }
});
server.listen(8080);
// GET /healthz → 200 {"status":"ok","uptime":"42s"}
// GET /readyz  → 200 {"status":"ready","checks":[...]}
// GET /livez   → 200 {"status":"ok"}
```

### Admin API

```typescript
import { AdminHandler } from '@agentprimordia/sdk';

const admin = new AdminHandler({
  apiToken: 'my-secret-token',
  registry: toolRegistry,
  getStats: () => ({ totalTasks: 100, completedTasks: 95, ... }),
  getAgents: () => ({ 'agent-1': 'running' }),
});
// Protected endpoints: /api/agents, /api/stats, /api/tasks, /api/tools, /api/workflows
// Web UI at: /
```

### Audit Logging

```typescript
import { AuditLogger, InMemoryAuditOutput } from '@agentprimordia/sdk';

const logger = new AuditLogger({ output: new InMemoryAuditOutput() });
await logger.log({ actor: 'user-1', action: 'tool.execute', resource: 'shell' });
const events = await logger.query({ actor: 'user-1' });
const report = await logger.generateReport('2026-01-01', '2026-12-31');
```

### SQLite Checkpoint Persistence

```typescript
import { SQLiteCheckpointStore } from '@agentprimordia/sdk';

const store = new SQLiteCheckpointStore('./checkpoints.db');
await store.saveState({
  agentID: 'agent-1',
  sessionID: 'session-1',
  status: 'running',
  messages: [{ role: 'user', content: 'Hello' }],
  turnCount: 3,
  metrics: { totalTurns: 3, totalTools: 1, duration: '5s' },
  savedAt: new Date().toISOString(),
});
const state = await store.loadState('agent-1');
```

## Documentation

- [Getting Started](./docs/guide/getting-started.md)
- [Agent Lifecycle](./docs/guide/agent.md)
- [Memory System](./docs/guide/memory.md)
- [Tool System](./docs/guide/tools.md)
- [API Reference](./docs/api/index.md)

## Go Framework Parity

This SDK maintains 100% feature parity with the Go framework (`agentprimordia/internal/`).
Every Go module has a corresponding TypeScript implementation:

| Go (`internal/`) | TS (`src/`) | Status |
|---|---|---|
| `agent/` | `agent/` | ✅ Complete |
| `llm/` | `llm/` | ✅ Complete |
| `tools/` | `tools/` | ✅ Complete |
| `memory/` | `memory/` | ✅ Complete |
| `orchestration/` | `orchestration/` | ✅ Complete |
| `pool/` | `pool/` | ✅ Complete |
| `events/` | `events/` | ✅ Complete |
| `security/` | `security/` | ✅ Complete |
| `guardrail/` | `security/` | ✅ Complete |
| `metrics/` | `metrics/` | ✅ Complete |
| `otel/` | `metrics/` | ✅ Complete |
| `prompt/` | `prompt/` | ✅ Complete |
| `resilience/` | `resilience/` | ✅ Complete |
| `concurrency/` | `pool/` | ✅ Complete |
| `config/` | `utils/` | ✅ Complete |
| `logger/` | `utils/` | ✅ Complete |
| `jsonutil/` | `utils/` | ✅ Complete |
| `audit/` | `audit/` | ✅ Complete |
| `admin/` | `admin/` | ✅ Complete |
| `debugger/` | `debugger/` | ✅ Complete |
| `persist/` | `persist/` | ✅ Complete |
| `health/` | `health/` | ✅ Complete |

## License

Apache-2.0
