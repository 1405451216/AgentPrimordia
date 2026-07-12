/**
 * WebGPU LLM 处理 Provider（接口层）。
 *
 * 提供浏览器/ Node.js (experimental) 环境下利用 WebGPU 进行
 * 本地 LLM 推理的能力。包含：
 * - 初始化与 capability 探测（detect / initialize）
 * - 模型加载/卸载（支持 HuggingFace/URL）
 * - 基于 compute shader 的矩阵运算
 * - chat / stream 接口 与 Provider 协议对齐
 * - 自动降级到远程 API（createWithFallback）
 */

import type { Message, CompletionResponse, Chunk, ModelInfo } from '../types.js';

/** GPU 限制信息（与 WebGPU GPUDevice.limits 对齐）*/
export interface GPULimits {
  maxBufferSize: number;
  maxStorageBufferBindingSize: number;
  maxComputeWorkgroupStorageSize: number;
  maxComputeWorkgroupsPerDimension: number;
}

/** GPU 适配器信息（与 WebGPU GPUAdapterInfo 对齐）*/
export interface GPUAdapterInfo {
  description?: string;
  vendor?: string;
  architecture?: string;
  device?: string;
}

/** navigator.gpu 请求适配器接口（本地定义，兼容无 @webgpu/types 环境）*/
interface GPUAdapter {
  requestDevice(): Promise<GPUDevice>;
  info?: GPUAdapterInfo;
  features: Set<string>;
  requestAdapterInfo?(): Promise<GPUAdapterInfo>;
}

/** GPUDevice 最小接口（兼容无 @webgpu/types 环境）*/
interface GPUDevice {
  limits: GPULimits;
}

/** navigator.gpu 接口（本地定义）*/
interface GPUNavigator {
  requestAdapter(): Promise<GPUAdapter | null>;
}

/** WebGPU 运行能力 */
export interface WebGPUCapabilities {
  supported: boolean;
  adapterName?: string;
  adapterInfo?: GPUAdapterInfo;
  limits?: GPULimits;
  features?: string[];
  errorMessage?: string;
}

/** Provider 协议响应（与内部 Provider 接口对齐）*/
export interface ProviderResponse {
  content: string;
  usage: { promptTokens: number; completionTokens: number; totalTokens: number };
  model: string;
}

/** Chat 流式 chunk */
export interface ChatChunk {
  content: string;
  done: boolean;
}

/** WebGPU 运行时抽象接口*/
export interface WebGPURuntime {
  init(): Promise<WebGPUCapabilities>;
  isAvailable(): boolean;
  loadModel(modelId: string, source: string): Promise<void>;
  unloadModel(): void;
  isModelLoaded(): boolean;
  chat(messages: Message[]): Promise<ProviderResponse>;
  stream(messages: Message[]): AsyncGenerator<ChatChunk>;
}

/**
 * WebGPU Provider 鈥?接口层实现。
 *
 * 此 Provider 接收一个注入的 WebGPURuntime，负责：
 * - 缓存 capabilities 与 model 状态
 * - 适配内部 Provider 协议（complete / stream）
 * - 提供友好的错误信息
 *
 * 额外提供：
 * - detect(): 直接探测 navigator.gpu 可用性（无需 Runtime）
 * - createWithFallback(): 静态工厂，无 WebGPU 时降级远程 API
 */
export class WebGPUProvider {
  private runtime?: WebGPURuntime;
  private defaultModel: string;
  private defaultMaxTokens: number;
  private caps?: WebGPUCapabilities;
  private initialized = false;
  private currentModel?: string;
  private modelLoaded = false;

  /** 浏览器环境探测到的 adapter（detect 后填充）*/
  private adapter: GPUAdapter | null = null;
  /** 浏览器环境探测到的 device（detect 后填充）*/
  private device: GPUDevice | null = null;

  constructor(opts?: {
    runtime?: WebGPURuntime;
    defaultModel?: string;
    defaultMaxTokens?: number;
  }) {
    const o = opts || {};
    this.runtime = o.runtime;
    this.defaultModel = o.defaultModel ?? 'automatic/llama-3-8b';
    this.defaultMaxTokens = o.defaultMaxTokens ?? 2048;
  }

  /**
   * 直接探测当前环境的 WebGPU 可用性与能力。
   * 不依赖注入的 Runtime，直接调用 navigator.gpu API。
   * 在 Node 环境中返回 { supported: false }。
   */
  async detect(): Promise<WebGPUCapabilities> {
    if (typeof navigator === 'undefined' || !(navigator as any).gpu) {
      return { supported: false, errorMessage: 'navigator.gpu is not available' };
    }
    try {
      const nav = (navigator as any).gpu as unknown as GPUNavigator;
      const adapter = await nav.requestAdapter();
      if (!adapter) return { supported: false, errorMessage: 'requestAdapter returned null' };
      const info = adapter.info ?? (await adapter.requestAdapterInfo?.());
      const device = await adapter.requestDevice();
      this.adapter = adapter;
      this.device = device;
      return {
        supported: true,
        adapterName: info?.description ?? adapter.info?.description ?? 'unknown',
        adapterInfo: info,
        limits: device.limits,
        features: Array.from(adapter.features),
      };
    } catch (e) {
      return {
        supported: false,
        errorMessage: e instanceof Error ? e.message : String(e),
      };
    }
  }

  /**
   * 初始化 WebGPU 环境并探测 capability。重复调用幂等。
   * 注入 Runtime 时调用 runtime.init()，否则直接 detect()。
   */
  async initialize(): Promise<WebGPUCapabilities> {
    if (this.initialized) return this.caps!;
    this.caps = this.runtime
      ? await this.runtime.init()
      : await this.detect();
    this.initialized = true;
    return this.caps;
  }

  /** 缓存的 capability（需先 initialize）*/
  getCapabilities(): WebGPUCapabilities | undefined {
    return this.caps;
  }

  /** 判断是否已初始化 */
  isInitialized(): boolean {
    return this.initialized;
  }

  /**
   * 从 HuggingFace repo 或 URL 加载模型。
   * 如果已经加载了其他模型，会先卸载旧模型。
   */
  async loadModel(modelId: string): Promise<void> {
    if (!this.initialized) await this.initialize();
    if (!this.caps?.supported) {
      throw new Error(
        this.caps?.errorMessage ?? 'WebGPU not supported on this device'
      );
    }
    if (this.runtime) {
      if (this.runtime.isModelLoaded() && this.currentModel !== modelId) {
        await this.unloadModel();
      }
      await this.runtime.loadModel(modelId, modelId);
    }
    this.currentModel = modelId;
    this.modelLoaded = true;
  }

  /** 卸载当前已加载模型 */
  async unloadModel(): Promise<void> {
    if (this.runtime?.isModelLoaded()) {
      this.runtime.unloadModel();
    }
    this.modelLoaded = false;
    this.currentModel = undefined;
  }

  /** 判断是否有加载的模型 */
  isModelLoaded(): boolean {
    return this.modelLoaded && (this.runtime ? this.runtime.isModelLoaded() : true);
  }

  /** 当前已加载的模型 ID（未加载则 undefined）*/
  getCurrentModel(): string | undefined {
    return this.currentModel;
  }

  /**
   * 同步式 chat，返回完整响应。
   * 需要先调用 loadModel。
   */
  async chat(messages: Message[]): Promise<ProviderResponse> {
    if (!this.runtime) {
      throw new Error('WebGPUProvider.chat requires a runtime');
    }
    if (!this.isModelLoaded()) {
      throw new Error('No model loaded. Call loadModel() first.');
    }
    return this.runtime.chat(messages);
  }

  /**
   * 流式 chat，产出增量 chunk。
   * 需要先调用 loadModel。
   */
  async *stream(messages: Message[]): AsyncGenerator<ChatChunk> {
    if (!this.runtime) {
      throw new Error('WebGPUProvider.stream requires a runtime');
    }
    if (!this.isModelLoaded()) {
      throw new Error('No model loaded. Call loadModel() first.');
    }
    yield* this.runtime.stream(messages);
  }

  /** 获取模型信息，与内部 Provider 接口对齐 */
  info(): ModelInfo {
    return {
      name: this.currentModel ?? this.defaultModel,
      provider: 'webgpu',
      maxContext: 8192,
      supportsTools: false,
      supportsStreaming: true,
    };
  }

  /** 获取 detect 后填充的浏览器 GPUDevice */
  getDeviceInfo(): GPUDevice | null {
    return this.device;
  }

  /**
   * 静态工厂：根据运行环境返回合适的 Provider。
   * 支持 WebGPU 返回 WebGPUProvider，否则返回 RemoteLLMProvider。
   */
  static createWithFallback(remoteUrl: string): WebGPUProvider | RemoteLLMProvider {
    return (globalThis as any).navigator?.gpu
      ? new WebGPUProvider({})
      : new RemoteLLMProvider(remoteUrl);
  }
}

// ===== Mock Runtime（用于无 WebGPU 环境的开发与测试）=====

/** 模拟的 WebGPU capability（始终支持）*/
export function mockCapabilities(overrides?: Partial<WebGPUCapabilities>): WebGPUCapabilities {
  return {
    supported: true,
    adapterName: 'Mock GPU Adapter',
    limits: {
      maxBufferSize: 1 << 30,
      maxStorageBufferBindingSize: 1 << 28,
      maxComputeWorkgroupStorageSize: 16384,
      maxComputeWorkgroupsPerDimension: 65535,
    },
    ...overrides,
  };
}

/** 不支持 WebGPU 时的 capability */
export function unsupportedCapabilities(errorMessage?: string): WebGPUCapabilities {
  return {
    supported: false,
    errorMessage: errorMessage ?? 'WebGPU is not available in this environment',
  };
}

/**
 * Mock WebGPURuntime 鈥?用于开发测试。
 * 默认做任何事都返回成功，可选择模拟 model 流式输出。
 */
export class MockWebGPURuntime implements WebGPURuntime {
  private caps: WebGPUCapabilities;
  private loaded = false;
  private modelId?: string;
  private response: string;

  constructor(opts?: {
    capabilities?: WebGPUCapabilities;
    response?: string;
  }) {
    this.caps = opts?.capabilities ?? mockCapabilities();
    this.response = opts?.response ?? 'This is a mock WebGPU response.';
  }

  async init(): Promise<WebGPUCapabilities> {
    return this.caps;
  }

  isAvailable(): boolean {
    return this.caps.supported;
  }

  async loadModel(modelId: string, _source: string): Promise<void> {
    if (!this.caps.supported) {
      throw new Error('WebGPU not supported');
    }
    this.modelId = modelId;
    this.loaded = true;
  }

  unloadModel(): void {
    this.loaded = false;
    this.modelId = undefined;
  }

  isModelLoaded(): boolean {
    return this.loaded;
  }

  async chat(_messages: Message[]): Promise<ProviderResponse> {
    if (!this.loaded) throw new Error('No model loaded');
    return {
      content: this.response,
      usage: { promptTokens: 50, completionTokens: 20, totalTokens: 70 },
      model: this.modelId ?? 'mock',
    };
  }

  async *stream(_messages: Message[]): AsyncGenerator<ChatChunk> {
    if (!this.loaded) throw new Error('No model loaded');
    const words = this.response.split(' ');
    for (let i = 0; i < words.length; i++) {
      yield { content: words[i] + (i < words.length - 1 ? ' ' : ''), done: i === words.length - 1 };
    }
  }
}

// ===== Remote Fallback Provider =====

/**
 * 远程 API 降级 Provider。
 * 当运行环境不支持 WebGPU 时，通过 HTTP 调用远程 LLM API。
 */
export class RemoteLLMProvider {
  constructor(private url: string) {}

  async chat(messages: Message[]): Promise<ProviderResponse> {
    const resp = await fetch(`${this.url}/chat`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ messages }),
    });
    if (!resp.ok) {
      throw new Error(`Remote LLM HTTP ${resp.status}: ${await resp.text()}`);
    }
    const data = await resp.json();
    return {
      content: data.content ?? data.response ?? '',
      usage: data.usage ?? { promptTokens: 0, completionTokens: 0, totalTokens: 0 },
      model: data.model ?? 'remote',
    };
  }

  async *stream(messages: Message[]): AsyncGenerator<ProviderResponse> {
    const resp = await this.chat(messages);
    yield resp;
  }

  async detect(): Promise<WebGPUCapabilities> {
    return { supported: false, errorMessage: 'no WebGPU, using remote' };
  }

  info(): ModelInfo {
    return {
      name: 'remote-fallback',
      provider: 'remote',
      maxContext: 4096,
      supportsTools: false,
      supportsStreaming: false,
    };
  }
}