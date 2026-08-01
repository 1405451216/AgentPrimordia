/**
 * Edge Agent 模板 — 开箱即用的 Cloudflare Workers/Deno/Bun Agent 入口。
 *
 * v3.0 方向2：Edge Agent 模板（CF Worker = Agent）
 *
 * 使用方式（Cloudflare Workers）：
 *
 *   import { createCloudflareAgentHandler } from '../src/edge/template.js';
 *   import { createProvider } from '../src/llm/provider.js';
 *
 *   const handler = createCloudflareAgentHandler({
 *     name: 'my-edge-agent',
 *     provider: createProvider({ type: 'openai', apiKey: env.OPENAI_API_KEY }),
 *     systemPrompt: 'You are a helpful assistant.',
 *   });
 *
 *   export default { fetch: handler };
 *
 * 使用方式（Deno）：
 *
 *   import { createDenoAgentHandler } from '../src/edge/template.js';
 *   const handler = createDenoAgentHandler({ ... });
 *   Deno.serve(handler);
 */

import type { Provider } from '../llm/provider.js';
import { buildEdgeAgent, MemoryEdgeStorage, type EdgeStorage } from './edge-storage.js';

// ===== 模板配置 =====

export interface EdgeAgentTemplateConfig {
  /** Agent 名称 */
  name?: string;
  /** LLM Provider */
  provider: Provider;
  /** 系统提示词 */
  systemPrompt?: string;
  /** 最大轮次 */
  maxTurns?: number;
  /** 请求超时（毫秒），默认 30000 */
  requestTimeoutMs?: number;
  /** 最大重试次数，默认 3 */
  maxRetries?: number;
  /** 限流：每分钟最大请求数，默认 60 */
  rateLimitPerMinute?: number;
  /** 自定义存储后端 */
  storage?: EdgeStorage;
  /** 是否启用 SSE 流式输出，默认 true */
  enableSSE?: boolean;
  /** 是否启用健康检查端点，默认 true */
  enableHealth?: boolean;
}

// ===== Cloudflare Workers 模板 =====

/**
 * 创建 Cloudflare Workers 的 fetch handler。
 *
 * 端点：
 * - POST /         运行 Agent（Body 为输入文本）
 * - POST /run       同上
 * - GET  /health    健康检查
 * - GET  /stream    SSE 流式运行（?input=xxx）
 * - POST /batch     批量运行（Body 为 string[]）
 */
export function createCloudflareAgentHandler(config: EdgeAgentTemplateConfig) {
  const agent = buildEdgeAgent({
    name: config.name ?? 'edge-agent',
    provider: config.provider,
    maxTurns: config.maxTurns,
    systemPrompt: config.systemPrompt,
  });

  const storage = config.storage ?? new MemoryEdgeStorage();
  const requestTimeoutMs = config.requestTimeoutMs ?? 30_000;
  const enableSSE = config.enableSSE ?? true;
  const enableHealth = config.enableHealth ?? true;
  let totalRequests = 0;
  let totalErrors = 0;
  const startTime = Date.now();

  async function handleRequest(request: Request): Promise<Response> {
    const url = new URL(request.url);

    // 健康检查
    if (enableHealth && url.pathname === '/health' && request.method === 'GET') {
      return new Response(
        JSON.stringify({
          healthy: totalErrors < totalRequests * 0.5,
          totalRequests,
          totalErrors,
          uptimeMs: Date.now() - startTime,
        }),
        { headers: { 'content-type': 'application/json' } },
      );
    }

    // 运行 Agent
    if ((url.pathname === '/' || url.pathname === '/run') && request.method === 'POST') {
      totalRequests++;
      const input = await request.text();

      try {
        const controller = new AbortController();
        const timeout = setTimeout(() => controller.abort(), requestTimeoutMs);

        try {
          const result = await agent.run(input);
          clearTimeout(timeout);

          // best-effort 存储写入
          try {
            await storage.set('last:input', input);
            await storage.set('last:output', result.content);
            await storage.set('last:timestamp', Date.now());
          } catch {
            // 存储失败不阻断响应
          }

          return new Response(JSON.stringify(result), {
            headers: { 'content-type': 'application/json' },
          });
        } catch (err) {
          clearTimeout(timeout);
          throw err;
        }
      } catch (err) {
        totalErrors++;
        return new Response(
          JSON.stringify({ error: (err as Error).message }),
          { status: 500, headers: { 'content-type': 'application/json' } },
        );
      }
    }

    // SSE 流式运行
    if (enableSSE && url.pathname === '/stream' && request.method === 'GET') {
      const input = url.searchParams.get('input') ?? '';
      totalRequests++;

      const encoder = new TextEncoder();
      const stream = new ReadableStream({
        async start(controller) {
          try {
            for await (const event of agent.streamEvents(input)) {
              const data = `data: ${JSON.stringify(event)}\n\n`;
              controller.enqueue(encoder.encode(data));
            }
            controller.enqueue(encoder.encode('data: [DONE]\n\n'));
          } catch (err) {
            totalErrors++;
            const errorData = `data: ${JSON.stringify({ type: 'error', error: (err as Error).message })}\n\n`;
            controller.enqueue(encoder.encode(errorData));
          } finally {
            controller.close();
          }
        },
      });

      return new Response(stream, {
        headers: {
          'content-type': 'text/event-stream',
          'cache-control': 'no-cache',
          'connection': 'keep-alive',
        },
      });
    }

    // 批量运行
    if (url.pathname === '/batch' && request.method === 'POST') {
      totalRequests++;
      try {
        const inputs: string[] = await request.json();
        const results = await Promise.all(
          inputs.map(async (input) => {
            try {
              const result = await agent.run(input);
              return { input, content: result.content, success: true };
            } catch (err) {
              return { input, error: (err as Error).message, success: false };
            }
          }),
        );
        return new Response(JSON.stringify(results), {
          headers: { 'content-type': 'application/json' },
        });
      } catch (err) {
        totalErrors++;
        return new Response(
          JSON.stringify({ error: (err as Error).message }),
          { status: 400, headers: { 'content-type': 'application/json' } },
        );
      }
    }

    return new Response('Not Found', { status: 404 });
  }

  return handleRequest;
}

// ===== Deno 模板 =====

/**
 * 创建 Deno 的 fetch handler（与 Cloudflare 接口兼容）。
 */
export function createDenoAgentHandler(config: EdgeAgentTemplateConfig) {
  // Deno 与 Cloudflare 的 fetch handler 接口相同
  return createCloudflareAgentHandler(config);
}

// ===== Bun 模板 =====

/**
 * 创建 Bun 的 fetch handler。
 */
export function createBunAgentHandler(config: EdgeAgentTemplateConfig) {
  return createCloudflareAgentHandler(config);
}

// ===== 通用 Edge Agent 模板（运行时无关） =====

/**
 * 创建运行时无关的 Edge Agent handler。
 * 自动检测运行时环境，适配不同 Edge 平台。
 */
export function createEdgeAgentHandler(config: EdgeAgentTemplateConfig) {
  const handler = createCloudflareAgentHandler(config);

  return async (request: Request): Promise<Response> => {
    // 可以在此添加运行时特定的适配逻辑
    return handler(request);
  };
}

// ===== wrangler.toml 模板生成 =====

/**
 * 生成 Cloudflare Workers 的 wrangler.toml 配置模板。
 */
export function generateWranglerConfig(opts: {
  name: string;
  compatibilityDate?: string;
  observability?: boolean;
}): string {
  const date = opts.compatibilityDate ?? new Date().toISOString().split('T')[0];
  const obs = opts.observability ?? true;

  return `# wrangler.toml — Auto-generated by AgentPrimordia v3.0
name = "${opts.name}"
main = "src/index.ts"
compatibility_date = "${date}"

[observability]
enabled = ${obs}

# 环境变量（在 CF Dashboard 中设置 secret）
# [vars]
# OPENAI_API_KEY = "..." # 建议使用 wrangler secret put OPENAI_API_KEY

# Durable Objects（可选，用于有状态 Agent）
# [[durable_objects.bindings]]
# name = "AGENT_DO"
# class_name = "AgentDurableObject"

# KV Namespace（可选，用于持久存储）
# [[kv_namespaces]]
# binding = "AGENT_KV"
# id = "..."
`;
}

// ===== 脚手架入口 =====

/**
 * 脚手架：生成 Edge Agent 项目文件。
 *
 * 使用方式：
 *   npx @agentprimordia/sdk create-edge-agent my-agent
 */
export interface ScaffoldConfig {
  /** 项目名称 */
  projectName: string;
  /** 运行时平台 */
  platform: 'cloudflare' | 'deno' | 'bun';
  /** Agent 名称 */
  agentName?: string;
  /** 系统提示词 */
  systemPrompt?: string;
  /** Provider 类型 */
  provider?: 'openai' | 'anthropic' | 'gemini';
}

/**
 * 生成 src/index.ts 入口文件内容。
 */
export function generateEntryFile(config: ScaffoldConfig): string {
  const agentName = config.agentName ?? config.projectName;
  const systemPrompt = config.systemPrompt ?? 'You are a helpful assistant.';
  const provider = config.provider ?? 'openai';

  return `// ${config.projectName} — Edge Agent
// Auto-generated by AgentPrimordia v3.0 Edge Agent Template
// Platform: ${config.platform}

import { createEdgeAgentHandler } from '@agentprimordia/sdk/edge';
import { createProvider } from '@agentprimordia/sdk/llm';

const handler = createEdgeAgentHandler({
  name: '${agentName}',
  provider: createProvider({
    type: '${provider}',
    apiKey: process.env.${provider.toUpperCase()}_API_KEY ?? '',
  }),
  systemPrompt: \`${systemPrompt}\`,
  maxTurns: 10,
  requestTimeoutMs: 30000,
});

export default { fetch: handler };
`;
}

/**
 * 生成 package.json 文件内容。
 */
export function generatePackageJSON(config: ScaffoldConfig): string {
  return JSON.stringify({
    name: config.projectName,
    version: '1.0.0',
    private: true,
    type: 'module',
    scripts: {
      dev: 'wrangler dev',
      deploy: 'wrangler deploy',
      'dev:deno': 'deno run --allow-net src/index.ts',
      'dev:bun': 'bun run src/index.ts',
    },
    dependencies: {
      '@agentprimordia/sdk': '^2.0.0',
    },
    devDependencies: {
      wrangler: '^3.0.0',
      typescript: '^5.0.0',
    },
  }, null, 2);
}

/**
 * 生成 tsconfig.json 文件内容。
 */
export function generateTSConfig(): string {
  return JSON.stringify({
    compilerOptions: {
      target: 'ES2022',
      module: 'ES2022',
      moduleResolution: 'bundler',
      strict: true,
      esModuleInterop: true,
      skipLibCheck: true,
      lib: ['ES2022'],
      types: ['@cloudflare/workers-types'],
    },
    include: ['src/**/*.ts'],
  }, null, 2);
}
