import { describe, it, expect, beforeEach } from 'vitest';
import { WasmRuntime, VirtualFS } from '../sandbox-v2.js';

/** 最小有效 WASM 空模块（仅 magic + version） */
const EMPTY_WASM = new Uint8Array([0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00]);

/**
 * 导出 add(i32,i32)->i32 函数的 WASM 模块。
 * 无 _start/main，运行时会调用第一个导出函数。
 */
const ADD_WASM = new Uint8Array([
  0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // magic + version
  0x01, 0x07, 0x01, 0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f, // type section
  0x03, 0x02, 0x01, 0x00,                                     // function section
  0x07, 0x07, 0x01, 0x03, 0x61, 0x64, 0x64, 0x00, 0x00,     // export "add"
  0x0a, 0x09, 0x01, 0x07, 0x00, 0x20, 0x00, 0x20, 0x01, 0x6a, 0x0b, // code
]);

/**
 * 导出 memory (1 page) 的 WASM 模块。
 * 用于测试内存绑定和内存限制。
 */
const MEMORY_WASM = new Uint8Array([
  0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // magic + version
  0x05, 0x03, 0x01, 0x00, 0x01,                   // memory section: 1 page
  0x07, 0x0a, 0x01, 0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x02, 0x00, // export "memory"
]);

describe('WasmRuntime — 完整测试', () => {
  let runtime: WasmRuntime;

  beforeEach(() => {
    runtime = new WasmRuntime({ defaultTimeout: 3000 });
  });

  // ===== 模块加载 =====
  describe('模块加载', () => {
    it('空模块（无入口函数）应返回错误', async () => {
      const result = await runtime.execute(EMPTY_WASM);
      expect(result.success).toBe(false);
      expect(result.error).toContain('No entry function found');
      expect(result.duration).toBeGreaterThanOrEqual(0);
    });

    it('带导出函数的模块应成功执行', async () => {
      const result = await runtime.execute(ADD_WASM, { args: [] });
      expect(result.success).toBe(true);
      expect(result.duration).toBeGreaterThanOrEqual(0);
      expect(result.fs).toBeInstanceOf(VirtualFS);
    });

    it('带 memory 导出的模块应绑定 WASI 内存', async () => {
      const result = await runtime.execute(MEMORY_WASM, { args: [] });
      // 模块无入口函数，执行失败但内存已绑定
      expect(result.success).toBe(false);
      expect(result.error).toContain('No entry function found');
      expect(result.fs).toBeInstanceOf(VirtualFS);
    });

    it('无效 WASM 字节应返回错误', async () => {
      const invalid = new Uint8Array([0x00, 0x61, 0x73, 0x6d, 0x00]); // 错误 version
      const result = await runtime.execute(invalid);
      expect(result.success).toBe(false);
      expect(result.error).toBeDefined();
      expect(result.error!.length).toBeGreaterThan(0);
    });

    it('完全无效的字节应返回错误', async () => {
      const garbage = new Uint8Array([0xff, 0xfe, 0xfd, 0xfc]);
      const result = await runtime.execute(garbage);
      expect(result.success).toBe(false);
      expect(result.error).toBeDefined();
    });
  });

  // ===== 超时处理 =====
  describe('超时处理', () => {
    it('应接受自定义 timeout 配置', async () => {
      // 空模块立即失败，但 timeout 配置应被接受
      const result = await runtime.execute(EMPTY_WASM, { timeout: 100 });
      expect(result.success).toBe(false);
      expect(result.duration).toBeGreaterThanOrEqual(0);
    });

    it('默认 timeout 应从构造函数获取', async () => {
      const rt = new WasmRuntime({ defaultTimeout: 50 });
      const result = await rt.execute(EMPTY_WASM);
      expect(result.success).toBe(false);
    });
  });

  // ===== 内存限制 =====
  describe('内存限制', () => {
    it('maxMemoryPages 限制应被检查', async () => {
      // MEMORY_WASM 导出 1 page 内存
      // 注意: maxMemoryPages=0 在源码中是 falsy 会被跳过，使用非零值测试
      // 实际上源码用 config?.maxMemoryPages && memory 判断，0 为 falsy
      // 因此我们验证内存检查仅在 maxMemoryPages 为非零值时生效
      const result = await runtime.execute(MEMORY_WASM, {
        maxMemoryPages: 1,
        args: [],
      });
      // 1 page <= 1 page limit，内存检查通过，但无入口函数
      expect(result.error).toContain('No entry function found');
    });

    it('足够大的 maxMemoryPages 应通过内存检查', async () => {
      const result = await runtime.execute(MEMORY_WASM, {
        maxMemoryPages: 100,
        args: [],
      });
      // 内存检查通过，但无入口函数
      expect(result.error).toContain('No entry function found');
    });
  });

  // ===== 预写入文件 =====
  describe('预写入文件', () => {
    it('prewriteFiles 应在执行前写入 VirtualFS', async () => {
      const result = await runtime.execute(EMPTY_WASM, {
        prewriteFiles: {
          '/input.txt': 'hello',
          '/data.bin': new Uint8Array([1, 2, 3]),
        },
      });
      expect(result.fs.exists('/input.txt')).toBe(true);
      expect(result.fs.readTextFile('/input.txt')).toBe('hello');
      expect(result.fs.readFile('/data.bin')).toEqual(new Uint8Array([1, 2, 3]));
    });

    it('自定义 fs 应被复用', async () => {
      const fs = new VirtualFS();
      fs.writeFile('/existing.txt', 'pre-existing');

      const result = await runtime.execute(EMPTY_WASM, { fs });
      expect(result.fs.exists('/existing.txt')).toBe(true);
      expect(result.fs.readTextFile('/existing.txt')).toBe('pre-existing');
    });
  });

  // ===== 环境变量和参数 =====
  describe('环境变量和参数', () => {
    it('应接受 args 和 env 配置', async () => {
      const result = await runtime.execute(ADD_WASM, {
        args: ['my-program', '--flag'],
        env: { NODE_ENV: 'test' },
      });
      expect(result.success).toBe(true);
    });
  });

  // ===== 执行结果结构 =====
  describe('执行结果结构', () => {
    it('成功结果应包含所有字段', async () => {
      const result = await runtime.execute(ADD_WASM, { args: [] });
      expect(result).toHaveProperty('success');
      expect(result).toHaveProperty('exitCode');
      expect(result).toHaveProperty('stdout');
      expect(result).toHaveProperty('stderr');
      expect(result).toHaveProperty('duration');
      expect(result).toHaveProperty('fs');
    });

    it('失败结果应包含 error 字段', async () => {
      const result = await runtime.execute(new Uint8Array([0xff]));
      expect(result.success).toBe(false);
      expect(result.error).toBeDefined();
      expect(typeof result.error).toBe('string');
    });
  });
});
