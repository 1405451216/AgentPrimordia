/**
 * 流式输出示例 — StreamRun 逐 token 消费
 *
 * 运行: npx tsx examples/streaming.ts
 */
import { ReActAgent } from '../src/agent/react-loop.js';
import type { Provider } from '../src/llm/provider.js';
import type { CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, ModelInfo } from '../src/types.js';

const streamProvider: Provider = {
  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    return {
      id: 'stream-' + Date.now(),
      content: 'This is a streamed response from AgentPrimordia.',
      role: 'assistant',
      usage: { promptTokens: 8, completionTokens: 12, totalTokens: 20 },
    };
  },
  async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
    return { content: '', toolCalls: [], usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 } };
  },
  info(): ModelInfo {
    return { name: 'stream-model', provider: 'demo', maxContext: 4096, supportsTools: false, supportsStreaming: true };
  },
};

async function main() {
  console.log('=== AgentPrimordia TS SDK: Streaming Example ===\n');

  const agent = new ReActAgent({
    name: 'StreamAgent',
    systemPrompt: 'You are a helpful assistant.',
    provider: streamProvider,
    maxTurns: 3,
  });

  // 流式运行
  console.log('Streaming response:');
  const stream = await agent.streamRun('Tell me about AI agents');
  for await (const event of stream) {
    if (event.type === 'token') {
      process.stdout.write(event.content);
    } else if (event.type === 'complete') {
      console.log(`\n[Complete] ${event.content}`);
    }
  }

  console.log('\n--- Done ---');
}

main().catch(console.error);
