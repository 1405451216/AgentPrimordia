const fs = require("fs");
const path = "E:/ap/AgentPrimordia/sdk/typescript/tests/unit/a2a-bus.test.ts";
let content = fs.readFileSync(path, "utf-8");

const oldMetadata = `    it('should include metadata in broadcast messages', async () => {
      let receivedMetadata: Record<string, unknown> | undefined;

      bus.register('receiver', async (msg) => {
        receivedMetadata = msg.metadata;
      });

      await bus.broadcast('receiver', 'content', { key: 'value', count: 42 });

      expect(receivedMetadata).toEqual({ key: 'value', count: 42 });
    });`;

const newMetadata = `    it('should include metadata in broadcast messages', async () => {
      let receivedMetadata: Record<string, unknown> | undefined;

      bus.register('sender', async () => {});
      bus.register('receiver', async (msg) => {
        receivedMetadata = msg.metadata;
      });

      await bus.broadcast('sender', 'content', { key: 'value', count: 42 });

      expect(receivedMetadata).toEqual({ key: 'value', count: 42 });
    });`;

content = content.replace(oldMetadata, newMetadata);

const oldTimeout = `    it('should respect timeout', async () => {
      const results: string[] = [];

      bus.register('fast', async (msg) => {
        results.push(msg.to!);
      });
      bus.register('slow', async () => {
        // 永远不返回，但超时会兜底
        await new Promise(() => {});
      });

      await bus.broadcast('fast', 'test', undefined, 50);

      // 快速 agent 应该收到消息
      expect(results).toContain('fast');
    }, 5000);`;

const newTimeout = `    it('should respect timeout', async () => {
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
    }, 5000);`;

content = content.replace(oldTimeout, newTimeout);

fs.writeFileSync(path, content, "utf-8");
console.log("Fixed. New length:", content.length);
