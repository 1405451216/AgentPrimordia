export interface Message {
  role: 'system' | 'user' | 'assistant' | 'tool';
  content: string;
  toolCalls?: ToolCall[];
  toolCallId?: string;
  name?: string;
}

export interface ToolCall {
  id: string;
  name: string;
  arguments: string;
}

export interface ToolResult {
  toolCallId: string;
  content: string;
  isError: boolean;
}

export interface ToolDefinition {
  type: 'function';
  function: {
    name: string;
    description: string;
    parameters: Record<string, unknown>;
  };
}

export interface Usage {
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
}

export interface CompletionRequest {
  messages: Message[];
  model?: string;
  temperature?: number;
  maxTokens?: number;
}

export interface CompletionResponse {
  id: string;
  content: string;
  role: string;
  usage: Usage;
  toolCalls?: ToolCall[];
}

export interface ToolCallRequest {
  messages: Message[];
  tools: ToolDefinition[];
  model?: string;
  temperature?: number;
  maxTokens?: number;
}

export interface ToolCallResponse {
  content: string;
  toolCalls: ToolCall[];
  usage: Usage;
}

export interface Chunk {
  content: string;
  done: boolean;
  usage?: Usage;
}

export interface ModelInfo {
  name: string;
  provider: string;
  maxContext: number;
  supportsTools: boolean;
  supportsStreaming: boolean;
}

export interface ProviderConfig {
  apiKey: string;
  baseURL?: string;
  model?: string;
  temperature?: number;
  maxTokens?: number;
}

export interface AgentMetrics {
  totalTurns: number;
  totalTools: number;
  duration: number;
  llmLatency: number;
  toolLatency: number;
}

export interface Response {
  content: string;
  metrics: AgentMetrics;
}

export type AgentStatus = 'idle' | 'running' | 'paused' | 'completed' | 'error';

export interface Tool {
  name: string;
  description: string;
  parameters: Record<string, unknown>;
  execute(args: Record<string, unknown>): Promise<string>;
}

export interface MemoryEpisode {
  id: string;
  sessionId: string;
  role: string;
  content: string;
  summary?: string;
  topics?: string;
  importance?: number;
  metadata?: Record<string, string>;
  createdAt: string;
}

export interface MemoryStats {
  totalEpisodes: number;
  totalSessions: number;
  oldestEpisode?: string;
  newestEpisode?: string;
  avgEpisodesPerSession: number;
}

export interface SearchOptions {
  sessionId?: string;
  limit?: number;
  offset?: number;
  roleFilter?: string;
}

export interface ListOptions {
  sessionId?: string;
  limit?: number;
  offset?: number;
  orderBy?: string;
  ascending?: boolean;
}

export interface VectorSearchResult {
  id: string;
  score: number;
  metadata?: Record<string, string>;
}

export const VERSION = '0.1.0';

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

export type ErrorCode = typeof ErrorCodes[keyof typeof ErrorCodes];
