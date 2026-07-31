/**
 * cold-start.ts unit tests
 *
 * Coverage:
 * - ColdStartOptimizer.analyze() report generation
 * - Lazy load candidate detection (edge vs node)
 * - Memory usage estimation
 * - Suggestion generation logic
 * - LazyLoader: get / isLoaded / getLoadMs / preload
 * - ConnectionPrewarmer: prewarmLLM / isPrewarmed
 * - recordModuleLoad critical vs non-critical
 * - generateCloudflareConfig / generateDenoConfig / generateLazyLoadWrapper
 * - initEdgeRuntime()
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  ColdStartOptimizer,
  LazyLoader,
  lazyLoad,
  ConnectionPrewarmer,
  initEdgeRuntime,
} from '../cold-start.js';
import { resetRuntimeCache } from '../runtime.js';

const g = globalThis as Record<string, unknown>;

function cleanGlobals() {
  delete g.Deno;
  delete g.Bun;
  delete g.caches;
  delete g.WebSocketPair;
}

describe('ColdStartOptimizer', () => {
  beforeEach(() => {
    cleanGlobals();
    resetRuntimeCache();
  });

  afterEach(() => {
    cleanGlobals();
    resetRuntimeCache();
  });

  describe('analyze()', () => {
    it('should return a valid ColdStartReport', async () => {
      const optimizer = new ColdStartOptimizer();
      const report = await optimizer.analyze();
      expect(report).toHaveProperty('runtime');
      expect(report).toHaveProperty('coldStartMs');
      expect(report).toHaveProperty('loadedModules');
      expect(report).toHaveProperty('memoryUsageMB');
      expect(report).toHaveProperty('suggestions');
      expect(report).toHaveProperty('isEdge');
      expect(report).toHaveProperty('lazyLoadCandidates');
      expect(report.runtime).toBe('node');
      expect(report.isEdge).toBe(false);
      expect(report.coldStartMs).toBeGreaterThanOrEqual(0);
    });

    it('should include suggestions sorted by priority', async () => {
      const optimizer = new ColdStartOptimizer();
      const report = await optimizer.analyze();
      expect(report.suggestions.length).toBeGreaterThan(0);
      for (let i = 1; i < report.suggestions.length; i++) {
        expect(report.suggestions[i - 1]!.priority).toBeGreaterThanOrEqual(report.suggestions[i]!.priority);
      }
    });

    it('should return node-specific lazy candidates in Node', async () => {
      const optimizer = new ColdStartOptimizer();
      const report = await optimizer.analyze();
      expect(report.lazyLoadCandidates.length).toBeLessThan(30);
      expect(report.lazyLoadCandidates).toContain('tools/builtin/shell.js');
    });

    it('should estimate memory usage > 0 in Node', async () => {
      const optimizer = new ColdStartOptimizer();
      const report = await optimizer.analyze();
      expect(report.memoryUsageMB).toBeGreaterThan(0);
    });
  });

  describe('analyze() in edge mode', () => {
    it('should return edge-specific suggestions and candidates', async () => {
      g.caches = {};
      g.WebSocketPair = class {};
      resetRuntimeCache();
      const opt = new ColdStartOptimizer();
      const report = await opt.analyze();
      expect(report.isEdge).toBe(true);
      expect(report.runtime).toBe('cloudflare');
      expect(report.lazyLoadCandidates.length).toBeGreaterThan(20);
      const types = report.suggestions.map((s: { type: string }) => s.type);
      expect(types).toContain('tree-shake');
      expect(types).toContain('cache');
      expect(types).toContain('lazy-load');
    });
  });

  describe('recordModuleLoad()', () => {
    it('should track module loads and reflect in analyze', async () => {
      const optimizer = new ColdStartOptimizer();
      optimizer.recordModuleLoad('types.js', 5, 10);
      optimizer.recordModuleLoad('llm/openai.js', 20, 200);
      const report = await optimizer.analyze();
      expect(report.loadedModules).toBe(2);
    });
  });

  describe('generateCloudflareConfig()', () => {
    it('should return valid wrangler.toml content', () => {
      const optimizer = new ColdStartOptimizer();
      const config = optimizer.generateCloudflareConfig();
      expect(config).toContain('agentprimordia-worker');
      expect(config).toContain('compatibility_date');
      expect(config).toContain('LAZY_LOAD_PROVIDERS');
    });
  });

  describe('generateDenoConfig()', () => {
    it('should return valid deno.json content', () => {
      const optimizer = new ColdStartOptimizer();
      const config = optimizer.generateDenoConfig();
      expect(config).toContain('deno run');
      expect(config).toContain('import_map.json');
    });
  });

  describe('generateLazyLoadWrapper()', () => {
    it('should generate wrapper code with exports', () => {
      const optimizer = new ColdStartOptimizer();
      const wrapper = optimizer.generateLazyLoadWrapper('llm/openai.js', ['OpenAIProvider', 'createOpenAI']);
      expect(wrapper).toContain('lazyLoad');
      expect(wrapper).toContain('llm/openai.js');
      expect(wrapper).toContain('OpenAIProvider');
      expect(wrapper).toContain('createOpenAI');
    });
  });
});

describe('LazyLoader', () => {
  it('should load module on first get()', async () => {
    const loader = vi.fn(async () => ({ value: 42 }));
    const lazy = new LazyLoader(loader);
    expect(lazy.isLoaded()).toBe(false);
    const mod = await lazy.get();
    expect(mod.value).toBe(42);
    expect(lazy.isLoaded()).toBe(true);
    expect(loader).toHaveBeenCalledOnce();
  });

  it('should cache module on subsequent get() calls', async () => {
    const loader = vi.fn(async () => ({ value: 1 }));
    const lazy = new LazyLoader(loader);
    await lazy.get();
    await lazy.get();
    expect(loader).toHaveBeenCalledOnce();
  });

  it('should report load time', async () => {
    const lazy = new LazyLoader(async () => {
      await new Promise((r) => setTimeout(r, 20));
      return { ok: true };
    });
    await lazy.get();
    expect(lazy.getLoadMs()).toBeGreaterThanOrEqual(10);
  });

  it('should preload without blocking', async () => {
    let resolved = false;
    const lazy = new LazyLoader(async () => {
      await new Promise((r) => setTimeout(r, 30));
      resolved = true;
      return {};
    });
    lazy.preload();
    expect(lazy.isLoaded()).toBe(false);
    await lazy.get();
    expect(resolved).toBe(true);
  });

  it('preload should not re-trigger if already loading', async () => {
    const loader = vi.fn(async () => ({}));
    const lazy = new LazyLoader(loader);
    lazy.preload();
    lazy.preload();
    await lazy.get();
    expect(loader).toHaveBeenCalledOnce();
  });
});

describe('lazyLoad', () => {
  it('should create a LazyLoader instance', () => {
    const lazy = lazyLoad(async () => ({ x: 1 }));
    expect(lazy).toBeInstanceOf(LazyLoader);
  });
});

describe('ConnectionPrewarmer', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    cleanGlobals();
    resetRuntimeCache();
  });

  it('should prewarm an endpoint', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('', { status: 200 }));
    resetRuntimeCache();
    const prewarmer = new ConnectionPrewarmer();
    await prewarmer.prewarmLLM('https://api.openai.com');
    expect(prewarmer.isPrewarmed('https://api.openai.com')).toBe(true);
    expect(fetchSpy).toHaveBeenCalled();
  });

  it('should not re-prewarm already prewarmed endpoint', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('', { status: 200 }));
    resetRuntimeCache();
    const prewarmer = new ConnectionPrewarmer();
    await prewarmer.prewarmLLM('https://api.openai.com');
    await prewarmer.prewarmLLM('https://api.openai.com');
    expect(fetchSpy).toHaveBeenCalledOnce();
  });

  it('should prewarm multiple endpoints', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('', { status: 200 }));
    resetRuntimeCache();
    const prewarmer = new ConnectionPrewarmer();
    await prewarmer.prewarmAll(['https://a.com', 'https://b.com', 'https://c.com']);
    expect(prewarmer.isPrewarmed('https://a.com')).toBe(true);
    expect(prewarmer.isPrewarmed('https://b.com')).toBe(true);
    expect(prewarmer.isPrewarmed('https://c.com')).toBe(true);
    expect(fetchSpy).toHaveBeenCalledTimes(3);
  });

  it('should tolerate fetch failures gracefully', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('network down'));
    resetRuntimeCache();
    const prewarmer = new ConnectionPrewarmer();
    await prewarmer.prewarmLLM('https://fail.com');
    expect(prewarmer.isPrewarmed('https://fail.com')).toBe(true);
  });

  it('should report not prewarmed for unknown endpoint', () => {
    resetRuntimeCache();
    const prewarmer = new ConnectionPrewarmer();
    expect(prewarmer.isPrewarmed('https://unknown.com')).toBe(false);
  });
});

describe('initEdgeRuntime', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    cleanGlobals();
    resetRuntimeCache();
  });

  it('should return optimizer, prewarmer, and report', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('', { status: 200 }));
    resetRuntimeCache();
    const result = await initEdgeRuntime();
    expect(result.optimizer).toBeInstanceOf(ColdStartOptimizer);
    expect(result.prewarmer).toBeInstanceOf(ConnectionPrewarmer);
    expect(result.report).toHaveProperty('runtime');
    expect(result.report).toHaveProperty('coldStartMs');
  });

  it('should use custom endpoints when provided', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('', { status: 200 }));
    resetRuntimeCache();
    await initEdgeRuntime({ prewarmEndpoints: ['https://custom.api.com'] });
    expect(fetchSpy).toHaveBeenCalledOnce();
    expect(fetchSpy).toHaveBeenCalledWith('https://custom.api.com', expect.any(Object));
  });
});
