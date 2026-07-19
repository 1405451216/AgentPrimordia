/**
 * Background Service Worker
 *
 * 职责：
 * - 路由 content script / popup / devtools 之间的消息
 * - 维护与 Studio 后端的连接状态
 * - 轮询 Studio API 获取运行状态
 * - 在 chrome.storage.local 中持久化最近会话
 */

import { defaultExtensionState, copy, type ExtensionMessage, type ExtensionState } from './shared/messages.js';
import { StudioApiClient, ApiError } from './shared/api.js';

/** 存储 key */
const STORAGE_KEY = 'ap_extension_state';
/** 默认轮询间隔（毫秒） */
const POLL_INTERVAL_MS = 10_000;

/** 当前内存中的状态（storage 的缓存镜像） */
let state: ExtensionState = defaultExtensionState();
/** API 客户端（URL 跟随 state.studioUrl 调整） */
let apiClient = new StudioApiClient(state.studioUrl);
let pollTimer: ReturnType<typeof setInterval> | null = null;

/** 从 storage 加载状态，失败则回退到默认值 */
function loadState(): Promise<void> {
    return new Promise((resolve) => {
        chrome.storage.local.get([STORAGE_KEY], (items) => {
            try {
                const stored = (items as Record<string, unknown>)[STORAGE_KEY];
                if (stored && typeof stored === 'object') {
                    state = { ...defaultExtensionState(), ...(stored as ExtensionState) };
                    apiClient = new StudioApiClient(state.studioUrl);
                }
            } catch {
                state = defaultExtensionState();
            }
            resolve();
        });
    });
}

/** 持久化当前状态到 storage */
function persistState(): Promise<void> {
    return new Promise((resolve) => {
        chrome.storage.local.set({ [STORAGE_KEY]: state }, () => resolve());
    });
}

/** 广播连接状态到所有监听者 */
function broadcastConnection(): void {
    const msg = copy('CONNECTION_STATUS', {
        connected: state.connected,
        studioUrl: state.studioUrl,
    });
    try {
        chrome.runtime.sendMessage(msg, () => {
            if (chrome.runtime.lastError) {
                /* 无监听者时忽略 */
            }
        });
    } catch {
        /* 无监听者时忽略 */
    }
}

/** 测试 Studio 后端连通性 */
async function checkConnection(): Promise<void> {
    try {
        await apiClient.health();
        if (!state.connected) {
            state.connected = true;
            await persistState();
            broadcastConnection();
        }
    } catch {
        if (state.connected) {
            state.connected = false;
            await persistState();
            broadcastConnection();
        }
    }
}

/** 拉取最近运行并更新状态 */
async function refreshRuns(): Promise<void> {
    if (!state.connected) return;
    try {
        const runs = await apiClient.recentRuns(20);
        state.recentRuns = runs;
        if (runs.length > 0 && runs[0]?.status === 'running') {
            state.currentSession = runs[0].id;
        }
        await persistState();
    } catch (err) {
        if (err instanceof ApiError && err.status && err.status >= 500) {
            state.connected = false;
            await persistState();
            broadcastConnection();
        }
    }
}

/** 启动定期轮询 */
function startPolling(): void {
    if (pollTimer !== null) return;
    void checkConnection();
    void refreshRuns();
    pollTimer = setInterval(() => {
        void checkConnection();
        void refreshRuns();
    }, POLL_INTERVAL_MS);
}

/** 路由收到的消息并返回响应 */
async function handleMessage(message: ExtensionMessage): Promise<unknown> {
    switch (message.type) {
        case 'AGENT_DETECTED':
        case 'AGENT_NOT_DETECTED':
            return { ok: true };

        case 'GET_AGENT_STATUS':
            return { connected: state.connected, currentSession: state.currentSession };

        case 'GET_RECENT_RUNS':
            return state.recentRuns;

        case 'GET_TRACES': {
            const traces = await apiClient.getTraces(message.payload.sessionId);
            return { sessionId: message.payload.sessionId, events: traces };
        }

        case 'SEND_MESSAGE':
            return { success: true, echo: message.payload.text };

        case 'REFRESH_STATUS':
            await checkConnection();
            await refreshRuns();
            return { connected: state.connected, runs: state.recentRuns.length };

        case 'OPEN_STUDIO':
            await chrome.tabs.create({
                url: `${state.studioUrl}${message.payload.path ?? '/'}`,
            });
            return { ok: true };

        default:
            return { error: 'unknown_message_type' };
    }
}

/** 初始化：注册监听器并启动轮询 */
async function init(): Promise<void> {
    await loadState();
    chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
        void sender;
        handleMessage(message as ExtensionMessage)
            .then(sendResponse)
            .catch((err) => sendResponse({ error: err instanceof Error ? err.message : String(err) }));
        return true; // 异步响应
    });
    startPolling();
}

void init();
