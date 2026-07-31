/**
 * 生产级 Agent 完整示例 — 组合所有核心能力
 *
 * 运行: npx tsx examples/production-agent.ts
 *
 * 演示：Agent + Tools + Memory + Hooks + Guardrails + Metrics 完整集成
 */
import { ReActAgent } from '../src/agent/react-loop.js';
import { ToolRegistry } from '../src/tools/registry.js';
import { HookManager } from '../src/agent/hooks.js';
import { InMemoryStore } from '../src/memory/store.js';
import { GuardrailEngine, PromptInjectionRule } from '../src/security/guardrails.js';
import { MetricsCollector } from '../src/metrics/collector.js';
import type { Provider } from '../src/llm/provider.js';
import type { CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, ModelInfo } from '../src/types.js';

// 模拟生产 LLM Provider
const productionProvider: Provider = {
  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    const input = req.messages[req.messages.length - 1]?.content ?? '';
    return {
      id: `prod-${Date.now()}`,
      content: `I analyzed your request: "${input.slice(0, 50)}". Here is my comprehensive response.`,
      role: 'assistant',
      usage: { promptTokens: 150, completionTokens: 80, totalTokens: 230 },
    };
  },
  async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
    return { content: '', toolCalls: [], usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 } };
  },
  info(): ModelInfo {
    return { name: 'production-model', provider: 'anthropic', maxContext: 200000, supportsTools: true, supportsStreaming: true };
  },
};

async function main() {
  console.log('=== AgentPrimordia TS SDK: Production Agent ===\n');
  const startTime = Date.now();

  // 1. 工具注册
  const tools = new ToolRegistry();
  tools.register({
    name: 'web_search',
    description: 'Search the web for information',
    parameters: { type: 'object', properties: { query: { type: 'string' } }, required: ['query'] },
    execute: async (args) => ({ content: `Search results for "${args.query}": [3 relevant documents found]` }),
  });
  tools.register({
    name: 'code_executor',
    description: 'Execute code in a sandboxed environment',
    parameters: { type: 'object', properties: { code: { type: 'string' }, language: { type: 'string' } }, required: ['code'] },
    execute: async (args) => ({ content: `Executed ${args.language ?? 'python'} code: OK` }),
  });

  // 2. 生命周期钩子
  const hooks = new HookManager();
  hooks.register('before_run', () => console.log('[Monitor] Agent starting...'));
  hooks.register('on_complete', () => console.log('[Monitor] Agent completed successfully'));
  hooks.register('on_error', (ctx) => console.error(`[Alert] Agent error: ${ctx.error}`));

  // 3. 护栏
  const guardrail = new GuardrailEngine({ rules: [new PromptInjectionRule()] });

  // 4. 记忆
  const memory = new InMemoryStore();

  // 5. 指标
  const metrics = new MetricsCollector();

  // 6. 创建生产 Agent
  const agent = new ReActAgent({
    name: 'ProductionAgent',
    systemPrompt: 'You are a production-grade AI assistant with web search and code execution capabilities.',
    provider: productionProvider,
    toolkit: tools,
    maxTurns: 5,
    hooks,
  });

  // 7. 输入验证（护栏检查）
  const userInput = 'Help me analyze the performance of our API endpoints';
  console.log(`User input: "${userInput}"`);

  const guardResult = await guardrail.check(userInput);
  if (!guardResult.passed) {
    console.log('BLOCKED by guardrail!');
    return;
  }
  console.log('Guardrail: PASSED\n');

  // 8. 执行 Agent
  metrics.recordLLMCall(0, false); // 开始计时
  const response = await agent.run(userInput);
  metrics.recordLLMCall(Date.now() - startTime, false);

  console.log(`Response: ${response.content}`);
  console.log(`Turns: ${response.metrics?.totalTurns ?? 1}`);

  // 9. 保存记忆
  await memory.add({ id: `ep-${Date.now()}`, sessionId: 'prod-session', role: 'user', content: userInput });
  await memory.add({ id: `ep-${Date.now() + 1}`, sessionId: 'prod-session', role: 'assistant', content: response.content });

  // 10. 输出指标
  const snapshot = metrics.snapshot();
  console.log(`\n--- Metrics ---`);
  console.log(`LLM calls: ${snapshot.llm_total_calls}`);
  console.log(`Total duration: ${Date.now() - startTime}ms`);
  console.log(`Tools available: ${tools.count()}`);
  console.log(`Memory entries: ${(await memory.search('')).length}`);

  console.log('\n--- Production Agent Complete ---');
}

main().catch(console.error);
