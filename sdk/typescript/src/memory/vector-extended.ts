import type { VectorSearchResult } from '../types.js';

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
      ml: config?.ml ?? 1 / Math.log(16),
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

    if (!this.entryPoint) {
      this.entryPoint = id;
      this.maxLevel = level;
    } else {
      if (level > this.maxLevel) {
        this.maxLevel = level;
        this.entryPoint = id;
      }
    }

    this.nodes.set(id, node);

    // Connect to neighbors (simplified: connect to nearest nodes at each level)
    for (let l = 0; l <= Math.min(level, this.maxLevel); l++) {
      const candidates = Array.from(this.nodes.values())
        .filter(n => n.id !== id && n.level >= l)
        .sort((a, b) => this.distance(vector, a.vector) - this.distance(vector, b.vector))
        .slice(0, this.config.maxConnections);

      for (const candidate of candidates) {
        node.neighbors.get(l)!.push(candidate.id);
        candidate.neighbors.get(l)?.push(id);

        // Trim connections if exceeding max
        const maxConn = l === 0 ? this.config.maxConnectionsLayer0 : this.config.maxConnections;
        const candidateNeighbors = candidate.neighbors.get(l)!;
        if (candidateNeighbors.length > maxConn) {
          candidateNeighbors.sort((a, b) => {
            const distA = this.distance(vector, this.nodes.get(a)!.vector);
            const distB = this.distance(vector, this.nodes.get(b)!.vector);
            return distA - distB;
          });
          candidate.neighbors.set(l, candidateNeighbors.slice(0, maxConn));
        }
      }
    }
  }

  search(query: number[], k: number): VectorSearchResult[] {
    if (!this.entryPoint || this.nodes.size === 0) return [];

    // Start from entry point
    let current = this.entryPoint;
    const visited = new Set<string>();

    // Greedy search from top level to level 1
    for (let l = this.maxLevel; l > 0; l--) {
      let improved = true;
      while (improved) {
        improved = false;
        const node = this.nodes.get(current)!;
        const neighbors = node.neighbors.get(l) ?? [];
        for (const neighborId of neighbors) {
          if (visited.has(neighborId)) continue;
          visited.add(neighborId);
          if (this.distance(query, this.nodes.get(neighborId)!.vector) <
              this.distance(query, node.vector)) {
            current = neighborId;
            improved = true;
          }
        }
      }
    }

    // EF search at level 0
    const ef = Math.max(k, this.config.efSearch);
    const candidates: Array<{ id: string; dist: number }> = [];
    const searchQueue: string[] = [current];
    const searchVisited = new Set<string>([current]);

    while (searchQueue.length > 0 && candidates.length < ef) {
      const nodeId = searchQueue.shift()!;
      const node = this.nodes.get(nodeId);
      if (!node) continue;

      const dist = this.distance(query, node.vector);
      candidates.push({ id: nodeId, dist });

      const neighbors = node.neighbors.get(0) ?? [];
      for (const neighborId of neighbors) {
        if (!searchVisited.has(neighborId)) {
          searchVisited.add(neighborId);
          searchQueue.push(neighborId);
        }
      }
    }

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
    const r = Math.random();
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
