/**
 * Zod Schema µ¥Ôª²âÊÔ
 */
import { describe, it, expect } from 'vitest';
import {
  MessageSchema, ToolCallSchema, ToolResultSchema, UsageSchema,
  CompletionResponseSchema, ToolCallResponseSchema, ChunkSchema,
  AgentMetricsSchema, ResponseSchema, StreamEventSchema,
  TokenEventSchema, ToolCallEventSchema, ToolResultEventSchema,
  TurnEndEventSchema, DoneEventSchema, ErrorEventSchema,
  ReActConfigSchema, CompletionRequestSchema, ProviderConfigSchema,
  ModelInfoSchema, CheckpointSchema, ResponseFormatTypeSchema,
  SchemaDefSchema, ResponseFormatSchema,
} from '../../src/schema/index.js';
import {
  validateLLMCompletion, validateToolCallResponse,
  validateMessage, validateStreamEvent,
} from '../../src/schema/index.js';
import type { Message, ToolCall, ToolResult } from '../../src/schema/messages.js';

describe('Schema', () => {
  describe('UsageSchema', () => {
    it('valid', () => {
      expect(UsageSchema.safeParse({ promptTokens: 10, completionTokens: 5, totalTokens: 15 }).success).toBe(true);
    });
    it('without totalTokens', () => {
      expect(UsageSchema.safeParse({ promptTokens: 10, completionTokens: 5 }).success).toBe(true);
    });
    it('reject negative', () => {
      expect(UsageSchema.safeParse({ promptTokens: -1, completionTokens: 5 }).success).toBe(false);
    });
    it('reject non-integer', () => {
      expect(UsageSchema.safeParse({ promptTokens: 10.5, completionTokens: 5 }).success).toBe(false);
    });
  });

  describe('ToolCallSchema', () => {
    it('valid', () => {
      expect(ToolCallSchema.safeParse({ id: 'tc-1', name: 'echo', arguments: '{}' }).success).toBe(true);
    });
    it('reject empty id', () => {
      expect(ToolCallSchema.safeParse({ id: '', name: 'echo', arguments: '{}' }).success).toBe(false);
    });
    it('reject empty name', () => {
      expect(ToolCallSchema.safeParse({ id: 'tc-1', name: '', arguments: '{}' }).success).toBe(false);
    });
    it('reject missing args', () => {
      expect(ToolCallSchema.safeParse({ id: 'tc-1', name: 'echo' }).success).toBe(false);
    });
  });

  describe('ToolResultSchema', () => {
    it('valid', () => {
      expect(ToolResultSchema.safeParse({ toolCallId: 'tc-1', content: 'output', isError: false }).success).toBe(true);
    });
    it('accept error', () => {
      expect(ToolResultSchema.safeParse({ toolCallId: 'tc-1', content: 'err', isError: true }).success).toBe(true);
    });
    it('reject empty toolCallId', () => {
      expect(ToolResultSchema.safeParse({ toolCallId: '', content: 'o', isError: false }).success).toBe(false);
    });
    it('reject missing isError', () => {
      expect(ToolResultSchema.safeParse({ toolCallId: 'tc-1', content: 'o' }).success).toBe(false);
    });
  });

  describe('MessageSchema', () => {
    it('valid user', () => {
      expect(MessageSchema.safeParse({ role: 'user', content: 'hello' }).success).toBe(true);
    });
    it('valid all roles', () => {
      for (const role of ['system', 'user', 'assistant', 'tool'] as const) {
        expect(MessageSchema.safeParse({ role, content: 't' }).success).toBe(true);
      }
    });
    it('reject invalid role', () => {
      expect(MessageSchema.safeParse({ role: 'x', content: 't' }).success).toBe(false);
    });
    it('valid assistant with toolCalls', () => {
      const r = MessageSchema.safeParse({ role: 'assistant', content: 't', toolCalls: [{ id: 't', name: 'e', arguments: '{}' }] });
      expect(r.success).toBe(true);
    });
    it('valid tool with toolCallId', () => {
      expect(MessageSchema.safeParse({ role: 'tool', content: 'r', toolCallId: 't', name: 'e' }).success).toBe(true);
    });
    it('reject missing role', () => {
      expect(MessageSchema.safeParse({ content: 't' }).success).toBe(false);
    });
  });

  describe('CompletionResponseSchema', () => {
    it('valid', () => {
      const r = CompletionResponseSchema.safeParse({ id: 'r1', content: 'Hi', role: 'assistant', usage: { promptTokens: 10, completionTokens: 5 } });
      expect(r.success).toBe(true);
    });
    it('reject missing id', () => {
      const r = CompletionResponseSchema.safeParse({ content: 'Hi', role: 'assistant', usage: { promptTokens: 10, completionTokens: 5 } });
      expect(r.success).toBe(false);
    });
    it('reject invalid role', () => {
      const r = CompletionResponseSchema.safeParse({ id: 'r1', content: 'Hi', role: 'x', usage: { promptTokens: 10, completionTokens: 5 } });
      expect(r.success).toBe(false);
    });
    it('accept usedModel', () => {
      const r = CompletionResponseSchema.safeParse({ id: 'r1', content: 'Hi', role: 'assistant', usage: { promptTokens: 10, completionTokens: 5 }, usedModel: 'gpt-4' });
      expect(r.success).toBe(true);
    });
  });

  describe('ToolCallResponseSchema', () => {
    it('valid', () => {
      const r = ToolCallResponseSchema.safeParse({ content: 'r', toolCalls: [{ id: 't', name: 'e', arguments: '{}' }], usage: { promptTokens: 10, completionTokens: 5 } });
      expect(r.success).toBe(true);
    });
    it('accept empty toolCalls', () => {
      const r = ToolCallResponseSchema.safeParse({ content: 'r', toolCalls: [], usage: { promptTokens: 10, completionTokens: 5 } });
      expect(r.success).toBe(true);
    });
    it('reject missing usage', () => {
      const r = ToolCallResponseSchema.safeParse({ content: 'r', toolCalls: [] });
      expect(r.success).toBe(false);
    });
  });

  describe('ChunkSchema', () => {
    it('valid', () => { expect(ChunkSchema.safeParse({ content: 't', done: false }).success).toBe(true); });
    it('reject missing done', () => { expect(ChunkSchema.safeParse({ content: 't' }).success).toBe(false); });
  });

  describe('AgentMetricsSchema', () => {
    it('valid', () => {
      const r = AgentMetricsSchema.safeParse({ totalTurns: 3, totalTools: 5, duration: 1000, llmLatency: 500, toolLatency: 200 });
      expect(r.success).toBe(true);
    });
    it('reject negative', () => {
      const r = AgentMetricsSchema.safeParse({ totalTurns: -1, totalTools: 5, duration: 1000, llmLatency: 500, toolLatency: 200 });
      expect(r.success).toBe(false);
    });
    it('reject missing fields', () => { expect(AgentMetricsSchema.safeParse({ totalTurns: 1 }).success).toBe(false); });
  });

  describe('ResponseSchema', () => {
    it('valid', () => {
      const r = ResponseSchema.safeParse({ content: 'a', metrics: { totalTurns: 1, totalTools: 0, duration: 100, llmLatency: 50, toolLatency: 0 } });
      expect(r.success).toBe(true);
    });
    it('reject missing metrics', () => { expect(ResponseSchema.safeParse({ content: 'a' }).success).toBe(false); });
  });

  describe('StreamEventSchema', () => {
    it('token', () => { expect(StreamEventSchema.safeParse({ type: 'token', content: 'hi' }).success).toBe(true); });
    it('tool_call', () => { expect(StreamEventSchema.safeParse({ type: 'tool_call', toolCall: { id: 't', name: 'e', arguments: '{}' }, turn: 0 }).success).toBe(true); });
    it('tool_result', () => { expect(StreamEventSchema.safeParse({ type: 'tool_result', result: { toolCallId: 't', content: 'r', isError: false }, turn: 0 }).success).toBe(true); });
    it('turn_end', () => { expect(StreamEventSchema.safeParse({ type: 'turn_end', turn: 2 }).success).toBe(true); });
    it('done', () => { expect(StreamEventSchema.safeParse({ type: 'done', response: { content: 'f', metrics: { totalTurns: 1, totalTools: 0, duration: 0, llmLatency: 0, toolLatency: 0 } } }).success).toBe(true); });
    it('error', () => { expect(StreamEventSchema.safeParse({ type: 'error', error: new Error('e') }).success).toBe(true); });
    it('reject unknown type', () => { expect(StreamEventSchema.safeParse({ type: 'x' }).success).toBe(false); });
    it('reject invalid token', () => { expect(StreamEventSchema.safeParse({ type: 'token' }).success).toBe(false); });
  });

  describe('Individual event schemas', () => {
    it('TokenEventSchema', () => { expect(TokenEventSchema.safeParse({ type: 'token', content: 'hi' }).success).toBe(true); });
    it('ToolCallEventSchema', () => { expect(ToolCallEventSchema.safeParse({ type: 'tool_call', toolCall: { id: 't', name: 'e', arguments: '{}' }, turn: 0 }).success).toBe(true); });
    it('ToolResultEventSchema', () => { expect(ToolResultEventSchema.safeParse({ type: 'tool_result', result: { toolCallId: 't', content: 'r', isError: false }, turn: 0 }).success).toBe(true); });
    it('TurnEndEventSchema', () => { expect(TurnEndEventSchema.safeParse({ type: 'turn_end', turn: 5 }).success).toBe(true); });
    it('DoneEventSchema', () => { expect(DoneEventSchema.safeParse({ type: 'done', response: { content: 'c', metrics: { totalTurns: 1, totalTools: 0, duration: 0, llmLatency: 0, toolLatency: 0 } } }).success).toBe(true); });
    it('ErrorEventSchema', () => { expect(ErrorEventSchema.safeParse({ type: 'error', error: new Error('e') }).success).toBe(true); });
  });

  describe('ReActConfigSchema', () => {
    it('valid minimal', () => { expect(ReActConfigSchema.safeParse({ name: 'test' }).success).toBe(true); });
    it('valid full', () => {
      const r = ReActConfigSchema.safeParse({ name: 't', maxTurns: 20, maxConsecutiveFailures: 5, systemPrompt: 'hi', sessionId: 's1', maxMessages: 100, parallelToolExecution: true, maxParallelTools: 3 });
      expect(r.success).toBe(true);
    });
    it('reject empty name', () => { expect(ReActConfigSchema.safeParse({ name: '' }).success).toBe(false); });
    it('reject maxTurns <= 0', () => { expect(ReActConfigSchema.safeParse({ name: 't', maxTurns: 0 }).success).toBe(false); });
    it('reject maxTurns > 1000', () => { expect(ReActConfigSchema.safeParse({ name: 't', maxTurns: 1001 }).success).toBe(false); });
  });

  describe('CompletionRequestSchema', () => {
    it('valid', () => {
      const r = CompletionRequestSchema.safeParse({ messages: [{ role: 'user', content: 'hi' }], model: 'gpt-4', temperature: 0.7, maxTokens: 1000 });
      expect(r.success).toBe(true);
    });
    it('reject empty messages', () => { expect(CompletionRequestSchema.safeParse({ messages: [] }).success).toBe(false); });
    it('reject missing messages', () => { expect(CompletionRequestSchema.safeParse({}).success).toBe(false); });
    it('reject temp > 2', () => { expect(CompletionRequestSchema.safeParse({ messages: [{ role: 'user', content: 'hi' }], temperature: 3 }).success).toBe(false); });
    it('accept responseFormat', () => { expect(CompletionRequestSchema.safeParse({ messages: [{ role: 'user', content: 'hi' }], responseFormat: { type: 'json_object' } }).success).toBe(true); });
  });

  describe('ProviderConfigSchema', () => {
    it('valid', () => { expect(ProviderConfigSchema.safeParse({ apiKey: 'sk', baseURL: 'https://api.example.com', model: 'gpt-4' }).success).toBe(true); });
    it('reject empty apiKey', () => { expect(ProviderConfigSchema.safeParse({ apiKey: '' }).success).toBe(false); });
    it('reject invalid URL', () => { expect(ProviderConfigSchema.safeParse({ apiKey: 'sk', baseURL: 'not-url' }).success).toBe(false); });
  });

  describe('ModelInfoSchema', () => {
    it('valid', () => { expect(ModelInfoSchema.safeParse({ name: 'gpt-4', provider: 'openai', maxContext: 128000, supportsTools: true, supportsStreaming: true }).success).toBe(true); });
    it('reject missing fields', () => { expect(ModelInfoSchema.safeParse({ name: 'gpt-4', provider: 'openai' }).success).toBe(false); });
  });

  describe('CheckpointSchema', () => {
    it('valid', () => {
      const r = CheckpointSchema.safeParse({ id: 'cp-1', sessionID: 's1', turn: 3, messages: [{ role: 'user', content: 'hi' }], metrics: { totalTurns: 3, totalTools: 2, duration: 500, llmLatency: 200, toolLatency: 100 }, createdAt: '2024-01-01' });
      expect(r.success).toBe(true);
    });
    it('reject missing createdAt', () => { expect(CheckpointSchema.safeParse({ id: 'cp-1', sessionID: 's1', turn: 1, messages: [], metrics: { totalTurns: 1, totalTools: 0, duration: 0, llmLatency: 0, toolLatency: 0 } }).success).toBe(false); });
  });

  describe('Format schemas', () => {
    it('valid format types', () => {
      expect(ResponseFormatTypeSchema.safeParse('text').success).toBe(true);
      expect(ResponseFormatTypeSchema.safeParse('json_object').success).toBe(true);
      expect(ResponseFormatTypeSchema.safeParse('json_schema').success).toBe(true);
    });
    it('reject invalid format type', () => { expect(ResponseFormatTypeSchema.safeParse('xml').success).toBe(false); });
    it('valid SchemaDef', () => { expect(SchemaDefSchema.safeParse({ name: 'S', description: 'd', schema: { type: 'object' }, strict: true }).success).toBe(true); });
    it('valid ResponseFormat', () => { expect(ResponseFormatSchema.safeParse({ type: 'text' }).success).toBe(true); });
    it('valid ResponseFormat with jsonSchema', () => { expect(ResponseFormatSchema.safeParse({ type: 'json_schema', jsonSchema: { name: 'S', schema: { type: 'object' } } }).success).toBe(true); });
  });

  describe('validateLLMCompletion', () => {
    it('ok=true for valid', () => {
      const result = validateLLMCompletion({ id: 'r1', content: 'Hello', role: 'assistant', usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 } });
      expect(result.ok).toBe(true);
      if (result.ok) expect(result.data.content).toBe('Hello');
    });
    it('ok=false for invalid', () => { expect(validateLLMCompletion({ invalid: true }).ok).toBe(false); });
    it('ok=false for null', () => { expect(validateLLMCompletion(null).ok).toBe(false); });
    it('ok=false for missing usage', () => { expect(validateLLMCompletion({ id: 'r1', content: 'Hi', role: 'assistant' }).ok).toBe(false); });
  });

  describe('validateToolCallResponse', () => {
    it('ok=true for valid', () => {
      const result = validateToolCallResponse({ content: 'r', toolCalls: [{ id: 't', name: 'e', arguments: '{}' }], usage: { promptTokens: 10, completionTokens: 5 } });
      expect(result.ok).toBe(true);
      if (result.ok) expect(result.data.toolCalls.length).toBe(1);
    });
    it('ok=false for missing toolCalls', () => { expect(validateToolCallResponse({ content: 'r', usage: {} }).ok).toBe(false); });
  });

  describe('validateMessage', () => {
    it('ok=true for valid', () => {
      const result = validateMessage({ role: 'user', content: 'hello' });
      expect(result.ok).toBe(true);
      if (result.ok) expect(result.data.role).toBe('user');
    });
    it('ok=false for invalid role', () => { expect(validateMessage({ role: 'bot', content: 'hello' }).ok).toBe(false); });
  });

  describe('validateStreamEvent', () => {
    it('ok=true for valid', () => {
      const result = validateStreamEvent({ type: 'token', content: 'hello' });
      expect(result.ok).toBe(true);
      if (result.ok) expect(result.data.type).toBe('token');
    });
    it('ok=false for invalid', () => { expect(validateStreamEvent({ type: 'invalid' }).ok).toBe(false); });
    it('ok=false for null', () => { expect(validateStreamEvent(null).ok).toBe(false); });
  });

  describe('z.infer type integration', () => {
    it('Message type', () => {
      const msg: Message = { role: 'assistant', content: 'test' };
      expect(MessageSchema.safeParse(msg).success).toBe(true);
    });
    it('ToolCall type', () => {
      const tc: ToolCall = { id: 't', name: 'n', arguments: '{}' };
      expect(ToolCallSchema.safeParse(tc).success).toBe(true);
    });
    it('ToolResult type', () => {
      const tr: ToolResult = { toolCallId: 't', content: 'c', isError: false };
      expect(ToolResultSchema.safeParse(tr).success).toBe(true);
    });
  });
});
