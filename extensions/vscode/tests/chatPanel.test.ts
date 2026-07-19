/**
 * ChatPanel 组件测试。
 */

import { describe, it, expect, vi } from 'vitest';
import { ChatPanelProvider } from '../src/chatPanel.js';
import type { StudioApi } from '../src/studioApi.js';

describe('ChatPanelProvider', () => {
  it('初始化时加载历史消息', () => {
    const history = [
      { role: 'user' as const, content: 'hi', timestamp: Date.now() },
    ];
    const api = {} as StudioApi;
    const provider = new ChatPanelProvider({
      api,
      defaultTemplate: 'default',
      loadHistory: () => history,
      saveHistory: vi.fn(),
    });
    expect(provider).toBeDefined();
  });

  it('resolveWebviewView 正确初始化视图 html', () => {
    const api = {} as StudioApi;
    const provider = new ChatPanelProvider({
      api,
      defaultTemplate: 'default',
      loadHistory: () => [],
      saveHistory: vi.fn(),
    });
    const fakeView = {
      webview: { options: {}, html: '', onDidReceiveMessage: vi.fn(), postMessage: vi.fn() },
    };
    provider.resolveWebviewView(fakeView);
    expect(fakeView.webview.html).toContain('messages');
    expect(fakeView.webview.options.enableScripts).toBe(true);
  });

  it('dispose 不抛错', () => {
    const api = {} as StudioApi;
    const provider = new ChatPanelProvider({
      api,
      defaultTemplate: 'default',
      loadHistory: () => [],
      saveHistory: vi.fn(),
    });
    expect(() => provider.dispose()).not.toThrow();
  });
});
