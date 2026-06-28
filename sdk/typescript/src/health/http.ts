// Health HTTP Handler — /healthz and /readyz HTTP endpoints
// Mirrors Go internal/health/http.go

import type { IncomingMessage, ServerResponse } from 'node:http';
import type { Debugger } from '../metrics/otel-prometheus.js';

// ===== Health Check =====

export interface HealthCheck {
  name: string;
  status: 'healthy' | 'unhealthy';
  details?: string;
}

// ===== Health Server =====

export interface HealthServerConfig {
  debugger?: Debugger;
}

export class HealthServer {
  private ready = false;
  private startupTime: Date;
  private debugger?: Debugger;

  constructor(config?: HealthServerConfig) {
    this.startupTime = new Date();
    this.debugger = config?.debugger;
  }

  /** Mark the server as ready to serve traffic. */
  setReady(ready: boolean): void {
    this.ready = ready;
  }

  /** Check if the server is ready. */
  isReady(): boolean {
    return this.ready;
  }

  /** Get uptime in seconds. */
  uptime(): number {
    return Math.floor((Date.now() - this.startupTime.getTime()) / 1000);
  }

  /** Handle an HTTP request for health endpoints. */
  async handle(req: IncomingMessage, res: ServerResponse): Promise<void> {
    const url = req.url ?? '/';
    const method = req.method ?? 'GET';

    if (method !== 'GET') {
      this.writeJSON(res, 405, { error: 'Method not allowed' });
      return;
    }

    if (url === '/healthz') {
      this.handleHealthz(res);
      return;
    }

    if (url === '/readyz') {
      this.handleReadyz(res);
      return;
    }

    if (url === '/livez') {
      // Kubernetes-style liveness probe (always 200 if process is running)
      this.writeJSON(res, 200, { status: 'ok' });
      return;
    }

    this.writeJSON(res, 404, { error: 'Not found' });
  }

  /** /healthz — always returns 200 if the process is alive. */
  private handleHealthz(res: ServerResponse): void {
    const uptimeSec = this.uptime();
    this.writeJSON(res, 200, {
      status: 'ok',
      uptime: `${uptimeSec}s`,
    });
  }

  /** /readyz — returns 200 only if the server is ready to serve traffic. */
  private handleReadyz(res: ServerResponse): void {
    if (!this.ready) {
      this.writeJSON(res, 503, {
        status: 'not ready',
        reason: 'server is not ready',
      });
      return;
    }

    const checks: HealthCheck[] = [
      { name: 'server', status: 'healthy' },
    ];

    // Optionally check debugger health
    if (this.debugger) {
      try {
        const eventCount = this.debugger.getEvents().length;
        checks.push({
          name: 'debugger',
          status: 'healthy',
          details: `${eventCount} events`,
        });
      } catch {
        checks.push({
          name: 'debugger',
          status: 'unhealthy',
          details: 'failed to query debugger',
        });
      }
    }

    const allHealthy = checks.every((c) => c.status === 'healthy');
    this.writeJSON(res, allHealthy ? 200 : 503, {
      status: allHealthy ? 'ready' : 'not ready',
      checks,
    });
  }

  private writeJSON(res: ServerResponse, status: number, data: unknown): void {
    res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' });
    res.end(JSON.stringify(data));
  }
}
