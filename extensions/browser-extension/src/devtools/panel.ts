/**
 * DevTools 面板逻辑 — 在 chrome.devtools.panels 创建的 iframe 中执行
 *
 * 通过 background service worker 中转，与 content script 通信：
 * 1) 发送 GET_AGENT_STATUS 到 content script 获取页面代理信息
 * 2) 发送 GET_TRACES 到 background 获取 trace 数据
 * 3) 定时刷新展示
 */

import type { ExtensionMessage, RunSummary, TraceEvent } from '../shared/messages.js';

/** 向 background service worker 发消息 */
function sendToBackground<T extends ExtensionMessage>(msg: T): Promise<unknown> {
    return new Promise((resolve, reject) => {
        chrome.runtime.sendMessage(msg, (resp) => {
            if (chrome.runtime.lastError) reject(new Error(chrome.runtime.lastError.message));
            else resolve(resp);
        });
    });
}

/** 向当前 inspected window 的 content script 发消息 */
function sendToContentScript<T extends ExtensionMessage>(msg: T): Promise<unknown> {
    return new Promise((resolve, reject) => {
        try {
            chrome.runtime.sendMessage(msg, (resp) => {
                if (chrome.runtime.lastError) reject(new Error(chrome.runtime.lastError.message));
                else resolve(resp);
            });
        } catch (err) {
            reject(err instanceof Error ? err : new Error(String(err)));
        }
    });
}

/** DOM 元素辅助 */
function el<T extends HTMLElement>(id: string): T {
    const e = document.getElementById(id);
    if (!e) throw new Error(`Missing element #${id}`);
    return e as T;
}

/** 渲染 trace 时间线 */
function renderTimeline(events: TraceEvent[]): void {
    const list = el<HTMLUListElement>('timeline');
    if (!events || events.length === 0) {
        list.innerHTML = '<li class="empty">暂无 trace 数据</li>';
        return;
    }
    const maxDur = Math.max(...events.map((e) => e.duration ?? 1), 1);
    list.innerHTML = events
        .map((e) => {
            const width = Math.max(4, ((e.duration ?? 0) / maxDur) * 200);
            const ts = new Date(e.timestamp).toLocaleTimeString();
            return `
                <li>
                    <span class="bar ${e.type}" style="width:${width}px" title="${e.duration ?? 0}ms"></span>
                    <span class="meta">
                        <span class="type">${e.type}</span>
                        <span class="time"> · ${ts}${e.tokens ? ` · ${e.tokens}t` : ''}</span>
                        ${e.label ? `<br/><span class="time">${e.label}</span>` : ''}
                    </span>
                </li>`;
        })
        .join('');
}

/** 渲染总 token 与成本 */
function renderCost(runs: RunSummary[]): void {
    const totalTokens = runs.reduce((sum, r) => sum + (r.totalTokens ?? 0), 0);
    el('total-tokens').textContent = totalTokens.toLocaleString();
    // 简化成本估算：$0.01 / 1K tokens
    const costUsd = (totalTokens / 1000) * 0.01;
    el('cost-summary').textContent = `≈ $${costUsd.toFixed(4)} (按 $0.01/1K tokens)`;
}

/** 刷新代理状态 */
async function refreshAgentState(): Promise<void> {
    try {
        const resp = (await sendToContentScript<Extract<ExtensionMessage, { type: 'GET_AGENT_STATUS' }>>({
            type: 'GET_AGENT_STATUS',
        })) as { detected?: boolean; agentId?: string; agentName?: string } | undefined;

        if (resp?.detected) {
            el('agent-state').className = 'value status-running';
            el('agent-state').textContent = '已检测';
            el('agent-id').textContent = resp.agentId ?? resp.agentName ?? 'unknown';
        } else {
            el('agent-state').className = 'value status-idle';
            el('agent-state').textContent = '未检测';
            el('agent-id').textContent = '—';
        }
    } catch {
        el('agent-state').className = 'value status-error';
        el('agent-state').textContent = '无法访问页面';
    }
}

/** 刷新 trace 与运行列表 */
async function refreshData(): Promise<void> {
    try {
        // 触发连接检查
        await sendToBackground<Extract<ExtensionMessage, { type: 'REFRESH_STATUS' }>>({
            type: 'REFRESH_STATUS',
        });

        const runsResp = (await sendToBackground<Extract<ExtensionMessage, { type: 'GET_RECENT_RUNS' }>>({
            type: 'GET_RECENT_RUNS',
        })) as RunSummary[] | undefined;

        const runs = runsResp ?? [];
        if (runs.length > 0) {
            el('session-id').textContent = runs[0].id;
        } else {
            el('session-id').textContent = '—';
        }

        renderCost(runs);

        // 若存在最新运行，尝试拉取 trace
        if (runs.length > 0) {
            const tracesResp = (await sendToBackground<Extract<ExtensionMessage, { type: 'GET_TRACES' }>>({
                type: 'GET_TRACES',
                payload: { sessionId: runs[0].id },
            })) as { events?: TraceEvent[] } | undefined;
            renderTimeline(tracesResp?.events ?? []);
        } else {
            renderTimeline([]);
        }
    } catch {
        renderTimeline([]);
    }
}

/** 导出 trace 为 JSON 文件 */
async function exportJson(): Promise<void> {
    try {
        const runsResp = (await sendToBackground<Extract<ExtensionMessage, { type: 'GET_RECENT_RUNS' }>>({
            type: 'GET_RECENT_RUNS',
        })) as RunSummary[] | undefined;

        const blob = new Blob([JSON.stringify({ exportedAt: Date.now(), runs: runsResp ?? [] }, null, 2)], {
            type: 'application/json',
        });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `ap-traces-${Date.now()}.json`;
        a.click();
        URL.revokeObjectURL(url);
    } catch {
        /* 静默降级 */
    }
}

/** 入口 */
async function main(): Promise<void> {
    el('btn-refresh').addEventListener('click', () => {
        void refreshAgentState();
        void refreshData();
    });
    el('btn-export').addEventListener('click', () => {
        void exportJson();
    });
    el('btn-reload').addEventListener('click', () => {
        chrome.devtools.inspectedWindow.reload({});
    });

    // 监听 background 广播的连接状态
    chrome.runtime.onMessage.addListener((msg) => {
        if ((msg as ExtensionMessage).type === 'CONNECTION_STATUS') {
            void refreshAgentState();
            void refreshData();
        }
    });

    await refreshAgentState();
    await refreshData();

    // 每 10 秒自动刷新
    setInterval(() => {
        void refreshAgentState();
        void refreshData();
    }, 10_000);
}

void main();
