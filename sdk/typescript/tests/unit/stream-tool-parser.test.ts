// stream-tool-parser.test.ts — T1-3 流式 tool_call 解析器测试
import { describe, it, expect } from 'vitest';
import { StreamToolCallParser } from '../../src/agent/stream-tool-parser.js';

describe('StreamToolCallParser', () => {
  it('should not extract when no tool_call marker', () => {
    const parser = new StreamToolCallParser();
    expect(parser.push('Hello, this is normal text')).toHaveLength(0);
    expect(parser.push('No tool calls here either.')).toHaveLength(0);
  });

  it('should extract single tool_call from complete JSON', () => {
    const parser = new StreamToolCallParser();
    const chunk = '{"name": "search", "arguments": "{\\"q\\": \\"test\\"}", "id": "call_1"}';
    const result = parser.push(chunk);
    expect(result.length).toBe(1);
    expect(result[0]!.name).toBe('search');
    expect(result[0]!.arguments).toBe('{"q": "test"}');
    expect(result[0]!.id).toBe('call_1');
  });

  it('should extract tool_call from incremental chunks', () => {
    const parser = new StreamToolCallParser();
    const full = '{"name": "calculator", "arguments": "{\\"expr\\": \\"2+2\\"}", "id": "call_2"}';
    // 模拟流式：分 5 个 chunk
    const chunks = [full.slice(0, 20), full.slice(20, 40), full.slice(40, 60), full.slice(60, 80), full.slice(80)];
    let totalFound: number = 0;
    for (const c of chunks) {
      const r = parser.push(c);
      totalFound += r.length;
    }
    expect(totalFound).toBe(1);
  });

  it('should extract multiple tool_calls from array', () => {
    const parser = new StreamToolCallParser();
    const chunk = JSON.stringify({
      tool_calls: [
        { name: 'search', arguments: '{"q":"a"}', id: 'tc1' },
        { name: 'calc', arguments: '{"x":1}', id: 'tc2' },
      ],
    });
    const result = parser.push(chunk);
    expect(result.length).toBe(2);
    expect(result[0]!.name).toBe('search');
    expect(result[1]!.name).toBe('calc');
  });

  it('should extract OpenAI function_call format', () => {
    const parser = new StreamToolCallParser();
    const chunk = JSON.stringify({
      id: 'call_xxx',
      type: 'function',
      function: { name: 'weather', arguments: '{"city":"SF"}' },
    });
    const result = parser.push(chunk);
    expect(result.length).toBe(1);
    expect(result[0]!.name).toBe('weather');
    expect(result[0]!.arguments).toBe('{"city":"SF"}');
  });

  it('should not extract incomplete JSON (unbalanced braces)', () => {
    const parser = new StreamToolCallParser();
    expect(parser.push('{"name": "x", "arg')).toHaveLength(0);
    expect(parser.isAccumulating()).toBe(true);
  });

  it('should complete extraction after more chunks arrive', () => {
    const parser = new StreamToolCallParser();
    expect(parser.push('{"name": "x", "arg')).toHaveLength(0);
    const r = parser.push('uments": "{}"}');
    expect(r.length).toBe(1);
    expect(parser.isAccumulating()).toBe(false);
  });

  it('should handle tool_call markers in mixed content', () => {
    const parser = new StreamToolCallParser();
    // LLM 先输出一些解释文本，然后输出 tool_call
    const result1 = parser.push('I need to use a tool to help. ');
    expect(result1).toHaveLength(0);
    const result2 = parser.push('{"name": "lookup", "arguments": "{}", "id": "t1"}');
    expect(result2.length).toBe(1);
  });

  it('should handle nested tool_calls object', () => {
    const parser = new StreamToolCallParser();
    const chunk = JSON.stringify({
      tool_calls: [{ name: 'a', arguments: '{}', id: 'a1' }],
      extra_field: 'ignored',
    });
    const result = parser.push(chunk);
    expect(result.length).toBe(1);
    expect(result[0]!.name).toBe('a');
  });

  it('should discard incomplete buffer on end()', () => {
    const parser = new StreamToolCallParser();
    parser.push('{"name": "x", "arg'); // 不完整
    expect(parser.isAccumulating()).toBe(true);
    const r = parser.end();
    expect(r).toHaveLength(0);
    expect(parser.isAccumulating()).toBe(false);
  });

  it('should handle reset()', () => {
    const parser = new StreamToolCallParser();
    parser.push('{"name": "x", "arg');
    expect(parser.isAccumulating()).toBe(true);
    parser.reset();
    expect(parser.isAccumulating()).toBe(false);
    // 重置后应能重新开始
    const r = parser.push('{"name": "y", "arguments": "{}", "id": "z"}');
    expect(r.length).toBe(1);
    expect(r[0]!.name).toBe('y');
  });

  it('should generate id when missing', () => {
    const parser = new StreamToolCallParser();
    const r = parser.push('{"name": "noId", "arguments": "{}"}');
    expect(r.length).toBe(1);
    expect(r[0]!.id).toBeTruthy();
    expect(r[0]!.id.length).toBeGreaterThan(0);
  });

  it('should handle multiple tool_calls across multiple pushes', () => {
    const parser = new StreamToolCallParser();
    const r1 = parser.push('{"name": "a", "arguments": "{}", "id": "1"}');
    expect(r1.length).toBe(1);
    const r2 = parser.push('{"name": "b", "arguments": "{}", "id": "2"}');
    expect(r2.length).toBe(1);
    expect(r2[0]!.name).toBe('b');
  });

  it('should handle empty chunk', () => {
    const parser = new StreamToolCallParser();
    expect(parser.push('')).toHaveLength(0);
    expect(parser.push('')).toHaveLength(0);
  });

  it('should handle special characters in arguments', () => {
    const parser = new StreamToolCallParser();
    const chunk = '{"name": "search", "arguments": "{\\"q\\": \\"中文 + emoji 🎉\\"}", "id": "x"}';
    const r = parser.push(chunk);
    expect(r.length).toBe(1);
    expect(r[0]!.arguments).toContain('中文');
    expect(r[0]!.arguments).toContain('🎉');
  });
});