/**
 * 多 Agent 编排示例 — Pipeline / Parallel 模式
 *
 * 运行: npx tsx examples/multi-agent.ts
 */
import { Pipeline, Parallel } from '../src/orchestration/advanced.js';
import type { Provider } from '../src/llm/provider.js';
import type { CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, ModelInfo } from '../src/types.js';

// 模拟 Provider：根据系统提示词返回不同响应
function createNamedProvider(name: string): Provider {
  return {
    async complete(req: CompletionRequest): Promise<CompletionResponse> {
      const input = req.messages[req.messages.length - 1]?.content ?? '';
      return {
        id: `${name}-${Date.now()}`,
        content: `[${name}] processed: ${input.slice(0, 50)}`,
        role: 'assistant',
        usage: { promptTokens: 5, completionTokens: 10, totalTokens: 15 },
      };
    },
    async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
      return { content: '', toolCalls: [], usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 } };
    },
    info(): ModelInfo {
      return { name, provider: 'demo', maxContext: 4096, supportsTools: false, supportsStreaming: false };
    },
  };
}

async function main() {
  console.log('=== AgentPrimordia TS SDK: Multi-Agent Orchestration ===\n');

  // Pipeline 模式：Agent 按顺序执行，前一个的输出作为后一个的输入
  console.log('--- Pipeline Mode ---');
  const pipeline = new Pipeline({
    stages: [
      { name: 'researcher', provider: createNamedProvider('Researcher') },
      { name: 'writer', provider: createNamedProvider('Writer') },
      { name: 'reviewer', provider: createNamedProvider('Reviewer') },
    ],
  });

  const pipelineResult = await pipeline.run('Write a report about AI agents');
  console.log(`Pipeline result: ${pipelineResult.content}\n`);

  // Parallel 模式：所有 Agent 并行执行
  console.log('--- Parallel Mode ---');
  const parallel = new Parallel({
    agents: [
      { name: 'analyst', provider: createNamedProvider('Analyst') },
      { name: 'critic', provider: createNamedProvider('Critic') },
    ],
  });

  const parallelResult = await parallel.run('Evaluate this proposal');
  console.log(`Parallel results: ${parallelResult.length} responses`);
  for (const r of parallelResult) {
    console.log(`  - ${r.content}`);
  }

  console.log('\n--- Done ---');
}

main().catch(console.error);
