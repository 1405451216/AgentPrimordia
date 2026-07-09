/**
 * 插件注册中心（Plugin Registry / Marketplace 本地层）
 *
 * 在 AgentPluginLoader 之上提供统一的插件生命周期管理：
 *  - install / uninstall / update：安装、卸载、更新插件并集中心记录元数据
 *  - list / get / has / search：本地检索（按名称、标签、描述）
 *  - executeTool：优先经 PluginSandbox 隔离执行插件工具
 *
 * 远程市场（registry server）的搜索/下载为可扩展点，本文件仅提供本地元数据层，
 * 不发起网络请求，保证离线、可单测。
 *
 * 使用方式：
 *   const registry = new PluginRegistry({ loader, sandbox, registry: toolRegistry });
 *   await registry.install('email');           // 解析为 @agentprimordia/plugin-email
 *   const tools = registry.search('mail');
 *   const result = await registry.executeTool({ id: '1', name: 'sendEmail', arguments: '{}' });
 */

import type { ToolRegistry } from './registry.js';
import { AgentPluginLoader } from './plugin-loader.js';
import type { PluginSandbox } from './plugin-sandbox.js';
import type { ToolCall, ToolResult } from '../types.js';

/** 插件元数据（安装后登记到 registry） */
export interface PluginMetadata {
  /** 完整包名（含前缀），如 @agentprimordia/plugin-email */
  name: string;
  version: string;
  description?: string;
  author?: string;
  homepage?: string;
  tags?: string[];
  dependencies?: string[];
  /** 该插件提供的工具名列表 */
  tools?: string[];
  installedAt?: Date;
}

/** 插件注册中心配置 */
export interface PluginRegistryOptions {
  /** 已有 loader（不传则内部新建一个） */
  loader?: AgentPluginLoader;
  /** 插件沙箱（提供隔离执行） */
  sandbox?: PluginSandbox;
  /** 工具注册表（无 sandbox 时直接执行工具） */
  registry?: ToolRegistry;
  /** 插件名前缀，默认 @agentprimordia/plugin- */
  pluginPrefix?: string;
}

/** 插件注册中心：本地插件市场元数据 + 生命周期管理 */
export class PluginRegistry {
  private metadata = new Map<string, PluginMetadata>();
  private loader: AgentPluginLoader;
  private sandbox?: PluginSandbox;
  private registry?: ToolRegistry;
  private prefix: string;

  constructor(options?: PluginRegistryOptions) {
    this.loader = options?.loader ?? new AgentPluginLoader({ registry: options?.registry });
    this.sandbox = options?.sandbox;
    this.registry = options?.registry;
    this.prefix = options?.pluginPrefix ?? '@agentprimordia/plugin-';
  }

  /** 安装并注册一个插件（支持完整包名或短名），记录元数据 */
  async install(specifier: string, initConfig?: Record<string, unknown>): Promise<PluginMetadata> {
    const fullName = this.normalizeName(specifier);
    const plugin = await this.loader.load(fullName, initConfig);
    const tools = plugin.getTools?.() ?? [];
    const meta: PluginMetadata = {
      name: fullName,
      version: plugin.version,
      tools: tools.map((t) => t.function.name),
      installedAt: new Date(),
    };
    this.metadata.set(fullName, meta);
    return meta;
  }

  /** 卸载插件（从 loader 与元数据表同步移除） */
  async uninstall(name: string): Promise<boolean> {
    const fullName = this.normalizeName(name);
    const existed = this.metadata.has(fullName);
    await this.loader.unload(fullName);
    return this.metadata.delete(fullName) || existed;
  }

  /** 更新插件：先卸载再重装，保留最新元数据 */
  async update(name: string, initConfig?: Record<string, unknown>): Promise<PluginMetadata> {
    const fullName = this.normalizeName(name);
    await this.loader.unload(fullName);
    return this.install(fullName, initConfig);
  }

  /** 按名称获取插件元数据 */
  get(name: string): PluginMetadata | undefined {
    return this.metadata.get(this.normalizeName(name));
  }

  /** 是否已安装 */
  has(name: string): boolean {
    return this.metadata.has(this.normalizeName(name));
  }

  /** 列出所有已安装插件 */
  list(): PluginMetadata[] {
    return Array.from(this.metadata.values());
  }

  /** 本地搜索：按名称 / 标签 / 描述模糊匹配（小写比较） */
  search(query: string): PluginMetadata[] {
    const q = query.toLowerCase();
    return this.list().filter((m) =>
      m.name.toLowerCase().includes(q) ||
      (m.tags ?? []).some((t) => t.toLowerCase().includes(q)) ||
      (m.description ?? '').toLowerCase().includes(q),
    );
  }

  /** 在沙箱中执行某个插件工具（优先隔离执行，否则主线程执行） */
  async executeTool(call: ToolCall): Promise<ToolResult> {
    if (this.sandbox) return this.sandbox.execute(call);
    if (this.registry) return this.registry.execute(call);
    throw new Error('PluginRegistry 未配置 sandbox 或 registry，无法执行工具');
  }

  /** 归一化插件名（短名自动补前缀） */
  private normalizeName(specifier: string): string {
    return specifier.startsWith(this.prefix) ? specifier : `${this.prefix}${specifier}`;
  }
}
