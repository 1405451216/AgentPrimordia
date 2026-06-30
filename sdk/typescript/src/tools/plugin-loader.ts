/**
 * 动态插件热加载 — 运行时 import() 自动发现和加载插件。
 *
 * 与 Go 端的 plugin 系统不同，Node.js 的 import() 是原生支持的、
 * 跨平台的动态加载机制。Go 的 plugin 包仅支持 Linux/macOS 且有限制。
 *
 * 这是 TS SDK 相对 Go SDK 的核心架构优势：
 * - npm 包即插件：`@agentprimordia/plugin-*` 自动发现
 * - 无需编译期链接
 * - 支持 ESM 动态导入
 * - 跨平台（Node.js / Deno / Bun）
 *
 * 使用方式：
 *   const loader = new PluginLoader();
 *   await loader.load('my-plugin');
 *   await loader.loadFromPackage('@agentprimordia/plugin-email');
 *   const tools = loader.getTools();
 */

import type { ToolDefinition, Tool } from '../types.js';
import type { ToolRegistry } from '../tools/registry.js';

// ===== 类型定义 =====

/** 插件接口 */
export interface AgentPlugin {
  name: string;
  version: string;
  /** 注册工具到 registry */
  registerTools?(registry: ToolRegistry): void;
  /** 获取插件提供的工具定义 */
  getTools?(): ToolDefinition[];
  /** 插件初始化 */
  init?(config?: Record<string, unknown>): Promise<void>;
  /** 插件销毁 */
  destroy?(): Promise<void>;
}

/** 插件清单 */
export interface PluginManifest {
  name: string;
  version: string;
  description?: string;
  author?: string;
  main: string;
  dependencies?: string[];
  configSchema?: Record<string, unknown>;
}

/** 已加载的插件实例 */
interface LoadedPlugin {
  manifest: PluginManifest;
  instance: AgentPlugin;
  loadedAt: Date;
}

/** 插件加载器配置 */
export interface PluginLoaderConfig {
  /** 插件搜索路径（默认 ['node_modules']） */
  searchPaths?: string[];
  /** 插件名前缀（默认 '@agentprimordia/plugin-'） */
  pluginPrefix?: string;
  /** 是否自动发现（默认 false） */
  autoDiscover?: boolean;
  /** ToolRegistry（可选，自动注册工具） */
  registry?: ToolRegistry;
}

// ===== 插件加载器 =====

export class AgentPluginLoader {
  private config: Omit<Required<PluginLoaderConfig>, 'registry'> & { registry: ToolRegistry | undefined };
  private loaded: Map<string, LoadedPlugin> = new Map();
  private loading: Set<string> = new Set(); // 防止循环加载

  constructor(config?: PluginLoaderConfig) {
    this.config = {
      searchPaths: config?.searchPaths ?? ['node_modules'],
      pluginPrefix: config?.pluginPrefix ?? '@agentprimordia/plugin-',
      autoDiscover: config?.autoDiscover ?? false,
      registry: config?.registry,
    };
  }

  /** 按名称加载插件（动态 import） */
  async load(pluginName: string, initConfig?: Record<string, unknown>): Promise<AgentPlugin> {
    // 已加载则直接返回
    const existing = this.loaded.get(pluginName);
    if (existing) return existing.instance;

    // 防止循环加载
    if (this.loading.has(pluginName)) {
      throw new Error(`Circular plugin loading detected: ${pluginName}`);
    }
    this.loading.add(pluginName);

    try {
      // 动态 import — Node.js / Deno / Bun 原生支持
      const module = await import(pluginName);
      const plugin: AgentPlugin = module.default ?? module;

      if (!plugin.name || !plugin.version) {
        throw new Error(`Plugin "${pluginName}" must export an object with name and version`);
      }

      // 初始化插件
      if (plugin.init) {
        await plugin.init(initConfig);
      }

      // 自动注册工具到 registry
      if (this.config.registry && plugin.registerTools) {
        plugin.registerTools(this.config.registry);
      }

      const manifest: PluginManifest = {
        name: plugin.name,
        version: plugin.version,
        main: pluginName,
      };

      this.loaded.set(pluginName, {
        manifest,
        instance: plugin,
        loadedAt: new Date(),
      });

      this.loading.delete(pluginName);
      return plugin;
    } catch (err) {
      this.loading.delete(pluginName);
      throw new Error(`Failed to load plugin "${pluginName}": ${err instanceof Error ? err.message : String(err)}`);
    }
  }

  /** 从 npm 包名加载（自动添加前缀） */
  async loadFromPackage(shortName: string, initConfig?: Record<string, unknown>): Promise<AgentPlugin> {
    const fullName = shortName.startsWith(this.config.pluginPrefix)
      ? shortName
      : `${this.config.pluginPrefix}${shortName}`;
    return this.load(fullName, initConfig);
  }

  /** 批量加载多个插件 */
  async loadAll(plugins: Array<string | { name: string; config?: Record<string, unknown> }>): Promise<void> {
    await Promise.all(
      plugins.map((p) => {
        if (typeof p === 'string') {
          return this.load(p);
        }
        return this.load(p.name, p.config);
      }),
    );
  }

  /** 自动发现并加载所有 @agentprimordia/plugin-* 包 */
  async autoDiscover(): Promise<string[]> {
    const discovered: string[] = [];

    // 尝试从 package.json 的 dependencies 中发现插件
    try {
      const { createRequire } = await import('node:module');
      const req = createRequire(import.meta.url);
      const pkgJson = req('./package.json');
      const deps = { ...pkgJson.dependencies, ...pkgJson.devDependencies };

      for (const dep of Object.keys(deps)) {
        if (dep.startsWith(this.config.pluginPrefix)) {
          try {
            await this.load(dep);
            discovered.push(dep);
          } catch {
            // Skip failed plugins
          }
        }
      }
    } catch {
      // Not in Node.js or no package.json — skip auto-discovery
    }

    return discovered;
  }

  /** 卸载插件 */
  async unload(pluginName: string): Promise<void> {
    const loaded = this.loaded.get(pluginName);
    if (!loaded) return;

    if (loaded.instance.destroy) {
      await loaded.instance.destroy();
    }
    this.loaded.delete(pluginName);
  }

  /** 卸载所有插件 */
  async unloadAll(): Promise<void> {
    const names = Array.from(this.loaded.keys());
    await Promise.all(names.map((n) => this.unload(n)));
  }

  /** 获取已加载的插件 */
  get(name: string): AgentPlugin | null {
    return this.loaded.get(name)?.instance ?? null;
  }

  /** 列出所有已加载插件 */
  list(): PluginManifest[] {
    return Array.from(this.loaded.values()).map((p) => p.manifest);
  }

  /** 获取所有已加载插件提供的工具 */
  getAllTools(): ToolDefinition[] {
    const tools: ToolDefinition[] = [];
    for (const { instance } of this.loaded.values()) {
      if (instance.getTools) {
        tools.push(...instance.getTools());
      }
    }
    return tools;
  }

  /** 重新加载插件（热更新） */
  async reload(pluginName: string, initConfig?: Record<string, unknown>): Promise<AgentPlugin> {
    await this.unload(pluginName);
    // 清除 require 缓存（Node.js 专用）
    try {
      const { createRequire } = await import('node:module');
      const req = createRequire(import.meta.url);
      const path = req.resolve(pluginName);
      delete req.cache[path];
    } catch {
      // ESM 没有缓存清除 API，只能重新 import
    }
    return this.load(pluginName, initConfig);
  }

  /** 获取插件统计 */
  get stats(): { total: number; loaded: string[] } {
    return {
      total: this.loaded.size,
      loaded: Array.from(this.loaded.keys()),
    };
  }
}

// ===== 插件创建辅助函数 =====

/** 快速创建一个插件（用于插件开发者） */
export function definePlugin(
  name: string,
  version: string,
  setup: (api: PluginAPI) => void,
): AgentPlugin {
  const tools: ToolDefinition[] = [];
  const toolMap: Map<string, Tool> = new Map();

  const api: PluginAPI = {
    registerTool(tool: Tool): void {
      tools.push({
        type: 'function',
        function: {
          name: tool.name,
          description: tool.description,
          parameters: tool.parameters,
        },
      });
      toolMap.set(tool.name, tool);
    },
    getTools(): ToolDefinition[] {
      return tools;
    },
    getTool(name: string): Tool | undefined {
      return toolMap.get(name);
    },
  };

  setup(api);

  return {
    name,
    version,
    getTools: () => tools,
    registerTools: (registry: ToolRegistry) => {
      for (const tool of toolMap.values()) {
        registry.register(tool);
      }
    },
  };
}

/** 插件 API 接口（供插件开发者使用） */
export interface PluginAPI {
  registerTool(tool: Tool): void;
  getTools(): ToolDefinition[];
  getTool(name: string): Tool | undefined;
}
