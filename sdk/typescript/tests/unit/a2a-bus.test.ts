import { describe, it, expect, beforeEach } from 'vitest';
import { A2ABus, AgentMessage } from '../../src/a2a/bus.js';

describe('A2ABus', () => {
  let bus: A2ABus;

  beforeEach(() => {
    bus = new A2ABus();
  });

  describe('broadcast', () => {
    it('should broadcast to all agents in parallel', async () => {
      const results: string[] = [];

      // 注册 5 个 agent，每个处理耗时 100ms
      for (let i = 0; i < 5; i++) {
        bus.register(`agent-${i}`, async (msg) => {
          await new Promise((r) => setTimeout(r, 100));
          results.push(msg.to!);
        });
      }

      const startTime = Date.now();
      await bus.broadcast('agent-0', 'hello');
      const elapsed = Date.now() - startTime;

      // 并行执行：5 个 100ms 任务应在 < 250ms 内完成（而非串行的 500ms）
      expect(elapsed).toBeLessThan(250);
      // 4 个 agent 收到了消息（排除发送者自身）
      expect(results).toHaveLength(4);
      expect(results).not.toContain('agent-0');
    });

    it('should not fail if one agent throws', async () => {
      bus.register('good', async () => {});
      bus.register('bad', async () => {
        throw new Error('agent error');
      });

      // 不应抛出异常
      await expect(bus.broadcast('good', 'test')).resolves.toBeUndefined();
    });

    it('should respect timeout', async () => {
      const results: string[] = [];

      bus.register('sender', async () => {});
      bus.register('fast', async (msg) => {
        results.push(msg.to!);
      });
      bus.register('slow', async () => {
        // 永远不返回，但超时会兜底
        await new Promise(() => {});
      });

      await bus.broadcast('sender', 'test', undefined, 50);

      // 快速 agent 应该收到消息
      expect(results).toContain('fast');
    }, 5000);

    it('should include metadata in broadcast messages', async () => {
      let receivedMetadata: Record<string, unknown> | undefined;

      bus.register('sender', async () => {});
      bus.register('receiver', async (msg) => {
        receivedMetadata = msg.metadata;
      });

      await bus.broadcast('sender', 'content', { key: 'value', count: 42 });

      expect(receivedMetadata).toEqual({ key: 'value', count: 42 });
    });

    it('should send unique to field per agent', async () => {
      const recipients: string[] = [];

      bus.register('a', async (msg) => { recipients.push(msg.to!); });
      bus.register('b', async (msg) => { recipients.push(msg.to!); });
      bus.register('c', async (msg) => { recipients.push(msg.to!); });

      await bus.broadcast('a', 'hello');

      expect(recipients).toContain('b');
      expect(recipients).toContain('c');
      expect(recipients).not.toContain('a');
    });

    it('should handle broadcast with no other agents', async () => {
      bus.register('only', async () => {});
      await expect(bus.broadcast('only', 'hello')).resolves.toBeUndefined();
    });

    it('should handle broadcast with many agents efficiently', async () => {
      let count = 0;

      // 注册 20 个 agent，每个处理耗时 50ms
      for (let i = 0; i < 20; i++) {
        bus.register(`agent-${i}`, async () => {
          await new Promise((r) => setTimeout(r, 50));
          count++;
        });
      }

      const startTime = Date.now();
      await bus.broadcast('agent-0', 'hello');
      const elapsed = Date.now() - startTime;

      // 20 个并行 50ms 任务应在 < 200ms 内完成（串行需要 1000ms）
      expect(elapsed).toBeLessThan(200);
      // 所有 19 个其他 agent 都处理了消息
      expect(count).toBe(19);
    });
  });

  describe('register/unregister', () => {
    it('should register and list agents', () => {
      bus.register('a', async () => {});
      bus.register('b', async () => {});

      expect(bus.listAgents()).toEqual(expect.arrayContaining(['a', 'b']));
    });

    it('should unregister agents', () => {
      bus.register('a', async () => {});
      bus.unregister('a');

      expect(bus.listAgents()).toHaveLength(0);
    });
  });

  describe('send', () => {
    it('should send message to specific agent', async () => {
      let received: AgentMessage | undefined;
      bus.register('target', async (msg) => {
        received = msg;
        return { ...msg, type: 'response', content: 'reply' } as AgentMessage;
      });

      const result = await bus.send('sender', 'target', 'hello');

      expect(received).toBeDefined();
      expect(received!.to).toBe('target');
      expect(received!.from).toBe('sender');
    });

    it('should throw for unregistered agent', async () => {
      await expect(bus.send('a', 'nonexistent', 'hello')).rejects.toThrow();
    });
  });
});

