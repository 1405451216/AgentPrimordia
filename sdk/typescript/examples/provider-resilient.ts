/**
 * 弹性 Provider 示例 — 自动重试 / 降级 / 熔断
 *
 * 运行: npx tsx examples/provider-resilient.ts
 */
import { ResilientProvider } from '../src/llm/resilient.js';
import type { Provider } from '../src/llm/provider.js';
import type { CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, ModelInfo } from '../src/types.js';

// 模拟不稳定的 Provider（50% 概率失败）
const unstableProvider: Provider = {
  async complete(_req: CompletionRequest): Promise<CompletionResponse> {
    if (Math.random() < 0.5) throw new Error('API rate limited');
    return { id: 'ok', content: 'Success!', role: 'assistant', usage: { promptTokens: 5, completionTokens: 3, totalTokens: 8 } };
  },
  async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
    return { content: '', toolCalls: [], usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 } };
  },
  info(): ModelInfo {
    return { name: 'unstable', provider: 'demo', maxContext: 4096, supportsTools: false, supportsStreaming: false };
  },
};

// 降级备用 Provider
const fallbackProvider: Provider = {
  async complete(_req: CompletionRequest): Promise<CompletionResponse> {
    return { id: 'fallback', content: 'Fallback response (degraded mode)', role: 'assistant', usage: { promptTokens: 5, completionTokens: 5, totalTokens: 10 } };
  },
  async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
    return { content: '', toolCalls: [], usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 } };
  },
  info(): ModelInfo {
    return { name: 'fallback', provider: 'demo', maxContext: 2048, supportsTools: false, supportsStreaming: false };
  },
};

async function main() {
  console.log('=== AgentPrimordia TS SDK: Resilient Provider ===\n');

  const resilient = new ResilientProvider({
    primary: unstableProvider,
    fallback: fallbackProvider,
    maxRetries: 3,
    retryDelayMs: 100,
    circuitBreakerThreshold: 5,
  });

  // 多次调用观察重试和降级行为
  for (let i = 1; i <= 5; i++) {
    try {
      const resp = await resilient.complete({ messages: [{ role: 'user', content: `Request #${i}` }] });
      console.log(`#${i}: [${resp.id}] ${resp.content}`);
    } catch (err) {
      console.log(`#${i}: Failed - ${(err as Error).message}`);
    }
  }

  console.log(`\nProvider info: ${resilient.info().name}`);
  console.log('--- Done ---');
}

main().catch(console.error);
