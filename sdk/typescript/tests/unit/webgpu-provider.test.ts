import { describe, it, expect } from 'vitest';
import {
  WebGPUProvider,
  MockWebGPURuntime,
  mockCapabilities,
  unsupportedCapabilities,
} from '../../src/llm/webgpu-provider.js';

describe('WebGPUProvider', () => {
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

describe('WebGPU Capability helpers', () => {
  it('mockCapabilities provides defaults', () => {
    const caps = mockCapabilities();
    expect(caps.supported).toBe(true);
    expect(caps.limits).toBeDefined();
    expect(caps.limits!.maxBufferSize).toBeGreaterThan(0);
  });

  it('mockCapabilities respects overrides', () => {
    const caps = mockCapabilities({ adapterName: 'Custom GPU' });
    expect(caps.adapterName).toBe('Custom GPU');
  });

  it('unsupportedCapabilities reports error', () => {
    const caps = unsupportedCapabilities('no GPU');
    expect(caps.supported).toBe(false);
    expect(caps.errorMessage).toBe('no GPU');
  });
});
