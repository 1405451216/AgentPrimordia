/**
 * 官方 OpenTelemetry SDK 桥接（v3.7-1）。
 *
 * 将框架内部的 OTelBridge 接口委托给官方 `@opentelemetry/api`：
 * - startSpan → tracer.startSpan()（真实 OTel Span）
 * - endSpan   → span.setStatus + span.end()
 * - addEvent  → span.addEvent()
 * - 支持 W3C TraceContext 传播（官方 API 原生支持）
 *
 * `@opentelemetry/api` 为可选 peer 依赖；未安装时该桥接不可用（返回 null）。
 */
import { trace, SpanStatusCode } from '@opentelemetry/api';
import type { OTelSpan } from './otel-extended.js';

/** 官方 OTel 桥接：把框架 span 委托给 @opentelemetry/api 的全局 Tracer。 */
export class OfficialOTelBridge {
  private spans: OTelSpan[] = [];
  private activeSpans: Map<string, unknown> = new Map();
  private snapshots: Map<string, OTelSpan> = new Map();

  /** 创建桥接（未安装 @opentelemetry/api 时返回 null）。 */
  static create(): OfficialOTelBridge | null {
    try {
      // 触发依赖解析；未安装时抛错 → 返回 null（优雅降级）
      void trace.getTracer('agentprimordia');
      return new OfficialOTelBridge();
    } catch {
      return null;
    }
  }

  startSpan(name: string, attributes?: Record<string, string | number | boolean>): string {
    const tracer = trace.getTracer('agentprimordia', '3.2.0');
    const span = tracer.startSpan(name, { attributes });
    const id = `otel-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    this.activeSpans.set(id, span);
    const snap: OTelSpan = {
      name,
      startTime: Date.now(),
      attributes: attributes ?? {},
      events: [],
      status: 'unset',
    };
    this.snapshots.set(id, snap);
    this.spans.push(snap);
    return id;
  }

  addAttribute(spanId: string, key: string, value: string | number | boolean): void {
    const span = this.activeSpans.get(spanId) as { setAttribute(k: string, v: unknown): void } | undefined;
    if (span) {
      span.setAttribute(key, value);
    }
  }

  addEvent(spanId: string, eventName: string, attributes?: Record<string, unknown>): void {
    const span = this.activeSpans.get(spanId) as { addEvent(n: string, a?: unknown): void } | undefined;
    if (span) {
      span.addEvent(eventName, attributes);
    }
    const snap = this.snapshots.get(spanId);
    if (snap) {
      snap.events.push({ name: eventName, time: Date.now(), attributes });
    }
  }

  endSpan(spanId: string, status: 'ok' | 'error' = 'ok'): void {
    const span = this.activeSpans.get(spanId) as { setStatus(s: SpanStatusCode): void; end(): void } | undefined;
    if (span) {
      span.setStatus(status === 'error' ? SpanStatusCode.ERROR : SpanStatusCode.OK);
      span.end();
      this.activeSpans.delete(spanId);
    }
    const snap = this.snapshots.get(spanId);
    if (snap) {
      snap.endTime = Date.now();
      snap.status = status;
      this.snapshots.delete(spanId);
    }
  }

  getSpans(): OTelSpan[] {
    return [...this.spans];
  }

  clear(): void {
    this.spans = [];
    this.activeSpans.clear();
    this.snapshots.clear();
  }
}
