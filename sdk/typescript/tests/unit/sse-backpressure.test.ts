import { describe, it, expect } from 'vitest';
import { SSEWriter, StreamCollector, CompressStrategy } from '../../src/agent/stream-extended.js';
import type { Chunk } from '../../src/types.js';

describe('SSEWriter — Backpressure', () => {
  it('accepts custom maxQueueDepth and writeTimeoutMs', () => {
    const writer = {
      write: (_chunk: string) => { /* no-op */ },
    };
    const sse = new SSEWriter(writer, { maxQueueDepth: 10, writeTimeoutMs: 1000 });
    expect(sse.getDroppedCount()).toBe(0);
  });

  it('increments dropped count when write fails', async () => {
    // Create a writer that throws on write
    const writer = {
      write: (_chunk: string) => {
        throw new Error('write failed');
      },
    };
    const sse = new SSEWriter(writer, { maxQueueDepth: 100, writeTimeoutMs: 100 });

    // writeToken should not throw, it should swallow the error
    await sse.writeToken('test');
    expect(sse.getDroppedCount()).toBe(1);
  });

  it('writes events successfully to a string writer', async () => {
    const received: string[] = [];
    const writer = {
      write: (chunk: string) => { received.push(chunk); },
    };
    const sse = new SSEWriter(writer);

    await sse.writeToken('hello');
    await sse.writeToken('world');
    await sse.writeDone();

    expect(received.length).toBe(3);
    expect(received[0]).toContain('event: token');
    expect(received[0]).toContain('data: hello');
    expect(received[2]).toContain('event: done');
    expect(received[2]).toContain('[DONE]');
  });

  it('supports heartbeat', async () => {
    const received: string[] = [];
    const writer = {
      write: (chunk: string) => { received.push(chunk); },
    };
    const sse = new SSEWriter(writer);
    await sse.writeHeartbeat();
    expect(received[0]).toContain('heartbeat');
  });

  it('sets retry and event ID', async () => {
    const received: string[] = [];
    const writer = {
      write: (chunk: string) => { received.push(chunk); },
    };
    const sse = new SSEWriter(writer);
    sse.setRetry(5000);
    sse.setEventID(100);

    await sse.writeToken('test');

    expect(received[0]).toContain('retry: 5000');
    expect(received[0]).toContain('id: 101'); // eventID incremented from 100
  });
});

describe('StreamCollector — Merge', () => {
  it('merges chunks from multiple streams', async () => {
    const collector = new StreamCollector();

    async function* stream1(): AsyncIterable<Chunk> {
      yield { content: 'a', done: false };
      yield { content: 'b', done: false };
      yield { content: '', done: true };
    }

    async function* stream2(): AsyncIterable<Chunk> {
      yield { content: '1', done: false };
      yield { content: '2', done: false };
      yield { content: '', done: true };
    }

    const chunks: string[] = [];
    for await (const chunk of collector.merge([stream1(), stream2()])) {
      if (chunk.content) chunks.push(chunk.content);
    }

    // Should contain all content from both streams
    expect(chunks.length).toBe(4);
    expect(chunks).toContain('a');
    expect(chunks).toContain('b');
    expect(chunks).toContain('1');
    expect(chunks).toContain('2');
  });

  it('handles empty streams', async () => {
    const collector = new StreamCollector();

    async function* emptyStream(): AsyncIterable<Chunk> {
      yield { content: '', done: true };
    }

    const chunks: Chunk[] = [];
    for await (const chunk of collector.merge([emptyStream()])) {
      chunks.push(chunk);
    }

    expect(chunks.length).toBe(1);
    expect(chunks[0]!.done).toBe(true);
  });

  it('handles single stream', async () => {
    const collector = new StreamCollector();

    async function* singleStream(): AsyncIterable<Chunk> {
      yield { content: 'hello', done: false };
      yield { content: '', done: true };
    }

    const chunks: string[] = [];
    for await (const chunk of collector.merge([singleStream()])) {
      if (chunk.content) chunks.push(chunk.content);
    }

    expect(chunks).toEqual(['hello']);
  });
});

describe('CompressStrategy — Timeout', () => {
  it('falls back to trim on LLM timeout', async () => {
    // Create a mock provider that never resolves
    const mockProvider = {
      complete: () => new Promise(() => { /* never resolves */ }),
    };

    const strategy = new CompressStrategy({
      maxTokens: 1000,
      summaryModel: mockProvider as never,
      keepSystemMessages: true,
      keepRecentN: 2,
      compressRatio: 0.3,
      compressTimeoutMs: 50, // 50ms timeout
    });

    const messages = [
      { role: 'user', content: 'message 1' },
      { role: 'assistant', content: 'response 1' },
      { role: 'user', content: 'message 2' },
      { role: 'assistant', content: 'response 2' },
      { role: 'user', content: 'message 3' },
      { role: 'assistant', content: 'response 3' },
    ];

    const start = Date.now();
    const result = await strategy.compressWithLLM(messages);
    const elapsed = Date.now() - start;

    // Should complete within ~500ms (timeout + overhead)
    expect(elapsed).toBeLessThan(2000);
    // Should return trimmed messages (fallback), not hang
    expect(result.length).toBeGreaterThan(0);
    expect(result.length).toBeLessThanOrEqual(messages.length);
  }, 10000); // Allow up to 10s for this test

  it('falls back to trim on LLM error', async () => {
    const mockProvider = {
      complete: () => Promise.reject(new Error('LLM unavailable')),
    };

    const strategy = new CompressStrategy({
      maxTokens: 1000,
      summaryModel: mockProvider as never,
      keepSystemMessages: true,
      keepRecentN: 2,
      compressRatio: 0.3,
      compressTimeoutMs: 5000,
    });

    const messages = [
      { role: 'user', content: 'message 1' },
      { role: 'assistant', content: 'response 1' },
      { role: 'user', content: 'message 2' },
      { role: 'assistant', content: 'response 2' },
      { role: 'user', content: 'message 3' },
    ];

    const result = await strategy.compressWithLLM(messages);
    // Should return trimmed messages (fallback), not throw
    expect(result.length).toBeGreaterThan(0);
  });

  it('returns original messages when keepRecentN >= message count', async () => {
    const mockProvider = {
      complete: () => Promise.resolve({ content: 'summary' }),
    };

    const strategy = new CompressStrategy({
      maxTokens: 1000,
      summaryModel: mockProvider as never,
      keepRecentN: 10,
      compressRatio: 0.3,
    });

    const messages = [
      { role: 'user', content: 'short' },
      { role: 'assistant', content: 'reply' },
    ];

    const result = await strategy.compressWithLLM(messages);
    expect(result).toEqual(messages);
  });
});
