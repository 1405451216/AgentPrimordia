/**
 * 多 Provider 路由示例 — 模型路由 + 隐私路由 + 批处理
 *
 * 运行: npx tsx examples/provider-multi.ts
 */
import { ModelRouter } from '../src/llm/model-router.js';
import { PrivacyRouter } from '../src/llm/privacy-router.js';
import type { Provider } from '../src/llm/provider.js';
import type { CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, ModelInfo } from '../src/types.js';

function mockProvider(name: string): Provider {
  return {
    async complete(req: CompletionRequest): Promise<CompletionResponse> {
      return { id: `${name}-${Date.now()}`, content: `[${name}] Response to: ${req.messages[req.messages.length - 1]?.content?.slice(0, 30)}`, role: 'assistant', usage: { promptTokens: 5, completionTokens: 8, totalTokens: 13 } };
    },
    async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
      return { content: '', toolCalls: [], usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 } };
    },
    info(): ModelInfo {
      return { name, provider: name, maxContext: 8192, supportsTools: true, supportsStreaming: true };
    },
  };
}

async function main() {
  console.log('=== AgentPrimordia TS SDK: Multi-Provider Routing ===\n');

  // 1. 模型路由：根据任务复杂度选择不同模型
  console.log('--- Model Router ---');
  const router = new ModelRouter({
    routes: [
      { pattern: /简单|hello|hi/i, provider: mockProvider('small-model'), priority: 1 },
      { pattern: /分析|推理|复杂/i, provider: mockProvider('large-model'), priority: 2 },
      { pattern: /.*/, provider: mockProvider('default-model'), priority: 0 },
    ],
  });

  const simpleResp = await router.complete({ messages: [{ role: 'user', content: 'Hello!' }] });
  console.log(`Simple query → ${simpleResp.content}`);

  const complexResp = await router.complete({ messages: [{ role: 'user', content: '分析这段代码的性能瓶颈' }] });
  console.log(`Complex query → ${complexResp.content}`);

  // 2. 隐私路由：敏感数据走本地模型
  console.log('\n--- Privacy Router ---');
  const privacyRouter = new PrivacyRouter({
    localProvider: mockProvider('local-llama'),
    remoteProvider: mockProvider('cloud-gpt4'),
    sensitivePatterns: [/密码|password|SSN|身份证|银行卡/],
  });

  const normalResp = await privacyRouter.complete({ messages: [{ role: 'user', content: 'What is AI?' }] });
  console.log(`Normal → ${normalResp.content}`);

  const sensitiveResp = await privacyRouter.complete({ messages: [{ role: 'user', content: '我的密码是 abc123' }] });
  console.log(`Sensitive → ${sensitiveResp.content}`);

  console.log('\n--- Done ---');
}

main().catch(console.error);
