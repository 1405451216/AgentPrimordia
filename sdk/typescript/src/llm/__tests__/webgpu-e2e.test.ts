/**
 * webgpu-e2e.test.ts — WebGPU 端到端推理集成测试。
 *
 * 使用 mock GPU 设备模拟完整的推理流程：
 * - 初始化 → 模型加载 → chat 推理 → 流式输出
 * - 验证 chat/stream 输出格式
 * - 集成测试标签（@integration）
 *
 * 所有测试使用 mock WebGPU API，可在无 GPU 的 CI 环境运行。
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  WebGPUProvider,
  MockWebGPURuntime,
  type ChatChunk,
  type ProviderResponse,
} from '../webgpu-provider.js';
import {
  WebGPUModelRunner,
  DEFAULT_INFERENCE_CONFIG,
} from '../webgpu-model-runner.js';

// ===== Mock GPU 环境 =====

function createMockGPUDevice() {
  return {
    limits: {
      maxBufferSize: 1 << 30,
      maxStorageBufferBindingSize: 1 << 28,
      maxComputeWorkgroupStorageSize: 16384,
      maxComputeWorkgroupsPerDimension: 65535,
    },
    createBuffer: vi.fn().mockReturnValue({
      getMappedRange: () => new ArrayBuffer(1024),
      unmap: vi.fn(),
    }),
  };
}

function createMockGPUAdapter(device: any = createMockGPUDevice()) {
  return {
    requestDevice: vi.fn().mockResolvedValue(device),
    info: { description: 'E2E Mock GPU', vendor: 'test', architecture: 'test' },
    features: new Set(['timestamp-query']),
    requestAdapterInfo: vi.fn().mockResolvedValue({ description: 'E2E Mock GPU' }),
  };
}

function setupMockNavigator(adapter: any = createMockGPUAdapter()) {
  const gpu = { requestAdapter: vi.fn().mockResolvedValue(adapter) };
  Object.defineProperty(globalThis, 'navigator', {
    value: { gpu },
    writable: true,
    configurable: true,
  });
}

function clearMockNavigator() {
  Object.defineProperty(globalThis, 'navigator', {
    value: undefined,
    writable: true,
    configurable: true,
  });
}

// ===== E2E: 完整推理流程 =====

describe('@integration WebGPU E2E inference pipeline', () => {
  let runtime: MockWebGPURuntime;
  let provider: WebGPUProvider;

  beforeEach(async () => {
    setupMockNavigator();
    runtime = new MockWebGPURuntime({
      response: 'The answer is 42.',
    });
    provider = new WebGPUProvider({ runtime, defaultModel: 'e2e-test-model' });
  });

  afterEach(() => {
    clearMockNavigator();
  });

  it('full lifecycle: init → load → chat → unload', async () => {
    // Step 1: Initialize
    const caps = await provider.initialize();
    expect(caps.supported).toBe(true);
    expect(provider.isInitialized()).toBe(true);

    // Step 2: Load model
    await provider.loadModel('e2e-model');
    expect(provider.isModelLoaded()).toBe(true);
    expect(provider.getCurrentModel()).toBe('e2e-model');

    // Step 3: Chat inference
    const response = await provider.chat([
      { role: 'system', content: 'You are a helpful assistant.' },
      { role: 'user', content: 'What is the answer?' },
    ]);

    expect(response.content).toBe('The answer is 42.');
    expect(response.model).toBe('e2e-model');
    expect(response.usage).toBeDefined();
    expect(response.usage.totalTokens).toBeGreaterThan(0);

    // Step 4: Unload
    await provider.unloadModel();
    expect(provider.isModelLoaded()).toBe(false);
  });

  it('stream output format should yield incremental chunks', async () => {
    await provider.loadModel('e2e-model');

    const chunks: ChatChunk[] = [];
    for await (const chunk of provider.stream([
      { role: 'user', content: 'Tell me something' },
    ])) {
      chunks.push(chunk);
    }

    // Should have multiple chunks
    expect(chunks.length).toBeGreaterThan(1);

    // Each chunk should have content string
    for (const chunk of chunks) {
      expect(typeof chunk.content).toBe('string');
      expect(typeof chunk.done).toBe('boolean');
    }

    // Only the last chunk should be done
    for (let i = 0; i < chunks.length - 1; i++) {
      expect(chunks[i].done).toBe(false);
    }
    expect(chunks[chunks.length - 1].done).toBe(true);

    // Concatenated content should reconstruct the response
    const fullText = chunks.map(c => c.content).join('');
    expect(fullText).toBe('The answer is 42.');
  });

  it('info() should reflect current model state', async () => {
    // Before loading
    const infoBefore = provider.info();
    expect(infoBefore.name).toBe('e2e-test-model');
    expect(infoBefore.provider).toBe('webgpu');
    expect(infoBefore.supportsStreaming).toBe(true);

    // After loading
    await provider.loadModel('custom-model');
    const infoAfter = provider.info();
    expect(infoAfter.name).toBe('custom-model');
  });
});

// ===== E2E: WebGPUModelRunner 推理流程 =====

describe('@integration WebGPUModelRunner E2E', () => {
  beforeEach(() => {
    setupMockNavigator();
  });

  afterEach(() => {
    clearMockNavigator();
  });

  it('should initialize and detect GPU capabilities', async () => {
    const runner = new WebGPUModelRunner({
      modelId: 'e2e-runner-model',
      inference: { maxTokens: 256 },
    });

    const caps = await runner.init();
    expect(caps.supported).toBe(true);
    expect(caps.adapterName).toBeDefined();
    expect(runner.isAvailable()).toBe(true);
    expect(runner.isModelLoaded()).toBe(false);
  });

  it('should report progress through lifecycle', async () => {
    const progressLog: string[] = [];
    const runner = new WebGPUModelRunner({
      modelId: 'e2e-runner-model',
      onProgress: (p) => progressLog.push(p.state),
    });

    await runner.init();
    expect(runner.getProgress().state).toBe('idle');

    runner.unloadModel();
    expect(progressLog).toContain('idle');
  });

  it('getProgress should return immutable snapshot', async () => {
    const runner = new WebGPUModelRunner({ modelId: 'test' });
    const p1 = runner.getProgress();
    const p2 = runner.getProgress();

    expect(p1).toEqual(p2);
    expect(p1).not.toBe(p2);
  });
});

// ===== E2E: 降级场景 =====

describe('@integration WebGPU fallback scenarios', () => {
  afterEach(() => {
    clearMockNavigator();
  });

  it('should handle WebGPU unavailable gracefully', async () => {
    clearMockNavigator();

    const provider = new WebGPUProvider();
    const caps = await provider.initialize();

    expect(caps.supported).toBe(false);
    expect(caps.errorMessage).toBeDefined();

    // loadModel should throw meaningful error
    await expect(provider.loadModel('any-model')).rejects.toThrow();
  });

  it('createWithFallback should return remote when no GPU', () => {
    clearMockNavigator();
    const provider = WebGPUProvider.createWithFallback('http://api.example.com');
    expect(provider).not.toBeInstanceOf(WebGPUProvider);
  });
});
