/**
 * 基础 Agent 示例 — 最小创建与运行
 *
 * 运行: npx tsx examples/basic.ts
 */
import { ReActAgent } from '../src/agent/react-loop.js';
import type { Provider } from '../src/llm/provider.js';
import type { CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, ModelInfo } from '../src/types.js';

// 简单 echo Provider（无需真实 API Key）
const echoProvider: Provider = {
  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    const lastMsg = req.messages[req.messages.length - 1];
    return {
      id: 'demo-' + Date.now(),
      content: `Echo: ${lastMsg?.content ?? ''}`,
      role: 'assistant',
      usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 },
    };
  },
  async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
    return { content: '', toolCalls: [], usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 } };
  },
  info(): ModelInfo {
    return { name: 'echo-model', provider: 'demo', maxContext: 4096, supportsTools: false, supportsStreaming: false };
  },
};

async function main() {
  console.log('=== AgentPrimordia TS SDK: Basic Example ===\n');

  const agent = new ReActAgent({
    name: 'MyFirstAgent',
    systemPrompt: 'You are a helpful assistant.',
    provider: echoProvider,
    maxTurns: 3,
  });

  const response = await agent.run('Hello, AgentPrimordia!');
  console.log(`Response: ${response.content}`);
  console.log(`Turns used: ${response.metrics?.totalTurns ?? 1}`);
  console.log('\n--- Done ---');
}

main().catch(console.error);
