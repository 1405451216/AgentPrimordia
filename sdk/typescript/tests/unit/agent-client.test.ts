/**
 * AgentCRDTClient 测试
 *
 * 测试 Agent CRDT 客户端的编辑、同步、冲突解决等功能
 */

import { describe, it, expect } from 'vitest';
import {
  AgentCRDTClient,
  ConflictResolver,
  type ConflictResolution,
} from '../../src/collaboration/agent-client.js';
import { CRDTDocumentImpl, LamportClock } from '../../src/collaboration/crdt.js';

// 辅助：创建一个空文档
function createDoc() {
  return new CRDTDocumentImpl<Record<string, unknown>>('test-doc', {});
}

describe('AgentCRDTClient', () => {
  describe('constructor', () => {
    it('should create client with default config', () => {
      const client = new AgentCRDTClient({
        clientID: 'agent-1',
        document: createDoc(),
      });

      expect(client).toBeDefined();
      expect(client.getConnectionState()).toBe('disconnected');
    });

    it('should create client with custom config', () => {
      const client = new AgentCRDTClient({
        clientID: 'agent-1',
        document: createDoc(),
        enableOperationLog: false,
        operationBufferSize: 5,
        reconnectInterval: 1000,
      });

      expect(client).toBeDefined();
    });
  });

  describe('connect / disconnect (offline mode)', () => {
    it('should connect in offline mode without syncEndpoint', async () => {
      const client = new AgentCRDTClient({
        clientID: 'agent-1',
        document: createDoc(),
      });

      await client.connect();

      // 离线模式直接标记为 connected
      expect(client.getConnectionState()).toBe('connected');

      client.disconnect();
      expect(client.getConnectionState()).toBe('disconnected');
    });
  });

  describe('edit operations', () => {
    it('should perform set edit', () => {
      const client = new AgentCRDTClient<Record<string, unknown>>({
        clientID: 'agent-1',
        document: createDoc(),
      });

      const edit = client.edit('name', 'Agent Generated Name');

      expect(edit.type).toBe('set');
      expect(edit.path).toBe('name');
      expect(edit.value).toBe('Agent Generated Name');
      expect(edit.source).toBe('agent');
    });

    it('should perform insert edit', () => {
      const client = new AgentCRDTClient<Record<string, unknown>>({
        clientID: 'agent-1',
        document: createDoc(),
      });

      const edit = client.insert('items', { id: 1, text: 'new item' });

      expect(edit.type).toBe('insert');
      expect(edit.source).toBe('agent');
    });

    it('should perform delete edit', () => {
      const client = new AgentCRDTClient<Record<string, unknown>>({
        clientID: 'agent-1',
        document: createDoc(),
      });

      // 先 set 再 delete
      client.edit('field', 'value');
      const edit = client.delete('field');

      expect(edit.type).toBe('delete');
      expect(edit.path).toBe('field');
      expect(edit.source).toBe('agent');
    });
  });

  describe('state management', () => {
    it('should get state after edits', () => {
      const client = new AgentCRDTClient<Record<string, unknown>>({
        clientID: 'agent-1',
        document: createDoc(),
      });

      client.edit('key1', 'value1');
      client.edit('key2', 'value2');

      const state = client.getState();
      expect(state.key1).toBe('value1');
      expect(state.key2).toBe('value2');
    });

    it('should get value by path', () => {
      const client = new AgentCRDTClient<Record<string, unknown>>({
        clientID: 'agent-1',
        document: createDoc(),
      });

      client.edit('name', 'TestValue');
      const value = client.get<string>('name');
      expect(value).toBe('TestValue');
    });

    it('should return undefined for nonexistent path', () => {
      const client = new AgentCRDTClient<Record<string, unknown>>({
        clientID: 'agent-1',
        document: createDoc(),
      });

      const value = client.get<string>('nonexistent');
      expect(value).toBeUndefined();
    });
  });

  describe('operation log', () => {
    it('should record operations when enabled', () => {
      const client = new AgentCRDTClient<Record<string, unknown>>({
        clientID: 'agent-1',
        document: createDoc(),
        enableOperationLog: true,
      });

      client.edit('a', 1);
      client.edit('b', 2);

      const log = client.getOperationLog();
      expect(log.length).toBeGreaterThanOrEqual(2);
    });

    it('should not record operations when disabled', () => {
      const client = new AgentCRDTClient<Record<string, unknown>>({
        clientID: 'agent-1',
        document: createDoc(),
        enableOperationLog: false,
      });

      client.edit('a', 1);

      const log = client.getOperationLog();
      expect(log.length).toBe(0);
    });
  });

  describe('clock management', () => {
    it('should have a Lamport clock', () => {
      const client = new AgentCRDTClient<Record<string, unknown>>({
        clientID: 'agent-1',
        document: createDoc(),
      });

      const clock = client.getClock();
      expect(typeof clock).toBe('number');
      expect(clock).toBeGreaterThanOrEqual(0);
    });
  });

  describe('event listeners', () => {
    it('should notify edit listeners', () => {
      const client = new AgentCRDTClient<Record<string, unknown>>({
        clientID: 'agent-1',
        document: createDoc(),
      });

      let receivedEdit: any = null;
      client.onEdit((edit) => {
        receivedEdit = edit;
      });

      client.edit('test', 'value');

      expect(receivedEdit).not.toBeNull();
      expect(receivedEdit.path).toBe('test');
    });

    it('should allow unsubscribing from edit events', () => {
      const client = new AgentCRDTClient<Record<string, unknown>>({
        clientID: 'agent-1',
        document: createDoc(),
      });

      let callCount = 0;
      const unsubscribe = client.onEdit(() => {
        callCount++;
      });

      client.edit('a', 1);
      unsubscribe();
      client.edit('b', 2);

      expect(callCount).toBe(1);
    });

    it('should notify state change listeners', async () => {
      const client = new AgentCRDTClient<Record<string, unknown>>({
        clientID: 'agent-1',
        document: createDoc(),
      });

      const states: string[] = [];
      client.onStateChange((state) => {
        states.push(state);
      });

      await client.connect();
      client.disconnect();

      expect(states).toContain('connected');
      expect(states).toContain('disconnected');
    });
  });

  describe('applyRemoteOperation', () => {
    it('should apply remote operations from human', () => {
      const client = new AgentCRDTClient<Record<string, unknown>>({
        clientID: 'agent-1',
        document: createDoc(),
      });

      // 创建一个远程操作
      const doc = new CRDTDocumentImpl<Record<string, unknown>>('human-1', {});
      const op = doc.set('remote_key', 'remote_value');

      client.applyRemoteOperation(op);

      const value = client.get<string>('remote_key');
      expect(value).toBe('remote_value');
    });

    it('should apply batch remote operations', () => {
      const client = new AgentCRDTClient<Record<string, unknown>>({
        clientID: 'agent-1',
        document: createDoc(),
      });

      const doc = new CRDTDocumentImpl<Record<string, unknown>>('human-1', {});
      const op1 = doc.set('key1', 'val1');
      const op2 = doc.set('key2', 'val2');

      client.applyRemoteOperations([op1, op2]);

      expect(client.get('key1')).toBe('val1');
      expect(client.get('key2')).toBe('val2');
    });
  });
});

describe('ConflictResolver', () => {
  describe('agent_wins strategy', () => {
    it('should always return agent operation', () => {
      const resolver = new ConflictResolver('agent_wins');

      const agentOp = { type: 'set' as const, path: 'a', value: 'agent', clock: 1, clientID: 'agent-1' };
      const humanOp = { type: 'set' as const, path: 'a', value: 'human', clock: 2, clientID: 'human-1' };

      const result = resolver.resolve(agentOp, humanOp);
      expect(result.value).toBe('agent');
    });
  });

  describe('human_wins strategy', () => {
    it('should always return human operation', () => {
      const resolver = new ConflictResolver('human_wins');

      const agentOp = { type: 'set' as const, path: 'a', value: 'agent', clock: 5, clientID: 'agent-1' };
      const humanOp = { type: 'set' as const, path: 'a', value: 'human', clock: 1, clientID: 'human-1' };

      const result = resolver.resolve(agentOp, humanOp);
      expect(result.value).toBe('human');
    });
  });

  describe('latest strategy (default)', () => {
    it('should return operation with higher clock', () => {
      const resolver = new ConflictResolver('latest');

      const agentOp = { type: 'set' as const, path: 'a', value: 'agent', clock: 10, clientID: 'agent-1' };
      const humanOp = { type: 'set' as const, path: 'a', value: 'human', clock: 5, clientID: 'human-1' };

      const result = resolver.resolve(agentOp, humanOp);
      expect(result.value).toBe('agent');
    });

    it('should break ties by clientID when clocks are equal', () => {
      const resolver = new ConflictResolver('latest');

      const agentOp = { type: 'set' as const, path: 'a', value: 'agent', clock: 5, clientID: 'zzz' };
      const humanOp = { type: 'set' as const, path: 'a', value: 'human', clock: 5, clientID: 'aaa' };

      const result = resolver.resolve(agentOp, humanOp);
      // clientID 大的胜出
      expect(result.value).toBe('agent');
    });
  });

  describe('merge strategy', () => {
    it('should not conflict when paths differ', () => {
      const resolver = new ConflictResolver('merge');

      const agentOp = { type: 'set' as const, path: 'a', value: 'agent', clock: 1, clientID: 'agent-1' };
      const humanOp = { type: 'set' as const, path: 'b', value: 'human', clock: 1, clientID: 'human-1' };

      const result = resolver.resolve(agentOp, humanOp);
      // 路径不同不冲突，返回 agent 操作
      expect(result.path).toBe('a');
    });

    it('should resolve by clock when paths are the same', () => {
      const resolver = new ConflictResolver('merge');

      const agentOp = { type: 'set' as const, path: 'x', value: 'agent', clock: 3, clientID: 'agent-1' };
      const humanOp = { type: 'set' as const, path: 'x', value: 'human', clock: 7, clientID: 'human-1' };

      const result = resolver.resolve(agentOp, humanOp);
      expect(result.value).toBe('human');
    });
  });

  describe('resolveBatch', () => {
    it('should resolve multiple conflicts in batch', () => {
      const resolver = new ConflictResolver('latest');

      const agentOps = [
        { type: 'set' as const, path: 'a', value: 'a1', clock: 1, clientID: 'agent' },
        { type: 'set' as const, path: 'b', value: 'b1', clock: 5, clientID: 'agent' },
      ];
      const humanOps = [
        { type: 'set' as const, path: 'a', value: 'a2', clock: 2, clientID: 'human' },
        { type: 'set' as const, path: 'b', value: 'b2', clock: 3, clientID: 'human' },
      ];

      const results = resolver.resolveBatch(agentOps, humanOps);

      // 'a' conflict: human clock=2 > agent clock=1 → human wins
      // 'b' conflict: agent clock=5 > human clock=3 → agent wins
      const aResult = results.find(r => r.path === 'a');
      const bResult = results.find(r => r.path === 'b');

      expect(aResult?.value).toBe('a2');
      expect(bResult?.value).toBe('b1');
    });

    it('should include non-conflicting operations', () => {
      const resolver = new ConflictResolver('agent_wins');

      const agentOps = [
        { type: 'set' as const, path: 'only-agent', value: 'val', clock: 1, clientID: 'agent' },
      ];
      const humanOps = [
        { type: 'set' as const, path: 'only-human', value: 'val', clock: 1, clientID: 'human' },
      ];

      const results = resolver.resolveBatch(agentOps, humanOps);

      // 两个不冲突的操作都应该在结果中
      const paths = results.map(r => r.path);
      expect(paths).toContain('only-agent');
      expect(paths).toContain('only-human');
    });
  });
});
