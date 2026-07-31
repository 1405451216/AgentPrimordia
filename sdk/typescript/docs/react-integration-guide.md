# React 集成指南

本指南介绍如何在 React 应用中使用 AgentPrimordia SDK 的 Hooks 和组件。

---

## 安装

```bash
npm install @agentprimordia/sdk react react-dom
```

SDK 的 React 模块通过 `peerDependencies` 声明，需确保项目中已安装 React 18+ 或 19。

---

## 核心 Hooks

### useAgent — Agent 生命周期管理

```tsx
import { useAgent } from '@agentprimordia/sdk';

function ChatPanel() {
  const { agent, status, error } = useAgent({
    name: 'assistant',
    model: 'gpt-4',
    systemPrompt: '你是一个有帮助的助手。',
  });

  if (status === 'connecting') return <div>连接中...</div>;
  if (error) return <div>错误: {error.message}</div>;

  return <div>Agent 就绪: {agent.id}</div>;
}
```

### useAgentStream — 流式对话

```tsx
import { useAgentStream } from '@agentprimordia/sdk';

function StreamChat() {
  const { messages, send, isStreaming, abort } = useAgentStream({
    apiBase: 'http://localhost:8080',
    agentId: 'my-agent',
  });

  return (
    <div>
      {messages.map((msg, i) => (
        <div key={i} className={msg.role}>
          {msg.content}
        </div>
      ))}
      <input
        onKeyDown={(e) => {
          if (e.key === 'Enter') send(e.currentTarget.value);
        }}
      />
      {isStreaming && <button onClick={abort}>停止</button>}
    </div>
  );
}
```

### useAgentSuspense — Suspense 模式

```tsx
import { useAgentSuspense } from '@agentprimordia/sdk';
import { Suspense } from 'react';

function AgentResult({ query }: { query: string }) {
  const result = useAgentSuspense({
    apiBase: 'http://localhost:8080',
    agentId: 'research-agent',
    input: query,
  });

  return <article>{result.response}</article>;
}

// 使用
<Suspense fallback={<div>思考中...</div>}>
  <AgentResult query="量子计算的最新进展" />
</Suspense>
```

---

## 协作组件

SDK 提供多 Agent 协作可视化组件：

```tsx
import {
  CollaborationView,
  MessageFlow,
  HITLPanel,
  CollaborationReplay,
} from '@agentprimordia/sdk';

function CollaborationDashboard() {
  return (
    <div>
      {/* 多 Agent 协作视图 */}
      <CollaborationView
        agents={[
          { id: 'a1', name: 'Researcher', status: 'working' },
          { id: 'a2', name: 'Writer', status: 'idle' },
        ]}
      />

      {/* 消息流 */}
      <MessageFlow
        messages={[
          { id: 'm1', from: 'a1', to: 'a2', content: '研究完成', kind: 'result' },
        ]}
        agents={agents}
      />

      {/* Human-in-the-Loop 审批面板 */}
      <HITLPanel
        approvals={[
          { id: 'ap1', tool: 'shell', args: 'rm -rf /tmp', status: 'pending' },
        ]}
        onApprove={(id) => console.log('approved', id)}
        onReject={(id) => console.log('rejected', id)}
      />
    </div>
  );
}
```

---

## Server Components (RSC)

SDK 支持 React Server Components：

```tsx
// app/agent/page.tsx (Next.js App Router)
import { AgentServerComponent } from '@agentprimordia/sdk';

export default async function AgentPage() {
  return (
    <AgentServerComponent
      apiBase={process.env.AP_API_BASE!}
      agentConfig={{
        name: 'ssr-agent',
        model: 'gpt-4',
        systemPrompt: '你是 SSR 助手',
      }}
      initialInput="你好"
    />
  );
}
```

---

## Storybook

组件 Storybook 已配置在 `.storybook/` 目录：

```bash
npx storybook dev
```

包含以下 stories：
- `Collaboration/AgentNode` — Agent 状态节点
- `Collaboration/CollaborationView` — 协作全景
- `Collaboration/MessageFlow` — 消息流
- `Collaboration/HITLPanel` — 审批面板
- `Collaboration/CollaborationReplay` — 回放

---

## 无障碍 (a11y)

所有组件遵循 WAI-ARIA 规范：
- 交互元素具有正确的 `role` 属性
- 支持键盘导航（Tab / Enter / Escape）
- 状态变化通过 `aria-live` 通知屏幕阅读器
- 颜色对比度满足 WCAG 2.1 AA 标准

---

## 最佳实践

- 使用 `useAgentStream` 而非轮询实现实时对话
- 在 `StrictMode` 下测试 Agent 连接/断开生命周期
- 协作组件配合 `CollaborationReplay` 提供调试回放能力
- 生产环境使用 `React.memo` 包裹消息列表避免不必要重渲染
