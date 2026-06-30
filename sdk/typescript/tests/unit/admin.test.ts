import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import http from 'node:http';
import { AdminHandler } from '../../src/admin/handler.js';
import type { AdminTaskInfo, AdminPoolStats } from '../../src/admin/handler.js';

describe('AdminHandler', () => {
  let handler: AdminHandler;
  let httpServer: http.Server;
  const API_TOKEN = 'test-secret-token';

  beforeEach(() => {
    const stats: AdminPoolStats = {
      totalTasks: 10,
      completedTasks: 8,
      failedTasks: 1,
      runningTasks: 1,
      queuedTasks: 0,
      maxConcurrency: 5,
      activeConcurrency: 1,
    };
    const tasks: AdminTaskInfo[] = [
      { id: 't1', title: 'Task 1', status: 'completed', duration: '1.2s' },
      { id: 't2', title: 'Task 2', status: 'running', duration: '0.5s' },
    ];

    handler = new AdminHandler({
      apiToken: API_TOKEN,
      getStats: () => stats,
      getTasks: () => tasks,
      getAgents: () => ({ 'agent-1': 'running', 'agent-2': 'idle' }),
    });

    httpServer = http.createServer(async (req, res) => {
      await handler.handle(req, res);
    });
  });

  afterEach(() => {
    httpServer.close();
  });

  function listen(): Promise<number> {
    return new Promise((resolve) => {
      httpServer.listen(0, '127.0.0.1', () => {
        const addr = httpServer.address();
        resolve(typeof addr === 'object' && addr ? addr.port : 0);
      });
    });
  }

  function authHeader(): Record<string, string> {
    return { Authorization: `Bearer ${API_TOKEN}` };
  }

  describe('public endpoints', () => {
    it('GET /api/health returns 200 without auth', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/api/health`);
      expect(resp.status).toBe(200);
      const body = await resp.json() as { status: string };
      expect(body.status).toBe('healthy');
    });

    it('GET / returns HTML index', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/`);
      expect(resp.status).toBe(200);
      const text = await resp.text();
      expect(text).toContain('html');
    });
  });

  describe('auth protection', () => {
    it('returns 401 without auth header', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/api/agents`);
      expect(resp.status).toBe(401);
    });

    it('returns 401 with wrong token', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/api/agents`, {
        headers: { Authorization: 'Bearer wrong-token' },
      });
      expect(resp.status).toBe(401);
    });

    it('returns 200 with correct token', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/api/agents`, {
        headers: authHeader(),
      });
      expect(resp.status).toBe(200);
    });
  });

  describe('protected endpoints', () => {
    it('GET /api/agents returns agent list', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/api/agents`, { headers: authHeader() });
      expect(resp.status).toBe(200);
      const body = await resp.json() as Record<string, string>;
      expect(Object.keys(body).length).toBeGreaterThan(0);
    });

    it('GET /api/stats returns pool stats', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/api/stats`, { headers: authHeader() });
      expect(resp.status).toBe(200);
      const body = await resp.json() as { totalTasks: number };
      expect(body.totalTasks).toBe(10);
    });

    it('GET /api/tasks returns task list', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/api/tasks`, { headers: authHeader() });
      expect(resp.status).toBe(200);
      const body = await resp.json() as Array<{ id: string }>;
      expect(body.length).toBe(2);
    });

    it('GET /api/system returns system info', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/api/system`, { headers: authHeader() });
      expect(resp.status).toBe(200);
      const body = await resp.json() as { node_version: string; platform: string };
      expect(body.node_version).toBeDefined();
      expect(body.platform).toBeDefined();
    });
  });

  describe('prefix routing', () => {
    it('GET /api/agents/:id returns specific agent', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/api/agents/agent-1`, { headers: authHeader() });
      expect(resp.status).toBe(200);
      const body = await resp.json() as { id: string; status: string };
      expect(body.id).toBe('agent-1');
    });

    it('GET /api/agents/unknown returns 404', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/api/agents/unknown`, { headers: authHeader() });
      expect(resp.status).toBe(404);
    });
  });

  describe('fallback routing', () => {
    it('GET / matches index page (catch-all)', async () => {
      const port = await listen();
      // GET / is a catch-all route that matches any unmatched path
      const resp = await fetch(`http://127.0.0.1:${port}/api/unknown`, { headers: authHeader() });
      expect(resp.status).toBe(200);
    });
  });
});
