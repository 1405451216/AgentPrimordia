/**
 * 英文语言包 (默认)
 * Default locale strings for the AgentPrimordia SDK.
 */

export const en = {
  // 通用
  'common.yes': 'Yes',
  'common.no': 'No',
  'common.ok': 'OK',
  'common.cancel': 'Cancel',
  'common.loading': 'Loading...',
  'common.error': 'Error',
  'common.success': 'Success',
  'common.warning': 'Warning',

  // 参数校验
  'validation.required': 'Field "{field}" is required',
  'validation.type': 'Field "{field}" must be of type {type}',
  'validation.range': 'Value must be between {min} and {max}',
  'validation.format': 'Invalid format for field "{field}"',

  // 超时
  'timeout.operation': 'Operation timed out after {seconds} seconds',
  'timeout.connect': 'Connection timed out after {seconds} seconds',
  'timeout.request': 'Request timed out after {seconds} seconds',

  // 权限
  'permission.denied': 'Permission denied',
  'permission.forbidden': 'Access forbidden',
  'permission.scope': 'Insufficient scope: {scope} required',
  'permission.auth': 'Authentication required',

  // Agent 相关
  'agent.running': 'Agent is already running',
  'agent.stopped': 'Agent has been stopped',
  'agent.maxTurns': 'Maximum turns ({count}) exceeded',
  'agent.noToolkit': 'No toolkit configured for agent',
  'agent.initFailed': 'Failed to initialize agent: {reason}',

  // Tool 相关
  'tool.notFound': 'Tool "{name}" not found',
  'tool.execution': 'Tool execution failed: {reason}',
  'tool.invalidConfig': 'Invalid tool configuration',
  'tool.confirmDenied': 'Tool confirmation denied by user',

  // LLM 相关
  'llm.callFailed': 'LLM call failed: {reason}',
  'llm.apiKey': 'API key is required for provider "{provider}"',
  'llm.emptyResponse': 'LLM returned empty response',
  'llm.parseFailed': 'Failed to parse LLM response',
  'llm.retriesExhausted': 'All retry attempts exhausted',
  'llm.circuitOpen': 'Circuit breaker is open for provider "{provider}"',
  'llm.rateLimited': 'Rate limit exceeded, retry after {seconds}s',

  // Memory 相关
  'memory.episodeNotFound': 'Episode "{id}" not found',
  'memory.invalidImportance': 'Importance must be between 0 and 1',
  'memory.emptyId': 'ID cannot be empty',
  'memory.emptyContent': 'Content cannot be empty',

  // Security 相关
  'security.commandBlocked': 'Command is blocked by security policy',
  'security.accessDenied': 'Access denied',
  'security.pathTraversal': 'Path traversal detected',

  // WebSocket 相关
  'ws.connectFailed': 'WebSocket connection failed: {reason}',
  'ws.maxReconnect': 'Maximum reconnection attempts ({count}) reached',
  'ws.timeout': 'WebSocket timeout after {seconds}s',

  // HTTP 相关
  'http.error': 'HTTP error {status}: {message}',
  'http.serverError': 'Server error: {status}',
} as const;

export type EnLocale = typeof en;
export type LocaleKey = keyof EnLocale;