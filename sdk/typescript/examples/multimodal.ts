/**
 * 多模态示例 — 视觉理解 + 多模态融合
 *
 * 运行: npx tsx examples/multimodal.ts
 */
import { MultimodalFusion } from '../src/llm/multimodal-fusion.js';
import type { Provider } from '../src/llm/provider.js';
import type { CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, ModelInfo } from '../src/types.js';

const visionProvider: Provider = {
  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    const hasImage = req.messages.some(m => m.content.includes('[image]'));
    const content = hasImage
      ? 'I can see an image showing a landscape with mountains and a lake.'
      : 'No image detected in the input.';
    return { id: 'vision-' + Date.now(), content, role: 'assistant', usage: { promptTokens: 100, completionTokens: 20, totalTokens: 120 } };
  },
  async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
    return { content: '', toolCalls: [], usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 } };
  },
  info(): ModelInfo {
    return { name: 'vision-model', provider: 'demo', maxContext: 8192, supportsTools: false, supportsStreaming: false };
  },
};

async function main() {
  console.log('=== AgentPrimordia TS SDK: Multimodal ===\n');

  const fusion = new MultimodalFusion({ provider: visionProvider });

  // 纯文本输入
  console.log('--- Text Only ---');
  const textResult = await fusion.process({ text: 'Describe this scene' });
  console.log(`Text result: ${textResult.content}`);

  // 图文混合输入
  console.log('\n--- Image + Text ---');
  const imageResult = await fusion.process({
    text: 'What is in this image?',
    images: [{ url: 'https://example.com/landscape.jpg', detail: 'high' }],
  });
  console.log(`Vision result: ${imageResult.content}`);

  // 多模态能力检测
  console.log('\n--- Capability Detection ---');
  console.log(`Supports vision: ${fusion.supportsVision()}`);
  console.log(`Supports audio: ${fusion.supportsAudio()}`);
  console.log(`Max image size: ${fusion.maxImageSize()}px`);

  console.log('\n--- Done ---');
}

main().catch(console.error);
