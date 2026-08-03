/**
 * Studio 真实引擎桥接（v3.9-2）。
 *
 * 把真实 ReActAgent 运行接入 Inspector（Studio 四面板显示真实运行，
 * 替换 demo 数据）。通过实现 OTelBridge 接口，agent 运行产生的 span
 * 实时写入 Inspector；InspectorServer 的 /api/inspector/* 即展示真实 trace。
 */
import type { OTelBridgeLike } from '../metrics/otel-extended.js';
import type { OTelSpan } from '../metrics/otel-prometheus.js';
import { Inspector } from './server.js';

let seq = 0;

/** 真实引擎 → Inspector 的 OTel 桥接。 */
export class StudioBridge implements OTelBridgeLike {
  private pending = new Map<string, OTelSpan>();
  private readonly traceID: string;

  constructor(private readonly inspector: Inspector, traceID = 'studio-live') {
    this.traceID = traceID;
  }

  startSpan(name: string, attributes?: Record<string, string | number | boolean>): string {
    const id = `studio-${Date.now()}-${seq++}`;
    this.pending.set(id, {
      traceID: this.traceID,
      spanID: id,
      name,
      kind: 'internal',
      startTime: Date.now(),
      attributes: (attributes ?? {}) as Record<string, unknown>,
      status: 'unset',
      events: [],
    });
    return id;
  }

  addAttribute(spanId: string, key: string, value: string | number | boolean): void {
    const s = this.pending.get(spanId);
    if (s) s.attributes[key] = value;
  }

  addEvent(_spanId: string, _eventName: string, _attributes?: Record<string, unknown>): void {
    // OTelSpan 无 events 字段，忽略（trace 面板展示 span 级信息即可）
  }

  endSpan(spanId: string, status: 'ok' | 'error' = 'ok'): void {
    const s = this.pending.get(spanId);
    if (s) {
      s.endTime = Date.now();
      s.status = status;
      this.inspector.recordSpan(s);
      this.pending.delete(spanId);
    }
  }

  getSpans(): never[] {
    return [];
  }

  clear(): void {
    this.pending.clear();
  }
}

/**
 * 创建接入真实引擎的 Studio 桥（v3.9-2）。
 *
 * 用法（替换 demo 数据，四面板显示真实运行）：
 *   const inspector = new Inspector();
 *   const agent = new ReActAgent({ ..., otelBridge: createStudioBridge(inspector) });
 *   await agent.run('真实任务');
 *   inspector.getTraces(); // 包含 agent.run 等真实 span
 */
export function createStudioBridge(inspector: Inspector): StudioBridge {
  return new StudioBridge(inspector);
}
