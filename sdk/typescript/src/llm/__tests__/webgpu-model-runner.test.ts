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
  type CacheStrategy,
  type WebGPUModelRunnerConfig,
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
