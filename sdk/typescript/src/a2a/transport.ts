import * as http from 'node:http';
import * as net from 'node:net';
import type { ReActAgent } from '../agent/react-loop.js';

// ===== A2A Message Types =====

export interface A2AMessage {
  id: string;
  from: string;
  to: string;
  type: 'request' | 'response' | 'notification' | 'error';
  content: string;
  metadata?: Record<string, unknown>;
  timestamp: string;
}

export interface A2AAgentInfo {
  id: string;
  name: string;
  description?: string;
  capabilities?: string[];
  endpoint: string;
  status: 'online' | 'offline' | 'busy';
}

// ===== A2A Transport Interface =====

export interface A2ATransport {
  start(): Promise<void>;
  stop(): Promise<void>;
  send(target: string, message: A2AMessage): Promise<A2AMessage>;
  onMessage(handler: (msg: A2AMessage) => Promise<A2AMessage>): void;
}

// ===== HTTP Transport =====

export interface HTTPTransportConfig {
  port: number;
  host?: string;
  authToken?: string;
}

export class HTTPTransport implements A2ATransport {
  private config: HTTPTransportConfig;
  private server?: http.Server;
  private messageHandler?: (msg: A2AMessage) => Promise<A2AMessage>;
  private agentRegistry: Map<string, string> = new Map(); // agentID -> endpoint URL

  constructor(config: HTTPTransportConfig) {
    this.config = { host: '0.0.0.0', ...config };
  }

  async start(): Promise<void> {
    return new Promise((resolve) => {
      this.server = http.createServer(async (req, res) => {
        if (req.method !== 'POST') {
          res.writeHead(405);
          res.end('Method Not Allowed');
          return;
        }

        // Auth check
        if (this.config.authToken) {
          const auth = req.headers.authorization;
          if (auth !== `Bearer ${this.config.authToken}`) {
            res.writeHead(401);
            res.end('Unauthorized');
            return;
          }
        }

        let body = '';
        req.on('data', (chunk) => { body += chunk; });
        req.on('end', async () => {
          try {
            const msg = JSON.parse(body) as A2AMessage;
            if (this.messageHandler) {
              const response = await this.messageHandler(msg);
              res.writeHead(200, { 'Content-Type': 'application/json' });
              res.end(JSON.stringify(response));
            } else {
              res.writeHead(503);
              res.end('No handler registered');
            }
          } catch (err) {
            res.writeHead(400);
            res.end(`Error: ${(err as Error).message}`);
          }
        });
      });

      this.server.listen(this.config.port, this.config.host, () => resolve());
    });
  }

  async stop(): Promise<void> {
    return new Promise((resolve) => {
      if (this.server) {
        this.server.close(() => resolve());
      } else {
        resolve();
      }
    });
  }

  async send(target: string, message: A2AMessage): Promise<A2AMessage> {
    const endpoint = this.agentRegistry.get(target);
    if (!endpoint) throw new Error(`Unknown agent: ${target}`);

    const resp = await fetch(endpoint, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(this.config.authToken ? { Authorization: `Bearer ${this.config.authToken}` } : {}),
      },
      body: JSON.stringify(message),
      signal: AbortSignal.timeout(60_000),
    });

    if (!resp.ok) throw new Error(`HTTP ${resp.status}: ${await resp.text()}`);
    return resp.json() as Promise<A2AMessage>;
  }

  onMessage(handler: (msg: A2AMessage) => Promise<A2AMessage>): void {
    this.messageHandler = handler;
  }

  registerAgent(agentID: string, endpoint: string): void {
    this.agentRegistry.set(agentID, endpoint);
  }

  unregisterAgent(agentID: string): void {
    this.agentRegistry.delete(agentID);
  }

  getEndpoint(): string {
    return `http://${this.config.host}:${this.config.port}`;
  }
}

// ===== TCP Transport =====

export interface TCPTransportConfig {
  port: number;
  host?: string;
}

export class TCPTransport implements A2ATransport {
  private config: TCPTransportConfig;
  private server?: net.Server;
  private messageHandler?: (msg: A2AMessage) => Promise<A2AMessage>;
  private connections: Map<string, net.Socket> = new Map();

  constructor(config: TCPTransportConfig) {
    this.config = { host: '0.0.0.0', ...config };
  }

  async start(): Promise<void> {
    return new Promise((resolve) => {
      this.server = net.createServer((socket) => {
        let buffer = '';

        socket.on('data', async (data) => {
          buffer += data.toString();
          // Messages are delimited by newlines
          const messages = buffer.split('\n');
          buffer = messages.pop() ?? '';

          for (const msgStr of messages) {
            if (!msgStr.trim()) continue;
            try {
              const msg = JSON.parse(msgStr) as A2AMessage;
              if (this.messageHandler) {
                const response = await this.messageHandler(msg);
                socket.write(JSON.stringify(response) + '\n');
              }
            } catch (err) {
              socket.write(JSON.stringify({
                id: 'error',
                from: 'server',
                to: 'unknown',
                type: 'error',
                content: `Error: ${(err as Error).message}`,
                timestamp: new Date().toISOString(),
              }) + '\n');
            }
          }
        });

        socket.on('error', () => {});
      });

      this.server.listen(this.config.port, this.config.host, () => resolve());
    });
  }

  async stop(): Promise<void> {
    return new Promise((resolve) => {
      for (const socket of this.connections.values()) {
        socket.destroy();
      }
      this.connections.clear();
      if (this.server) {
        this.server.close(() => resolve());
      } else {
        resolve();
      }
    });
  }

  async send(target: string, message: A2AMessage): Promise<A2AMessage> {
    // For TCP, we need a connection to the target
    // This is a simplified implementation
    const [host, portStr] = target.split(':');
    const port = parseInt(portStr);

    return new Promise((resolve, reject) => {
      const socket = net.createConnection(port, host, () => {
        socket.write(JSON.stringify(message) + '\n');
      });

      let response = '';
      socket.on('data', (data) => {
        response += data.toString();
        if (response.includes('\n')) {
          try {
            const msg = JSON.parse(response.trim()) as A2AMessage;
            resolve(msg);
            socket.destroy();
          } catch (err) {
            reject(err);
            socket.destroy();
          }
        }
      });

      socket.on('error', reject);
      socket.setTimeout(60_000, () => {
        reject(new Error('TCP timeout'));
        socket.destroy();
      });
    });
  }

  onMessage(handler: (msg: A2AMessage) => Promise<A2AMessage>): void {
    this.messageHandler = handler;
  }
}

// ===== Agent Discovery =====

export interface DiscoveryConfig {
  transport: A2ATransport;
  agentInfo: A2AAgentInfo;
  registryURL?: string; // Optional central registry
}

export class AgentDiscovery {
  private config: DiscoveryConfig;
  private knownAgents: Map<string, A2AAgentInfo> = new Map();

  constructor(config: DiscoveryConfig) {
    this.config = config;
  }

  /** Register this agent for discovery. */
  async register(): Promise<void> {
    this.knownAgents.set(this.config.agentInfo.id, {
      ...this.config.agentInfo,
      status: 'online',
    });

    // If central registry, register there too
    if (this.config.registryURL) {
      try {
        await fetch(`${this.config.registryURL}/agents`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(this.config.agentInfo),
          signal: AbortSignal.timeout(10_000),
        });
      } catch {}
    }
  }

  /** Deregister this agent. */
  async deregister(): Promise<void> {
    this.knownAgents.delete(this.config.agentInfo.id);

    if (this.config.registryURL) {
      try {
        await fetch(`${this.config.registryURL}/agents/${this.config.agentInfo.id}`, {
          method: 'DELETE',
          signal: AbortSignal.timeout(10_000),
        });
      } catch {}
    }
  }

  /** Discover available agents. */
  async discover(): Promise<A2AAgentInfo[]> {
    // Check local registry first
    if (this.config.registryURL) {
      try {
        const resp = await fetch(`${this.config.registryURL}/agents`, {
          signal: AbortSignal.timeout(10_000),
        });
        if (resp.ok) {
          const agents = await resp.json() as A2AAgentInfo[];
          for (const agent of agents) {
            this.knownAgents.set(agent.id, agent);
          }
        }
      } catch {}
    }

    return Array.from(this.knownAgents.values()).filter((a) => a.status === 'online');
  }

  /** Find a specific agent by ID. */
  findAgent(id: string): A2AAgentInfo | undefined {
    return this.knownAgents.get(id);
  }

  /** Find agents by capability. */
  findByCapability(capability: string): A2AAgentInfo[] {
    return Array.from(this.knownAgents.values()).filter(
      (a) => a.status === 'online' && a.capabilities?.includes(capability)
    );
  }
}

// ===== A2A Authentication =====

export interface AuthConfig {
  type: 'bearer' | 'api_key' | 'basic';
  token?: string;
  apiKey?: string;
  username?: string;
  password?: string;
}

export class A2AAuth {
  private config: AuthConfig;

  constructor(config: AuthConfig) {
    this.config = config;
  }

  /** Generate auth headers for a request. */
  getHeaders(): Record<string, string> {
    switch (this.config.type) {
      case 'bearer':
        return { Authorization: `Bearer ${this.config.token}` };
      case 'api_key':
        return { 'X-API-Key': this.config.apiKey ?? '' };
      case 'basic': {
        const cred = Buffer.from(`${this.config.username}:${this.config.password}`).toString('base64');
        return { Authorization: `Basic ${cred}` };
      }
      default:
        return {};
    }
  }

  /** Verify an incoming request's auth. */
  verify(headers: Record<string, string>): boolean {
    switch (this.config.type) {
      case 'bearer':
        return headers.authorization === `Bearer ${this.config.token}`;
      case 'api_key':
        return headers['x-api-key'] === this.config.apiKey;
      case 'basic': {
        const cred = Buffer.from(`${this.config.username}:${this.config.password}`).toString('base64');
        return headers.authorization === `Basic ${cred}`;
      }
      default:
        return true;
    }
  }
}

// ===== A2A Agent Server =====
// Wraps a ReActAgent as an A2A server

export interface A2AServerConfig {
  agent: ReActAgent;
  transport: A2ATransport;
  agentInfo?: Partial<A2AAgentInfo>;
}

export class A2AAgentServer {
  private config: A2AServerConfig;
  private agentInfo: A2AAgentInfo;

  constructor(config: A2AServerConfig) {
    this.config = config;
    this.agentInfo = {
      id: config.agentInfo?.id ?? `agent-${Date.now()}`,
      name: config.agentInfo?.name ?? 'agent',
      description: config.agentInfo?.description,
      capabilities: config.agentInfo?.capabilities ?? ['chat'],
      endpoint: '',
      status: 'online',
    };
  }

  async start(): Promise<void> {
    this.config.transport.onMessage(async (msg) => {
      if (msg.type === 'request') {
        try {
          this.agentInfo.status = 'busy';
          const response = await this.config.agent.run(msg.content);
          this.agentInfo.status = 'online';

          return {
            id: `resp-${Date.now()}`,
            from: this.agentInfo.id,
            to: msg.from,
            type: 'response' as const,
            content: response.content,
            metadata: { originalMsgId: msg.id },
            timestamp: new Date().toISOString(),
          };
        } catch (err) {
          this.agentInfo.status = 'online';
          return {
            id: `err-${Date.now()}`,
            from: this.agentInfo.id,
            to: msg.from,
            type: 'error' as const,
            content: `Error: ${(err as Error).message}`,
            timestamp: new Date().toISOString(),
          };
        }
      }
      return {
        id: `resp-${Date.now()}`,
        from: this.agentInfo.id,
        to: msg.from,
        type: 'response' as const,
        content: 'Unknown message type',
        timestamp: new Date().toISOString(),
      };
    });

    await this.config.transport.start();

    // Update endpoint if HTTP transport
    if (this.config.transport instanceof HTTPTransport) {
      this.agentInfo.endpoint = this.config.transport.getEndpoint();
    }
  }

  async stop(): Promise<void> {
    await this.config.transport.stop();
  }

  getAgentInfo(): A2AAgentInfo {
    return { ...this.agentInfo };
  }
}

// ===== A2A Client =====
// Sends messages to remote agents

export class A2AClient {
  private transport: A2ATransport;
  private agentID: string;

  constructor(transport: A2ATransport, agentID: string) {
    this.transport = transport;
    this.agentID = agentID;
  }

  async sendMessage(target: string, content: string): Promise<string> {
    const msg: A2AMessage = {
      id: `msg-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      from: this.agentID,
      to: target,
      type: 'request',
      content,
      timestamp: new Date().toISOString(),
    };

    const response = await this.transport.send(target, msg);
    return response.content;
  }
}
