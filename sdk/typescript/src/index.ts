/**
 * @agentprimordia/sdk — AgentPrimordia TypeScript SDK
 *
 * Build cross-platform AI Agent applications with ReAct loop,
 * multi-provider LLM support, episodic memory, and tool system.
 * @packageDocumentation
 */

export { ReActAgent, HookManager, Lifecycle } from './agent/react-loop.js';
export type { ReActConfig, HookPoint, HookContext, HookFunc, StreamEvent, RunOptions } from './agent/react-loop.js';

// ===== Phase 1: Engine Enhancements =====
export { Session } from './agent/session.js';
export type { SessionOption } from './agent/session.js';
export { PromptTemplate, defaultSystemPrompt, codeAssistantTemplate, ragContextTemplate, formatRAGDocuments } from './agent/prompt-template.js';
export type { RAGDoc } from './agent/prompt-template.js';

// ===== Phase 27: Prompt System + Document Loader =====
export { TemplateRegistry } from './prompt/registry.js';
export { FewShotTemplate, KeywordSelector } from './prompt/few-shot.js';
export type { Example, ExampleSelector, FewShotConfig } from './prompt/few-shot.js';
export { JSONParser, MarkdownParser, RegexParser } from './prompt/parser.js';
export type { OutputParser, JSONParserConfig } from './prompt/parser.js';
export { TextLoader, MDLoader, JSONDocLoader, CodeLoader, DirectoryLoader } from './prompt/document-loader.js';
export type { ExtractedDocument, FileDocLoader, TextLoaderConfig } from './prompt/document-loader.js';
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
export { MemoryFailureStore, diagnoseFailure } from './agent/failure.js';
export type { FailureRecord, FailureStore, FailurePhase, FailureFilter } from './agent/failure.js';
export { CapabilityAgent, newAgent, NoopTracer } from './agent/capability-agent.js';
export { AgentTool } from './agent/agent-tool.js';
export type { AgentToolConfig } from './agent/agent-tool.js';
export type {
  AgentOption, MemoryCapable, RAGCapable, HookCapable, TraceCapable,
  CostCapable, CheckpointCapable, HITLCapable, ContextWindowCapable, FailureCapable,
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
export { ResilientProvider, RateLimitedProvider } from './llm/resilient.js';
export type { RateLimitConfig } from './llm/resilient.js';
export { AnthropicProvider } from './llm/anthropic.js';
export { GeminiProvider } from './llm/gemini.js';
export { OllamaProvider } from './llm/ollama.js';

// ===== Phase 2: Additional LLM Providers =====
export { DeepSeekProvider, QwenProvider, GLMProvider, MistralProvider, CohereProvider, AzureOpenAIProvider, OpenAICompatibleProvider } from './llm/providers.js';
export type { AzureConfig } from './llm/providers.js';

// ===== Phase 26: LLM Batch + Structured Output + Config =====
export { BatchRequestProcessor, defaultBatchConfig } from './llm/batch.js';
export type { BatchConfig } from './llm/batch.js';
export { StructuredOutputExtractor } from './llm/structured-output.js';
export type { StructuredOutputConfig } from './llm/structured-output.js';
export { validateConfig, validateConfigOrThrow, configFromEnv, configFromEnvValidated, LLMConfigWatcher } from './llm/config.js';
export type { ConfigChangeCallback, LLMConfigWatcherOptions } from './llm/config.js';
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
export { RAGStore, RAGPipeline, RAGReranker, Summarizer, MemoryCompressor, defaultFusionConfig, MMRReranker } from './memory/rag.js';
export type { RAGDocument, RAGStoreConfig, RAGPipelineConfig, RerankOptions, SummarizerConfig, FusionMode, RAGFusionConfig, MMRConfig, MMRRerankOptions } from './memory/rag.js';

// ===== Phase 25: Memory Advanced Features =====

// Summarizer (Go-aligned)
export { LLMSummarizer, SimpleSummarizer } from './memory/summarizer.js';
export type { SummaryResult, SummaryExtractor, SummarizerConfig as LLMSummarizerConfig } from './memory/summarizer.js';

// Chat Memory (Go-aligned ConversationalMemory)
export { ChatMemory, DefaultCompressor } from './memory/conversational-memory.js';
export type { ChatMessage, SummaryCompressor, ChatMemoryConfig } from './memory/conversational-memory.js';

// Memory Compressor (Go-aligned)
export { Compressor, LLMCompressSummarizer } from './memory/compressor.js';
export type { CompressorConfig, CompressorSummary, CompressSummarizer } from './memory/compressor.js';

// Agent Shared Store (Go-aligned)
export { AgentSharedStore } from './memory/shared-store.js';

// RAG Pipeline (Go-aligned)
export { EnhancedRAGPipeline, SimpleTextLoader, registerSplitter, createSplitter, availableStrategies } from './memory/rag-pipeline.js';
export type { EnhancedRAGPipelineConfig, IngestResult, DocumentLoader, SplitterStrategy, SplitterConfig, RAGTextSplitter } from './memory/rag-pipeline.js';

export { AgentPool } from './pool/agent-pool.js';
export type { PoolTask, PoolResult } from './pool/agent-pool.js';

export { Bus } from './events/bus.js';
export type { EventType, Event } from './events/bus.js';

export { ACL, Sandbox, CommandSandbox, CodeSandbox, CodeSecurityChecker, newArgPattern } from './security/sandbox.js';
export type { AccessLevel, SandboxConfig, SandboxResult, ArgPattern } from './security/sandbox.js';

// CodeSandbox v2: WASM Runtime + VirtualFS + 多语言执行
export { VirtualFS, WasiShim, WasmRuntime, CodeSandboxV2 } from './security/sandbox-v2.js';
export type {
  VfsNodeType,
  VfsOpenFlags,
  WasmExecConfig,
  WasmExecResult,
  SandboxV2Config,
  ExecRequest,
  ExecResult,
  CodeLanguage,
} from './security/sandbox-v2.js';

// ===== Phase 6: Guardrails =====
export { PIIDetector, InjectionDetector, TopicFilter, OutputGuardrail, GuardrailEngine, Trie, Sanitizer, GuardrailHook, PromptInjectionRule, OutputSafetyRule, TopicConstraintRule, RuleEngine, normalizeForCheck } from './security/guardrails.js';
export type { PIIPattern, PIIDetectorConfig, PIIDetectionResult, InjectionDetectionResult, TopicFilterConfig, OutputRule, GuardrailConfig, GuardrailResult, SanitizeStrategy, SanitizerConfig, Position, GuardrailHookConfig, GuardrailHookContext, CheckPoint, GuardrailAction, GuardrailSeverity, GuardrailRuleResult, GuardrailReport, GuardrailRule, PromptInjectionRuleConfig, OutputSafetyRuleConfig, TopicMode, TopicConstraintRuleConfig } from './security/guardrails.js';
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
export type { MCPServerConfig, MCPToolDefinition, MCPToolCall, MCPToolResult, MCPListToolsResponse, MCPServerInfo, MCPResource, MCPContentBlock, MCPPromptDefinition } from './mcp/types.js';

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

// ===== Phase 14b: Context Window & Token Estimation =====
export { ContextWindow, estimateTokens, estimateTokenCount } from './agent/context-compress.js';
export type { ContextWindowConfig } from './agent/context-compress.js';

// ===== Phase 14c: Message Convert Utilities =====
export {
  toOpenAIMessages, toOpenAIToolDefinitions, fromOpenAIToolCalls, toOpenAIToolCalls,
  fromOpenAIMessage, fromOpenAIMessages, buildMultimodalContent as buildMultimodalContentFromParts,
  extractTextContent, hasMultimodal, summarizeHistory,
} from './agent/react-convert.js';
export type { OpenAIMessage, OpenAIContentItem, OpenAIToolCall, OpenAIToolDefinition, ContentPart, ExtendedMessage } from './agent/react-convert.js';

// ===== Phase 14d: Reasoning Engine =====
export { ReasoningEngine, singleRoundReasoning, singleRoundReasoningStream } from './agent/react-reasoning.js';
export type { Thought, StreamEvent as ReasoningStreamEvent, StreamEventType, ReasoningConfig } from './agent/react-reasoning.js';

// ===== Phase 14e: Distributed Agent Execution =====
export {
  LocalDiscovery, HTTPDiscoveryClient, TokenAuthenticator, AuthenticatedDiscovery,
  DistributedHTTPTransport, DistributedAgent,
} from './agent/distributed.js';
export type {
  AgentInfo, AgentIdentity, Discovery, Transport, BusMessage, BusMessageType,
  DistributedConfig,
} from './agent/distributed.js';

// ===== Phase 15: AutoScaler, Dispatcher, FileLock, ConcurrencyPool =====
export { AutoScaler, Dispatcher, FileLock, ConcurrencyPool } from './pool/dispatcher-autoscaler.js';
export type { AutoScalerConfig, DispatchTask, DispatcherConfig, ConcurrencyPool as ConcurrencyPoolType } from './pool/dispatcher-autoscaler.js';

// ===== Phase 16: K8s Operator CRD =====
export { basicAgentDeployment, multiAgentDeployment, withAutoscaling, withHealthCheck, withMetrics, withTracing, toYAML } from './operator/crd.js';
export type { AgentDeployment, AgentDeploymentSpec, AgentTemplateSpec, ToolSpec, MemorySpec, ResourceSpec, AutoscalingSpec, HealthCheckSpec, MetricsSpec, TracingSpec, AgentDeploymentStatus, Condition } from './operator/crd.js';

// ===== Phase 17: OTel Extensions =====
export { Baggage, BaggagePropagator, OTelBridge, OTLPExporter, MetricExporter } from './metrics/otel-extended.js';
export { OfficialOTelBridge } from './metrics/otel-official.js';
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

// ===== JSON Util =====
export {
  ObjectPool, Marshal, Unmarshal, DecodeString, DecodeBuffer, MarshalBody,
  getRecord, putRecord, getArray, putArray, recordPoolSize, arrayPoolSize,
} from './jsonutil/pool.js';

// ===== Phase 2: P1 差异化能力 =====

// Edge Runtime 适配
export { detectRuntime, getWebSocketConstructor, createTimer, supports, resetRuntimeCache } from './edge/runtime.js';
export type { RuntimeInfo, RuntimeName } from './edge/runtime.js';

// WebSocket 双向流传输
export { WebSocketTransport, WebSocketA2AServer } from './a2a/websocket-transport.js';
export type { WebSocketTransportConfig, WSMessageHandler, WSConnectionHandler, WSErrorHandler } from './a2a/websocket-transport.js';

// 类型安全 Builder DSL
export { createAgent, createBasicAgent, createAgentWithMemory, createAgentWithBudget } from './agent/builder.js';

// Worker Threads 真并行
export { ComputeWorkerPool, isWorkerThreadsAvailable } from './agent/worker-pool.js';
export type { WorkerPoolConfig as ComputeWorkerPoolConfig, WorkerTask as ComputeWorkerTask, WorkerResult as ComputeWorkerResult } from './agent/worker-pool.js';

// 动态插件热加载
export { AgentPluginLoader, definePlugin } from './tools/plugin-loader.js';
export type { AgentPlugin, PluginManifest, PluginLoaderConfig, PluginAPI } from './tools/plugin-loader.js';

// 插件沙箱（Worker Thread 隔离执行）
export { PluginSandbox } from './tools/plugin-sandbox.js';
export type { PluginSandboxOptions } from './tools/plugin-sandbox.js';

// 插件注册中心（本地市场元数据 + 生命周期管理）
export { PluginRegistry } from './tools/plugin-registry.js';
export type { PluginMetadata, PluginRegistryOptions } from './tools/plugin-registry.js';

// ===== Phase 4: 性能优化与进化成长 =====

// Agent 自省与自我调优（TS 独有）
export { AgentSelfTuner } from './agent/self-tuning.js';
export type { RunMetrics, TuningSuggestion, IntrospectionStats } from './agent/self-tuning.js';

// ===== Phase 5: 高级进化能力 =====

// 投机执行（TS 独有）
export { SpeculativeExecutor, ToolResultPredictor } from './agent/speculative-exec.js';
export type { SpeculativeExecConfig, SpeculativeResult, SpeculationStats } from './agent/speculative-exec.js';

// Prompt A/B 测试
export { PromptABTest, KeywordEvaluator, CompletenessEvaluator, PromptLLMEvaluator } from './agent/prompt-ab-test.js';
export type { PromptVariant, ExperimentResult, ABTestResult, PromptEvaluator as ABTestPromptEvaluator, ABTestConfig } from './agent/prompt-ab-test.js';

// Tool Learning 闭环增强
export { EnhancedToolLearner } from './agent/tool-learning.js';
export type { FewShotExample as ToolFewShotExample, ToolUsagePattern, EnhancedToolLearningConfig } from './agent/tool-learning.js';

// Edge-Native 冷启动优化
export { ColdStartOptimizer, LazyLoader, ConnectionPrewarmer, initEdgeRuntime, lazyLoad } from './edge/cold-start.js';
export type { ColdStartReport, ColdStartSuggestion, ModuleLoadRecord } from './edge/cold-start.js';

// 分布式 Agent 编排（WebSocket 传输层）
export { DistributedOrchestrator } from './agent/distributed-orchestration.js';
export type { AgentNode, DistributedTask, DistributedTaskResult, MapReduceResult, DistributedOrchestrationConfig } from './agent/distributed-orchestration.js';

// Phase 6: 智能模型路由
export { ModelRouter, ComplexityEvaluator, estimateTokens as estimateTokensForRouting } from './llm/model-router.js';
export type { ModelRouteConfig, RouteDecision, RouteStrategy } from './llm/model-router.js';

// Phase 6: 流式编排管道
export { StreamingPipeline } from './orchestration/streaming-pipeline.js';
export type { StreamingPipelineStep, StreamingPipelineEvent } from './orchestration/streaming-pipeline.js';

// Phase 6: 长期记忆持久化
export { LongTermMemory, HashEmbedding } from './memory/long-term.js';
export type { LongTermMemoryConfig, HybridSearchResult } from './memory/long-term.js';

// Phase 6: 可视化监控面板
export { AgentMonitor, DashboardServer } from './agent/dashboard.js';
export type { AgentState as DashboardAgentState, TokenUsage, ToolCallStat, RunRecord, DashboardSnapshot } from './agent/dashboard.js';

// Phase 6: 多模态深度融合
export { MultimodalFusion, createMultimodalAgent } from './llm/multimodal-fusion.js';
export type { MultimodalInput, ModalityResult, FusionResult, MultimodalFusionConfig } from './llm/multimodal-fusion.js';

// ===== Phase C: 代码生成 + 包优化 + i18n + WebSocket =====

// 代码生成器
export { CodeGenerator } from './codegen/generator.js';
export type { ToolTemplate, ToolParameter } from './codegen/templates/tool.js';
export type { AgentTemplateConfig } from './codegen/templates/agent.js';
export type { ProviderTemplateConfig } from './codegen/templates/provider.js';

// i18n 国际化
export { setLocale, getLocale, t, interpolate, hasKey, getSupportedLocales, mergeMessages } from './i18n/index.js';
export type { Locale, LocaleKey } from './i18n/index.js';

// CRDT 协作编辑
export {
  LamportClock,
  LWWRegister,
  LWWElementSet,
  CRDTDocumentImpl,
  LCROperations,
  compareOperations,
  generateOperationID,
} from './collaboration/crdt.js';
export type { Operation, OperationType, CRDTDocument } from './collaboration/crdt.js';

// 增强型 WebSocket 传输
export { EnhancedWSTransport } from './a2a/enhanced-websocket-transport.js';
export type { EnhancedWSOptions, ConnectionStatus } from './a2a/enhanced-websocket-transport.js';

// ===== 集群协调（对齐 Go internal/agent/cluster/） =====
export {
  MemKVStore, DistributedDiscovery, ClusterCoordinator,
  ClusterManager, ConsistentHash, clusterConfigWithDefaults,
} from './cluster/index.js';
export type {
  KVStore, KVEvent, KVEventType,
  DistributedDiscoveryConfig,
  ClusterMessage, ClusterReply, RemoteNode, CoordinationConfig,
  NodeInfo, NodeRole, NodeStatus, AgentDiscovery as ClusterAgentDiscovery,
  ClusterConfig,
} from './cluster/index.js';
// 注: AgentInfo / Discovery 已从 agent 模块导出，避免重复

// ===== 混沌工程（对齐 Go internal/chaos/） =====
export {
  ChaosEngine,
  // 基础故障
  NetworkDelayFault, PartitionFault, ConnectionRefusedFault,
  CPUStressFault, MemoryStressFault, ProcessKillFault,
  CompositeFault, NoopFault,
  // 向后兼容
  LatencyFault, ErrorFault, ResourceFault,
  // LLM 故障
  LLMHTTPStatusFault, LLMTimeoutFault, LLMIntermittentFault, LLMSlowResponseFault,
  llmHTTP503Fault, llmHTTP429Fault, llmHTTP500Fault,
  llmFailoverScenario, llmChaosScenario,
  LLMErrorFault, LLMRateLimitFault,
  // 稳态
  SLOSteadyState, AvailabilitySteadyState, LatencySteadyState,
  CompositeSteadyState, CustomSteadyState,
  // 报告
  summarize, formatReport, formatSummaryTable,
} from './chaos/index.js';
export type {
  ExperimentStatus, EngineOptions,
  Fault, FaultInjector, CleanupFunc, FaultResult,
  SteadyState, SteadyStateResult,
  LLMFaultScenario, ExperimentSummary,
} from './chaos/index.js';
// 注: Experiment / ExperimentResult 类型从 './chaos/index.js' 直接导入，
// 避免与 A/B 测试模块的同名类型冲突

// ===== Agent Marketplace（对齐 Go internal/agent/marketplace/） =====
export { TemplateRegistry as MarketplaceTemplateRegistry, Deployer, validateTemplate } from './marketplace/index.js';
export type { AgentTemplate as MarketplaceTemplate, ValidationResult as TemplateValidationResult, DeployConfig, DeployResult, TemplateCategory, MemoryStrategy } from './marketplace/index.js';

// ===== Governance 多租户治理（对齐 Go internal/governance/） =====
export { TenantManager, TokenBucket, QuotaManager, ResourceManager, PolicyEnforcer, PolicyViolationError, InMemoryAuditLogger, defaultQuota } from './governance/index.js';
export type { Tenant, TenantPlan, TenantStatus, TenantQuota, Policy, PolicySpec, EnforcerSnapshot, AuditEvent as GovernanceAuditEvent, AuditLogger as GovernanceAuditLogger } from './governance/index.js';

// Tree-shakeable 标记
export { __treeShakeable, __bundlerMarker } from './internal/tree-shakeable.js';