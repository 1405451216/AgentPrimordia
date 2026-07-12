/**
 * WASM SIMD Vector Search - optimized cosine similarity using TypedArray.
 */
export interface SearchResult {
  index: number;
  score: number;
}

const SIMD_WIDTH = 4;
const BLOCK_SIZE = 64;

export class SIMDVectorSearch {
  cosineSimilarity(a: Float32Array, b: Float32Array): number {
    if (a.length !== b.length) throw new Error(`Dimension mismatch: a.length=${a.length}, b.length=${b.length}`);
    if (a.length === 0) return 0;
    const len = a.length;
    let dot = 0, normA = 0, normB = 0;
    let i = 0;
    const simdEnd = len - (len % SIMD_WIDTH);
    for (; i < simdEnd; i += SIMD_WIDTH) {
      const a0 = a[i]!, a1 = a[i + 1]!, a2 = a[i + 2]!, a3 = a[i + 3]!;
      const b0 = b[i]!, b1 = b[i + 1]!, b2 = b[i + 2]!, b3 = b[i + 3]!;
      dot += a0 * b0 + a1 * b1 + a2 * b2 + a3 * b3;
      normA += a0 * a0 + a1 * a1 + a2 * a2 + a3 * a3;
      normB += b0 * b0 + b1 * b1 + b2 * b2 + b3 * b3;
    }
    for (; i < len; i++) {
      const ai = a[i]!, bi = b[i]!;
      dot += ai * bi;
      normA += ai * ai;
      normB += bi * bi;
    }
    const denom = Math.sqrt(normA) * Math.sqrt(normB);
    if (denom === 0) return 0;
    return dot / denom;
  }

  batchSearch(query: Float32Array, vectors: Float32Array[], topK: number): SearchResult[] {
    if (vectors.length === 0) return [];
    if (topK < 1) return [];
    if (topK > vectors.length) topK = vectors.length;
    const scores: SearchResult[] = [];
    for (let i = 0; i < vectors.length; i++) {
      scores.push({ index: i, score: this.cosineSimilarity(query, vectors[i]!) });
    }
    return this.selectTopK(scores, topK);
  }

  batchSearchChunked(query: Float32Array, vectors: Float32Array[], topK: number): SearchResult[] {
    if (vectors.length === 0) return [];
    if (topK < 1) return [];
    if (topK > vectors.length) topK = vectors.length;
    const heap: SearchResult[] = [];
    for (let blockStart = 0; blockStart < vectors.length; blockStart += BLOCK_SIZE) {
      const blockEnd = Math.min(blockStart + BLOCK_SIZE, vectors.length);
      for (let i = blockStart; i < blockEnd; i++) {
        const score = this.cosineSimilarity(query, vectors[i]!);
        this.insertToHeap(heap, { index: i, score }, topK);
      }
    }
    return heap.sort((a, b) => b.score - a.score);
  }

  static benchmark(query: Float32Array, vectors: Float32Array[], topK: number, iterations: number = 100): { optimizedMs: number; baselineMs: number; speedup: number } {
    const search = new SIMDVectorSearch();
    const baseline = new NaiveVectorSearch();
    for (let i = 0; i < 10; i++) {
      search.batchSearch(query, vectors, topK);
      baseline.batchSearch(query, vectors, topK);
    }
    const t1 = performance.now();
    for (let i = 0; i < iterations; i++) search.batchSearch(query, vectors, topK);
    const optimizedMs = performance.now() - t1;
    const t2 = performance.now();
    for (let i = 0; i < iterations; i++) baseline.batchSearch(query, vectors, topK);
    const baselineMs = performance.now() - t2;
    return { optimizedMs, baselineMs, speedup: baselineMs > 0 ? optimizedMs / baselineMs : 1 };
  }

  private selectTopK(scores: SearchResult[], topK: number): SearchResult[] {
    const heap: SearchResult[] = [];
    for (const item of scores) this.insertToHeap(heap, item, topK);
    return heap.sort((a, b) => b.score - a.score);
  }

  private insertToHeap(heap: SearchResult[], item: SearchResult, maxSize: number): void {
    if (heap.length < maxSize) {
      heap.push(item);
      this.heapifyUp(heap, heap.length - 1);
    } else if (item.score > heap[0]!.score) {
      heap[0] = item;
      this.heapifyDown(heap, 0);
    }
  }

  private heapifyUp(heap: SearchResult[], idx: number): void {
    while (idx > 0) {
      const parent = Math.floor((idx - 1) / 2);
      if (heap[parent]!.score <= heap[idx]!.score) break;
      [heap[parent], heap[idx]] = [heap[idx]!, heap[parent]!];
      idx = parent;
    }
  }

  private heapifyDown(heap: SearchResult[], idx: number): void {
    const len = heap.length;
    while (true) {
      let smallest = idx;
      const left = idx * 2 + 1;
      const right = idx * 2 + 2;
      if (left < len && heap[left]!.score < heap[smallest]!.score) smallest = left;
      if (right < len && heap[right]!.score < heap[smallest]!.score) smallest = right;
      if (smallest === idx) break;
      [heap[smallest], heap[idx]] = [heap[idx]!, heap[smallest]!];
      idx = smallest;
    }
  }
}

class NaiveVectorSearch {
  cosineSimilarity(a: Float32Array, b: Float32Array): number {
    if (a.length !== b.length) throw new Error('Dimension mismatch');
    let dot = 0, normA = 0, normB = 0;
    for (let i = 0; i < a.length; i++) {
      dot += a[i]! * b[i]!;
      normA += a[i]! * a[i]!;
      normB += b[i]! * b[i]!;
    }
    const denom = Math.sqrt(normA) * Math.sqrt(normB);
    if (denom === 0) return 0;
    return dot / denom;
  }
  batchSearch(query: Float32Array, vectors: Float32Array[], topK: number): SearchResult[] {
    const scores: SearchResult[] = [];
    for (let i = 0; i < vectors.length; i++) {
      scores.push({ index: i, score: this.cosineSimilarity(query, vectors[i]!) });
    }
    return scores.sort((a, b) => b.score - a.score).slice(0, topK);
  }
}

export function randomVector(dim: number): Float32Array {
  const v = new Float32Array(dim);
  for (let i = 0; i < dim; i++) v[i] = Math.random() * 2 - 1;
  return v;
}

export function normalize(v: Float32Array): Float32Array {
  let norm = 0;
  for (let i = 0; i < v.length; i++) norm += v[i]! * v[i]!;
  norm = Math.sqrt(norm);
  if (norm === 0) return v;
  const result = new Float32Array(v.length);
  for (let i = 0; i < v.length; i++) result[i] = v[i]! / norm;
  return result;
}
