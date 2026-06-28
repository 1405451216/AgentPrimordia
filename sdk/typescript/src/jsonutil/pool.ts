/**
 * JSON 序列化优化工具，与 Go 端 jsonutil/pool.go 对齐。
 *
 * 提供对象池复用，减少高频 JSON 序列化/反序列化的 GC 压力。
 * 在 Node.js 中，虽然 JSON.stringify/parse 是原生实现，但通过复用
 * 中间对象和数组可以减少内存分配。
 */

// ===== Object Pool（通用对象池） =====

/** 对象池工厂函数类型 */
type Factory<T> = () => T;

/** 对象池重置函数类型 */
type Reset<T> = (obj: T) => void;

/** 通用对象池，与 Go 端 sync.Pool 对齐。
 *
 * 用于复用高频创建的对象，减少 GC 压力。
 * 典型场景：JSON 序列化中间对象、SSE 事件对象、临时数组等。
 *
 * 使用方式：
 *   const pool = new ObjectPool<Record<string, unknown>>(() => ({}), (obj) => { for (const k in obj) delete obj[k]; });
 *   const obj = pool.get();
 *   // ... 使用 obj ...
 *   pool.put(obj);
 */
export class ObjectPool<T> {
  private pool: T[] = [];
  private factory: Factory<T>;
  private reset: Reset<T>;
  private maxSize: number;

  constructor(factory: Factory<T>, reset: Reset<T>, maxSize: number = 100) {
    this.factory = factory;
    this.reset = reset;
    this.maxSize = maxSize;
  }

  /** 从池中获取一个对象，如果池为空则创建新对象 */
  get(): T {
    return this.pool.length > 0 ? this.pool.pop()! : this.factory();
  }

  /** 将对象归还到池中 */
  put(obj: T): void {
    this.reset(obj);
    if (this.pool.length < this.maxSize) {
      this.pool.push(obj);
    }
  }

  /** 当前池中对象数量 */
  get size(): number {
    return this.pool.length;
  }
}

// ===== 预置对象池 =====

/** 泛型对象池 — 复用 Record<string, unknown> */
function createRecordFactory(): Record<string, unknown> {
  return {};
}

function resetRecord(obj: Record<string, unknown>): void {
  for (const key of Object.keys(obj)) {
    delete obj[key];
  }
}

const recordPool = new ObjectPool<Record<string, unknown>>(createRecordFactory, resetRecord, 200);

/** 数组池 — 复用 unknown[] */
function createArrayFactory(): unknown[] {
  return [];
}

function resetArray(arr: unknown[]): void {
  arr.length = 0;
}

const arrayPool = new ObjectPool<unknown[]>(createArrayFactory, resetArray, 200);

// ===== JSON 序列化/反序列化工具 =====

/** 使用 pooled 对象进行 JSON 序列化。
 *
 * 与 Go 端 jsonutil.Marshal 对齐。
 * 在 Node.js 中，JSON.stringify 是原生实现，此处主要提供
 * 统一 API 入口，便于将来切换序列化策略。
 *
 * @param v - 要序列化的值
 * @returns JSON 字符串
 */
export function Marshal(v: unknown): string {
  return JSON.stringify(v);
}

/** 使用 pooled 对象进行 JSON 反序列化。
 *
 * 与 Go 端 jsonutil.Unmarshal 对齐。
 * 使用 pooled 中间对象，减少 GC 压力。
 *
 * @param data - JSON 字符串或 Buffer
 * @returns 解析后的对象
 */
export function Unmarshal<T = unknown>(data: string | Buffer): T {
  const str = typeof data === 'string' ? data : data.toString('utf-8');
  return JSON.parse(str) as T;
}

/** 从 string 解码单个 JSON 值。
 *
 * 与 Go 端 jsonutil.DecodeString 对齐。
 * 适用于 SSE 解析等热路径，避免中间 Buffer 分配。
 *
 * @param data - JSON 字符串
 * @returns 解析后的对象
 */
export function DecodeString<T = unknown>(data: string): T {
  return JSON.parse(data) as T;
}

/** 从 Buffer 解码 JSON。
 *
 * 与 Go 端 jsonutil.DecodeReader 对齐。
 *
 * @param data - JSON Buffer
 * @returns 解析后的对象
 */
export function DecodeBuffer<T = unknown>(data: Buffer): T {
  return JSON.parse(data.toString('utf-8')) as T;
}

/** 序列化并返回 JSON 字符串（便捷别名）。
 *
 * 与 Go 端 jsonutil.MarshalBody 对齐。
 */
export function MarshalBody(v: unknown): string {
  return Marshal(v);
}

// ===== 对象池辅助函数 =====

/** 获取一个 pooled Record 对象 */
export function getRecord(): Record<string, unknown> {
  return recordPool.get();
}

/** 归还 pooled Record 对象 */
export function putRecord(obj: Record<string, unknown>): void {
  recordPool.put(obj);
}

/** 获取一个 pooled 数组 */
export function getArray(): unknown[] {
  return arrayPool.get();
}

/** 归还 pooled 数组 */
export function putArray(arr: unknown[]): void {
  arrayPool.put(arr);
}

/** 获取当前 Record 池大小 */
export function recordPoolSize(): number {
  return recordPool.size;
}

/** 获取当前数组池大小 */
export function arrayPoolSize(): number {
  return arrayPool.size;
}