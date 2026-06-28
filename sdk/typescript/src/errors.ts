// Stability: Stable — 错误码与 sentinel 错误在 v1.0 前冻结，向后兼容。

/**
 * CodeError 是带错误码的错误类型，用于结构化错误返回。
 * 对齐 Go 端 pkg/errors.go 的 CodeError 设计。
 */
export class CodeError extends Error {
  /** 结构化错误码，如 "AGENT_001", "TOOL_002" */
  readonly code: string;

  constructor(code: string, message: string) {
    super(message);
    this.code = code;
    this.name = 'CodeError';
  }
}

/** 创建带错误码的 CodeError 实例 */
export function withCode(code: string, message: string): CodeError {
  return new CodeError(code, message);
}

// ===== Agent 错误 =====
export const ErrAgentStopped     = new CodeError('AGENT_001', 'agent is stopped');
export const ErrAgentRunning     = new CodeError('AGENT_002', 'agent is already running');
export const ErrMaxTurnsExceeded = new CodeError('AGENT_003', 'max turns exceeded');
export const ErrNoToolkit        = new CodeError('AGENT_004', 'no toolkit configured');

// ===== Tool 错误 =====
export const ErrToolNotFound  = new CodeError('TOOL_001', 'tool not found');
export const ErrToolExecution = new CodeError('TOOL_002', 'tool execution failed');
export const ErrInvalidConfig = new CodeError('TOOL_003', 'invalid tool configuration');
export const ErrConfirmDenied = new CodeError('TOOL_004', 'tool confirmation denied');

// ===== LLM 错误 =====
export const ErrLLMCallFailed       = new CodeError('LLM_001', 'LLM call failed');
export const ErrNotSupported        = new CodeError('LLM_002', 'operation not supported');
export const ErrCircuitOpen         = new CodeError('LLM_003', 'circuit breaker is open');
export const ErrAPIKeyRequired      = new CodeError('LLM_004', 'API key is required');
export const ErrEmptyResponse       = new CodeError('LLM_005', 'LLM returned empty response');
export const ErrResponseParseFailed = new CodeError('LLM_006', 'LLM response parse failed');
export const ErrRetriesExhausted    = new CodeError('LLM_007', 'all retries exhausted');
export const ErrFallbackFailed      = new CodeError('LLM_008', 'all fallback providers failed');

// ===== Pool 错误 =====
export const ErrPoolFull     = new CodeError('POOL_001', 'pool is at max capacity');
export const ErrTaskNotFound = new CodeError('POOL_002', 'task not found');
export const ErrTimeout      = new CodeError('POOL_003', 'operation timed out');

// ===== Context 错误 =====
export const ErrContextCanceled = new CodeError('CTX_001', 'context canceled');

// ===== Memory 错误 =====
export const ErrEpisodeNotFound   = new CodeError('MEM_001', 'episode not found');
export const ErrInvalidImportance = new CodeError('MEM_002', 'invalid importance value');
export const ErrEmptyEpisodeID    = new CodeError('MEM_003', 'episode ID is empty');
export const ErrEmptySessionID    = new CodeError('MEM_004', 'session ID is empty');
export const ErrEmptyRole         = new CodeError('MEM_005', 'role is empty');
export const ErrEmptyContent      = new CodeError('MEM_006', 'content is empty');
export const ErrDimensionMismatch = new CodeError('MEM_007', 'vector dimension mismatch');
export const ErrVectorNotFound    = new CodeError('MEM_008', 'vector record not found');

// ===== Security 错误 =====
export const ErrCommandBlocked    = new CodeError('SEC_001', 'command is blocked');
export const ErrCommandNotAllowed = new CodeError('SEC_002', 'command is not in allowed list');
export const ErrAccessDenied      = new CodeError('SEC_003', 'access denied');
export const ErrPathTraversal     = new CodeError('SEC_004', 'path traversal detected');

// ===== Event 错误 =====
export const ErrBusClosed = new CodeError('EVT_001', 'event bus is closed');

// ===== Persistence 错误 =====
export const ErrCheckpointNotFound = new CodeError('PST_001', 'checkpoint not found');

// ===== Concurrency 错误 =====
export const ErrGlobalWriteConflict = new CodeError('CON_001', 'global write conflict');
export const ErrScopeOverlap        = new CodeError('CON_002', 'scope overlap detected');

/**
 * 从错误中提取结构化错误码。
 * 支持 CodeError 实例和 sentinel 错误。
 */
export function getErrorCode(err: unknown): string {
  if (err instanceof CodeError) {
    return err.code;
  }
  return 'UNKNOWN';
}