// hnsw-recall.test.ts — v5.1 检索质量革命：双实现 recall@10 量化门（V6-ROADMAP §三 任务 2）
//
// 验收标准（V6-ROADMAP.md 铁律 4）：
//   - 双线（VectorStore / HNSW）recall@10 ≥ 0.95
//   - 数据集规模 300 / 1000 / 3000 三档，固定种子可复现
//   - 聚类高斯分布模拟真实 embedding 数据（10 个簇心）
//
// 立项证据：HNSW.search() 原 layer-0 为 BFS + 硬上限，按队列顺序扩展，
// 召回实测 ≈0.7；本测试即其回归门。
import { describe, it, expect } from 'vitest';
import { VectorStore } from '../../src/memory/vector.js';
import { HNSW } from '../../src/memory/vector-extended.js';

// ===== 固定种子 PRNG（mulberry32），保证数据集跨运行可复现 =====
function mulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

// Box-Muller 变换生成标准正态随机数
function gaussian(rand: () => number): number {
  let u = 0;
  let v = 0;
  while (u === 0) u = rand();
  while (v === 0) v = rand();
  return Math.sqrt(-2 * Math.log(u)) * Math.cos(2 * Math.PI * v);
}

const DIM = 16;
const CLUSTERS = 10;

interface Dataset {
  vectors: number[][];
  queries: number[][];
}

/**
 * 生成聚类高斯数据集：CLUSTERS 个簇心 + 簇内高斯扰动，
 * 模拟真实 embedding 的流形结构（比均匀随机更能暴露 ANN 图连通性缺陷）。
 */
function generateDataset(n: number, seed: number): Dataset {
  const rand = mulberry32(seed);
  const centroids: number[][] = [];
  for (let c = 0; c < CLUSTERS; c++) {
    const centroid: number[] = [];
    for (let d = 0; d < DIM; d++) centroid.push(gaussian(rand) * 5);
    centroids.push(centroid);
  }
  const pick = (v: number[][]): number[] => {
    const base = centroids[Math.floor(rand() * CLUSTERS)]!;
    return base.map((x) => x + gaussian(rand));
  };
  const vectors: number[][] = [];
  for (let i = 0; i < n; i++) vectors.push(pick(vectors));
  const queries: number[][] = [];
  const Q = 20;
  for (let q = 0; q < Q; q++) queries.push(pick(vectors));
  return { vectors, queries };
}

// ===== 距离度量（与被测实现一致）=====

function euclidean(a: number[], b: number[]): number {
  let sum = 0;
  for (let i = 0; i < a.length; i++) {
    const diff = a[i]! - b[i]!;
    sum += diff * diff;
  }
  return Math.sqrt(sum);
}

function cosineSim(a: number[], b: number[]): number {
  let dot = 0;
  let na = 0;
  let nb = 0;
  for (let i = 0; i < a.length; i++) {
    dot += a[i]! * b[i]!;
    na += a[i]! * a[i]!;
    nb += b[i]! * b[i]!;
  }
  if (na === 0 || nb === 0) return 0;
  return dot / (Math.sqrt(na) * Math.sqrt(nb));
}

/** 暴力搜索 ground truth：返回前 k 近的 id 集合 */
function bruteForce(
  vectors: Array<{ id: string; vector: number[] }>,
  query: number[],
  k: number,
  dist: (a: number[], b: number[]) => number,
): Set<string> {
  const scored = vectors.map((v) => ({ id: v.id, d: dist(query, v.vector) }));
  scored.sort((a, b) => a.d - b.d);
  return new Set(scored.slice(0, k).map((s) => s.id));
}

const SIZES = [300, 1000, 3000];
const K = 10;
// 门限：双线 recall@10 ≥ 0.95（V6-ROADMAP 验收）
const RECALL_GATE = 0.95;

describe('v5.1 检索质量革命 — recall@10 固定种子量化门', () => {
  for (const size of SIZES) {
    it(`HNSW (vector-extended) recall@10 >= ${RECALL_GATE} @ N=${size}`, () => {
      const { vectors, queries } = generateDataset(size, 42 + size);
      // 注入固定种子随机源：图构建（层级生成）确定性可复现
      const hnsw = new HNSW({
        maxConnections: 16,
        efConstruction: 200,
        efSearch: 50,
        random: mulberry32(7 + size),
      });
      const items = vectors.map((v, i) => ({ id: `v${i}`, vector: v }));
      for (const item of items) hnsw.insert(item.id, item.vector);

      let totalRecall = 0;
      for (const query of queries) {
        const truth = bruteForce(items, query, K, euclidean);
        const results = hnsw.search(query, K);
        const hits = results.filter((r) => truth.has(r.id)).length;
        totalRecall += hits / K;
      }
      const recall = totalRecall / queries.length;
      expect(recall).toBeGreaterThanOrEqual(RECALL_GATE);
    });

    it(`VectorStore (vector) recall@10 >= ${RECALL_GATE} @ N=${size}`, () => {
      const { vectors, queries } = generateDataset(size, 42 + size);
      // 注意：vector.ts 在 N<=100 时走暴力路径，此处 N>=300 强制走图路径；
      // 注入固定种子随机源保证图构建确定性可复现
      const store = new VectorStore(DIM, {
        M: 16,
        efConstruction: 200,
        efSearch: 50,
        random: mulberry32(11 + size),
      });
      const items = vectors.map((v, i) => ({ id: `v${i}`, vector: v }));
      for (const item of items) store.add(item.id, item.vector);

      let totalRecall = 0;
      for (const query of queries) {
        const truth = bruteForce(items, query, K, (a, b) => -cosineSim(a, b));
        const results = store.search(query, K);
        const hits = results.filter((r) => truth.has(r.id)).length;
        totalRecall += hits / K;
      }
      const recall = totalRecall / queries.length;
      expect(recall).toBeGreaterThanOrEqual(RECALL_GATE);
    });
  }
});
