import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { detectWebGPU } from '../../src/llm/webgpu-capability.js';
import {
  WebGPUProvider,
  MockWebGPURuntime,
  mockCapabilities,
  unsupportedCapabilities,
  RemoteLLMProvider,
} from '../../src/llm/webgpu-provider.js';

// ---- detectWebGPU 测试 ----

describe('detectWebGPU', () => {
  it('should return none tier when navigator is undefined (Node env)', async () => {
    // Node 环境默认无 navigator，detectWebGPU 应返回 { supported: false, tier: 'none' }
    const result = await detectWebGPU();
    expect(result.supported).toBe(false);
    expect(result.tier).toBe('none');
  });

  it('should return none tier when navigator.gpu is missing', async () => {
    vi.stubGlobal('navigator', {});
    const result = await detectWebGPU();
    expect(result.supported).toBe(false);
    expect(result.tier).toBe('none');
    vi.unstubAllGlobals();
  });

  it('should detect full tier when mock GPU meets thresholds', async () => {
    const mockAdapter = {
      requestDevice: vi.fn().mockResolvedValue({
        limits: { maxBufferSize: 256 * 1024 * 1024, maxComputeWorkgroupsPerDimension: 65535 },
      }),
      info: { description: 'Mock GPU' },
      features: new Set(['shader-f16']),
      requestAdapterInfo: vi.fn().mockResolvedValue({ description: 'Mock GPU' }),
    };
    vi.stubGlobal('navigator', { gpu: { requestAdapter: vi.fn().mockResolvedValue(mockAdapter) } });

    const result = await detectWebGPU();
    expect(result.supported).toBe(true);
    expect(result.tier).toBe('full');
    expect(result.adapterName).toBe('Mock GPU');
    expect(result.maxBufferSize).toBe(256 * 1024 * 1024);
    expect(result.maxComputeWorkgroups).toBe(65535);
    vi.unstubAllGlobals();
  });

  it('should detect basic tier when GPU does not meet full thresholds', async () => {
    const mockAdapter = {
      requestDevice: vi.fn().mockResolvedValue({
        limits: { maxBufferSize: 64 * 1024 * 1024, maxComputeWorkgroupsPerDimension: 1024 },
      }),
      info: { description: 'Weak GPU' },
      features: new Set<string>(),
      requestAdapterInfo: vi.fn().mockResolvedValue({ description: 'Weak GPU' }),
    };
    vi.stubGlobal('navigator', { gpu: { requestAdapter: vi.fn().mockResolvedValue(mockAdapter) } });

    const result = await detectWebGPU();
    expect(result.supported).toBe(true);
    expect(result.tier).toBe('basic');
    vi.unstubAllGlobals();
  });

  it('should handle requestAdapter returning null', async () => {
    vi.stubGlobal('navigator', { gpu: { requestAdapter: vi.fn().mockResolvedValue(null) } });
    const result = await detectWebGPU();
    expect(result.supported).toBe(false);
    expect(result.tier).toBe('none');
    vi.unstubAllGlobals();
  });

  it('should handle requestAdapter throwing', async () => {
    vi.stubGlobal('navigator', { gpu: { requestAdapter: vi.fn().mockRejectedValue(new Error('no gpu')) } });
    const result = await detectWebGPU();
    expect(result.supported).toBe(false);
    expect(result.tier).toBe('none');
    vi.unstubAllGlobals();
  });
});

// ---- WebGPUProvider.detect 测试 ----

describe('WebGPUProvider.detect', () => {
  it('should return unsupported in Node environment', async () => {
    const provider = new WebGPUProvider();
    const caps = await provider.detect();
    expect(caps.supported).toBe(false);
  });

  it('should detect when navigator.gpu is available (mocked)', async () => {
    const mockAdapter = {
      requestDevice: vi.fn().mockResolvedValue({
        limits: {
          maxBufferSize: 1 << 30,
          maxStorageBufferBindingSize: 1 << 28,
          maxComputeWorkgroupStorageSize: 16384,
          maxComputeWorkgroupsPerDimension: 65535,
        },
      }),
      info: { description: 'Mock Adapter' },
      features: new Set(['shader-f16']),
      requestAdapterInfo: vi.fn().mockResolvedValue({ description: 'Mock Adapter' }),
    };
    vi.stubGlobal('navigator', { gpu: { requestAdapter: vi.fn().mockResolvedValue(mockAdapter) } });

    const provider = new WebGPUProvider();
    const caps = await provider.detect();
    expect(caps.supported).toBe(true);
    expect(caps.adapterName).toBe('Mock Adapter');
    expect(caps.limits?.maxBufferSize).toBe(1 << 30);
    expect(caps.features).toContain('shader-f16');
    expect(provider.getDeviceInfo()).not.toBeNull();

    vi.unstubAllGlobals();
  });
});

// ---- WebGPUProvider 已有功能回归测试 ----

describe('WebGPUProvider (existing tests)', () => {
  it('initialize returns capabilities', async () => {
    const runtime = new MockWebGPURuntime({ capabilities: mockCapabilities() });
    const provider = new WebGPUProvider({ runtime });
    const caps = await provider.initialize();
    expect(caps.supported).toBe(true);
    expect(caps.adapterName).toBe('Mock GPU Adapter');
    expect(provider.isInitialized()).toBe(true);
  });

  it('initialize is idempotent', async () => {
    const runtime = new MockWebGPURuntime();
    const provider = new WebGPUProvider({ runtime });
    const caps1 = await provider.initialize();
    const caps2 = await provider.initialize();
    expect(caps1).toBe(caps2);
  });

  it('loadModel throws when WebGPU unsupported', async () => {
    const runtime = new MockWebGPURuntime({ capabilities: unsupportedCapabilities() });
    const provider = new WebGPUProvider({ runtime });
    await provider.initialize();
    await expect(provider.loadModel('test-model')).rejects.toThrow(/not supported|WebGPU/);
  });

  it('loadModel succeeds when supported', async () => {
    const runtime = new MockWebGPURuntime({ capabilities: mockCapabilities() });
    const provider = new WebGPUProvider({ runtime, defaultModel: 'test' });
    await provider.initialize();
    await provider.loadModel('llama-3');
    expect(provider.isModelLoaded()).toBe(true);
    expect(provider.getCurrentModel()).toBe('llama-3');
  });

  it('chat throws if no model loaded', async () => {
    const runtime = new MockWebGPURuntime({ capabilities: mockCapabilities() });
    const provider = new WebGPUProvider({ runtime });
    await provider.initialize();
    await expect(provider.chat([{ role: 'user', content: 'hi' }])).rejects.toThrow(/loadModel/);
  });

  it('chat returns response', async () => {
    const runtime = new MockWebGPURuntime({
      capabilities: mockCapabilities(),
      response: 'Hello from GPU!',
    });
    const provider = new WebGPUProvider({ runtime });
    await provider.initialize();
    await provider.loadModel('test');
    const resp = await provider.chat([{ role: 'user', content: 'Hi' }]);
    expect(resp.content).toBe('Hello from GPU!');
    expect(resp.model).toBe('test');
  });

  it('stream yields chunks', async () => {
    const runtime = new MockWebGPURuntime({
      capabilities: mockCapabilities(),
      response: 'one two three',
    });
    const provider = new WebGPUProvider({ runtime });
    await provider.initialize();
    await provider.loadModel('test');

    const chunks: string[] = [];
    for await (const c of provider.stream([{ role: 'user', content: 'count' }])) {
      chunks.push(c.content);
    }
    expect(chunks.join('')).toBe('one two three');
  });

  it('unloadModel clears state', async () => {
    const runtime = new MockWebGPURuntime({ capabilities: mockCapabilities() });
    const provider = new WebGPUProvider({ runtime });
    await provider.initialize();
    await provider.loadModel('model-a');
    expect(provider.isModelLoaded()).toBe(true);
    await provider.unloadModel();
    expect(provider.isModelLoaded()).toBe(false);
    expect(provider.getCurrentModel()).toBeUndefined();
  });

  it('loadModel replaces existing model', async () => {
    const runtime = new MockWebGPURuntime({ capabilities: mockCapabilities() });
    const provider = new WebGPUProvider({ runtime });
    await provider.initialize();
    await provider.loadModel('model-a');
    await provider.loadModel('model-b');
    expect(provider.getCurrentModel()).toBe('model-b');
  });

  it('info returns correct metadata', () => {
    const runtime = new MockWebGPURuntime();
    const provider = new WebGPUProvider({ runtime, defaultModel: 'llama-3-8b' });
    const info = provider.info();
    expect(info.name).toBe('llama-3-8b');
    expect(info.provider).toBe('webgpu');
    expect(info.supportsStreaming).toBe(true);
    expect(info.supportsTools).toBe(false);
  });
});

// ---- createWithFallback 测试 ----

describe('WebGPUProvider.createWithFallback', () => {
  it('should return undefined-provider when no WebGPU (Node env)', () => {
    // Node 环境：navigator.gpu 不存在，createWithFallback 返回 RemoteLLMProvider
    const provider = WebGPUProvider.createWithFallback('http://localhost:8080');
    expect(provider).toBeDefined();
  });

  it('should return WebGPUProvider when navigator.gpu exists (mocked)', () => {
    const mockAdapter = {
      requestDevice: vi.fn().mockResolvedValue({ limits: {} }),
      info: { description: 'Mock' },
      features: new Set<string>(),
      requestAdapterInfo: vi.fn().mockResolvedValue({ description: 'Mock' }),
    };
    vi.stubGlobal('navigator', { gpu: { requestAdapter: vi.fn().mockResolvedValue(mockAdapter) } });

    const provider = WebGPUProvider.createWithFallback('http://localhost:8080');
    expect(provider).toBeInstanceOf(WebGPUProvider);

    vi.unstubAllGlobals();
  });
});

// ---- RemoteLLMProvider 测试 ----

describe('RemoteLLMProvider', () => {
  it('should report unsupported via detect()', async () => {
    const remote = new RemoteLLMProvider('http://localhost:8080');
    const caps = await remote.detect();
    expect(caps.supported).toBe(false);
  });

  it('should report correct info()', () => {
    const remote = new RemoteLLMProvider('http://localhost:8080');
    const info = remote.info();
    expect(info.provider).toBe('remote');
    expect(info.name).toBe('remote-fallback');
  });
});