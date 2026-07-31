# 浏览器端 Agent 教程

本教程介绍如何在纯浏览器环境中运行 AI Agent，无需后端服务器。

---

## 概述

AgentPrimordia SDK 支持在浏览器中直接运行 Agent，利用以下技术：

- **WebGPU** — 本地模型推理（无需 API Key）
- **IndexedDB** — 客户端持久化存储
- **WASM 沙箱** — 安全的工具代码执行
- **SSE/WebSocket** — 与远程 Agent 服务通信

---

## 快速开始：远程 Agent 模式

最简单的方式是连接远程 Playground 服务：

```html
<script type="module">
import { PlaygroundClient } from 'https://cdn.jsdelivr.net/npm/@agentprimordia/sdk/dist/browser/index.js';

const pg = new PlaygroundClient({
  apiBase: 'http://localhost:8080',
  defaultModel: 'gpt-4',
});

const agent = await pg.createAgent({ name: 'browser-agent' });
const reply = await pg.chat(agent.id, '你好！');
document.getElementById('output').textContent = reply.response;
</script>
```

---

## WebGPU 本地推理

在支持 WebGPU 的浏览器中运行本地模型：

```typescript
import { WebGPUProvider } from '@agentprimordia/sdk';

// 检测 WebGPU 支持
const supported = await WebGPUProvider.detect();
if (!supported) {
  console.log('WebGPU 不可用，回退到远程 API');
}

// 创建本地推理 Provider
const provider = await WebGPUProvider.create({
  model: 'phi-3-mini',        // 本地模型
  maxTokens: 2048,
  temperature: 0.7,
});

// 使用 Provider 创建 Agent
const agent = new ReActAgent({
  name: 'local-agent',
  llm: provider,
  systemPrompt: '你是一个完全离线运行的助手。',
});

const result = await agent.run('解释量子纠缠');
```

### 降级策略

```typescript
import { WebGPUProvider } from '@agentprimordia/sdk';

// 自动降级：WebGPU → 远程 API
const provider = await WebGPUProvider.createWithFallback({
  localModel: 'phi-3-mini',
  remoteUrl: 'https://api.example.com/v1',
  remoteApiKey: 'sk-...',
});
// 如果 WebGPU 不可用，自动使用远程 API
```

---

## IndexedDB 持久化

浏览器端记忆存储：

```typescript
import { IndexedDBVectorStore } from '@agentprimordia/sdk';

const store = new IndexedDBVectorStore({
  dbName: 'agent-memory',
  storeName: 'episodes',
});

// 存储记忆
await store.add({
  id: 'mem-1',
  content: '用户喜欢 TypeScript',
  embedding: [0.1, 0.2, ...],
});

// 语义搜索
const results = await store.search(queryEmbedding, { topK: 5 });
```

---

## WASM 沙箱工具执行

在浏览器中安全执行用户代码：

```typescript
import { CodeSandboxV2 } from '@agentprimordia/sdk';

const sandbox = new CodeSandboxV2({
  memoryLimitMB: 32,
  timeoutMs: 3000,
});

// 执行用户提交的代码
const result = await sandbox.execute(wasmModuleBytes, {
  function: 'calculate',
  input: JSON.stringify({ x: 42 }),
});

console.log(result.output); // 安全的沙箱输出
sandbox.terminate();
```

---

## 完整示例：浏览器端聊天应用

```html
<!DOCTYPE html>
<html>
<head>
  <title>Browser Agent Chat</title>
</head>
<body>
  <div id="messages"></div>
  <input id="input" placeholder="输入消息..." />
  <button id="send">发送</button>

  <script type="module">
    import { PlaygroundClient } from './dist/browser/index.js';

    const pg = new PlaygroundClient({
      apiBase: 'http://localhost:8080',
      defaultModel: 'gpt-4',
    });

    let agentId = null;

    async function init() {
      const agent = await pg.createAgent({
        name: 'web-chat',
        systemPrompt: '你是网页助手',
      });
      agentId = agent.id;
    }

    document.getElementById('send').onclick = async () => {
      const input = document.getElementById('input');
      const msg = input.value;
      input.value = '';

      appendMessage('user', msg);

      // 流式响应
      let content = '';
      const div = appendMessage('assistant', '...');
      for await (const event of pg.streamChat(agentId, msg)) {
        if (event.type === 'token') {
          content += event.content;
          div.textContent = content;
        }
      }
    };

    function appendMessage(role, text) {
      const div = document.createElement('div');
      div.className = role;
      div.textContent = text;
      document.getElementById('messages').appendChild(div);
      return div;
    }

    init();
  </script>
</body>
</html>
```

---

## 构建优化

使用 browser 构建产物获得最小 bundle：

```javascript
// vite.config.js
export default {
  resolve: {
    conditions: ['browser'],  // 使用 browser 条件导出
  },
  build: {
    rollupOptions: {
      external: ['better-sqlite3'],  // 排除 Node 模块
    },
  },
};
```

---

## 浏览器兼容性

| 特性 | Chrome | Firefox | Safari | Edge |
|------|--------|---------|--------|------|
| WebGPU | 113+ | 开发中 | 开发中 | 113+ |
| WASM | 57+ | 52+ | 11+ | 79+ |
| IndexedDB | 全部 | 全部 | 全部 | 全部 |
| SSE | 全部 | 全部 | 全部 | 全部 |
