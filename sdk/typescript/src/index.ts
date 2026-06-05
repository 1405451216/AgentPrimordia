/**
 * @agentprimordia/sdk — AgentPrimordia TypeScript SDK
 *
 * Build cross-platform AI Agent applications with ReAct loop,
 * multi-provider LLM support, episodic memory, and tool system.
 * @packageDocumentation
 */

export { ReActAgent, HookManager, Lifecycle } from './agent/react-loop.js';
export type { ReActConfig, HookPoint, HookContext, HookFunc } from './agent/react-loop.js';

export { MockProvider } from './llm/provider.js';
export type { Provider } from './llm/provider.js';
export { OpenAIProvider, APIError } from './llm/openai.js';
export { ResilientProvider } from './llm/resilient.js';

export { ToolRegistry } from './tools/registry.js';
export { FileScopePolicy } from './tools/scope.js';

export { InMemoryStore } from './memory/store.js';
export type { Memory } from './memory/store.js';
export { VectorStore } from './memory/vector.js';
export { SqliteStore } from './memory/sqlite-store.js';

export { AgentPool } from './pool/agent-pool.js';
export type { PoolTask, PoolResult } from './pool/agent-pool.js';

export { Bus } from './events/bus.js';
export type { EventType, Event } from './events/bus.js';

export { ACL, Sandbox } from './security/sandbox.js';
export type { AccessLevel } from './security/sandbox.js';

export { MetricsCollector } from './metrics/collector.js';

export type {
  Message,
  ToolCall,
  ToolResult,
  ToolDefinition,
  Usage,
  CompletionRequest,
  CompletionResponse,
  ToolCallRequest,
  ToolCallResponse,
  Chunk,
  ModelInfo,
  ProviderConfig,
  AgentMetrics,
  Response,
  AgentStatus,
  Tool,
  MemoryEpisode,
  MemoryStats,
  SearchOptions,
  ListOptions,
  VectorSearchResult,
  ErrorCode,
} from './types.js';

export { VERSION, ErrorCodes } from './types.js';

export { Pipeline, ParallelRun, Handoff } from './orchestration/pipeline.js';
export type { PipelineStep, StepResult } from './orchestration/pipeline.js';

export { A2ABus } from './a2a/bus.js';
export type { AgentMessage, MessageHandler } from './a2a/bus.js';

export { MCPClient } from './mcp/types.js';
export type { MCPServerConfig, MCPToolDefinition, MCPToolCall, MCPToolResult, MCPListToolsResponse } from './mcp/types.js';
