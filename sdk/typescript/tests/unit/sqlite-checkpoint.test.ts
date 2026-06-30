import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { createRequire } from 'node:module';

// Check if better-sqlite3 is available (peer dependency)
const require = createRequire(import.meta.url);
let sqliteAvailable = false;
try {
  require('better-sqlite3');
  sqliteAvailable = true;
} catch {
  // better-sqlite3 not installed — skip tests
}

import { SQLiteCheckpointStore } from '../../src/persist/sqlite-checkpoint.js';

// ===== SQLiteCheckpointStore tests =====
// Requires better-sqlite3 peer dependency to be installed

const describeOrSkip = sqliteAvailable ? describe : describe.skip;

describeOrSkip('SQLiteCheckpointStore', () => {
  let store: SQLiteCheckpointStore;

  beforeEach(() => {
    store = SQLiteCheckpointStore.inMemory();
  });

  afterEach(() => {
    if (store) store.close();
  });

  it('should create in-memory store', () => {
    expect(store).toBeDefined();
  });

  it('should save and load checkpoint', async () => {
    const checkpoint = {
      id: 'agent-1',
      sessionID: 'session-1',
      turn: 5,
      messages: [{ role: 'user', content: 'hello' }] as any,
      metrics: { totalTurns: 5, totalTools: 2, duration: '10s' } as any,
      createdAt: '2024-01-01T00:00:00Z',
    };
    await store.save(checkpoint);
    const loaded = await store.load('agent-1');
    expect(loaded).not.toBeNull();
    expect(loaded!.id).toBe('agent-1');
    expect(loaded!.sessionID).toBe('session-1');
    expect(loaded!.turn).toBe(5);
    expect(loaded!.messages).toHaveLength(1);
    expect(loaded!.messages[0].content).toBe('hello');
  });

  it('should return null for non-existent checkpoint', async () => {
    const loaded = await store.load('nonexistent');
    expect(loaded).toBeNull();
  });

  it('should list checkpoints by session', async () => {
    await store.save({
      id: 'agent-1',
      sessionID: 'session-1',
      turn: 1,
      messages: [],
      metrics: { totalTurns: 1, totalTools: 0, duration: '1s' } as any,
      createdAt: '2024-01-01T00:00:00Z',
    });
    await store.save({
      id: 'agent-2',
      sessionID: 'session-1',
      turn: 2,
      messages: [],
      metrics: { totalTurns: 2, totalTools: 1, duration: '2s' } as any,
      createdAt: '2024-01-02T00:00:00Z',
    });
    await store.save({
      id: 'agent-3',
      sessionID: 'session-2',
      turn: 1,
      messages: [],
      metrics: { totalTurns: 1, totalTools: 0, duration: '1s' } as any,
      createdAt: '2024-01-03T00:00:00Z',
    });

    const list = await store.list('session-1');
    expect(list).toHaveLength(2);
    expect(list.every(c => c.sessionID === 'session-1')).toBe(true);
  });

  it('should return empty list for session with no checkpoints', async () => {
    const list = await store.list('empty-session');
    expect(list).toHaveLength(0);
  });

  it('should delete checkpoint', async () => {
    await store.save({
      id: 'agent-1',
      sessionID: 'session-1',
      turn: 1,
      messages: [],
      metrics: { totalTurns: 1, totalTools: 0, duration: '1s' } as any,
      createdAt: '2024-01-01T00:00:00Z',
    });
    await store.delete('agent-1');
    const loaded = await store.load('agent-1');
    expect(loaded).toBeNull();
  });

  it('should throw when deleting non-existent checkpoint', async () => {
    await expect(store.delete('nonexistent')).rejects.toThrow('checkpoint not found');
  });

  it('should overwrite checkpoint on re-save', async () => {
    await store.save({
      id: 'agent-1',
      sessionID: 'session-1',
      turn: 1,
      messages: [{ role: 'user', content: 'first' }] as any,
      metrics: { totalTurns: 1, totalTools: 0, duration: '1s' } as any,
      createdAt: '2024-01-01T00:00:00Z',
    });
    await store.save({
      id: 'agent-1',
      sessionID: 'session-1',
      turn: 2,
      messages: [{ role: 'user', content: 'second' }] as any,
      metrics: { totalTurns: 2, totalTools: 1, duration: '2s' } as any,
      createdAt: '2024-01-02T00:00:00Z',
    });
    const loaded = await store.load('agent-1');
    expect(loaded!.turn).toBe(2);
    expect(loaded!.messages[0].content).toBe('second');
  });

  // ===== AgentState-based API tests =====

  it('should save and load state', async () => {
    const state = {
      agentID: 'agent-1',
      sessionID: 'session-1',
      status: 'running',
      messages: [{ role: 'user', content: 'test' }],
      turnCount: 3,
      metrics: { totalTurns: 3, totalTools: 1, duration: '5s' },
      savedAt: '2024-01-01T00:00:00Z',
    };
    await store.saveState(state);
    const loaded = await store.loadState('agent-1');
    expect(loaded).not.toBeNull();
    expect(loaded!.agentID).toBe('agent-1');
    expect(loaded!.status).toBe('running');
    expect(loaded!.turnCount).toBe(3);
  });

  it('should return null for non-existent state', async () => {
    const loaded = await store.loadState('nonexistent');
    expect(loaded).toBeNull();
  });

  it('should list states by session', async () => {
    await store.saveState({
      agentID: 'a1',
      sessionID: 's1',
      status: 'running',
      messages: [],
      turnCount: 1,
      metrics: { totalTurns: 1, totalTools: 0, duration: '1s' },
      savedAt: '2024-01-01T00:00:00Z',
    });
    await store.saveState({
      agentID: 'a2',
      sessionID: 's1',
      status: 'completed',
      messages: [],
      turnCount: 5,
      metrics: { totalTurns: 5, totalTools: 3, duration: '10s' },
      savedAt: '2024-01-02T00:00:00Z',
    });
    await store.saveState({
      agentID: 'a3',
      sessionID: 's2',
      status: 'running',
      messages: [],
      turnCount: 1,
      metrics: { totalTurns: 1, totalTools: 0, duration: '1s' },
      savedAt: '2024-01-03T00:00:00Z',
    });
    const list = await store.listStates('s1');
    expect(list).toHaveLength(2);
  });

  it('should delete state', async () => {
    await store.saveState({
      agentID: 'a1',
      sessionID: 's1',
      status: 'running',
      messages: [],
      turnCount: 1,
      metrics: { totalTurns: 1, totalTools: 0, duration: '1s' },
      savedAt: '2024-01-01T00:00:00Z',
    });
    await store.deleteState('a1');
    const loaded = await store.loadState('a1');
    expect(loaded).toBeNull();
  });

  it('should throw when deleting non-existent state', async () => {
    await expect(store.deleteState('nonexistent')).rejects.toThrow('checkpoint not found');
  });

  it('should close database', () => {
    const s = SQLiteCheckpointStore.inMemory();
    s.close();
    // After close, operations should throw
    expect(s.load('test')).rejects.toThrow('database is closed');
  });

  it('should handle close when already closed', () => {
    const s = SQLiteCheckpointStore.inMemory();
    s.close();
    s.close(); // Should not throw
  });

  it('should handle empty messages and metrics', async () => {
    await store.save({
      id: 'agent-empty',
      sessionID: 'session-empty',
      turn: 0,
      messages: [] as any,
      metrics: {} as any,
      createdAt: '2024-01-01T00:00:00Z',
    });
    const loaded = await store.load('agent-empty');
    expect(loaded).not.toBeNull();
    expect(loaded!.messages).toHaveLength(0);
  });

  it('should handle complex messages with metadata', async () => {
    const messages = [
      { role: 'user', content: 'hello' },
      { role: 'assistant', content: 'hi there', tool_calls: [{ id: 'call_1', function: { name: 'search', arguments: '{}' } }] },
      { role: 'tool', content: 'result data', tool_call_id: 'call_1' },
    ] as any;
    await store.save({
      id: 'agent-complex',
      sessionID: 'session-complex',
      turn: 3,
      messages,
      metrics: { totalTurns: 3, totalTools: 1, duration: '15s', tokensUsed: 500 } as any,
      createdAt: '2024-01-01T00:00:00Z',
    });
    const loaded = await store.load('agent-complex');
    expect(loaded!.messages).toHaveLength(3);
    expect(loaded!.messages[1].tool_calls).toBeDefined();
  });
});
