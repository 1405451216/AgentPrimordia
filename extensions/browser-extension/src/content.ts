/**
 * Content Script — 经典脚本（无 ES 模块导入导出）
 *
 * 背景：Chrome MV3 的 content script 不支持 ES 模块，只能作为经典脚本运行。
 * 因此本文件不能 import 任何其它脚本模块，所有逻辑（检测逻辑、消息桥接）
 * 都必须内联。background / popup / devtools 运行时可使用 modules/shared/messages.ts。
 *
 * 职责：
 * - 检测当前页面是否嵌入了 AgentPrimordia 代理
 * - 若检测到，在页面右下角注入悬浮按钮
 * - 通过 chrome.runtime.sendMessage 与 background 通信
 */

// ---- 类型定义（仅供 tsc 编译期检查，运行时被擦除）---------------------

interface AgentDetectionResult {
    detected: boolean;
    source: AgentSource;
    agentId?: string;
    agentName?: string;
    endpoint?: string;
}

type AgentSource = 'meta' | 'global' | 'manual' | 'none';

interface ExtensionMessage {
    type: string;
    payload?: unknown;
}

// ---- 检测逻辑 ------------------------------------------------------------

/** 检测结果的缓存，避免重复通知 */
let lastDetected = false;

/** 从 <meta> 标签读取代理配置（前缀 ap-agent-） */
function detectFromMeta(): Record<string, string> | null {
    const prefix = 'ap-agent-';
    const metas = document.querySelectorAll('meta[name]');
    const config: Record<string, string> = {};
    metas.forEach((m) => {
        const name = m.getAttribute('name') ?? '';
        if (name.startsWith(prefix)) {
            const key = name.slice(prefix.length);
            const value = m.getAttribute('content');
            if (value) config[key] = value;
        }
    });
    return Object.keys(config).length > 0 ? config : null;
}

/** 从全局变量（window.__AP_AGENT__）读取代理配置 */
function detectFromGlobal(): Record<string, string> | null {
    const g = (window as unknown as Record<string, unknown>).__AP_AGENT__;
    if (g && typeof g === 'object') {
        const obj = g as Record<string, unknown>;
        const config: Record<string, string> = {};
        if (typeof obj.id === 'string') config['agentId'] = obj.id;
        if (typeof obj.name === 'string') config['agentName'] = obj.name;
        if (typeof obj.endpoint === 'string') config['endpoint'] = obj.endpoint;
        return Object.keys(config).length > 0 ? config : null;
    }
    return null;
}

/** 执行代理检测，返回配置或 null */
function detectAgent(): { source: AgentSource; config: Record<string, string> } | null {
    const meta = detectFromMeta();
    if (meta) return { source: 'meta', config: meta };
    const g = detectFromGlobal();
    if (g) return { source: 'global', config: g };
    return null;
}

// ---- 消息桥接 ------------------------------------------------------------

/** 通知 background：检测到/未检测到代理 */
function notifyAgentDetection(detected: boolean, meta?: Record<string, string>): void {
    const msg: ExtensionMessage = detected
        ? {
              type: 'AGENT_DETECTED',
              payload: {
                  detected: true,
                  source: (meta?.['source'] as AgentSource) ?? 'manual',
                  agentId: meta?.['agentId'],
                  agentName: meta?.['agentName'],
                  endpoint: meta?.['endpoint'],
              } satisfies AgentDetectionResult,
          }
        : { type: 'AGENT_NOT_DETECTED' };
    try {
        chrome.runtime.sendMessage(msg);
    } catch {
        /* background 未就绪时静默降级 */
    }
}

// ---- UI 注入 -------------------------------------------------------------

/** 创建悬浮按钮（右下角，紫色圆形 "AP"） */
function injectFloatingButton(): void {
    if (document.getElementById('ap-ext-floating-btn')) return;

    const btn = document.createElement('button');
    btn.id = 'ap-ext-floating-btn';
    btn.textContent = 'AP';
    btn.title = 'AgentPrimordia 代理检测 — 点击打开 Studio';
    Object.assign(btn.style, {
        position: 'fixed',
        bottom: '20px',
        right: '20px',
        width: '44px',
        height: '44px',
        borderRadius: '50%',
        background: '#6C5CE7',
        color: '#fff',
        border: 'none',
        cursor: 'pointer',
        zIndex: '2147483647',
        fontSize: '14px',
        fontWeight: 'bold',
        boxShadow: '0 2px 8px rgba(0,0,0,0.3)',
        fontFamily: 'system-ui, sans-serif',
    } as CSSStyleDeclaration);

    btn.addEventListener('click', () => {
        try {
            chrome.runtime.sendMessage({ type: 'REFRESH_STATUS' });
            chrome.runtime.sendMessage({ type: 'OPEN_STUDIO', payload: {} });
        } catch {
            /* 静默降级 */
        }
    });

    document.body?.appendChild(btn);
}

/** 移除悬浮按钮 */
function removeFloatingButton(): void {
    document.getElementById('ap-ext-floating-btn')?.remove();
}

// ---- 主流程 --------------------------------------------------------------

/** 检测代理并通知 background，按需注入/移除 UI */
function scan(): void {
    const result = detectAgent();
    const detected = result !== null;
    if (detected === lastDetected) return;
    lastDetected = detected;

    if (detected && result) {
        injectFloatingButton();
        notifyAgentDetection(true, { source: result.source, ...result.config });
    } else {
        removeFloatingButton();
        notifyAgentDetection(false);
    }
}

/** 监听来自 background / popup 的状态请求 */
chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
    const msg = message as ExtensionMessage;
    if (msg.type === 'GET_AGENT_STATUS') {
        const result = detectAgent();
        const resp: Record<string, unknown> = {
            detected: result !== null,
            source: result?.source ?? 'none',
        };
        if (result?.config) {
            resp['agentId'] = result.config['agentId'];
            resp['agentName'] = result.config['agentName'];
            resp['endpoint'] = result.config['endpoint'];
        }
        sendResponse(resp);
    }
    return false;
});

/** 启动：初始扫描 + MutationObserver 监听动态 DOM 变化 */
function main(): void {
    scan();
    const observer = new MutationObserver(() => {
        scan();
    });
    observer.observe(document.documentElement, { childList: true, subtree: true });
}

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', main);
} else {
    main();
}
