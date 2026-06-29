/** 消息结构，与 Go 端 Message 对齐。
 *
 * 字段说明：
 * - role: 角色（system/user/assistant/tool）
 * - content: 消息内容
 * - toolCalls: 工具调用列表（assistant 消息可选）
 * - toolCallId: 工具调用 ID（tool 消息可选）
 * - name: 发送者名称（可选）
 */
export interface Message {
  role: 'system' | 'user' | 'assistant' | 'tool';
  content: string;
  toolCalls?: ToolCall[];
  toolCallId?: string;
  name?: string;
}

/** LLM 工具调用，与 Go 端 ToolCall 对齐 */
export interface ToolCall {
  id: string;
  name: string;
  arguments: string;
}

/** 工具执行结果，与 Go 端 ToolResult 对齐 */
export interface ToolResult {
  toolCallId: string;
  content: string;
  isError: boolean;
}

/** 工具定义，格式兼容 OpenAI Function Calling */
export interface ToolDefinition {
  type: 'function';
  function: {
    name: string;
    description: string;
    parameters: Record<string, unknown>;
  };
}

/** Token 用量统计 */
export interface Usage {
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
}

/** 补全请求，与 Go 端 CompletionRequest 对齐 */
export interface CompletionRequest {
  messages: Message[];
  model?: string;
  temperature?: number;
  maxTokens?: number;
  /** 响应格式控制（结构化输出），与 Go 端 ResponseFormat 对齐 */
  responseFormat?: ResponseFormat;
}

/** 响应格式类型，与 Go 端 ResponseFormatType 对齐 */
export type ResponseFormatType = 'text' | 'json_object' | 'json_schema';

/** 响应格式控制，与 Go 端 ResponseFormat 对齐 */
export interface ResponseFormat {
  type: ResponseFormatType;
  jsonSchema?: SchemaDef;
}

/** JSON Schema 定义，与 Go 端 SchemaDef 对齐 */
export interface SchemaDef {
  name: string;
  description?: string;
  schema: Record<string, unknown>;
  strict?: boolean;
}

/** 补全响应，与 Go 端 CompletionResponse 对齐 */
export interface CompletionResponse {
  id: string;
  content: string;
  role: 'system' | 'user' | 'assistant' | 'tool';
  usage: Usage;
  toolCalls?: ToolCall[];
}

/** 工具调用请求 */
export interface ToolCallRequest {
  messages: Message[];
  tools: ToolDefinition[];
  model?: string;
  temperature?: number;
  maxTokens?: number;
}

/** 工具调用响应 */
export interface ToolCallResponse {
  content: string;
  toolCalls: ToolCall[];
  usage: Usage;
}

/** 流式数据块 */
export interface Chunk {
  content: string;
  done: boolean;
  usage?: Usage;
}

/** 模型信息 */
export interface ModelInfo {
  name: string;
  provider: string;
  maxContext: number;
  supportsTools: boolean;
  supportsStreaming: boolean;
}

/** Provider 配置 */
export interface ProviderConfig {
  apiKey: string;
  baseURL?: string;
  model?: string;
  temperature?: number;
  maxTokens?: number;
}

/** Agent 运行指标，与 Go 端 AgentMetrics 对齐 */
export interface AgentMetrics {
  totalTurns: number;
  totalTools: number;
  duration: number;
  llmLatency: number;
  toolLatency: number;
}

/** Agent 最终响应，包含内容和指标 */
export interface Response {
  content: string;
  metrics: AgentMetrics;
}

/** Agent 状态，与 Go 端 AgentStatus 对齐 */
export type AgentStatus = 'idle' | 'running' | 'paused' | 'completed' | 'error';

/** 可执行工具，与 Go 端 Tool 接口对齐 */
export interface Tool {
  name: string;
  description: string;
  parameters: Record<string, unknown>;
  execute(args: Record<string, unknown>): Promise<string>;
}

/** 记忆片段，与 Go 端 MemoryEpisode 对齐 */
export interface MemoryEpisode {
  id: string;
  sessionId: string;
  role: 'system' | 'user' | 'assistant' | 'tool';
  content: string;
  summary?: string;
  topics?: string;
  importance?: number;
  metadata?: Record<string, string>;
  createdAt: string;
}

/** 记忆存储统计 */
export interface MemoryStats {
  totalEpisodes: number;
  totalSessions: number;
  oldestEpisode?: string;
  newestEpisode?: string;
  avgEpisodesPerSession: number;
}

/** 搜索选项 */
export interface SearchOptions {
  sessionId?: string;
  limit?: number;
  offset?: number;
  roleFilter?: string;
}

/** 列表选项 */
export interface ListOptions {
  sessionId?: string;
  limit?: number;
  offset?: number;
  orderBy?: string;
  ascending?: boolean;
}

/** 向量搜索结果 */
export interface VectorSearchResult {
  id: string;
  score: number;
  metadata?: Record<string, string>;
}

/** SDK 版本 */
export const VERSION = '1.0.0';

/** 错误码常量，与 Go 端 CodeError 错误码体系对齐。
 *
 * 分类：
 * - AGENT_xxx: Agent 运行时错误
 * - TOOL_xxx: 工具相关错误
 * - LLM_xxx: LLM 调用错误
 * - POOL_xxx: 池相关错误
 * - CTX_xxx: 上下文错误
 * - MEM_xxx: 记忆存储错误
 * - SEC_xxx: 安全相关错误
 * - EVT_xxx: 事件总线错误
 * - PST_xxx: 持久化错误
 * - CON_xxx: 并发控制错误
 */
export const ErrorCodes = {
  AGENT_STOPPED: 'AGENT_001',
  AGENT_RUNNING: 'AGENT_002',
  MAX_TURNS_EXCEEDED: 'AGENT_003',
  NO_TOOLKIT: 'AGENT_004',
  TOOL_NOT_FOUND: 'TOOL_001',
  TOOL_EXECUTION: 'TOOL_002',
  INVALID_CONFIG: 'TOOL_003',
  LLM_CALL_FAILED: 'LLM_001',
  NOT_SUPPORTED: 'LLM_002',
  CIRCUIT_OPEN: 'LLM_003',
  API_KEY_REQUIRED: 'LLM_004',
  EMPTY_RESPONSE: 'LLM_005',
  RESPONSE_PARSE_FAILED: 'LLM_006',
  RETRIES_EXHAUSTED: 'LLM_007',
  FALLBACK_FAILED: 'LLM_008',
  POOL_FULL: 'POOL_001',
  TASK_NOT_FOUND: 'POOL_002',
  TIMEOUT: 'POOL_003',
  CONTEXT_CANCELED: 'CTX_001',
  EPISODE_NOT_FOUND: 'MEM_001',
  INVALID_IMPORTANCE: 'MEM_002',
  EMPTY_EPISODE_ID: 'MEM_003',
  EMPTY_SESSION_ID: 'MEM_004',
  EMPTY_ROLE: 'MEM_005',
  EMPTY_CONTENT: 'MEM_006',
  DIMENSION_MISMATCH: 'MEM_007',
  VECTOR_NOT_FOUND: 'MEM_008',
  COMMAND_BLOCKED: 'SEC_001',
  COMMAND_NOT_ALLOWED: 'SEC_002',
  ACCESS_DENIED: 'SEC_003',
  PATH_TRAVERSAL: 'SEC_004',
  BUS_CLOSED: 'EVT_001',
  CHECKPOINT_NOT_FOUND: 'PST_001',
  GLOBAL_WRITE_CONFLICT: 'CON_001',
  SCOPE_OVERLAP: 'CON_002',
} as const;

/** 错误码联合类型 */
export type ErrorCode = typeof ErrorCodes[keyof typeof ErrorCodes];
