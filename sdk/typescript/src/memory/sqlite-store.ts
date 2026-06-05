import type { MemoryEpisode, MemoryStats, SearchOptions, ListOptions } from '../types.js';
import type { Memory } from './store.js';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);

interface CheckpointRow {
  id: string;
  session_id: string;
  role: string;
  content: string;
  summary: string | null;
  topics: string | null;
  importance: number | null;
  metadata: string | null;
  created_at: string;
}

export class SqliteStore implements Memory {
  private db: any = null;

  constructor(dbPath: string) {
    try {
      const Database = require('better-sqlite3');
      this.db = new Database(dbPath);
      this.db.pragma('journal_mode = WAL');
      this.db.exec(`
        CREATE TABLE IF NOT EXISTS episodes (
          id TEXT PRIMARY KEY,
          session_id TEXT NOT NULL,
          role TEXT NOT NULL,
          content TEXT NOT NULL,
          summary TEXT,
          topics TEXT,
          importance REAL,
          metadata TEXT,
          created_at TEXT NOT NULL
        );
        CREATE INDEX IF NOT EXISTS idx_episodes_session ON episodes(session_id);
        CREATE INDEX IF NOT EXISTS idx_episodes_created ON episodes(created_at);
      `);
    } catch {
      throw new Error('better-sqlite3 is required for SqliteStore. Install it with: npm install better-sqlite3');
    }
  }

  async add(episode: MemoryEpisode): Promise<void> {
    if (!episode.id?.trim()) throw new Error('Episode ID is required');
    if (!episode.content?.trim()) throw new Error('Episode content is required');
    this.db.prepare(
      'INSERT OR REPLACE INTO episodes (id, session_id, role, content, summary, topics, importance, metadata, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)'
    ).run(
      episode.id,
      episode.sessionId,
      episode.role,
      episode.content,
      episode.summary ?? null,
      episode.topics ?? null,
      episode.importance ?? null,
      episode.metadata ? JSON.stringify(episode.metadata) : null,
      episode.createdAt
    );
  }

  async search(query: string, opts?: SearchOptions): Promise<MemoryEpisode[]> {
    let sql = 'SELECT * FROM episodes WHERE (content LIKE ? OR summary LIKE ? OR topics LIKE ?)';
    const params: unknown[] = [`%${query}%`, `%${query}%`, `%${query}%`];
    if (opts?.sessionId) {
      sql += ' AND session_id = ?';
      params.push(opts.sessionId);
    }
    if (opts?.roleFilter) {
      sql += ' AND role = ?';
      params.push(opts.roleFilter);
    }
    sql += ' ORDER BY created_at DESC LIMIT ? OFFSET ?';
    params.push(opts?.limit ?? 10, opts?.offset ?? 0);
    return this.db.prepare(sql).all(...params).map(rowToEpisode);
  }

  async get(id: string): Promise<MemoryEpisode | null> {
    const row = this.db.prepare('SELECT * FROM episodes WHERE id = ?').get(id);
    return row ? rowToEpisode(row) : null;
  }

  async delete(id: string): Promise<void> {
    this.db.prepare('DELETE FROM episodes WHERE id = ?').run(id);
  }

  async count(sessionId: string): Promise<number> {
    const row = this.db.prepare('SELECT COUNT(*) as cnt FROM episodes WHERE session_id = ?').get(sessionId);
    return row?.cnt ?? 0;
  }

  async list(opts?: ListOptions): Promise<MemoryEpisode[]> {
    let sql = 'SELECT * FROM episodes';
    const params: unknown[] = [];
    if (opts?.sessionId) {
      sql += ' WHERE session_id = ?';
      params.push(opts.sessionId);
    }
    const order = opts?.ascending ? 'ASC' : 'DESC';
    sql += ` ORDER BY created_at ${order} LIMIT ? OFFSET ?`;
    params.push(opts?.limit ?? 10, opts?.offset ?? 0);
    return this.db.prepare(sql).all(...params).map(rowToEpisode);
  }

  async updateSummary(id: string, summary: string, topics: string): Promise<void> {
    const result = this.db.prepare('UPDATE episodes SET summary = ?, topics = ? WHERE id = ?').run(summary, topics, id);
    if (result.changes === 0) throw new Error(`Episode ${id} not found`);
  }

  async setImportance(id: string, importance: number): Promise<void> {
    if (importance < 0 || importance > 1) throw new Error('Importance must be between 0 and 1');
    const result = this.db.prepare('UPDATE episodes SET importance = ? WHERE id = ?').run(importance, id);
    if (result.changes === 0) throw new Error(`Episode ${id} not found`);
  }

  async searchByTag(tag: string, opts?: SearchOptions): Promise<MemoryEpisode[]> {
    let sql = "SELECT * FROM episodes WHERE topics LIKE ?";
    const params: unknown[] = [`%${tag}%`];
    if (opts?.sessionId) {
      sql += ' AND session_id = ?';
      params.push(opts.sessionId);
    }
    sql += ' LIMIT ?';
    params.push(opts?.limit ?? 10);
    return this.db.prepare(sql).all(...params).map(rowToEpisode);
  }

  async getImportant(threshold: number, limit: number): Promise<MemoryEpisode[]> {
    return this.db.prepare('SELECT * FROM episodes WHERE importance >= ? ORDER BY importance DESC LIMIT ?').all(threshold, limit).map(rowToEpisode);
  }

  async getTimeline(days: number): Promise<Record<string, MemoryEpisode[]>> {
    const cutoff = new Date(Date.now() - days * 86400000).toISOString();
    const rows = this.db.prepare('SELECT * FROM episodes WHERE created_at >= ? ORDER BY created_at DESC').all(cutoff);
    const timeline: Record<string, MemoryEpisode[]> = {};
    for (const row of rows) {
      const date = (row as CheckpointRow).created_at.slice(0, 10);
      if (!timeline[date]) timeline[date] = [];
      timeline[date].push(rowToEpisode(row));
    }
    return timeline;
  }

  async cleanupExpired(maxAgeDays: number): Promise<number> {
    const cutoff = new Date(Date.now() - maxAgeDays * 86400000).toISOString();
    const result = this.db.prepare('DELETE FROM episodes WHERE created_at < ?').run(cutoff);
    return result.changes;
  }

  async stats(): Promise<MemoryStats> {
    const total = this.db.prepare('SELECT COUNT(*) as cnt FROM episodes').get()?.cnt ?? 0;
    const sessions = this.db.prepare('SELECT COUNT(DISTINCT session_id) as cnt FROM episodes').get()?.cnt ?? 0;
    const oldest = this.db.prepare('SELECT MIN(created_at) as val FROM episodes').get()?.val;
    const newest = this.db.prepare('SELECT MAX(created_at) as val FROM episodes').get()?.val;
    return {
      totalEpisodes: total,
      totalSessions: sessions,
      oldestEpisode: oldest ?? undefined,
      newestEpisode: newest ?? undefined,
      avgEpisodesPerSession: sessions > 0 ? total / sessions : 0,
    };
  }

  close(): void {
    if (this.db) {
      this.db.close();
      this.db = null;
    }
  }
}

function rowToEpisode(row: CheckpointRow): MemoryEpisode {
  return {
    id: row.id,
    sessionId: row.session_id,
    role: row.role,
    content: row.content,
    summary: row.summary ?? undefined,
    topics: row.topics ?? undefined,
    importance: row.importance ?? undefined,
    metadata: row.metadata ? JSON.parse(row.metadata) : undefined,
    createdAt: row.created_at,
  };
}
