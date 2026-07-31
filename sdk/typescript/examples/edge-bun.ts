/**
 * 边缘运行时示例 — Bun Edge Agent（重试/限流/健康检查）
 *
 * 运行: npx tsx examples/edge-bun.ts
 */
import { BunEdgeAgent } from '../src/edge/bun-agent.js';
import { MemoryEdgeStorage } from '../src/edge/edge-storage.js';
import type { Provider } from '../src/llm/provider.js';
import type { CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, ModelInfo } from '../src/types.js';

const mockProvider: Provider = {
  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    const input = req.messages[req.messages.length - 1]?.content ?? '';
    return {
      id: 'edge-' + Date.now(),
      content: `Edge response to: ${input.slice(0, 30)}`,
      role: 'assistant',
      usage: { promptTokens: 5, completionTokens: 8, totalTokens: 13 },
    };
  },
  async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
    return { content: '', toolCalls: [], usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 } };
  },
  info(): ModelInfo {
    return { name: 'edge-model', provider: 'demo', maxContext: 2048, supportsTools: false, supportsStreaming: false };
  },
};

async function main() {
  console.log('=== AgentPrimordia TS SDK: Edge Bun Agent ===\n');

  const agent = new BunEdgeAgent({
    name: 'edge-agent',
    provider: mockProvider,
    storage: new MemoryEdgeStorage(),
    maxTurns: 3,
    systemPrompt: 'You are a lightweight edge agent.',
    // 生产配置
    requestTimeoutMs: 10_000,
    maxRetries: 2,
    retryBaseDelayMs: 500,
    rateLimitPerMinute: 30,
  });

  // 执行推理（带重试和超时保护）
  const result = await agent.runDetailed('What is the weather?');
  console.log(`Response: ${result.content}`);
  console.log(`Duration: ${result.durationMs}ms`);
  console.log(`Retries: ${result.retries}`);

  // 健康检查
  const health = agent.health();
  console.log(`\nHealth: healthy=${health.healthy}, requests=${health.totalRequests}, errors=${health.totalErrors}`);
  console.log(`Rate limit remaining: ${health.rateLimitRemaining}/30`);

  // 清理
  agent.close();
  console.log('\n--- Done ---');
}

main().catch(console.error);
