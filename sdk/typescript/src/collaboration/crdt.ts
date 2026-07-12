/**
 * CRDT 协作编辑 - Yjs-like 简化实现
 *
 * 提供无冲突复制数据类型（CRDT）用于分布式协作编辑。
 * 核心特性：
 * - Lamport clock 解决并发冲突
 * - Last-Write-Wins (LWW) 用于标量值
 * - 数组的增量合并
 * - 支持离线编辑后合并
 *
 * 不依赖任何外部 CRDT 库，纯 TypeScript 实现。
 */

// ===== 类型定义 =====

/** 操作类型 */
export type OperationType = 'insert' | 'delete' | 'update';

/** 操作结构 */
export interface Operation {
  type: OperationType;
  path: string;
  value?: unknown;
  /** Lamport clock 时间戳 */
  clock: number;
  /** 客户端 ID */
  clientID: string;
  /** 操作 ID（去重用） */
  id?: string;
}

/** CRDT 文档接口 */
export interface CRDTDocument<T> {
  getState(): T;
  apply(operation: Operation): void;
  merge(other: CRDTDocument<T>): void;
  getOperations(): Operation[];
}

// ===== Lamport Clock =====

/**
 * Lamport 逻辑时钟
 *
 * 用于确定分布式系统中事件的偏序关系。
 * 每个客户端维护一个单调递增的计数器。
 */
export class LamportClock {
  private clock: number;
  readonly clientID: string;

  constructor(clientID: string, initialClock = 0) {
    this.clientID = clientID;
    this.clock = initialClock;
  }

  /**
   * 获取下一个时间戳（本地事件）
   */
  tick(): number {
    return ++this.clock;
  }

  /**
   * 接收远程事件时更新时钟
   * @param remoteClock - 远程时钟值
   */
  update(remoteClock: number): void {
    this.clock = Math.max(this.clock, remoteClock) + 1;
  }

  /** 获取当前时钟值 */
  get value(): number {
    return this.clock;
  }
}

// ===== LWW Register =====

/**
 * Last-Write-Wins 寄存器
 *
 * 用于解决标量值的并发写入冲突。
 * 比较规则：clock 大的胜出；clock 相同时，clientID 大的胜出。
 */
export class LWWRegister<T> {
  private value: T | undefined;
  private clock: number = 0;
  private clientID: string = '';

  constructor(initialValue?: T) {
    this.value = initialValue;
  }

  /**
   * 设置值
   * @param newValue - 新值
   * @param clientID - 客户端 ID
   * @param clock - Lamport clock 值
   */
  set(newValue: T, clientID: string, clock: number): boolean {
    if (this.shouldUpdate(clock, clientID)) {
      this.value = newValue;
      this.clock = clock;
      this.clientID = clientID;
      return true;
    }
    return false;
  }

  /** 获取当前值 */
  get(): T | undefined {
    return this.value;
  }

  /** 获取当前时钟 */
  getClock(): number {
    return this.clock;
  }

  /** 判断是否应该更新 */
  private shouldUpdate(clock: number, clientID: string): boolean {
    if (clock > this.clock) return true;
    if (clock === this.clock && clientID > this.clientID) return true;
    return false;
  }
}

// ===== LWW Element Set =====

/**
 * Last-Write-Wins 元素集合
 *
 * 用于数组的增量合并，支持：
 * - 添加元素（insert）
 * - 删除元素（delete）
 * - 更新元素（update）
 */
export class LWWElementSet<T = unknown> {
  /** 添加操作集合: key -> Operation */
  private adds: Map<string, Operation> = new Map();
  /** 删除操作集合: key -> Operation */
  private removes: Map<string, Operation> = new Map();

  /**
   * 添加元素
   * @param key - 元素唯一标识
   * @param value - 元素值
   * @param clock - Lamport clock
   * @param clientID - 客户端 ID
   */
  add(key: string, value: T, clock: number, clientID: string): void {
    const existing = this.adds.get(key);
    if (!existing || clock > existing.clock ||
        (clock === existing.clock && clientID > existing.clientID!)) {
      this.adds.set(key, { type: 'insert', path: key, value, clock, clientID });
    }
  }

  /**
   * 删除元素（逻辑删除）
   * @param key - 元素唯一标识
   * @param clock - Lamport clock
   * @param clientID - 客户端 ID
   */
  remove(key: string, clock: number, clientID: string): void {
    const existing = this.removes.get(key);
    if (!existing || clock > existing.clock ||
        (clock === existing.clock && clientID > existing.clientID!)) {
      this.removes.set(key, { type: 'delete', path: key, clock, clientID });
    }
  }

  /**
   * 获取当前有效元素列表
   * 返回所有 add 晚于 remove 的元素
   */
  getElements(): Array<{ key: string; value: T; clock: number; clientID: string }> {
    const result: Array<{ key: string; value: T; clock: number; clientID: string }> = [];

    for (const [key, addOp] of this.adds) {
      const removeOp = this.removes.get(key);
      // 如果 remove 不存在，或者 add 的 clock 更晚，则保留
      if (!removeOp ||
          addOp.clock > removeOp.clock ||
          (addOp.clock === removeOp.clock && addOp.clientID! > removeOp.clientID!)) {
        result.push({
          key,
          value: addOp.value as T,
          clock: addOp.clock,
          clientID: addOp.clientID!,
        });
      }
    }

    // 按 clock 排序，相同时按 clientID 排序
    result.sort((a, b) => a.clock - b.clock || a.clientID.localeCompare(b.clientID));
    return result;
  }

  /** 合并另一个 LWWElementSet */
  merge(other: LWWElementSet<T>): void {
    // 合并 adds
    for (const [key, op] of other.adds) {
      const existing = this.adds.get(key);
      if (!existing || op.clock > existing.clock ||
          (op.clock === existing.clock && op.clientID! > existing.clientID!)) {
        this.adds.set(key, op);
      }
    }
    // 合并 removes
    for (const [key, op] of other.removes) {
      const existing = this.removes.get(key);
      if (!existing || op.clock > existing.clock ||
          (op.clock === existing.clock && op.clientID! > existing.clientID!)) {
        this.removes.set(key, op);
      }
    }
  }
}

// ===== CRDT 文档实现 =====

/**
 * 通用 CRDT 文档
 *
 * 基于 LWW Register 实现，支持任意 JSON-like 数据结构的协作编辑。
 *
 * @example
 * const doc1 = new CRDTDocument<MyType>('client-1');
 * const doc2 = new CRDTDocument<MyType>('client-2');
 *
 * doc1.set('name', 'Alice');
 * doc2.set('name', 'Bob');
 *
 * doc1.merge(doc2); // Last-Write-Wins 决定最终值
 */
export class CRDTDocumentImpl<T extends object> implements CRDTDocument<T> {
  private registers: Map<string, LWWRegister<unknown>> = new Map();
  private operations: Operation[] = [];
  private clock: LamportClock;
  private initialValue: T;

  constructor(clientID: string, initialState: T = {} as T) {
    this.clock = new LamportClock(clientID);
    this.initialValue = { ...initialState };

    // 初始化 registers
    for (const [key, value] of Object.entries(initialState)) {
      const reg = new LWWRegister<unknown>(value);
      this.registers.set(key, reg);
    }
  }

  /**
   * 获取当前文档状态
   */
  getState(): T {
    const state = { ...this.initialValue } as T;
    for (const [key, reg] of this.registers) {
      const val = reg.get();
      if (val !== undefined) {
        (state as Record<string, unknown>)[key] = val;
      }
    }
    return state;
  }

  /**
   * 设置值（创建 update 操作）
   * @param path - 字段路径（支持点号分隔）
   * @param value - 新值
   */
  set(path: string, value: unknown): Operation {
    const clock = this.clock.tick();
    const op: Operation = {
      type: 'update',
      path,
      value,
      clock,
      clientID: this.clock.clientID,
    };

    this.apply(op);
    return op;
  }

  /**
   * 插入值（创建 insert 操作）
   * @param path - 字段路径
   * @param value - 要插入的值
   */
  insert(path: string, value: unknown): Operation {
    const clock = this.clock.tick();
    const op: Operation = {
      type: 'insert',
      path,
      value,
      clock,
      clientID: this.clock.clientID,
    };

    this.apply(op);
    return op;
  }

  /**
   * 删除值（创建 delete 操作）
   * @param path - 字段路径
   */
  delete(path: string): Operation {
    const clock = this.clock.tick();
    const op: Operation = {
      type: 'delete',
      path,
      clock,
      clientID: this.clock.clientID,
    };

    this.apply(op);
    return op;
  }

  /**
   * 获取指定路径的值
   */
  get<K>(path: string): K | undefined {
    const reg = this.registers.get(path);
    return reg?.get() as K | undefined;
  }

  /**
   * 应用操作
   */
  apply(operation: Operation): void {
    // 更新本地时钟
    this.clock.update(operation.clock);
    this.operations.push(operation);

    switch (operation.type) {
      case 'update':
      case 'insert': {
        let reg = this.registers.get(operation.path);
        if (!reg) {
          reg = new LWWRegister<unknown>();
          this.registers.set(operation.path, reg);
        }
        if (operation.value !== undefined) {
          reg.set(operation.value, operation.clientID, operation.clock);
        }
        break;
      }
      case 'delete': {
        this.registers.delete(operation.path);
        break;
      }
    }
  }

  /**
   * 合并另一个 CRDT 文档
   * 通过交换所有操作实现最终一致性
   */
  merge(other: CRDTDocument<T>): void {
    for (const op of other.getOperations()) {
      this.apply(op);
    }
  }

  /**
   * 获取所有已应用的操作
   */
  getOperations(): Operation[] {
    return [...this.operations];
  }

  /**
   * 获取当前 Lamport clock 值
   */
  getClockValue(): number {
    return this.clock.value;
  }
}

// ===== LCROperations (便捷 API) =====

/**
 * Last-Write-Wins 操作集
 *
 * 提供简单的 set/get 接口，底层使用 CRDT 文档。
 */
export class LCROperations<T extends object> {
  private doc: CRDTDocumentImpl<T>;

  constructor(clientID: string, initialState: T = {} as T) {
    this.doc = new CRDTDocumentImpl<T>(clientID, initialState);
  }

  /**
   * 设置路径的值
   * @param path - 字段路径
   * @param value - 值
   * @param clientID - 客户端 ID
   * @param clock - Lamport clock
   */
  set(path: string, value: unknown, clientID: string, clock: number): void {
    this.doc.apply({
      type: 'update',
      path,
      value,
      clock,
      clientID,
    });
  }

  /**
   * 获取路径的值
   */
  get<K>(path: string): K | undefined {
    return this.doc.get<K>(path);
  }

  /**
   * 获取完整状态
   */
  getState(): T {
    return this.doc.getState();
  }
}

// ===== 辅助函数 =====

/**
 * 生成唯一操作 ID
 */
export function generateOperationID(clientID: string, clock: number): string {
  return `${clientID}-${clock}-${Date.now()}`;
}

/**
 * 比较两个操作的因果顺序
 * @returns 负数表示 op1 在前，正数表示 op2 在前，0 表示并发
 */
export function compareOperations(op1: Operation, op2: Operation): number {
  if (op1.clock !== op2.clock) {
    return op1.clock - op2.clock;
  }
  return op1.clientID.localeCompare(op2.clientID);
}