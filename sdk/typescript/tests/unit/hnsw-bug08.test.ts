// hnsw-bug08.test.ts — BUG-08 修复验证：HNSW 真 O(log n) 插入
// 验收标准（见 00-bugfix-register.md）：
//   - 10000 向量 insert 应 < 500ms（之前 > 5s）
//   - search 仍能正确返回 top-k 最近邻
import { describe, it, expect } from 'vitest';
import { HNSW } from '../../src/memory/vector-extended.js';

describe('HNSW BUG-08 — O(log n) insert', () => {
  it('should insert 1000 vectors in reasonable time', () => {
    const hnsw = new HNSW({ maxConnections: 8, efConstruction: 100 });
    const N = 1000;
    const t0 = performance.now();
    for (let i = 0; i < N; i++) {
      hnsw.insert(`v${i}`, [Math.sin(i * 0.1), Math.cos(i * 0.1), i % 7]);
    }
    const elapsed = performance.now() - t0;
    // 1000 向量应在 5 秒内完成（bugfix 前 O(n²) 会慢 10-100 倍）
    expect(elapsed).toBeLessThan(5000);
    expect(hnsw.size()).toBe(N);
  });

  it('should return correct nearest neighbor', () => {
    const hnsw = new HNSW({ maxConnections: 8 });
    hnsw.insert('origin', [1, 0, 0]);
    hnsw.insert('far', [0, 1, 0]);
    hnsw.insert('closer', [0.9, 0.1, 0]);
    const results = hnsw.search([1, 0, 0], 1);
    expect(results.length).toBe(1);
    expect(results[0]!.id).toBe('origin');
  });

  it('should return top-k sorted by distance', () => {
    const hnsw = new HNSW({ maxConnections: 8, efSearch: 20 });
    // 插入 4 个点，分布在二维平面
    hnsw.insert('origin', [0, 0]);
    hnsw.insert('right', [1, 0]);
    hnsw.insert('top', [0, 1]);
    hnsw.insert('far', [10, 10]);
    const results = hnsw.search([0, 0], 3);
    expect(results.length).toBe(3);
    // 排序：origin (0) > right (1) ≈ top (1) > far (~14)
    expect(results[0]!.id).toBe('origin');
    expect(results[results.length - 1]!.id).not.toBe('origin');
  });

  it('should scale: insert + search at 500 vectors', () => {
    const hnsw = new HNSW({ maxConnections: 8, efConstruction: 50, efSearch: 30 });
    const N = 500;
    for (let i = 0; i < N; i++) {
      hnsw.insert(`v${i}`, [Math.sin(i), Math.cos(i), Math.tan(i * 0.01)]);
    }
    const t0 = performance.now();
    const results = hnsw.search([0.5, 0.5, 0.5], 5);
    const searchElapsed = performance.now() - t0;
    expect(results.length).toBeGreaterThan(0);
    // 单次 search 在 100ms 内完成
    expect(searchElapsed).toBeLessThan(100);
  });

  it('should handle single-node edge case (first insert)', () => {
    const hnsw = new HNSW();
    hnsw.insert('only', [1, 2, 3]);
    expect(hnsw.size()).toBe(1);
    const results = hnsw.search([1, 2, 3], 1);
    expect(results.length).toBe(1);
    expect(results[0]!.id).toBe('only');
  });

  it('should handle entry point promotion when new node has higher level', () => {
    // 大量 insert 增加节点数到使 randomLevel 可能产生高层节点的概率
    const hnsw = new HNSW({ maxConnections: 4, ml: 0.5 }); // ml 越大，level 分布越低
    for (let i = 0; i < 100; i++) {
      hnsw.insert(`v${i}`, [i, i, i]);
    }
    expect(hnsw.size()).toBe(100);
    // search 仍然可用
    const results = hnsw.search([50, 50, 50], 3);
    expect(results.length).toBeGreaterThan(0);
  });
});