/**
 * faults.ts — 故障注入器接口与实现
 *
 * 对齐 Go 端 internal/chaos/injector.go
 * Stability: Experimental
 */

/** 故障清理函数类型 */
export type CleanupFunc = () => Promise<void>;

/** 故障注入结果 */
export interface FaultResult {
  faultType: string;
  description: string;
  injected: boolean;
  injectTime: Date;
  cleanupTime?: Date;
  error?: Error;
}

/** 故障定义接口（对齐 Go Fault） */
export interface Fault {
  /** 返回故障类型 */
  type(): string;
  /** 返回故障描述 */
  description(): string;
  /** 注入故障，返回清理函数 */
  inject(): Promise<CleanupFunc>;
}

/** 故障注入器接口（引擎使用的高层接口，含 inject + cleanup） */
export interface FaultInjector {
  /** 注入故障 */
  inject(): Promise<CleanupFunc | void>;
  /** 清理故障 */
  cleanup(): Promise<void>;
  /** 故障类型标识 */
  type(): string;
  /** 故障描述 */
  description(): string;
}

// ===== 网络故障 =====

/** 网络延迟故障（对齐 Go NetworkDelayFault） */
export class NetworkDelayFault implements Fault {
  readonly target: string;
  readonly delayMs: number;
  readonly jitterMs: number;
  private _affected = false;

  constructor(target: string, delayMs: number, jitterMs: number) {
    this.target = target;
    this.delayMs = delayMs;
    this.jitterMs = jitterMs;
  }

  type(): string { return 'network_delay'; }

  description(): string {
    return `对 ${this.target} 注入 ${this.delayMs}ms 网络延迟 (jitter=${this.jitterMs}ms)`;
  }

  async inject(): Promise<CleanupFunc> {
    this._affected = true;
    return async () => { this._affected = false; };
  }

  get affected(): boolean { return this._affected; }
}

/** 网络分区故障（对齐 Go NetworkPartitionFault） */
export class PartitionFault implements Fault {
  readonly from: string;
  readonly to: string;
  readonly durationMs: number;
  private _active = false;

  constructor(from: string, to: string, durationMs: number) {
    this.from = from;
    this.to = to;
    this.durationMs = durationMs;
  }

  type(): string { return 'network_partition'; }

  description(): string {
    return `在 ${this.from} 和 ${this.to} 之间创建网络分区持续 ${this.durationMs}ms`;
  }

  async inject(): Promise<CleanupFunc> {
    this._active = true;
    return async () => { this._active = false; };
  }

  get active(): boolean { return this._active; }
}

/** 连接拒绝故障（对齐 Go ConnectionRefusedFault） */
export class ConnectionRefusedFault implements Fault {
  readonly target: string;
  private _active = false;

  constructor(target: string) {
    this.target = target;
  }

  type(): string { return 'connection_refused'; }

  description(): string {
    return `拒绝到 ${this.target} 的连接`;
  }

  async inject(): Promise<CleanupFunc> {
    this._active = true;
    return async () => { this._active = false; };
  }

  get active(): boolean { return this._active; }
}

// ===== 资源压力故障 =====

/** CPU 压力故障（对齐 Go CPUStressFault） */
export class CPUStressFault implements Fault {
  readonly cores: number;
  readonly durationMs: number;
  private _running = false;
  private _stopResolve: (() => void) | null = null;

  constructor(cores: number, durationMs: number) {
    this.cores = cores;
    this.durationMs = durationMs;
  }

  type(): string { return 'cpu_stress'; }

  description(): string {
    return `占用 ${this.cores} 个 CPU 核心持续 ${this.durationMs}ms`;
  }

  async inject(): Promise<CleanupFunc> {
    this._running = true;
    const stopPromise = new Promise<void>(resolve => { this._stopResolve = resolve; });
    // 模拟 CPU 密集计算的框架（实际环境需 Worker Threads）
    void stopPromise.then(() => { this._running = false; });
    return async () => {
      if (this._running && this._stopResolve) {
        this._stopResolve();
        this._running = false;
      }
    };
  }

  get running(): boolean { return this._running; }
}

/** 内存压力故障（对齐 Go MemoryStressFault） */
export class MemoryStressFault implements Fault {
  readonly sizeMB: number;
  readonly durationMs: number;
  private _running = false;
  private _blocks: ArrayBuffer[] = [];

  constructor(sizeMB: number, durationMs: number) {
    this.sizeMB = sizeMB;
    this.durationMs = durationMs;
  }

  type(): string { return 'memory_stress'; }

  description(): string {
    return `分配 ${this.sizeMB}MB 内存持续 ${this.durationMs}ms`;
  }

  async inject(): Promise<CleanupFunc> {
    this._running = true;
    // 分配并持有内存
    for (let i = 0; i < this.sizeMB; i++) {
      this._blocks.push(new ArrayBuffer(1024 * 1024)); // 1MB
    }
    return async () => {
      if (this._running) {
        this._blocks = [];
        this._running = false;
      }
    };
  }

  get running(): boolean { return this._running; }
}

/** 进程杀死故障（对齐 Go ProcessKillFault） */
export class ProcessKillFault implements Fault {
  readonly pid: number;
  readonly signal: string;
  private _executed = false;

  constructor(pid: number, signal: string) {
    this.pid = pid;
    this.signal = signal;
  }

  type(): string { return 'process_kill'; }

  description(): string {
    return `向 PID ${this.pid} 发送 ${this.signal} 信号`;
  }

  async inject(): Promise<CleanupFunc> {
    // 框架层仅记录意图
    this._executed = true;
    return async () => { this._executed = false; };
  }

  get executed(): boolean { return this._executed; }
}

// ===== 组合与测试故障 =====

/** 组合故障 — 多个故障同时注入（对齐 Go CompositeFault） */
export class CompositeFault implements Fault {
  readonly faults: Fault[];

  constructor(faults: Fault[]) {
    this.faults = faults;
  }

  type(): string { return 'composite'; }

  description(): string {
    return `组合故障（${this.faults.length} 个）`;
  }

  async inject(): Promise<CleanupFunc> {
    const cleanups: CleanupFunc[] = [];
    for (const fault of this.faults) {
      try {
        const cleanup = await fault.inject();
        cleanups.push(cleanup);
      } catch (err) {
        // 回滚已注入的故障（逆序清理）
        for (let i = cleanups.length - 1; i >= 0; i--) {
          await cleanups[i]();
        }
        throw err;
      }
    }
    return async () => {
      let firstErr: Error | undefined;
      for (let i = cleanups.length - 1; i >= 0; i--) {
        try {
          await cleanups[i]();
        } catch (err) {
          if (!firstErr) firstErr = err instanceof Error ? err : new Error(String(err));
        }
      }
      if (firstErr) throw firstErr;
    };
  }
}

/** 空操作故障 — 用于测试框架（对齐 Go NoopFault） */
export class NoopFault implements Fault {
  readonly faultName: string;

  constructor(name = 'default') {
    this.faultName = name;
  }

  type(): string { return `noop_${this.faultName}`; }

  description(): string {
    return `空操作故障: ${this.faultName}（用于测试）`;
  }

  async inject(): Promise<CleanupFunc> {
    return async () => {};
  }
}

// ===== 向后兼容别名（保留旧 API 名称） =====

/** @deprecated 使用 NetworkDelayFault */
export class LatencyFault implements FaultInjector {
  private readonly inner: NetworkDelayFault;

  constructor(delayMs: number, jitterMs: number, target: string, _probability = 1.0) {
    this.inner = new NetworkDelayFault(target, delayMs, jitterMs);
  }

  async inject(): Promise<void> { await this.inner.inject(); }
  async cleanup(): Promise<void> { /* cleanup handled by engine */ }
  type(): string { return this.inner.type(); }
  description(): string { return this.inner.description(); }
}

/** @deprecated 使用 LLMHTTPStatusFault (from llm-faults.ts) */
export class ErrorFault implements FaultInjector {
  readonly errorCode: number;
  readonly errorMessage: string;
  readonly target: string;
  private active = false;

  constructor(errorCode: number, errorMessage: string, target: string, _probability = 1.0) {
    this.errorCode = errorCode;
    this.errorMessage = errorMessage;
    this.target = target;
  }

  async inject(): Promise<void> { this.active = true; }
  async cleanup(): Promise<void> { this.active = false; }
  type(): string { return 'error'; }
  description(): string { return `对 ${this.target} 注入错误 ${this.errorCode}: ${this.errorMessage}`; }
}

/** @deprecated 使用 CPUStressFault / MemoryStressFault */
export class ResourceFault implements FaultInjector {
  readonly resourceType: string;
  readonly limit: number;
  readonly target: string;
  readonly durationMs: number;
  private active = false;

  constructor(resourceType: string, limit: number, target: string, durationMs: number) {
    this.resourceType = resourceType;
    this.limit = limit;
    this.target = target;
    this.durationMs = durationMs;
  }

  async inject(): Promise<void> { this.active = true; }
  async cleanup(): Promise<void> { this.active = false; }
  type(): string { return 'resource'; }
  description(): string { return `对 ${this.target} 注入 ${this.resourceType} 资源限制 (${this.limit})`; }
}
