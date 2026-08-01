/**
 * webgpu-provider.test.ts — WebGPU Provider 单元测试。
 *
 * 测试 WebGPUProvider 的核心功能：
 * - 初始化与能力探测（detect / initialize）
 * - 模型加载/卸载
 * - createWithFallback 降级逻辑
 * - chat / stream 请求构造
 * - MockWebGPURuntime 行为验证
 *
 * 所有测试使用 mock WebGPU API，可在无 GPU 的 CI 环境运行。
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  WebGPUProvider,
  MockWebGPURuntime,
  RemoteLLMProvider,
  mockCapabilities,
  unsupportedCapabilities,
  type ChatChunk,
} from '../webgpu-provider.js';

// ===== Mock navigator.gpu =====

function createMockGPUDevice() {
  return {
    limits: {
      maxBufferSize: 1 << 30,
      maxStorageBufferBindingSize: 1 << 28,
      maxComputeWorkgroupStorageSize: 16384,
      maxComputeWorkgroupsPerDimension: 65535,
    },
  };
}

function createMockGPUAdapter(device: any = createMockGPUDevice()) {
  return {
    requestDevice: vi.fn().mockResolvedValue(device),
    info: { description: 'Test GPU Adapter', vendor: 'test' },
    features: new Set(['timestamp-query', 'shader-f16']),
    requestAdapterInfo: vi.fn().mockResolvedValue({ description: 'Test GPU' }),
  };
}

function setupMockNavigator(adapter: any = createMockGPUAdapter()) {
  const gpu = { requestAdapter: vi.fn().mockResolvedValue(adapter) };
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

// ===== mockCapabilities / unsupportedCapabilities 测试 =====

describe('capability helpers', () => {
  it('mockCapabilities should return supported capabilities', () => {
    const caps = mockCapabilities();
    expect(caps.supported).toBe(true);
    expect(caps.adapterName).toBe('Mock GPU Adapter');
    expect(caps.limits).toBeDefined();
    expect(caps.limits!.maxBufferSize).toBeGreaterThan(0);
  });

  it('mockCapabilities should accept overrides', () => {
    const caps = mockCapabilities({ adapterName: 'Custom GPU' });
    expect(caps.adapterName).toBe('Custom GPU');
    expect(caps.supported).toBe(true);
  });

  it('unsupportedCapabilities should return unsupported', () => {
    const caps = unsupportedCapabilities();
    expect(caps.supported).toBe(false);
    expect(caps.errorMessage).toBeDefined();
  });

  it('unsupportedCapabilities should accept custom error message', () => {
    const caps = unsupportedCapabilities('Custom error');
    expect(caps.errorMessage).toBe('Custom error');
  });
});

// ===== MockWebGPURuntime 测试 =====

describe('MockWebGPURuntime', () => {
  it('should initialize and report available', async () => {
    const runtime = new MockWebGPURuntime();
    const caps = await runtime.init();
    expect(caps.supported).toBe(true);
    expect(runtime.isAvailable()).toBe(true);
  });

  it('should load and unload model', async () => {
    const runtime = new MockWebGPURuntime();
    await runtime.init();
    expect(runtime.isModelLoaded()).toBe(false);

    await runtime.loadModel('test-model', 'test-source');
    expect(runtime.isModelLoaded()).toBe(true);

    runtime.unloadModel();
    expect(runtime.isModelLoaded()).toBe(false);
  });

  it('chat should throw when no model loaded', async () => {
    const runtime = new MockWebGPURuntime();
    await expect(runtime.chat([{ role: 'user', content: 'hi' }]))
      .rejects.toThrow('No model loaded');
  });

  it('chat should return mock response when model loaded', async () => {
    const runtime = new MockWebGPURuntime({ response: 'Mock reply' });
    await runtime.init();
    await runtime.loadModel('test-model', 'src');

    const resp = await runtime.chat([{ role: 'user', content: 'hi' }]);
    expect(resp.content).toBe('Mock reply');
    expect(resp.model).toBe('test-model');
    expect(resp.usage.totalTokens).toBe(70);
  });

  it('stream should yield chunks', async () => {
    const runtime = new MockWebGPURuntime({ response: 'hello world' });
    await runtime.init();
    await runtime.loadModel('m', 's');

    const chunks: ChatChunk[] = [];
    for await (const chunk of runtime.stream([{ role: 'user', content: 'hi' }])) {
      chunks.push(chunk);
    }
    expect(chunks.length).toBe(2);
    expect(chunks[0].content).toBe('hello ');
    expect(chunks[0].done).toBe(false);
    expect(chunks[1].content).toBe('world');
    expect(chunks[1].done).toBe(true);
  });

  it('stream should throw when no model loaded', async () => {
    const runtime = new MockWebGPURuntime();
    const gen = runtime.stream([{ role: 'user', content: 'hi' }]);
    await expect(gen.next()).rejects.toThrow('No model loaded');
  });

  it('loadModel should throw when not supported', async () => {
    const runtime = new MockWebGPURuntime({
      capabilities: unsupportedCapabilities(),
    });
    await expect(runtime.loadModel('m', 's')).rejects.toThrow('WebGPU not supported');
  });
});

// ===== WebGPUProvider 测试 =====

describe('WebGPUProvider', () => {
  let runtime: MockWebGPURuntime;

  beforeEach(() => {
    runtime = new MockWebGPURuntime({ response: 'Provider response' });
  });

  afterEach(() => {
    clearMockNavigator();
  });

  it('should construct with default options', () => {
    const provider = new WebGPUProvider();
    expect(provider.isInitialized()).toBe(false);
    expect(provider.isModelLoaded()).toBe(false);
    expect(provider.getCurrentModel()).toBeUndefined();
  });

  it('should initialize with runtime', async () => {
    const provider = new WebGPUProvider({ runtime });
    const caps = await provider.initialize();
    expect(caps.supported).toBe(true);
    expect(provider.isInitialized()).toBe(true);
  });

  it('initialize should be idempotent', async () => {
    const provider = new WebGPUProvider({ runtime });
    const caps1 = await provider.initialize();
    const caps2 = await provider.initialize();
    expect(caps1).toBe(caps2);
  });

  it('detect should work without runtime', async () => {
    setupMockNavigator();
    const provider = new WebGPUProvider();
    const caps = await provider.detect();
    expect(caps.supported).toBe(true);
    expect(caps.adapterName).toBeDefined();
  });

  it('detect should return unsupported in node env', async () => {
    clearMockNavigator();
    const provider = new WebGPUProvider();
    const caps = await provider.detect();
    expect(caps.supported).toBe(false);
    expect(caps.errorMessage).toContain('navigator.gpu');
  });

  it('loadModel should call runtime.loadModel', async () => {
    const provider = new WebGPUProvider({ runtime });
    await provider.initialize();
    await provider.loadModel('my-model');

    expect(provider.isModelLoaded()).toBe(true);
    expect(provider.getCurrentModel()).toBe('my-model');
  });

  it('loadModel should auto-initialize', async () => {
    const provider = new WebGPUProvider({ runtime });
    expect(provider.isInitialized()).toBe(false);

    await provider.loadModel('my-model');
    expect(provider.isInitialized()).toBe(true);
    expect(provider.isModelLoaded()).toBe(true);
  });

  it('loadModel should throw when not supported', async () => {
    const unsupportedRuntime = new MockWebGPURuntime({
      capabilities: unsupportedCapabilities(),
    });
    const provider = new WebGPUProvider({ runtime: unsupportedRuntime });
    await expect(provider.loadModel('model')).rejects.toThrow();
  });

  it('unloadModel should clear model state', async () => {
    const provider = new WebGPUProvider({ runtime });
    await provider.loadModel('model');
    await provider.unloadModel();

    expect(provider.isModelLoaded()).toBe(false);
    expect(provider.getCurrentModel()).toBeUndefined();
  });

  it('chat should throw without runtime', async () => {
    const provider = new WebGPUProvider();
    await expect(provider.chat([{ role: 'user', content: 'hi' }]))
      .rejects.toThrow('requires a runtime');
  });

  it('chat should throw when no model loaded', async () => {
    const provider = new WebGPUProvider({ runtime });
    await provider.initialize();
    await expect(provider.chat([{ role: 'user', content: 'hi' }]))
      .rejects.toThrow('No model loaded');
  });

  it('chat should return response from runtime', async () => {
    const provider = new WebGPUProvider({ runtime });
    await provider.loadModel('test-model');

    const resp = await provider.chat([{ role: 'user', content: 'hi' }]);
    expect(resp.content).toBe('Provider response');
    expect(resp.model).toBe('test-model');
  });

  it('stream should yield chunks from runtime', async () => {
    const provider = new WebGPUProvider({ runtime });
    await provider.loadModel('test-model');

    const chunks: ChatChunk[] = [];
    for await (const chunk of provider.stream([{ role: 'user', content: 'hi' }])) {
      chunks.push(chunk);
    }
    expect(chunks.length).toBeGreaterThan(0);
    expect(chunks[chunks.length - 1].done).toBe(true);
  });

  it('info should return model info', async () => {
    const provider = new WebGPUProvider({ runtime, defaultModel: 'default/model' });
    expect(provider.info().provider).toBe('webgpu');
    expect(provider.info().name).toBe('default/model');

    await provider.loadModel('loaded/model');
    expect(provider.info().name).toBe('loaded/model');
  });
});

// ===== createWithFallback 测试 =====

describe('WebGPUProvider.createWithFallback', () => {
  afterEach(() => {
    clearMockNavigator();
  });

  it('should return WebGPUProvider when navigator.gpu exists', () => {
    setupMockNavigator();
    const provider = WebGPUProvider.createWithFallback('http://api.example.com');
    expect(provider).toBeInstanceOf(WebGPUProvider);
  });

  it('should return RemoteLLMProvider when navigator.gpu is absent', () => {
    clearMockNavigator();
    const provider = WebGPUProvider.createWithFallback('http://api.example.com');
    expect(provider).toBeInstanceOf(RemoteLLMProvider);
  });
});

// ===== RemoteLLMProvider 测试 =====

describe('RemoteLLMProvider', () => {
  it('should construct with URL', () => {
    const provider = new RemoteLLMProvider('http://api.example.com');
    expect(provider).toBeDefined();
  });

  it('detect should return unsupported', async () => {
    const provider = new RemoteLLMProvider('http://api.example.com');
    const caps = await provider.detect();
    expect(caps.supported).toBe(false);
  });

  it('info should return remote provider info', () => {
    const provider = new RemoteLLMProvider('http://api.example.com');
    const info = provider.info();
    expect(info.provider).toBe('remote');
    expect(info.name).toBe('remote-fallback');
    expect(info.supportsStreaming).toBe(false);
  });

  it('chat should call remote API', async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        content: 'Remote response',
        usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 },
        model: 'remote-model',
      }),
    });
    vi.stubGlobal('fetch', mockFetch);

    const provider = new RemoteLLMProvider('http://api.example.com');
    const resp = await provider.chat([{ role: 'user', content: 'hello' }]);

    expect(resp.content).toBe('Remote response');
    expect(mockFetch).toHaveBeenCalledWith('http://api.example.com/chat', expect.any(Object));

    vi.unstubAllGlobals();
  });
});
