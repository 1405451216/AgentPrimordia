import type { VectorSearchResult } from '../types.js';

export class VectorStore {
  private entries: Map<string, { vector: number[]; metadata?: Record<string, string> }> = new Map();
  private dim: number;

  constructor(dimensions: number = 16) {
    this.dim = dimensions;
  }

  add(id: string, vector: number[], metadata?: Record<string, string>): void {
    if (vector.length !== this.dim) {
      throw new Error(`vector dimension mismatch: expected ${this.dim}, got ${vector.length}`);
    }
    if (!vector.every(Number.isFinite)) {
      throw new Error('vector contains non-finite values (NaN or Infinity)');
    }
    this.entries.set(id, { vector, metadata });
  }

  search(query: number[], topK: number = 10): VectorSearchResult[] {
    if (query.length !== this.dim) {
      throw new Error(`query dimension mismatch: expected ${this.dim}, got ${query.length}`);
    }
    if (!query.every(Number.isFinite)) {
      throw new Error('query contains non-finite values (NaN or Infinity)');
    }

    const results: VectorSearchResult[] = [];
    for (const [id, entry] of this.entries) {
      const score = cosineSimilarity(query, entry.vector);
      results.push({ id, score, metadata: entry.metadata });
    }

    results.sort((a, b) => b.score - a.score);
    return results.slice(0, topK);
  }

  delete(id: string): boolean {
    return this.entries.delete(id);
  }

  get(id: string): { vector: number[]; metadata?: Record<string, string> } | undefined {
    return this.entries.get(id);
  }

  count(): number {
    return this.entries.size;
  }

  dimensions(): number {
    return this.dim;
  }
}

function cosineSimilarity(a: number[], b: number[]): number {
  if (a.length !== b.length) return 0;
  let dot = 0, normA = 0, normB = 0;
  for (let i = 0; i < a.length; i++) {
    dot += a[i] * b[i];
    normA += a[i] * a[i];
    normB += b[i] * b[i];
  }
  if (normA === 0 || normB === 0) return 0;
  return dot / (Math.sqrt(normA) * Math.sqrt(normB));
}
