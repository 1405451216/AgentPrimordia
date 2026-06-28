import type { CostTracker, CostRecord } from '../agent/request-id.js';

// ===== Metrics Types =====

export interface MetricSample {
  name: string;
  value: number;
  labels: Record<string, string>;
  timestamp: Date;
}

export interface MetricDefinition {
  name: string;
  help: string;
  type: 'counter' | 'gauge' | 'histogram';
  labelNames: string[];
}

// ===== Metrics Registry (Prometheus-compatible) =====

export class MetricsRegistry {
  private counters: Map<string, number> = new Map();
  private gauges: Map<string, number> = new Map();
  private histograms: Map<string, number[]> = new Map();
  private definitions: Map<string, MetricDefinition> = new Map();
  private labelMaps: Map<string, Record<string, string>> = new Map();

  registerCounter(name: string, help: string, labelNames: string[] = []): void {
    this.definitions.set(name, { name, help, type: 'counter', labelNames });
    this.counters.set(name, 0);
  }

  registerGauge(name: string, help: string, labelNames: string[] = []): void {
    this.definitions.set(name, { name, help, type: 'gauge', labelNames });
    this.gauges.set(name, 0);
  }

  registerHistogram(name: string, help: string, labelNames: string[] = []): void {
    this.definitions.set(name, { name, help, type: 'histogram', labelNames });
    this.histograms.set(name, []);
  }

  incCounter(name: string, value: number = 1, labels: Record<string, string> = {}): void {
    const current = this.counters.get(name) ?? 0;
    this.counters.set(name, current + value);
    this.labelMaps.set(name, labels);
  }

  setGauge(name: string, value: number, labels: Record<string, string> = {}): void {
    this.gauges.set(name, value);
    this.labelMaps.set(name, labels);
  }

  observe(name: string, value: number, labels: Record<string, string> = {}): void {
    const histogram = this.histograms.get(name) ?? [];
    histogram.push(value);
    this.histograms.set(name, histogram);
    this.labelMaps.set(name, labels);
  }

  /** Export metrics in Prometheus text format. */
  export(): string {
    const lines: string[] = [];

    for (const [name, def] of this.definitions) {
      lines.push(`# HELP ${name} ${def.help}`);
      lines.push(`# TYPE ${name} ${def.type}`);

      const labels = this.labelMaps.get(name);
      const labelStr = labels && Object.keys(labels).length > 0
        ? '{' + Object.entries(labels).map(([k, v]) => `${k}="${v}"`).join(',') + '}'
        : '';

      switch (def.type) {
        case 'counter':
          lines.push(`${name}${labelStr} ${this.counters.get(name) ?? 0}`);
          break;
        case 'gauge':
          lines.push(`${name}${labelStr} ${this.gauges.get(name) ?? 0}`);
          break;
        case 'histogram':
          const values = this.histograms.get(name) ?? [];
          if (values.length > 0) {
            const sum = values.reduce((a, b) => a + b, 0);
            const count = values.length;
            lines.push(`${name}_count${labelStr} ${count}`);
            lines.push(`${name}_sum${labelStr} ${sum}`);

            // Buckets
            const buckets = [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10];
            for (const bucket of buckets) {
              const countBelow = values.filter((v) => v <= bucket).length;
              lines.push(`${name}_bucket{le="${bucket}"} ${countBelow}`);
            }
            lines.push(`${name}_bucket{le="+Inf"} ${count}`);
          }
          break;
      }
      lines.push('');
    }

    return lines.join('\n');
  }

  /** Get all metrics as samples. */
  samples(): MetricSample[] {
    const samples: MetricSample[] = [];

    for (const [name, def] of this.definitions) {
      const labels = this.labelMaps.get(name) ?? {};
      const timestamp = new Date();

      switch (def.type) {
        case 'counter':
          samples.push({ name, value: this.counters.get(name) ?? 0, labels, timestamp });
          break;
        case 'gauge':
          samples.push({ name, value: this.gauges.get(name) ?? 0, labels, timestamp });
          break;
        case 'histogram':
          const values = this.histograms.get(name) ?? [];
          if (values.length > 0) {
            const avg = values.reduce((a, b) => a + b, 0) / values.length;
            samples.push({ name: `${name}_avg`, value: avg, labels, timestamp });
            samples.push({ name: `${name}_count`, value: values.length, labels, timestamp });
          }
          break;
      }
    }

    return samples;
  }
}

// ===== Agent Metrics (Pre-configured metrics for agent monitoring) =====

export class AgentMetrics {
  private registry: MetricsRegistry;

  constructor() {
    this.registry = new MetricsRegistry();

    // Register standard agent metrics
    this.registry.registerCounter('agent_requests_total', 'Total number of agent requests', ['agent']);
    this.registry.registerCounter('agent_errors_total', 'Total number of agent errors', ['agent']);
    this.registry.registerCounter('agent_tool_calls_total', 'Total number of tool calls', ['agent', 'tool']);
    this.registry.registerCounter('agent_llm_calls_total', 'Total number of LLM API calls', ['agent', 'provider']);
    this.registry.registerGauge('agent_active_sessions', 'Number of active sessions', ['agent']);
    this.registry.registerHistogram('agent_request_duration_seconds', 'Agent request duration in seconds', ['agent']);
    this.registry.registerHistogram('agent_llm_latency_seconds', 'LLM API call latency in seconds', ['agent', 'provider']);
    this.registry.registerHistogram('agent_tool_duration_seconds', 'Tool execution duration in seconds', ['agent', 'tool']);
    this.registry.registerGauge('agent_cost_usd', 'Total cost in USD', ['agent']);
    this.registry.registerGauge('agent_tokens_total', 'Total tokens used', ['agent', 'type']);
  }

  recordRequest(agent: string): void {
    this.registry.incCounter('agent_requests_total', 1, { agent });
  }

  recordError(agent: string): void {
    this.registry.incCounter('agent_errors_total', 1, { agent });
  }

  recordToolCall(agent: string, tool: string, durationSeconds: number): void {
    this.registry.incCounter('agent_tool_calls_total', 1, { agent, tool });
    this.registry.observe('agent_tool_duration_seconds', durationSeconds, { agent, tool });
  }

  recordLLMCall(agent: string, provider: string, latencySeconds: number): void {
    this.registry.incCounter('agent_llm_calls_total', 1, { agent, provider });
    this.registry.observe('agent_llm_latency_seconds', latencySeconds, { agent, provider });
  }

  setActiveSessions(agent: string, count: number): void {
    this.registry.setGauge('agent_active_sessions', count, { agent });
  }

  recordCost(agent: string, costUSD: number): void {
    this.registry.setGauge('agent_cost_usd', costUSD, { agent });
  }

  recordTokens(agent: string, type: 'input' | 'output', count: number): void {
    this.registry.setGauge('agent_tokens_total', count, { agent, type });
  }

  recordRequestDuration(agent: string, durationSeconds: number): void {
    this.registry.observe('agent_request_duration_seconds', durationSeconds, { agent });
  }

  getRegistry(): MetricsRegistry {
    return this.registry;
  }

  export(): string {
    return this.registry.export();
  }
}

// ===== Prometheus HTTP Endpoint =====

export class PrometheusExporter {
  private metrics: AgentMetrics;
  private port: number;
  private server?: import('node:http').Server;

  constructor(metrics: AgentMetrics, port: number = 9090) {
    this.metrics = metrics;
    this.port = port;
  }

  async start(): Promise<void> {
    const http = await import('node:http');
    this.server = http.createServer((req, res) => {
      if (req.url === '/metrics') {
        res.writeHead(200, { 'Content-Type': 'text/plain; version=0.0.4' });
        res.end(this.metrics.export());
      } else {
        res.writeHead(404);
        res.end('Not Found');
      }
    });

    return new Promise((resolve) => {
      this.server!.listen(this.port, () => resolve());
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
}

// ===== OpenTelemetry Tracer =====

export interface OTelSpan {
  traceID: string;
  spanID: string;
  parentSpanID?: string;
  name: string;
  kind: 'internal' | 'client' | 'server';
  startTime: number;
  endTime?: number;
  attributes: Record<string, unknown>;
  status: 'ok' | 'error' | 'unset';
  statusDescription?: string;
  events: { name: string; timestamp: number; attributes?: Record<string, unknown> }[];
}

export class OTelTracer {
  private spans: OTelSpan[] = [];
  private enabled: boolean;

  constructor(enabled: boolean = true) {
    this.enabled = enabled;
  }

  start(name: string, kind: 'internal' | 'client' | 'server' = 'internal', opts?: { parent?: string; attributes?: Record<string, unknown> }): OTelSpan & { end: (status?: 'ok' | 'error', description?: string) => void } {
    const span: OTelSpan = {
      traceID: this.generateTraceID(),
      spanID: this.generateSpanID(),
      parentSpanID: opts?.parent,
      name,
      kind,
      startTime: Date.now(),
      attributes: opts?.attributes ?? {},
      status: 'unset',
      events: [],
    };

    if (this.enabled) {
      this.spans.push(span);
    }

    return {
      ...span,
      end: (status?: 'ok' | 'error', description?: string) => {
        span.endTime = Date.now();
        span.status = status ?? 'ok';
        span.statusDescription = description;
      },
      setAttribute: (key: string, value: unknown) => { span.attributes[key] = value; },
      setAttributes: (attrs: Record<string, unknown>) => { Object.assign(span.attributes, attrs); },
      setStatus: (status: 'ok' | 'error', description?: string) => {
        span.status = status;
        span.statusDescription = description;
      },
      addEvent: (name: string, attributes?: Record<string, unknown>) => {
        span.events.push({ name, timestamp: Date.now(), attributes });
      },
    } as OTelSpan & { end: (status?: 'ok' | 'error', description?: string) => void };
  }

  getSpans(): OTelSpan[] {
    return [...this.spans];
  }

  clear(): void {
    this.spans = [];
  }

  /** Export spans in OTLP JSON format. */
  exportJSON(): string {
    return JSON.stringify({
      resourceSpans: [{
        resource: { attributes: {} },
        scopeSpans: [{
          scope: { name: 'agentprimordia-ts' },
          spans: this.spans.map((s) => ({
            traceId: s.traceID,
            spanId: s.spanID,
            parentSpanId: s.parentSpanID,
            name: s.name,
            kind: s.kind === 'internal' ? 0 : s.kind === 'client' ? 2 : 1,
            startTimeUnixNano: (s.startTime * 1_000_000).toString(),
            endTimeUnixNano: s.endTime ? (s.endTime * 1_000_000).toString() : '0',
            attributes: Object.entries(s.attributes).map(([k, v]) => ({ key: k, value: { stringValue: String(v) } })),
            status: { code: s.status === 'ok' ? 1 : s.status === 'error' ? 2 : 0, message: s.statusDescription ?? '' },
          })),
        }],
      }],
    }, null, 2);
  }

  private generateTraceID(): string {
    return Array.from({ length: 32 }, () => Math.floor(Math.random() * 16).toString(16)).join('');
  }

  private generateSpanID(): string {
    return Array.from({ length: 16 }, () => Math.floor(Math.random() * 16).toString(16)).join('');
  }
}

// ===== Debugger / Inspector =====

export interface DebugEvent {
  type: 'llm_call' | 'tool_call' | 'reasoning' | 'error' | 'info';
  timestamp: Date;
  data: Record<string, unknown>;
}

export class Debugger {
  private events: DebugEvent[] = [];
  private enabled: boolean = false;
  private maxEvents: number;

  constructor(maxEvents: number = 10_000) {
    this.maxEvents = maxEvents;
  }

  enable(): void { this.enabled = true; }
  disable(): void { this.enabled = false; }

  log(type: DebugEvent['type'], data: Record<string, unknown>): void {
    if (!this.enabled) return;
    this.events.push({ type, timestamp: new Date(), data });
    if (this.events.length > this.maxEvents) {
      this.events.shift();
    }
  }

  getEvents(filter?: { type?: DebugEvent['type']; since?: Date }): DebugEvent[] {
    let result = [...this.events];
    if (filter?.type) result = result.filter((e) => e.type === filter.type);
    if (filter?.since) result = result.filter((e) => e.timestamp > filter.since!);
    return result;
  }

  clear(): void {
    this.events = [];
  }

  /** Generate a debug report. */
  report(): string {
    const lines: string[] = ['=== Debug Report ==='];
    lines.push(`Total events: ${this.events.length}`);
    lines.push('');

    const byType: Record<string, number> = {};
    for (const event of this.events) {
      byType[event.type] = (byType[event.type] ?? 0) + 1;
    }
    for (const [type, count] of Object.entries(byType)) {
      lines.push(`${type}: ${count}`);
    }

    lines.push('');
    lines.push('=== Recent Events ===');
    const recent = this.events.slice(-20);
    for (const event of recent) {
      lines.push(`[${event.timestamp.toISOString()}] ${event.type}: ${JSON.stringify(event.data)}`);
    }

    return lines.join('\n');
  }
}

// ===== Health Checker =====

export interface HealthStatus {
  status: 'healthy' | 'degraded' | 'unhealthy';
  checks: { name: string; status: 'pass' | 'fail'; message?: string; latency?: number }[];
  timestamp: Date;
}

export class HealthChecker {
  private checks: Map<string, () => Promise<{ healthy: boolean; message?: string }>> = new Map();

  register(name: string, check: () => Promise<{ healthy: boolean; message?: string }>): void {
    this.checks.set(name, check);
  }

  async check(): Promise<HealthStatus> {
    const results: { name: string; status: 'pass' | 'fail'; message?: string; latency?: number }[] = [];
    let allHealthy = true;
    let anyFail = false;

    for (const [name, check] of this.checks) {
      const start = Date.now();
      try {
        const result = await check();
        const latency = Date.now() - start;
        results.push({
          name,
          status: result.healthy ? 'pass' : 'fail',
          message: result.message,
          latency,
        });
        if (!result.healthy) {
          anyFail = true;
        }
      } catch (err) {
        results.push({
          name,
          status: 'fail',
          message: (err as Error).message,
          latency: Date.now() - start,
        });
        anyFail = true;
      }
    }

    // If all fail, unhealthy; if some fail, degraded; else healthy
    const failedCount = results.filter((r) => r.status === 'fail').length;
    const status: 'healthy' | 'degraded' | 'unhealthy' =
      failedCount === 0 ? 'healthy' :
      failedCount === results.length ? 'unhealthy' :
      'degraded';

    return { status, checks: results, timestamp: new Date() };
  }
}
