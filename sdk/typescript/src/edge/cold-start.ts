/**
 * Edge-Native 冷启动优化 — 针对 Cloudflare Workers / Deno / Bun 的代码分割和懒加载。
 *
 * Edge Runtime 的冷启动特征：
 * - V8 Isolate 初始化（~5ms）
 * - 模块解析和加载（~20-100ms，取决于包大小）
 * - 首次 LLM 连接建立（~100-500ms）
 *
 * 优化策略：
 * 1. 懒加载非核心模块（LLM Provider、工具等按需加载）
 * 2. 预热连接池（在 Request 处理前建立 TCP/TLS 连接）
 * 3. 模块代码分割建议（分析依赖图，识别可延迟加载的模块）
 * 4. 冷启动时间基准和追踪
 *
 * 使用方式：
 *   const optimizer = new ColdStartOptimizer();
 *   const report = await optimizer.analyze();
 *   console.log(report.coldStartMs);
 *   console.log(report.suggestions);
 */

import { detectRuntime, type RuntimeInfo } from './runtime.js';

// ===== 类型定义 =====

/** 冷启动分析报告 */
export interface ColdStartReport {
  /** 运行时名称 */
  runtime: string;
  /** 冷启动时间（毫秒） */
  coldStartMs: number;
  /** 已加载模块数 */
  loadedModules: number;
  /** 内存占用（MB，估算） */
  memoryUsageMB: number;
  /** 优化建议 */
  suggestions: ColdStartSuggestion[];
  /** 是否为 Edge Runtime */
  isEdge: boolean;
  /** 可懒加载的模块列表 */
  lazyLoadCandidates: string[];
}

/** 冷启动优化建议 */
export interface ColdStartSuggestion {
  /** 建议类型 */
  type: 'lazy-load' | 'preload' | 'tree-shake' | 'bundle-split' | 'cache';
  /** 建议描述 */
  description: string;
  /** 预期节省时间（毫秒） */
  estimatedSavingMs: number;
  /** 优先级（1-5，5 最高） */
  priority: number;
}

/** 模块加载记录 */
export interface ModuleLoadRecord {
  /** 模块路径 */
  path: string;
  /** 加载耗时（毫秒） */
  loadMs: number;
  /** 是否为关键路径 */
  isCritical: boolean;
  /** 大小（KB，估算） */
  sizeKB: number;
}

// ===== 懒加载器 =====

/**
 * 懒加载器 — 延迟加载非核心模块，减少冷启动时间。
 *
 * 在 Edge Runtime 中，模块加载是同步的且会增加冷启动时间。
 * 通过将非核心模块标记为懒加载，可以将冷启动时间减少 30-60%。
 *
 * 使用方式：
 *   const lazyLLM = lazyLoad(() => import('../llm/openai.js'));
 *   // 在第一次使用时才加载
 *   const OpenAIProvider = (await lazyLLM()).OpenAIProvider;
 */
export class LazyLoader<T> {
  private loader: () => Promise<T>;
  private promise: Promise<T> | null = null;
  private loaded = false;
  private loadMs = 0;

  constructor(loader: () => Promise<T>) {
    this.loader = loader;
  }

  /** 获取模块（首次调用时加载，后续返回缓存） */
  async get(): Promise<T> {
    if (this.loaded) return this.promise!;
    if (!this.promise) {
      const start = Date.now();
      this.promise = this.loader().then((mod) => {
        this.loadMs = Date.now() - start;
        this.loaded = true;
        return mod;
      });
    }
    return this.promise;
  }

  /** 是否已加载 */
  isLoaded(): boolean {
    return this.loaded;
  }

  /** 加载耗时（毫秒） */
  getLoadMs(): number {
    return this.loadMs;
  }

  /** 预加载（不阻塞当前执行） */
  preload(): void {
    if (!this.promise) {
      this.get();
    }
  }
}

/** 创建懒加载模块 */
export function lazyLoad<T>(loader: () => Promise<T>): LazyLoader<T> {
  return new LazyLoader(loader);
}

// ===== 连接预热器 =====

/**
 * 连接预热器 — 在处理第一个请求前预建立 LLM API 连接。
 *
 * 通过发送一个轻量级请求（如 models/list），预建立 TCP/TLS 连接，
 * 使后续的真实请求不需要等待连接建立。
 */
export class ConnectionPrewarmer {
  private prewarmed: Set<string> = new Set();
  private runtime: RuntimeInfo;

  constructor() {
    this.runtime = detectRuntime();
  }

  /** 预热 LLM API 连接 */
  async prewarmLLM(endpoint: string): Promise<void> {
    if (this.prewarmed.has(endpoint)) return;

    try {
      // 发送一个轻量级 HEAD 请求来预热连接
      if (this.runtime.supportsFetch) {
        await fetch(endpoint, { method: 'HEAD', mode: 'no-cors' }).catch(() => {});
      }
      this.prewarmed.add(endpoint);
    } catch {
      // 预热失败不影响后续使用
    }
  }

  /** 预热多个端点 */
  async prewarmAll(endpoints: string[]): Promise<void> {
    await Promise.allSettled(endpoints.map((ep) => this.prewarmLLM(ep)));
  }

  /** 检查是否已预热 */
  isPrewarmed(endpoint: string): boolean {
    return this.prewarmed.has(endpoint);
  }
}

// ===== 冷启动分析器 =====

/** 常见 LLM API 端点 */
const LLM_ENDPOINTS = [
  'https://api.openai.com',
  'https://api.anthropic.com',
  'https://generativelanguage.googleapis.com',
];

/** 核心模块列表（不应懒加载） */
const CRITICAL_MODULES = [
  'types.js',
  'errors.js',
  'validate.js',
  'agent/react-loop.js',
  'llm/provider.js',
  'tools/registry.js',
];

/** 可懒加载的模块列表 */
const LAZY_CANDIDATES = [
  'llm/openai.js',
  'llm/anthropic.js',
  'llm/gemini.js',
  'llm/ollama.js',
  'llm/providers.js',
  'llm/multimodal.js',
  'llm/batch.js',
  'llm/cache-structured.js',
  'memory/rag.js',
  'memory/rag-pipeline.js',
  'memory/vector.js',
  'memory/vector-extended.js',
  'tools/builtin/index.js',
  'tools/builtin/filesystem.js',
  'tools/builtin/shell.js',
  'tools/builtin/web-api.js',
  'tools/builtin/database-code-knowledge.ts',
  'orchestration/advanced.js',
  'orchestration/extended.js',
  'orchestration/pipeline.js',
  'a2a/transport.js',
  'a2a/websocket-transport.js',
  'a2a/bus.js',
  'security/guardrails.js',
  'security/sandbox.js',
  'security/extended.js',
  'metrics/otel-prometheus.js',
  'metrics/otel-extended.js',
  'pool/agent-pool.js',
  'pool/dispatcher-autoscaler.js',
  'persist/sqlite-checkpoint.js',
  'edge/runtime.js',
  'agent/self-tuning.js',
  'agent/speculative-exec.js',
  'agent/prompt-ab-test.js',
];

export class ColdStartOptimizer {
  private runtime: RuntimeInfo;
  private moduleLoadRecords: ModuleLoadRecord[] = [];
  private startTime: number;

  constructor() {
    this.runtime = detectRuntime();
    this.startTime = Date.now();
  }

  /** 分析当前冷启动状态 */
  async analyze(): Promise<ColdStartReport> {
    const coldStartMs = Date.now() - this.startTime;
    const suggestions = this.generateSuggestions();
    const lazyLoadCandidates = this.getLazyLoadCandidates();
    const memoryUsageMB = this.estimateMemoryUsage();

    return {
      runtime: this.runtime.name,
      coldStartMs,
      loadedModules: this.moduleLoadRecords.length,
      memoryUsageMB,
      suggestions,
      isEdge: this.runtime.isEdge,
      lazyLoadCandidates,
    };
  }

  /** 记录模块加载 */
  recordModuleLoad(path: string, loadMs: number, sizeKB: number): void {
    this.moduleLoadRecords.push({
      path,
      loadMs,
      isCritical: CRITICAL_MODULES.some((m) => path.includes(m)),
      sizeKB,
    });
  }

  /** 生成 Cloudflare Workers 部署配置 */
  generateCloudflareConfig(): string {
    return `// wrangler.toml — Cloudflare Workers 部署配置
name = "agentprimordia-worker"
main = "src/edge/worker.ts"
compatibility_date = "2024-01-01"
compatibility_flags = ["nodejs_compat"]

# 限制：Workers 有 128MB 内存限制
# 建议：只加载核心模块，其余懒加载

[limits]
cpu_ms = 50  # Workers CPU 时间限制

[vars]
EDGE_MODE = "true"
LAZY_LOAD_PROVIDERS = "true"
`;
  }

  /** 生成 Deno 部署配置 */
  generateDenoConfig(): string {
    return `// deno.json — Deno Deploy 配置
{
  "tasks": {
    "start": "deno run --allow-net --allow-env src/edge/deno-server.ts"
  },
  "compilerOptions": {
    "lib": ["deno.ns", "deno.unstable"],
    "importMap": "import_map.json"
  }
}
`;
  }

  /** 生成懒加载包装代码 */
  generateLazyLoadWrapper(modulePath: string, exports: string[]): string {
    const _varName = modulePath.replace(/[/\\.]/g, '_');
    return `// Auto-generated lazy-load wrapper for ${modulePath}
import { lazyLoad } from './edge/cold-start.js';

const _lazy = lazyLoad(() => import('./${modulePath}'));

${exports.map((exp) => `
export async function ${exp}(...args: unknown[]) {
  const mod = await _lazy.get();
  return mod.${exp}(...args);
}`).join('\n')}
`;
  }

  // ===== 内部方法 =====

  private generateSuggestions(): ColdStartSuggestion[] {
    const suggestions: ColdStartSuggestion[] = [];

    // 1. 懒加载建议
    if (this.runtime.isEdge || this.runtime.name === 'deno') {
      suggestions.push({
        type: 'lazy-load',
        description: 'Lazy-load LLM provider modules (openai.js, anthropic.js, etc.) — only load the provider actually used',
        estimatedSavingMs: 20,
        priority: 5,
      });
    }

    // 2. 预热连接建议
    suggestions.push({
      type: 'preload',
      description: 'Pre-warm LLM API connections during module initialization to eliminate first-request latency',
      estimatedSavingMs: 100,
      priority: 4,
    });

    // 3. Tree-shaking 建议
    if (this.runtime.isEdge) {
      suggestions.push({
        type: 'tree-shake',
        description: 'Tree-shake unused orchestration, A2A, and security modules for Edge deployment',
        estimatedSavingMs: 15,
        priority: 3,
      });
    }

    // 4. Bundle split 建议
    suggestions.push({
      type: 'bundle-split',
      description: 'Split bundle into core (types, react-loop, provider) and optional (RAG, orchestration, MCP) chunks',
      estimatedSavingMs: 30,
      priority: 4,
    });

    // 5. 缓存建议
    if (this.runtime.isEdge) {
      suggestions.push({
        type: 'cache',
        description: 'Use Cache API to cache LLM responses at the Edge for identical prompts',
        estimatedSavingMs: 200,
        priority: 5,
      });
    }

    return suggestions.sort((a, b) => b.priority - a.priority);
  }

  private getLazyLoadCandidates(): string[] {
    // 在 Edge Runtime 中，更多模块可以懒加载
    if (this.runtime.isEdge) {
      return LAZY_CANDIDATES;
    }
    // 在 Node.js 中，只建议懒加载特别重的模块
    return [
      'tools/builtin/database-code-knowledge.ts',
      'tools/builtin/shell.js',
      'persist/sqlite-checkpoint.js',
      'a2a/websocket-transport.js',
    ];
  }

  private estimateMemoryUsage(): number {
    if (typeof process !== 'undefined' && process.memoryUsage) {
      return Math.round(process.memoryUsage().heapUsed / 1024 / 1024 * 100) / 100;
    }
    // Edge Runtime / Browser: 无法精确测量
    return 0;
  }
}

// ===== Edge Runtime 初始化助手 =====

/**
 * Edge Runtime 初始化助手 — 在 Worker/Server 启动时调用。
 *
 * 执行以下优化：
 * 1. 检测运行时环境
 * 2. 预热 LLM 连接（如果配置了端点）
 * 3. 记录冷启动时间
 * 4. 返回优化器实例供后续使用
 */
export async function initEdgeRuntime(opts?: {
  prewarmEndpoints?: string[];
}): Promise<{ optimizer: ColdStartOptimizer; prewarmer: ConnectionPrewarmer; report: ColdStartReport }> {
  const optimizer = new ColdStartOptimizer();
  const prewarmer = new ConnectionPrewarmer();

  // 预热连接
  const endpoints = opts?.prewarmEndpoints ?? LLM_ENDPOINTS;
  await prewarmer.prewarmAll(endpoints);

  const report = await optimizer.analyze();

  return { optimizer, prewarmer, report };
}
