/**
 * 序列化/反序列化工具 + 验证。
 *
 * 与 Go 端 internal/protocol/types.go 配套，确保跨语言兼容性。
 */
import type {
  AgentMessage,
  ToolCall,
  ToolResult,
  EventMessage,
  MemoryEntry,
} from './types.js';
import {
  RoleSystem,
  RoleUser,
  RoleAssistant,
  RoleTool,
} from './types.js';
import { ProtocolParseError, ProtocolValidationError } from './types.js';

/** 生成唯一 ID（浏览器/Node 通用） */
export function generateId(): string {
  const cryptoObj = globalThis.crypto;
  if (cryptoObj?.randomUUID) {
    return cryptoObj.randomUUID();
  }
  // 降级方案：16 字节随机 hex
  const bytes = new Uint8Array(16);
  if (cryptoObj?.getRandomValues) {
    cryptoObj.getRandomValues(bytes);
  } else {
    for (let i = 0; i < 16; i++) bytes[i] = Math.floor(Math.random() * 256);
  }
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

/** 当前 Unix 毫秒时间戳 */
export function now(): number {
  return Date.now();
}

// ===== AgentMessage =====

/** AgentMessage → JSON 字符串 */
export function serializeAgentMessage(msg: AgentMessage): string {
  return JSON.stringify(msg);
}

/** JSON 字符串 → AgentMessage */
export function deserializeAgentMessage(json: string): AgentMessage {
  let parsed: unknown;
  try {
    parsed = JSON.parse(json);
  } catch (e) {
    throw new ProtocolParseError('AgentMessage', `invalid JSON: ${(e as Error).message}`);
  }
  if (typeof parsed !== 'object' || parsed === null) {
    throw new ProtocolParseError('AgentMessage', 'expected an object');
  }
  return validateAgentMessage(parsed as Record<string, unknown>);
}

/** 校验并转换未知对象为 AgentMessage */
export function validateAgentMessage(obj: Record<string, unknown>): AgentMessage {
  if (typeof obj.id !== 'string' || obj.id === '') {
    throw new ProtocolValidationError('id', 'cannot be empty');
  }
  const role = obj.role;
  if (role !== RoleSystem && role !== RoleUser && role !== RoleAssistant && role !== RoleTool) {
    throw new ProtocolValidationError('role', 'must be one of system/user/assistant/tool');
  }
  if (typeof obj.content !== 'string') {
    throw new ProtocolValidationError('content', 'must be a string');
  }
  if (typeof obj.timestamp !== 'number') {
    throw new ProtocolValidationError('timestamp', 'must be a number (Unix ms)');
  }

  let toolCalls: ToolCall[] | undefined;
  if (obj.tool_calls !== undefined) {
    if (!Array.isArray(obj.tool_calls)) {
      throw new ProtocolValidationError('tool_calls', 'must be an array');
    }
    toolCalls = obj.tool_calls.map((tc, i) => validateToolCall(tc, i));
  }

  let metadata: Record<string, string> | undefined;
  if (obj.metadata !== undefined) {
    if (typeof obj.metadata !== 'object' || obj.metadata === null) {
      throw new ProtocolValidationError('metadata', 'must be an object');
    }
    metadata = {};
    for (const [k, v] of Object.entries(obj.metadata as Record<string, unknown>)) {
      if (typeof v !== 'string') {
        throw new ProtocolValidationError(`metadata.${k}`, 'must be a string');
      }
      metadata[k] = v;
    }
  }

  // 校验 content 非空或 tool_calls 存在
  if (obj.content === '' && (!toolCalls || toolCalls.length === 0)) {
    throw new ProtocolValidationError('content', 'cannot be empty when tool_calls is absent');
  }

  return {
    id: obj.id,
    role: role as AgentMessage['role'],
    content: obj.content,
    tool_calls: toolCalls,
    metadata: metadata,
    timestamp: obj.timestamp,
  };
}

// ===== ToolCall =====

function validateToolCall(tc: unknown, index: number): ToolCall {
  if (typeof tc !== 'object' || tc === null) {
    throw new ProtocolValidationError(`tool_calls[${index}]`, 'must be an object');
  }
  const obj = tc as Record<string, unknown>;
  if (typeof obj.id !== 'string' || obj.id === '') {
    throw new ProtocolValidationError(`tool_calls[${index}].id`, 'cannot be empty');
  }
  if (typeof obj.name !== 'string' || obj.name === '') {
    throw new ProtocolValidationError(`tool_calls[${index}].name`, 'cannot be empty');
  }
  if (typeof obj.arguments !== 'string') {
    throw new ProtocolValidationError(`tool_calls[${index}].arguments`, 'must be a string');
  }
  return { id: obj.id, name: obj.name, arguments: obj.arguments };
}

/** 序列化 ToolResult */
export function serializeToolResult(tr: ToolResult): string {
  return JSON.stringify(tr);
}

/** 反序列化 ToolResult */
export function deserializeToolResult(json: string): ToolResult {
  let parsed: unknown;
  try {
    parsed = JSON.parse(json);
  } catch (e) {
    throw new ProtocolParseError('ToolResult', `invalid JSON: ${(e as Error).message}`);
  }
  if (typeof parsed !== 'object' || parsed === null) {
    throw new ProtocolParseError('ToolResult', 'expected an object');
  }
  const obj = parsed as Record<string, unknown>;
  if (typeof obj.tool_call_id !== 'string' || obj.tool_call_id === '') {
    throw new ProtocolValidationError('tool_call_id', 'cannot be empty');
  }
  if (typeof obj.result !== 'string') {
    throw new ProtocolValidationError('result', 'must be a string');
  }
  if (typeof obj.is_error !== 'boolean') {
    throw new ProtocolValidationError('is_error', 'must be a boolean');
  }
  return { tool_call_id: obj.tool_call_id, result: obj.result, is_error: obj.is_error };
}

// ===== EventMessage =====

/** 序列化 EventMessage */
export function serializeEventMessage(evt: EventMessage): string {
  return JSON.stringify(evt);
}

/** 反序列化 EventMessage */
export function deserializeEventMessage(json: string): EventMessage {
  let parsed: unknown;
  try {
    parsed = JSON.parse(json);
  } catch (e) {
    throw new ProtocolParseError('EventMessage', `invalid JSON: ${(e as Error).message}`);
  }
  if (typeof parsed !== 'object' || parsed === null) {
    throw new ProtocolParseError('EventMessage', 'expected an object');
  }
  const obj = parsed as Record<string, unknown>;
  if (typeof obj.id !== 'string' || obj.id === '') {
    throw new ProtocolValidationError('id', 'cannot be empty');
  }
  if (typeof obj.type !== 'string' || obj.type === '') {
    throw new ProtocolValidationError('type', 'cannot be empty');
  }
  if (typeof obj.source !== 'string') {
    throw new ProtocolValidationError('source', 'must be a string');
  }
  if (typeof obj.payload !== 'string') {
    throw new ProtocolValidationError('payload', 'must be a string');
  }
  if (typeof obj.timestamp !== 'number') {
    throw new ProtocolValidationError('timestamp', 'must be a number');
  }
  let metadata: Record<string, string> | undefined;
  if (obj.metadata !== undefined) {
    if (typeof obj.metadata !== 'object' || obj.metadata === null) {
      throw new ProtocolValidationError('metadata', 'must be an object');
    }
    metadata = {};
    for (const [k, v] of Object.entries(obj.metadata as Record<string, unknown>)) {
      if (typeof v !== 'string') {
        throw new ProtocolValidationError(`metadata.${k}`, 'must be a string');
      }
      metadata[k] = v;
    }
  }
  return {
    id: obj.id,
    type: obj.type,
    source: obj.source,
    payload: obj.payload,
    timestamp: obj.timestamp,
    metadata: metadata,
  };
}

// ===== MemoryEntry =====

/** 序列化 MemoryEntry */
export function serializeMemoryEntry(entry: MemoryEntry): string {
  return JSON.stringify(entry);
}

/** 反序列化 MemoryEntry */
export function deserializeMemoryEntry(json: string): MemoryEntry {
  let parsed: unknown;
  try {
    parsed = JSON.parse(json);
  } catch (e) {
    throw new ProtocolParseError('MemoryEntry', `invalid JSON: ${(e as Error).message}`);
  }
  if (typeof parsed !== 'object' || parsed === null) {
    throw new ProtocolParseError('MemoryEntry', 'expected an object');
  }
  const obj = parsed as Record<string, unknown>;
  if (typeof obj.id !== 'string' || obj.id === '') {
    throw new ProtocolValidationError('id', 'cannot be empty');
  }
  if (typeof obj.topic !== 'string') {
    throw new ProtocolValidationError('topic', 'must be a string');
  }
  if (typeof obj.content !== 'string') {
    throw new ProtocolValidationError('content', 'must be a string');
  }
  if (typeof obj.importance !== 'number') {
    throw new ProtocolValidationError('importance', 'must be a number');
  }
  if (typeof obj.created_at !== 'number') {
    throw new ProtocolValidationError('created_at', 'must be a number');
  }
  return {
    id: obj.id,
    topic: obj.topic,
    content: obj.content,
    importance: obj.importance,
    created_at: obj.created_at,
  };
}

// ===== 跨语言兼容性工具 =====

/**
 * 压缩 JSON 字符串（去除不影响语义的空白字符）。
 * 用于跨语言对比测试中消除格式差异。
 */
export function compactJSON(s: string): string {
  let result = '';
  let inString = false;
  let escaped = false;
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (inString) {
      result += c;
      if (escaped) {
        escaped = false;
      } else if (c === '\\') {
        escaped = true;
      } else if (c === '"') {
        inString = false;
      }
      continue;
    }
    switch (c) {
      case '"':
        inString = true;
        result += c;
        break;
      case ' ':
      case '\t':
      case '\n':
      case '\r':
        // 跳过空白
        break;
      default:
        result += c;
    }
  }
  return result;
}
