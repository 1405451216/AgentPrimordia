/**
 * Agent / Provider / Tool 运行时配置的 Zod schema。
 * Schema 为类型唯一真源，TS 类型通过 z.infer<T> 自动推导。
 */
import { z } from 'zod';
import { ToolCallSchema, ToolResultSchema, MessageSchema } from './messages.js';
import { AgentMetricsSchema } from './responses.js';

/** 响应格式类型 */
export const ResponseFormatTypeSchema = z.enum(['text', 'json_object', 'json_schema']);

/** JSON Schema 定义 schema（与 Go 端 SchemaDef 对齐） */
export const SchemaDefSchema = z.object({
  name: z.string(),
  description: z.string().optional(),
  schema: z.record(z.unknown()),
  strict: z.boolean().optional(),
});

/** 输出格式控制 schema（与 Go 端 ResponseFormat 对齐） */
export const ResponseFormatSchema = z.object({
  type: ResponseFormatTypeSchema,
  jsonSchema: SchemaDefSchema.optional(),
});

/** 补全请求 schema（与 Go 端 CompletionRequest 对齐） */
export const CompletionRequestSchema = z.object({
  messages: z.array(MessageSchema).min(1, 'messages 至少包含一条消息'),
  model: z.string().optional(),
  temperature: z.number().min(0).max(2).optional(),
  maxTokens: z.number().int().positive().optional(),
  responseFormat: ResponseFormatSchema.optional(),
});

/** Provider 配置 schema（API Key / baseURL / model） */
export const ProviderConfigSchema = z.object({
  apiKey: z.string().min(1, 'API Key 不可为空'),
  baseURL: z.string().url().optional(),
  model: z.string().optional(),
  temperature: z.number().min(0).max(2).optional(),
  maxTokens: z.number().int().positive().optional(),
});

/** ReActAgent 配置 schema — 用于 run 前的配置校验（可选字段） */
export const ReActConfigSchema = z.object({
  name: z.string().min(1, 'Agent name 不可为空'),
  maxTurns: z.number().int().positive().max(1000).optional(),
  maxConsecutiveFailures: z.number().int().positive().max(100).optional(),
  systemPrompt: z.string().max(100_000).optional(),
  sessionId: z.string().optional(),
  maxMessages: z.number().int().positive().optional(),
  costTracker: z.unknown().optional(),
  memoryStore: z.unknown().optional(),
  checkpointStore: z.unknown().optional(),
  otelBridge: z.unknown().optional(),
  parallelToolExecution: z.boolean().optional(),
  maxParallelTools: z.number().int().nonnegative().optional(),
}).strict();

/** Model 信息 schema */
export const ModelInfoSchema = z.object({
  name: z.string(),
  provider: z.string(),
  maxContext: z.number().int().positive(),
  supportsTools: z.boolean(),
  supportsStreaming: z.boolean(),
});

// ===== Checkpoint schema =====
export const CheckpointSchema = z.object({
  id: z.string(),
  sessionID: z.string(),
  turn: z.number().int(),
  messages: z.array(MessageSchema),
  metrics: AgentMetricsSchema,
  createdAt: z.string(),
});

// ===== 工具定义 JSON Schema 生成助手 =====

/**
 * 使用 zodToJsonSchema 将任意 Zod 参数 schema 转为 JSON Schema，
 * 用于在 ToolDefinition.function.parameters 中嵌入符合 OpenAI 规范的参数定义。
 */
export { zodToJsonSchema } from 'zod-to-json-schema';

// ===== 类型推导 =====

export type ResponseFormatType = z.infer<typeof ResponseFormatTypeSchema>;
export type SchemaDef = z.infer<typeof SchemaDefSchema>;
export type ResponseFormat = z.infer<typeof ResponseFormatSchema>;
export type CompletionRequestConfig = z.infer<typeof CompletionRequestSchema>;
export type ProviderConfig = z.infer<typeof ProviderConfigSchema>;
export type ReActConfigStrict = z.infer<typeof ReActConfigSchema>;
export type ModelInfo = z.infer<typeof ModelInfoSchema>;
export type Checkpoint = z.infer<typeof CheckpointSchema>;

// 重新导出以保持向后兼容
export type { ToolCall, ToolResult, Message } from './messages.js';
