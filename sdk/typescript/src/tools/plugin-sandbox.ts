/**
 * 插件沙箱 — 将不可信插件的工具执行隔离到 Worker Thread，
 * 并提供硬性超时与错误隔离，防止失控插件拖垮宿主进程。
 *
 * 设计要点：
 *  - 仅 Node.js 主线程下启用 Worker 隔离（Deno/Bun/Edge 自动降级为主线程 + 超时隔离）
 *  - Worker 内通过动态 import 插件包名，调用 registerTools 注册到 Worker 内的
 *    轻量 registry，从而在隔离的 isolate 中执行工具函数
 *  - 超时通过 worker.terminate() 强制中断，避免挂死
 *  - 任何错误都被收敛为 ToolResult(isError=true)，不会抛出到宿主
 *
 * 使用方式：
 *   const sandbox = new PluginSandbox(registry, {
 *     timeoutMs: 5000,
 *     resolvePluginModule: (toolName) => moduleForTool.get(toolName),
 *   });
 *   const result = await sandbox.execute({ id: '1', name: 'myTool', arguments: '{}' });
 */

import type { ToolRegistry } from './registry.js';
import type { ToolCall, ToolResult } from '../types.js';

/**
 * Worker 主体（以 eval 方式运行在独立 isolate 内）。
 * 注意：这是纯 CommonJS 字符串，运行在 Worker 上下文中，
 * 不依赖宿主模块解析，自身包含最小 registry 实现。
 */
const WORKER_CODE = `
const { parentPort, workerData } = require('node:worker_threads');
(async () => {
  try {
    const mod = await import(workerData.pluginModule);
    const plugin = mod && mod.default ? mod.default : mod;
    const localTools = new Map();
    const fakeRegistry = {
      register: (t) => { localTools.set(t.name, t); },
    };
    if (typeof plugin.registerTools === 'function') {
      plugin.registerTools(fakeRegistry);
    } else if (typeof plugin.getTools === 'function') {
      // getTools() 仅返回 ToolDefinition 元数据（无 execute），无法在沙箱中执行
      parentPort.postMessage({
        isError: true,
        content: 'plugin does not expose executable tools via registerTools; cannot run in sandbox',
      });
      return;
    }
    const tool = localTools.get(workerData.toolName);
    if (!tool) {
      parentPort.postMessage({ isError: true, content: 'tool not found in sandbox: ' + workerData.toolName });
      return;
    }
    const result = await tool.execute(workerData.args);
    const content = typeof result === 'string' ? result : JSON.stringify(result, null, 2);
    parentPort.postMessage({ isError: false, content });
  } catch (err) {
    const msg = err && err.message ? err.message : String(err);
    parentPort.postMessage({ isError: true, content: msg });
  }
})();
`;

/** 沙箱配置 */
export interface PluginSandboxOptions {
  /** 单次工具执行超时（毫秒），默认 5000 */
  timeoutMs?: number;
  /** 是否启用 Worker 隔离，默认按运行环境自动探测 */
  enabled?: boolean;
  /** 根据工具名解析其所属插件模块的 specifier；未提供则降级为主线程执行 */
  resolvePluginModule?: (toolName: string) => string | undefined;
}

/** 判断是否具备 Worker Threads 隔离能力（仅 Node.js 主线程） */
function canUseWorkerThreads(): boolean {
  try {
    if (typeof process === 'undefined' || process.release?.name !== 'node') return false;
    return true;
  } catch {
    return false;
  }
}

/** 插件沙箱：隔离 + 超时 + 错误隔离的工具执行器 */
export class PluginSandbox {
  private registry: ToolRegistry;
  private timeoutMs: number;
  private enabled: boolean;
  private resolvePluginModule?: (toolName: string) => string | undefined;

  constructor(registry: ToolRegistry, options?: PluginSandboxOptions) {
    this.registry = registry;
    this.timeoutMs = options?.timeoutMs ?? 5000;
    this.resolvePluginModule = options?.resolvePluginModule;
    this.enabled = options?.enabled ?? canUseWorkerThreads();
  }

  /** 执行工具调用：优先 Worker 隔离，失败降级为主线程 */
  async execute(call: ToolCall): Promise<ToolResult> {
    const moduleSpecifier = this.resolvePluginModule?.(call.name);
    if (this.enabled && moduleSpecifier) {
      try {
        return await this.executeInWorker(call, moduleSpecifier);
      } catch {
        // Worker 创建失败（例如运行时不支持），降级到主线程
        return this.executeDirect(call);
      }
    }
    return this.executeDirect(call);
  }

  /** 主线程执行（带超时与错误隔离） */
  private async executeDirect(call: ToolCall): Promise<ToolResult> {
    try {
      const result = await withTimeout(this.registry.execute(call), this.timeoutMs);
      return result;
    } catch (err) {
      return {
        toolCallId: call.id,
        content: err instanceof Error ? err.message : String(err),
        isError: true,
      };
    }
  }

  /** Worker 隔离执行 */
  private async executeInWorker(call: ToolCall, moduleSpecifier: string): Promise<ToolResult> {
    // 动态导入以避免在非 Node 运行时顶层加载失败
    const wt = await import('node:worker_threads');
    const worker = new wt.Worker(WORKER_CODE, {
      eval: true,
      workerData: {
        pluginModule: moduleSpecifier,
        toolName: call.name,
        args: safeParseArgs(call.arguments),
      },
    });

    return new Promise<ToolResult>((resolve) => {
      let settled = false;
      const finish = (r: ToolResult): void => {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        resolve(r);
      };

      const timer = setTimeout(() => {
        worker.terminate().catch(() => undefined);
        finish({
          toolCallId: call.id,
          content: `plugin tool "${call.name}" execution timed out after ${this.timeoutMs}ms`,
          isError: true,
        });
      }, this.timeoutMs);

      worker.on('message', (msg: { isError: boolean; content: string }) => {
        finish({ toolCallId: call.id, content: msg.content, isError: msg.isError });
      });
      worker.on('error', (err: Error) => {
        finish({ toolCallId: call.id, content: `worker error: ${err.message}`, isError: true });
      });
      worker.on('exit', (code: number) => {
        if (!settled) {
          finish({
            toolCallId: call.id,
            content: `worker exited with code ${code}`,
            isError: true,
          });
        }
      });
    });
  }
}

/** Promise 超时包装 */
function withTimeout<T>(p: Promise<T>, ms: number): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`execution timed out after ${ms}ms`)), ms);
    p.then(
      (v) => {
        clearTimeout(timer);
        resolve(v);
      },
      (e) => {
        clearTimeout(timer);
        reject(e);
      },
    );
  });
}

/** 安全解析工具参数（失败回退空对象） */
function safeParseArgs(args: string): unknown {
  try {
    return JSON.parse(args);
  } catch {
    return {};
  }
}
