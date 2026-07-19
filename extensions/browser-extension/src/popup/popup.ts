/**
 * Popup 脚本 — 用户点击扩展图标时弹出的小窗
 *
 * 职责：
 * - 展示当前页面代理状态
 * - 提供快速对话输入
 * - 展示最近运行列表
 * - 提供快捷操作按钮
 */

import type { ExtensionMessage, RunSummary } from '../shared/messages.js';

/** DOM 元素辅助 */
function el<T extends HTMLElement>(id: string): T {
    const e = document.getElementById(id);
    if (!e) throw new Error(`Missing element #${id}`);
    return e as T;
}

/** 发送消息到 background，等待响应 */
function send<T extends ExtensionMessage>(msg: T): Promise<unknown> {
    return new Promise((resolve, reject) => {
        chrome.runtime.sendMessage(msg, (resp) => {
            if (chrome.runtime.lastError) reject(new Error(chrome.runtime.lastError.message));
            else resolve(resp);
        });
    });
}

/** 发送消息到指定标签页的 content script，等待响应 */
function sendToTab<T>(tabId: number, msg: T): Promise<unknown> {
    return new Promise((resolve, reject) => {
        chrome.tabs.sendMessage(tabId, msg, (resp) => {
            if (chrome.runtime.lastError) reject(new Error(chrome.runtime.lastError.message));
            else resolve(resp);
        });
    });
}

/** 更新 Studio 连接状态 UI */
function updateStudioStatus(connected: boolean): void {
    const dot = el('studio-status-dot');
    const text = el('studio-status-text');
    dot.className = `status-dot ${connected ? 'connected' : 'disconnected'}`;
    text.textContent = connected ? 'Studio 已连接' : 'Studio 未连接';
}

/** 更新代理检测状态 UI */
function updateAgentStatus(detected: boolean, agentName?: string): void {
    const dot = el('agent-status-dot');
    const text = el('agent-status-text');
    const sendBtn = el<HTMLButtonElement>('btn-send');
    const inspectBtn = el<HTMLButtonElement>('btn-inspect');
    const tracesBtn = el<HTMLButtonElement>('btn-traces');

    if (detected) {
        dot.className = 'status-dot detected';
        text.textContent = agentName ? `已检测：${agentName}` : '已检测到代理';
        sendBtn.disabled = false;
        inspectBtn.disabled = false;
        tracesBtn.disabled = false;
    } else {
        dot.className = 'status-dot';
        text.textContent = '未检测到代理';
        sendBtn.disabled = true;
        inspectBtn.disabled = true;
        tracesBtn.disabled = true;
    }
}

/** 渲染最近运行列表 */
function renderRuns(runs: RunSummary[]): void {
    const list = el<HTMLUListElement>('runs-list');
    if (!runs || runs.length === 0) {
        list.innerHTML = '<li class="empty">暂无运行记录</li>';
        return;
    }
    list.innerHTML = runs
        .map(
            (r) => `
        <li>
            <span class="run-id">${r.id.slice(0, 8)}…</span>
            <span class="run-status ${r.status}">${r.status}</span>
        </li>`,
        )
        .join('');
}

/** 查询当前标签页的代理状态 */
function queryAgentStatus(): Promise<void> {
    return new Promise<void>((resolve) => {
        chrome.tabs.query({ active: true, currentWindow: true }, (tabs) => {
            const tab = (tabs as chrome.tabs.Tab[])[0];
            if (!tab?.id) {
                updateAgentStatus(false);
                resolve();
                return;
            }
            sendToTab<Extract<ExtensionMessage, { type: 'GET_AGENT_STATUS' }>>(tab.id, {
                type: 'GET_AGENT_STATUS',
            })
                .then((resp) => {
                    const r = resp as { detected?: boolean; agentName?: string } | undefined;
                    updateAgentStatus(!!r?.detected, r?.agentName);
                })
                .catch(() => updateAgentStatus(false))
                .finally(() => resolve());
        });
    });
}

/** 加载最近运行列表 */
async function loadRuns(): Promise<void> {
    try {
        const resp = (await send<Extract<ExtensionMessage, { type: 'GET_RECENT_RUNS' }>>({
            type: 'GET_RECENT_RUNS',
        })) as RunSummary[] | undefined;
        renderRuns(resp ?? []);
    } catch {
        renderRuns([]);
    }
}

/** 入口 */
async function main(): Promise<void> {
    // 1) Studio 连接状态
    try {
        const resp = (await send<Extract<ExtensionMessage, { type: 'GET_AGENT_STATUS' }>>({
            type: 'GET_AGENT_STATUS',
        })) as { connected?: boolean } | undefined;
        updateStudioStatus(!!resp?.connected);
    } catch {
        updateStudioStatus(false);
    }

    // 2) 代理状态
    await queryAgentStatus();

    // 3) 最近运行
    await loadRuns();

    // 4) 事件绑定
    el<HTMLButtonElement>('btn-send').addEventListener('click', async () => {
        const input = el<HTMLInputElement>('chat-input');
        const result = el('send-result');
        const text = input.value.trim();
        if (!text) return;
        input.disabled = true;
        try {
            await send<Extract<ExtensionMessage, { type: 'SEND_MESSAGE' }>>({
                type: 'SEND_MESSAGE',
                payload: { text },
            });
            result.textContent = '已发送';
            input.value = '';
        } catch (err) {
            result.textContent = `发送失败：${err instanceof Error ? err.message : String(err)}`;
        } finally {
            input.disabled = false;
        }
    });

    el<HTMLButtonElement>('btn-inspect').addEventListener('click', () => {
        void send<Extract<ExtensionMessage, { type: 'REFRESH_STATUS' }>>({ type: 'REFRESH_STATUS' });
    });

    el<HTMLButtonElement>('btn-traces').addEventListener('click', () => {
        void send<Extract<ExtensionMessage, { type: 'OPEN_STUDIO' }>>({
            type: 'OPEN_STUDIO',
            payload: { path: '/traces' },
        });
    });

    el<HTMLButtonElement>('btn-open-studio').addEventListener('click', () => {
        void send<Extract<ExtensionMessage, { type: 'OPEN_STUDIO' }>>({ type: 'OPEN_STUDIO', payload: {} });
    });

    // 监听连接状态广播
    chrome.runtime.onMessage.addListener((msg) => {
        if ((msg as ExtensionMessage).type === 'CONNECTION_STATUS') {
            updateStudioStatus(!!(msg as { payload?: { connected?: boolean } }).payload?.connected);
        }
    });
}

void main();
