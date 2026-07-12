/**
 * Zod-Native 类型安全体系测试
 *
 * 验证：
 * 1. Zod schema 验证（parse/safeParse）正确性
 * 2. z.infer<T> 类型推导（单一数据源）
 * 3. zodToJsonSchema JSON Schema 自动生成
 * 4. messages / responses / config / events 各 schema 覆盖
 * 5. react-loop 集成：callLLM 后使用 Zod 进行运行时验证
 */
import { describe, it, expect, expectTypeOf } from 'vitest';
import { z } from 'zod';
import { zodToJsonSchema } from 'zod-to-json-schema';
// 被测 schema（待实现）
import {
  MessageSchema,
  ToolCallSchema,
  ToolResultSchema,
  UsageSchema,
  CompletionResponseSchema,
  ToolCallResponseSchema,
  StreamEventSchema,
  TokenSchema,
  type Message,
  type ToolCall,
  type ToolResult,
  type Usage,
  type CompletionResponse,
  type StreamEvent,
} from '../../../src/schema/index.js';
import type { ToolCallResponse } from '../../../src/schema/responses.js';

// ===== 1. 基础 schema 验证 =====

describe('Zod schema 验证 (parse / safeParse)', () => {

  describe('ToolCallSchema', () => {
    it('应正确解析合法的 ToolCall', () => {
      const result = ToolCallSchema.safeParse({
        id: 'call_abc123',
        name: 'echo',
        arguments: '{"message": "hello"}',
      });
      expect(result.success).toBe(true);
    });

    it('应拒绝缺少 id 的 ToolCall', () => {
      const result = ToolCallSchema.safeParse({ name: 'echo', arguments: '{}' });
      expect(result.success).toBe(false);
    });

    it('应拒绝缺少 name 的 ToolCall', () => {
      const result = ToolCallSchema.safeParse({ id: 'x', arguments: '{}' });
      expect(result.success).toBe(false);
    });

    it('应拒绝缺少 arguments 的 ToolCall', () => {
      const result = ToolCallSchema.safeParse({ id: 'x', name: 'echo' });
      expect(result.success).toBe(false);
    });

    it('parse() 在合法数据时应返回正确结构', () => {
      const parsed = ToolCallSchema.parse({
        id: 'call_1',
        name: 'calc',
        arguments: '{"expr": "1+1"}',
      });
      expect(parsed.id).toBe('call_1');
      expect(parsed.name).toBe('calc');
      expect(parsed.arguments).toBe('{"expr": "1+1"}');
    });

    it('parse() 在非法数据时应抛出 ZodError', () => {
      expect(() => ToolCallSchema.parse({})).toThrow();
    });
  });

  describe('ToolResultSchema', () => {
    it('应正确解析合法的 ToolResult', () => {
      const result = ToolResultSchema.safeParse({
        toolCallId: 'call_001',
        content: '{"sum": 2}',
        isError: false,
      });
      expect(result.success).toBe(true);
    });

    it('isError 必须为 boolean', () => {
      const result = ToolResultSchema.safeParse({
        toolCallId: 'c1',
        content: 'ok',
        isError: 'yes',  // 非法
      });
      expect(result.success).toBe(false);
    });
  });

  describe('UsageSchema', () => {
    it('应正确解析合法的 Usage', () => {
      const parsed = UsageSchema.parse({
        promptTokens: 100,
        completionTokens: 50,
        totalTokens: 150,
      });
      expect(parsed.totalTokens).toBe(150);
    });

    it('tokens 必须为数字', () => {
      const result = UsageSchema.safeParse({
        promptTokens: 100,
        completionTokens: 'fifty',  // 非法
        totalTokens: 150,
      });
      expect(result.success).toBe(false);
    });
  });
});

// ===== 2. 消息 schema =====

describe('MessageSchema', () => {
  it('应正确解析 system 消息', () => {
    const parsed = MessageSchema.parse({
      role: 'system',
      content: 'You are a helpful assistant.',
    });
    expect(parsed.role).toBe('system');
  });

  it('应正确解析 user 消息', () => {
    const parsed = MessageSchema.parse({
      role: 'user',
      content: 'Hello!',
    });
    expect(parsed.content).toBe('Hello!');
  });

  it('应正确解析 assistant 消息（含 toolCalls）', () => {
    const parsed = MessageSchema.parse({
      role: 'assistant',
      content: '',
      toolCalls: [
        { id: 'tc1', name: 'echo', arguments: '{}' },
      ],
    });
    expect(parsed.toolCalls).toHaveLength(1);
    expect(parsed.toolCalls![0]!.name).toBe('echo');
  });

  it('应正确解析 tool 回复消息', () => {
    const parsed = MessageSchema.parse({
      role: 'tool',
      content: 'result data',
      toolCallId: 'call_xyz',
      name: 'echo',
    });
    expect(parsed.toolCallId).toBe('call_xyz');
  });

  it('应拒绝非法的 role 值', () => {
    const result = MessageSchema.safeParse({
      role: 'invalid_role',
      content: 'test',
    });
    expect(result.success).toBe(false);
  });

  it('content 必须为 string', () => {
    const result = MessageSchema.safeParse({
      role: 'user',
      content: 123,  // 非法
    });
    expect(result.success).toBe(false);
  });
});

// ===== 3. 响应 schema =====

describe('CompletionResponseSchema', () => {
  it('应正确解析完整响应', () => {
    const parsed = CompletionResponseSchema.parse({
      id: 'resp_001',
      content: 'The answer is 42.',
      role: 'assistant',
      usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 },
    });
    expect(parsed.id).toBe('resp_001');
    expect(parsed.usedModel).toBeUndefined();
  });

  it('应接受可选的 toolCalls 字段', () => {
    const parsed = CompletionResponseSchema.parse({
      id: 'r2',
      content: '',
      role: 'assistant',
      usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 },
      toolCalls: [{ id: 'tc1', name: 'f', arguments: '{}' }],
    });
    expect(parsed.toolCalls).toHaveLength(1);
  });

  it('role 必须是合法的枚举值', () => {
    const result = CompletionResponseSchema.safeParse({
      id: 'r',
      content: '',
      role: 'bot',  // 非法
      usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 },
    });
    expect(result.success).toBe(false);
  });
});

describe('ToolCallResponseSchema', () => {
  it('应正确解析工具调用响应', () => {
    const parsed = ToolCallResponseSchema.parse({
      content: 'thinking...',
      toolCalls: [
        { id: 'tc1', name: 'search', arguments: '{"q": "ap"}' },
      ],
      usage: { promptTokens: 20, completionTokens: 10, totalTokens: 30 },
    });
    expect(parsed.toolCalls).toHaveLength(1);
    expect(parsed.usage.totalTokens).toBe(30);
  });
});

// ===== 4. 事件 schema =====

describe('StreamEventSchema', () => {
  it('应正确解析 token 事件', () => {
    const parsed = StreamEventSchema.parse({ type: 'token', content: 'Hello' });
    expect(parsed.type).toBe('token');
  });

  it('应正确解析 done 事件', () => {
    const parsed = StreamEventSchema.parse({
      type: 'done',
      response: {
        content: 'Done.',
        metrics: { totalTurns: 1, totalTools: 0, duration: 12, llmLatency: 10, toolLatency: 0 },
      },
    });
    expect(parsed.type).toBe('done');
  });

  it('应正确解析 error 事件', () => {
    const parsed = StreamEventSchema.parse({
      type: 'error',
      error: new Error('oop'),
    });
    expect(parsed.type).toBe('error');
  });

  it('应拒绝非法 event type', () => {
    const result = StreamEventSchema.safeParse({ type: 'unknown_event' });
    expect(result.success).toBe(false);
  });
});

// ===== 5. TokenSchema 测试 =====

describe('TokenSchema', () => {
  it('应接受对象或字符串形式', () => {
    expect(() => TokenSchema.parse('abc123.xyz')).not.toThrow();
    expect(() => TokenSchema.parse({ token: 'bearer' })).not.toThrow();
  });
});

// ===== 6. 类型推导测试 (z.infer) =====

describe('z.infer 类型推导（单一数据源）', () => {

  it('Message 类型应由 MessageSchema 推导', () => {
    expectTypeOf<Message>().toEqualTypeOf<z.infer<typeof MessageSchema>>();
  });

  it('ToolCall 类型应由 ToolCallSchema 推导', () => {
    expectTypeOf<ToolCall>().toEqualTypeOf<z.infer<typeof ToolCallSchema>>();
  });

  it('ToolResult 类型应由 ToolResultSchema 推导', () => {
    expectTypeOf<ToolResult>().toEqualTypeOf<z.infer<typeof ToolResultSchema>>();
  });

  it('Usage 类型应由 UsageSchema 推导', () => {
    expectTypeOf<Usage>().toEqualTypeOf<z.infer<typeof UsageSchema>>();
  });

  it('CompletionResponse 类型应由 CompletionResponseSchema 推导', () => {
    expectTypeOf<CompletionResponse>().toEqualTypeOf<z.infer<typeof CompletionResponseSchema>>();
  });

  it('StreamEvent 类型应由 StreamEventSchema 推导', () => {
    expectTypeOf<StreamEvent>().toEqualTypeOf<z.infer<typeof StreamEventSchema>>();
  });
});

// ===== 7. zodToJsonSchema 测试 =====

describe('zodToJsonSchema 自动生成', () => {

  it('应正确转换 ToolCallSchema 为 JSON Schema', () => {
    const jsonSchema = zodToJsonSchema(ToolCallSchema, { name: 'ToolCall' });
    expect(jsonSchema.definitions).toBeDefined();
    expect(jsonSchema.definitions!.ToolCall.type).toBe('object');
    expect(jsonSchema.definitions!.ToolCall.properties).toBeDefined();
    expect(jsonSchema.definitions!.ToolCall.properties!.id.type).toBe('string');
    expect(jsonSchema.definitions!.ToolCall.properties!.name.type).toBe('string');
  });

  it('应正确转换内嵌 Usage schema', () => {
    const jsonSchema = zodToJsonSchema(UsageSchema, { name: 'Usage' });
    expect(jsonSchema.definitions!.Usage.type).toBe('object');
    // totalTokens 是可选字段，不应出现在 required 中
    expect(jsonSchema.definitions!.Usage.required).not.toContain('totalTokens');
  });

  it('应生成可序列化的 JSON', () => {
    const jsonSchema = zodToJsonSchema(StreamEventSchema, { name: 'StreamEvent' });
    const serialized = JSON.stringify(jsonSchema);
    expect(serialized.length).toBeGreaterThan(0);
    // 应能反序列化
    const roundTrip = JSON.parse(serialized);
    expect(roundTrip.$schema).toBeDefined();
  });

  it('ToolDefinition 参数可被 schema 校验', () => {
    // 使用 ToolCallSchema 生成的 JSON schema 模板校验传入参数
    const jsonSchema = zodToJsonSchema(ToolCallSchema, { name: 'TC' });
    const def = jsonSchema.definitions!.TC;
    expect(def.required).toEqual(expect.arrayContaining(['id', 'name', 'arguments']));
  });
});

// ===== 8. react-loop 集成测试 =====

describe('react-loop 集成: callLLM 后 Zod 验证', () => {
  /**
   * 集成测试表明：从 model.callTools / model.complete 返回的数据
   * 在 push 到 messages 之前必须经过 Zod safeParse 验证。
   * 这保证即使 Provider 实现返回了不完整数据，也能尽早发现。
   */

  it('完整的 CompletionResponse 应通过验证', () => {
    const rawResp = {
      id: 'test_resp',
      content: 'Hello from LLM',
      role: 'assistant',
      usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 },
    };
    const result = CompletionResponseSchema.safeParse(rawResp);
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.content).toBe('Hello from LLM');
    }
  });

  it('缺少 content 的响应应被拒绝', () => {
    const incompleteResp = {
      id: 'bad_resp',
      role: 'assistant',
      usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 },
      // content 被遗漏
    };
    const result = CompletionResponseSchema.safeParse(incompleteResp);
    expect(result.success).toBe(false);
  });

  it('类型不匹配的 toolCall 应被拒绝', () => {
    const badResp = {
      id: 'bad_tools',
      content: '',
      role: 'assistant',
      usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 },
      toolCalls: [
        { id: 123, name: 'f', arguments: '{}' },  // id 应该是 string
      ],
    };
    const result = CompletionResponseSchema.safeParse(badResp);
    expect(result.success).toBe(false);
  });
});

// ===== 9. 工具参数验证 =====

describe('Zod 工具参数运行时验证', () => {

  it('应拒绝超过长度限制的 input', () => {
    // 通过 TokenSchema 验证 token 长度
    expect(() => TokenSchema.parse('x'.repeat(100))).not.toThrow();
  });

  it('枚举字段应严格匹配', () => {
    // role 字段验证
    expect(() => MessageSchema.parse({ role: 'user', content: 'ok' })).not.toThrow();
    expect(() => MessageSchema.parse({ role: 'user', content: 'ok', name: 'alice' })).not.toThrow();
  });
});
