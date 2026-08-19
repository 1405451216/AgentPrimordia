/**
 * searchLayer MinHeap 优化测试（TODO 欠账清理 Task 2）
 *
 * 覆盖：
 * - BinaryHeap 单元行为（push/pop 有序性、自定义比较器、空堆边界）
 * - HNSW 检索质量回归守卫：与暴力 kNN 对比召回率，
 *   确保堆替换不改变 searchLayer 的搜索语义
 */
import { describe, it, expect } from 'vitest';
import { HNSW, BinaryHeap } from '../../src/memory/vector-extended.js';

// ===== 确定性伪随机数（LCG），保证测试可复现 =====

function makeRng(seed: number): () => number {
  let s = seed >>> 0;
  return () => {
    s = (s * 1664525 + 1013904223) >>> 0;
    return s / 0x100000000;
  };
}

/**
 * 在 fn 执行期间用种子化 LCG 替换 Math.random。
 * HNSW.randomLevel() 依赖 Math.random，若不种子化，图层级结构
 * 每次运行都不同，召回率会在较大区间内抖动（实测 0.47~0.79），
 * 导致回归守卫测试 flaky。种子化后整张图确定，召回可复现。
 */
function withSeededRandom<T>(seed: number, fn: () => T): T {
  const original = Math.random;
  Math.random = makeRng(seed);
  try {
    return fn();
  } finally {
    Math.random = original;
  }
}

describe('BinaryHeap', () => {
  it('push/pop 应按比较器升序弹出全部元素', () => {
    const heap = new BinaryHeap<number>((a, b) => a - b);
    const rng = makeRng(42);
    const input: number[] = [];
    for (let i = 0; i < 1000; i++) {
      const v = Math.floor(rng() * 100000);
      input.push(v);
      heap.push(v);
    }
    expect(heap.size).toBe(1000);

    const sorted = [...input].sort((a, b) => a - b);
    const popped: number[] = [];
    while (!heap.isEmpty()) {
      popped.push(heap.pop()!);
    }
    expect(popped).toEqual(sorted);
    expect(heap.pop()).toBeUndefined();
  });

  it('反向比较器应实现最大堆语义', () => {
    const maxHeap = new BinaryHeap<number>((a, b) => b - a);
    for (const v of [3, 1, 4, 1, 5, 9, 2, 6]) maxHeap.push(v);
    expect(maxHeap.peek()).toBe(9);
    expect(maxHeap.pop()).toBe(9);
    expect(maxHeap.pop()).toBe(6);
    expect(maxHeap.size).toBe(6);
  });

  it('peek 不移除堆顶；空堆 pop/peek 安全', () => {
    const heap = new BinaryHeap<{ id: string; dist: number }>((a, b) => a.dist - b.dist);
    expect(heap.isEmpty()).toBe(true);
    expect(heap.peek()).toBeUndefined();
    expect(heap.pop()).toBeUndefined();

    heap.push({ id: 'a', dist: 0.5 });
    heap.push({ id: 'b', dist: 0.1 });
    expect(heap.peek()!.id).toBe('b');
    expect(heap.size).toBe(2);
  });

  it('对象元素按字段比较：与 searchLayer 用法一致', () => {
    const heap = new BinaryHeap<{ id: string; dist: number }>((a, b) => a.dist - b.dist);
    const rng = makeRng(7);
    const items = Array.from({ length: 200 }, (_, i) => ({ id: `n${i}`, dist: rng() }));
    for (const it of items) heap.push(it);

    const sorted = [...items].sort((a, b) => a.dist - b.dist);
    for (const expected of sorted) {
      expect(heap.pop()!.id).toBe(expected.id);
    }
  });
});

describe('HNSW searchLayer 堆优化后的检索质量（回归守卫）', () => {
  it('300 向量 8 维：k=10 召回率相对暴力 kNN 不低于 0.6（回归守卫）', () => {
    // 阈值依据：A/B 实测该简化版 HNSW 的召回率基线约 0.70-0.79
    // （瓶颈在 search() 的 BFS 候选上限，与 searchLayer 无关）。
    // 本守卫用于捕捉 searchLayer 语义破坏（如提前退出条件错误会显著拉低召回），
    // 阈值留足图随机性（randomLevel 用 Math.random）的波动余量。
    const rng = makeRng(2026);
    const dims = 8;
    const N = 300;
    const vectors: number[][] = [];
    const hnsw = new HNSW({ maxConnections: 16, efConstruction: 100, efSearch: 100 });

    // 种子化 Math.random 使图层级结构确定化（消除召回抖动）
    withSeededRandom(7, () => {
      for (let i = 0; i < N; i++) {
        const v = Array.from({ length: dims }, () => rng());
        vectors.push(v);
        hnsw.insert(`v${i}`, v);
      }
    });
    expect(hnsw.size()).toBe(N);

    const dist = (a: number[], b: number[]): number => {
      let sum = 0;
      for (let i = 0; i < a.length; i++) {
        const d = a[i]! - b[i]!;
        sum += d * d;
      }
      return Math.sqrt(sum);
    };

    let totalRecall = 0;
    const queries = 10;
    for (let q = 0; q < queries; q++) {
      const query = Array.from({ length: dims }, () => rng());
      const k = 10;

      // 暴力精确 kNN（基准）
      const exact = vectors
        .map((v, i) => ({ id: `v${i}`, d: dist(query, v) }))
        .sort((a, b) => a.d - b.d)
        .slice(0, k)
        .map((x) => x.id);

      const got = hnsw.search(query, k).map((r) => r.id);
      expect(got.length).toBe(k);
      const hit = got.filter((id) => exact.includes(id)).length;
      totalRecall += hit / k;
    }

    const avgRecall = totalRecall / queries;
    expect(avgRecall).toBeGreaterThanOrEqual(0.6);
  });

  it('插入路径走 searchLayer：1000 向量批量插入后 top-1 质量不退化（对暴力最优近似比 ≤ 2）', () => {
    const rng = makeRng(99);
    const hnsw = new HNSW({ maxConnections: 12, efConstruction: 60, efSearch: 100 });
    const N = 1000;
    const vectors: number[][] = [];
    withSeededRandom(13, () => {
      for (let i = 0; i < N; i++) {
        const v = [rng(), rng(), rng()];
        vectors.push(v);
        hnsw.insert(`v${i}`, v);
      }
    });

    const dist = (a: number[], b: number[]): number => {
      let sum = 0;
      for (let i = 0; i < a.length; i++) {
        const d = a[i]! - b[i]!;
        sum += d * d;
      }
      return Math.sqrt(sum);
    };

    // 用数据集外的随机查询点，暴力最优距离非零，近似比有定义
    const query = [rng(), rng(), rng()];
    const bestDist = Math.min(...vectors.map((v) => dist(query, v)));
    expect(bestDist).toBeGreaterThan(0);

    const probe = hnsw.search(query, 1);
    expect(probe.length).toBe(1);
    // 由 score 反推实际距离：score = 1/(1+dist)
    const gotDist = 1 / probe[0]!.score - 1;
    expect(gotDist / bestDist).toBeLessThanOrEqual(2.0);
  });
});
