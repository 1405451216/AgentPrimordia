// ===== OTel Extensions: Baggage, Bridge, OTLP Exporter =====

// ===== Baggage =====

export interface BaggageEntry {
  key: string;
  value: string;
  metadata?: string;
}

export class Baggage {
  private entries: Map<string, BaggageEntry> = new Map();

  static create(): Baggage { return new Baggage(); }

  set(key: string, value: string, metadata?: string): this {
    this.entries.set(key, { key, value, metadata });
    return this;
  }

  get(key: string): BaggageEntry | undefined {
    return this.entries.get(key);
  }

  remove(key: string): this {
    this.entries.delete(key);
    return this;
  }

  getAll(): BaggageEntry[] {
    return Array.from(this.entries.values());
  }

  has(key: string): boolean {
    return this.entries.has(key);
  }

  clear(): this {
    this.entries.clear();
    return this;
  }

  /** Serialize to W3C Baggage header format. */
  toHeader(): string {
    return this.getAll()
      .map(e => `${encodeURIComponent(e.key)}=${encodeURIComponent(e.value)}`)
      .join(',');
  }

  /** Parse from W3C Baggage header. */
  static fromHeader(header: string): Baggage {
    const baggage = new Baggage();
    if (!header) return baggage;
    const entries = header.split(',');
    for (const entry of entries) {
      const [keyValue, ...rest] = entry.split(';');
      const [key, value] = keyValue.trim().split('=');
      if (key && value) {
        baggage.set(decodeURIComponent(key), decodeURIComponent(value), rest.join(';'));
      }
    }
    return baggage;
  }
}

// ===== Baggage Propagation =====

export class BaggagePropagator {
  headerName = 'baggage';

  inject(context: { baggage: Baggage }, headers: Record<string, string>): void {
    const header = context.baggage.toHeader();
    if (header) headers[this.headerName] = header;
  }

  extract(headers: Record<string, string>): Baggage {
    const header = headers[this.headerName] ?? headers['Baggage'] ?? '';
    return Baggage.fromHeader(header);
  }
}

// ===== OTel Bridge — connects framework metrics to OTel =====

/** OTel 桥接的结构化接口（v3.9-2 Studio 桥接等可结构实现）。 */
export interface OTelBridgeLike {
  startSpan(name: string, attributes?: Record<string, string | number | boolean>): string;
  addAttribute(spanId: string, key: string, value: string | number | boolean): void;
  addEvent(spanId: string, eventName: string, attributes?: Record<string, unknown>): void;
  endSpan(spanId: string, status?: 'ok' | 'error'): void;
  getSpans(): OTelSpan[];
  clear(): void;
}

export interface OTelSpan {
  name: string;
  startTime: number;
  endTime?: number;
  attributes: Record<string, string | number | boolean>;
  events: Array<{ name: string; time: number; attributes?: Record<string, unknown> }>;
  status: 'ok' | 'error' | 'unset';
}

export class OTelBridge implements OTelBridgeLike {
  private spans: OTelSpan[] = [];
  private activeSpans: Map<string, OTelSpan> = new Map();

  startSpan(name: string, attributes?: Record<string, string | number | boolean>): string {
    const spanId = `span-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const span: OTelSpan = {
      name,
      startTime: Date.now(),
      attributes: attributes ?? {},
      events: [],
      status: 'unset',
    };
    this.activeSpans.set(spanId, span);
    return spanId;
  }

  addAttribute(spanId: string, key: string, value: string | number | boolean): void {
    const span = this.activeSpans.get(spanId);
    if (span) span.attributes[key] = value;
  }

  addEvent(spanId: string, eventName: string, attributes?: Record<string, unknown>): void {
    const span = this.activeSpans.get(spanId);
    if (span) {
      span.events.push({ name: eventName, time: Date.now(), attributes });
    }
  }

  endSpan(spanId: string, status: 'ok' | 'error' = 'ok'): void {
    const span = this.activeSpans.get(spanId);
    if (span) {
      span.endTime = Date.now();
      span.status = status;
      this.activeSpans.delete(spanId);
      this.spans.push(span);
    }
  }

  getSpans(): OTelSpan[] {
    return [...this.spans];
  }

  clear(): void {
    this.spans = [];
    this.activeSpans.clear();
  }

  /** Export spans in OTLP JSON format. */
  exportOTLP(): Record<string, unknown> {
    return {
      resourceSpans: [{
        resource: { attributes: [{ key: 'service.name', value: { stringValue: 'agentprimordia-ts' } }] },
        scopeSpans: [{
          scope: { name: 'agentprimordia' },
          spans: this.spans.map(s => ({
            traceId: '00000000000000000000000000000000',
            spanId: '0000000000000000',
            name: s.name,
            kind: 0,
            startTimeUnixNano: String(s.startTime * 1_000_000),
            endTimeUnixNano: String((s.endTime ?? Date.now()) * 1_000_000),
            attributes: Object.entries(s.attributes).map(([k, v]) => ({
              key: k,
              value: typeof v === 'string' ? { stringValue: v } :
                    typeof v === 'number' ? { doubleValue: v } :
                    { boolValue: v }
            })),
            status: { code: s.status === 'ok' ? 1 : s.status === 'error' ? 2 : 0 },
          })),
        }],
      }],
    };
  }
}

// ===== OTLP Exporter =====

export interface OTLPExporterConfig {
  endpoint: string;
  headers?: Record<string, string>;
  timeoutMs?: number;
}

export class OTLPExporter {
  private config: OTLPExporterConfig;
  private batch: OTelSpan[] = [];
  private batchSize: number;
  private flushTimer?: NodeJS.Timeout;

  constructor(config: OTLPExporterConfig, batchSize: number = 100) {
    this.config = {
      timeoutMs: 5000,
      ...config,
    };
    this.batchSize = batchSize;
  }

  addSpan(span: OTelSpan): void {
    this.batch.push(span);
    if (this.batch.length >= this.batchSize) {
      this.flush().catch((err) => {
        console.error('OTLPExporter addSpan flush failed:', err);
      });
    }
  }

  async flush(): Promise<void> {
    if (this.batch.length === 0) return;

    const spans = [...this.batch];
    this.batch = [];

    const payload = {
      resourceSpans: [{
        resource: { attributes: [{ key: 'service.name', value: { stringValue: 'agentprimordia-ts' } }] },
        scopeSpans: [{
          scope: { name: 'agentprimordia' },
          spans: spans.map(s => ({
            name: s.name,
            startTimeUnixNano: String(s.startTime * 1_000_000),
            endTimeUnixNano: String((s.endTime ?? Date.now()) * 1_000_000),
            attributes: Object.entries(s.attributes).map(([k, v]) => ({
              key: k,
              value: typeof v === 'string' ? { stringValue: v } :
                    typeof v === 'number' ? { doubleValue: v } :
                    { boolValue: v }
            })),
          })),
        }],
      }],
    };

    try {
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), this.config.timeoutMs);
      await fetch(this.config.endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...this.config.headers },
        body: JSON.stringify(payload),
        signal: controller.signal,
      });
      clearTimeout(timer);
    } catch {
      // Re-add spans to batch on failure
      this.batch.unshift(...spans);
    }
  }

  startAutoFlush(intervalMs: number = 10000): () => void {
    this.flushTimer = setInterval(() => this.flush().catch(() => {}), intervalMs);
    return () => { if (this.flushTimer) clearInterval(this.flushTimer); };
  }

  async shutdown(): Promise<void> {
    if (this.flushTimer) clearInterval(this.flushTimer);
    await this.flush();
  }
}

// ===== Metric Export =====

export interface MetricDataPoint {
  name: string;
  value: number;
  labels: Record<string, string>;
  timestamp: number;
  type: 'counter' | 'gauge' | 'histogram';
}

export class MetricExporter {
  private config: OTLPExporterConfig;
  private metrics: MetricDataPoint[] = [];

  constructor(config: OTLPExporterConfig) {
    this.config = { timeoutMs: 5000, ...config };
  }

  record(metric: MetricDataPoint): void {
    this.metrics.push(metric);
  }

  async flush(): Promise<void> {
    if (this.metrics.length === 0) return;

    const metrics = [...this.metrics];
    this.metrics = [];

    const payload = {
      resourceMetrics: [{
        resource: { attributes: [{ key: 'service.name', value: { stringValue: 'agentprimordia-ts' } }] },
        scopeMetrics: [{
          scope: { name: 'agentprimordia' },
          metrics: metrics.map(m => ({
            name: m.name,
            unit: '1',
            gauge: m.type === 'gauge' ? {
              dataPoints: [{
                asDouble: m.value,
                timeUnixNano: String(m.timestamp * 1_000_000),
                attributes: Object.entries(m.labels).map(([k, v]) => ({
                  key: k, value: { stringValue: v }
                })),
              }],
            } : undefined,
            sum: m.type === 'counter' ? {
              dataPoints: [{
                asDouble: m.value,
                timeUnixNano: String(m.timestamp * 1_000_000),
                attributes: Object.entries(m.labels).map(([k, v]) => ({
                  key: k, value: { stringValue: v }
                })),
              }],
              aggregationTemporality: 2,
              isMonotonic: true,
            } : undefined,
          })),
        }],
      }],
    };

    try {
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), this.config.timeoutMs);
      await fetch(this.config.endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...this.config.headers },
        body: JSON.stringify(payload),
        signal: controller.signal,
      });
      clearTimeout(timer);
    } catch {
      this.metrics.unshift(...metrics);
    }
  }
}
