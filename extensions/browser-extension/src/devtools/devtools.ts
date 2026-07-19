/**
 * DevTools 入口脚本 — 在 chrome.devtools 上下文中执行
 *
 * 通过 chrome.devtools.panels.create 注册一个名为 "AgentPrimordia" 的自定义面板。
 * 面板内容由 panel.html 渲染。
 */

chrome.devtools.panels.create(
    'AgentPrimordia',
    '', // MV3 下图标建议放在 dist；空串使用默认图标
    'dist/devtools/panel.html',
    (panel) => {
        // 面板创建成功后的回调；可用于后续事件绑定
        void panel;
    },
);
