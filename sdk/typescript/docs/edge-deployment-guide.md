# 边缘运行时部署指南

本指南介绍如何将 AgentPrimordia TS SDK 部署到 Cloudflare Workers、Deno Deploy 和 Bun 等边缘运行时。

---

## 支持的边缘运行时

| 运行时 | 适配模块 | 状态 |
|--------|---------|------|
| Cloudflare Workers | `src/edge/cloudflare-adapter.ts` | Stable |
| Deno Deploy | `src/edge/deno-agent.ts` | Stable |
| Bun | `src/edge/bun-agent.ts` | Stable |
| Node.js | 默认（无需适配） | Stable |

---

## Cloudflare Workers

### 1. 安装依赖

```bash
npm install @agentprimordia/sdk
npm install -D wrangler
```

### 2. 创建 Worker

```typescript
// src/worker.ts
import { CloudflareAgentAdapter } from '@agentprimordia/sdk';

const adapter = new CloudflareAgentAdapter({
  name: 'edge-assistant',
  model: 'gpt-4',
  systemPrompt: '你是一个低延迟边缘助手。',
});

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);

    if (url.pathname === '/chat' && request.method === 'POST') {
      const { message } = await request.json<{ message: string }>();
      const reply = await adapter.run(message);
      return Response.json({ reply });
    }

    if (url.pathname === '/health') {
      return Response.json(adapter.getHealth());
    }

    return new Response('Not Found', { status: 404 });
  },
};
```

### 3. 配置 KV 存储（可选）

```toml
# wrangler.toml
[[kv_namespaces]]
binding = "AGENT_KV"
id = "your-kv-namespace-id"
```

SDK 会自动检测 Cloudflare KV 绑定并用于会话持久化。

### 4. 部署

```bash
npx wrangler deploy
```

---

## Deno Deploy

### 1. 创建入口

```typescript
// main.ts
import { DenoEdgeAgent } from '@agentprimordia/sdk';

const agent = new DenoEdgeAgent({
  name: 'deno-assistant',
  model: 'gpt-4',
  systemPrompt: '你是 Deno 边缘助手。',
  rateLimit: { maxRequests: 100, windowMs: 60_000 },
});

Deno.serve(async (req) => {
  const url = new URL(req.url);

  if (url.pathname === '/chat') {
    const { message } = await req.json();
    const result = await agent.runWithDetails(message);
    return Response.json(result);
  }

  return new Response('OK');
});
```

### 2. 部署

```bash
deployctl deploy --project=my-agent main.ts
```

---

## Bun

### 1. 创建服务

```typescript
// server.ts
import { BunEdgeAgent } from '@agentprimordia/sdk';

const agent = new BunEdgeAgent({
  name: 'bun-assistant',
  model: 'gpt-4',
});

Bun.serve({
  port: 3000,
  async fetch(req) {
    const url = new URL(req.url);
    if (url.pathname === '/chat') {
      const { message } = await req.json();
      const reply = await agent.run(message);
      return Response.json({ reply });
    }
    return new Response('Bun Agent Running');
  },
});
```

### 2. 运行

```bash
bun run server.ts
```

---

## 冷启动优化

SDK 内置冷启动优化模块（`src/edge/cold-start.ts`）：

```typescript
import { ColdStartOptimizer } from '@agentprimordia/sdk';

const optimizer = new ColdStartOptimizer({
  warmupInterval: 30_000,  // 每 30s 预热
  lazyLoadModules: true,    // 延迟加载非关键模块
  connectionPooling: true,  // 复用 LLM 连接
});

// 在 Worker 初始化时调用
optimizer.warmup();
```

---

## 运行时检测

SDK 提供自动运行时检测：

```typescript
import { detectRuntime, detectEnv } from '@agentprimordia/sdk';

const runtime = detectRuntime(); // 'cloudflare' | 'deno' | 'bun' | 'node'
const env = detectEnv();         // 'production' | 'development' | 'test'
```

---

## 注意事项

- 边缘运行时不支持 `better-sqlite3`，记忆存储请使用 KV 或远程 API
- Cloudflare Workers 有 50ms CPU 限制（付费版 15s），复杂 Agent 逻辑建议拆分
- Deno Deploy 不支持 Node.js 原生模块，SDK 的 browser 构建产物已排除这些依赖
- 使用 `tsup` 的 browser 构建（`dist/browser/`）可获得更小的边缘 bundle
