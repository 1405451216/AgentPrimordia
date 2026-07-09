import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  VirtualFS,
  WasiShim,
  WasmRuntime,
  CodeSandboxV2,
} from '../../src/security/sandbox-v2.js';
import type { ExecResult } from '../../src/security/sandbox-v2.js';

// ===== VirtualFS 测试 =====

describe('VirtualFS', () => {
  let fs: VirtualFS;

  beforeEach(() => {
    fs = new VirtualFS({ maxFileSize: 1024, maxTotalBytes: 4096 });
  });

  describe('基本文件操作', () => {
    it('应该写入和读取文件', () => {
      fs.writeFile('/test.txt', 'hello world');
      expect(fs.exists('/test.txt')).toBe(true);
      expect(fs.readTextFile('/test.txt')).toBe('hello world');
    });

    it('应该处理二进制数据', () => {
      const data = new Uint8Array([0, 1, 2, 3, 255, 254]);
      fs.writeFile('/bin.dat', data);
      const result = fs.readFile('/bin.dat');
      expect(result).toEqual(data);
    });

    it('读取不存在的文件应抛出错误', () => {
      expect(() => fs.readFile('/nonexistent.txt')).toThrow('File not found');
    });

    it('exists 应正确返回', () => {
      expect(fs.exists('/nope.txt')).toBe(false);
      fs.writeFile('/nope.txt', 'data');
      expect(fs.exists('/nope.txt')).toBe(true);
    });
  });

  describe('路径解析', () => {
    it('应该规范化相对路径', () => {
      fs.writeFile('/dir/sub/file.txt', 'content');
      expect(fs.exists('/dir/../dir/sub/file.txt')).toBe(true);
      expect(fs.exists('./dir/sub/file.txt')).toBe(true);
    });

    it('应该处理 .. 路径遍历', () => {
      fs.writeFile('/a/b/c.txt', 'data');
      // .. 应该在虚拟根目录内解析，不会逃逸
      expect(fs.exists('/a/b/../b/c.txt')).toBe(true);
    });

    it('写根目录应抛出错误', () => {
      expect(() => fs.writeFile('/', 'data')).toThrow('Cannot write to root');
    });
  });

  describe('目录操作', () => {
    it('应该创建目录（递归）', () => {
      fs.mkdir('/deep/nested/dir');
      expect(fs.exists('/deep/nested/dir')).toBe(true);
    });

    it('应该列出目录内容', () => {
      fs.writeFile('/file1.txt', 'a');
      fs.writeFile('/file2.txt', 'b');
      fs.mkdir('/subdir');
      const listing = fs.listDir('/');
      expect(listing).toContain('file1.txt');
      expect(listing).toContain('file2.txt');
      expect(listing).toContain('subdir');
    });

    it('listDir 非目录应抛出错误', () => {
      fs.writeFile('/file.txt', 'data');
      expect(() => fs.listDir('/file.txt')).toThrow('Not a directory');
    });
  });

  describe('删除操作', () => {
    it('应该删除文件', () => {
      fs.writeFile('/temp.txt', 'data');
      expect(fs.exists('/temp.txt')).toBe(true);
      fs.unlink('/temp.txt');
      expect(fs.exists('/temp.txt')).toBe(false);
    });

    it('删除根目录应抛出错误', () => {
      expect(() => fs.unlink('/')).toThrow('Cannot remove root');
    });

    it('删除不存在的文件应抛出错误', () => {
      expect(() => fs.unlink('/nope.txt')).toThrow('Not found');
    });
  });

  describe('文件描述符操作', () => {
    it('应该打开、写入、读取文件', () => {
      const fd = fs.open('/fdtest.txt', {
        read: true,
        write: true,
        create: true,
        append: false,
        truncate: false,
      });
      expect(fd).toBeGreaterThanOrEqual(3);

      fs.write(fd, new TextEncoder().encode('hello'));
      expect(fs.getUsedBytes()).toBe(5);

      // 关闭后重新打开以读取
      fs.close(fd);
      const fd2 = fs.open('/fdtest.txt', {
        read: true,
        write: false,
        create: false,
        append: false,
        truncate: false,
      });
      const data = fs.read(fd2, 100);
      expect(new TextDecoder().decode(data)).toBe('hello');
      fs.close(fd2);
    });

    it('truncate 标志应清空文件', () => {
      fs.writeFile('/trunc.txt', 'original content');
      const fd = fs.open('/trunc.txt', {
        read: true,
        write: true,
        create: false,
        append: false,
        truncate: true,
      });
      const data = fs.read(fd, 100);
      expect(data.length).toBe(0);
      fs.close(fd);
    });

    it('append 模式应追加内容', () => {
      fs.writeFile('/append.txt', 'line1\n');
      const fd = fs.open('/append.txt', {
        read: false,
        write: true,
        create: false,
        append: true,
        truncate: false,
      });
      fs.write(fd, new TextEncoder().encode('line2\n'));
      fs.close(fd);
      expect(fs.readTextFile('/append.txt')).toBe('line1\nline2\n');
    });

    it('打开不存在的文件（无 create）应抛出错误', () => {
      expect(() =>
        fs.open('/nope.txt', {
          read: true,
          write: false,
          create: false,
          append: false,
          truncate: false,
        }),
      ).toThrow('File not found');
    });

    it('无效 fd 应抛出错误', () => {
      expect(() => fs.read(999, 10)).toThrow('Invalid fd');
      expect(() => fs.write(999, new Uint8Array(1))).toThrow('Invalid fd');
    });
  });

  describe('大小限制', () => {
    it('应拒绝超过单文件大小限制的写入', () => {
      const bigData = new Uint8Array(2048); // 超过 maxFileSize=1024
      expect(() => fs.writeFile('/big.dat', bigData)).toThrow('exceeds max');
    });

    it('应拒绝超过总大小限制的写入', () => {
      fs.writeFile('/f1.dat', new Uint8Array(1024));
      fs.writeFile('/f2.dat', new Uint8Array(1024));
      fs.writeFile('/f3.dat', new Uint8Array(1024));
      fs.writeFile('/f4.dat', new Uint8Array(1024));
      // 总共 4096，再写应超限
      expect(() => fs.writeFile('/f5.dat', new Uint8Array(1))).toThrow('exceed max');
    });

    it('getUsedBytes 应返回正确大小', () => {
      fs.writeFile('/a.txt', 'hello');
      expect(fs.getUsedBytes()).toBe(5);
      fs.writeFile('/a.txt', 'hi'); // 覆盖
      expect(fs.getUsedBytes()).toBe(2);
    });
  });

  describe('reset', () => {
    it('应该清空所有内容', () => {
      fs.writeFile('/a.txt', 'data');
      fs.mkdir('/dir');
      fs.reset();
      expect(fs.exists('/a.txt')).toBe(false);
      expect(fs.exists('/dir')).toBe(false);
      expect(fs.getUsedBytes()).toBe(0);
    });
  });
});

// ===== WasiShim 测试 =====

describe('WasiShim', () => {
  it('应该正确构建 imports 对象', () => {
    const wasi = new WasiShim({
      args: ['prog', '--flag'],
      env: { FOO: 'bar' },
    });

    const imports = wasi.buildImports();
    expect(imports['wasi_snapshot_preview1']).toBeDefined();
    expect(imports['wasi_snapshot_preview1']['fd_write']).toBeDefined();
    expect(imports['wasi_snapshot_preview1']['proc_exit']).toBeDefined();
    expect(imports['wasi_snapshot_preview1']['environ_sizes_get']).toBeDefined();
    expect(imports['wasi_snapshot_preview1']['args_sizes_get']).toBeDefined();
    expect(imports['wasi_snapshot_preview1']['clock_time_get']).toBeDefined();
    expect(imports['wasi_snapshot_preview1']['random_get']).toBeDefined();
  });

  it('初始 stdout/stderr 应为空', () => {
    const wasi = new WasiShim();
    expect(wasi.getStdout()).toBe('');
    expect(wasi.getStderr()).toBe('');
    expect(wasi.getExitCode()).toBeNull();
  });

  it('应该接受自定义 args 和 env', () => {
    const wasi = new WasiShim({
      args: ['test', 'arg1'],
      env: { NODE_ENV: 'test' },
    });
    // 验证不抛错
    expect(wasi.buildImports()).toBeDefined();
  });
});

// ===== WasmRuntime 测试 =====

describe('WasmRuntime', () => {
  let runtime: WasmRuntime;

  beforeEach(() => {
    runtime = new WasmRuntime({ defaultTimeout: 3000 });
  });

  it('执行无效 WASM 应返回错误', async () => {
    const invalidWasm = new Uint8Array([0x00, 0x61, 0x73, 0x6d, 0x00]); // 不完整的 magic
    const result = await runtime.execute(invalidWasm);
    expect(result.success).toBe(false);
    expect(result.error).toBeDefined();
    expect(result.duration).toBeGreaterThanOrEqual(0);
  });

  it('应该执行简单的 WASM 模块（无 WASI）', async () => {
    // 构造一个最小的有效 WASM 模块
    // 这个模块导出一个 add 函数
    const wasmBytes = new Uint8Array([
      0x00, 0x61, 0x73, 0x6d, // magic
      0x01, 0x00, 0x00, 0x00, // version
      // Type section: (func (param i32 i32) (result i32))
      0x01, 0x07, 0x01, 0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f,
      // Function section
      0x03, 0x02, 0x01, 0x00,
      // Export section: export "add" (func 0)
      0x07, 0x07, 0x01, 0x03, 0x61, 0x64, 0x64, 0x00, 0x00,
      // Code section
      0x0a, 0x09, 0x01, 0x07, 0x00, 0x20, 0x00, 0x20, 0x01, 0x6a, 0x0b,
    ]);

    const result = await runtime.execute(wasmBytes, { args: [] });
    // 模块没有 _start，会调用第一个导出函数 add()
    // add() 无参数会失败或返回 0
    expect(result.duration).toBeGreaterThanOrEqual(0);
    expect(result.fs).toBeDefined();
  });

  it('应该支持预写入文件', async () => {
    const wasmBytes = new Uint8Array([
      0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
    ]);
    const result = await runtime.execute(wasmBytes, {
      prewriteFiles: { '/input.txt': 'test data' },
    });
    // 会因为模块不完整而失败，但 fs 应该有预写入文件
    expect(result.fs.exists('/input.txt')).toBe(true);
    expect(result.fs.readTextFile('/input.txt')).toBe('test data');
  });

  it('应该处理超时配置', async () => {
    const result = await runtime.execute(new Uint8Array([0x00]), {
      timeout: 100,
    });
    expect(result.success).toBe(false);
  });
});

// ===== CodeSandboxV2 测试 =====

describe('CodeSandboxV2', () => {
  let sandbox: CodeSandboxV2;

  beforeEach(() => {
    sandbox = new CodeSandboxV2({
      timeout: 3000,
      memoryLimit: 32 * 1024 * 1024,
    });
  });

  afterEach(() => {
    sandbox.terminate();
  });

  describe('JS 执行', () => {
    it('应该执行简单的 JS 代码', async () => {
      const result = await sandbox.run({
        code: 'return 1 + 2',
        language: 'js',
      });
      expect(result.success).toBe(true);
      expect(result.result).toBe(3);
      expect(result.language).toBe('js');
    });

    it('应该捕获 console 输出', async () => {
      const result = await sandbox.run({
        code: 'console.log("hello"); return 42;',
        language: 'js',
      });
      expect(result.success).toBe(true);
      expect(result.stdout).toContain('hello');
      expect(result.result).toBe(42);
    });

    it('应该拒绝恶意代码（require）', async () => {
      const result = await sandbox.run({
        code: 'require("fs")',
        language: 'js',
      });
      expect(result.success).toBe(false);
      expect(result.errorType).toBe('security');
    });

    it('应该拒绝 eval', async () => {
      const result = await sandbox.run({
        code: 'eval("1+1")',
        language: 'js',
      });
      expect(result.success).toBe(false);
      expect(result.errorType).toBe('security');
    });

    it('应该处理语法错误', async () => {
      const result = await sandbox.run({
        code: 'this is not valid javascript {{{',
        language: 'js',
      });
      expect(result.success).toBe(false);
    });

    it('应该支持上下文注入', async () => {
      const result = await sandbox.run({
        code: 'return x * y',
        language: 'js',
        context: { x: 6, y: 7 },
      });
      expect(result.success).toBe(true);
      expect(result.result).toBe(42);
    });
  });

  describe('WASM 执行', () => {
    it('应该拒绝无效 WASM 并返回结构化结果', async () => {
      const result = await sandbox.run({
        code: new Uint8Array([0xFF, 0xFF]),
        language: 'wasm',
      });
      expect(result.success).toBe(false);
      expect(result.language).toBe('wasm');
      expect(result.error).toBeDefined();
      expect(result.fs).toBeDefined();
    });

    it('应该支持预写入文件到虚拟 FS', async () => {
      const result = await sandbox.run({
        code: new Uint8Array([0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00]),
        language: 'wasm',
        prewriteFiles: { '/data.csv': 'a,b,c\n1,2,3' },
      });
      // 即使 WASM 执行失败，预写入文件应在 FS 中
      expect(result.fs).toBeDefined();
      expect(result.fs!.exists('/data.csv')).toBe(true);
    });
  });

  describe('runWasm 直接调用', () => {
    it('应该接受 BufferSource 参数', async () => {
      const result = await sandbox.runWasm({
        code: new Uint8Array([0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00]),
        args: ['--test'],
      });
      expect(result.language).toBe('wasm');
      expect(result.fs).toBeDefined();
    });
  });

  describe('不支持的', () => {
    it('应该返回错误', async () => {
      const result = await sandbox.run({
        code: 'test',
        language: 'cobol' as never,
      });
      expect(result.success).toBe(false);
      expect(result.error).toContain('Unsupported language');
    });
  });

  describe('terminate', () => {
    it('应该可以安全调用多次', () => {
      expect(() => {
        sandbox.terminate();
        sandbox.terminate();
      }).not.toThrow();
    });
  });

  describe('Python 支持', () => {
    it('isPythonSupported 初始应为 false', () => {
      expect(sandbox.isPythonSupported()).toBe(false);
    });
  });
});

// ===== 集成测试 =====

describe('CodeSandboxV2 集成', () => {
  it('VirtualFS + WasmRuntime 协作', async () => {
    const fs = new VirtualFS();
    fs.writeFile('/shared.txt', 'shared content');

    const runtime = new WasmRuntime();
    const result = await runtime.execute(
      new Uint8Array([0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00]),
      { fs },
    );

    // 共享的 FS 应保留预写入文件
    expect(result.fs.exists('/shared.txt')).toBe(true);
    expect(result.fs.readTextFile('/shared.txt')).toBe('shared content');
  });

  it('多次执行应使用独立的 VirtualFS', async () => {
    const sandbox = new CodeSandboxV2();

    const r1 = await sandbox.run({
      code: new Uint8Array([0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00]),
      language: 'wasm',
      prewriteFiles: { '/run1.txt': 'first' },
    });

    const r2 = await sandbox.run({
      code: new Uint8Array([0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00]),
      language: 'wasm',
      prewriteFiles: { '/run2.txt': 'second' },
    });

    // 每次执行应有独立的 FS
    expect(r1.fs!.exists('/run1.txt')).toBe(true);
    expect(r1.fs!.exists('/run2.txt')).toBe(false);
    expect(r2.fs!.exists('/run2.txt')).toBe(true);
    expect(r2.fs!.exists('/run1.txt')).toBe(false);

    sandbox.terminate();
  });
});
