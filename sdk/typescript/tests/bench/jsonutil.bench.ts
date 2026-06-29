/**
 * JSON 序列化/反序列化性能基准测试
 * 与 Go 端 jsonutil/pool_test.go 的 Benchmark 对齐
 */
import { describe, bench } from 'vitest';
import { Marshal, Unmarshal, DecodeString, ObjectPool, getRecord, putRecord, getArray, putArray } from '../../src/jsonutil/pool.js';

// 测试数据（与 Go 端 benchPayload 对齐）
interface BenchPayload {
  model: string;
  messages: Array<Record<string, unknown>>;
  stream: boolean;
  top_p: number;
  user: string;
}

function makePayload(): BenchPayload {
  const msgs: Array<Record<string, unknown>> = [];
  for (let i = 0; i < 20; i++) {
    msgs.push({
      role: 'user',
      content: 'What is the capital of France?',
      meta: { turn: i, tokens: 100 + i },
    });
  }
  return {
    model: 'gpt-4o',
    messages: msgs,
    stream: false,
    top_p: 0.95,
    user: 'test_user',
  };
}

const payload = makePayload();
const serialized = JSON.stringify(payload);

describe('JSON Marshal', () => {
  bench('JSON.stringify (native)', () => {
    JSON.stringify(payload);
  });

  bench('Marshal (pooled)', () => {
    Marshal(payload);
  });
});

describe('JSON Unmarshal', () => {
  bench('JSON.parse (native)', () => {
    JSON.parse(serialized);
  });

  bench('Unmarshal (pooled)', () => {
    Unmarshal<BenchPayload>(serialized);
  });

  bench('DecodeString', () => {
    DecodeString<BenchPayload>(serialized);
  });
});

describe('ObjectPool', () => {
  bench('get/put Record', () => {
    const obj = getRecord();
    obj.key = 'value';
    putRecord(obj);
  });

  bench('get/put Array', () => {
    const arr = getArray();
    arr.push('item');
    putArray(arr);
  });

  bench('ObjectPool direct', () => {
    const pool = new ObjectPool(
      () => ({ data: '' }),
      (obj: { data: string }) => { obj.data = ''; },
      100,
    );
    const obj = pool.get();
    obj.data = 'test';
    pool.put(obj);
  });

  bench('ObjectPool vs new object', () => {
    // Simulate high-frequency object creation
    const pool = new ObjectPool(
      () => ({ items: [] as number[] }),
      (obj: { items: number[] }) => { obj.items.length = 0; },
      200,
    );
    for (let i = 0; i < 100; i++) {
      const obj = pool.get();
      obj.items.push(i);
      pool.put(obj);
    }
  });
});

describe('SSE Simulation', () => {
  // Simulate SSE message parsing hot path
  const sseMessages: string[] = [];
  for (let i = 0; i < 100; i++) {
    sseMessages.push(JSON.stringify({
      id: `msg-${i}`,
      type: 'message',
      content: `Hello world ${i}`,
      timestamp: Date.now(),
    }));
  }

  bench('Parse 100 SSE messages', () => {
    for (const msg of sseMessages) {
      DecodeString(msg);
    }
  });
});
