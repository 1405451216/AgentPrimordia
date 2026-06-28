// tests/unit/batch.test.ts
// BatchRequestProcessor 单元测试

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { BatchRequestProcessor, defaultBatchConfig } from '../../src/llm/batch.js';
import type { Provider, CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, ModelInfo } from '../../src/types.js';

function makeMockProvider(latency = 5): Provider & { calls: CompletionRequest[] } {
  const calls: CompletionRequest[] = [];
  return {
    calls,
    async complete(req: CompletionRequest): Promise<CompletionResponse> {
      calls.push(req);
      await new Promise((r) => setTimeout(r, latency));
      return { content: `echo:${req.messages[0]?.content ?? ''}` };
    },
    async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
      return { toolCalls: [] };
    },
    info(): ModelInfo {
      return { name: 'mock', provider: 'mock' };
    },
  };
}

describe('BatchRequestProcessor', () => {
  it('exports a sensible default config', () => {
    const cfg = defaultBatchConfig();
    expect(cfg.maxBatchSize).toBeGreaterThan(0);
    expect(cfg.flushTimeout).toBeGreaterThan(0);
  });

  it('batches concurrent calls and forwards each request independently', async () => {
    const mock = makeMockProvider(2);
    const bp = new BatchRequestProcessor(mock, { maxBatchSize: 3, flushTimeoutMs: 50 });
    const reqs: CompletionRequest[] = [
      { messages: [{ role: 'user', content: 'a' }] },
      { messages: [{ role: 'user', content: 'b' }] },
      { messages: [{ role: 'user', content: 'c' }] },
    ];
    const results = await Promise.all(reqs.map((r) => bp.complete(r)));
    expect(results.map((r) => r.content)).toEqual(['echo:a', 'echo:b', 'echo:c']);
    expect(mock.calls).toHaveLength(3);
    bp.close();
  });

  it('flushes early when maxBatchSize is reached', async () => {
    const mock = makeMockProvider(0);
    const bp = new BatchRequestProcessor(mock, { maxBatchSize: 2, flushTimeoutMs: 10_000 });
    const p1 = bp.complete({ messages: [{ role: 'user', content: '1' }] });
    const p2 = bp.complete({ messages: [{ role: 'user', content: '2' }] });
    const [r1, r2] = await Promise.all([p1, p2]);
    expect(r1.content).toBe('echo:1');
    expect(r2.content).toBe('echo:2');
    bp.close();
  });

  it('flushes when the timer fires even if not full', async () => {
    const mock = makeMockProvider(0);
    const bp = new BatchRequestProcessor(mock, { maxBatchSize: 100, flushTimeoutMs: 20 });
    const r = await bp.complete({ messages: [{ role: 'user', content: 'late' }] });
    expect(r.content).toBe('echo:late');
    bp.close();
  });

  it('rejects requests after close()', async () => {
    const mock = makeMockProvider();
    const bp = new BatchRequestProcessor(mock);
    bp.close();
    await expect(bp.complete({ messages: [{ role: 'user', content: 'x' }] })).rejects.toThrow(/closed/);
  });

  it('passes callTools through to the underlying provider', async () => {
    const mock = makeMockProvider();
    const bp = new BatchRequestProcessor(mock);
    const out = await bp.callTools({ messages: [], tools: [] });
    expect(out).toEqual({ toolCalls: [] });
    bp.close();
  });

  it('exposes info() from the underlying provider', () => {
    const mock = makeMockProvider();
    const bp = new BatchRequestProcessor(mock);
    expect(bp.info()).toEqual({ name: 'mock', provider: 'mock' });
    bp.close();
  });
});