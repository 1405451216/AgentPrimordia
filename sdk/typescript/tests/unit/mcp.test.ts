import { describe, it, expect, beforeEach } from 'vitest';
import { MCPRegistry } from '../../src/mcp/registry-adapter.js';
import type { MCPClient } from '../../src/mcp/types.js';
import type { MCPToolDefinition, MCPToolResult, MCPServerConfig, MCPResource } from '../../src/mcp/types.js';

// ===== Mock MCP Client =====
class MockMCPClient implements MCPClient {
  private connected = false;
  private tools: MCPToolDefinition[];
  private resources: MCPResource[] = [];
  private callResults: Map<string, MCPToolResult> = new Map();

  constructor(tools: MCPToolDefinition[] = []) {
    this.tools = tools;
  }

  async connect(): Promise<void> { this.connected = true; }
  async disconnect(): Promise<void> { this.connected = false; }
  isConnected(): boolean { return this.connected; }
  listTools(): MCPToolDefinition[] { return this.tools; }
  async listResources(): Promise<MCPResource[]> { return this.resources; }
  async callTool(call: { name: string; arguments: Record<string, unknown> }): Promise<MCPToolResult> {
    const result = this.callResults.get(call.name);
    if (!result) {
      return { content: [{ type: 'text', text: 'no result' }] };
    }
    return result;
  }

  // Test helpers
  setCallResult(toolName: string, result: MCPToolResult): void {
    this.callResults.set(toolName, result);
  }
}

describe('MCPRegistry', () => {
  let registry: MCPRegistry;

  beforeEach(() => {
    registry = new MCPRegistry();
  });

  describe('register', () => {
    it('registers a server with tools', async () => {
      const tools: MCPToolDefinition[] = [
        { name: 'search', description: 'Search tool', inputSchema: {} },
        { name: 'fetch', description: 'Fetch tool', inputSchema: {} },
      ];
      const client = new MockMCPClient(tools);
      const config: MCPServerConfig = { name: 'test-server', version: '1.0', transport: 'http', url: 'http://localhost' };

      await registry.register('test', config, client);

      expect(registry.listServers()).toContain('test');
      expect(registry.getAllTools().length).toBe(2);
    });

    it('connects client if not connected', async () => {
      const client = new MockMCPClient([]);
      expect(client.isConnected()).toBe(false);

      const config: MCPServerConfig = { name: 'srv', version: '1.0', transport: 'http', url: 'http://localhost' };
      await registry.register('srv', config, client);

      expect(client.isConnected()).toBe(true);
    });

    it('does not reconnect if already connected', async () => {
      const client = new MockMCPClient([]);
      await client.connect();

      const config: MCPServerConfig = { name: 'srv', version: '1.0', transport: 'http', url: 'http://localhost' };
      await registry.register('srv', config, client);

      expect(client.isConnected()).toBe(true);
    });
  });

  describe('unregister', () => {
    it('removes a server and its tools', async () => {
      const tools: MCPToolDefinition[] = [
        { name: 'tool1', description: 'T1', inputSchema: {} },
      ];
      const client = new MockMCPClient(tools);
      const config: MCPServerConfig = { name: 'srv', version: '1.0', transport: 'http', url: 'http://localhost' };

      await registry.register('srv', config, client);
      expect(registry.getAllTools().length).toBe(1);

      await registry.unregister('srv');
      expect(registry.listServers()).not.toContain('srv');
      expect(registry.getAllTools().length).toBe(0);
    });

    it('does nothing for unknown server', async () => {
      await registry.unregister('unknown');
      // Should not throw
    });
  });

  describe('unregisterAll', () => {
    it('removes all servers', async () => {
      const config: MCPServerConfig = { name: 'srv', version: '1.0', transport: 'http', url: 'http://localhost' };
      await registry.register('s1', config, new MockMCPClient([{ name: 't1', description: 'T1', inputSchema: {} }]));
      await registry.register('s2', config, new MockMCPClient([{ name: 't2', description: 'T2', inputSchema: {} }]));

      await registry.unregisterAll();
      expect(registry.listServers().length).toBe(0);
    });
  });

  describe('getServer', () => {
    it('returns registered server', async () => {
      const client = new MockMCPClient([]);
      const config: MCPServerConfig = { name: 'srv', version: '1.0', transport: 'http', url: 'http://localhost' };
      await registry.register('srv', config, client);

      const server = registry.getServer('srv');
      expect(server).toBeDefined();
      expect(server!.name).toBe('srv');
    });

    it('returns undefined for unknown server', () => {
      expect(registry.getServer('unknown')).toBeUndefined();
    });
  });

  describe('getTool', () => {
    it('finds a tool by name', async () => {
      const tools: MCPToolDefinition[] = [
        { name: 'search', description: 'Search', inputSchema: {} },
      ];
      const config: MCPServerConfig = { name: 'srv', version: '1.0', transport: 'http', url: 'http://localhost' };
      await registry.register('srv', config, new MockMCPClient(tools));

      const entry = registry.getTool('search');
      expect(entry).toBeDefined();
      expect(entry!.serverName).toBe('srv');
      expect(entry!.tool.name).toBe('search');
    });

    it('returns undefined for unknown tool', () => {
      expect(registry.getTool('unknown')).toBeUndefined();
    });
  });

  describe('callTool', () => {
    it('calls a tool and returns text result', async () => {
      const client = new MockMCPClient([{ name: 'echo', description: 'Echo', inputSchema: {} }]);
      client.setCallResult('echo', {
        content: [{ type: 'text', text: 'hello world' }],
      });
      const config: MCPServerConfig = { name: 'srv', version: '1.0', transport: 'http', url: 'http://localhost' };
      await registry.register('srv', config, client);

      const result = await registry.callTool('echo', { msg: 'test' });
      expect(result).toBe('hello world');
    });

    it('throws for unknown tool', async () => {
      await expect(registry.callTool('unknown', {})).rejects.toThrow();
    });

    it('throws when tool result is an error', async () => {
      const client = new MockMCPClient([{ name: 'fail', description: 'Fail', inputSchema: {} }]);
      client.setCallResult('fail', {
        content: [{ type: 'text', text: 'execution failed' }],
        isError: true,
      });
      const config: MCPServerConfig = { name: 'srv', version: '1.0', transport: 'http', url: 'http://localhost' };
      await registry.register('srv', config, client);

      await expect(registry.callTool('fail', {})).rejects.toThrow('execution failed');
    });

    it('joins multiple text content blocks', async () => {
      const client = new MockMCPClient([{ name: 'multi', description: 'Multi', inputSchema: {} }]);
      client.setCallResult('multi', {
        content: [
          { type: 'text', text: 'line1' },
          { type: 'text', text: 'line2' },
        ],
      });
      const config: MCPServerConfig = { name: 'srv', version: '1.0', transport: 'http', url: 'http://localhost' };
      await registry.register('srv', config, client);

      const result = await registry.callTool('multi', {});
      expect(result).toBe('line1\nline2');
    });
  });
});
