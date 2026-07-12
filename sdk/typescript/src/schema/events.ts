/**
 * Agent 运行时事件 Zod schema。
 * Schema 为类型唯一真源，TS 类型通过 z.infer<T> 自动推导。
 */
import { z } from 'zod';
import { ToolCallSchema, ToolResultSchema } from './messages.js';
import { ResponseSchema } from './responses.js';

/** token 事件 schema — LLM 流式输出 token */
export const TokenEventSchema = z.object({
  type: z.literal('token'),
  content: z.string(),
});

/** tool_call 事件 schema — LLM 请求调用工具 */
export const ToolCallEventSchema = z.object({
  type: z.literal('tool_call'),
  toolCall: ToolCallSchema,
  turn: z.number().int(),
});

/** tool_result 事件 schema — 工具执行完成 */
export const ToolResultEventSchema = z.object({
  type: z.literal('tool_result'),
  result: ToolResultSchema,
  turn: z.number().int(),
});

/** turn_end 事件 schema — 一轮 ReAct 循环完成 */
export const TurnEndEventSchema = z.object({
  type: z.literal('turn_end'),
  turn: z.number().int(),
});

/** done 事件 schema — Agent 运行完成 */
export const DoneEventSchema = z.object({
  type: z.literal('done'),
  response: ResponseSchema,
});

/** error 事件 schema — 运行过程发生错误 */
export const ErrorEventSchema = z.object({
  type: z.literal('error'),
  error: z.instanceof(Error),
});

/** 流式事件 union schema — 所有可能事件类型的并集 */
export const StreamEventSchema = z.union([
  TokenEventSchema,
  ToolCallEventSchema,
  ToolResultEventSchema,
  TurnEndEventSchema,
  DoneEventSchema,
  ErrorEventSchema,
]);

// ===== 类型推导 =====

export type TokenEvent = z.infer<typeof TokenEventSchema>;
export type ToolCallEvent = z.infer<typeof ToolCallEventSchema>;
export type ToolResultEvent = z.infer<typeof ToolResultEventSchema>;
export type TurnEndEvent = z.infer<typeof TurnEndEventSchema>;
export type DoneEvent = z.infer<typeof DoneEventSchema>;
export type ErrorEvent = z.infer<typeof ErrorEventSchema>;
export type StreamEvent = z.infer<typeof StreamEventSchema>;
