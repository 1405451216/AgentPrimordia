import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import * as http from 'node:http';
import * as net from 'node:net';
import {
  A2ABus,
  type AgentMessage,
  type MessageHandler,
} from '../../src/a2a/bus.js';
import {
  HTTPTransport,
  TCPTransport,
  AgentDiscovery,
  A2AAuth,
  A2AAgentServer,
  A2AClient,
  type A2AMessage,
  type A2AAgentInfo,
  type HTTPTransportConfig,
} from '../../src/a2a/transport.js';
import type { ReActAgent } from '../../src/agent/react-loop.js';
import type { Provider } from '../../src/llm/provider.js';
import type { Response } from '../../src/types.js';

// ===== A2A Bus Tests =====

describe('A2ABus', () => {
  it('should register and list agents', () => {
    const bus = new A2ABus();
    const handler: MessageHandler = async () => {};
    bus.register('agent-1', handler);
    bus.register('agent-2', handler);
    expect(bus.listAgents()).toEqual(['agent-1', 'agent-2']);
  });

  it('should unregister agents', () => {
    const bus = new A2ABus();
    const handler: MessageHandler = async () => {};
    bus.register('agent-1', handler);
    bus.register('agent-2', handler);
    bus.unregister('agent-1');
    expect(bus.listAgents()).toEqual(['agent-2']);
  });

  it('should send messages between agents', async () => {
    const bus = new A2ABus();
    let received: AgentMessage | null = null;
    bus.register('receiver', async (msg) => {
      received = msg;
      return { ...msg, type: 'response', content: 'reply' };
    });
    const result = await bus.send('sender', 'receiver', 'hello', { key: 'val' });
    expect(received).not.toBeNull();
    expect(received!.from).toBe('sender');
    expect(received!.to).toBe('receiver');
    expect(received!.content).toBe('hello');
    expect(received!.metadata).toEqual({ key: 'val' });
    expect(result).toBeDefined();
    expect((result as AgentMessage).content).toBe('reply');
  });

  it('should throw when sending to unregistered agent', async () => {
    const bus = new A2ABus();
    await expect(bus.send('sender', 'unknown', 'hello')).rejects.toThrow(
      'Agent unknown not registered'
    );
  });

  it('should broadcast to all agents except sender', async () => {
    const bus = new A2ABus();
    const received: string[] = [];
    bus.register('a', async (msg) => {
      received.push(msg.content);
    });
    bus.register('b', async (msg) => {
      received.push(msg.content);
    });
    bus.register('c', async (msg) => {
      received.push(msg.content);
    });
    await bus.broadcast('a', 'announcement');
    expect(received).toHaveLength(2);
    expect(received).toContain('announcement');
    expect(received).not.toContain(undefined);
  });

  it('should swallow errors during broadcast', async () => {
    const bus = new A2ABus();
    bus.register('err-agent', async () => {
      throw new Error('boom');
    });
    bus.register('ok-agent', async (msg) => {
      return;
    });
    await expect(bus.broadcast('sender', 'hello')).resolves.not.toThrow();
  });

  it('should generate unique message IDs', async () => {
    const bus = new A2ABus();
    const ids: string[] = [];
    bus.register('r', async (msg) => {
      ids.push(msg.id);
      return;
    });
    await bus.send('s', 'r', 'm1');
    await bus.send('s', 'r', 'm2');
    await bus.send('s', 'r', 'm3');
    expect(ids).toEqual(['msg-1', 'msg-2', 'msg-3']);
  });
});

// ===== A2A Auth Tests =====

describe('A2AAuth', () => {
  it('should generate bearer auth headers', () => {
    const auth = new A2AAuth({ type: 'bearer', token: 'my-token' });
    const headers = auth.getHeaders();
    expect(headers.Authorization).toBe('Bearer my-token');
  });

  it('should generate api_key auth headers', () => {
    const auth = new A2AAuth({ type: 'api_key', apiKey: 'key123' });
    const headers = auth.getHeaders();
    expect(headers['X-API-Key']).toBe('key123');
  });

  it('should generate basic auth headers', () => {
    const auth = new A2AAuth({ type: 'basic', username: 'user', password: 'pass' });
    const headers = auth.getHeaders();
    const expected = Buffer.from('user:pass').toString('base64');
    expect(headers.Authorization).toBe(`Basic ${expected}`);
  });

  it('should return empty headers for unknown type', () => {
    const auth = new A2AAuth({ type: 'bearer' as any });
    // bypassing type check for default case
    const config = auth as unknown as { config: { type: string } };
    config.config.type = 'unknown';
    expect(auth.getHeaders()).toEqual({});
  });

  it('should verify bearer auth', () => {
    const auth = new A2AAuth({ type: 'bearer', token: 'my-token' });
    expect(auth.verify({ authorization: 'Bearer my-token' })).toBe(true);
    expect(auth.verify({ authorization: 'Bearer wrong' })).toBe(false);
  });

  it('should verify api_key auth', () => {
    const auth = new A2AAuth({ type: 'api_key', apiKey: 'key123' });
    expect(auth.verify({ 'x-api-key': 'key123' })).toBe(true);
    expect(auth.verify({ 'x-api-key': 'wrong' })).toBe(false);
  });

  it('should verify basic auth', () => {
    const auth = new A2AAuth({ type: 'basic', username: 'user', password: 'pass' });
    const cred = Buffer.from('user:pass').toString('base64');
    expect(auth.verify({ authorization: `Basic ${cred}` })).toBe(true);
    expect(auth.verify({ authorization: 'Basic wrong' })).toBe(false);
  });

  it('should return true for unknown auth type verification', () => {
    const auth = new A2AAuth({ type: 'bearer' as any });
    const config = auth as unknown as { config: { type: string } };
    config.config.type = 'unknown';
    expect(auth.verify({})).toBe(true);
  });
});

// ===== HTTP Transport Tests =====

describe('HTTPTransport', () => {
  let transport: HTTPTransport;
  let port: number;

  afterEach(async () => {
    if (transport) await transport.stop();
  });

  it('should start and stop the server', async () => {
    transport = new HTTPTransport({ port: 0, host: '127.0.0.1' });
    await transport.start();
    // Server is running
    const endpoint = transport.getEndpoint();
    expect(endpoint).toContain('127.0.0.1');
    await transport.stop();
    transport = null as any;
  });

  it('should handle POST requests with auth', async () => {
    transport = new HTTPTransport({ port: 0, host: '127.0.0.1', authToken: 'secret' });
    await transport.start();
    const addr = (transport as any).server.address() as net.AddressInfo;
    port = addr.port;

    transport.onMessage(async (msg) => {
      return {
        id: 'resp-1',
        from: 'server',
        to: msg.from,
        type: 'response' as const,
        content: 'processed',
        timestamp: new Date().toISOString(),
      };
    });

    // Valid auth
    const resp = await fetch(`http://127.0.0.1:${port}/`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: 'Bearer secret',
      },
      body: JSON.stringify({
        id: 'msg-1',
        from: 'client',
        to: 'server',
        type: 'request',
        content: 'hello',
        timestamp: new Date().toISOString(),
      }),
    });
    expect(resp.status).toBe(200);
    const data = await resp.json();
    expect(data.content).toBe('processed');
  });

  it('should reject requests with invalid auth', async () => {
    transport = new HTTPTransport({ port: 0, host: '127.0.0.1', authToken: 'secret' });
    await transport.start();
    const addr = (transport as any).server.address() as net.AddressInfo;
    port = addr.port;

    const resp = await fetch(`http://127.0.0.1:${port}/`, {
      method: 'POST',
      headers: { Authorization: 'Bearer wrong' },
      body: '{}',
    });
    expect(resp.status).toBe(401);
  });

  it('should reject non-POST methods', async () => {
    transport = new HTTPTransport({ port: 0, host: '127.0.0.1' });
    await transport.start();
    const addr = (transport as any).server.address() as net.AddressInfo;
    port = addr.port;

    const resp = await fetch(`http://127.0.0.1:${port}/`, { method: 'GET' });
    expect(resp.status).toBe(405);
  });

  it('should return 503 when no handler registered', async () => {
    transport = new HTTPTransport({ port: 0, host: '127.0.0.1' });
    await transport.start();
    const addr = (transport as any).server.address() as net.AddressInfo;
    port = addr.port;

    const resp = await fetch(`http://127.0.0.1:${port}/`, {
      method: 'POST',
      body: JSON.stringify({
        id: '1', from: 'a', to: 'b', type: 'request', content: 'x', timestamp: '',
      }),
    });
    expect(resp.status).toBe(503);
  });

  it('should return 400 on invalid JSON', async () => {
    transport = new HTTPTransport({ port: 0, host: '127.0.0.1' });
    transport.onMessage(async (msg) => msg);
    await transport.start();
    const addr = (transport as any).server.address() as net.AddressInfo;
    port = addr.port;

    const resp = await fetch(`http://127.0.0.1:${port}/`, {
      method: 'POST',
      body: 'not-json',
    });
    expect(resp.status).toBe(400);
  });

  it('should register and unregister agents', () => {
    transport = new HTTPTransport({ port: 0 });
    transport.registerAgent('agent-1', 'http://localhost:8001');
    transport.registerAgent('agent-2', 'http://localhost:8002');
    transport.unregisterAgent('agent-1');
    // No direct accessor, test via send which should throw
    expect(transport.send('agent-1', {} as A2AMessage)).rejects.toThrow('Unknown agent: agent-1');
  });

  it('should send to registered agents via fetch', async () => {
    // Create a mock HTTP server as the target
    const targetServer = http.createServer((req, res) => {
      let body = '';
      req.on('data', (c) => (body += c));
      req.on('end', () => {
        const msg = JSON.parse(body);
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ ...msg, type: 'response', content: 'ok' }));
      });
    });

    await new Promise<void>((resolve) => targetServer.listen(0, '127.0.0.1', resolve));
    const targetPort = (targetServer.address() as net.AddressInfo).port;

    transport = new HTTPTransport({ port: 0, host: '127.0.0.1' });
    await transport.start();
    transport.registerAgent('target', `http://127.0.0.1:${targetPort}/`);

    const msg: A2AMessage = {
      id: 'msg-1',
      from: 'sender',
      to: 'target',
      type: 'request',
      content: 'hello',
      timestamp: new Date().toISOString(),
    };
    const resp = await transport.send('target', msg);
    expect(resp.content).toBe('ok');
    expect(resp.type).toBe('response');

    await new Promise<void>((resolve) => targetServer.close(() => resolve()));
  });

  it('should throw when sending to unknown agent', async () => {
    transport = new HTTPTransport({ port: 0 });
    await expect(
      transport.send('unknown', {} as A2AMessage)
    ).rejects.toThrow('Unknown agent: unknown');
  });

  it('should stop gracefully without server', async () => {
    transport = new HTTPTransport({ port: 0 });
    // Don't start, just stop
    await expect(transport.stop()).resolves.not.toThrow();
    transport = null as any;
  });
});

// ===== TCP Transport Tests =====

describe('TCPTransport', () => {
  let transport: TCPTransport;

  afterEach(async () => {
    if (transport) await transport.stop();
  });

  it('should start and stop the server', async () => {
    transport = new TCPTransport({ port: 0, host: '127.0.0.1' });
    await transport.start();
    await transport.stop();
    transport = null as any;
  });

  it('should handle incoming TCP messages', async () => {
    transport = new TCPTransport({ port: 0, host: '127.0.0.1' });
    transport.onMessage(async (msg) => {
      return {
        id: 'resp-1',
        from: 'server',
        to: msg.from,
        type: 'response' as const,
        content: 'processed',
        timestamp: new Date().toISOString(),
      };
    });
    await transport.start();
    const port = ((transport as any).server.address() as net.AddressInfo).port;

    // Connect as a client
    const response = await new Promise<string>((resolve, reject) => {
      const socket = net.createConnection(port, '127.0.0.1', () => {
        socket.write(JSON.stringify({
          id: 'msg-1',
          from: 'client',
          to: 'server',
          type: 'request',
          content: 'hello',
          timestamp: new Date().toISOString(),
        }) + '\n');
      });
      let data = '';
      socket.on('data', (d) => {
        data += d.toString();
        if (data.includes('\n')) {
          resolve(data.trim());
          socket.destroy();
        }
      });
      socket.on('error', reject);
      setTimeout(() => { reject(new Error('timeout')); socket.destroy(); }, 5000);
    });

    const parsed = JSON.parse(response);
    expect(parsed.content).toBe('processed');
    expect(parsed.type).toBe('response');
  });

  it('should handle invalid JSON gracefully', async () => {
    transport = new TCPTransport({ port: 0, host: '127.0.0.1' });
    transport.onMessage(async (msg) => msg);
    await transport.start();
    const port = ((transport as any).server.address() as net.AddressInfo).port;

    const response = await new Promise<string>((resolve, reject) => {
      const socket = net.createConnection(port, '127.0.0.1', () => {
        socket.write('invalid-json\n');
      });
      let data = '';
      socket.on('data', (d) => {
        data += d.toString();
        if (data.includes('\n')) {
          resolve(data.trim());
          socket.destroy();
        }
      });
      socket.on('error', reject);
      setTimeout(() => { reject(new Error('timeout')); socket.destroy(); }, 5000);
    });

    const parsed = JSON.parse(response);
    expect(parsed.type).toBe('error');
    expect(parsed.content).toContain('Error');
  });

  it('should handle empty messages', async () => {
    transport = new TCPTransport({ port: 0, host: '127.0.0.1' });
    transport.onMessage(async (msg) => msg);
    await transport.start();
    const port = ((transport as any).server.address() as net.AddressInfo).port;

    // Send an empty line followed by a valid message
    const response = await new Promise<string>((resolve, reject) => {
      const socket = net.createConnection(port, '127.0.0.1', () => {
        socket.write('\n' + JSON.stringify({
          id: 'msg-1',
          from: 'client',
          to: 'server',
          type: 'request',
          content: 'hello',
          timestamp: new Date().toISOString(),
        }) + '\n');
      });
      let data = '';
      socket.on('data', (d) => {
        data += d.toString();
        if (data.includes('\n')) {
          resolve(data.trim());
          socket.destroy();
        }
      });
      socket.on('error', reject);
      setTimeout(() => { reject(new Error('timeout')); socket.destroy(); }, 5000);
    });

    const parsed = JSON.parse(response);
    expect(parsed.content).toBe('hello');
  });

  it('should stop without server gracefully', async () => {
    transport = new TCPTransport({ port: 0 });
    await expect(transport.stop()).resolves.not.toThrow();
    transport = null as any;
  });

  it('should send via TCP connection', async () => {
    // Create a mock TCP server
    const mockServer = net.createServer((socket) => {
      let buffer = '';
      socket.on('data', (data) => {
        buffer += data.toString();
        if (buffer.includes('\n')) {
          const msg = JSON.parse(buffer.trim());
          socket.write(JSON.stringify({ ...msg, type: 'response', content: 'tcp-reply' }) + '\n');
        }
      });
    });

    await new Promise<void>((resolve) => mockServer.listen(0, '127.0.0.1', resolve));
    const mockPort = (mockServer.address() as net.AddressInfo).port;

    transport = new TCPTransport({ port: 0, host: '127.0.0.1' });

    const msg: A2AMessage = {
      id: 'tcp-1',
      from: 'client',
      to: 'server',
      type: 'request',
      content: 'tcp-hello',
      timestamp: new Date().toISOString(),
    };

    const resp = await transport.send(`localhost:${mockPort}`, msg);
    expect(resp.content).toBe('tcp-reply');
    expect(resp.type).toBe('response');

    await new Promise<void>((resolve) => mockServer.close(() => resolve()));
  });
});

// ===== Agent Discovery Tests =====

describe('AgentDiscovery', () => {
  it('should register and find agents locally', async () => {
    const mockTransport = {
      start: vi.fn().mockResolvedValue(undefined),
      stop: vi.fn().mockResolvedValue(undefined),
      send: vi.fn(),
      onMessage: vi.fn(),
    };

    const discovery = new AgentDiscovery({
      transport: mockTransport as any,
      agentInfo: {
        id: 'agent-1',
        name: 'Test Agent',
        description: 'A test agent',
        capabilities: ['chat', 'search'],
        endpoint: 'http://localhost:8001',
        status: 'online',
      },
    });

    await discovery.register();
    const agent = discovery.findAgent('agent-1');
    expect(agent).toBeDefined();
    expect(agent!.name).toBe('Test Agent');
    expect(agent!.status).toBe('online');
  });

  it('should find agents by capability', async () => {
    const mockTransport = {
      start: vi.fn(),
      stop: vi.fn(),
      send: vi.fn(),
      onMessage: vi.fn(),
    };

    const discovery = new AgentDiscovery({
      transport: mockTransport as any,
      agentInfo: {
        id: 'agent-1',
        name: 'Test Agent',
        capabilities: ['chat', 'search'],
        endpoint: 'http://localhost:8001',
        status: 'online',
      },
    });

    await discovery.register();
    const chatAgents = discovery.findByCapability('chat');
    expect(chatAgents).toHaveLength(1);
    expect(chatAgents[0].id).toBe('agent-1');

    const codingAgents = discovery.findByCapability('coding');
    expect(codingAgents).toHaveLength(0);
  });

  it('should discover agents from local registry only', async () => {
    const mockTransport = {
      start: vi.fn(),
      stop: vi.fn(),
      send: vi.fn(),
      onMessage: vi.fn(),
    };

    const discovery = new AgentDiscovery({
      transport: mockTransport as any,
      agentInfo: {
        id: 'agent-1',
        name: 'Test Agent',
        capabilities: ['chat'],
        endpoint: 'http://localhost:8001',
        status: 'online',
      },
    });

    await discovery.register();
    const agents = await discovery.discover();
    expect(agents).toHaveLength(1);
    expect(agents[0].id).toBe('agent-1');
  });

  it('should deregister agents', async () => {
    const mockTransport = {
      start: vi.fn(),
      stop: vi.fn(),
      send: vi.fn(),
      onMessage: vi.fn(),
    };

    const discovery = new AgentDiscovery({
      transport: mockTransport as any,
      agentInfo: {
        id: 'agent-1',
        name: 'Test Agent',
        capabilities: ['chat'],
        endpoint: 'http://localhost:8001',
        status: 'online',
      },
    });

    await discovery.register();
    expect(discovery.findAgent('agent-1')).toBeDefined();
    await discovery.deregister();
    expect(discovery.findAgent('agent-1')).toBeUndefined();
  });

  it('should register with central registry when registryURL is set', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      json: async () => [],
    } as Response);

    const mockTransport = {
      start: vi.fn(),
      stop: vi.fn(),
      send: vi.fn(),
      onMessage: vi.fn(),
    };

    const discovery = new AgentDiscovery({
      transport: mockTransport as any,
      agentInfo: {
        id: 'agent-1',
        name: 'Test Agent',
        capabilities: ['chat'],
        endpoint: 'http://localhost:8001',
        status: 'online',
      },
      registryURL: 'http://registry.example.com',
    });

    await discovery.register();
    expect(fetchSpy).toHaveBeenCalled();
    fetchSpy.mockRestore();
  });

  it('should discover from central registry when registryURL is set', async () => {
    const remoteAgents: A2AAgentInfo[] = [
      {
        id: 'remote-1',
        name: 'Remote Agent',
        capabilities: ['chat'],
        endpoint: 'http://remote:8001',
        status: 'online',
      },
    ];

    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      json: async () => remoteAgents,
    } as Response);

    const mockTransport = {
      start: vi.fn(),
      stop: vi.fn(),
      send: vi.fn(),
      onMessage: vi.fn(),
    };

    const discovery = new AgentDiscovery({
      transport: mockTransport as any,
      agentInfo: {
        id: 'agent-1',
        name: 'Test Agent',
        capabilities: ['chat'],
        endpoint: 'http://localhost:8001',
        status: 'online',
      },
      registryURL: 'http://registry.example.com',
    });

    await discovery.register();
    const agents = await discovery.discover();
    expect(agents.length).toBeGreaterThanOrEqual(2);
    expect(agents.some((a) => a.id === 'remote-1')).toBe(true);
    fetchSpy.mockRestore();
  });

  it('should handle registry fetch errors gracefully', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('network error'));

    const mockTransport = {
      start: vi.fn(),
      stop: vi.fn(),
      send: vi.fn(),
      onMessage: vi.fn(),
    };

    const discovery = new AgentDiscovery({
      transport: mockTransport as any,
      agentInfo: {
        id: 'agent-1',
        name: 'Test Agent',
        capabilities: ['chat'],
        endpoint: 'http://localhost:8001',
        status: 'online',
      },
      registryURL: 'http://registry.example.com',
    });

    // Should not throw
    await expect(discovery.register()).resolves.not.toThrow();
    await expect(discovery.deregister()).resolves.not.toThrow();
    // After deregister + failed registry fetch, discover should return empty
    const agents = await discovery.discover();
    expect(agents).toHaveLength(0);
    fetchSpy.mockRestore();
  });

  it('should handle registry returning non-ok response', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false,
      json: async () => [],
    } as Response);

    const mockTransport = {
      start: vi.fn(),
      stop: vi.fn(),
      send: vi.fn(),
      onMessage: vi.fn(),
    };

    const discovery = new AgentDiscovery({
      transport: mockTransport as any,
      agentInfo: {
        id: 'agent-1',
        name: 'Test Agent',
        capabilities: ['chat'],
        endpoint: 'http://localhost:8001',
        status: 'online',
      },
      registryURL: 'http://registry.example.com',
    });

    await discovery.register();
    const agents = await discovery.discover();
    expect(agents).toHaveLength(1); // Only local
    fetchSpy.mockRestore();
  });
});

// ===== A2A Agent Server Tests =====

describe('A2AAgentServer', () => {
  function createMockAgent(): ReActAgent {
    const mockProvider: Provider = {
      complete: vi.fn().mockResolvedValue({
        id: 'resp-1',
        content: 'agent reply',
        role: 'assistant',
        usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 },
      }),
      callTools: vi.fn(),
      info: () => ({
        name: 'mock',
        provider: 'mock',
        maxContext: 4096,
        supportsTools: true,
        supportsStreaming: true,
      }),
    };

    return {
      run: vi.fn().mockResolvedValue({
        content: 'agent reply',
        metrics: {
          totalTurns: 1,
          totalTools: 0,
          duration: 100,
          llmLatency: 50,
          toolLatency: 0,
        },
      } as Response),
    } as unknown as ReActAgent;
  }

  it('should start and configure transport', async () => {
    const mockTransport = {
      start: vi.fn().mockResolvedValue(undefined),
      stop: vi.fn().mockResolvedValue(undefined),
      send: vi.fn(),
      onMessage: vi.fn(),
    };

    const server = new A2AAgentServer({
      agent: createMockAgent(),
      transport: mockTransport as any,
      agentInfo: { id: 'test-agent', name: 'Test' },
    });

    await server.start();
    expect(mockTransport.start).toHaveBeenCalled();
    expect(mockTransport.onMessage).toHaveBeenCalled();

    const info = server.getAgentInfo();
    expect(info.id).toBe('test-agent');
    expect(info.name).toBe('Test');
    expect(info.status).toBe('online');

    await server.stop();
    expect(mockTransport.stop).toHaveBeenCalled();
  });

  it('should use default agent info when not provided', async () => {
    const mockTransport = {
      start: vi.fn().mockResolvedValue(undefined),
      stop: vi.fn().mockResolvedValue(undefined),
      send: vi.fn(),
      onMessage: vi.fn(),
    };

    const server = new A2AAgentServer({
      agent: createMockAgent(),
      transport: mockTransport as any,
    });

    const info = server.getAgentInfo();
    expect(info.id).toMatch(/^agent-\d+$/);
    expect(info.name).toBe('agent');
    expect(info.capabilities).toEqual(['chat']);
    expect(info.status).toBe('online');
  });

  it('should handle request messages via transport handler', async () => {
    const handlers: ((msg: A2AMessage) => Promise<A2AMessage>)[] = [];
    const mockTransport = {
      start: vi.fn().mockResolvedValue(undefined),
      stop: vi.fn().mockResolvedValue(undefined),
      send: vi.fn(),
      onMessage: vi.fn((h: (msg: A2AMessage) => Promise<A2AMessage>) => handlers.push(h)),
    };

    const mockAgent = createMockAgent();
    const server = new A2AAgentServer({
      agent: mockAgent,
      transport: mockTransport as any,
      agentInfo: { id: 'srv-1', name: 'Server' },
    });

    await server.start();
    expect(handlers).toHaveLength(1);

    const requestMsg: A2AMessage = {
      id: 'req-1',
      from: 'client-1',
      to: 'srv-1',
      type: 'request',
      content: 'hello',
      timestamp: new Date().toISOString(),
    };

    const response = await handlers[0](requestMsg);
    expect(response.type).toBe('response');
    expect(response.content).toBe('agent reply');
    expect(response.to).toBe('client-1');
    expect(mockAgent.run).toHaveBeenCalledWith('hello');
  });

  it('should handle non-request messages', async () => {
    const handlers: ((msg: A2AMessage) => Promise<A2AMessage>)[] = [];
    const mockTransport = {
      start: vi.fn().mockResolvedValue(undefined),
      stop: vi.fn().mockResolvedValue(undefined),
      send: vi.fn(),
      onMessage: vi.fn((h: any) => handlers.push(h)),
    };

    const server = new A2AAgentServer({
      agent: createMockAgent(),
      transport: mockTransport as any,
    });

    await server.start();

    const notificationMsg: A2AMessage = {
      id: 'ntf-1',
      from: 'client-1',
      to: 'srv-1',
      type: 'notification',
      content: 'notification',
      timestamp: new Date().toISOString(),
    };

    const response = await handlers[0](notificationMsg);
    expect(response.type).toBe('response');
    expect(response.content).toBe('Unknown message type');
  });

  it('should handle agent errors gracefully', async () => {
    const handlers: ((msg: A2AMessage) => Promise<A2AMessage>)[] = [];
    const mockTransport = {
      start: vi.fn().mockResolvedValue(undefined),
      stop: vi.fn().mockResolvedValue(undefined),
      send: vi.fn(),
      onMessage: vi.fn((h: any) => handlers.push(h)),
    };

    const errorAgent = {
      run: vi.fn().mockRejectedValue(new Error('agent crashed')),
    } as unknown as ReActAgent;

    const server = new A2AAgentServer({
      agent: errorAgent,
      transport: mockTransport as any,
    });

    await server.start();

    const requestMsg: A2AMessage = {
      id: 'req-1',
      from: 'client-1',
      to: 'srv-1',
      type: 'request',
      content: 'hello',
      timestamp: new Date().toISOString(),
    };

    const response = await handlers[0](requestMsg);
    expect(response.type).toBe('error');
    expect(response.content).toContain('agent crashed');
  });

  it('should set endpoint for HTTP transport', async () => {
    const httpTransport = new HTTPTransport({ port: 0, host: '127.0.0.1' });

    const server = new A2AAgentServer({
      agent: createMockAgent(),
      transport: httpTransport,
    });

    await server.start();
    const info = server.getAgentInfo();
    expect(info.endpoint).toContain('127.0.0.1');
    await server.stop();
  });
});

// ===== A2A Client Tests =====

describe('A2AClient', () => {
  it('should send messages via transport', async () => {
    const mockTransport = {
      start: vi.fn(),
      stop: vi.fn(),
      send: vi.fn().mockResolvedValue({
        id: 'resp-1',
        from: 'target',
        to: 'client',
        type: 'response',
        content: 'reply from target',
        timestamp: new Date().toISOString(),
      }),
      onMessage: vi.fn(),
    };

    const client = new A2AClient(mockTransport as any, 'client-1');
    const result = await client.sendMessage('target', 'hello');
    expect(result).toBe('reply from target');
    expect(mockTransport.send).toHaveBeenCalled();
    const sentMsg = mockTransport.send.mock.calls[0][1] as A2AMessage;
    expect(sentMsg.from).toBe('client-1');
    expect(sentMsg.to).toBe('target');
    expect(sentMsg.content).toBe('hello');
    expect(sentMsg.type).toBe('request');
  });

  it('should generate unique message IDs', async () => {
    const mockTransport = {
      start: vi.fn(),
      stop: vi.fn(),
      send: vi.fn().mockImplementation(async (_target: string, msg: A2AMessage) => ({
        ...msg,
        type: 'response' as const,
        content: 'ok',
      })),
      onMessage: vi.fn(),
    };

    const client = new A2AClient(mockTransport as any, 'client-1');
    await client.sendMessage('target', 'msg1');
    await client.sendMessage('target', 'msg2');

    const id1 = mockTransport.send.mock.calls[0][1].id as string;
    const id2 = mockTransport.send.mock.calls[1][1].id as string;
    expect(id1).not.toBe(id2);
  });
});
