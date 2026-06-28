// Admin HTTP API — Bearer Token authenticated management endpoints
// Mirrors Go internal/admin/handler.go

import type { IncomingMessage, ServerResponse } from 'node:http';
import type { ToolRegistry } from '../tools/registry.js';
import type { AgentPool, PoolTask, PoolResult } from '../pool/agent-pool.js';

// ===== Admin Handler Config =====

export interface AdminHandlerConfig {
  pool?: AgentPool;
  registry?: ToolRegistry;
  apiToken: string;
  /** Optional: provide task history for /api/tasks and /api/workflows */
  getTasks?: () => AdminTaskInfo[];
  /** Optional: provide agent list for /api/agents */
  getAgents?: () => Record<string, string>;
  /** Optional: provide pool stats for /api/stats */
  getStats?: () => AdminPoolStats;
}

export interface AdminTaskInfo {
  id: string;
  title: string;
  status: string;
  duration: string;
  sessionId?: string;
  error?: string;
  prompt?: string;
  response?: string;
}

export interface AdminPoolStats {
  totalTasks: number;
  completedTasks: number;
  failedTasks: number;
  runningTasks: number;
  queuedTasks: number;
  maxConcurrency: number;
  activeConcurrency: number;
}

// ===== Admin Handler =====

export class AdminHandler {
  private config: AdminHandlerConfig;
  private routes: Map<string, (req: IncomingMessage, res: ServerResponse) => void | Promise<void>>;

  constructor(config: AdminHandlerConfig) {
    this.config = config;
    this.routes = new Map();

    // Public health endpoint
    this.routes.set('GET /api/health', this.health.bind(this));

    // Protected endpoints
    this.routes.set('GET /api/agents', this.requireAuth(this.listAgents.bind(this)));
    this.routes.set('GET /api/agents/', this.requireAuth(this.getAgent.bind(this)));
    this.routes.set('GET /api/stats', this.requireAuth(this.stats.bind(this)));
    this.routes.set('GET /api/tasks', this.requireAuth(this.tasks.bind(this)));
    this.routes.set('GET /api/system', this.requireAuth(this.systemInfo.bind(this)));
    this.routes.set('GET /api/tools', this.requireAuth(this.listTools.bind(this)));
    this.routes.set('GET /api/tools/', this.requireAuth(this.getTool.bind(this)));
    this.routes.set('GET /api/tools/categories', this.requireAuth(this.toolCategories.bind(this)));
    this.routes.set('GET /api/workflows', this.requireAuth(this.listWorkflows.bind(this)));
    this.routes.set('GET /api/workflows/', this.requireAuth(this.getWorkflow.bind(this)));
    this.routes.set('GET /api/logs/stream', this.requireAuth(this.logStream.bind(this)));
    this.routes.set('GET /', this.index.bind(this));
  }

  /** Handle an HTTP request. */
  async handle(req: IncomingMessage, res: ServerResponse): Promise<void> {
    const method = req.method ?? 'GET';
    const url = req.url ?? '/';

    // Try exact match first
    let handler = this.routes.get(`${method} ${url}`);

    // Try prefix match for path parameters
    if (!handler) {
      for (const [pattern, h] of this.routes) {
        const [pMethod, pPath] = pattern.split(' ');
        if (pMethod === method && pPath!.endsWith('/')) {
          if (url.startsWith(pPath!)) {
            handler = h;
            break;
          }
        }
      }
    }

    if (!handler) {
      this.writeJSON(res, 404, { error: 'Not found' });
      return;
    }

    try {
      await handler(req, res);
    } catch (err) {
      this.writeJSON(res, 500, { error: (err as Error).message });
    }
  }

  /** Auth middleware wrapper. */
  private requireAuth(
    next: (req: IncomingMessage, res: ServerResponse) => void | Promise<void>
  ): (req: IncomingMessage, res: ServerResponse) => void | Promise<void> {
    return (req, res) => {
      if (!this.config.apiToken) {
        this.writeJSON(res, 401, { error: 'Admin API token not configured' });
        return;
      }

      const auth = req.headers['authorization'] ?? '';
      const prefix = 'Bearer ';
      if (!auth.startsWith(prefix) || auth.slice(prefix.length) !== this.config.apiToken) {
        this.writeJSON(res, 401, { error: 'Invalid or missing Bearer token' });
        return;
      }

      return next(req, res);
    };
  }

  // ===== Endpoint Handlers =====

  private health(_req: IncomingMessage, res: ServerResponse): void {
    const stats = this.config.getStats?.() ?? { totalTasks: 0, runningTasks: 0 } as Partial<AdminPoolStats>;
    this.writeJSON(res, 200, {
      status: 'healthy',
      timestamp: new Date().toISOString(),
      tasks: stats.totalTasks ?? 0,
      running: stats.runningTasks ?? 0,
    });
  }

  private listAgents(_req: IncomingMessage, res: ServerResponse): void {
    const agents = this.config.getAgents?.() ?? {};
    this.writeJSON(res, 200, agents);
  }

  private getAgent(req: IncomingMessage, res: ServerResponse): void {
    const id = (req.url ?? '').split('/').pop() ?? '';
    if (!id) {
      this.writeJSON(res, 400, { error: 'Missing agent ID' });
      return;
    }

    const agents = this.config.getAgents?.() ?? {};
    if (id in agents) {
      this.writeJSON(res, 200, { id, status: agents[id] });
    } else {
      this.writeJSON(res, 404, { error: 'Agent not found' });
    }
  }

  private stats(_req: IncomingMessage, res: ServerResponse): void {
    const stats = this.config.getStats?.() ?? {
      totalTasks: 0, completedTasks: 0, failedTasks: 0,
      runningTasks: 0, queuedTasks: 0, maxConcurrency: 0, activeConcurrency: 0,
    };
    this.writeJSON(res, 200, stats);
  }

  private tasks(_req: IncomingMessage, res: ServerResponse): void {
    const tasks = this.config.getTasks?.() ?? [];
    this.writeJSON(res, 200, tasks);
  }

  private systemInfo(_req: IncomingMessage, res: ServerResponse): void {
    const mem = process.memoryUsage();
    this.writeJSON(res, 200, {
      node_version: process.version,
      platform: process.platform,
      arch: process.arch,
      pid: process.pid,
      uptime_seconds: Math.floor(process.uptime()),
      mem_rss_mb: Math.round((mem.rss / 1024 / 1024) * 100) / 100,
      mem_heap_used_mb: Math.round((mem.heapUsed / 1024 / 1024) * 100) / 100,
      mem_heap_total_mb: Math.round((mem.heapTotal / 1024 / 1024) * 100) / 100,
    });
  }

  private listTools(_req: IncomingMessage, res: ServerResponse): void {
    if (!this.config.registry) {
      this.writeJSON(res, 200, []);
      return;
    }
    const tools = this.config.registry.list().map((t) => ({
      name: t.name,
      description: t.description,
      parameters: t.parameters,
    }));
    this.writeJSON(res, 200, tools);
  }

  private getTool(req: IncomingMessage, res: ServerResponse): void {
    if (!this.config.registry) {
      this.writeJSON(res, 404, { error: 'Tool registry is empty' });
      return;
    }
    const name = (req.url ?? '').split('/').pop() ?? '';
    if (!name) {
      this.writeJSON(res, 400, { error: 'Missing tool name' });
      return;
    }
    const tool = this.config.registry.get(name);
    if (!tool) {
      this.writeJSON(res, 404, { error: 'Tool not found' });
      return;
    }
    this.writeJSON(res, 200, {
      name: tool.name,
      description: tool.description,
      parameters: tool.parameters,
    });
  }

  private toolCategories(_req: IncomingMessage, res: ServerResponse): void {
    if (!this.config.registry) {
      this.writeJSON(res, 200, {});
      return;
    }
    // Group tools by name prefix or "general"
    const categories: Record<string, string[]> = {};
    for (const tool of this.config.registry.list()) {
      const cat = tool.name.includes('_') ? tool.name.split('_')[0]! : 'general';
      if (!categories[cat]) categories[cat] = [];
      categories[cat].push(tool.name);
    }
    this.writeJSON(res, 200, categories);
  }

  private listWorkflows(_req: IncomingMessage, res: ServerResponse): void {
    const tasks = this.config.getTasks?.() ?? [];
    this.writeJSON(res, 200, tasks.map((t) => ({
      id: t.id,
      title: t.title,
      status: t.status,
      duration: t.duration,
      session_id: t.sessionId,
      error: t.error,
    })));
  }

  private getWorkflow(req: IncomingMessage, res: ServerResponse): void {
    const id = (req.url ?? '').split('/').pop() ?? '';
    if (!id) {
      this.writeJSON(res, 400, { error: 'Missing workflow ID' });
      return;
    }
    const tasks = this.config.getTasks?.() ?? [];
    const task = tasks.find((t) => t.id === id);
    if (!task) {
      this.writeJSON(res, 404, { error: 'Workflow not found' });
      return;
    }
    this.writeJSON(res, 200, task);
  }

  private logStream(req: IncomingMessage, res: ServerResponse): void {
    res.writeHead(200, {
      'Content-Type': 'text/event-stream',
      'Cache-Control': 'no-cache',
      'Connection': 'keep-alive',
    });
    res.write('data: {"message":"Log stream connected"}\n\n');
    // Keep connection alive with heartbeat
    const interval = setInterval(() => {
      res.write(': heartbeat\n\n');
    }, 30000);
    req.on('close', () => clearInterval(interval));
  }

  private index(_req: IncomingMessage, res: ServerResponse): void {
    res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
    res.end(ADMIN_HTML);
  }

  // ===== Helpers =====

  private writeJSON(res: ServerResponse, status: number, data: unknown): void {
    res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' });
    res.end(JSON.stringify(data));
  }
}

const ADMIN_HTML = `<!DOCTYPE html>
<html>
<head>
  <title>AgentPrimordia Admin</title>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
    .container { max-width: 1200px; margin: 0 auto; }
    h1 { color: #333; }
    .card { background: white; border-radius: 8px; padding: 20px; margin-bottom: 20px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
    table { width: 100%; border-collapse: collapse; }
    th, td { padding: 12px; text-align: left; border-bottom: 1px solid #eee; }
    th { background: #f9f9f9; color: #666; }
    .status-ok { color: #28a745; font-weight: bold; }
    .status-error { color: #dc3545; font-weight: bold; }
    .status-running { color: #007bff; font-weight: bold; }
  </style>
</head>
<body>
  <div class="container">
    <h1>AgentPrimordia Admin Dashboard</h1>
    <div class="card">
      <h2>Health</h2>
      <div id="health">Loading...</div>
    </div>
    <div class="card">
      <h2>Agents</h2>
      <div id="agents">Loading...</div>
    </div>
    <div class="card">
      <h2>Tasks</h2>
      <div id="tasks">Loading...</div>
    </div>
    <div class="card">
      <h2>Tools</h2>
      <div id="tools">Loading...</div>
    </div>
  </div>
  <script>
    const token = new URLSearchParams(location.search).get('token') || '';
    const headers = token ? { 'Authorization': 'Bearer ' + token } : {};
    async function fetchJSON(path) {
      const res = await fetch(path, { headers });
      return res.json();
    }
    async function loadAll() {
      try {
        const health = await fetchJSON('/api/health');
        document.getElementById('health').innerHTML = '<pre>' + JSON.stringify(health, null, 2) + '</pre>';
      } catch {}
      try {
        const agents = await fetchJSON('/api/agents');
        document.getElementById('agents').innerHTML = '<pre>' + JSON.stringify(agents, null, 2) + '</pre>';
      } catch {}
      try {
        const tasks = await fetchJSON('/api/tasks');
        document.getElementById('tasks').innerHTML = '<pre>' + JSON.stringify(tasks, null, 2) + '</pre>';
      } catch {}
      try {
        const tools = await fetchJSON('/api/tools');
        document.getElementById('tools').innerHTML = '<pre>' + JSON.stringify(tools, null, 2) + '</pre>';
      } catch {}
    }
    loadAll();
    setInterval(loadAll, 5000);
  </script>
</body>
</html>`;
