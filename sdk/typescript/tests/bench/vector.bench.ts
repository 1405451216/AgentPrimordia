/**
 * Vector Store 性能基准测试
 * 测试 HNSW 索引的 add/search 性能，与 Go 端 VectorStore 对齐
 */
import { describe, bench } from 'vitest';
import { VectorStore } from '../../src/memory/vector.js';

function randomVector(dim: number): number[] {
  const v: number[] = [];
  let norm = 0;
  for (let i = 0; i < dim; i++) {
    const val = Math.random() * 2 - 1;
    v.push(val);
    norm += val * val;
  }
  // Normalize
  norm = Math.sqrt(norm);
  if (norm > 0) {
    for (let i = 0; i < dim; i++) {
      v[i] /= norm;
    }
  }
  return v;
}

describe('VectorStore Add', () => {
  bench('add 100 vectors (dim=16)', () => {
    const store = new VectorStore(16);
    for (let i = 0; i < 100; i++) {
      store.add(`vec-${i}`, randomVector(16));
    }
  });

  bench('add 100 vectors (dim=128)', () => {
    const store = new VectorStore(128);
    for (let i = 0; i < 100; i++) {
      store.add(`vec-${i}`, randomVector(128));
    }
  });

  bench('add 500 vectors (dim=16)', () => {
    const store = new VectorStore(16);
    for (let i = 0; i < 500; i++) {
      store.add(`vec-${i}`, randomVector(16));
    }
  });
});

describe('VectorStore Search', () => {
  // Pre-populate
  const setupStore = (n: number, dim: number): VectorStore => {
    const store = new VectorStore(dim);
    for (let i = 0; i < n; i++) {
      store.add(`vec-${i}`, randomVector(dim));
    }
    return store;
  };

  const store100 = setupStore(100, 16);
  const store500 = setupStore(500, 16);

  bench('search top-5 in 100 vectors', () => {
    store100.search(randomVector(16), 5);
  });

  bench('search top-10 in 100 vectors', () => {
    store100.search(randomVector(16), 10);
  });

  bench('search top-5 in 500 vectors', () => {
    store500.search(randomVector(16), 5);
  });

  bench('search top-10 in 500 vectors', () => {
    store500.search(randomVector(16), 10);
  });
});

describe('VectorStore Delete', () => {
  bench('delete 50 from 100 vectors', () => {
    const store = new VectorStore(16);
    for (let i = 0; i < 100; i++) {
      store.add(`vec-${i}`, randomVector(16));
    }
    for (let i = 0; i < 50; i++) {
      store.delete(`vec-${i}`);
    }
  });
});

describe('VectorStore vs Brute Force', () => {
  // Compare HNSW vs brute force for small datasets
  bench('brute force search (100 vectors)', () => {
    const vectors: { id: string; vec: number[] }[] = [];
    const dim = 16;
    for (let i = 0; i < 100; i++) {
      vectors.push({ id: `bf-${i}`, vec: randomVector(dim) });
    }
    const query = randomVector(dim);
    // Brute force
    const scored = vectors.map((v) => ({
      id: v.id,
      score: v.vec.reduce((sum, val, idx) => sum + val * query[idx], 0),
    }));
    scored.sort((a, b) => b.score - a.score);
    scored.slice(0, 5);
  });
});
