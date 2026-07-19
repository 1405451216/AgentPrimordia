/**
 * StatusBar 组件测试。
 */

import { describe, it, expect } from 'vitest';
import {
  statusIcon,
  formatTokens,
  formatCost,
  statusText,
  buildStatusBarTitle,
  StatusBarManager,
  type StatusBarData,
} from '../src/statusBar.js';

describe('statusIcon', () => {
  it('idle 返回暂停图标', () => {
    expect(statusIcon('idle')).toContain('debug-pause');
  });
  it('running 返回旋转图标', () => {
    expect(statusIcon('running')).toContain('sync~spin');
  });
  it('error 返回错误图标', () => {
    expect(statusIcon('error')).toContain('error');
  });
});

describe('formatTokens', () => {
  it('小于 1000 直接显示', () => {
    expect(formatTokens(999)).toBe('999');
  });
  it('大于 1000 用 k 单位', () => {
    expect(formatTokens(1500)).toBe('1.5k');
    expect(formatTokens(100000)).toBe('100.0k');
  });
});

describe('formatCost', () => {
  it('格式化为美元', () => {
    expect(formatCost(0.0012)).toBe('$0.0012');
    expect(formatCost(1.5)).toBe('$1.5000');
  });
});

describe('statusText', () => {
  it('返回中文标签', () => {
    expect(statusText('idle')).toBe('空闲');
    expect(statusText('running')).toBe('运行中');
    expect(statusText('error')).toBe('错误');
  });
});

describe('buildStatusBarTitle', () => {
  it('包含 agent 名、token、成本', () => {
    const data: StatusBarData = { agentName: 'my-agent', status: 'running', tokens: 1500, cost: 0.002 };
    const title = buildStatusBarTitle(data);
    expect(title).toContain('my-agent');
    expect(title).toContain('1.5k');
    expect(title).toContain('$0.0020');
  });
});

describe('StatusBarManager', () => {
  it('create 后 getData 返回初始数据', () => {
    const mgr = new StatusBarManager();
    const fakeItem: any = { text: '', tooltip: '', show() {}, dispose() {} };
    const deps = {
      createItem: () => fakeItem,
    };
    mgr.create(deps);
    const data = mgr.getData();
    expect(data.status).toBe('idle');
    expect(data.tokens).toBe(0);
    mgr.dispose();
  });

  it('update 更新数据', () => {
    const mgr = new StatusBarManager();
    const fakeItem: any = { text: '', tooltip: '', show() {}, dispose() {} };
    mgr.create({ createItem: () => fakeItem });
    mgr.update({ agentName: 'a1', status: 'running', tokens: 100, cost: 0.001 });
    const data = mgr.getData();
    expect(data.agentName).toBe('a1');
    expect(data.status).toBe('running');
    expect(data.tokens).toBe(100);
    mgr.dispose();
  });
});
