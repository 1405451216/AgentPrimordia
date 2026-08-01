/**
 * Agent 记忆持久化 — 长期记忆 + 向量数据库。
 *
 * 在 Memory 接口之上构建的长期记忆层，提供：
 * - 跨会话记忆持久化（文件/SQLite 后端）
 * - 向量语义搜索（基于 VectorStore HNSW 索引）
 * - 记忆摘要与重要性衰减
 * - 混合检索：全文搜索 + 语义搜索 + 时间衰减
 *
 * 架构：
 *   ShortTermMemory (InMemoryStore)  ← 当前会话
 *         ↓ flush
 *   LongTermMemory (PersistentStore) ← 跨会话持久化
 *         ↓ index
 *   VectorIndex (HNSW)               ← 语义搜索
 *
 * 使用方式：
 *   const ltm = new LongTermMemory({ persistPath: './memory.db' });
 *   await ltm.add(episode);
 *   const results = await ltm.semanticSearch('用户偏好', 5);
 */

import type { MemoryEpisode, MemoryStats, SearchOptions, ListOptions, VectorSearchResult } from '../types.js';
import type { Memory } from './store.js';
import { InMemoryStore } from './store.js';
import { VectorStore } from './vector.js';
import * as fs from 'node:fs';
import * as path from 'node:path';

// ===== 类型定义 =====

/** 长期记忆配置 */
export interface LongTermMemoryConfig {
  /** 持久化文件路径（JSON 格式，可选） */
  persistPath?: string;
  /** 向量维度（默认 128） */
  vectorDimensions?: number;
  /** 是否自动持久化（默认 true） */
  autoPersist?: boolean;
  /** 持久化间隔（毫秒，默认 5000） */
  persistInterval?: number;
  /** 重要性衰减因子（每天衰减比例，默认 0.95） */
  importanceDecay?: number;
  /** HNSW 参数 */
  hnswM?: number;
  hnswEfConstruction?: number;
  hnswEfSearch?: number;
}

/** 混合搜索结果 */
export interface HybridSearchResult {
  /** 记忆片段 */
  episode: MemoryEpisode;
  /** 综合得分 [0, 1] */
  score: number;
  /** 得分组成 */
  components: {
    /** 全文搜索得分 */
    lexical: number;
    /** 语义相似度得分 */
    semantic: number;
    /** 时间衰减因子 */
    recency: number;
    /** 重要性加成 */
    importance: number;
  };
}

// ===== 简易 Embedding 生成器 =====

/**
 * 轻量级文本向量化器。
 *
 * 使用哈希 + 降维的方式生成固定维度向量，
 * 无需外部模型依赖。适合原型开发和轻量场景。
 *
 * 对于生产环境，建议替换为真实 embedding 模型
 * （如 OpenAI text-embedding-3-small）。
 */
export class HashEmbedding {
  private dim: number;

  constructor(dimensions: number = 128) {
    this.dim = dimensions;
  }

  /** 将文本转为向量 */
  embed(text: string): number[] {
    const vector = new Array(this.dim).fill(0);
    const tokens = text.toLowerCase().split(/[^\p{L}\p{N}]+/u).filter(Boolean);

    for (const token of tokens) {
      // DJB2 哈希
      let hash = 5381;
      for (let i = 0; i < token.length; i++) {
        hash = ((hash << 5) + hash + token.charCodeAt(i)) | 0;
      }
      // 双哈希分散到不同维度
      const idx1 = Math.abs(hash) % this.dim;
      const idx2 = Math.abs(hash >> 8) % this.dim;
      vector[idx1]! += 1;
      vector[idx2]! += 0.5;
    }

    // L2 归一化
    const norm = Math.sqrt(vector.reduce((s, v) => s + v * v, 0));
    if (norm > 0) {
      for (let i = 0; i < this.dim; i++) {
        vector[i] = vector[i]! / norm;
      }
    }

    return vector;
  }

  /** 批量向量化 */
  embedBatch(texts: string[]): number[][] {
    return texts.map((t) => this.embed(t));
  }
}

// ===== 长期记忆实现 =====

/**
 * 长期记忆存储 — 持久化 + 向量索引。
 *
 * 实现 Memory 接口，可直接替换 InMemoryStore。
 */
export class LongTermMemory implements Memory {
  private store: InMemoryStore;
  private vectorIndex: VectorStore;
  private embedding: HashEmbedding;
  private config: Required<LongTermMemoryConfig>;
  private dirty: boolean = false;
  private persistTimer?: ReturnType<typeof setInterval>;
  private idToVectorId: Map<string, string> = new Map();

  constructor(config?: LongTermMemoryConfig) {
    this.config = {
      persistPath: config?.persistPath ?? '',
      vectorDimensions: config?.vectorDimensions ?? 128,
      autoPersist: config?.autoPersist ?? true,
      persistInterval: config?.persistInterval ?? 5000,
      importanceDecay: config?.importanceDecay ?? 0.95,
      hnswM: config?.hnswM ?? 16,
      hnswEfConstruction: config?.hnswEfConstruction ?? 200,
      hnswEfSearch: config?.hnswEfSearch ?? 50,
    };

    this.store = new InMemoryStore();
    this.embedding = new HashEmbedding(this.config.vectorDimensions);
    this.vectorIndex = new VectorStore(this.config.vectorDimensions, {
      M: this.config.hnswM,
      efConstruction: this.config.hnswEfConstruction,
      efSearch: this.config.hnswEfSearch,
    });

    // 从磁盘加载（异步，不阻塞构造函数）
    if (this.config.persistPath) {
      void this.load();
      if (this.config.autoPersist) {
        this.startAutoPersist();
      }
    }
  }

  // ===== Memory 接口实现 =====

  async add(episode: MemoryEpisode): Promise<void> {
    await this.store.add(episode);

    // 添加到向量索引
    const vector = this.embedding.embed(
      `${episode.content} ${episode.summary ?? ''} ${episode.topics ?? ''}`,
    );
    const vectorId = `vec-${episode.id}`;
    this.vectorIndex.add(vectorId, vector, {
      episodeId: episode.id,
      sessionId: episode.sessionId,
      role: episode.role,
    });
    this.idToVectorId.set(episode.id, vectorId);

    this.dirty = true;
  }

  async search(query: string, opts?: SearchOptions): Promise<MemoryEpisode[]> {
    return this.store.search(query, opts);
  }

  async get(id: string): Promise<MemoryEpisode | null> {
    return this.store.get(id);
  }

  async delete(id: string): Promise<void> {
    await this.store.delete(id);
    const vectorId = this.idToVectorId.get(id);
    if (vectorId) {
      this.vectorIndex.delete(vectorId);
      this.idToVectorId.delete(id);
    }
    this.dirty = true;
  }

  async count(sessionId: string): Promise<number> {
    return this.store.count(sessionId);
  }

  async list(opts?: ListOptions): Promise<MemoryEpisode[]> {
    return this.store.list(opts);
  }

  async updateSummary(id: string, summary: string, topics: string): Promise<void> {
    await this.store.updateSummary(id, summary, topics);

    // 更新向量索引
    const episode = await this.store.get(id);
    if (episode) {
      const vectorId = this.idToVectorId.get(id);
      if (vectorId) {
        this.vectorIndex.delete(vectorId);
      }
      const vector = this.embedding.embed(
        `${episode.content} ${summary} ${topics}`,
      );
      const newVectorId = `vec-${id}`;
      this.vectorIndex.add(newVectorId, vector, {
        episodeId: id,
        sessionId: episode.sessionId,
        role: episode.role,
      });
      this.idToVectorId.set(id, newVectorId);
    }

    this.dirty = true;
  }

  async setImportance(id: string, importance: number): Promise<void> {
    await this.store.setImportance(id, importance);
    this.dirty = true;
  }

  async searchByTag(tag: string, opts?: SearchOptions): Promise<MemoryEpisode[]> {
    return this.store.searchByTag(tag, opts);
  }

  async getImportant(threshold: number, limit: number): Promise<MemoryEpisode[]> {
    return this.store.getImportant(threshold, limit);
  }

  async getTimeline(days: number): Promise<Record<string, MemoryEpisode[]>> {
    return this.store.getTimeline(days);
  }

  async cleanupExpired(maxAgeDays: number): Promise<number> {
    const deleted = await this.store.cleanupExpired(maxAgeDays);
    if (deleted > 0) {
      // 重建向量索引（确保异步完成）
      await this.rebuildVectorIndex();
    }
    this.dirty = true;
    return deleted;
  }

  async stats(): Promise<MemoryStats> {
    return this.store.stats();
  }

  close(): void {
    this.stopAutoPersist();
    if (this.config.autoPersist && this.config.persistPath) {
      // 同步写入最终状态
      try {
        const episodes = this.store.list({ limit: 100000 });
        episodes.then((eps) => {
          const data = JSON.stringify({ version: 1, episodes: eps });
          fs.writeFileSync(this.config.persistPath!, data, 'utf-8');
        });
      } catch {
        // 忽略关闭时持久化失败
      }
    }
    this.store.close();
  }

  // ===== 语义搜索 =====

  /** 语义搜索 — 基于向量相似度 */
  semanticSearch(query: string, limit: number = 10): VectorSearchResult[] {
    const queryVector = this.embedding.embed(query);
    return this.vectorIndex.search(queryVector, limit);
  }

  /**
   * 混合搜索 — 全文 + 语义 + 时间衰减 + 重要性。
   *
   * 综合得分 = lexicalScore * 0.3 + semanticScore * 0.4 + recencyScore * 0.2 + importanceScore * 0.1
   */
  async hybridSearch(
    query: string,
    limit: number = 10,
    opts?: { sessionId?: string; daysDecay?: number },
  ): Promise<HybridSearchResult[]> {
    const daysDecay = opts?.daysDecay ?? 30;

    // 1. 全文搜索
    const lexicalResults = await this.store.search(query, {
      sessionId: opts?.sessionId,
      limit: limit * 3, // 多取一些用于合并
    });
    const lexicalMap = new Map<string, number>();
    for (let i = 0; i < lexicalResults.length; i++) {
      // 简单得分：按排序位置递减
      lexicalMap.set(lexicalResults[i]!.id, 1 - i / lexicalResults.length);
    }

    // 2. 语义搜索
    const semanticResults = this.semanticSearch(query, limit * 3);
    const semanticMap = new Map<string, number>();
    for (const result of semanticResults) {
      const episodeId = result.metadata?.episodeId;
      if (episodeId) {
        semanticMap.set(episodeId, result.score);
      }
    }

    // 3. 合并所有候选 ID
    const allIds = new Set([...lexicalMap.keys(), ...semanticMap.keys()]);
    const now = Date.now();

    // 4. 计算综合得分
    const results: HybridSearchResult[] = [];
    for (const id of allIds) {
      const episode = await this.store.get(id);
      if (!episode) continue;

      const lexicalScore = lexicalMap.get(id) ?? 0;
      const semanticScore = semanticMap.get(id) ?? 0;

      // 时间衰减：距离今天越近得分越高
      const ageDays = (now - new Date(episode.createdAt).getTime()) / 86400000;
      const recencyScore = Math.exp(-ageDays / daysDecay);

      // 重要性加成
      const importanceScore = episode.importance ?? 0.5;

      const score =
        lexicalScore * 0.3 +
        semanticScore * 0.4 +
        recencyScore * 0.2 +
        importanceScore * 0.1;

      results.push({
        episode,
        score,
        components: {
          lexical: lexicalScore,
          semantic: semanticScore,
          recency: recencyScore,
          importance: importanceScore,
        },
      });
    }

    // 5. 排序并返回 top-K
    results.sort((a, b) => b.score - a.score);
    return results.slice(0, limit);
  }

  // ===== 重要性衰减 =====

  /** 应用重要性衰减 — 随时间推移降低旧记忆的重要性 */
  async applyImportanceDecay(): Promise<void> {
    const allEpisodes = await this.store.list({ limit: 10000 });
    const now = Date.now();
    const decayFactor = this.config.importanceDecay;

    for (const ep of allEpisodes) {
      if (ep.importance === undefined) continue;
      const ageDays = (now - new Date(ep.createdAt).getTime()) / 86400000;
      const decayedImportance = ep.importance * Math.pow(decayFactor, ageDays);
      if (Math.abs(decayedImportance - ep.importance) > 0.01) {
        await this.store.setImportance(ep.id, decayedImportance);
      }
    }

    this.dirty = true;
  }

  // ===== 持久化 =====

  /** 异步持久化到磁盘 */
  async persist(): Promise<void> {
    if (!this.config.persistPath) return;

    try {
      const episodes = await this.store.list({ limit: 100000 });
      const data = JSON.stringify({ version: 1, episodes });
      const dir = path.dirname(this.config.persistPath);
      if (!fs.existsSync(dir)) {
        fs.mkdirSync(dir, { recursive: true });
      }
      fs.writeFileSync(this.config.persistPath, data, 'utf-8');
      this.dirty = false;
    } catch {
      // 持久化失败不影响运行
    }
  }

  /** 从磁盘加载 */
  private async load(): Promise<void> {
    if (!this.config.persistPath) return;
    try {
      if (!fs.existsSync(this.config.persistPath)) return;
      const data = fs.readFileSync(this.config.persistPath, 'utf-8');
      const parsed = JSON.parse(data) as { version: number; episodes: MemoryEpisode[] };
      if (parsed.version === 1 && Array.isArray(parsed.episodes)) {
        for (const ep of parsed.episodes) {
          await this.store.add(ep);
          // 同步到向量索引
          const vector = this.embedding.embed(
            `${ep.content} ${ep.summary ?? ''} ${ep.topics ?? ''}`,
          );
          const vectorId = `vec-${ep.id}`;
          this.vectorIndex.add(vectorId, vector, {
            episodeId: ep.id,
            sessionId: ep.sessionId,
            role: ep.role,
          });
          this.idToVectorId.set(ep.id, vectorId);
        }
      }
    } catch {
      // 加载失败从空状态开始
    }
  }

  /** 启动自动持久化定时器 */
  private startAutoPersist(): void {
    this.persistTimer = setInterval(() => {
      if (this.dirty) {
        void this.persist();
      }
    }, this.config.persistInterval);
  }

  /** 停止自动持久化 */
  private stopAutoPersist(): void {
    if (this.persistTimer) {
      clearInterval(this.persistTimer);
      this.persistTimer = undefined;
    }
  }

  /** 重建向量索引 — 创建新的 VectorStore 实例以清除旧数据 */
  private async rebuildVectorIndex(): Promise<void> {
    const allEpisodes = await this.store.list({ limit: 100000 });
    this.idToVectorId.clear();

    // 创建新的 VectorStore 实例，丢弃旧索引中已删除的条目
    this.vectorIndex = new VectorStore(this.config.vectorDimensions, {
      M: this.config.hnswM,
      efConstruction: this.config.hnswEfConstruction,
      efSearch: this.config.hnswEfSearch,
    });

    for (const ep of allEpisodes) {
      const vector = this.embedding.embed(
        `${ep.content} ${ep.summary ?? ''} ${ep.topics ?? ''}`,
      );
      const vectorId = `vec-${ep.id}`;
      this.vectorIndex.add(vectorId, vector, {
        episodeId: ep.id,
        sessionId: ep.sessionId,
        role: ep.role,
      });
      this.idToVectorId.set(ep.id, vectorId);
    }
  }
}
