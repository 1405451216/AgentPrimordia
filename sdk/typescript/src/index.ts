/**
 * @agentprimordia/sdk — AgentPrimordia TypeScript SDK
 *
 * Build cross-platform AI Agent applications with ReAct loop,
 * multi-provider LLM support, episodic memory, and tool system.
 * @packageDocumentation
 */

export { ReActAgent, HookManager, Lifecycle } from './agent/react-loop.js';
export type { ReActConfig, HookPoint, HookContext, HookFunc, StreamEvent } from './agent/react-loop.js';

// ===== Phase 1: Engine Enhancements =====
export { Session } from './agent/session.js';
export type { SessionOption } from './agent/session.js';
export { PromptTemplate, defaultSystemPrompt, codeAssistantTemplate, ragContextTemplate, formatRAGDocuments } from './agent/prompt-template.js';
export type { RAGDoc } from './agent/prompt-template.js';
export {
  newRequestID, withRequestID, getRequestID,
  KeepLastNStrategy, TokenBudgetStrategy,
  InMemoryCheckpointStore,
  HITLManager,
  CostTracker,
} from './agent/request-id.js';
export type {
  ContextWindowStrategy, Checkpoint, CheckpointStore,
  InterruptReason, InterruptRequest, InterruptResponse, InterruptHandler, HITLConfig,
  ModelPricing, CostRecord, BudgetConfig, CostSummary,
} from './agent/request-id.js';
export { CapabilityAgent, newAgent, NoopTracer } from './agent/capability-agent.js';
export type {
  AgentOption, MemoryCapable, RAGCapable, HookCapable, TraceCapable,
  CostCapable, CheckpointCapable, HITLCapable, ContextWindowCapable,
  Tracer, Span, SpanKind,
  RAGProvider, RAGDocument as RAGDocCap, RAGMode, RAGConfig,
} from './agent/capability-agent.js';
export { LifecycleManager } from './agent/lifecycle-extended.js';
export type {
  AgentStatus as LifecycleAgentStatus, AgentStats,
  WorkflowType as WorkflowTypeExt, WorkflowStatus as WorkflowStatusExt,
  WorkflowNode as WorkflowNodeExt, WorkflowTransition, WorkflowConfig as WorkflowConfigExt,
  WorkflowContext, WorkflowResult as WorkflowResultExt, WorkflowEvent,
} from './agent/lifecycle-extended.js';

export { MockProvider } from './llm/provider.js';
export type { Provider } from './llm/provider.js';
export { OpenAIProvider, APIError } from './llm/openai.js';
export { ResilientProvider } from './llm/resilient.js';
export { AnthropicProvider } from './llm/anthropic.js';
export { GeminiProvider } from './llm/gemini.js';
export { OllamaProvider } from './llm/ollama.js';

// ===== Phase 2: Additional LLM Providers =====
export { DeepSeekProvider, QwenProvider, GLMProvider, MistralProvider, CohereProvider, AzureOpenAIProvider, OpenAICompatibleProvider } from './llm/providers.js';
export type { AzureConfig } from './llm/providers.js';
export {
  MultimodalAdapter, OpenAIMultimodalProvider,
  textContent, imageUrlContent, imageB64Content, audioContent, videoContent,
} from './llm/multimodal.js';
export type { MultimodalCapability, MultimodalContent, MultimodalMessage, MultimodalRequest, MultimodalResponse, MultimodalProvider } from './llm/multimodal.js';
export {
  InMemoryCache, FingerprintCache, CachedProvider,
  StructuredExtractor, schemaFromStruct,
  SentimentSchema, ClassificationSchema, SummarySchema, NERSchema,
  RateLimiter, BatchProcessor,
} from './llm/cache-structured.js';
export type { CacheStats, CacheEntry, LLMCache, SchemaDef, ExtractorConfig, BatchRequest, BatchResult } from './llm/cache-structured.js';

export { ToolRegistry } from './tools/registry.js';
export { FileScopePolicy } from './tools/scope.js';

// ===== Phase 3: Built-in Tools & Plugin System =====
export {
  FileSystemTool, ShellTool, WebTool, APITool,
  DatabaseTool, CodeExecutionTool, KnowledgeTool,
  JSONLoader, CSVLoader, HTMLLoader, MarkdownLoader, TextSplitter,
  PluginLoader, defaultToolkit,
} from './tools/builtin/index.js';
export type {
  FileSystemConfig, ShellConfig, WebConfig,
  LoadedDocument, ToolPlugin, PluginContext, ToolkitConfig,
} from './tools/builtin/index.js';
export { ToolPermission, ScopedExecutor } from './tools/scope-extended.js';
export type { ScopeRule, PermissionRequest, PermissionResult, PermissionHandler } from './tools/scope-extended.js';

export { InMemoryStore } from './memory/store.js';
export type { Memory } from './memory/store.js';
export { VectorStore } from './memory/vector.js';
export { SqliteStore } from './memory/sqlite-store.js';

// ===== Phase 4: RAG System =====
export { RAGStore, RAGPipeline, RAGReranker, Summarizer, MemoryCompressor } from './memory/rag.js';
export type { RAGDocument, RAGStoreConfig, RAGPipelineConfig, RerankOptions, SummarizerConfig } from './memory/rag.js';

export { AgentPool } from './pool/agent-pool.js';
export type { PoolTask, PoolResult } from './pool/agent-pool.js';

export { Bus } from './events/bus.js';
export type { EventType, Event } from './events/bus.js';

export { ACL, Sandbox } from './security/sandbox.js';
export type { AccessLevel } from './security/sandbox.js';

// ===== Phase 6: Guardrails =====
export { PIIDetector, InjectionDetector, TopicFilter, OutputGuardrail, GuardrailEngine } from './security/guardrails.js';
export type { PIIPattern, PIIDetectorConfig, PIIDetectionResult, InjectionDetectionResult, TopicFilterConfig, OutputRule, GuardrailConfig, GuardrailResult } from './security/guardrails.js';
export { containsShellMetacharacter, validatePathTraversal, resolvePathSafe, InputSanitizer, CommandGuard } from './security/extended.js';

export { MetricsCollector } from './metrics/collector.js';

// ===== Phase 8: Observability =====
export { MetricsRegistry, AgentMetrics as ObservabilityMetrics, PrometheusExporter, OTelTracer, Debugger, HealthChecker } from './metrics/otel-prometheus.js';
export type { MetricSample, MetricDefinition, OTelSpan, DebugEvent, HealthStatus } from './metrics/otel-prometheus.js';

// ===== Phase 9: Advanced Utilities =====
export { ConfigWatcher, BufferPool, StructuredLogger, defaultLogger, AsyncMemoryWriter, EventBus } from './utils/advanced.js';
export type { ConfigWatcherOptions, LogLevel, LogEntry, AsyncMemoryWriterConfig, EventHandler, EventBusSubscription } from './utils/advanced.js';

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

export { ValidationError, requireNonEmpty, requirePositiveInt, requireNonNegative, requireValidUrl, requireApiKey, requireInRange, validateTemperature, validateMaxTokens, validateMessages, validateToolName, validateAgentInput, validateModelName } from './validate.js';

export { Pipeline, ParallelRun, Handoff } from './orchestration/pipeline.js';
export type { PipelineStep, StepResult } from './orchestration/pipeline.js';

// ===== Phase 5: Advanced Orchestration =====
export { DAGBuilder, DAGWorkflow, GroupChat, Debate, Supervisor, WorkflowExecution, PlanBuilder } from './orchestration/advanced.js';
export type {
  DAGNode, DAGEdge, DAGContext, DAGNodeResult, DAGResult,
  GroupChatConfig, GroupChatMessage, GroupChatResult,
  DebateConfig, DebateResult,
  SupervisorConfig, SupervisorResult,
  WorkflowType, WorkflowStatus, WorkflowNode, WorkflowConfig, WorkflowResult,
  PlanStep, Plan,
} from './orchestration/advanced.js';

export { A2ABus } from './a2a/bus.js';
export type { AgentMessage, MessageHandler } from './a2a/bus.js';

// ===== Phase 7: A2A Communication =====
export { HTTPTransport, TCPTransport, AgentDiscovery, A2AAuth, A2AAgentServer, A2AClient } from './a2a/transport.js';
export type { A2AMessage, A2AAgentInfo, A2ATransport, HTTPTransportConfig, TCPTransportConfig, DiscoveryConfig, AuthConfig, A2AServerConfig } from './a2a/transport.js';

export { MCPClient } from './mcp/types.js';
export type { MCPServerConfig, MCPToolDefinition, MCPToolCall, MCPToolResult, MCPListToolsResponse, MCPServerInfo, MCPResource, MCPContentBlock } from './mcp/types.js';

// ===== Phase 11: MCP Registry & Adapter =====
export { MCPRegistry, MCPAdapter, JSONRPCHandler, A2ATaskManager, A2ABridge, A2ASSETransport } from './mcp/registry-adapter.js';
export type { RegisteredMCPServer, JSONRPCRequest, JSONRPCNotification, JSONRPCError, JSONRPCResponse, A2ATask, A2ATaskState, A2AArtifact, BridgeAgent } from './mcp/registry-adapter.js';

// ===== Phase 12: Reflection, Planning, Tool Learning, Eval =====
export { LLMReflector } from './agent/reflection.js';
export type { Reflection, Critique, Issue, Correction, Severity, Reflector } from './agent/reflection.js';
export { LLMPlanner, PlanExecutor } from './agent/planning.js';
export type { SubTask, Plan as TaskPlan, TaskStatus, Planner, PlanExecutorConfig } from './agent/planning.js';
export { MemoryToolLearner } from './agent/tool-learning.js';
export type { BestPractice, Suggestion, ToolUsageRecord, ToolLearner } from './agent/tool-learning.js';
export { ExactMatchEvaluator, ContainsEvaluator, RegexEvaluator, LLMEvaluator, CompositeEvaluator, EvalSuite } from './agent/eval.js';
export type { EvalToolCall, EvalResponse, EvalInput, CriterionResult, EvalResult, EvalCase, CaseResult, EvalSuiteResult, Evaluator, EvalSuiteConfig } from './agent/eval.js';

// ===== Phase 13: Visualization =====
export { MermaidGenerator, DOTGenerator, WorkflowVisualizer, VisualEditor } from './agent/visualize.js';
export type { VisualizeConfig, VizNode, VizTransition, VizWorkflow, EditorAction, EditorState } from './agent/visualize.js';

// ===== Phase 14: SSE, Stream Middleware, Stream Collector, Context Compress =====
export { SSEWriter, createSSEResponse, StreamPipeline, filterEmpty, bufferMiddleware, rateLimitMiddleware, transformMiddleware, logMiddleware, StreamCollector, CompressStrategy } from './agent/stream-extended.js';
export type { SSEEventType, SSEEvent, SSEResponseOptions, StreamHandler, StreamMiddleware, CollectedResult, CompressConfig } from './agent/stream-extended.js';

// ===== Phase 15: AutoScaler, Dispatcher, FileLock, ConcurrencyPool =====
export { AutoScaler, Dispatcher, FileLock, ConcurrencyPool } from './pool/dispatcher-autoscaler.js';
export type { AutoScalerConfig, DispatchTask, DispatcherConfig, ConcurrencyPool as ConcurrencyPoolType } from './pool/dispatcher-autoscaler.js';

// ===== Phase 16: K8s Operator CRD =====
export { basicAgentDeployment, multiAgentDeployment, withAutoscaling, withHealthCheck, withMetrics, withTracing, toYAML } from './operator/crd.js';
export type { AgentDeployment, AgentDeploymentSpec, AgentTemplateSpec, ToolSpec, MemorySpec, ResourceSpec, AutoscalingSpec, HealthCheckSpec, MetricsSpec, TracingSpec, AgentDeploymentStatus, Condition } from './operator/crd.js';

// ===== Phase 17: OTel Extensions =====
export { Baggage, BaggagePropagator, OTelBridge, OTLPExporter, MetricExporter } from './metrics/otel-extended.js';
export type { BaggageEntry, OTelSpan as OTelBridgeSpan, OTLPExporterConfig, MetricDataPoint } from './metrics/otel-extended.js';

// ===== Phase 18: Prompt Engine =====
export { PromptEngine, FewShotPrompt, PromptParser, PromptRegistry } from './prompt/engine.js';
export type { PromptTemplate as PromptTpl, FewShotExample, ParsedPrompt } from './prompt/engine.js';

// ===== Phase 19: Resilience =====
export { CircuitBreaker, Retry, ResilientWrapper, FallbackHandler } from './resilience/circuit-retry.js';
export type { CircuitBreakerConfig, CircuitState, RetryConfig, ResilientConfig } from './resilience/circuit-retry.js';

// ===== Phase 20: Extended Vector Stores =====
export { HNSW, MilvusProvider, QdrantProvider, ConversationalMemory, SharedStore } from './memory/vector-extended.js';
export type { HNSWConfig, MilvusConfig, QdrantConfig, ConversationalMemoryConfig, SharedStoreEntry } from './memory/vector-extended.js';

// ===== Phase 21: Document Loaders, Data Tools, Tool Cache, Trie Rule =====
export { PDFLoader, DOCXLoader, DataTools, ToolCache, TrieRule } from './tools/document-loaders.js';
export type { LoadedDocument as DocLoadedDocument, TextSplitterConfig } from './tools/document-loaders.js';

// ===== Phase 22: Extended Orchestration =====
export { DynamicOrchestrator, Scheduler, StepExecutor, WorkerPool, OrchestrationVisualizer, CollaborationHub } from './orchestration/extended.js';
export type {
  DynamicRouter, DynamicRoute, TaskPriority, ScheduledTask,
  StepType as WorkflowStepType, WorkflowStep, StepConfig, StepResult as StepExecResult,
  WorkerPoolConfig, OrchNode, OrchEdge, CollaborationMessage,
} from './orchestration/extended.js';

// ===== Phase 23: Zero-Copy Optimization, Pricing, StringBuilder =====
export { ZeroCopyMessage, batchConvertToZeroCopy, ZeroCopyPool, StringBuilder, ByteBufferPool, PricingCalculator, defaultPricingTable } from './utils/zerocopy-pricing.js';
export type { ModelPricing as ModelPricingInfo } from './utils/zerocopy-pricing.js';

// ===== Phase 24: Infrastructure Parity — Audit, Admin, Debugger, Persist, Health =====

// Audit Logger
export { AuditLogger, InMemoryAuditOutput } from './audit/logger.js';
export type { AuditEvent, QueryFilter, ActorStats, PeriodStats, ComplianceReport, AuditOutput, AuditLoggerConfig } from './audit/logger.js';

// Admin HTTP API
export { AdminHandler } from './admin/handler.js';
export type { AdminHandlerConfig, AdminTaskInfo, AdminPoolStats } from './admin/handler.js';

// Inspector & Debug Server
export { Inspector, InspectorServer, DebugServer } from './debugger/server.js';
export type { InspectorConfig, MemorySnapshot, SessionTrace } from './debugger/server.js';

// SQLite Checkpoint Store
export { SQLiteCheckpointStore } from './persist/sqlite-checkpoint.js';
export type { AgentState } from './persist/sqlite-checkpoint.js';

// Health HTTP Handler
export { HealthServer } from './health/http.js';
export type { HealthCheck, HealthServerConfig } from './health/http.js';
