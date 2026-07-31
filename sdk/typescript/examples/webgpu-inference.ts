/**
 * WebGPU 本地推理示例 — 可插拔推理后端（v3.2.0 新增）
 *
 * 运行: npx tsx examples/webgpu-inference.ts
 *
 * 前置条件（启用真实推理）:
 *   npm install @xenova/transformers
 *
 * 未安装时自动回退到骨架后端（echo 模式）。
 */
import {
  detectInferenceBackend,
  TransformersBackend,
  SkeletonBackend,
  DEFAULT_INFERENCE_CONFIG,
} from '../src/llm/webgpu-model-runner.js';

async function main() {
  console.log('=== AgentPrimordia TS SDK: WebGPU Inference Backend ===\n');

  // 自动检测可用推理后端
  const backend = await detectInferenceBackend();
  console.log(`Detected backend: ${backend.name}`);

  if (backend.name === 'skeleton') {
    console.log('(@xenova/transformers 未安装，使用骨架后端)');
    console.log('安装方式: npm install @xenova/transformers\n');
  }

  // 加载模型
  console.log('Loading model...');
  await backend.load('Xenova/phi-2', DEFAULT_INFERENCE_CONFIG);
  console.log('Model loaded.\n');

  // 执行推理
  const prompt = 'Explain what an AI agent framework is in one sentence:';
  console.log(`Prompt: ${prompt}`);
  const output = await backend.generate(prompt, 64);
  console.log(`Output: ${output}\n`);

  // 释放资源
  backend.dispose();
  console.log('Backend disposed.');

  // 演示手动选择后端
  console.log('\n--- Manual Backend Selection ---');
  const skeleton = new SkeletonBackend();
  await skeleton.load('any-model', DEFAULT_INFERENCE_CONFIG);
  const echoOutput = await skeleton.generate('Hello WebGPU!', 32);
  console.log(`Skeleton output: ${echoOutput}`);

  console.log('\n--- Done ---');
}

main().catch(console.error);
