# React Hooks 指南

> 在 React 应用中使用 `useAgent` / `useReActLoop` / `useStreamRun` Hooks。

## 安装

```bash
npm install @agentprimordia/react
# 或
yarn add @agentprimordia/react
```

## useAgent

```tsx
import { useAgentWithReact } from '@agentprimordia/react';
import React from 'react';

const useAgent = useAgentWithReact(React, (opts) => new Agent(opts));

function ChatComponent() {
  const { agent, isRunning, error, run, streamRun, stop } = useAgent({
    name: 'chat-agent',
    systemPrompt: '你是助手',
    maxTurns: 10,
  });

  return (
    <div>
      <button onClick={() => run('你好')} disabled={isRunning}>
        {isRunning ? '运行中...' : '发送'}
      </button>
      {error && <p style={{ color: 'red' }}>{error.message}</p>}
    </div>
  );
}
```

## useReActLoop

实时显示 ReAct 思考-行动-观察过程：

```tsx
import { useReActLoopWithReact } from '@agentprimordia/react';

const useReActLoop = useReActLoopWithReact(React);

function Inspector() {
  const { currentStep, thoughts, actions, observations, run, stop } = useReActLoop(() => ({
    stream: async function* () {
      yield { type: 'thought', text: '分析问题...' };
      yield { type: 'action', tool: 'search', args: { q: 'test' } };
      yield { type: 'observation', content: '搜索结果', isError: false };
      yield { type: 'done', text: 'final answer' };
    },
  }));

  return (
    <div>
      <p>当前阶段: {currentStep}</p>
      {thoughts.map((t, i) => <p key={i}>💭 {t}</p>)}
      {actions.map((a, i) => <p key={i}>🔧 {a.tool}</p>)}
    </div>
  );
}
```

## useStreamRun

逐 token 累积显示流式输出：

```tsx
const useStreamRun = useStreamRunWithReact(React);

function StreamView() {
  const { text, isStreaming, run, stop } = useStreamRun();

  return (
    <div>
      <button onClick={() => run({ prompt: '写一首诗' })}>生成</button>
      <pre>{text}</pre>
      {isStreaming && <span>▌</span>}
    </div>
  );
}
```

## 依赖注入

Hooks 通过 `UseAgentDeps` 接口与 React 解耦，支持 React 16/17/18+：

```tsx
interface UseAgentDeps {
  useState: <T>(initial: T) => [T, Setter<T>]:T];
  useCallback: (fn: T, deps: readonly unknown[]) => T;
  useRef: <T>(initial: T) => { current: T };
  useEffect: (fn: () => void | (() => void), deps?: readonly unknown[]) => void;
}
```

自定义 deps（例如测试、React Native）只需实现该接口传入工厂函数。
