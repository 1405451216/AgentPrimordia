import { describe, it, expect, beforeEach } from 'vitest';
import { WasiShim, VirtualFS } from '../sandbox-v2.js';

/** 创建一块 WASM 线性内存用于测试 */
function createMemory(pages = 1): WebAssembly.Memory {
  return new WebAssembly.Memory({ initial: pages });
}

/** 向内存写入字符串（无 null 终止） */
function writeString(mem: WebAssembly.Memory, offset: number, str: string): void {
  const buf = new TextEncoder().encode(str);
  new Uint8Array(mem.buffer).set(buf, offset);
}

/** 从内存读取 null 终止字符串 */
function readCString(mem: WebAssembly.Memory, offset: number): string {
  const view = new Uint8Array(mem.buffer);
  let end = offset;
  while (view[end] !== 0 && end < view.length) end++;
  return new TextDecoder().decode(view.slice(offset, end));
}

describe('WasiShim — 完整测试', () => {
  let memory: WebAssembly.Memory;
  let wasi: WasiShim;

  beforeEach(() => {
    memory = createMemory();
    wasi = new WasiShim({
      args: ['prog', '--verbose', 'file.txt'],
      env: { HOME: '/home/user', LANG: 'en_US.UTF-8' },
    });
    wasi.bindMemory(memory);
  });

  // ===== buildImports =====
  describe('buildImports 结构', () => {
    it('应包含 wasi_snapshot_preview1 命名空间', () => {
      const imports = wasi.buildImports();
      expect(imports['wasi_snapshot_preview1']).toBeDefined();
    });

    it('应包含所有核心 WASI 函数', () => {
      const fns = wasi.buildImports()['wasi_snapshot_preview1'];
      const expected = [
        'fd_write', 'fd_read', 'fd_close', 'fd_seek',
        'proc_exit', 'environ_get', 'environ_sizes_get',
        'args_get', 'args_sizes_get', 'clock_time_get',
        'random_get', 'clock_res_get', 'fd_fdstat_get',
        'fd_fdstat_set_flags', 'fd_prestat_get', 'fd_prestat_dir_name',
        'path_open',
      ];
      for (const name of expected) {
        expect(fns[name], `missing ${name}`).toBeDefined();
      }
    });
  });

  // ===== 初始状态 =====
  describe('初始状态', () => {
    it('stdout/stderr 初始为空', () => {
      expect(wasi.getStdout()).toBe('');
      expect(wasi.getStderr()).toBe('');
    });

    it('exitCode 初始为 null', () => {
      expect(wasi.getExitCode()).toBeNull();
    });
  });

  // ===== fd_write =====
  describe('fd_write — 写入文件描述符', () => {
    it('写入 stdout (fd=1) 应捕获到 getStdout', () => {
      const imports = wasi.buildImports();
      const fdWrite = imports['wasi_snapshot_preview1']['fd_write'] as Function;

      const text = 'hello stdout';
      // 在内存 offset=100 写入文本
      writeString(memory, 100, text);
      // iovec at offset=0: buf=100, bufLen=text.length
      const view = new DataView(memory.buffer);
      view.setUint32(0, 100, true);   // iov.buf
      view.setUint32(4, text.length, true); // iov.bufLen
      // nwritten pointer at offset=16

      const errno = fdWrite(1, 0, 1, 16);
      expect(errno).toBe(0); // SUCCESS
      expect(view.getUint32(16, true)).toBe(text.length);
      expect(wasi.getStdout()).toBe(text);
    });

    it('写入 stderr (fd=2) 应捕获到 getStderr', () => {
      const imports = wasi.buildImports();
      const fdWrite = imports['wasi_snapshot_preview1']['fd_write'] as Function;

      writeString(memory, 200, 'error msg');
      const view = new DataView(memory.buffer);
      view.setUint32(0, 200, true);
      view.setUint32(4, 9, true);

      const errno = fdWrite(2, 0, 1, 16);
      expect(errno).toBe(0);
      expect(wasi.getStderr()).toBe('error msg');
    });

    it('写入 stdin (fd=0) 应返回 EBADF', () => {
      const imports = wasi.buildImports();
      const fdWrite = imports['wasi_snapshot_preview1']['fd_write'] as Function;

      const view = new DataView(memory.buffer);
      view.setUint32(0, 100, true);
      view.setUint32(4, 5, true);

      const errno = fdWrite(0, 0, 1, 16);
      expect(errno).toBe(8); // EBADF
    });

    it('多个 iovec 应累加写入', () => {
      const imports = wasi.buildImports();
      const fdWrite = imports['wasi_snapshot_preview1']['fd_write'] as Function;

      // 字符串放在 offset=500 和 offset=600，避免与其他内存区域重叠
      writeString(memory, 500, 'part1');
      writeString(memory, 600, 'part2');
      const view = new DataView(memory.buffer);
      // iovec 0 at offset=0: buf=500, len=5
      view.setUint32(0, 500, true);
      view.setUint32(4, 5, true);
      // iovec 1 at offset=8: buf=600, len=5
      view.setUint32(8, 600, true);
      view.setUint32(12, 5, true);

      const errno = fdWrite(1, 0, 2, 1000);
      expect(errno).toBe(0); // SUCCESS
      // 验证 stdout 包含了两个 iovec 的内容
      expect(wasi.getStdout()).toContain('part1');
      expect(wasi.getStdout()).toContain('part2');
    });
  });

  // ===== fd_read =====
  describe('fd_read — 读取文件描述符', () => {
    it('读取 stdin (fd=0) 应返回 EOF（0字节）', () => {
      const imports = wasi.buildImports();
      const fdRead = imports['wasi_snapshot_preview1']['fd_read'] as Function;

      const view = new DataView(memory.buffer);
      view.setUint32(0, 100, true);  // iov.buf
      view.setUint32(4, 64, true);   // iov.bufLen

      const errno = fdRead(0, 0, 1, 32);
      expect(errno).toBe(0);
      expect(view.getUint32(32, true)).toBe(0); // nread = 0 (EOF)
    });

    it('读取 stdout (fd=1) 应返回 EBADF', () => {
      const imports = wasi.buildImports();
      const fdRead = imports['wasi_snapshot_preview1']['fd_read'] as Function;

      const view = new DataView(memory.buffer);
      view.setUint32(0, 100, true);
      view.setUint32(4, 64, true);

      const errno = fdRead(1, 0, 1, 32);
      expect(errno).toBe(8); // EBADF
    });
  });

  // ===== environ_sizes_get / environ_get =====
  describe('environ_get — 环境变量', () => {
    it('environ_sizes_get 应返回正确的 count 和 bufSize', () => {
      const imports = wasi.buildImports();
      const sizesGet = imports['wasi_snapshot_preview1']['environ_sizes_get'] as Function;

      const view = new DataView(memory.buffer);
      const errno = sizesGet(0, 4);
      expect(errno).toBe(0);
      expect(view.getUint32(0, true)).toBe(2); // HOME, LANG

      // HOME=/home/user => "HOME=/home/user\0" = 4+1+9+1 = 15
      // LANG=en_US.UTF-8 => "LANG=en_US.UTF-8\0" = 4+1+10+1 = 16
      // total bufSize = 31
      const expectedBufSize = 'HOME=/home/user'.length + 1 + 'LANG=en_US.UTF-8'.length + 1;
      expect(view.getUint32(4, true)).toBe(expectedBufSize);
    });

    it('environ_get 应将环境变量写入内存', () => {
      const imports = wasi.buildImports();
      const envGet = imports['wasi_snapshot_preview1']['environ_get'] as Function;

      // 指针数组从 offset=0 开始，字符串缓冲区从 offset=100 开始
      const errno = envGet(0, 100);
      expect(errno).toBe(0);

      const view = new DataView(memory.buffer);
      const ptr0 = view.getUint32(0, true);
      const ptr1 = view.getUint32(4, true);

      expect(readCString(memory, ptr0)).toBe('HOME=/home/user');
      expect(readCString(memory, ptr1)).toBe('LANG=en_US.UTF-8');
    });
  });

  // ===== args_sizes_get / args_get =====
  describe('args_get — 命令行参数', () => {
    it('args_sizes_get 应返回正确的 count 和 bufSize', () => {
      const imports = wasi.buildImports();
      const sizesGet = imports['wasi_snapshot_preview1']['args_sizes_get'] as Function;

      const view = new DataView(memory.buffer);
      const errno = sizesGet(0, 4);
      expect(errno).toBe(0);
      expect(view.getUint32(0, true)).toBe(3); // prog, --verbose, file.txt

      // prog\0=5, --verbose\0=10, file.txt\0=9 => total=24
      expect(view.getUint32(4, true)).toBe(24);
    });

    it('args_get 应将参数写入内存', () => {
      const imports = wasi.buildImports();
      const argsGet = imports['wasi_snapshot_preview1']['args_get'] as Function;

      const errno = argsGet(0, 100);
      expect(errno).toBe(0);

      const view = new DataView(memory.buffer);
      const ptr0 = view.getUint32(0, true);
      const ptr1 = view.getUint32(4, true);
      const ptr2 = view.getUint32(8, true);

      expect(readCString(memory, ptr0)).toBe('prog');
      expect(readCString(memory, ptr1)).toBe('--verbose');
      expect(readCString(memory, ptr2)).toBe('file.txt');
    });
  });

  // ===== clock_time_get =====
  describe('clock_time_get — 时间获取', () => {
    it('REALTIME 应返回当前时间（纳秒）', () => {
      const imports = wasi.buildImports();
      const clockGet = imports['wasi_snapshot_preview1']['clock_time_get'] as Function;

      const before = BigInt(Date.now()) * 1_000_000n;
      const view = new DataView(memory.buffer);
      const errno = clockGet(0, 0n, 0); // REALTIME=0
      expect(errno).toBe(0);

      const time = view.getBigUint64(0, true);
      const after = BigInt(Date.now() + 1) * 1_000_000n;
      expect(time).toBeGreaterThanOrEqual(before);
      expect(time).toBeLessThanOrEqual(after);
    });

    it('MONOTONIC 应返回经过时间', () => {
      const imports = wasi.buildImports();
      const clockGet = imports['wasi_snapshot_preview1']['clock_time_get'] as Function;

      const view = new DataView(memory.buffer);
      const errno = clockGet(1, 0n, 0); // MONOTONIC=1
      expect(errno).toBe(0);

      const time = view.getBigUint64(0, true);
      // 应 >= 0（刚创建）
      expect(time).toBeGreaterThanOrEqual(0n);
    });
  });

  // ===== proc_exit =====
  describe('proc_exit — 退出处理', () => {
    it('应抛出 WasiExitError 并设置 exitCode', () => {
      const imports = wasi.buildImports();
      const procExit = imports['wasi_snapshot_preview1']['proc_exit'] as Function;

      expect(() => procExit(42)).toThrow();
      expect(wasi.getExitCode()).toBe(42);
    });

    it('exitCode 0 也应被记录', () => {
      const imports = wasi.buildImports();
      const procExit = imports['wasi_snapshot_preview1']['proc_exit'] as Function;

      expect(() => procExit(0)).toThrow();
      expect(wasi.getExitCode()).toBe(0);
    });
  });

  // ===== 未绑定内存 =====
  describe('内存未绑定', () => {
    it('未 bindMemory 时调用应抛出 Memory not bound', () => {
      const unbound = new WasiShim();
      const imports = unbound.buildImports();
      const fdWrite = imports['wasi_snapshot_preview1']['fd_write'] as Function;

      expect(() => fdWrite(1, 0, 1, 0)).toThrow('Memory not bound');
    });
  });

  // ===== random_get =====
  describe('random_get', () => {
    it('应填充随机字节', () => {
      const imports = wasi.buildImports();
      const randomGet = imports['wasi_snapshot_preview1']['random_get'] as Function;

      // 先清零
      new Uint8Array(memory.buffer).fill(0, 0, 32);
      const errno = randomGet(0, 32);
      expect(errno).toBe(0);

      // 检查至少有一些非零字节（概率极高）
      const bytes = new Uint8Array(memory.buffer, 0, 32);
      const hasNonZero = bytes.some((b) => b !== 0);
      expect(hasNonZero).toBe(true);
    });
  });
});
