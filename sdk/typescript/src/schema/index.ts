/**
 * Zod-Native 类型安全体系 — 统一导出入口。
 *
 * 单一数据源原则：
 * - 所有核心类型的 Zod schema 定义在此模块下（messages / responses / config / events）
 * - TS 类型通过 `z.infer<>` 从 schema 自动推导，不允许手写 interface 作为类型主定义
 * - runtime 使用 `safeParse` / `parse` 对 LLM 返回结果做运行时验证
 */
import { CompletionResponseSchema, ToolCallResponseSchema } from './responses.js';
import { MessageSchema } from './messages.js';
import { StreamEventSchema } from './events.js';
import type { CompletionResponse, ToolCallResponse } from './responses.js';
import type { Message } from './messages.js';
import type { StreamEvent } from './events.js';

// ===== Schema 统一导出 =====
export * from './messages.js';
export * from './responses.js';
export * from './config.js';
export * from './events.js';

// ===== 类型统一导出 =====
export type {
  Message,
  ToolCall,
  ToolResult,
  Usage,
} from './messages.js';

export type {
  CompletionResponse,
  ToolCallResponse,
  Chunk,
  AgentMetrics,
  AgentResponse,
} from './responses.js';

export type {
  StreamEvent,
  TokenEvent,
  ToolCallEvent,
  ToolResultEvent,
  TurnEndEvent,
  DoneEvent,
  ErrorEvent,
} from './events.js';

// ===== 验证辅助函数 =====

/**
 * 安全解析 LLM 完整响应。
 * 在 callLLM 返回后调用，确保输出结构完整。
 * 解析失败时返回详细 error 信息而非抛出异常。
 */
export function validateLLMCompletion(
  raw: unknown,
):
  | { ok: true; data: CompletionResponse }
  | { ok: false; errors: string[] }
{
  const result = CompletionResponseSchema.safeParse(raw);
  if (result.success) return { ok: true, data: result.data };
  return { ok: false, errors: result.error.issues.map((i) => `${i.path.join('.')}: ${i.message}`) };
}

/**
 * 安全解析工具调用响应（callTools 接口返回值）。
 */
export function validateToolCallResponse(
  raw: unknown,
):
  | { ok: true; data: ToolCallResponse }
  | { ok: false; errors: string[] }
{
  const result = ToolCallResponseSchema.safeParse(raw);
  if (result.success) return { ok: true, data: result.data };
  return { ok: false, errors: result.error.issues.map((i) => `${i.path.join('.')}: ${i.message}`) };
}

/**
 * 安全解析消息（用于外部输入校验）。
 */
export function validateMessage(
  raw: unknown,
):
  | { ok: true; data: Message }
  | { ok: false; errors: string[] }
{
  const result = MessageSchema.safeParse(raw);
  if (result.success) return { ok: true, data: result.data };
  return { ok: false, errors: result.error.issues.map((i) => `${i.path.join('.')}: ${i.message}`) };
}

/**
 * 安全解析流式事件（用于 SSE/WebSocket 输入校验）。
 */
export function validateStreamEvent(
  raw: unknown,
):
  | { ok: true; data: StreamEvent }
  | { ok: false; errors: string[] }
{
  const result = StreamEventSchema.safeParse(raw);
  if (result.success) return { ok: true, data: result.data };
  return { ok: false, errors: result.error.issues.map((i) => `${i.path.join('.')}: ${i.message}`) };
}

// ===== z 统一导出 =====
export { z } from 'zod';
