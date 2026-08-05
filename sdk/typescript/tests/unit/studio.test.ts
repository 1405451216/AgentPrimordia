/**
 * Studio 真实引擎桥接测试（v3.9-2）。
 * 验证真实 ReActAgent 运行产生的 span 写入 Inspector（Studio 显示真实运行，替换 demo）。
 */
import { describe, it, expect } from 'vitest';
import { Inspector, InspectorServer } from '../../src/debugger/server.js';
import { createStudioBridge, StudioBridge } from '../../src/debugger/studio.js';
import { ReActAgent } from '../../src/agent/react-loop.js';
import { MockProvider } from '../../src/llm/provider.js';
import { ToolRegistry } from '../../src/tools/registry.js';
import { createServer } from 'node:http';
import type { AddressInfo } from 'node:net';

describe('StudioBridge', () => {
  it('should record real agent.run span into Inspector', async () => {
    const inspector = new Inspector();
    const bridge = createStudioBridge(inspector);

    const agent = new ReActAgent({
      name: 'studio-agent',
      model: new MockProvider({ response: 'real result' }),
      toolkit: new ToolRegistry(),
      maxTurns: 3,
      otelBridge: bridge,
    });

    const resp = await agent.run('真实任务');
    expect(resp.content).toBe('real result');

    // 真实运行 span 已写入 Inspector（Studio 面板数据源）
    const traces = inspector.getTraces();
    expect(traces.length).toBeGreaterThanOrEqual(1);
    const names = traces.map((t) => t.name);
    expect(names).toContain('agent.run');
    // span 带真实时间与状态
    const runSpan = traces.find((t) => t.name === 'agent.run');
    expect(runSpan).toBeDefined();
    expect(runSpan!.status).toBe('ok');
    expect(runSpan!.startTime).toBeGreaterThan(0);
    expect(runSpan!.endTime).toBeGreaterThanOrEqual(runSpan!.startTime);
  });

  it('should mark error status on failed run', async () => {
    const inspector = new Inspector();
    const bridge = createStudioBridge(inspector);
    const agent = new ReActAgent({
      name: 'studio-fail',
      model: new MockProvider({ error: true }),
      toolkit: new ToolRegistry(),
      maxTurns: 2,
      otelBridge: bridge,
    });

    await agent.run('会失败的任务');
    const runSpan = inspector.getTraces().find((t) => t.name === 'agent.run');
    expect(runSpan).toBeDefined();
    expect(runSpan!.status).toBe('error');
  });

  it('should expose bridge methods (OTelBridge parity)', () => {
    const bridge = new StudioBridge(new Inspector());
    expect(typeof bridge.startSpan).toBe('function');
    expect(typeof bridge.endSpan).toBe('function');
    expect(typeof bridge.addEvent).toBe('function');
    expect(typeof bridge.clear).toBe('function');
  });
});

describe('InspectorServer real data', () => {
  it('should serve recorded real spans via /api/inspector/traces', async () => {
    const inspector = new Inspector();
    const agent = new ReActAgent({
      name: 'studio-http',
      model: new MockProvider({ response: 'done' }),
      toolkit: new ToolRegistry(),
      maxTurns: 2,
      otelBridge: createStudioBridge(inspector),
    });
    await agent.run('真实运行');

    const server = createServer((req, res) => {
      void new InspectorServer(inspector).handle(req, res);
    });
    await new Promise<void>((resolve) => server.listen(0, resolve));
    const port = (server.address() as AddressInfo).port;

    const res = await fetch(`http://127.0.0.1:${port}/api/inspector/traces`);
    const traces = (await res.json()) as Array<{ name: string }>;
    expect(res.status).toBe(200);
    expect(traces.some((t) => t.name === 'agent.run')).toBe(true);

    server.close();
  });
});
