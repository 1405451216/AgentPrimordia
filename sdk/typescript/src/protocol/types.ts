/**
 * AgentPrimordia 统一协议层（TypeScript 端）。
 *
 * 与 Go 端 internal/protocol/types.go 对齐：
 * - 字段名使用 camelCase（对应 Go 的 json tag）
 * - 所有 timestamp 为 Unix 毫秒（number）
 * - omitempty 字段用 optional (?) 表示
 */

/** 协议版本 */
export const PROTOCOL_VERSION = '1.0.0';

/** 角色常量，与 Go 端对齐 */
export const RoleSystem = 'system';
export const RoleUser = 'user';
export const RoleAssistant = 'assistant';
export const RoleTool = 'tool';

export type Role =
  | typeof RoleSystem
  | typeof RoleUser
  | typeof RoleAssistant
  | typeof RoleTool;

// ===== Agent 消息 =====

/** 统一 Agent 消息格式，对应 Go 端 AgentMessage */
export interface AgentMessage {
  id: string;
  role: Role;
  content: string;
  tool_calls?: ToolCall[];
  metadata?: Record<string, string>;
  timestamp: number;
}

/** 工具调用请求 */
export interface ToolCall {
  id: string;
  name: string;
  arguments: string; // JSON 字符串
}

/** 工具执行结果 */
export interface ToolResult {
  tool_call_id: string;
  result: string; // JSON 字符串
  is_error: boolean;
}

// ===== 记忆消息 =====

/** 记忆条目 */
export interface MemoryEntry {
  id: string;
  topic: string;
  content: string;
  importance: number;
  metadata?: Record<string, string>;
  created_at: number;
}

/** 记忆查询请求 */
export interface MemoryQuery {
  topic?: string;
  keyword?: string;
  top_k: number;
  metadata?: Record<string, string>;
}

// ===== 事件消息 =====

/** 系统内部事件 */
export interface EventMessage {
  id: string;
  type: string; // tool_call / tool_result / error / lifecycle
  source: string;
  payload: string; // JSON 字符串
  timestamp: number;
  metadata?: Record<string, string>;
}

// ===== 解析错误 =====

/** 反序列化失败时的详细信息 */
export class ProtocolParseError extends Error {
  field: string;
  constructor(field: string, message: string) {
    super(`protocol: parse error on field "${field}": ${message}`);
    this.name = 'ProtocolParseError';
    this.field = field;
  }
}

/** 校验错误 */
export class ProtocolValidationError extends Error {
  field: string;
  constructor(field: string, message: string) {
    super(`protocol: validation error on field "${field}": ${message}`);
    this.name = 'ProtocolValidationError';
    this.field = field;
  }
}
