# Edge Runtime 指南

> 在 Cloudflare Workers / Vercel Edge 等边缘运行时中运行 AgentPrimordia TS SDK。

## 运行时检测

```ts
import { detectRuntime, EdgeRequest, EdgeResponse, EdgeKV } from '@agentprimordia/edge';

console.log(detectRuntime());
// 'cloudflare-workers' | 'vercel-edge' | 'deno' | 'node'
```

## Cloudflare Workers 示例

```ts
// worker.ts
import { Agent } from '@agentprimordia/typescript';
import { FromHTTPRequest, ToHTTPRequest, WriteHTTPResponse } from '@agentprimordia/edge';

export default {
  async fetch(request: Request): Promise<Response> {
    const edgeReq = await FromHTTPRequest(request);

    const agent = new Agent({
      name: 'edge-agent',
      llm: { provider: 'anthropic', model: 'claude-3-5-sonnet-20241022' },
    });

    const result = await agent.run(edgeReq.Body.toString());

    const edgeResp: EdgeResponse = {
      Status: 200,
      Headers: { 'content-type': 'application/json' },
      Body: JSON.stringify({ content: result.content }),
    };

    return WriteHTTPResponse(new Response(), edgeResp);
  },
};
```

## KV Store（边缘缓存）

```ts
import { createEdgeKV, EdgeKV } from '@agentprimordia/edge';

const kv: EdgeKV = createEdgeKV();
await kv.put('session:123', JSON.stringify({ turns: 5 }), {
  expirationTTL: 3600, // 1 hour TTL
});

const entry = await kv.get('session:123');
```

## 兼容性矩阵

| 运行时 | fetch | KV | Cache API | 状态 |
|--------|-------|-----|-----------|------|
| Cloudflare Workers | ✅ | ✅ | ✅ | 完全支持 |
| Vercel Edge | ✅ | ⚠️ 用 Upstash | 部分支持 |
| Deno Deploy | ✅ | ✅ Deno KV | ✅ | 完全支持 |
| Node.js | ✅ (polyfill) | 内存回退 | ⚠️ | 部分支持 |

## 限制

- Agent 状态（session）默认存储在 KV，重启恢复
- 流式输出使用 SSE（Server-Sent Events）
- 不能在边缘使用 SQLite 数据库（改用 KV 或远程 pgvector）
