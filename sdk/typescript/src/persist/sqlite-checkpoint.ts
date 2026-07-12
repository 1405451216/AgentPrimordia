// SQLite Checkpoint Store — Persistent agent state checkpoints
// Mirrors Go internal/persist/sqlite_checkpoint.go

import { createRequire } from 'node:module';
import type { Checkpoint, CheckpointStore } from '../agent/request-id.js';
import type { Message, AgentMetrics } from '../types.js';

const require = createRequire(import.meta.url);

/** better-sqlite3 Database 最小类型接口，避免 any 类型 */
interface SqliteStatement {
  run(...params: unknown[]): { changes: number };
  get(...params: unknown[]): unknown;
  all(...params: unknown[]): unknown[];
}

interface SqliteDatabase {
  exec(sql: string): void;
  pragma(pragma: string): void;
  prepare(sql: string): SqliteStatement;
  close(): void;
}

// ===== Agent State (matches Go AgentState) =====

export interface AgentState {
  agentID: string;
  sessionID: string;
  status: string;
  messages: Array<{ role: string; content: string }>;
  turnCount: number;
  metrics: {
    totalTurns: number;
    totalTools: number;
    duration: string;
  };
  savedAt: string;
}

// ===== SQLite Checkpoint Store =====

interface CheckpointRow {
  agent_id: string;
  session_id: string;
  status: string;
  messages: string;
  turn_count: number;
  metrics: string;
  saved_at: string;
}

export class SQLiteCheckpointStore implements CheckpointStore {
  private db: SqliteDatabase | null = null;

  constructor(dbPath: string) {
    try {
      const Database = require('better-sqlite3');
      this.db = new Database(dbPath);
      this.db!.pragma('journal_mode = WAL');
      this.db!.exec(`
        CREATE TABLE IF NOT EXISTS checkpoints (
          agent_id TEXT PRIMARY KEY,
          session_id TEXT NOT NULL,
          status TEXT NOT NULL,
          messages TEXT NOT NULL,
          turn_count INTEGER NOT NULL,
          metrics TEXT NOT NULL,
          saved_at TEXT NOT NULL
        );
        CREATE INDEX IF NOT EXISTS idx_checkpoints_session ON checkpoints(session_id);
      `);
    } catch {
      throw new Error(
        'better-sqlite3 is required for SQLiteCheckpointStore. Install it with: npm install better-sqlite3'
      );
    }
  }

  /** 获取数据库实例（确保非 null） */
  private getDb(): SqliteDatabase {
    if (!this.db) throw new Error('database is closed');
    return this.db;
  }

  /** Create an in-memory checkpoint store (for testing). */
  static inMemory(): SQLiteCheckpointStore {
    return new SQLiteCheckpointStore(':memory:');
  }

  // ===== CheckpointStore interface (Checkpoint-based) =====

  async save(checkpoint: Checkpoint): Promise<void> {
    const messages = JSON.stringify(checkpoint.messages);
    const metrics = JSON.stringify(checkpoint.metrics);
    this.getDb().prepare(
      `INSERT OR REPLACE INTO checkpoints (agent_id, session_id, status, messages, turn_count, metrics, saved_at)
       VALUES (?, ?, ?, ?, ?, ?, ?)`
    ).run(
      checkpoint.id,
      checkpoint.sessionID,
      'running',
      messages,
      checkpoint.turn,
      metrics,
      checkpoint.createdAt,
    );
  }

  async load(id: string): Promise<Checkpoint | null> {
    const row = this.getDb().prepare(
      'SELECT * FROM checkpoints WHERE agent_id = ?'
    ).get(id) as CheckpointRow | undefined;

    if (!row) return null;

    try {
      const messages = JSON.parse(row.messages) as Message[];
      const metrics = JSON.parse(row.metrics) as AgentMetrics;
      return {
        id: row.agent_id,
        sessionID: row.session_id,
        turn: row.turn_count,
        messages,
        metrics,
        createdAt: row.saved_at,
      };
    } catch (err) {
      console.error(`Failed to parse checkpoint ${id}:`, err);
      throw new Error(`Invalid checkpoint data for ${id}`);
    }
  }

  async list(sessionID: string): Promise<Checkpoint[]> {
    const rows = this.getDb().prepare(
      'SELECT * FROM checkpoints WHERE session_id = ? ORDER BY saved_at DESC'
    ).all(sessionID) as CheckpointRow[];

    return rows.map((row) => {
      try {
        return {
          id: row.agent_id,
          sessionID: row.session_id,
          turn: row.turn_count,
          messages: JSON.parse(row.messages) as Message[],
          metrics: JSON.parse(row.metrics) as AgentMetrics,
          createdAt: row.saved_at,
        };
      } catch (err) {
        console.error(`Failed to parse checkpoint ${row.agent_id}:`, err);
        return null;
      }
    }).filter((cp): cp is Checkpoint => cp !== null);
  }

  async delete(id: string): Promise<void> {
    const result = this.getDb().prepare(
      'DELETE FROM checkpoints WHERE agent_id = ?'
    ).run(id);
    if (result.changes === 0) {
      throw new Error(`checkpoint not found: ${id}`);
    }
  }

  // ===== AgentState-based API (matches Go directly) =====

  /** Save an AgentState (Go-compatible format). */
  async saveState(state: AgentState): Promise<void> {
    const messages = JSON.stringify(state.messages);
    const metrics = JSON.stringify(state.metrics);
    this.getDb().prepare(
      `INSERT OR REPLACE INTO checkpoints (agent_id, session_id, status, messages, turn_count, metrics, saved_at)
       VALUES (?, ?, ?, ?, ?, ?, ?)`
    ).run(
      state.agentID,
      state.sessionID,
      state.status,
      messages,
      state.turnCount,
      metrics,
      state.savedAt,
    );
  }

  /** Load an AgentState by agent ID. */
  async loadState(agentID: string): Promise<AgentState | null> {
    const row = this.getDb().prepare(
      'SELECT * FROM checkpoints WHERE agent_id = ?'
    ).get(agentID) as CheckpointRow | undefined;

    if (!row) return null;

    try {
      const messages = JSON.parse(row.messages);
      const metrics = JSON.parse(row.metrics);
      return {
        agentID: row.agent_id,
        sessionID: row.session_id,
        status: row.status,
        messages,
        turnCount: row.turn_count,
        metrics,
        savedAt: row.saved_at,
      };
    } catch (err) {
      console.error(`Failed to parse agent state ${agentID}:`, err);
      throw new Error(`Invalid agent state data for ${agentID}`);
    }
  }

  /** List all checkpoints for a session (AgentState format). */
  async listStates(sessionID: string): Promise<AgentState[]> {
    const rows = this.getDb().prepare(
      'SELECT * FROM checkpoints WHERE session_id = ? ORDER BY saved_at DESC'
    ).all(sessionID) as CheckpointRow[];

    return rows.map((row) => {
      try {
        const messages = JSON.parse(row.messages);
        const metrics = JSON.parse(row.metrics);
        return {
          agentID: row.agent_id,
          sessionID: row.session_id,
          status: row.status,
          messages,
          turnCount: row.turn_count,
          metrics,
          savedAt: row.saved_at,
        };
      } catch (err) {
        console.error(`Failed to parse agent state ${row.agent_id}:`, err);
        return null;
      }
    }).filter((state): state is AgentState => state !== null);
  }

  /** Delete a checkpoint by agent ID. */
  async deleteState(agentID: string): Promise<void> {
    const result = this.getDb().prepare(
      'DELETE FROM checkpoints WHERE agent_id = ?'
    ).run(agentID);
    if (result.changes === 0) {
      throw new Error(`checkpoint not found: ${agentID}`);
    }
  }

  /** Close the database connection. */
  close(): void {
    if (this.db) {
      this.db.close();
      this.db = null;
    }
  }
}
