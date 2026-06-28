import type { MCPClient, MCPServerConfig, MCPToolDefinition } from './types.js';
import type { Tool } from '../types.js';

// ===== MCP Registry — manages multiple MCP server connections =====

export interface RegisteredMCPServer {
  name: string;
  config: MCPServerConfig;
  client: MCPClient;
  tools: MCPToolDefinition[];
}

export class MCPRegistry {
  private servers: Map<string, RegisteredMCPServer> = new Map();
  private toolIndex: Map<string, { serverName: string; tool: MCPToolDefinition }> = new Map();

  async register(name: string, config: MCPServerConfig, client: MCPClient): Promise<void> {
    if (!client.isConnected()) {
      await client.connect();
    }

    const tools = client.listTools();

    this.servers.set(name, { name, config, client, tools });

    // Index tools by name
    for (const tool of tools) {
      this.toolIndex.set(tool.name, { serverName: name, tool });
    }
  }

  async unregister(name: string): Promise<void> {
    const server = this.servers.get(name);
    if (!server) return;

    // Remove from tool index
    for (const tool of server.tools) {
      const entry = this.toolIndex.get(tool.name);
      if (entry && entry.serverName === name) {
        this.toolIndex.delete(tool.name);
      }
    }

    await server.client.disconnect();
    this.servers.delete(name);
  }

  async unregisterAll(): Promise<void> {
    const names = Array.from(this.servers.keys());
    await Promise.all(names.map(n => this.unregister(n)));
  }

  getServer(name: string): RegisteredMCPServer | undefined {
    return this.servers.get(name);
  }

  getAllTools(): MCPToolDefinition[] {
    return Array.from(this.toolIndex.values()).map(e => e.tool);
  }

  getTool(toolName: string): { serverName: string; tool: MCPToolDefinition } | undefined {
    return this.toolIndex.get(toolName);
  }

  async callTool(toolName: string, args: Record<string, unknown>): Promise<string> {
    const entry = this.toolIndex.get(toolName);
    if (!entry) {
      throw new Error(`Tool "${toolName}" not found in any MCP server`);
    }

    const server = this.servers.get(entry.serverName);
    if (!server) {
      throw new Error(`MCP server "${entry.serverName}" not registered`);
    }

    const result = await server.client.callTool({ name: toolName, arguments: args });

    // Extract text from content
    const text = result.content
      .filter(c => c.type === 'text')
      .map(c => c.text)
      .join('\n');

    if (result.isError) {
      throw new Error(text);
    }

    return text;
  }

  listServers(): string[] {
    return Array.from(this.servers.keys());
  }
}

// ===== MCP Adapter — bridges MCP tools to the framework's Tool interface =====

export class MCPAdapter {
  private registry: MCPRegistry;

  constructor(registry: MCPRegistry) {
    this.registry = registry;
  }

  /** Convert all MCP tools to framework Tool format. */
  toTools(): Tool[] {
    return this.registry.getAllTools().map(tool => this.toTool(tool));
  }

  /** Convert a single MCP tool to framework Tool format. */
  toTool(mcpTool: MCPToolDefinition): Tool {
    return {
      name: mcpTool.name,
      description: mcpTool.description,
      parameters: mcpTool.inputSchema as Record<string, unknown>,
      execute: async (args: Record<string, unknown>): Promise<string> => {
        return this.registry.callTool(mcpTool.name, args);
      },
    };
  }
}

// ===== A2A JSON-RPC =====

export interface JSONRPCRequest {
  jsonrpc: '2.0';
  id: number;
  method: string;
  params?: unknown;
}

export interface JSONRPCNotification {
  jsonrpc: '2.0';
  method: string;
  params?: unknown;
}

export interface JSONRPCError {
  code: number;
  message: string;
  data?: unknown;
}

export interface JSONRPCResponse {
  jsonrpc: '2.0';
  id: number;
  result?: unknown;
  error?: JSONRPCError | null;
}

export class JSONRPCHandler {
  private handlers: Map<string, (params: unknown) => Promise<unknown>> = new Map();

  register(method: string, handler: (params: unknown) => Promise<unknown>): void {
    this.handlers.set(method, handler);
  }

  async handle(request: JSONRPCRequest): Promise<JSONRPCResponse> {
    const handler = this.handlers.get(request.method);
    if (!handler) {
      return {
        jsonrpc: '2.0',
        id: request.id,
        error: { code: -32601, message: `Method not found: ${request.method}` },
      };
    }

    try {
      const result = await handler(request.params);
      return { jsonrpc: '2.0', id: request.id, result };
    } catch (err) {
      return {
        jsonrpc: '2.0',
        id: request.id,
        error: {
          code: -32603,
          message: err instanceof Error ? err.message : 'Internal error',
          data: String(err),
        },
      };
    }
  }

  createRequest(id: number, method: string, params?: unknown): JSONRPCRequest {
    return { jsonrpc: '2.0', id, method, params };
  }

  createNotification(method: string, params?: unknown): JSONRPCNotification {
    return { jsonrpc: '2.0', method, params };
  }
}

// ===== A2A Task Manager =====

export type A2ATaskState = 'submitted' | 'working' | 'input_required' | 'completed' | 'canceled' | 'failed';

export interface A2ATask {
  id: string;
  state: A2ATaskState;
  message?: { role: string; content: unknown };
  result?: unknown;
  artifacts?: A2AArtifact[];
  createdAt: string;
  updatedAt: string;
}

export interface A2AArtifact {
  name: string;
  mimeType: string;
  content: string;
}

export class A2ATaskManager {
  private tasks: Map<string, A2ATask> = new Map();
  private subscribers: Map<string, ((task: A2ATask) => void)[]> = new Map();

  createTask(message?: { role: string; content: unknown }): A2ATask {
    const task: A2ATask = {
      id: `task-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      state: 'submitted',
      message,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };
    this.tasks.set(task.id, task);
    return task;
  }

  getTask(id: string): A2ATask | undefined {
    return this.tasks.get(id);
  }

  updateTask(id: string, updates: Partial<A2ATask>): A2ATask | null {
    const task = this.tasks.get(id);
    if (!task) return null;
    Object.assign(task, updates, { updatedAt: new Date().toISOString() });
    this.notify(id, task);
    return task;
  }

  cancelTask(id: string): A2ATask | null {
    return this.updateTask(id, { state: 'canceled' });
  }

  completeTask(id: string, result: unknown, artifacts?: A2AArtifact[]): A2ATask | null {
    return this.updateTask(id, { state: 'completed', result, artifacts });
  }

  failTask(id: string, error: string): A2ATask | null {
    return this.updateTask(id, { state: 'failed', result: { error } });
  }

  listTasks(state?: A2ATaskState): A2ATask[] {
    const all = Array.from(this.tasks.values());
    return state ? all.filter(t => t.state === state) : all;
  }

  subscribe(taskId: string, callback: (task: A2ATask) => void): () => void {
    if (!this.subscribers.has(taskId)) {
      this.subscribers.set(taskId, []);
    }
    this.subscribers.get(taskId)!.push(callback);

    return () => {
      const subs = this.subscribers.get(taskId);
      if (subs) {
        const idx = subs.indexOf(callback);
        if (idx >= 0) subs.splice(idx, 1);
      }
    };
  }

  private notify(taskId: string, task: A2ATask): void {
    const subs = this.subscribers.get(taskId);
    if (subs) {
      for (const cb of subs) cb(task);
    }
  }
}

// ===== A2A Bridge — connect framework agents to A2A protocol =====

export interface BridgeAgent {
  name: string;
  description: string;
  capabilities: string[];
  run(input: unknown): Promise<unknown>;
}

export class A2ABridge {
  private agents: Map<string, BridgeAgent> = new Map();
  private taskManager: A2ATaskManager;

  constructor(taskManager?: A2ATaskManager) {
    this.taskManager = taskManager ?? new A2ATaskManager();
  }

  registerAgent(agent: BridgeAgent): void {
    this.agents.set(agent.name, agent);
  }

  unregisterAgent(name: string): void {
    this.agents.delete(name);
  }

  listAgents(): Array<{ name: string; description: string; capabilities: string[] }> {
    return Array.from(this.agents.values()).map(a => ({
      name: a.name,
      description: a.description,
      capabilities: a.capabilities,
    }));
  }

  async sendTask(agentName: string, input: unknown): Promise<A2ATask> {
    const agent = this.agents.get(agentName);
    if (!agent) throw new Error(`Agent "${agentName}" not registered`);

    const task = this.taskManager.createTask({ role: 'user', content: input });
    this.taskManager.updateTask(task.id, { state: 'working' });

    try {
      const result = await agent.run(input);
      this.taskManager.completeTask(task.id, result);
    } catch (err) {
      this.taskManager.failTask(task.id, err instanceof Error ? err.message : String(err));
    }

    return this.taskManager.getTask(task.id)!;
  }

  getTaskManager(): A2ATaskManager {
    return this.taskManager;
  }
}

// ===== A2A SSE Transport =====

export class A2ASSETransport {
  private encoder = new TextEncoder();

  async writeSSE(
    writer: WritableStreamDefaultWriter<Uint8Array>,
    event: string,
    data: unknown
  ): Promise<void> {
    const dataStr = typeof data === 'string' ? data : JSON.stringify(data);
    const lines = dataStr.split('\n').map(l => `data: ${l}`).join('\n');
    const output = `event: ${event}\n${lines}\n\n`;
    await writer.write(this.encoder.encode(output));
  }

  async *readSSE(
    reader: ReadableStreamDefaultReader<Uint8Array>
  ): AsyncIterable<{ event?: string; data: string }> {
    let buffer = '';
    let currentEvent: string | undefined;
    let dataLines: string[] = [];

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += new TextDecoder().decode(value);
      const lines = buffer.split('\n');
      buffer = lines.pop() ?? '';

      for (const line of lines) {
        if (line.startsWith('event: ')) {
          currentEvent = line.slice(7).trim();
        } else if (line.startsWith('data: ')) {
          dataLines.push(line.slice(6));
        } else if (line === '' && dataLines.length > 0) {
          yield { event: currentEvent, data: dataLines.join('\n') };
          currentEvent = undefined;
          dataLines = [];
        }
      }
    }
  }
}
