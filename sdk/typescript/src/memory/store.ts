import type { MemoryEpisode, MemoryStats, SearchOptions, ListOptions } from '../types.js';
import { CodeError } from '../errors.js';

/** 记忆存储接口，与 Go 端 Memory 接口对齐。
 *
 * 核心操作：
 * - add: 添加记忆片段
 * - search: 全文搜索（InMemoryStore 走倒排索引，SqliteStore 走 FTS5）
 * - get/delete: 按 ID 获取/删除
 * - list: 按会话/时间分页列表
 * - updateSummary: 更新摘要和标签
 * - setImportance: 设置重要性（0-1）
 * - searchByTag: 按标签搜索
 * - getImportant: 获取高重要性记忆
 * - getTimeline: 按时间线获取记忆
 * - cleanupExpired: 清理过期记忆
 * - stats: 获取统计信息
 * - close: 关闭存储
 */
export interface Memory {
  add(episode: MemoryEpisode): Promise<void>;
  search(query: string, opts?: SearchOptions): Promise<MemoryEpisode[]>;
  get(id: string): Promise<MemoryEpisode | null>;
  delete(id: string): Promise<void>;
  count(sessionId: string): Promise<number>;
  list(opts?: ListOptions): Promise<MemoryEpisode[]>;
  updateSummary(id: string, summary: string, topics: string): Promise<void>;
  setImportance(id: string, importance: number): Promise<void>;
  searchByTag(tag: string, opts?: SearchOptions): Promise<MemoryEpisode[]>;
  getImportant(threshold: number, limit: number): Promise<MemoryEpisode[]>;
  getTimeline(days: number): Promise<Record<string, MemoryEpisode[]>>;
  cleanupExpired(maxAgeDays: number): Promise<number>;
  stats(): Promise<MemoryStats>;
  close(): void;
}

// 分词正则：按非字母数字字符分割，与 Go 端 tokenizeRe 行为一致
const TOKENIZE_RE = /[^\p{L}\p{N}]+/u;

/**
 * 将文本分词为小写 token 集合。
 * 对 content、summary、topics 全部字段进行分词，与 Go 端 indexTokens 行为一致。
 */
function tokenize(...fields: string[]): string[] {
  const combined = fields.join(' ');
  if (!combined.trim()) return [];
  return combined.toLowerCase().split(TOKENIZE_RE).filter((t) => t !== '');
}

/**
 * InMemoryStore 内存版记忆存储。
 *
 * 优化（perf-v2）：新增倒排索引 ftsIndex（token → episode ID 集合），
 * search 走索引而非全表扫描，与 Go 端 InMemoryStore 行为对齐。
 * - 添加/删除/更新时自动维护索引
 * - 单 token 查询走索引，多 token 查询取交集
 */
export class InMemoryStore implements Memory {
  private episodes: Map<string, MemoryEpisode> = new Map();
  /** 倒排索引：小写 token → episode ID 集合 */
  private ftsIndex: Map<string, Set<string>> = new Map();

  // ===== 索引维护 =====

  /** 将 episode 的 content + summary + topics 的所有 token 加入倒排索引 */
  private addToIndex(ep: MemoryEpisode): void {
    for (const tok of tokenize(ep.content, ep.summary ?? '', ep.topics ?? '')) {
      let postings = this.ftsIndex.get(tok);
      if (!postings) {
        postings = new Set();
        this.ftsIndex.set(tok, postings);
      }
      postings.add(ep.id);
    }
  }

  /** 从倒排索引移除 episode 的所有 token */
  private removeFromIndex(ep: MemoryEpisode): void {
    for (const tok of tokenize(ep.content, ep.summary ?? '', ep.topics ?? '')) {
      const postings = this.ftsIndex.get(tok);
      if (!postings) continue;
      postings.delete(ep.id);
      if (postings.size === 0) {
        this.ftsIndex.delete(tok);
      }
    }
  }

  // ===== 公共 API =====

  async add(episode: MemoryEpisode): Promise<void> {
    // 输入校验对齐 Go 端 Episode.Validate()（internal/memory/episode.go），
    // 错误码与 Go pkg/errors.go 保持一致；sessionId/role 由 TS 类型系统强制非空。
    if (!episode.id?.trim()) throw new CodeError('MEM_003', 'Episode ID is required');
    if (!episode.content?.trim()) throw new CodeError('MEM_006', 'Episode content is required');
    if (episode.importance !== undefined && (episode.importance < 0 || episode.importance > 1)) {
      throw new CodeError('MEM_002', 'Importance must be between 0 and 1');
    }
    // 如果已存在，先移除旧索引（使用存储的旧值）
    const existing = this.episodes.get(episode.id);
    if (existing) this.removeFromIndex(existing);
    // 存储快照副本，避免调用方修改已有对象引用导致索引不一致
    const snapshot: MemoryEpisode = { ...episode };
    this.episodes.set(episode.id, snapshot);
    this.addToIndex(snapshot);
  }

  async search(query: string, opts?: SearchOptions): Promise<MemoryEpisode[]> {
    if (!query.trim()) return [];
    const tokens = tokenize(query);
    if (tokens.length === 0) return [];

    // 对每个 token 查找倒排索引，取交集
    let candidateIds: Set<string> | null = null;
    for (const tok of tokens) {
      const postings = this.ftsIndex.get(tok);
      if (!postings || postings.size === 0) {
        return []; // 任一 token 无匹配，整体无结果
      }
      if (candidateIds === null) {
        candidateIds = new Set(postings);
      } else {
        // 取交集
        for (const id of candidateIds) {
          if (!postings.has(id)) candidateIds.delete(id);
        }
      }
      if (candidateIds.size === 0) return [];
    }

    let results: MemoryEpisode[] = [];
    for (const id of candidateIds!) {
      const ep = this.episodes.get(id);
      if (ep) results.push(ep);
    }

    // 应用过滤器
    if (opts?.sessionId) results = results.filter((e) => e.sessionId === opts.sessionId);
    if (opts?.roleFilter) results = results.filter((e) => e.role === opts.roleFilter);

    results.sort((a, b) => b.createdAt.localeCompare(a.createdAt));
    return results.slice(opts?.offset ?? 0, (opts?.offset ?? 0) + (opts?.limit ?? 10));
  }

  async get(id: string): Promise<MemoryEpisode | null> {
    return this.episodes.get(id) ?? null;
  }

  async delete(id: string): Promise<void> {
    const ep = this.episodes.get(id);
    if (ep) this.removeFromIndex(ep);
    this.episodes.delete(id);
  }

  async count(sessionId: string): Promise<number> {
    let count = 0;
    for (const ep of this.episodes.values()) {
      if (ep.sessionId === sessionId) count++;
    }
    return count;
  }

  async list(opts?: ListOptions): Promise<MemoryEpisode[]> {
    let results = Array.from(this.episodes.values());
    if (opts?.sessionId) results = results.filter((e) => e.sessionId === opts.sessionId);
    const order = opts?.ascending ? 1 : -1;
    results.sort((a, b) => order * a.createdAt.localeCompare(b.createdAt));
    return results.slice(opts?.offset ?? 0, (opts?.offset ?? 0) + (opts?.limit ?? 10));
  }

  async updateSummary(id: string, summary: string, topics: string): Promise<void> {
    const ep = this.episodes.get(id);
    if (!ep) throw new CodeError('MEM_001', `Episode ${id} not found`);
    // 移除旧索引
    this.removeFromIndex(ep);
    ep.summary = summary;
    ep.topics = topics;
    // 重新索引
    this.addToIndex(ep);
  }

  async setImportance(id: string, importance: number): Promise<void> {
    // 校验语义对齐 Go 端 ErrInvalidImportance（MEM_002）
    if (importance < 0 || importance > 1) {
      throw new CodeError('MEM_002', 'Importance must be between 0 and 1');
    }
    const ep = this.episodes.get(id);
    if (!ep) throw new CodeError('MEM_001', `Episode ${id} not found`);
    ep.importance = importance;
  }

  async searchByTag(tag: string, opts?: SearchOptions): Promise<MemoryEpisode[]> {
    let results = Array.from(this.episodes.values());
    if (opts?.sessionId) results = results.filter((e) => e.sessionId === opts.sessionId);
    results = results.filter((e) => (e.topics ?? '').includes(tag));
    return results.slice(0, opts?.limit ?? 10);
  }

  async getImportant(threshold: number, limit: number): Promise<MemoryEpisode[]> {
    return Array.from(this.episodes.values())
      .filter((e) => (e.importance ?? 0) >= threshold)
      .sort((a, b) => (b.importance ?? 0) - (a.importance ?? 0))
      .slice(0, limit);
  }

  async getTimeline(days: number): Promise<Record<string, MemoryEpisode[]>> {
    const cutoff = new Date(Date.now() - days * 86400000).toISOString();
    const timeline: Record<string, MemoryEpisode[]> = {};
    for (const ep of this.episodes.values()) {
      if (ep.createdAt >= cutoff) {
        const date = ep.createdAt.slice(0, 10);
        if (!timeline[date]) timeline[date] = [];
        timeline[date].push(ep);
      }
    }
    return timeline;
  }

  async cleanupExpired(maxAgeDays: number): Promise<number> {
    const cutoff = new Date(Date.now() - maxAgeDays * 86400000).toISOString();
    let deleted = 0;
    for (const [id, ep] of this.episodes) {
      if (ep.createdAt < cutoff) {
        this.removeFromIndex(ep);
        this.episodes.delete(id);
        deleted++;
      }
    }
    return deleted;
  }

  async stats(): Promise<MemoryStats> {
    const episodes = Array.from(this.episodes.values());
    const sessions = new Set(episodes.map((e) => e.sessionId));
    let oldest: string | undefined;
    let newest: string | undefined;
    if (episodes.length > 0) {
      oldest = episodes[0].createdAt;
      newest = episodes[0].createdAt;
      for (let i = 1; i < episodes.length; i++) {
        if (episodes[i].createdAt < oldest) oldest = episodes[i].createdAt;
        if (episodes[i].createdAt > newest) newest = episodes[i].createdAt;
      }
    }
    return {
      totalEpisodes: episodes.length,
      totalSessions: sessions.size,
      oldestEpisode: oldest,
      newestEpisode: newest,
      avgEpisodesPerSession: sessions.size > 0 ? episodes.length / sessions.size : 0,
    };
  }

  close(): void {
    this.episodes.clear();
    this.ftsIndex.clear();
  }
}
