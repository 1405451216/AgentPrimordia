/**
 * RunHistory 组件测试。
 */

import { describe, it, expect, vi } from 'vitest';
import {
  groupLabel,
  runStatusIcon,
  runSummary,
  groupByDate,
  RunHistoryProvider,
} from '../src/runHistory.js';
import type { StudioApi, Run } from '../src/studioApi.js';

function makeRun(overrides: Partial<Run> = {}): Run {
  return {
    id: 'r1',
    template: 'default',
    agent: 'agent-1',
    message: 'hi',
    turns: 3,
    tokens: 150,
    cost: 0.002,
    status: 'done',
    startedAt: Date.now() - 1000,
    endedAt: Date.now(),
    ...overrides,
  };
}

describe('groupLabel', () => {
  it('今天返回“今天”', () => {
    const label = groupLabel(Date.now());
    expect(label).toBe('今天');
  });
  it('昨天返回“昨天”', () => {
    const yesterday = Date.now() - 86400000;
    expect(groupLabel(yesterday)).toBe('昨天');
  });
  it('更早返回 MM-DD', () => {
    const old = Date.UTC(2026, 5, 10, 0, 0, 0);
    expect(groupLabel(old)).toBe('6-10');
  });
});

describe('runStatusIcon', () => {
  it('done 返回 check', () => {
    expect(runStatusIcon('done')).toContain('check');
  });
  it('running 返回 sync', () => {
    expect(runStatusIcon('running')).toContain('sync');
  });
  it('error 返回 error', () => {
    expect(runStatusIcon('error')).toContain('error');
  });
});

describe('runSummary', () => {
  it('包含关键字段信息', () => {
    const s = runSummary(makeRun());
    expect(s).toContain('模板: default');
    expect(s).toContain('轮次: 3');
    expect(s).toContain('Token: 150');
    expect(s).toContain('$0.0020');
  });
});

describe('groupByDate', () => {
  it('按日期分组并合并', () => {
    const runs = [makeRun({ id: 'a' }), makeRun({ id: 'b' })];
    const groups = groupByDate(runs);
    expect(groups).toHaveLength(1);
    expect(groups[0].runs).toHaveLength(2);
  });
});

describe('RunHistoryProvider', () => {
  it('refresh 重新加载数据', async () => {
    const api = {
      getRuns: vi.fn().mockResolvedValue({ items: [makeRun()], total: 1 }),
    } as unknown as StudioApi;
    const provider = new RunHistoryProvider({ api, limit: 10 });
    await provider.refresh();
    expect(api.getRuns).toHaveBeenCalledWith(10);
    expect(provider.getRuns()).toHaveLength(1);
  });

  it('refresh 失败时设置空数组', async () => {
    const api = {
      getRuns: vi.fn().mockRejectedValue(new Error('network')),
    } as unknown as StudioApi;
    const provider = new RunHistoryProvider({ api });
    await provider.refresh();
    expect(provider.getRuns()).toEqual([]);
  });

  it('removeRun 删除指定 id', () => {
    const api = {
      getRuns: vi.fn().mockResolvedValue({ items: [makeRun({ id: 'a' }), makeRun({ id: 'b' })], total: 2 }),
    } as unknown as StudioApi;
    const provider = new RunHistoryProvider({ api });
    // 手动设置内部状态（避免 async）
    (provider as any).runs = [makeRun({ id: 'a' }), makeRun({ id: 'b' })];
    provider.removeRun('a');
    expect(provider.getRuns()).toHaveLength(1);
    expect(provider.getRuns()[0].id).toBe('b');
  });

  it('getChildren 根节点返回日期分组', () => {
    const api = {} as StudioApi;
    const provider = new RunHistoryProvider({ api });
    (provider as any).runs = [makeRun()];
    const roots = provider.getChildren();
    expect(roots).toHaveLength(1);
    expect(roots[0].type).toBe('date');
  });
});
