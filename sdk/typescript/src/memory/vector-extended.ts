import type { VectorSearchResult } from '../types.js';

// ===== 二叉堆 =====

/**
 * BinaryHeap 通用二叉堆：push/pop/peek 均为 O(log n)。
 *
 * 比较器语义与 Array.prototype.sort 一致：
 * compare(a, b) < 0 时 a 先出堆（升序比较器 = 最小堆，反向 = 最大堆）。
 * 用于 HNSW searchLayer 的候选集（最小堆）与结果集（最大堆）维护，
 * 替代早期 sort+shift 的 O(n log n) 简化实现。
 */
export class BinaryHeap<T> {
  private items: T[] = [];

  constructor(private readonly compare: (a: T, b: T) => number) {}

  get size(): number {
    return this.items.length;
  }

  isEmpty(): boolean {
    return this.items.length === 0;
  }

  push(item: T): void {
    this.items.push(item);
    this.siftUp(this.items.length - 1);
  }

  /** 弹出堆顶；空堆返回 undefined */
  pop(): T | undefined {
    const n = this.items.length;
    if (n === 0) return undefined;
    const top = this.items[0]!;
    const last = this.items.pop()!;
    if (n > 1) {
      this.items[0] = last;
      this.siftDown(0);
    }
    return top;
  }

  /** 查看堆顶但不弹出；空堆返回 undefined */
  peek(): T | undefined {
    return this.items[0];
  }

  /** 返回堆内全部元素的副本（无顺序保证，调用方按需自行排序） */
  toArray(): T[] {
    return [...this.items];
  }

  private siftUp(start: number): void {
    let i = start;
    const item = this.items[i]!;
    while (i > 0) {
      const parent = (i - 1) >> 1;
      const parentItem = this.items[parent]!;
      if (this.compare(item, parentItem) < 0) {
        this.items[i] = parentItem;
        i = parent;
      } else {
        break;
      }
    }
    this.items[i] = item;
  }

  private siftDown(start: number): void {
    const n = this.items.length;
    let i = start;
    const item = this.items[i]!;
    for (;;) {
      const left = i * 2 + 1;
      const right = left + 1;
      let best = i;
      let bestItem = item;
      if (left < n && this.compare(this.items[left]!, bestItem) < 0) {
        best = left;
        bestItem = this.items[left]!;
      }
      if (right < n && this.compare(this.items[right]!, bestItem) < 0) {
        best = right;
        bestItem = this.items[right]!;
      }
      if (best === i) break;
      this.items[i] = bestItem;
      i = best;
    }
    this.items[i] = item;
  }
}

// ===== HNSW (Hierarchical Navigable Small World) — Simplified implementation =====

interface HNSWNode {
  id: string;
  vector: number[];
  level: number;
  neighbors: Map<number, string[]>; // level → neighbor ids
}

export interface HNSWConfig {
  maxConnections: number;
  maxConnectionsLayer0: number;
  efConstruction: number;
  efSearch: number;
  ml: number; // level generation factor
  /**
   * 随机源（用于层级生成），默认 Math.random。
   * 可注入固定种子 PRNG 以获得确定性的图构建——召回量化门（v5.1）依赖可复现构建。
   */
  random?: () => number;
}

export class HNSW {
  private config: HNSWConfig;
  private nodes: Map<string, HNSWNode> = new Map();
  private entryPoint: string | null = null;
  private maxLevel: number = 0;

  constructor(config?: Partial<HNSWConfig>) {
    this.config = {
      maxConnections: config?.maxConnections ?? 16,
      maxConnectionsLayer0: config?.maxConnectionsLayer0 ?? 32,
      efConstruction: config?.efConstruction ?? 200,
      efSearch: config?.efSearch ?? 50,
      // ml 默认取 1.0 而非论文的 1/ln(M)：后者上层节点仅占 1/M（M=16 时 6.25%），
      // 在聚类数据上 layer-0 桥接边不足导致图碎片化（recall@10 卡在 ≈0.62，
      // 提高 efSearch 也无法恢复——不可达簇与搜索努力无关）。
      // ml=1.0 上层占 e^-1≈37%，实测 recall@10 ≥0.99，且更贴近 Go 侧
      // internal/memory/hnsw.go randomLevel 的实际拓扑（全部节点 ≥layer 1）。
      ml: config?.ml ?? 1.0,
      random: config?.random ?? Math.random,
    };
  }

  insert(id: string, vector: number[]): void {
    const level = this.randomLevel();
    const node: HNSWNode = {
      id,
      vector,
      level,
      neighbors: new Map(),
    };

    for (let l = 0; l <= level; l++) {
      node.neighbors.set(l, []);
    }

    // 第一个节点：直接作为 entry point
    if (!this.entryPoint) {
      this.entryPoint = id;
      this.maxLevel = level;
      this.nodes.set(id, node);
      return;
    }

    // Phase 1：从顶层向 node.level+1 层做贪心搜索（greedy descend）
    // 目的：在 node.level 之上的层只追踪最近节点，O(log n)
    let currentEntry = this.entryPoint;
    for (let l = this.maxLevel; l > level; l--) {
      currentEntry = this.greedyDescend(vector, currentEntry, l);
    }

    // Phase 2：在 node.level 之下的每一层做 ef-search，建立双向连接
    // 每层 O(efConstruction * log n)，总复杂度 O(log n)
    const insertLevels: number[] = [];
    for (let l = Math.min(level, this.maxLevel); l >= 0; l--) {
      insertLevels.push(l);
    }

    let entryAtLevel = currentEntry;
    for (const l of insertLevels) {
      const candidates = this.searchLayer(vector, entryAtLevel, this.config.efConstruction, l);
      // 选择 M 个邻居；layer 0 用更宽松的 M0 上限（对齐 Go hnsw.go Insert：
      // lev==0 时 maxConn = MaxConnections*2，M0>M 显著提升召回）。
      // 用 Algorithm 4 多样性启发式而非简单截断：保留簇间桥接边。
      const selectCount = l === 0 ? this.config.maxConnectionsLayer0 : this.config.maxConnections;
      const selectedIds = this.selectNeighborsHeuristic(
        vector,
        candidates.map(c => c.id),
        selectCount,
      );
      node.neighbors.set(l, selectedIds);

      // 反向：每个 selected 节点也加上 node，超限用启发式裁剪
      for (const nid of selectedIds) {
        const peer = this.nodes.get(nid)!;
        const peerNeighbors = peer.neighbors.get(l) ?? [];
        peerNeighbors.push(id);
        // 裁剪到 maxConnections（layer 0 用更宽松的上限）
        const maxConn = l === 0 ? this.config.maxConnectionsLayer0 : this.config.maxConnections;
        if (peerNeighbors.length > maxConn) {
          // 注意：新节点 id 此时尚未写入 this.nodes，需通过 resolve 闭包提供其向量
          peer.neighbors.set(
            l,
            this.selectNeighborsHeuristic(peer.vector, peerNeighbors, maxConn, (x) =>
              x === id ? vector : this.nodes.get(x)!.vector,
            ),
          );
        }
      }

      // 进入下一层时，把入口收敛到当前层最近的节点
      if (selectedIds.length > 0) {
        entryAtLevel = selectedIds[0]!;
      }
    }

    // 更新 entry point：如果新节点层级更高
    if (level > this.maxLevel) {
      this.maxLevel = level;
      this.entryPoint = id;
    }

    this.nodes.set(id, node);
  }

  /**
   * selectNeighborsHeuristic — 论文 Algorithm 4（Malkov & Yashunin 2016）的
   * select-neighbors-heuristic（extendCandidates=false，keepPrunedConnections=true）：
   * 候选按到 query 距离升序遍历；若候选到某已选点的距离小于其到 query 的距离，
   * 视为被该已选点"覆盖"而跳过（多样性保持）；不足 M 时用被跳过的最近点补齐。
   *
   * 相比简单距离截断，启发式保留方向多样的连接（尤其是簇间桥接边），
   * 避免聚类数据上图碎片化——这是 v5.1 召回门（recall@10 ≥0.95）的关键之一。
   *
   * @param resolve 可选的 id→向量解析器；插入期新节点尚未入表时由调用方提供其向量
   */
  private selectNeighborsHeuristic(
    queryVector: number[],
    candidateIds: string[],
    m: number,
    resolve?: (id: string) => number[],
  ): string[] {
    const vecOf = resolve ?? ((id: string) => this.nodes.get(id)!.vector);
    const cand = candidateIds
      .map((id) => ({ id, dQ: this.distance(queryVector, vecOf(id)) }))
      .sort((a, b) => a.dQ - b.dQ);

    const selected: Array<{ id: string; vec: number[] }> = [];
    const pruned: Array<{ id: string }> = [];

    for (const c of cand) {
      if (selected.length >= m) break;
      const cv = vecOf(c.id);
      let dominated = false;
      for (const s of selected) {
        if (this.distance(cv, s.vec) < c.dQ) {
          dominated = true;
          break;
        }
      }
      if (!dominated) selected.push({ id: c.id, vec: cv });
      else pruned.push({ id: c.id });
    }
    // keepPrunedConnections：用被裁剪的最近点补齐至 M
    for (const p of pruned) {
      if (selected.length >= m) break;
      selected.push({ id: p.id, vec: vecOf(p.id) });
    }
    return selected.map((s) => s.id);
  }

  /**
   * greedyDescend 在单层内做贪心下降：从 entry 开始，沿当前层邻居中最近的节点前进，直到不再改善。
   * 用于在 node.level 之上的"高层导航"，每层只访问 O(1) 个节点。
   */
  private greedyDescend(query: number[], entryPoint: string, level: number): string {
    let current = entryPoint;
    let currentDist = this.distance(query, this.nodes.get(current)!.vector);
    let improved = true;
    while (improved) {
      improved = false;
      const neighbors = this.nodes.get(current)?.neighbors.get(level) ?? [];
      for (const nid of neighbors) {
        const d = this.distance(query, this.nodes.get(nid)!.vector);
        if (d < currentDist) {
          current = nid;
          currentDist = d;
          improved = true;
        }
      }
    }
    return current;
  }

  /**
   * searchLayer 在单层做 ef-搜索：返回距离 query 最近的最多 ef 个候选。
   * 实现细节：candidates 用最小堆按距离取最近（O(log n) push/pop），
   * results 用堆顶为最远项的有界堆在超过 ef 时裁剪。
   * 返回顺序不保证，调用方按需自行排序。
   */
  private searchLayer(query: number[], entryPoint: string, ef: number, level: number): Array<{ id: string; dist: number }> {
    const visited = new Set<string>([entryPoint]);
    const startDist = this.distance(query, this.nodes.get(entryPoint)!.vector);

    // candidates：待扩展候选集，按距离最小堆（替代早期 sort+shift 简化实现）
    const candidates = new BinaryHeap<{ id: string; dist: number }>((a, b) => a.dist - b.dist);
    candidates.push({ id: entryPoint, dist: startDist });
    // results：当前最好的 ef 个，堆顶为最远项，便于超限裁剪
    const results = new BinaryHeap<{ id: string; dist: number }>((a, b) => b.dist - a.dist);
    results.push({ id: entryPoint, dist: startDist });

    while (!candidates.isEmpty()) {
      // 取最近候选
      const current = candidates.pop()!;
      // 如果当前候选比 results 最远还差且 results 已满，则提前退出
      if (current.dist > results.peek()!.dist && results.size >= ef) {
        break;
      }

      // 扩展邻居
      const neighbors = this.nodes.get(current.id)?.neighbors.get(level) ?? [];
      for (const nid of neighbors) {
        if (visited.has(nid)) continue;
        visited.add(nid);
        const d = this.distance(query, this.nodes.get(nid)!.vector);
        if (results.size < ef || d < results.peek()!.dist) {
          candidates.push({ id: nid, dist: d });
          results.push({ id: nid, dist: d });
          // 超过 ef 上限时弹出最远项
          if (results.size > ef) results.pop();
        }
      }
    }
    return results.toArray();
  }

  search(query: number[], k: number): VectorSearchResult[] {
    if (!this.entryPoint || this.nodes.size === 0) return [];

    // Phase 1：从顶层贪心下降到 layer 1（对齐 Go hnsw.go Search 的 greedyClosest 循环）
    let current = this.entryPoint;
    for (let l = this.maxLevel; l > 0; l--) {
      current = this.greedyDescend(query, current, l);
    }

    // Phase 2：layer 0 用堆式 ef-search（最小堆候选 + 最大堆结果 + 最远剪枝），
    // 与 Go internal/memory/hnsw.go searchLayer 的 beam search 语义一致。
    // v5.1 修复：原实现为 BFS + candidates.length < ef 硬上限，按队列顺序
    // 而非最近优先扩展候选，N=1000 时 recall@10 仅 ≈0.5、N=3000 时 ≈0.08。
    const ef = Math.max(k, this.config.efSearch);
    const candidates = this.searchLayer(query, current, ef, 0);

    return candidates
      .sort((a, b) => a.dist - b.dist)
      .slice(0, k)
      .map(c => ({ id: c.id, score: 1 / (1 + c.dist) }));
  }

  remove(id: string): boolean {
    const node = this.nodes.get(id);
    if (!node) return false;

    // Remove from neighbors
    for (const [, neighborIds] of node.neighbors) {
      for (const neighborId of neighborIds) {
        const neighbor = this.nodes.get(neighborId);
        if (neighbor) {
          for (const [, ids] of neighbor.neighbors) {
            const idx = ids.indexOf(id);
            if (idx >= 0) ids.splice(idx, 1);
          }
        }
      }
    }

    this.nodes.delete(id);

    if (this.entryPoint === id) {
      this.entryPoint = this.nodes.size > 0 ? Array.from(this.nodes.keys())[0]! : null;
      this.maxLevel = this.entryPoint ? this.nodes.get(this.entryPoint)!.level : 0;
    }

    return true;
  }

  size(): number { return this.nodes.size; }

  private randomLevel(): number {
    // 构造函数保证 random 必有值（默认 Math.random）
    const r = this.config.random!();
    return Math.floor(-Math.log(r) * this.config.ml);
  }

  private distance(a: number[], b: number[]): number {
    let sum = 0;
    const len = Math.min(a.length, b.length);
    for (let i = 0; i < len; i++) {
      const diff = a[i]! - b[i]!;
      sum += diff * diff;
    }
    return Math.sqrt(sum);
  }
}

// ===== Milvus Provider (HTTP API) =====

export interface MilvusConfig {
  endpoint: string;
  collection: string;
  apiKey?: string;
  dimension: number;
}

export class MilvusProvider {
  private config: MilvusConfig;

  constructor(config: MilvusConfig) {
    this.config = config;
  }

  async insert(vectors: Array<{ id: string; vector: number[]; metadata?: Record<string, string> }>): Promise<void> {
    await fetch(`${this.config.endpoint}/v2/vectors/insert`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...(this.config.apiKey ? { Authorization: `Bearer ${this.config.apiKey}` } : {}) },
      body: JSON.stringify({
        collectionName: this.config.collection,
        data: vectors.map(v => ({ id: v.id, vector: v.vector, ...v.metadata })),
      }),
    });
  }

  async search(query: number[], k: number, filter?: string): Promise<VectorSearchResult[]> {
    const resp = await fetch(`${this.config.endpoint}/v2/vectors/search`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...(this.config.apiKey ? { Authorization: `Bearer ${this.config.apiKey}` } : {}) },
      body: JSON.stringify({
        collectionName: this.config.collection,
        data: [query],
        limit: k,
        filter: filter ?? '',
      }),
    });
    const data = await resp.json() as { results?: Array<{ id: string; distance: number }> };
    return (data.results ?? []).map(r => ({
      id: String(r.id),
      score: 1 / (1 + Math.abs(r.distance)),
    }));
  }

  async delete(ids: string[]): Promise<void> {
    await fetch(`${this.config.endpoint}/v2/vectors/delete`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...(this.config.apiKey ? { Authorization: `Bearer ${this.config.apiKey}` } : {}) },
      body: JSON.stringify({
        collectionName: this.config.collection,
        id: ids,
      }),
    });
  }
}

// ===== Qdrant Provider (HTTP API) =====

export interface QdrantConfig {
  endpoint: string;
  collection: string;
  apiKey?: string;
  dimension: number;
}

export class QdrantProvider {
  private config: QdrantConfig;

  constructor(config: QdrantConfig) {
    this.config = config;
  }

  async insert(vectors: Array<{ id: string; vector: number[]; metadata?: Record<string, unknown> }>): Promise<void> {
    await fetch(`${this.config.endpoint}/collections/${this.config.collection}/points`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', ...(this.config.apiKey ? { 'api-key': this.config.apiKey } : {}) },
      body: JSON.stringify({
        points: vectors.map(v => ({
          id: v.id,
          vector: v.vector,
          payload: v.metadata ?? {},
        })),
      }),
    });
  }

  async search(query: number[], k: number, filter?: Record<string, unknown>): Promise<VectorSearchResult[]> {
    const resp = await fetch(`${this.config.endpoint}/collections/${this.config.collection}/points/search`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...(this.config.apiKey ? { 'api-key': this.config.apiKey } : {}) },
      body: JSON.stringify({
        vector: query,
        limit: k,
        filter: filter,
      }),
    });
    const data = await resp.json() as { result?: Array<{ id: string; score: number }> };
    return (data.result ?? []).map(r => ({
      id: String(r.id),
      score: r.score,
    }));
  }

  async delete(ids: string[]): Promise<void> {
    await fetch(`${this.config.endpoint}/collections/${this.config.collection}/points/delete`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...(this.config.apiKey ? { 'api-key': this.config.apiKey } : {}) },
      body: JSON.stringify({ points: ids }),
    });
  }
}

// ===== Conversational Memory =====

import type { Memory } from '../memory/store.js';
import type { MemoryEpisode } from '../types.js';

export interface ConversationalMemoryConfig {
  maxTurns: number;
  summaryThreshold: number;
  autoSummarize: boolean;
}

export class ConversationalMemory implements Memory {
  private config: ConversationalMemoryConfig;
  private episodes: Map<string, MemoryEpisode> = new Map();
  private sessionOrder: Map<string, string[]> = new Map();

  constructor(config?: Partial<ConversationalMemoryConfig>) {
    this.config = {
      maxTurns: config?.maxTurns ?? 20,
      summaryThreshold: config?.summaryThreshold ?? 15,
      autoSummarize: config?.autoSummarize ?? true,
    };
  }

  async add(episode: MemoryEpisode): Promise<void> {
    this.episodes.set(episode.id, episode);
    if (!this.sessionOrder.has(episode.sessionId)) {
      this.sessionOrder.set(episode.sessionId, []);
    }
    this.sessionOrder.get(episode.sessionId)!.push(episode.id);

    // Auto-summarize if too many messages
    if (this.config.autoSummarize) {
      const sessionEpisodes = this.sessionOrder.get(episode.sessionId)!;
      if (sessionEpisodes.length > this.config.summaryThreshold) {
        await this.summarizeOldMessages(episode.sessionId);
      }
    }
  }

  async search(query: string, opts?: { sessionId?: string; limit?: number }): Promise<MemoryEpisode[]> {
    let results = Array.from(this.episodes.values());
    if (opts?.sessionId) results = results.filter(e => e.sessionId === opts.sessionId);
    results = results.filter(e =>
      e.content.includes(query) ||
      (e.summary ?? '').includes(query) ||
      (e.topics ?? '').includes(query)
    );
    return results.slice(0, opts?.limit ?? 10);
  }

  async get(id: string): Promise<MemoryEpisode | null> {
    return this.episodes.get(id) ?? null;
  }

  async delete(id: string): Promise<void> {
    const ep = this.episodes.get(id);
    if (ep) {
      const order = this.sessionOrder.get(ep.sessionId);
      if (order) {
        const idx = order.indexOf(id);
        if (idx >= 0) order.splice(idx, 1);
      }
    }
    this.episodes.delete(id);
  }

  async count(sessionId: string): Promise<number> {
    return this.sessionOrder.get(sessionId)?.length ?? 0;
  }

  async list(opts?: { sessionId?: string; limit?: number; offset?: number }): Promise<MemoryEpisode[]> {
    let results = Array.from(this.episodes.values());
    if (opts?.sessionId) results = results.filter(e => e.sessionId === opts.sessionId);
    results.sort((a, b) => b.createdAt.localeCompare(a.createdAt));
    return results.slice(opts?.offset ?? 0, (opts?.offset ?? 0) + (opts?.limit ?? 10));
  }

  async updateSummary(id: string, summary: string, topics: string): Promise<void> {
    const ep = this.episodes.get(id);
    if (ep) {
      ep.summary = summary;
      ep.topics = topics;
    }
  }

  async setImportance(id: string, importance: number): Promise<void> {
    const ep = this.episodes.get(id);
    if (ep) ep.importance = importance;
  }

  async searchByTag(tag: string, opts?: { sessionId?: string; limit?: number }): Promise<MemoryEpisode[]> {
    let results = Array.from(this.episodes.values());
    if (opts?.sessionId) results = results.filter(e => e.sessionId === opts.sessionId);
    results = results.filter(e => (e.topics ?? '').includes(tag));
    return results.slice(0, opts?.limit ?? 10);
  }

  async getImportant(threshold: number, limit: number): Promise<MemoryEpisode[]> {
    return Array.from(this.episodes.values())
      .filter(e => (e.importance ?? 0) >= threshold)
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
        this.episodes.delete(id);
        const order = this.sessionOrder.get(ep.sessionId);
        if (order) {
          const idx = order.indexOf(id);
          if (idx >= 0) order.splice(idx, 1);
        }
        deleted++;
      }
    }
    return deleted;
  }

  async stats(): Promise<{ totalEpisodes: number; totalSessions: number; oldestEpisode?: string; newestEpisode?: string; avgEpisodesPerSession: number }> {
    const episodes = Array.from(this.episodes.values());
    const sessions = new Set(episodes.map(e => e.sessionId));
    return {
      totalEpisodes: episodes.length,
      totalSessions: sessions.size,
      oldestEpisode: episodes.length > 0 ? episodes.reduce((a, b) => a.createdAt < b.createdAt ? a : b).createdAt : undefined,
      newestEpisode: episodes.length > 0 ? episodes.reduce((a, b) => a.createdAt > b.createdAt ? a : b).createdAt : undefined,
      avgEpisodesPerSession: sessions.size > 0 ? episodes.length / sessions.size : 0,
    };
  }

  close(): void {
    this.episodes.clear();
    this.sessionOrder.clear();
  }

  /** Get conversation history as messages. */
  getHistory(sessionId: string, maxMessages?: number): MemoryEpisode[] {
    const ids = this.sessionOrder.get(sessionId) ?? [];
    const limit = maxMessages ?? this.config.maxTurns;
    return ids.slice(-limit).map(id => this.episodes.get(id)!).filter(Boolean);
  }

  private async summarizeOldMessages(sessionId: string): Promise<void> {
    const ids = this.sessionOrder.get(sessionId)!;
    const toSummarize = ids.slice(0, -this.config.maxTurns);

    if (toSummarize.length === 0) return;

    const messages = toSummarize.map(id => this.episodes.get(id)).filter(Boolean) as MemoryEpisode[];
    const summary = `[Summarized ${messages.length} messages]\n` +
      messages.map(m => `${m.role}: ${m.content.slice(0, 100)}...`).join('\n');

    // Replace old messages with a summary
    const summaryEpisode: MemoryEpisode = {
      id: `summary-${sessionId}-${Date.now()}`,
      sessionId,
      role: 'system',
      content: summary,
      summary,
      createdAt: new Date().toISOString(),
    };

    for (const id of toSummarize) {
      this.episodes.delete(id);
    }

    this.episodes.set(summaryEpisode.id, summaryEpisode);
    this.sessionOrder.set(sessionId, [summaryEpisode.id, ...ids.slice(-this.config.maxTurns)]);
  }
}

// ===== Shared Store — cross-agent shared memory =====

export interface SharedStoreEntry {
  key: string;
  value: unknown;
  owner?: string;
  ttl?: number;
  createdAt: number;
}

export class SharedStore {
  private data: Map<string, SharedStoreEntry> = new Map();
  private watchers: Map<string, Array<(entry: SharedStoreEntry | null) => void>> = new Map();

  set(key: string, value: unknown, opts?: { owner?: string; ttlMs?: number }): void {
    const entry: SharedStoreEntry = {
      key,
      value,
      owner: opts?.owner,
      ttl: opts?.ttlMs,
      createdAt: Date.now(),
    };

    this.data.set(key, entry);

    if (opts?.ttlMs) {
      setTimeout(() => {
        if (this.data.get(key) === entry) {
          this.data.delete(key);
          this.notify(key, null);
        }
      }, opts.ttlMs);
    }

    this.notify(key, entry);
  }

  get(key: string): SharedStoreEntry | undefined {
    return this.data.get(key);
  }

  delete(key: string): boolean {
    const deleted = this.data.delete(key);
    if (deleted) this.notify(key, null);
    return deleted;
  }

  has(key: string): boolean {
    return this.data.has(key);
  }

  keys(): string[] {
    return Array.from(this.data.keys());
  }

  watch(key: string, callback: (entry: SharedStoreEntry | null) => void): () => void {
    if (!this.watchers.has(key)) this.watchers.set(key, []);
    this.watchers.get(key)!.push(callback);

    return () => {
      const watchers = this.watchers.get(key);
      if (watchers) {
        const idx = watchers.indexOf(callback);
        if (idx >= 0) watchers.splice(idx, 1);
      }
    };
  }

  /** Acquire a named lock (returns release function). */
  async lock(key: string, timeoutMs: number = 5000): Promise<(() => void) | null> {
    const lockKey = `__lock__:${key}`;
    const deadline = Date.now() + timeoutMs;

    while (Date.now() < deadline) {
      if (!this.data.has(lockKey)) {
        this.data.set(lockKey, {
          key: lockKey,
          value: process.pid,
          owner: 'lock',
          createdAt: Date.now(),
        });
        return () => { this.data.delete(lockKey); };
      }
      await new Promise(r => setTimeout(r, 50));
    }
    return null;
  }

  private notify(key: string, entry: SharedStoreEntry | null): void {
    const watchers = this.watchers.get(key);
    if (watchers) {
      for (const cb of watchers) cb(entry);
    }
  }
}
