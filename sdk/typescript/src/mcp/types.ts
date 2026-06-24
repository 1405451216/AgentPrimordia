// MCP (Model Context Protocol) client implementation
// Supports both stdio (child process) and http (REST) transport modes.
// Protocol spec: https://spec.modelcontextprotocol.io/

import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';

// ===== MCP Protocol Types =====

export interface MCPToolDefinition {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
}

export interface MCPToolCall {
  name: string;
  arguments: Record<string, unknown>;
}

export interface MCPContentBlock {
  type: 'text' | 'image';
  text?: string;
  data?: string;
  mimeType?: string;
}

export interface MCPToolResult {
  content: MCPContentBlock[];
  isError?: boolean;
}

export interface MCPServerInfo {
  name: string;
  version: string;
}

export interface MCPResource {
  uri: string;
  name: string;
  description?: string;
  mimeType?: string;
}

export interface MCPServerConfig {
  name: string;
  version: string;
  transport: 'stdio' | 'http';
  // stdio mode
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  // http mode
  url?: string;
  // common
  timeout?: number;
}

export interface MCPListToolsResponse {
  tools: MCPToolDefinition[];
}

// ===== JSON-RPC 2.0 Types =====

interface JSONRPCRequest {
  jsonrpc: '2.0';
  id: number;
  method: string;
  params?: unknown;
}

interface JSONRPCNotification {
  jsonrpc: '2.0';
  method: string;
  params?: unknown;
}

interface JSONRPCError {
  code: number;
  message: string;
  data?: unknown;
}

interface JSONRPCResponse {
  jsonrpc: '2.0';
  id: number;
  result?: unknown;
  error?: JSONRPCError | null;
}

// ===== Transport Layer =====

interface Transport {
  sendRequest(req: JSONRPCRequest, timeout: number): Promise<JSONRPCResponse>;
  sendNotification(req: JSONRPCNotification): Promise<void>;
  close(): Promise<void>;
}

// ===== HTTP Transport =====

class HTTPTransport implements Transport {
  private baseURL: string;

  constructor(url: string) {
    this.baseURL = url.replace(/\/+$/, '');
  }

  async sendRequest(req: JSONRPCRequest, timeout: number): Promise<JSONRPCResponse> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeout);

    try {
      const resp = await fetch(this.baseURL, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
        signal: controller.signal,
      });

      if (!resp.ok) {
        const text = await resp.text();
        throw new Error(`MCP server returned HTTP ${resp.status}: ${text}`);
      }

      const data = (await resp.json()) as JSONRPCResponse;
      return data;
    } finally {
      clearTimeout(timer);
    }
  }

  async sendNotification(req: JSONRPCNotification): Promise<void> {
    await fetch(this.baseURL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
  }

  async close(): Promise<void> {
    // HTTP transport has no persistent state to close
  }
}

// ===== stdio Transport =====

class StdioTransport implements Transport {
  private proc: ChildProcessWithoutNullStreams;
  private pending = new Map<number, { resolve: (v: JSONRPCResponse) => void; reject: (e: Error) => void }>();
  private buffer = '';
  private closed = false;

  constructor(command: string, args: string[], env?: Record<string, string>) {
    const spawnEnv = { ...process.env, ...env };
    this.proc = spawn(command, args, {
      env: spawnEnv,
      stdio: ['pipe', 'pipe', 'pipe'],
    });

    this.proc.stdout.on('data', (data: Buffer) => {
      this.buffer += data.toString('utf-8');
      this.processBuffer();
    });

    this.proc.stderr.on('data', () => {
      // Discard stderr to avoid leaking sensitive info
    });

    this.proc.on('error', (err) => {
      if (!this.closed) {
        for (const { reject } of this.pending.values()) {
          reject(new Error(`MCP process error: ${err.message}`));
        }
        this.pending.clear();
      }
    });

    this.proc.on('exit', (code) => {
      if (!this.closed) {
        for (const { reject } of this.pending.values()) {
          reject(new Error(`MCP process exited with code ${code}`));
        }
        this.pending.clear();
      }
    });
  }

  private processBuffer(): void {
    while (true) {
      const idx = this.buffer.indexOf('\n');
      if (idx === -1) break;

      const line = this.buffer.slice(0, idx).trim();
      this.buffer = this.buffer.slice(idx + 1);

      if (line === '') continue;

      try {
        const resp = JSON.parse(line) as JSONRPCResponse;
        const entry = this.pending.get(resp.id);
        if (entry) {
          this.pending.delete(resp.id);
          entry.resolve(resp);
        }
      } catch {
        // Ignore malformed JSON lines
      }
    }
  }

  async sendRequest(req: JSONRPCRequest, timeout: number): Promise<JSONRPCResponse> {
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        if (this.pending.has(req.id)) {
          this.pending.delete(req.id);
          reject(new Error(`MCP request "${req.method}" timed out after ${timeout}ms`));
        }
      }, timeout);

      // Wrap resolve to clear timeout on response
      const wrappedResolve = (v: JSONRPCResponse) => {
        clearTimeout(timer);
        resolve(v);
      };
      this.pending.set(req.id, { resolve: wrappedResolve, reject });

      const body = JSON.stringify(req) + '\n';
      this.proc.stdin.write(body, (err) => {
        if (err) {
          this.pending.delete(req.id);
          clearTimeout(timer);
          reject(new Error(`Failed to write to MCP stdin: ${err.message}`));
        }
      });
    });
  }

  async sendNotification(req: JSONRPCNotification): Promise<void> {
    const body = JSON.stringify(req) + '\n';
    return new Promise((resolve, reject) => {
      this.proc.stdin.write(body, (err) => {
        if (err) reject(new Error(`Failed to write notification: ${err.message}`));
        else resolve();
      });
    });
  }

  async close(): Promise<void> {
    if (this.closed) return;
    this.closed = true;

    // Reject all pending requests
    for (const { reject } of this.pending.values()) {
      reject(new Error('MCP transport closed'));
    }
    this.pending.clear();

    // Gracefully terminate the child process
    return new Promise((resolve) => {
      const proc = this.proc;
      const killTimer = setTimeout(() => {
        proc.kill('SIGKILL');
      }, 3000);

      proc.on('exit', () => {
        clearTimeout(killTimer);
        resolve();
      });

      // Try graceful shutdown first
      proc.stdin.end();
      proc.kill('SIGTERM');
    });
  }
}

// ===== MCP Client =====

const MCP_PROTOCOL_VERSION = '2024-11-05';
const DEFAULT_TIMEOUT = 30_000;

export class MCPClient {
  private config: MCPServerConfig;
  private transport: Transport | null = null;
  private tools: MCPToolDefinition[] = [];
  private resources: MCPResource[] = [];
  private serverInfo: MCPServerInfo | null = null;
  private requestCounter = 0;
  private connected = false;

  constructor(config: MCPServerConfig) {
    this.config = config;
  }

  /**
   * Connect to the MCP server and initialize the session.
   * Performs the MCP handshake: initialize → notifications/initialized → tools/list
   */
  async connect(): Promise<void> {
    if (this.connected) return;

    // Create transport based on config
    if (this.config.transport === 'http') {
      if (!this.config.url) {
        throw new Error('HTTP transport requires a "url" in config');
      }
      this.transport = new HTTPTransport(this.config.url);
    } else if (this.config.transport === 'stdio') {
      if (!this.config.command) {
        throw new Error('stdio transport requires a "command" in config');
      }
      this.transport = new StdioTransport(
        this.config.command,
        this.config.args ?? [],
        this.config.env,
      );
    } else {
      throw new Error(`Unsupported transport type: ${this.config.transport}`);
    }

    const timeout = this.config.timeout ?? DEFAULT_TIMEOUT;

    // 1. Send initialize request
    const initResp = await this.sendRequest('initialize', {
      protocolVersion: MCP_PROTOCOL_VERSION,
      capabilities: {},
      clientInfo: {
        name: this.config.name || 'AgentPrimordia-TS',
        version: this.config.version || '0.1.0',
      },
    }, timeout);

    if (initResp.error) {
      throw new Error(`MCP initialize failed: ${initResp.error.message}`);
    }

    // Parse server info
    const initResult = initResp.result as Record<string, unknown> | undefined;
    if (initResult) {
      const si = initResult.serverInfo as Record<string, string> | undefined;
      if (si) {
        this.serverInfo = { name: si.name ?? '', version: si.version ?? '' };
      }
    }

    // 2. Send initialized notification
    await this.sendNotification('notifications/initialized');

    // 3. Fetch tool list
    await this.refreshTools();

    this.connected = true;
  }

  /** Re-fetch the tool list from the server */
  async refreshTools(): Promise<void> {
    const resp = await this.sendRequest('tools/list', null, this.config.timeout ?? DEFAULT_TIMEOUT);

    if (resp.error) {
      throw new Error(`MCP tools/list failed: ${resp.error.message}`);
    }

    const result = resp.result as { tools?: MCPToolDefinition[] } | undefined;
    this.tools = result?.tools ?? [];
  }

  /** List available resources from the server */
  async listResources(): Promise<MCPResource[]> {
    const resp = await this.sendRequest('resources/list', null, this.config.timeout ?? DEFAULT_TIMEOUT);
    if (resp.error) {
      throw new Error(`MCP resources/list failed: ${resp.error.message}`);
    }
    const result = resp.result as { resources?: MCPResource[] } | undefined;
    this.resources = result?.resources ?? [];
    return this.resources;
  }

  /** Read a resource by URI */
  async readResource(uri: string): Promise<string> {
    const resp = await this.sendRequest('resources/read', { uri }, this.config.timeout ?? DEFAULT_TIMEOUT);
    if (resp.error) {
      throw new Error(`MCP resources/read failed: ${resp.error.message}`);
    }
    const result = resp.result as { contents?: Array<{ text?: string }> } | undefined;
    return result?.contents?.[0]?.text ?? '';
  }

  /** Get the list of discovered tools */
  listTools(): MCPToolDefinition[] {
    return this.tools;
  }

  /** Get server info */
  getServerInfo(): MCPServerInfo | null {
    return this.serverInfo;
  }

  /** Check if the client is connected */
  isConnected(): boolean {
    return this.connected;
  }

  /** Call a tool on the MCP server */
  async callTool(call: MCPToolCall): Promise<MCPToolResult> {
    if (!this.connected || !this.transport) {
      return {
        content: [{ type: 'text', text: 'MCP client is not connected' }],
        isError: true,
      };
    }

    const resp = await this.sendRequest('tools/call', {
      name: call.name,
      arguments: call.arguments,
    }, this.config.timeout ?? DEFAULT_TIMEOUT);

    if (resp.error) {
      return {
        content: [{ type: 'text', text: resp.error.message }],
        isError: true,
      };
    }

    const result = resp.result as { content?: MCPContentBlock[]; isError?: boolean } | undefined;
    return {
      content: result?.content ?? [],
      isError: result?.isError ?? false,
    };
  }

  /** Disconnect from the MCP server */
  async disconnect(): Promise<void> {
    if (!this.connected || !this.transport) {
      this.tools = [];
      this.resources = [];
      this.connected = false;
      return;
    }

    // Try to send shutdown notification (best effort)
    try {
      await this.sendNotification('shutdown');
    } catch {
      // Ignore errors during shutdown
    }

    await this.transport.close();
    this.transport = null;
    this.tools = [];
    this.resources = [];
    this.serverInfo = null;
    this.connected = false;
  }

  // ===== Internal JSON-RPC methods =====

  private nextId(): number {
    return ++this.requestCounter;
  }

  private async sendRequest(method: string, params: unknown, timeout: number): Promise<JSONRPCResponse> {
    if (!this.transport) {
      throw new Error('MCP transport is not initialized');
    }

    const req: JSONRPCRequest = {
      jsonrpc: '2.0',
      id: this.nextId(),
      method,
      params: params ?? undefined,
    };

    return this.transport.sendRequest(req, timeout);
  }

  private async sendNotification(method: string, params?: unknown): Promise<void> {
    if (!this.transport) {
      throw new Error('MCP transport is not initialized');
    }

    const notif: JSONRPCNotification = {
      jsonrpc: '2.0',
      method,
      params,
    };

    await this.transport.sendNotification(notif);
  }
}
