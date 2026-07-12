/**
 * AgentMessage / ToolCall / ToolResult 的 Zod schema 定义。
 * Schema 为类型唯一真源，所有 TS 类型通过 z.infer<T> 自动推导。
 */
import { z } from 'zod';

/** Token schema — 接受裸字符串或 {token} 对象（如 API Key 配置） */
export const TokenSchema = z.union([
  z.string(),
  z.object({ token: z.string() }),
]);

/**
 * ToolCall schema — 与 Go 端 ToolCall 对齐。
 * LLM 输出的每一条工具调用必须符合此结构。
 */
export const ToolCallSchema = z.object({
  id: z.string().min(1, 'ToolCall id 不可为空'),
  name: z.string().min(1, 'ToolCall name 不可为空'),
  arguments: z.string(),
});

/**
 * ToolResult schema — 工具执行结果，与 Go 端 ToolResult 对齐。
 */
export const ToolResultSchema = z.object({
  toolCallId: z.string().min(1, 'toolCallId 不可为空'),
  content: z.string(),
  isError: z.boolean(),
});

/**
 * Usage schema — Token 用量统计。
 * totalTokens 为可选字段（部分 Provider 可能不返回聚合值）。
 */
export const UsageSchema = z.object({
  promptTokens: z.number().int().nonnegative(),
  completionTokens: z.number().int().nonnegative(),
  totalTokens: z.number().int().nonnegative().optional(),
});

/**
 * Message schema — Agent 消息，与 Go 端 Message 对齐。
 * role 限定为 system / user / assistant / tool。
 */
export const MessageSchema = z.object({
  role: z.enum(['system', 'user', 'assistant', 'tool']),
  content: z.string(),
  toolCalls: z.array(ToolCallSchema).optional(),
  toolCallId: z.string().optional(),
  name: z.string().optional(),
});

// ===== 类型推导（schema 为唯一真源） =====

/** Agent 消息类型 — 由 MessageSchema 自动推导 */
export type Message = z.infer<typeof MessageSchema>;

/** LLM 工具调用类型 — 由 ToolCallSchema 自动推导 */
export type ToolCall = z.infer<typeof ToolCallSchema>;

/** 工具执行结果类型 — 由 ToolResultSchema 自动推导 */
export type ToolResult = z.infer<typeof ToolResultSchema>;

/** Token 用量类型 — 由 UsageSchema 自动推导 */
export type Usage = z.infer<typeof UsageSchema>;
