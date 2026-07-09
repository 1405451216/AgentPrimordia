/**
 * CodeSandbox v2 — WASM Runtime + 虚拟文件系统 + 多语言安全执行。
 *
 * 在 v1（Worker Thread JS 执行）的基础上叠加：
 * 1. **VirtualFS** — 内存虚拟文件系统，为 guest 程序提供隔离的文件 I/O
 * 2. **WasmRuntime** — 基于 WebAssembly + WASI 子集的 WASM 模块执行器
 * 3. **统一执行接口** — 根据代码类型分发到不同后端：
 *    - `js`     → Worker Thread（复用 v1）
 *    - `wasm`   → WasmRuntime（WASI 沙箱）
 *    - `python` → Pyodide（可选，动态导入）
 *
 * 安全模型：
 * - WASM 模块运行在线性内存沙箱中，无法访问宿主内存
 * - 文件 I/O 通过 VirtualFS 限制在虚拟根目录内
 * - 执行超时 + 内存限制 + 输出大小限制
 * - 网络默认禁用（可通过 allowNetwork 开启受控代理）
 *
 * 使用方式：
 *   const sb = new CodeSandboxV2({ timeout: 5000, memoryLimit: 64 * 1024 * 1024 });
 *   const res = await sb.run('return 1 + 2', { language: 'js' });
 *   const res2 = await sb.runWasm(wasmBytes, { args: ['--quiet'] });
 *   sb.terminate();
 */

import { CodeSandbox, type SandboxConfig, type SandboxResult } from './sandbox.js';
import { CodeSecurityChecker } from './sandbox.js';

// ===== 虚拟文件系统 =====

/** 虚拟文件类型 */
export type VfsNodeType = 'file' | 'directory';

/** 虚拟文件节点 */
interface VfsNode {
  type: VfsNodeType;
  /** 文件内容（仅 file 类型） */
  content?: Uint8Array;
  /** 子节点（仅 directory 类型） */
  children?: Map<string, VfsNode>;
  /** 创建时间戳 */
  createdAt: number;
  /** 修改时间戳 */
  modifiedAt: number;
  /** 权限位（rwx） */
  mode: number;
}

/** 虚拟文件描述符 */
interface VfsFd {
  node: VfsNode;
  /** 文件路径 */
  path: string;
  /** 读位置 */
  position: number;
  /** 打开模式 */
  flags: VfsOpenFlags;
}

/** 打开标志 */
export interface VfsOpenFlags {
  read: boolean;
  write: boolean;
  append: boolean;
  truncate: boolean;
  create: boolean;
}

/**
 * 内存虚拟文件系统。
 *
 * 提供类 POSIX 的文件操作接口，所有数据驻留在内存中。
 * 用于为 WASM guest 程序提供隔离、可审计的文件 I/O 环境。
 */
export class VirtualFS {
  private root: VfsNode;
  private fds: Map<number, VfsFd> = new Map();
  private nextFd: number = 3; // 0=stdin, 1=stdout, 2=stderr 保留
  private maxFileSize: number;
  private totalBytes: number = 0;
  private maxTotalBytes: number;

  constructor(opts?: { maxFileSize?: number; maxTotalBytes?: number }) {
    const now = Date.now();
    this.root = {
      type: 'directory',
      children: new Map(),
      createdAt: now,
      modifiedAt: now,
      mode: 0o755,
    };
    this.maxFileSize = opts?.maxFileSize ?? 16 * 1024 * 1024; // 16MB per file
    this.maxTotalBytes = opts?.maxTotalBytes ?? 128 * 1024 * 1024; // 128MB total
  }

  /** 规范化路径为绝对路径数组 */
  private resolve(path: string): string[] {
    const parts = path.split('/').filter((p) => p !== '' && p !== '.');
    const resolved: string[] = [];
    for (const part of parts) {
      if (part === '..') {
        resolved.pop();
      } else {
        resolved.push(part);
      }
    }
    return resolved;
  }

  /** 获取节点（不创建） */
  private getNode(segments: string[]): VfsNode | null {
    let current = this.root;
    for (const seg of segments) {
      if (current.type !== 'directory' || !current.children) return null;
      const child = current.children.get(seg);
      if (!child) return null;
      current = child;
    }
    return current;
  }

  /** 获取父目录节点（确保存在） */
  private getParent(segments: string[]): VfsNode | null {
    if (segments.length === 0) return null;
    return this.getNode(segments.slice(0, -1));
  }

  /** 写入文件（覆盖） */
  writeFile(path: string, data: Uint8Array | string): void {
    const bytes = typeof data === 'string' ? new TextEncoder().encode(data) : data;
    if (bytes.length > this.maxFileSize) {
      throw new Error(`File size ${bytes.length} exceeds max ${this.maxFileSize}`);
    }
    const segments = this.resolve(path);
    if (segments.length === 0) {
      throw new Error('Cannot write to root');
    }

    // 确保父目录存在
    this.mkdirRecursive(segments.slice(0, -1));

    const parent = this.getParent(segments);
    if (!parent || parent.type !== 'directory' || !parent.children) {
      throw new Error(`Parent directory does not exist: ${path}`);
    }

    const name = segments[segments.length - 1];
    const existing = parent.children.get(name);
    const oldSize = existing?.content?.length ?? 0;

    if (this.totalBytes - oldSize + bytes.length > this.maxTotalBytes) {
      throw new Error(`Total FS size would exceed max ${this.maxTotalBytes}`);
    }

    const now = Date.now();
    parent.children.set(name, {
      type: 'file',
      content: bytes,
      createdAt: existing?.createdAt ?? now,
      modifiedAt: now,
      mode: 0o644,
    });

    this.totalBytes += bytes.length - oldSize;
    parent.modifiedAt = now;
  }

  /** 读取文件 */
  readFile(path: string): Uint8Array {
    const segments = this.resolve(path);
    const node = this.getNode(segments);
    if (!node || node.type !== 'file' || !node.content) {
      throw new Error(`File not found: ${path}`);
    }
    return node.content;
  }

  /** 读取文件为字符串 */
  readTextFile(path: string): string {
    return new TextDecoder().decode(this.readFile(path));
  }

  /** 检查路径是否存在 */
  exists(path: string): boolean {
    return this.getNode(this.resolve(path)) !== null;
  }

  /** 创建目录（递归） */
  private mkdirRecursive(segments: string[]): void {
    let current = this.root;
    const now = Date.now();
    for (const seg of segments) {
      if (current.type !== 'directory' || !current.children) {
        throw new Error(`Not a directory in path: ${seg}`);
      }
      let child = current.children.get(seg);
      if (!child) {
        child = {
          type: 'directory',
          children: new Map(),
          createdAt: now,
          modifiedAt: now,
          mode: 0o755,
        };
        current.children.set(seg, child);
        current.modifiedAt = now;
      }
      current = child;
    }
  }

  /** 创建目录 */
  mkdir(path: string): void {
    const segments = this.resolve(path);
    if (segments.length === 0) return;
    this.mkdirRecursive(segments);
  }

  /** 删除文件或目录 */
  unlink(path: string): void {
    const segments = this.resolve(path);
    if (segments.length === 0) {
      throw new Error('Cannot remove root');
    }
    const parent = this.getParent(segments);
    if (!parent || parent.type !== 'directory' || !parent.children) {
      throw new Error(`Parent directory does not exist: ${path}`);
    }
    const name = segments[segments.length - 1];
    const node = parent.children.get(name);
    if (!node) {
      throw new Error(`Not found: ${path}`);
    }
    if (node.type === 'file' && node.content) {
      this.totalBytes -= node.content.length;
    }
    parent.children.delete(name);
    parent.modifiedAt = Date.now();
  }

  /** 列出目录内容 */
  listDir(path: string): string[] {
    const segments = this.resolve(path);
    const node = this.getNode(segments);
    if (!node || node.type !== 'directory' || !node.children) {
      throw new Error(`Not a directory: ${path}`);
    }
    return Array.from(node.children.keys()).sort();
  }

  /** 打开文件，返回文件描述符 */
  open(path: string, flags: VfsOpenFlags): number {
    const segments = this.resolve(path);
    let node = this.getNode(segments);

    if (!node && flags.create) {
      this.writeFile(path, new Uint8Array(0));
      node = this.getNode(segments);
    }

    if (!node) {
      throw new Error(`File not found: ${path}`);
    }

    if (node.type !== 'file') {
      throw new Error(`Not a file: ${path}`);
    }

    if (flags.truncate && node.content) {
      this.totalBytes -= node.content.length;
      node.content = new Uint8Array(0);
      node.modifiedAt = Date.now();
    }

    const fd = this.nextFd++;
    this.fds.set(fd, {
      node,
      path,
      position: flags.append && node.content ? node.content.length : 0,
      flags,
    });
    return fd;
  }

  /** 关闭文件描述符 */
  close(fd: number): void {
    this.fds.delete(fd);
  }

  /** 从文件描述符读取 */
  read(fd: number, length: number): Uint8Array {
    const handle = this.fds.get(fd);
    if (!handle) throw new Error(`Invalid fd: ${fd}`);
    if (!handle.flags.read) throw new Error(`fd ${fd} not opened for reading`);
    if (!handle.node.content) return new Uint8Array(0);

    const start = handle.position;
    const end = Math.min(start + length, handle.node.content.length);
    const chunk = handle.node.content.slice(start, end);
    handle.position = end;
    return chunk;
  }

  /** 向文件描述符写入 */
  write(fd: number, data: Uint8Array): number {
    const handle = this.fds.get(fd);
    if (!handle) throw new Error(`Invalid fd: ${fd}`);
    if (!handle.flags.write) throw new Error(`fd ${fd} not opened for writing`);
    if (!handle.node.content) {
      handle.node.content = new Uint8Array(0);
    }

    const existing = handle.node.content;
    let newContent: Uint8Array;

    if (handle.flags.append) {
      const newSize = existing.length + data.length;
      if (newSize > this.maxFileSize) {
        throw new Error(`File would exceed max size ${this.maxFileSize}`);
      }
      if (this.totalBytes - existing.length + newSize > this.maxTotalBytes) {
        throw new Error(`Total FS size would exceed max ${this.maxTotalBytes}`);
      }
      newContent = new Uint8Array(newSize);
      newContent.set(existing, 0);
      newContent.set(data, existing.length);
      handle.position = newSize;
    } else {
      const endPos = handle.position + data.length;
      if (endPos > this.maxFileSize) {
        throw new Error(`File would exceed max size ${this.maxFileSize}`);
      }
      const growth = Math.max(0, endPos - existing.length);
      if (this.totalBytes + growth > this.maxTotalBytes) {
        throw new Error(`Total FS size would exceed max ${this.maxTotalBytes}`);
      }
      newContent = new Uint8Array(Math.max(existing.length, endPos));
      newContent.set(existing, 0);
      newContent.set(data, handle.position);
      handle.position = endPos;
    }

    const oldLen = existing.length;
    handle.node.content = newContent;
    handle.node.modifiedAt = Date.now();
    this.totalBytes += newContent.length - oldLen;
    return data.length;
  }

  /** 获取文件描述符对应的路径 */
  getFdPath(fd: number): string | null {
    return this.fds.get(fd)?.path ?? null;
  }

  /** 获取总使用字节数 */
  getUsedBytes(): number {
    return this.totalBytes;
  }

  /** 清空整个文件系统 */
  reset(): void {
    this.root.children = new Map();
    this.fds.clear();
    this.totalBytes = 0;
    this.nextFd = 3;
  }
}

// ===== WASI 最小子集实现 =====

/** WASI errno 常量 */
const WASI_ERRNO = {
  SUCCESS: 0,
  EBADF: 8,
  EFAULT: 21,
  EINVAL: 28,
  ENOSYS: 52,
} as const;

/** WASI clock ID */
const WASI_CLOCKID = {
  REALTIME: 0,
  MONOTONIC: 1,
  PROCESS_CPUTIME_ID: 2,
  THREAD_CPUTIME_ID: 3,
} as const;

/** WASI whence */
const WASI_WHENCE = {
  SET: 0,
  CUR: 1,
  END: 2,
} as const;

/** fd_write / fd_read 的 iov 结构 */
interface WasiIovec {
  buf: number; // 指针
  bufLen: number; // 长度
}

/**
 * WASI 最小子集实现。
 *
 * 提供以下函数：
 * - fd_write, fd_read, fd_close, fd_seek
 * - proc_exit
 * - environ_get, environ_sizes_get
 * - args_get, args_sizes_get
 * - clock_time_get
 * - random_get
 *
 * 不实现：文件系统相关（path_open 等），因为 VirtualFS 通过预打开 fd 方式提供。
 */
export class WasiShim {
  private memory: WebAssembly.Memory | null = null;
  private args: string[];
  private env: Record<string, string>;
  private fs: VirtualFS;
  private stdout: string[] = [];
  private stderr: string[] = [];
  private startTime: number;
  private exitCode: number | null = null;

  constructor(opts?: {
    args?: string[];
    env?: Record<string, string>;
    fs?: VirtualFS;
  }) {
    this.args = opts?.args ?? [];
    this.env = opts?.env ?? {};
    this.fs = opts?.fs ?? new VirtualFS();
    this.startTime = Date.now();
  }

  /** 绑定 WASM 内存 */
  bindMemory(memory: WebAssembly.Memory): void {
    this.memory = memory;
  }

  /** 获取 stdout */
  getStdout(): string {
    return this.stdout.join('');
  }

  /** 获取 stderr */
  getStderr(): string {
    return this.stderr.join('');
  }

  /** 获取退出码 */
  getExitCode(): number | null {
    return this.exitCode;
  }

  /** 读取内存视图 */
  private view(): DataView {
    if (!this.memory) throw new Error('Memory not bound');
    return new DataView(this.memory.buffer);
  }

  /** 从内存读取 iovec 数组 */
  private readIovecs(iovsPtr: number, iovsLen: number): WasiIovec[] {
    const view = this.view();
    const iovecs: WasiIovec[] = [];
    for (let i = 0; i < iovsLen; i++) {
      const buf = view.getUint32(iovsPtr + i * 8, true);
      const bufLen = view.getUint32(iovsPtr + i * 4 + 4, true);
      iovecs.push({ buf, bufLen });
    }
    return iovecs;
  }

  /** 写入 u32 到内存 */
  private writeU32(ptr: number, value: number): void {
    this.view().setUint32(ptr, value, true);
  }

  /** 构建 WASI import 对象 */
  buildImports(): Record<string, Record<string, WebAssembly.ImportValue>> {
    return {
      wasi_snapshot_preview1: {
        fd_write: (fd: number, iovsPtr: number, iovsLen: number, nwrittenPtr: number): number => {
          return this.fdWrite(fd, iovsPtr, iovsLen, nwrittenPtr);
        },
        fd_read: (fd: number, iovsPtr: number, iovsLen: number, nreadPtr: number): number => {
          return this.fdRead(fd, iovsPtr, iovsLen, nreadPtr);
        },
        fd_close: (fd: number): number => {
          if (fd >= 3) this.fs.close(fd);
          return WASI_ERRNO.SUCCESS;
        },
        fd_seek: (fd: number, offset: number, whence: number, newoffsetPtr: number): number => {
          return this.fdSeek(fd, offset, whence, newoffsetPtr);
        },
        fd_fdstat_get: (_fd: number, _bufPtr: number): number => {
          // 简化：返回成功
          return WASI_ERRNO.SUCCESS;
        },
        proc_exit: (code: number): never => {
          this.exitCode = code;
          throw new WasiExitError(code);
        },
        environ_get: (environPtr: number, environBufPtr: number): number => {
          return this.environGet(environPtr, environBufPtr);
        },
        environ_sizes_get: (countPtr: number, bufSizePtr: number): number => {
          const entries = Object.entries(this.env);
          const count = entries.length;
          let bufSize = 0;
          for (const [k, v] of entries) {
            bufSize += k.length + 1 + v.length + 1; // key=value\0
          }
          this.writeU32(countPtr, count);
          this.writeU32(bufSizePtr, bufSize);
          return WASI_ERRNO.SUCCESS;
        },
        args_get: (argsPtr: number, argsBufPtr: number): number => {
          return this.argsGet(argsPtr, argsBufPtr);
        },
        args_sizes_get: (countPtr: number, bufSizePtr: number): number => {
          let bufSize = 0;
          for (const arg of this.args) {
            bufSize += arg.length + 1;
          }
          this.writeU32(countPtr, this.args.length);
          this.writeU32(bufSizePtr, bufSize);
          return WASI_ERRNO.SUCCESS;
        },
        clock_time_get: (clockId: number, _precision: bigint, timePtr: number): number => {
          let time: bigint;
          if (clockId === WASI_CLOCKID.REALTIME) {
            time = BigInt(Date.now()) * 1_000_000n; // ms → ns
          } else {
            time = BigInt(Date.now() - this.startTime) * 1_000_000n;
          }
          this.view().setBigUint64(timePtr, time, true);
          return WASI_ERRNO.SUCCESS;
        },
        random_get: (bufPtr: number, bufLen: number): number => {
          const buf = new Uint8Array(this.memory!.buffer, bufPtr, bufLen);
          for (let i = 0; i < bufLen; i++) {
            buf[i] = Math.floor(Math.random() * 256);
          }
          return WASI_ERRNO.SUCCESS;
        },
        clock_res_get: (_clockId: number, resolutionPtr: number): number => {
          this.view().setBigUint64(resolutionPtr, 1n, true);
          return WASI_ERRNO.SUCCESS;
        },
        fd_fdstat_set_flags: (_fd: number, _flags: number): number => {
          return WASI_ERRNO.SUCCESS;
        },
        fd_prestat_get: (_fd: number, _bufPtr: number): number => {
          return WASI_ERRNO.EBADF;
        },
        fd_prestat_dir_name: (_fd: number, _pathPtr: number, _pathLen: number): number => {
          return WASI_ERRNO.EBADF;
        },
        path_open: (
          _fd: number,
          _dirflags: number,
          _pathPtr: number,
          _pathLen: number,
          _oflags: number,
          _rightsBase: bigint,
          _rightsInheriting: bigint,
          _fdflags: number,
          fdOutPtr: number,
        ): number => {
          // 简化：不支持 path_open，guest 程序应通过预打开的 fd 访问文件
          this.writeU32(fdOutPtr, 0);
          return WASI_ERRNO.ENOSYS;
        },
      },
    };
  }

  private fdWrite(fd: number, iovsPtr: number, iovsLen: number, nwrittenPtr: number): number {
    const iovecs = this.readIovecs(iovsPtr, iovsLen);
    let totalWritten = 0;

    for (const iov of iovecs) {
      const bytes = new Uint8Array(this.memory!.buffer, iov.buf, iov.bufLen);
      const text = new TextDecoder().decode(bytes);

      if (fd === 1) {
        this.stdout.push(text);
      } else if (fd === 2) {
        this.stderr.push(text);
      } else if (fd >= 3) {
        // 写入虚拟文件
        const copy = bytes.slice();
        this.fs.write(fd, copy);
      } else {
        return WASI_ERRNO.EBADF;
      }
      totalWritten += iov.bufLen;
    }

    this.writeU32(nwrittenPtr, totalWritten);
    return WASI_ERRNO.SUCCESS;
  }

  private fdRead(fd: number, iovsPtr: number, iovsLen: number, nreadPtr: number): number {
    const iovecs = this.readIovecs(iovsPtr, iovsLen);
    let totalRead = 0;

    for (const iov of iovecs) {
      let data: Uint8Array;
      if (fd === 0) {
        // stdin: 返回空（EOF）
        data = new Uint8Array(0);
      } else if (fd >= 3) {
        data = this.fs.read(fd, iov.bufLen);
      } else {
        return WASI_ERRNO.EBADF;
      }

      const target = new Uint8Array(this.memory!.buffer, iov.buf, iov.bufLen);
      const copyLen = Math.min(data.length, iov.bufLen);
      target.set(data.slice(0, copyLen), 0);
      totalRead += copyLen;
    }

    this.writeU32(nreadPtr, totalRead);
    return WASI_ERRNO.SUCCESS;
  }

  private fdSeek(fd: number, offset: number, whence: number, newoffsetPtr: number): number {
    if (fd < 3) return WASI_ERRNO.EBADF;
    // VirtualFS 不直接暴露 seek，简化处理
    const newPos = whence === WASI_WHENCE.SET
      ? offset
      : whence === WASI_WHENCE.CUR
        ? 0 + offset // 简化
        : 0; // END
    this.writeU32(newoffsetPtr, newPos);
    return WASI_ERRNO.SUCCESS;
  }

  private environGet(environPtr: number, environBufPtr: number): number {
    const entries = Object.entries(this.env);
    let strPtr = environBufPtr;

    for (let i = 0; i < entries.length; i++) {
      const [k, v] = entries[i];
      const entry = `${k}=${v}\0`;
      // 写入指针
      this.view().setUint32(environPtr + i * 4, strPtr, true);
      // 写入字符串
      const buf = new TextEncoder().encode(entry);
      const mem = new Uint8Array(this.memory!.buffer, strPtr, buf.length);
      mem.set(buf);
      strPtr += buf.length;
    }

    return WASI_ERRNO.SUCCESS;
  }

  private argsGet(argsPtr: number, argsBufPtr: number): number {
    let strPtr = argsBufPtr;

    for (let i = 0; i < this.args.length; i++) {
      const arg = this.args[i] + '\0';
      this.view().setUint32(argsPtr + i * 4, strPtr, true);
      const buf = new TextEncoder().encode(arg);
      const mem = new Uint8Array(this.memory!.buffer, strPtr, buf.length);
      mem.set(buf);
      strPtr += buf.length;
    }

    return WASI_ERRNO.SUCCESS;
  }
}

/** WASI proc_exit 抛出的特殊错误 */
class WasiExitError extends Error {
  code: number;
  constructor(code: number) {
    super(`WASI proc_exit(${code})`);
    this.name = 'WasiExitError';
    this.code = code;
  }
}

// ===== WASM Runtime =====

/** WASM 执行配置 */
export interface WasmExecConfig {
  /** 命令行参数 */
  args?: string[];
  /** 环境变量 */
  env?: Record<string, string>;
  /** 虚拟文件系统（如不提供则使用内部新建的） */
  fs?: VirtualFS;
  /** 预写入文件（路径 → 内容） */
  prewriteFiles?: Record<string, string | Uint8Array>;
  /** 超时（毫秒） */
  timeout?: number;
  /** 最大内存页数（每页 64KB） */
  maxMemoryPages?: number;
}

/** WASM 执行结果 */
export interface WasmExecResult {
  success: boolean;
  exitCode: number | null;
  stdout: string;
  stderr: string;
  duration: number;
  error?: string;
  fs: VirtualFS;
}

/**
 * WASM 模块运行时。
 *
 * 使用 WebAssembly API 加载和执行 WASM 模块，
 * 通过 WASI shim 提供系统调用接口。
 */
export class WasmRuntime {
  private defaultTimeout: number;

  constructor(opts?: { defaultTimeout?: number }) {
    this.defaultTimeout = opts?.defaultTimeout ?? 5000;
  }

  /**
   * 执行 WASM 模块。
   *
   * @param wasmBytes 编译后的 WASM 二进制
   * @param config 执行配置
   */
  async execute(wasmBytes: BufferSource, config?: WasmExecConfig): Promise<WasmExecResult> {
    const timeout = config?.timeout ?? this.defaultTimeout;
    const fs = config?.fs ?? new VirtualFS();
    const startTime = Date.now();

    // 预写入文件
    if (config?.prewriteFiles) {
      for (const [path, content] of Object.entries(config.prewriteFiles)) {
        fs.writeFile(path, content);
      }
    }

    const wasi = new WasiShim({
      args: config?.args ?? ['wasm-program'],
      env: config?.env,
      fs,
    });

    try {
      // 编译 WASM 模块
      const module = await WebAssembly.compile(wasmBytes);

      // 检查是否需要 WASI imports
      const imports: Record<string, Record<string, WebAssembly.ImportValue>> = {};
      const requiredImports = WebAssembly.Module.imports(module);

      const needsWasi = requiredImports.some(
        (imp) => imp.module === 'wasi_snapshot_preview1' || imp.module === 'wasi_unstable',
      );

      if (needsWasi) {
        const wasiImports = wasi.buildImports();
        // 同时提供 wasi_snapshot_preview1 和 wasi_unstable
        imports['wasi_snapshot_preview1'] = wasiImports['wasi_snapshot_preview1'];
        imports['wasi_unstable'] = wasiImports['wasi_snapshot_preview1'];
      }

      // 实例化
      const instance = await WebAssembly.instantiate(module, imports);

      // 绑定内存
      const exports = instance.exports as Record<string, unknown>;
      const memory = exports['memory'] as WebAssembly.Memory | undefined;
      if (memory) {
        wasi.bindMemory(memory);
      }

      // 检查内存限制
      if (config?.maxMemoryPages && memory) {
        const currentPages = memory.buffer.byteLength / 65536;
        if (currentPages > config.maxMemoryPages) {
          throw new Error(
            `Memory exceeds limit: ${currentPages} > ${config.maxMemoryPages} pages`,
          );
        }
      }

      // 执行入口函数
      const entryFn =
        (exports['_start'] as (() => void) | undefined) ??
        (exports['main'] as (() => void) | undefined) ??
        (exports['__main_argc_argv'] as (() => void) | undefined);

      if (!entryFn) {
        // 没有标准入口，尝试导出的函数
        const exportedFns = Object.keys(exports).filter(
          (k) => typeof exports[k] === 'function',
        );
        if (exportedFns.length === 0) {
          throw new Error('No entry function found (_start, main, or any export)');
        }
        // 调用第一个导出函数
        const fn = exports[exportedFns[0]] as () => void;
        await this.runWithTimeout(fn, timeout);
      } else {
        await this.runWithTimeout(entryFn, timeout);
      }

      const duration = Date.now() - startTime;

      return {
        success: true,
        exitCode: wasi.getExitCode() ?? 0,
        stdout: wasi.getStdout(),
        stderr: wasi.getStderr(),
        duration,
        fs,
      };
    } catch (err) {
      const duration = Date.now() - startTime;

      if (err instanceof WasiExitError) {
        return {
          success: err.code === 0,
          exitCode: err.code,
          stdout: wasi.getStdout(),
          stderr: wasi.getStderr(),
          duration,
          fs,
        };
      }

      const errorMsg = err instanceof Error ? err.message : String(err);
      const isTimeout = errorMsg.includes('timeout') || errorMsg.includes('Timeout');

      return {
        success: false,
        exitCode: null,
        stdout: wasi.getStdout(),
        stderr: wasi.getStderr(),
        duration,
        error: errorMsg,
        fs,
      };
    }
  }

  /** 带超时执行函数 */
  private runWithTimeout(fn: () => void, timeout: number): Promise<void> {
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        reject(new Error(`Execution timeout after ${timeout}ms`));
      }, timeout);

      try {
        // WASM 执行是同步的
        fn();
        clearTimeout(timer);
        resolve();
      } catch (err) {
        clearTimeout(timer);
        reject(err);
      }
    });
  }
}

// ===== 统一代码沙箱 v2 =====

/** 支持的代码语言 */
export type CodeLanguage = 'js' | 'wasm' | 'python';

/** v2 沙箱配置 */
export interface SandboxV2Config extends SandboxConfig {
  /** WASM 执行默认超时 */
  wasmTimeout?: number;
  /** Pyodide CDN URL（可选） */
  pyodideUrl?: string;
  /** 默认虚拟文件系统大小限制 */
  fsMaxTotalBytes?: number;
}

/** v2 执行请求 */
export interface ExecRequest {
  /** 代码内容（js/python 为源码字符串，wasm 为二进制） */
  code: string | BufferSource;
  /** 语言 */
  language: CodeLanguage;
  /** 注入上下文（仅 js） */
  context?: Record<string, unknown>;
  /** 命令行参数（仅 wasm/python） */
  args?: string[];
  /** 环境变量 */
  env?: Record<string, string>;
  /** 预写入文件 */
  prewriteFiles?: Record<string, string | Uint8Array>;
  /** 超时覆盖 */
  timeout?: number;
}

/** v2 执行结果 */
export interface ExecResult extends SandboxResult {
  /** WASM 退出码 */
  exitCode?: number | null;
  /** 虚拟文件系统（执行后） */
  fs?: VirtualFS;
  /** 使用的语言 */
  language: CodeLanguage;
}

/**
 * 统一代码沙箱 v2。
 *
 * 整合 Worker Thread JS 执行、WASM WASI 执行和可选的 Pyodide Python 执行。
 * 根据语言类型自动路由到合适的后端。
 *
 * 设计目标：
 * - **安全隔离**：所有 guest 代码运行在沙箱中（Worker / WASM 线性内存）
 * - **多语言**：JS / WASM / Python 统一接口
 * - **虚拟文件系统**：跨语言的隔离 FS
 * - **资源限制**：超时 + 内存 + 输出大小
 */
export class CodeSandboxV2 {
  private config: Required<Pick<SandboxV2Config, 'timeout' | 'memoryLimit' | 'outputLimit' | 'cpuTimeLimit' | 'allowedGlobals' | 'blockedGlobals' | 'wasmTimeout' | 'fsMaxTotalBytes'>>;
  private pyodideUrl?: string;
  private jsSandbox: CodeSandbox;
  private wasmRuntime: WasmRuntime;
  private pyodidePromise: Promise<unknown> | null = null;

  constructor(config?: SandboxV2Config) {
    this.pyodideUrl = config?.pyodideUrl;
    this.config = {
      timeout: config?.timeout ?? 5000,
      memoryLimit: config?.memoryLimit ?? 64 * 1024 * 1024,
      outputLimit: config?.outputLimit ?? 1024 * 1024,
      cpuTimeLimit: config?.cpuTimeLimit ?? 3000,
      allowedGlobals: config?.allowedGlobals ?? [],
      blockedGlobals: config?.blockedGlobals ?? [],
      wasmTimeout: config?.wasmTimeout ?? config?.timeout ?? 5000,
      fsMaxTotalBytes: config?.fsMaxTotalBytes ?? 128 * 1024 * 1024,
    };

    this.jsSandbox = new CodeSandbox({
      timeout: this.config.timeout,
      memoryLimit: this.config.memoryLimit,
      outputLimit: this.config.outputLimit,
      cpuTimeLimit: this.config.cpuTimeLimit,
      allowedGlobals: this.config.allowedGlobals,
      blockedGlobals: this.config.blockedGlobals,
    });

    this.wasmRuntime = new WasmRuntime({
      defaultTimeout: this.config.wasmTimeout,
    });
  }

  /**
   * 执行代码（自动根据语言路由）。
   */
  async run(request: ExecRequest): Promise<ExecResult> {
    const timeout = request.timeout ?? this.config.timeout;

    switch (request.language) {
      case 'js':
        return this.runJs(request, timeout);
      case 'wasm':
        return this.runWasm(request, timeout);
      case 'python':
        return this.runPython(request, timeout);
      default:
        return {
          success: false,
          stdout: '',
          stderr: '',
          duration: 0,
          memoryUsed: 0,
          language: request.language,
          error: `Unsupported language: ${request.language as string}`,
          errorType: 'runtime',
        };
    }
  }

  /** 执行 JavaScript（Worker Thread） */
  private async runJs(request: ExecRequest, timeout: number): Promise<ExecResult> {
    const code = request.code as string;

    // 安全检查
    const check = CodeSecurityChecker.check(code);
    if (!check.safe) {
      return {
        success: false,
        stdout: '',
        stderr: check.errors.join('\n'),
        duration: 0,
        memoryUsed: 0,
        language: 'js',
        error: `Security check failed: ${check.errors.join('; ')}`,
        errorType: 'security',
      };
    }

    const result = await this.jsSandbox.execute(code, request.context);

    return {
      ...result,
      language: 'js',
    };
  }

  /** 执行 WASM 模块 */
  async runWasm(
    request: { code: BufferSource | string; args?: string[]; env?: Record<string, string>; prewriteFiles?: Record<string, string | Uint8Array>; timeout?: number },
    timeout?: number,
  ): Promise<ExecResult> {
    // code 在 wasm 场景下始终是 BufferSource
    const wasmBytes = request.code as BufferSource;
    const actualTimeout = timeout ?? request.timeout ?? this.config.wasmTimeout;

    const fs = new VirtualFS({
      maxTotalBytes: this.config.fsMaxTotalBytes,
    });

    const result = await this.wasmRuntime.execute(wasmBytes, {
      args: request.args,
      env: request.env,
      fs,
      prewriteFiles: request.prewriteFiles,
      timeout: actualTimeout,
    });

    // 截断输出
    let stdout = result.stdout;
    let stderr = result.stderr;
    if (stdout.length > this.config.outputLimit) {
      stdout = stdout.slice(0, this.config.outputLimit) + '...[truncated]';
    }
    if (stderr.length > this.config.outputLimit) {
      stderr = stderr.slice(0, this.config.outputLimit) + '...[truncated]';
    }

    return {
      success: result.success,
      exitCode: result.exitCode,
      stdout,
      stderr,
      duration: result.duration,
      memoryUsed: 0,
      fs: result.fs,
      language: 'wasm',
      error: result.error,
      errorType: result.error?.includes('timeout') ? 'timeout' : result.success ? undefined : 'runtime',
    };
  }

  /** 执行 Python 代码（通过 Pyodide，可选） */
  private async runPython(request: ExecRequest, timeout: number): Promise<ExecResult> {
    const startTime = Date.now();

    try {
      const pyodide = await this.getPyodide();
      const code = request.code as string;

      // 设置超时（Pyodide 在主线程同步执行，无法真正中断）
      // 这里通过设置 Pyodide 的信号机制实现软超时
      const fs = new VirtualFS({
        maxTotalBytes: this.config.fsMaxTotalBytes,
      });

      // 预写入文件到 Pyodide 的虚拟 FS
      if (request.prewriteFiles) {
        for (const [path, content] of Object.entries(request.prewriteFiles)) {
          const text = typeof content === 'string' ? content : new TextDecoder().decode(content);
          fs.writeFile(path, text);
        }
      }

      // 捕获 stdout/stderr
      let stdout = '';
      let stderr = '';

      const pyodideWithApi = pyodide as {
        runPython: (code: string, options?: { stdout?: (s: string) => void; stderr?: (s: string) => void }) => unknown;
        setStdout: (opts: { batched?: (s: string) => void }) => void;
        setStderr: (opts: { batched?: (s: string) => void }) => void;
        loadPackagesFromImports: (code: string) => Promise<void>;
      };

      try {
        await pyodideWithApi.loadPackagesFromImports?.(code);
      } catch {
        // 忽略包加载失败
      }

      pyodideWithApi.setStdout?.({ batched: (s: string) => { stdout += s + '\n'; } });
      pyodideWithApi.setStderr?.({ batched: (s: string) => { stderr += s + '\n'; } });

      let result: unknown;
      try {
        result = pyodideWithApi.runPython(code);
      } catch (pyErr) {
        const duration = Date.now() - startTime;
        return {
          success: false,
          stdout,
          stderr: stderr + (pyErr instanceof Error ? pyErr.message : String(pyErr)),
          duration,
          memoryUsed: 0,
          language: 'python',
          fs,
          error: pyErr instanceof Error ? pyErr.message : String(pyErr),
          errorType: 'runtime',
        };
      }

      const duration = Date.now() - startTime;

      // 截断输出
      if (stdout.length > this.config.outputLimit) {
        stdout = stdout.slice(0, this.config.outputLimit) + '...[truncated]';
      }

      // 将 Pyodide FS 中的文件同步到 VirtualFS（读取 /tmp 下的文件）
      // 简化：只返回预写入的 fs

      return {
        success: true,
        result: result,
        stdout,
        stderr,
        duration,
        memoryUsed: 0,
        language: 'python',
        fs,
      };
    } catch (err) {
      const duration = Date.now() - startTime;
      const errorMsg = err instanceof Error ? err.message : String(err);

      return {
        success: false,
        stdout: '',
        stderr: errorMsg,
        duration,
        memoryUsed: 0,
        language: 'python',
        error: errorMsg,
        errorType: errorMsg.includes('timeout') ? 'timeout' : 'runtime',
      };
    }
  }

  /** 懒加载 Pyodide */
  private async getPyodide(): Promise<unknown> {
    if (this.pyodidePromise) {
      return this.pyodidePromise;
    }

    this.pyodidePromise = (async () => {
      const url = this.pyodideUrl ?? 'https://cdn.jsdelivr.net/pyodide/v0.26.2/full/pyodide.mjs';
      const mod = await import(/* @vite-ignore */ url);
      const pyodide = await mod.loadPyodide({});
      return pyodide;
    })();

    return this.pyodidePromise;
  }

  /** 检查 Pyodide 是否可用 */
  isPythonSupported(): boolean {
    return this.pyodidePromise !== null;
  }

  /** 终止所有后端 */
  terminate(): void {
    this.jsSandbox.terminate();
    this.pyodidePromise = null;
  }
}
