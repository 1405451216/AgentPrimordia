/**
 * LLM 响应格式的 Zod schema。
 * Schema 为类型唯一真源，TS 类型通过 z.infer<T> 自动推导。
 */
import { z } from 'zod';
import { UsageSchema } from './messages.js';

/** LLM 补全响应 schema — 与 Go 端 CompletionResponse 对齐 */
export const CompletionResponseSchema = z.object({
  id: z.string(),
  content: z.string(),
  role: z.enum(['system', 'user', 'assistant', 'tool']),
  usage: UsageSchema,
  toolCalls: z.array(
    z.object({
      id: z.string().min(1),
      name: z.string().min(1),
      arguments: z.string(),
    }),
  ).optional(),
  /** 实际使用的模型名（可选，用于成本追踪和模型路由） */
  usedModel: z.string().optional(),
});

/** 工具调用响应 schema — 与 Go 端 ToolCallResponse 对齐 */
export const ToolCallResponseSchema = z.object({
  content: z.string(),
  toolCalls: z.array(
    z.object({
      id: z.string().min(1),
      name: z.string().min(1),
      arguments: z.string(),
    }),
  ),
  usage: UsageSchema,
});

/** 流式数据块 schema — 与 Go 端 Chunk 对齐 */
export const ChunkSchema = z.object({
  content: z.string(),
  done: z.boolean(),
  usage: UsageSchema.optional(),
});

/** AgentMetrics schema — 与 Go 端 AgentMetrics 对齐 */
export const AgentMetricsSchema = z.object({
  totalTurns: z.number().int().nonnegative(),
  totalTools: z.number().int().nonnegative(),
  duration: z.number().nonnegative(),
  llmLatency: z.number().nonnegative(),
  toolLatency: z.number().nonnegative(),
});

/** Agent 最终 Response schema — 与 Go 端 Response 对齐 */
export const ResponseSchema = z.object({
  content: z.string(),
  metrics: AgentMetricsSchema,
  usage: UsageSchema.optional(),
  model: z.string().optional(),
});

// ===== 类型推导 =====

/** LLM 补全响应类型 */
export type CompletionResponse = z.infer<typeof CompletionResponseSchema>;

/** 工具调用响应类型 */
export type ToolCallResponse = z.infer<typeof ToolCallResponseSchema>;

/** 流式数据块类型 */
export type Chunk = z.infer<typeof ChunkSchema>;

/** Agent 运行指标类型 */
export type AgentMetrics = z.infer<typeof AgentMetricsSchema>;

/** Agent 最终响应类型 */
export type AgentResponse = z.infer<typeof ResponseSchema>;
