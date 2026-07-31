/**
 * SIMD Vector Search unit tests
 *
 * Coverage:
 * - Cosine similarity correctness
 * - batchSearch topK return
 * - Chunked search consistency with standard search
 * - Edge cases (empty input, dimension mismatch)
 * - Benchmark
 */
import { describe, it, expect } from 'vitest';
import { SIMDVectorSearch, randomVector, normalize } from '../../src/edge/simd-search.js';
import { WasmSimdSearchEngine, createOptimalSearch } from '../../src/edge/simd-search-wasm.js';

describe('SIMDVectorSearch', () => {
  const search = new SIMDVectorSearch();

  describe('cosineSimilarity', () => {
    it('should return 1.0 for identical vectors', () => {
      const a = new Float32Array([1, 2, 3, 4]);
      const b = new Float32Array([1, 2, 3, 4]);
      expect(search.cosineSimilarity(a, b)).toBeCloseTo(1.0, 5);
    });

    it('should return -1.0 for opposite vectors', () => {
      const a = new Float32Array([1, 0, 0]);
      const b = new Float32Array([-1, 0, 0]);
      expect(search.cosineSimilarity(a, b)).toBeCloseTo(-1.0, 5);
    });

    it('should return 0 for orthogonal vectors', () => {
      const a = new Float32Array([1, 0, 0]);
      const b = new Float32Array([0, 1, 0]);
      expect(search.cosineSimilarity(a, b)).toBeCloseTo(0, 5);
    });

    it('should handle zero vectors', () => {
      const a = new Float32Array([0, 0, 0]);
      const b = new Float32Array([1, 2, 3]);
      expect(search.cosineSimilarity(a, b)).toBe(0);
    });

    it('should handle empty vectors', () => {
      const a = new Float32Array(0);
      const b = new Float32Array(0);
      expect(search.cosineSimilarity(a, b)).toBe(0);
    });

    it('should throw on dimension mismatch', () => {
      const a = new Float32Array([1, 2, 3]);
      const b = new Float32Array([1, 2]);
      expect(() => search.cosineSimilarity(a, b)).toThrow('Dimension mismatch');
    });

    it('should produce correct results for SIMD-aligned and non-aligned lengths', () => {
      const a4 = new Float32Array([1, 2, 3, 4]);
      const b4 = new Float32Array([4, 3, 2, 1]);
      const result4 = search.cosineSimilarity(a4, b4);

      const a5 = new Float32Array([1, 2, 3, 4, 5]);
      const b5 = new Float32Array([4, 3, 2, 1, 0]);
      const result5 = search.cosineSimilarity(a5, b5);

      const dot = 1*4 + 2*3 + 3*2 + 4*1;
      const normA = Math.sqrt(1+4+9+16);
      const normB = Math.sqrt(16+9+4+1);
      expect(result4).toBeCloseTo(dot / (normA * normB), 5);

      const dot5 = 1*4 + 2*3 + 3*2 + 4*1 + 5*0;
      const normA5 = Math.sqrt(1+4+9+16+25);
      const normB5 = Math.sqrt(16+9+4+1+0);
      expect(result5).toBeCloseTo(dot5 / (normA5 * normB5), 5);
    });

    it('should handle single-element vectors', () => {
      const a = new Float32Array([3]);
      const b = new Float32Array([4]);
      expect(search.cosineSimilarity(a, b)).toBeCloseTo(1.0, 5);
    });
  });

  describe('batchSearch', () => {
    it('should return topK results sorted by score', () => {
      const query = new Float32Array([1, 0, 0]);
      const vectors = [
        new Float32Array([0, 1, 0]),
        new Float32Array([1, 0, 0]),
        new Float32Array([0.707, 0.707, 0]),
        new Float32Array([-1, 0, 0]),
      ];

      const results = search.batchSearch(query, vectors, 2);
      expect(results.length).toBe(2);
      expect(results[0]!.index).toBe(1);
      expect(results[1]!.index).toBe(2);
      expect(results[0]!.score).toBeGreaterThan(results[1]!.score);
    });

    it('should return empty for empty vectors', () => {
      const query = new Float32Array([1, 0, 0]);
      expect(search.batchSearch(query, [], 3)).toEqual([]);
    });

    it('should clamp topK to vectors length', () => {
      const query = new Float32Array([1, 0, 0]);
      const vectors = [
        new Float32Array([1, 0, 0]),
        new Float32Array([0, 1, 0]),
      ];
      const results = search.batchSearch(query, vectors, 10);
      expect(results.length).toBe(2);
    });

    it('should handle topK=1', () => {
      const query = new Float32Array([1, 0, 0]);
      const vectors = [
        new Float32Array([0, 1, 0]),
        new Float32Array([1, 0, 0]),
        new Float32Array([0.5, 0.5, 0]),
      ];
      const results = search.batchSearch(query, vectors, 1);
      expect(results.length).toBe(1);
      expect(results[0]!.index).toBe(1);
    });

    it('should return empty for topK < 1', () => {
      const query = new Float32Array([1, 0, 0]);
      const vectors = [new Float32Array([1, 0, 0])];
      expect(search.batchSearch(query, vectors, 0)).toEqual([]);
    });
  });

  describe('batchSearchChunked', () => {
    it('should produce same results as batchSearch', () => {
      const query = new Float32Array([1, 0, 0, 0]);
      const vectors: Float32Array[] = [];
      for (let i = 0; i < 100; i++) {
        vectors.push(randomVector(4));
      }

      const results1 = search.batchSearch(query, vectors, 5);
      const results2 = search.batchSearchChunked(query, vectors, 5);

      expect(results1.length).toBe(results2.length);
      for (let i = 0; i < results1.length; i++) {
        expect(results1[i]!.index).toBe(results2[i]!.index);
        expect(results1[i]!.score).toBeCloseTo(results2[i]!.score, 4);
      }
    });

    it('should handle large datasets', () => {
      const query = new Float32Array([1, 0, 0, 0, 0, 0, 0, 0]);
      const vectors: Float32Array[] = [];
      for (let i = 0; i < 1000; i++) {
        vectors.push(randomVector(8));
      }

      const results = search.batchSearchChunked(query, vectors, 10);
      expect(results.length).toBe(10);
      for (let i = 1; i < results.length; i++) {
        expect(results[i]!.score).toBeLessThanOrEqual(results[i - 1]!.score);
      }
    });
  });

  describe('benchmark', () => {
    it('should run benchmark without error', () => {
      const dim = 64;
      const query = randomVector(dim);
      const vectors: Float32Array[] = [];
      for (let i = 0; i < 100; i++) {
        vectors.push(randomVector(dim));
      }

      const result = SIMDVectorSearch.benchmark(query, vectors, 5, 10);
      expect(result.optimizedMs).toBeGreaterThanOrEqual(0);
      expect(result.baselineMs).toBeGreaterThanOrEqual(0);
      expect(result.speedup).toBeGreaterThan(0);
    });
  });

  describe('utility functions', () => {
    it('should generate random vectors of correct length', () => {
      const v = randomVector(32);
      expect(v.length).toBe(32);
      expect(v).toBeInstanceOf(Float32Array);
    });

    it('should normalize vectors to unit length', () => {
      const v = new Float32Array([3, 4]);
      const n = normalize(v);
      const len = Math.sqrt(n[0]! * n[0]! + n[1]! * n[1]!);
      expect(len).toBeCloseTo(1.0, 5);
    });
  });
});

describe('WasmSimdSearchEngine', () => {
  const jsSearch = new SIMDVectorSearch();

  describe('cosineSimilarity consistency with JS path', () => {
    it('should return same results as SIMDVectorSearch for identical vectors', async () => {
      const wasm = new WasmSimdSearchEngine();
      await wasm.init();

      const a = new Float32Array([1, 2, 3, 4]);
      const b = new Float32Array([1, 2, 3, 4]);
      const wasmResult = wasm.cosineSimilarity(a, b);
      const jsResult = jsSearch.cosineSimilarity(a, b);
      expect(wasmResult).toBeCloseTo(jsResult, 5);
    });

    it('should return same results for orthogonal vectors', async () => {
      const wasm = new WasmSimdSearchEngine();
      await wasm.init();

      const a = new Float32Array([1, 0, 0]);
      const b = new Float32Array([0, 1, 0]);
      expect(wasm.cosineSimilarity(a, b)).toBeCloseTo(jsSearch.cosineSimilarity(a, b), 5);
    });

    it('should return same results for random 128-dim vectors', async () => {
      const wasm = new WasmSimdSearchEngine();
      await wasm.init();

      const a = randomVector(128);
      const b = randomVector(128);
      const wasmResult = wasm.cosineSimilarity(a, b);
      const jsResult = jsSearch.cosineSimilarity(a, b);
      expect(wasmResult).toBeCloseTo(jsResult, 4);
    });

    it('should handle zero vectors', async () => {
      const wasm = new WasmSimdSearchEngine();
      await wasm.init();

      const a = new Float32Array([0, 0, 0]);
      const b = new Float32Array([1, 2, 3]);
      expect(wasm.cosineSimilarity(a, b)).toBe(0);
    });

    it('should throw on dimension mismatch', async () => {
      const wasm = new WasmSimdSearchEngine();
      await wasm.init();

      const a = new Float32Array([1, 2, 3]);
      const b = new Float32Array([1, 2]);
      expect(() => wasm.cosineSimilarity(a, b)).toThrow('Dimension mismatch');
    });
  });

  describe('batchSearch consistency', () => {
    it('should return same topK results as JS implementation', async () => {
      const wasm = new WasmSimdSearchEngine();
      await wasm.init();

      const query = randomVector(64);
      const vectors: Float32Array[] = [];
      for (let i = 0; i < 50; i++) vectors.push(randomVector(64));

      const wasmResults = wasm.batchSearch(query, vectors, 5);
      const jsResults = jsSearch.batchSearch(query, vectors, 5);

      expect(wasmResults.length).toBe(jsResults.length);
      for (let i = 0; i < wasmResults.length; i++) {
        expect(wasmResults[i]!.index).toBe(jsResults[i]!.index);
        expect(wasmResults[i]!.score).toBeCloseTo(jsResults[i]!.score, 4);
      }
    });

    it('should handle empty vectors array', async () => {
      const wasm = new WasmSimdSearchEngine();
      await wasm.init();

      const query = new Float32Array([1, 0, 0]);
      expect(wasm.batchSearch(query, [], 3)).toEqual([]);
    });
  });

  describe('createOptimalSearch', () => {
    it('should return a valid search engine with backend info', async () => {
      const { engine, backend } = await createOptimalSearch();
      expect(engine).toBeDefined();
      expect(['wasm-simd', 'wasm-scalar', 'js-typedarray']).toContain(backend);

      // Verify it can perform search
      const query = new Float32Array([1, 0, 0]);
      const vectors = [new Float32Array([1, 0, 0]), new Float32Array([0, 1, 0])];
      const results = engine.batchSearch(query, vectors, 1);
      expect(results.length).toBe(1);
      expect(results[0]!.index).toBe(0);
    });
  });
});
