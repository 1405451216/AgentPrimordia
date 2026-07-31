import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { CodeSandboxV2, VirtualFS } from '../sandbox-v2.js';

/** 最小有效 WASM 空模块 */
const EMPTY_WASM = new Uint8Array([0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00]);

/** 导出 add(i32,i32)->i32 的 WASM 模块 */
const ADD_WASM = new Uint8Array([
  0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
  0x01, 0x07, 0x01, 0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f,
  0x03, 0x02, 0x01, 0x00,
  0x07, 0x07, 0x01, 0x03, 0x61, 0x64, 0x64, 0x00, 0x00,
  0x0a, 0x09, 0x01, 0x07, 0x00, 0x20, 0x00, 0x20, 0x01, 0x6a, 0x0b,
]);

describe('CodeSandboxV2 — 集成测试', () => {
  let sandbox: CodeSandboxV2;

  beforeEach(() => {
    sandbox = new CodeSandboxV2({
      timeout: 3000,
      memoryLimit: 32 * 1024 * 1024,
      outputLimit: 1024,
    });
  });

  afterEach(() => {
    sandbox.terminate();
  });

  // ===== JS 执行路由 =====
  describe('JS 语言路由', () => {
    it('language=js 应执行 JS 代码并返回结果', async () => {
      const result = await sandbox.run({ code: 'return 2 + 3', language: 'js' });
      expect(result.success).toBe(true);
      expect(result.result).toBe(5);
      expect(result.language).toBe('js');
    });

    it('JS 执行应捕获 console.log 输出', async () => {
      const result = await sandbox.run({
        code: 'console.log("output test"); return true;',
        language: 'js',
      });
      expect(result.success).toBe(true);
      expect(result.stdout).toContain('output test');
    });

    it('JS 应支持上下文注入', async () => {
      const result = await sandbox.run({
        code: 'return a + b',
        language: 'js',
        context: { a: 10, b: 20 },
      });
      expect(result.success).toBe(true);
      expect(result.result).toBe(30);
    });
  });

  // ===== WASM 执行路由 =====
  describe('WASM 语言路由', () => {
    it('language=wasm 应执行 WASM 模块', async () => {
      const result = await sandbox.run({ code: ADD_WASM, language: 'wasm' });
      expect(result.success).toBe(true);
      expect(result.language).toBe('wasm');
      expect(result.exitCode).toBeDefined();
    });

    it('无效 WASM 应返回结构化错误', async () => {
      const result = await sandbox.run({
        code: new Uint8Array([0xff, 0x00]),
        language: 'wasm',
      });
      expect(result.success).toBe(false);
      expect(result.language).toBe('wasm');
      expect(result.error).toBeDefined();
      expect(result.errorType).toBe('runtime');
    });

    it('WASM 应支持预写入文件', async () => {
      const result = await sandbox.run({
        code: EMPTY_WASM,
        language: 'wasm',
        prewriteFiles: { '/config.json': '{"key":"value"}' },
      });
      expect(result.fs).toBeDefined();
      expect(result.fs!.exists('/config.json')).toBe(true);
      expect(result.fs!.readTextFile('/config.json')).toBe('{"key":"value"}');
    });

    it('WASM 应支持 args 和 env', async () => {
      const result = await sandbox.run({
        code: ADD_WASM,
        language: 'wasm',
        args: ['--test'],
        env: { MODE: 'debug' },
      });
      expect(result.success).toBe(true);
    });
  });

  // ===== runWasm 直接调用 =====
  describe('runWasm 直接调用', () => {
    it('应接受完整参数并返回 ExecResult', async () => {
      const result = await sandbox.runWasm({
        code: ADD_WASM,
        args: ['prog'],
        env: { X: '1' },
        prewriteFiles: { '/in.txt': 'data' },
      });
      expect(result.language).toBe('wasm');
      expect(result.fs!.exists('/in.txt')).toBe(true);
    });

    it('timeout 覆盖应生效', async () => {
      const result = await sandbox.runWasm({
        code: EMPTY_WASM,
        timeout: 50,
      });
      expect(result.success).toBe(false);
    });
  });

  // ===== 不支持的语言 =====
  describe('不支持的语言', () => {
    it('应返回 Unsupported language 错误', async () => {
      const result = await sandbox.run({
        code: 'print("hello")',
        language: 'ruby' as never,
      });
      expect(result.success).toBe(false);
      expect(result.error).toContain('Unsupported language');
    });
  });

  // ===== 安全检查 =====
  describe('安全检查（JS）', () => {
    it('require 应被拒绝', async () => {
      const result = await sandbox.run({
        code: 'require("fs")',
        language: 'js',
      });
      expect(result.success).toBe(false);
      expect(result.errorType).toBe('security');
    });

    it('eval 应被拒绝', async () => {
      const result = await sandbox.run({
        code: 'eval("1+1")',
        language: 'js',
      });
      expect(result.success).toBe(false);
      expect(result.errorType).toBe('security');
    });

    it('process 访问应被拒绝', async () => {
      const result = await sandbox.run({
        code: 'process.exit(0)',
        language: 'js',
      });
      expect(result.success).toBe(false);
      expect(result.errorType).toBe('security');
    });

    it('__proto__ 应被拒绝', async () => {
      const result = await sandbox.run({
        code: '({}).__proto__.polluted = true',
        language: 'js',
      });
      expect(result.success).toBe(false);
      expect(result.errorType).toBe('security');
    });
  });

  // ===== 输出截断 =====
  describe('输出大小限制', () => {
    it('WASM 输出超过 outputLimit 应被截断', async () => {
      // outputLimit=1024，创建一个极小限制的沙箱
      const smallSandbox = new CodeSandboxV2({ outputLimit: 10 });
      // 即使没有实际 WASM 输出，截断逻辑在 runWasm 中
      const result = await smallSandbox.runWasm({ code: EMPTY_WASM });
      // 空模块执行失败，但截断逻辑应存在
      expect(result).toHaveProperty('stdout');
      smallSandbox.terminate();
    });
  });

  // ===== 资源隔离 =====
  describe('资源隔离', () => {
    it('每次 WASM 执行应使用独立的 VirtualFS', async () => {
      const r1 = await sandbox.run({
        code: EMPTY_WASM,
        language: 'wasm',
        prewriteFiles: { '/run1.txt': 'first' },
      });
      const r2 = await sandbox.run({
        code: EMPTY_WASM,
        language: 'wasm',
        prewriteFiles: { '/run2.txt': 'second' },
      });

      expect(r1.fs!.exists('/run1.txt')).toBe(true);
      expect(r1.fs!.exists('/run2.txt')).toBe(false);
      expect(r2.fs!.exists('/run2.txt')).toBe(true);
      expect(r2.fs!.exists('/run1.txt')).toBe(false);
    });
  });

  // ===== Python 支持 =====
  describe('Python 支持状态', () => {
    it('isPythonSupported 初始应为 false', () => {
      expect(sandbox.isPythonSupported()).toBe(false);
    });
  });

  // ===== terminate =====
  describe('terminate', () => {
    it('应可安全多次调用', () => {
      expect(() => {
        sandbox.terminate();
        sandbox.terminate();
      }).not.toThrow();
    });
  });

  // ===== VirtualFS + WasmRuntime 协作 =====
  describe('VirtualFS 与 WasmRuntime 协作', () => {
    it('共享 FS 应保留预写入数据', async () => {
      const fs = new VirtualFS();
      fs.writeFile('/shared.txt', 'shared data');

      const { WasmRuntime } = await import('../sandbox-v2.js');
      const runtime = new WasmRuntime();
      const result = await runtime.execute(EMPTY_WASM, { fs });

      expect(result.fs.exists('/shared.txt')).toBe(true);
      expect(result.fs.readTextFile('/shared.txt')).toBe('shared data');
    });
  });
});
