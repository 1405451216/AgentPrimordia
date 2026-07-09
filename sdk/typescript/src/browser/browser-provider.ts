/**
 * 浏览器端本地推理 Provider（T3-2）。
 *
 * 设计：实现标准 Provider 接口，优先使用 WebGPU 在浏览器本地推理，
 * 让数据不离开浏览器。在 Node / 无 WebGPU 环境自动进入「模拟推理」模式，
 * 返回确定性结果，便于单测与无 GPU 开发。
 *
 * 注意：真实 WebGPU compute shader 推理是平台相关代码；本实现给出接口骨架
 * 与可运行的模拟路径，真实权重加载/推理可在浏览器构建期接入。
 */

import type {
  CompletionRequest,
  CompletionResponse,
  ToolCallRequest,
  ToolCallResponse,
  Chunk,
  ModelInfo,
} from '../types.js';

export class WebGPUProvider {
  private modelUrl: string | null = null;
  private simulated = true;
  private device: unknown = null;

  /** 初始化：尝试请求 WebGPU adapter；不可用则进入模拟模式 */
  async init(modelUrl?: string): Promise<void> {
    this.modelUrl = modelUrl ?? null;
    const nav = (globalThis as { navigator?: { gpu?: { requestAdapter(): Promise<unknown> } } }).navigator;
    if (nav?.gpu) {
      try {
        this.device = await nav.gpu.requestAdapter();
        if (this.device) this.simulated = false;
      } catch {
        this.simulated = true;
      }
    }
  }

  /** 是否在模拟模式（无 GPU） */
  isSimulated(): boolean {
    return this.simulated;
  }

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    const text = this.infer(req.messages.map((m) => m.content).join(' '));
    return {
      id: 'webgpu-' + Math.random().toString(36).slice(2),
      content: text,
      role: 'assistant',
      usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 },
    };
  }

  async *stream(req: CompletionRequest): AsyncIterable<Chunk> {
    const text = this.infer(req.messages.map((m) => m.content).join(' '));
    const words = text.split(' ');
    for (let i = 0; i < words.length; i++) {
      yield { content: words[i] + (i < words.length - 1 ? ' ' : ''), done: i === words.length - 1 };
    }
  }

  async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
    // 本地模型桩：当前不发起工具调用，交由上层 ReAct 引擎处理
    return {
      content: this.infer(_req.messages.map((m) => m.content).join(' ')),
      toolCalls: [],
      usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 },
    };
  }

  info(): ModelInfo {
    return {
      name: this.modelUrl ?? 'webgpu-local',
      provider: 'webgpu',
      maxContext: 4096,
      supportsTools: false,
      supportsStreaming: true,
    };
  }

  /** 本地推理（模拟）：确定性回显，便于测试与可重现 */
  private infer(prompt: string): string {
    const snippet = prompt.length > 64 ? prompt.slice(0, 64) + '…' : prompt;
    return `[webgpu-local] response to: ${snippet}`;
  }
}
