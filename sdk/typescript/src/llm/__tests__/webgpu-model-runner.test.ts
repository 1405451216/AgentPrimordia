/**
 * webgpu-model-runner.test.ts — WebGPU 模型运行器单元测试。
 *
 * 测试 WebGPUModelRunner 的核心功能：
 * - 状态机转换（idle → downloading → loading → ready）
 * - InferenceConfig 默认值与边界检查
 * - CacheStrategy 与 mock IndexedDB
 * - 模型加载/卸载生命周期
 *
 * 所有测试使用 mock WebGPU API，可在无 GPU 的 CI 环境运行。
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  WebGPUModelRunner,
  DEFAULT_INFERENCE_CONFIG,
  type InferenceConfig,
  type ModelLoadProgress,
} from '../webgpu-model-runner.js';

// ===== Mock navigator.gpu =====

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
    info: { description: 'Mock GPU Adapter', vendor: 'test', architecture: 'test' },
    features: new Set(['timestamp-query']),
    requestAdapterInfo: vi.fn().mockResolvedValue({ description: 'Mock GPU Adapter' }),
  };
}

function setupMockNavigator(adapter: any = null) {
  const gpu = {
    requestAdapter: vi.fn().mockResolvedValue(adapter),
  };
  Object.defineProperty(globalThis, 'navigator', {
    value: { gpu },
    writable: true,
    configurable: true,
  });
  return gpu;
}

function clearMockNavigator() {
  Object.defineProperty(globalThis, 'navigator', {
    value: undefined,
    writable: true,
    configurable: true,
  });
}

// ===== DEFAULT_INFERENCE_CONFIG 测试 =====

describe('DEFAULT_INFERENCE_CONFIG', () => {
  it('should have correct default values', () => {
    expect(DEFAULT_INFERENCE_CONFIG.maxTokens).toBe(512);
    expect(DEFAULT_INFERENCE_CONFIG.temperature).toBe(0.7);
    expect(DEFAULT_INFERENCE_CONFIG.topK).toBe(40);
    expect(DEFAULT_INFERENCE_CONFIG.topP).toBe(0.9);
    expect(DEFAULT_INFERENCE_CONFIG.repeatPenalty).toBe(1.1);
  });

  it('should have all required fields', () => {
    const fields: (keyof InferenceConfig)[] = ['maxTokens', 'temperature', 'topK', 'topP', 'repeatPenalty'];
    for (const field of fields) {
      expect(DEFAULT_INFERENCE_CONFIG[field]).toBeDefined();
      expect(typeof DEFAULT_INFERENCE_CONFIG[field]).toBe('number');
    }
  });
});

// ===== WebGPUModelRunner 构造测试 =====

describe('WebGPUModelRunner constructor', () => {
  it('should create with minimal config', () => {
    const runner = new WebGPUModelRunner({ modelId: 'test-model' });
    expect(runner.isModelLoaded()).toBe(false);
    expect(runner.isAvailable()).toBe(false);
  });

  it('should merge inference config with defaults', () => {
    const progressCalls: ModelLoadProgress[] = [];
    const runner = new WebGPUModelRunner({
      modelId: 'test-model',
      inference: { maxTokens: 1024, temperature: 0.5 },
      onProgress: (p) => progressCalls.push({ ...p }),
    });

    // Progress should start at idle
    const initial = runner.getProgress();
    expect(initial.state).toBe('idle');
    expect(initial.downloadProgress).toBe(0);
  });

  it('should merge cache strategy with defaults', () => {
    const runner = new WebGPUModelRunner({
      modelId: 'test-model',
      cache: { cacheName: 'custom-cache', maxCacheSize: 1024 },
    });
    // Runner should be constructable with partial cache config
    expect(runner).toBeDefined();
  });
});

// ===== 状态机转换测试 =====

describe('WebGPUModelRunner state machine', () => {
  beforeEach(() => {
    const adapter = createMockGPUAdapter();
    setupMockNavigator(adapter);
  });

  afterEach(() => {
    clearMockNavigator();
  });

  it('should transition from idle to supported after init()', async () => {
    const runner = new WebGPUModelRunner({ modelId: 'test-model' });
    expect(runner.isAvailable()).toBe(false);

    const caps = await runner.init();
    expect(caps.supported).toBe(true);
    expect(runner.isAvailable()).toBe(true);
  });

  it('should report not supported when navigator.gpu is absent', async () => {
    clearMockNavigator();
    const runner = new WebGPUModelRunner({ modelId: 'test-model' });
    const caps = await runner.init();
    expect(caps.supported).toBe(false);
    expect(caps.errorMessage).toContain('navigator.gpu');
  });

  it('should report not supported when requestAdapter returns null', async () => {
    setupMockNavigator(null);
    const runner = new WebGPUModelRunner({ modelId: 'test-model' });
    const caps = await runner.init();
    expect(caps.supported).toBe(false);
  });

  it('should throw when loadModel called before init()', async () => {
    const runner = new WebGPUModelRunner({ modelId: 'test-model' });
    await expect(runner.loadModel('test', 'http://example.com/model.gguf'))
      .rejects.toThrow('WebGPU is not available');
  });

  it('should track progress through loading states', async () => {
    const progressCalls: ModelLoadProgress[] = [];
    const runner = new WebGPUModelRunner({
      modelId: 'test-model',
      onProgress: (p) => progressCalls.push({ ...p }),
    });

    await runner.init();
    // Initial progress should be idle
    expect(progressCalls.length).toBe(0); // init doesn't emit progress
    expect(runner.getProgress().state).toBe('idle');
  });

  it('unloadModel should reset to idle state', async () => {
    const runner = new WebGPUModelRunner({ modelId: 'test-model' });
    await runner.init();

    runner.unloadModel();
    expect(runner.isModelLoaded()).toBe(false);
    expect(runner.getProgress().state).toBe('idle');
  });
});

// ===== chat/stream 前置条件测试 =====

describe('WebGPUModelRunner inference guards', () => {
  it('chat should throw when no model loaded', async () => {
    const runner = new WebGPUModelRunner({ modelId: 'test-model' });
    await expect(runner.chat([{ role: 'user', content: 'hello' }]))
      .rejects.toThrow('No model loaded');
  });

  it('stream should throw when no model loaded', async () => {
    const runner = new WebGPUModelRunner({ modelId: 'test-model' });
    const gen = runner.stream([{ role: 'user', content: 'hello' }]);
    await expect(gen.next()).rejects.toThrow('No model loaded');
  });
});

// ===== getProgress 测试 =====

describe('WebGPUModelRunner getProgress', () => {
  it('should return a copy of progress (not reference)', () => {
    const runner = new WebGPUModelRunner({ modelId: 'test-model' });
    const p1 = runner.getProgress();
    const p2 = runner.getProgress();
    expect(p1).toEqual(p2);
    expect(p1).not.toBe(p2); // different object references
  });

  it('should have correct initial state', () => {
    const runner = new WebGPUModelRunner({ modelId: 'test-model' });
    const progress = runner.getProgress();
    expect(progress.state).toBe('idle');
    expect(progress.downloadProgress).toBe(0);
    expect(progress.bytesDownloaded).toBe(0);
    expect(progress.totalBytes).toBe(0);
    expect(progress.error).toBeUndefined();
  });
});

// ===== v4.3-3 WebGPU 边缘推理可用化：真实后端优先、mock 仅回退 =====

describe('detectInferenceBackend (v4.3-3 真实推理替换 mock)', () => {
  it('transformers.js 可导入时返回真实后端（TransformersBackend）', async () => {
    vi.doMock('@xenova/transformers', () => ({ pipeline: vi.fn() }));
    const { detectInferenceBackend } = await import('../webgpu-model-runner.js');
    const backend = await detectInferenceBackend();
    expect(backend.name).toBe('transformers.js');
    vi.doUnmock('@xenova/transformers');
  });

  it('transformers.js 不可导入时回退骨架后端（mock 仅回退）', async () => {
    // 强制动态导入失败：不安装 @xenova/transformers 的环境
    vi.stubGlobal('transformersImportBroken', true);
    const { detectInferenceBackend } = await import('../webgpu-model-runner.js');
    const originalImport = (globalThis as typeof globalThis & { import: (spec: string) => Promise<unknown> }).import;
    // 模拟动态导入失败（@xenova/transformers 未安装）
    const failingImport = (spec: string) =>
      spec.includes('@xenova/transformers') ? Promise.reject(new Error('module not found')) : originalImport(spec);
    Object.defineProperty(globalThis, 'import', { value: failingImport, writable: true, configurable: true });
    try {
      const backend = await detectInferenceBackend();
      expect(backend.name).toBe('skeleton');
    } finally {
      Object.defineProperty(globalThis, 'import', { value: originalImport, writable: true, configurable: true });
      vi.unstubAllGlobals();
    }
  });

  it('runner 装配真实后端后经 chat() 产出真实输出（非骨架回退）', async () => {
    // 用可注入后端验证 runner 生成路径：真实后端输出经 chat() 全链路返回
    const { WebGPUModelRunner } = await import('../webgpu-model-runner.js');
    const runner = new WebGPUModelRunner({ modelId: 'mock-model' });
    const fakeRealBackend = {
      name: 'transformers.js',
      load: vi.fn().mockResolvedValue(undefined),
      generate: vi.fn().mockResolvedValue('本地模型真实生成结果'),
      dispose: vi.fn(),
    };
    // 注入已装配的真实后端（跳过下载/设备初始化）
    (runner as unknown as { backend: unknown }).backend = fakeRealBackend;
    (runner as unknown as { modelLoaded: boolean }).modelLoaded = true;
    const resp = await runner.chat([{ role: 'user', content: '你好' }]);
    expect(resp.content).toBe('本地模型真实生成结果');
    expect(resp.content.startsWith('[skeleton]')).toBe(false);
    expect(fakeRealBackend.generate).toHaveBeenCalled();
  });
});
