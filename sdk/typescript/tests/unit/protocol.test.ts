/**
 * Protocol 序列化/反序列化单元测试。
 *
 * 验证 Go ↔ TS 跨语言兼容性。
 */
import { describe, it, expect } from 'vitest';
import {
  generateId,
  now,
  serializeAgentMessage,
  deserializeAgentMessage,
  serializeToolResult,
  deserializeToolResult,
  serializeEventMessage,
  deserializeEventMessage,
  serializeMemoryEntry,
  deserializeMemoryEntry,
  compactJSON,
} from '../../src/protocol/serializer.js';
import {
  AgentMessage,
  ToolCall,
  ToolResult,
  EventMessage,
  MemoryEntry,
} from '../../src/protocol/types.js';
import { ProtocolParseError, ProtocolValidationError } from '../../src/protocol/types.js';

describe('generateId', () => {
  it('should generate a non-empty string', () => {
    const id = generateId();
    expect(typeof id).toBe('string');
    expect(id.length).toBeGreaterThan(0);
  });

  it('should generate unique IDs', () => {
    const ids = new Set(Array.from({ length: 100 }, () => generateId()));
    expect(ids.size).toBe(100);
  });
});

describe('deserializeAgentMessage', () => {
  it('should deserialize a valid JSON string', () => {
    const json = JSON.stringify({
      id: 'msg-001',
      role: 'user',
      content: 'Hello, world!',
      metadata: { lang: 'zh' },
      timestamp: 1700000000000,
    });
    const msg = deserializeAgentMessage(json);
    expect(msg.id).toBe('msg-001');
    expect(msg.role).toBe('user');
    expect(msg.content).toBe('Hello, world!');
    expect(msg.metadata).toEqual({ lang: 'zh' });
    expect(msg.timestamp).toBe(1700000000000);
  });

  it('should deserialize with tool_calls', () => {
    const json = JSON.stringify({
      id: 'msg-002',
      role: 'assistant',
      content: 'I can help you.',
      tool_calls: [
        { id: 'tc-1', name: 'search', arguments: '{"query":"go"}' },
      ],
      metadata: { model: 'gpt-4' },
      timestamp: 1700000001000,
    });
    const msg = deserializeAgentMessage(json);
    expect(msg.tool_calls).toHaveLength(1);
    expect(msg.tool_calls![0].name).toBe('search');
    expect(msg.tool_calls![0].arguments).toBe('{"query":"go"}');
  });

  it('should throw ProtocolParseError on invalid JSON', () => {
    expect(() => deserializeAgentMessage('not json')).toThrow(ProtocolParseError);
  });

  it('should throw ProtocolValidationError on missing id', () => {
    const json = JSON.stringify({ role: 'user', content: 'hi', timestamp: 1 });
    expect(() => deserializeAgentMessage(json)).toThrow(ProtocolValidationError);
  });

  it('should throw ProtocolValidationError on invalid role', () => {
    const json = JSON.stringify({ id: '1', role: 'ghost', content: 'hi', timestamp: 1 });
    expect(() => deserializeAgentMessage(json)).toThrow(ProtocolValidationError);
  });

  it('should throw ProtocolValidationError when content empty and no tool_calls', () => {
    const json = JSON.stringify({ id: '1', role: 'assistant', content: '', timestamp: 1 });
    expect(() => deserializeAgentMessage(json)).toThrow(ProtocolValidationError);
  });
});

describe('round-trip compatibility', () => {
  it('should round-trip AgentMessage', () => {
    const original: AgentMessage = {
      id: 'roundtrip-1',
      role: 'assistant',
      content: 'The answer is 42.',
      tool_calls: [
        { id: 'tc-abc', name: 'calculator', arguments: '{"expr":"6*7"}' },
      ],
      metadata: { source: 'test', version: '1' },
      timestamp: 1700000000000,
    };
    const json = serializeAgentMessage(original);
    const restored = deserializeAgentMessage(json);
    expect(restored).toEqual(original);
  });

  it('should handle AgentMessage without optional fields', () => {
    const original: AgentMessage = {
      id: 'roundtrip-2',
      role: 'user',
      content: 'hello',
      timestamp: now(),
    };
    const json = serializeAgentMessage(original);
    const restored = deserializeAgentMessage(json);
    expect(restored.id).toBe(original.id);
    expect(restored.role).toBe(original.role);
    expect(restored.content).toBe(original.content);
    expect(restored.tool_calls).toBeUndefined();
    expect(restored.metadata).toBeUndefined();
  });

  it('should round-trip ToolResult', () => {
    const original: ToolResult = {
      tool_call_id: 'tc-1',
      result: '{"status":"ok","data":"hello"}',
      is_error: false,
    };
    const json = serializeToolResult(original);
    const restored = deserializeToolResult(json);
    expect(restored).toEqual(original);
  });

  it('should round-trip EventMessage', () => {
    const original: EventMessage = {
      id: 'evt-1',
      type: 'tool_call',
      source: 'agent-executor',
      payload: '{"tool":"search"}',
      timestamp: now(),
      metadata: { session: 'sess-1' },
    };
    const json = serializeEventMessage(original);
    const restored = deserializeEventMessage(json);
    expect(restored).toEqual(original);
  });

  it('should round-trip MemoryEntry', () => {
    const original: MemoryEntry = {
      id: 'mem-1',
      topic: 'greetings',
      content: 'User prefers formal greeting',
      importance: 0.8,
      created_at: now(),
    };
    const json = serializeMemoryEntry(original);
    const restored = deserializeMemoryEntry(json);
    expect(restored).toEqual(original);
  });
});

describe('cross-language compat', () => {
  it('should parse JSON serialized by Go convention', () => {
    // 模拟 Go 端标准 JSON 输出（字段名、大小写与 Go json tag 一致）
    const goStyleJSON = '{"id":"compat-1","role":"assistant","content":"Cross-language test",' +
      '"tool_calls":[{"id":"tc-compat","name":"echo","arguments":"{\\"msg\\":\\"hi\\"}"}],' +
      '"metadata":{"phase":"B"},"timestamp":1700000000000}';
    const msg = deserializeAgentMessage(goStyleJSON);
    expect(msg.id).toBe('compat-1');
    expect(msg.role).toBe('assistant');
    expect(msg.tool_calls).toHaveLength(1);
    expect(msg.tool_calls![0].name).toBe('echo');
    expect(msg.metadata!.phase).toBe('B');
  });

  it('should produce JSON compatible with Go convention', () => {
    const msg: AgentMessage = {
      id: 'compat-2',
      role: 'assistant',
      content: 'Test',
      tool_calls: [{ id: 'tc-1', name: 'echo', arguments: '{}' }],
      timestamp: 1700000000000,
    };
    const json = serializeAgentMessage(msg);
    // 验证字段名为 snake_case
    expect(json).toContain('"tool_calls"');
    expect(json).not.toContain('"toolCalls"');
  });
});

describe('compactJSON', () => {
  it('should remove whitespace outside strings', () => {
    const input = '{\n  "id": "msg-1",\n  "content": "HelloWorld"\n}';
    const result = compactJSON(input);
    expect(result).not.toMatch(/\s/);
    expect(result).toContain('"content":"HelloWorld"');
  });

  it('should preserve whitespace inside strings', () => {
    const input = '{"msg": "Hello World"}';
    const result = compactJSON(input);
    expect(result).toBe('{"msg":"Hello World"}');
  });
});
