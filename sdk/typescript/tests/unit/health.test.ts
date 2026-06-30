import { describe, it, expect, beforeEach } from 'vitest';
import http from 'node:http';
import { HealthServer } from '../../src/health/http.js';

describe('HealthServer', () => {
  let server: HealthServer;

  beforeEach(() => {
    server = new HealthServer();
  });

  describe('basic state', () => {
    it('starts not ready', () => {
      expect(server.isReady()).toBe(false);
    });

    it('can be set ready', () => {
      server.setReady(true);
      expect(server.isReady()).toBe(true);
    });

    it('can be set not ready again', () => {
      server.setReady(true);
      server.setReady(false);
      expect(server.isReady()).toBe(false);
    });

    it('reports uptime >= 0', () => {
      expect(server.uptime()).toBeGreaterThanOrEqual(0);
    });
  });

  describe('HTTP endpoints', () => {
    function makeRequest(method: string, url: string): Promise<{ status: number; body: Record<string, unknown> }> {
      return new Promise((resolve, reject) => {
        const req = http.request(
          { method, host: '127.0.0.1', port: 0, path: url },
          (res) => {
            let data = '';
            res.on('data', (chunk) => { data += chunk; });
            res.on('end', () => {
              resolve({ status: res.statusCode ?? 0, body: JSON.parse(data || '{}') });
            });
          },
        );
        req.on('error', reject);
        req.end();
      });
    }

    let httpServer: http.Server;

    beforeEach(() => {
      httpServer = http.createServer(async (req, res) => {
        await server.handle(req, res);
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

    it('returns 200 on /healthz', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/healthz`);
      expect(resp.status).toBe(200);
      const body = await resp.json() as { status: string; uptime: string };
      expect(body.status).toBe('ok');
      expect(body.uptime).toBeDefined();
    });

    it('returns 503 on /readyz when not ready', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/readyz`);
      expect(resp.status).toBe(503);
      const body = await resp.json() as { status: string };
      expect(body.status).toBe('not ready');
    });

    it('returns 200 on /readyz when ready', async () => {
      server.setReady(true);
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/readyz`);
      expect(resp.status).toBe(200);
      const body = await resp.json() as { status: string; checks: Array<{ name: string }> };
      expect(body.status).toBe('ready');
      expect(body.checks.length).toBeGreaterThan(0);
    });

    it('returns 200 on /livez', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/livez`);
      expect(resp.status).toBe(200);
    });

    it('returns 404 on unknown path', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/unknown`);
      expect(resp.status).toBe(404);
    });

    it('returns 405 on POST', async () => {
      const port = await listen();
      const resp = await fetch(`http://127.0.0.1:${port}/healthz`, { method: 'POST' });
      expect(resp.status).toBe(405);
    });
  });
});
