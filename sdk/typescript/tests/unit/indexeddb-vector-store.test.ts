// indexeddb-vector-store.test.ts — T1-2 IndexedDB Vector Store 测试
// 由于 Node.js 测试环境没有 IndexedDB，使用 InMemoryVectorStore（同接口）验证逻辑。
// 真 IndexedDB 行为需在浏览器环境 e2e 验证。
import { describe, it, expect } from 'vitest';
import { InMemoryVectorStore, isIndexedDBAvailable, type IndexedDBVectorRecord } from '../../src/memory/indexeddb-vector-store.js';

describe('IndexedDBVectorStore (via InMemory mock)', () => {
  it('should expose isIndexedDBAvailable', () => {
    // Node 环境应为 false（这正是预期 —— 浏览器端才为 true）
    expect(isIndexedDBAvailable()).toBe(false);
  });

  it('should add and count vectors', async () => {
    const store = new InMemoryVectorStore();
    await store.add('a', [1, 0, 0]);
    await store.add('b', [0, 1, 0]);
    expect(await store.count()).toBe(2);
  });

  it('should overwrite on duplicate add', async () => {
    const store = new InMemoryVectorStore();
    await store.add('a', [1, 0, 0]);
    await store.add('a', [0, 1, 0]);
    expect(await store.count()).toBe(1);
    const results = await store.search([0, 1, 0], 1);
    expect(results[0]!.id).toBe('a');
  });

  it('should delete vectors', async () => {
    const store = new InMemoryVectorStore();
    await store.add('a', [1, 0, 0]);
    expect(await store.delete('a')).toBe(true);
    expect(await store.delete('missing')).toBe(false); // Map.delete 返回 boolean，InMemory 总是 true
    expect(await store.count()).toBe(0);
  });

  it('should clear all vectors', async () => {
    const store = new InMemoryVectorStore();
    await store.add('a', [1, 0, 0]);
    await store.add('b', [0, 1, 0]);
    await store.clear();
    expect(await store.count()).toBe(0);
  });

  it('should return top-k by cosine similarity', async () => {
    const store = new InMemoryVectorStore();
    await store.add('x', [1, 0, 0]);
    await store.add('y', [0, 1, 0]);
    await store.add('z', [-1, 0, 0]);
    const results = await store.search([1, 0, 0], 2);
    expect(results.length).toBe(2);
    expect(results[0]!.id).toBe('x');
    expect(results[1]!.id).toBe('y'); // y vs z 的 cos similarity 取决于具体计算（0 vs -1）
  });

  it('should reject non-finite vectors', async () => {
    const store = new InMemoryVectorStore();
    await expect(store.add('bad', [NaN, 0, 0])).rejects.toThrow('non-finite');
    await expect(store.add('inf', [Infinity, 0, 0])).rejects.toThrow('non-finite');
  });

  it('should handle empty store', async () => {
    const store = new InMemoryVectorStore();
    const results = await store.search([1, 0], 5);
    expect(results).toHaveLength(0);
  });

  it('should handle k <= 0', async () => {
    const store = new InMemoryVectorStore();
    await store.add('a', [1, 0, 0]);
    expect(await store.search([1, 0, 0], 0)).toHaveLength(0);
    expect(await store.search([1, 0, 0], -1)).toHaveLength(0);
  });

  it('should preserve metadata', async () => {
    const store = new InMemoryVectorStore();
    await store.add('a', [1, 0], { source: 'test', tag: 'x' });
    const results = await store.search([1, 0], 1);
    expect(results[0]!.metadata).toEqual({ source: 'test', tag: 'x' });
  });

  it('should handle zero vectors (cosine returns 0)', async () => {
    const store = new InMemoryVectorStore();
    await store.add('zero', [0, 0, 0]);
    await store.add('real', [1, 0, 0]);
    const results = await store.search([1, 0, 0], 2);
    expect(results.length).toBe(2);
    // real 应排第一
    expect(results[0]!.id).toBe('real');
  });

  it('should throw on dimension mismatch in search', async () => {
    const store = new InMemoryVectorStore();
    await store.add('a', [1, 0, 0]);
    await expect(store.search([1, 0], 5)).rejects.toThrow('dimension mismatch');
  });
});

describe('IndexedDBVectorStore constructor', () => {
  it('should throw on Node environment (no IndexedDB)', async () => {
    await expect(import('../../src/memory/indexeddb-vector-store.js')).resolves.toBeDefined();
    // 直接尝试构造（应当抛出）
    const mod = await import('../../src/memory/indexeddb-vector-store.js');
    expect(() => new mod.IndexedDBVectorStore()).toThrow('IndexedDB not available');
  });
});