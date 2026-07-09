// indexeddb-vector-store.ts — Phase 1 T1-2 浏览器端向量存储
// 使用 IndexedDB 持久化向量数据，配合暴力搜索（适合中小规模数据集）。
// 生产规模建议迁移到 HNSW 索引 + IndexedDB 元数据，或使用 pgvector/Milvus。
//
// 兼容性：仅在浏览器环境可用（依赖 window.indexedDB）。
// Node.js 环境会抛错，需先用 isIndexedDBAvailable() 检查。
import type { VectorSearchResult } from '../types.js';

export interface IndexedDBVectorRecord {
  id: string;
  vector: number[];
  metadata?: Record<string, unknown>;
  createdAt: number;
}

export interface IndexedDBVectorStoreOptions {
  dbName?: string;
  storeName?: string;
  /** 暴力搜索结果上限（默认 10000） */
  bruteForceLimit?: number;
}

/** 检测当前环境是否支持 IndexedDB */
export function isIndexedDBAvailable(): boolean {
  return typeof globalThis !== 'undefined'
    && typeof (globalThis as { indexedDB?: unknown }).indexedDB !== 'undefined';
}

/**
 * IndexedDBVectorStore — 浏览器端向量存储
 *
 * 设计取舍：
 *   - 简单暴力搜索：scale ≤ ~10k 向量时延迟 < 50ms；超过此规模建议迁移 HNSW
 *   - 持久化：使用 IndexedDB，支持跨页面/跨会话保留
 *   - 异步 API：与 IndexedDB 的事务模型对齐（add/search/delete 均返回 Promise）
 *
 * 已知限制：
 *   - 不支持 cosine vs euclidean 选择（统一用 cosine similarity）
 *   - 单事务大量写入可能触发 QuotaExceededError
 *   - 不支持 partial update（需先 delete 再 add）
 */
export class IndexedDBVectorStore {
  private dbName: string;
  private storeName: string;
  private bruteForceLimit: number;
  private dbPromise: Promise<IDBDatabase> | null = null;

  constructor(options: IndexedDBVectorStoreOptions = {}) {
    this.dbName = options.dbName ?? 'agentprimordia';
    this.storeName = options.storeName ?? 'vectors';
    this.bruteForceLimit = options.bruteForceLimit ?? 10000;
    if (!isIndexedDBAvailable()) {
      throw new Error('IndexedDBVectorStore: IndexedDB not available in this environment. Use isIndexedDBAvailable() to check first.');
    }
  }

  /**
   * 初始化数据库连接（幂等）。通常在构造后立即调用一次，或在第一次 add 前隐式触发。
   */
  async init(): Promise<void> {
    await this.getDB();
  }

  /** 关闭数据库连接（用于资源清理） */
  async close(): Promise<void> {
    if (!this.dbPromise) return;
    const db = await this.dbPromise;
    db.close();
    this.dbPromise = null;
  }

  /**
   * 存储一条向量记录。重复 id 会覆盖。
   */
  async add(id: string, vector: number[], metadata?: Record<string, unknown>): Promise<void> {
    if (!vector.every(Number.isFinite)) {
      throw new Error('IndexedDBVectorStore.add: vector contains non-finite values');
    }
    const db = await this.getDB();
    return new Promise<void>((resolve, reject) => {
      const tx = db.transaction(this.storeName, 'readwrite');
      const store = tx.objectStore(this.storeName);
      const record: IndexedDBVectorRecord = {
        id,
        vector,
        metadata,
        createdAt: Date.now(),
      };
      const req = store.put(record);
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error ?? new Error('IndexedDB transaction failed'));
      req.onerror = () => reject(req.error ?? new Error('IndexedDB put failed'));
    });
  }

  /**
   * 删除指定 id 的记录。
   */
  async delete(id: string): Promise<boolean> {
    const db = await this.getDB();
    return new Promise<boolean>((resolve, reject) => {
      const tx = db.transaction(this.storeName, 'readwrite');
      const store = tx.objectStore(this.storeName);
      const req = store.delete(id);
      req.onsuccess = () => resolve(true);
      req.onerror = () => reject(req.error ?? new Error('IndexedDB delete failed'));
    });
  }

  /**
   * 清空所有记录。
   */
  async clear(): Promise<void> {
    const db = await this.getDB();
    return new Promise<void>((resolve, reject) => {
      const tx = db.transaction(this.storeName, 'readwrite');
      const store = tx.objectStore(this.storeName);
      const req = store.clear();
      req.onsuccess = () => resolve();
      req.onerror = () => reject(req.error ?? new Error('IndexedDB clear failed'));
    });
  }

  /**
   * 获取当前存储的记录数。
   */
  async count(): Promise<number> {
    const db = await this.getDB();
    return new Promise<number>((resolve, reject) => {
      const tx = db.transaction(this.storeName, 'readonly');
      const req = tx.objectStore(this.storeName).count();
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error ?? new Error('IndexedDB count failed'));
    });
  }

  /**
   * 暴力搜索：遍历所有记录计算 cosine similarity，返回 top-k。
   * 当记录数 > bruteForceLimit 时打印警告（不阻断）。
   */
  async search(query: number[], k: number): Promise<VectorSearchResult[]> {
    if (k <= 0) return [];
    if (!query.every(Number.isFinite)) {
      throw new Error('IndexedDBVectorStore.search: query contains non-finite values');
    }
    const records = await this.getAll();
    if (records.length > this.bruteForceLimit) {
      console.warn(
        `[IndexedDBVectorStore] ${records.length} records exceeds brute-force limit ${this.bruteForceLimit}. Consider migrating to HNSW index.`,
      );
    }
    const scored: VectorSearchResult[] = [];
    for (const r of records) {
      const score = cosineSimilarity(query, r.vector);
      scored.push({
        id: r.id,
        score,
        metadata: r.metadata as Record<string, string> | undefined,
      });
    }
    scored.sort((a, b) => b.score - a.score);
    return scored.slice(0, k);
  }

  // ===== 内部 =====

  private async getDB(): Promise<IDBDatabase> {
    if (this.dbPromise) return this.dbPromise;
    this.dbPromise = new Promise<IDBDatabase>((resolve, reject) => {
      const req = indexedDB.open(this.dbName, 1);
      req.onupgradeneeded = () => {
        const db = req.result;
        if (!db.objectStoreNames.contains(this.storeName)) {
          db.createObjectStore(this.storeName, { keyPath: 'id' });
        }
      };
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error ?? new Error('IndexedDB open failed'));
      req.onblocked = () => reject(new Error('IndexedDB open blocked by another connection'));
    });
    return this.dbPromise;
  }

  private async getAll(): Promise<IndexedDBVectorRecord[]> {
    const db = await this.getDB();
    return new Promise<IndexedDBVectorRecord[]>((resolve, reject) => {
      const tx = db.transaction(this.storeName, 'readonly');
      const req = tx.objectStore(this.storeName).getAll();
      req.onsuccess = () => resolve((req.result ?? []) as IndexedDBVectorRecord[]);
      req.onerror = () => reject(req.error ?? new Error('IndexedDB getAll failed'));
    });
  }
}

// ===== 数学辅助 =====

/** cosine similarity：返回 [-1, 1]；零向量返回 0 */
function cosineSimilarity(a: number[], b: number[]): number {
  if (a.length !== b.length) {
    throw new Error(`cosineSimilarity: dimension mismatch ${a.length} vs ${b.length}`);
  }
  let dot = 0;
  let normA = 0;
  let normB = 0;
  for (let i = 0; i < a.length; i++) {
    const av = a[i]!;
    const bv = b[i]!;
    dot += av * bv;
    normA += av * av;
    normB += bv * bv;
  }
  if (normA === 0 || normB === 0) return 0;
  return dot / (Math.sqrt(normA) * Math.sqrt(normB));
}

// ===== Mock 实现（用于 Node 测试） =====

/**
 * InMemoryVectorStore — 与 IndexedDBVectorStore 同接口的内存实现
 * 专为 Node 测试环境设计，不依赖 IndexedDB。
 */
export class InMemoryVectorStore {
  private records = new Map<string, IndexedDBVectorRecord>();

  async init(): Promise<void> { /* no-op */ }
  async close(): Promise<void> { this.records.clear(); }

  async add(id: string, vector: number[], metadata?: Record<string, unknown>): Promise<void> {
    if (!vector.every(Number.isFinite)) {
      throw new Error('InMemoryVectorStore.add: vector contains non-finite values');
    }
    this.records.set(id, { id, vector, metadata, createdAt: Date.now() });
  }

  async delete(id: string): Promise<boolean> {
    return this.records.delete(id);
  }

  async clear(): Promise<void> {
    this.records.clear();
  }

  async count(): Promise<number> {
    return this.records.size;
  }

  async search(query: number[], k: number): Promise<VectorSearchResult[]> {
    const scored: VectorSearchResult[] = [];
    for (const r of this.records.values()) {
      scored.push({
        id: r.id,
        score: cosineSimilarity(query, r.vector),
        metadata: r.metadata as Record<string, string> | undefined,
      });
    }
    scored.sort((a, b) => b.score - a.score);
    return scored.slice(0, k);
  }
}