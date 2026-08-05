/**
 * 官方 OpenTelemetry SDK 桥接测试（v3.7-1）。
 * 验证 OfficialOTelBridge 委托官方 @opentelemetry/api 的契约行为，
 * 以及接入 ReActAgent 后 span 正常产生。
 */
import { describe, it, expect } from 'vitest';
import { SpanStatusCode } from '@opentelemetry/api';
import { OfficialOTelBridge } from '../../src/metrics/otel-official.js';
import { ReActAgent } from '../../src/agent/react-loop.js';
import { MockProvider } from '../../src/llm/provider.js';
import { ToolRegistry } from '../../src/tools/registry.js';

describe('OfficialOTelBridge', () => {
  it('create should return an instance when @opentelemetry/api is installed', () => {
    const bridge = OfficialOTelBridge.create();
    expect(bridge).not.toBeNull();
  });

  it('should delegate span lifecycle to the official API', () => {
    const bridge = OfficialOTelBridge.create();
    expect(bridge).not.toBeNull();
    if (!bridge) return;

    const id = bridge.startSpan('test.span', { 'agent.name': 'a' });
    expect(id).toBeTruthy();

    bridge.addAttribute(id, 'custom', 42);
    bridge.addEvent(id, 'test.event', { ok: true });
    bridge.endSpan(id, 'ok');

    const spans = bridge.getSpans();
    expect(spans.length).toBe(1);
    expect(spans[0].name).toBe('test.span');
    expect(spans[0].status).toBe('ok');
    expect(spans[0].endTime).toBeDefined();
  });

  it('should mark error status on endSpan with error', () => {
    const bridge = OfficialOTelBridge.create();
    expect(bridge).not.toBeNull();
    if (!bridge) return;

    const id = bridge.startSpan('fail.span');
    bridge.endSpan(id, 'error');
    expect(bridge.getSpans()[0].status).toBe('error');
  });

  it('should clear recorded spans', () => {
    const bridge = OfficialOTelBridge.create();
    expect(bridge).not.toBeNull();
    if (!bridge) return;

    const id = bridge.startSpan('a');
    bridge.endSpan(id, 'ok');
    expect(bridge.getSpans().length).toBe(1);
    bridge.clear();
    expect(bridge.getSpans().length).toBe(0);
  });

  it('should be usable as ReActAgent otelBridge (agent.run span recorded)', async () => {
    const bridge = OfficialOTelBridge.create();
    expect(bridge).not.toBeNull();
    if (!bridge) return;

    const provider = new MockProvider({ response: 'completed' });
    const agent = new ReActAgent({
      name: 'otel-agent',
      model: provider,
      toolkit: new ToolRegistry(),
      maxTurns: 3,
      otelBridge: bridge,
    });

    const resp = await agent.run('hello');
    expect(resp.content).toBeTruthy();

    // 运行应产生 agent.run span
    const names = bridge.getSpans().map((s) => s.name);
    expect(names).toContain('agent.run');
  });
});

describe('OTelBridge interface parity', () => {
  it('should expose the same method shape as the framework OTelBridge', () => {
    const bridge = OfficialOTelBridge.create();
    expect(bridge).not.toBeNull();
    if (!bridge) return;
    for (const m of ['startSpan', 'endSpan', 'addEvent', 'addAttribute', 'getSpans', 'clear'] as const) {
      expect(typeof (bridge as unknown as Record<string, unknown>)[m]).toBe('function');
    }
  });
});

describe('SpanStatusCode import', () => {
  it('should expose ERROR and OK constants', () => {
    expect(SpanStatusCode.ERROR).toBe(2);
    expect(SpanStatusCode.OK).toBe(1);
  });
});
