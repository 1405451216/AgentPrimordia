import type { MemoryEpisode, MemoryStats, SearchOptions, ListOptions } from '../types.js';

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

export class InMemoryStore implements Memory {
  private episodes: Map<string, MemoryEpisode> = new Map();

  async add(episode: MemoryEpisode): Promise<void> {
    if (!episode.id?.trim()) throw new Error('Episode ID is required');
    if (!episode.content?.trim()) throw new Error('Episode content is required');
    this.episodes.set(episode.id, episode);
  }

  async search(query: string, opts?: SearchOptions): Promise<MemoryEpisode[]> {
    let results = Array.from(this.episodes.values());
    if (opts?.sessionId) results = results.filter((e) => e.sessionId === opts.sessionId);
    if (opts?.roleFilter) results = results.filter((e) => e.role === opts.roleFilter);
    results = results.filter(
      (e) => e.content.includes(query) || (e.summary ?? '').includes(query) || (e.topics ?? '').includes(query)
    );
    results.sort((a, b) => b.createdAt.localeCompare(a.createdAt));
    return results.slice(opts?.offset ?? 0, (opts?.offset ?? 0) + (opts?.limit ?? 10));
  }

  async get(id: string): Promise<MemoryEpisode | null> {
    return this.episodes.get(id) ?? null;
  }

  async delete(id: string): Promise<void> {
    this.episodes.delete(id);
  }

  async count(sessionId: string): Promise<number> {
    return Array.from(this.episodes.values()).filter((e) => e.sessionId === sessionId).length;
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
    if (!ep) throw new Error(`Episode ${id} not found`);
    ep.summary = summary;
    ep.topics = topics;
  }

  async setImportance(id: string, importance: number): Promise<void> {
    if (importance < 0 || importance > 1) throw new Error('Importance must be between 0 and 1');
    const ep = this.episodes.get(id);
    if (!ep) throw new Error(`Episode ${id} not found`);
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
        this.episodes.delete(id);
        deleted++;
      }
    }
    return deleted;
  }

  async stats(): Promise<MemoryStats> {
    const episodes = Array.from(this.episodes.values());
    const sessions = new Set(episodes.map((e) => e.sessionId));
    return {
      totalEpisodes: episodes.length,
      totalSessions: sessions.size,
      oldestEpisode: episodes.length > 0 ? episodes.reduce((a, b) => (a.createdAt < b.createdAt ? a : b)).createdAt : undefined,
      newestEpisode: episodes.length > 0 ? episodes.reduce((a, b) => (a.createdAt > b.createdAt ? a : b)).createdAt : undefined,
      avgEpisodesPerSession: sessions.size > 0 ? episodes.length / sessions.size : 0,
    };
  }

  close(): void {
    this.episodes.clear();
  }
}
