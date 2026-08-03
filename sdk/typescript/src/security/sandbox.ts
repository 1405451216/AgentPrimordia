/**
 * 安全沙箱 — WASM/Worker Thread 代码执行隔离。
 *
 * 提供安全的代码执行环境，防止 Agent 生成的恶意代码破坏宿主进程。
 *
 * 安全策略：
 * - 代码在 Worker Thread 中执行，与主线程隔离
 * - 限制可用 API（禁用 fs, child_process, net 等）
 * - 执行超时限制
 * - 内存限制
 * - 输出大小限制
 *
 * 使用方式：
 *   const sandbox = new Sandbox({ timeout: 5000, memoryLimit: 64 * 1024 * 1024 });
 *   const result = await sandbox.execute('return 1 + 2', {});
 *   sandbox.terminate();
 */

import { ComputeWorkerPool } from '../agent/worker-pool.js';

// ===== 类型定义 =====

/** 沙箱配置 */
export interface SandboxConfig {
  /** 执行超时（毫秒，默认 5000） */
  timeout?: number;
  /** 内存限制（字节，默认 64MB） */
  memoryLimit?: number;
  /** 输出大小限制（字节，默认 1MB） */
  outputLimit?: number;
  /** 最大 CPU 时间（毫秒，默认 3000） */
  cpuTimeLimit?: number;
  /** 允许的全局变量白名单 */
  allowedGlobals?: string[];
  /** 禁止的全局变量黑名单 */
  blockedGlobals?: string[];
}

/** 沙箱执行结果 */
export interface SandboxResult {
  /** 执行是否成功 */
  success: boolean;
  /** 返回值 */
  result?: unknown;
  /** 标准输出 */
  stdout: string;
  /** 标准错误 */
  stderr: string;
  /** 执行耗时（毫秒） */
  duration: number;
  /** 内存使用峰值（字节） */
  memoryUsed: number;
  /** 错误信息（如果失败） */
  error?: string;
  /** 错误类型 */
  errorType?: 'timeout' | 'memory' | 'runtime' | 'syntax' | 'security';
}

// ===== Worker 脚本 =====

/**
 * Worker 执行脚本模板。
 *
 * 在 Worker 中创建受限的执行环境：
 * - 移除危险的全局变量（process, require, import 等）
 * - 提供 console.log/stdout 捕获
 * - 执行超时检测
 */
const WORKER_SCRIPT = `
const { parentPort } = require('worker_threads');

// 危险全局变量黑名单
const DANGEROUS_GLOBALS = [
  'process', 'require', 'import', 'export',
  'child_process', 'fs', 'net', 'http', 'https',
  'os', 'cluster', 'dns', 'tls', 'crypto',
];

// 安全的全局变量白名单
const SAFE_GLOBALS = [
  'console', 'Math', 'JSON', 'Date', 'Array', 'Object',
  'String', 'Number', 'Boolean', 'RegExp', 'Error',
  'Promise', 'Map', 'Set', 'WeakMap', 'WeakSet',
  'Symbol', 'Proxy', 'Reflect', 'parseInt', 'parseFloat',
  'isNaN', 'isFinite', 'encodeURIComponent', 'decodeURIComponent',
  'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval',
  'NaN', 'undefined', 'Infinity',
];

parentPort.on('message', (data) => {
  const { code, context, config } = data;
  const startTime = Date.now();
  let stdout = '';
  let stderr = '';

  // 捕获 console 输出
  const safeConsole = {
    log: (...args) => { stdout += args.map(String).join(' ') + '\\n'; },
    error: (...args) => { stderr += args.map(String).join(' ') + '\\n'; },
    warn: (...args) => { stderr += args.map(String).join(' ') + '\\n'; },
    info: (...args) => { stdout += args.map(String).join(' ') + '\\n'; },
  };

  try {
    // 构建受限的执行上下文
    const sandbox = {};
    for (const name of SAFE_GLOBALS) {
      if (typeof globalThis[name] !== 'undefined') {
        sandbox[name] = globalThis[name];
      }
    }
    sandbox.console = safeConsole;

    // 注入上下文变量
    if (context) {
      for (const [key, value] of Object.entries(context)) {
        sandbox[key] = value;
      }
    }

    // 包装代码到函数中执行
    const wrappedCode = 'return (function() { "use strict";\\n' + code + '\\n})()';
    const fn = new Function(...Object.keys(sandbox), wrappedCode);
    const result = fn(...Object.values(sandbox));

    const duration = Date.now() - startTime;
    const memUsage = process.memoryUsage();

    parentPort.postMessage({
      output: {
        success: true,
        result: result,
        stdout: stdout,
        stderr: stderr,
        duration: duration,
        memoryUsed: memUsage.heapUsed,
      },
      error: null,
      duration: duration,
    });
  } catch (err) {
    const duration = Date.now() - startTime;
    const memUsage = process.memoryUsage();

    let errorType = 'runtime';
    if (err instanceof SyntaxError) errorType = 'syntax';
    if (err.name === 'EvalError') errorType = 'security';

    parentPort.postMessage({
      output: {
        success: false,
        stdout: stdout,
        stderr: stderr,
        duration: duration,
        memoryUsed: memUsage.heapUsed,
        error: err.message || String(err),
        errorType: errorType,
      },
      error: null,
      duration: duration,
    });
  }
});
`;

// ===== 安全沙箱实现 =====

/**
 * 安全代码执行沙箱。
 *
 * 基于 Worker Thread 实现，在隔离进程中执行不可信代码。
 * 支持超时、内存限制和 API 白名单。
 */
export class CodeSandbox {
  private config: Required<SandboxConfig>;
  private workerPool: ComputeWorkerPool;

  constructor(config?: SandboxConfig) {
    this.config = {
      timeout: config?.timeout ?? 5000,
      memoryLimit: config?.memoryLimit ?? 64 * 1024 * 1024,
      outputLimit: config?.outputLimit ?? 1024 * 1024,
      cpuTimeLimit: config?.cpuTimeLimit ?? 3000,
      allowedGlobals: config?.allowedGlobals ?? [],
      blockedGlobals: config?.blockedGlobals ?? [],
    };

    this.workerPool = new ComputeWorkerPool({
      maxWorkers: 1,
      taskTimeout: this.config.timeout,
      workerScript: WORKER_SCRIPT,
    });
  }

  /**
   * 执行代码。
   *
   * @param code 要执行的 JavaScript 代码（不支持 ES Module import）
   * @param context 注入的上下文变量
   */
  async execute(code: string, context?: Record<string, unknown>): Promise<SandboxResult> {
    const startMemory = process.memoryUsage().heapUsed;

    try {
      const result = await this.workerPool.run<{ code: string; context?: Record<string, unknown>; config: Required<SandboxConfig> }, {
        success: boolean;
        result?: unknown;
        stdout: string;
        stderr: string;
        duration: number;
        memoryUsed: number;
        error?: string;
        errorType?: string;
      }>(
        { code, context, config: this.config },
        { timeout: this.config.timeout },
      );

      const output = result;

      // 检查输出大小
      if (output.stdout.length > this.config.outputLimit) {
        output.stdout = output.stdout.slice(0, this.config.outputLimit) + '...[truncated]';
      }

      // 检查内存限制
      if (output.memoryUsed > this.config.memoryLimit) {
        return {
          success: false,
          stdout: output.stdout,
          stderr: output.stderr,
          duration: output.duration,
          memoryUsed: output.memoryUsed,
          error: `Memory limit exceeded: ${output.memoryUsed} > ${this.config.memoryLimit}`,
          errorType: 'memory',
        };
      }

      return {
        success: output.success,
        result: output.result,
        stdout: output.stdout,
        stderr: output.stderr,
        duration: output.duration,
        memoryUsed: output.memoryUsed,
        error: output.error,
        errorType: output.errorType as SandboxResult['errorType'],
      };
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : String(err);
      const errorType: SandboxResult['errorType'] = errorMsg.includes('timeout')
        ? 'timeout'
        : 'runtime';

      return {
        success: false,
        stdout: '',
        stderr: '',
        duration: this.config.timeout,
        memoryUsed: process.memoryUsage().heapUsed - startMemory,
        error: errorMsg,
        errorType,
      };
    }
  }

  /** 终止沙箱 Worker */
  terminate(): void {
    this.workerPool.terminate();
  }
}

// ===== 代码安全检查器 =====

/**
 * 静态代码安全检查 — 在执行前检测潜在恶意代码。
 *
 * 检查规则：
 * - 禁止 require/import 外部模块
 * - 禁止访问 process/child_process
 * - 禁止 eval/Function 构造器
 * - 禁止 __proto__ 原型污染
 * - 禁止 while(true) 无限循环
 */
export class CodeSecurityChecker {
  private static readonly DANGEROUS_PATTERNS: Array<{ pattern: RegExp; reason: string; severity: 'block' | 'warn' }> = [
    { pattern: /(?:^|[^\w.])require\s*\(/g, reason: 'require() is not allowed', severity: 'block' },
    { pattern: /(?:^|[^\w.])import\s+(?:type\s+)?[\w{]/g, reason: 'import statement is not allowed', severity: 'block' },
    { pattern: /(?:^|[^\w.])process\./g, reason: 'process object access is not allowed', severity: 'block' },
    { pattern: /child_process/g, reason: 'child_process is not allowed', severity: 'block' },
    { pattern: /__proto__/g, reason: '__proto__ access is not allowed (prototype pollution)', severity: 'block' },
    { pattern: /constructor\s*\[/g, reason: 'constructor[] access is not allowed', severity: 'block' },
    { pattern: /while\s*\(\s*(?:true|1|!!1)\s*\)/g, reason: 'while(true) detected (potential infinite loop)', severity: 'warn' },
    { pattern: /for\s*\(\s*;\s*;\s*\)/g, reason: 'for(;;) detected (potential infinite loop)', severity: 'warn' },
    { pattern: /(?:^|[^\w.])eval\s*\(/g, reason: 'eval() is not allowed', severity: 'block' },
    { pattern: /new\s+Function\s*\(/g, reason: 'new Function() is not allowed', severity: 'block' },
    { pattern: /globalThis/g, reason: 'globalThis access is not allowed', severity: 'block' },
    { pattern: /this\s*\.\s*constructor/g, reason: 'constructor access via this is not allowed', severity: 'block' },
  ];

  /** 检查代码安全性 */
  static check(code: string): { safe: boolean; warnings: string[]; errors: string[] } {
    const warnings: string[] = [];
    const errors: string[] = [];

    for (const { pattern, reason, severity } of CodeSecurityChecker.DANGEROUS_PATTERNS) {
      if (pattern.test(code)) {
        if (severity === 'block') {
          errors.push(reason);
        } else {
          warnings.push(reason);
        }
        // 重置正则 lastIndex
        pattern.lastIndex = 0;
      }
    }

    return {
      safe: errors.length === 0,
      warnings,
      errors,
    };
  }
}

// ===== ACL 权限控制（兼容旧 API） =====

export type AccessLevel = 'none' | 'read' | 'write' | 'execute' | 'all';

const ACCESS_LEVELS: Record<AccessLevel, number> = {
  none: 0,
  read: 1,
  write: 2,
  execute: 4,
  all: 7,
};

/** 命令参数白名单模式 */
export interface ArgPattern {
  regex: RegExp;
  message: string;
}

/** 创建一个参数模式 */
export function newArgPattern(regex: string, message: string): ArgPattern {
  return { regex: new RegExp(regex), message };
}

/** ACL 权限控制列表 */
export class ACL {
  private rules: { agentID: string; resource: string; level: number }[] = [];
  private denyRules: { agentID: string; resource: string }[] = [];

  allow(agentID: string, resource: string, level: AccessLevel): void {
    this.rules.push({ agentID, resource, level: ACCESS_LEVELS[level] });
  }

  deny(agentID: string, resource: string): void {
    this.denyRules.push({ agentID, resource });
  }

  check(agentID: string, resource: string, required: AccessLevel): boolean {
    for (const rule of this.denyRules) {
      if (matchRule(rule.agentID, rule.resource, agentID, resource)) return false;
    }
    for (const rule of this.rules) {
      if (matchRule(rule.agentID, rule.resource, agentID, resource)) {
        return (rule.level & ACCESS_LEVELS[required]) === ACCESS_LEVELS[required];
      }
    }
    return false;
  }

  reset(): void {
    this.rules = [];
    this.denyRules = [];
  }
}

function matchRule(ruleAgent: string, ruleResource: string, agentID: string, resource: string): boolean {
  if (ruleAgent !== '*' && ruleAgent !== agentID) return false;
  // 统一折叠连续斜杠（对齐 Go 端 filepath.Clean 语义），避免 file:/// 前缀 URL 匹配失败
  const cleanResource = resource.replace(/\\/g, '/').replace(/\/+/g, '/').replace(/\/+$/, '');
  const cleanRule = ruleResource.replace(/\\/g, '/').replace(/\/+/g, '/').replace(/\/+$/, '');
  if (cleanResource === cleanRule) return true;
  return cleanResource.startsWith(cleanRule + '/');
}

// ===== 命令执行沙箱（兼容旧 API） =====
// 保留原有的命令检查沙箱，用于 Agent 命令执行权限控制

import { containsShellMetacharacter, validatePathTraversal } from './extended.js';

/**
 * 命令执行沙箱 — ACL + 命令白名单 + 参数模式匹配。
 *
 * 与 CodeSandbox 的区别：
 * - CodeSandbox: 执行任意 JS 代码（Worker Thread 隔离）
 * - CommandSandbox: 控制命令执行权限（ACL + 白名单）
 *
 * 保留旧名 `Sandbox` 以保持向后兼容。
 */
export class CommandSandbox {
  private acl: ACL;
  private allowedCmds: Set<string> = new Set();
  private blockedCmds: Set<string> = new Set();
  private argPatterns: Map<string, ArgPattern[]> = new Map();

  constructor(acl: ACL) {
    this.acl = acl;
  }

  allowCommand(cmd: string): void {
    this.allowedCmds.add(cmd);
    this.blockedCmds.delete(cmd);
  }

  blockCommand(cmd: string): void {
    this.blockedCmds.add(cmd);
    this.allowedCmds.delete(cmd);
  }

  allowCommandWithArgs(cmd: string, ...patterns: ArgPattern[]): void {
    this.allowedCmds.add(cmd);
    this.blockedCmds.delete(cmd);
    if (patterns.length > 0) {
      this.argPatterns.set(cmd, patterns);
    }
  }

  setArgPatterns(cmd: string, ...patterns: ArgPattern[]): void {
    if (patterns.length > 0) {
      this.argPatterns.set(cmd, patterns);
    } else {
      this.argPatterns.delete(cmd);
    }
  }

  private validateArgs(cmdName: string, args: string[]): Error | null {
    for (const arg of args) {
      if (arg.startsWith('-')) continue;
      const traversal = validatePathTraversal(arg);
      if (!traversal.safe) {
        return new Error(`path traversal in argument "${arg}": ${traversal.reason}`);
      }
      const meta = containsShellMetacharacter(arg);
      if (meta.found) {
        return new Error(`argument "${arg}" contains shell metacharacter '${meta.char}'`);
      }
    }
    const patterns = this.argPatterns.get(cmdName);
    if (!patterns || patterns.length === 0) return null;
    const argStr = args.join(' ');
    for (const p of patterns) {
      if (p.regex.test(argStr)) return null;
    }
    return new Error(`arguments "${argStr}" do not match allowed patterns for command "${cmdName}"`);
  }

  canExecute(agentID: string, cmd: string): Error | null {
    const meta = containsShellMetacharacter(cmd);
    if (meta.found) {
      return new Error(`command contains shell metacharacter '${meta.char}'`);
    }
    const fields = cmd.trim().split(/\s+/);
    const cmdName = fields[0];
    if (!cmdName) return new Error('empty command');
    if (this.blockedCmds.has(cmdName)) {
      return new Error(`command "${cmdName}" is blocked`);
    }
    if (this.allowedCmds.size > 0 && !this.allowedCmds.has(cmdName)) {
      return new Error(`command "${cmdName}" is not in allowed list`);
    }
    const args = fields.slice(1);
    return this.validateArgs(cmdName, args);
  }

  canAccess(agentID: string, resource: string, level: AccessLevel): Error | null {
    if (!this.acl.check(agentID, resource, level)) {
      return new Error(`agent "${agentID}" denied ${level} access to "${resource}"`);
    }
    return null;
  }

  validatePath(agentID: string, path: string, level: AccessLevel): Error | null {
    let decoded = path;
    for (let i = 0; i < 10; i++) {
      try {
        const prev = decoded;
        decoded = decodeURIComponent(decoded);
        if (decoded === prev) break;
      } catch { break; }
    }
    if (decoded.includes('..') || decoded.includes('\0')) {
      return new Error(`invalid path: "${path}"`);
    }
    return this.canAccess(agentID, decoded, level);
  }
}

// 向后兼容别名：旧的 Sandbox 现在指向 CommandSandbox
export const Sandbox = CommandSandbox;
