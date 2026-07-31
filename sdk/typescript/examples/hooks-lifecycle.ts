/**
 * 生命周期钩子示例 — 20+ 钩子点监控 Agent 执行
 *
 * 运行: npx tsx examples/hooks-lifecycle.ts
 */
import { ReActAgent } from '../src/agent/react-loop.js';
import { HookManager } from '../src/agent/hooks.js';
import type { Provider } from '../src/llm/provider.js';
import type { CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, ModelInfo } from '../src/types.js';

const provider: Provider = {
  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    return { id: 'hook-demo', content: `Processed: ${req.messages[req.messages.length - 1]?.content}`, role: 'assistant', usage: { promptTokens: 5, completionTokens: 5, totalTokens: 10 } };
  },
  async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
    return { content: '', toolCalls: [], usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 } };
  },
  info(): ModelInfo {
    return { name: 'hook-model', provider: 'demo', maxContext: 4096, supportsTools: false, supportsStreaming: false };
  },
};

async function main() {
  console.log('=== AgentPrimordia TS SDK: Hooks & Lifecycle ===\n');

  const hooks = new HookManager();

  // 注册各阶段钩子
  hooks.register('before_run', (ctx) => console.log(`[Hook] before_run: agent=${ctx.agentID}`));
  hooks.register('before_turn', (ctx) => console.log(`[Hook] before_turn: turn=${ctx.turn}`));
  hooks.register('after_turn', (ctx) => console.log(`[Hook] after_turn: turn=${ctx.turn}`));
  hooks.register('on_complete', (ctx) => console.log(`[Hook] on_complete: agent=${ctx.agentID}`));
  hooks.register('on_error', (ctx) => console.log(`[Hook] on_error: ${ctx.error}`));

  const agent = new ReActAgent({
    name: 'HookedAgent',
    systemPrompt: 'You are monitored by hooks.',
    provider,
    maxTurns: 2,
    hooks,
  });

  console.log('Running agent with hooks...\n');
  const response = await agent.run('Hello with hooks!');
  console.log(`\nFinal response: ${response.content}`);
  console.log('--- Done ---');
}

main().catch(console.error);
