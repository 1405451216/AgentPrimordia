/**
 * discovery.ts — Agent 服务发现接口与实现
 *
 * 对齐 Go 端 internal/agent/cluster/discovery_distributed.go
 * Stability: Experimental
 */

/** Agent 注册信息 */
export interface AgentInfo {
  id: string;
  name: string;
  address: string;
  capabilities?: string[];
  metadata?: Record<string, string>;
  lastSeen?: Date;
}

/** KV 事件类型 */
export type KVEventType = 'put' | 'delete';

/** KV 变化事件 */
export interface KVEvent {
  type: KVEventType;
  key: string;
  value?: string;
}

/** KVStore 分布式 KV 存储接口（对齐 Go KVStore） */
export interface KVStore {
  put(key: string, value: string, ttlMs?: number): Promise<void>;
  get(key: string): Promise<string>;
  delete(key: string): Promise<void>;
  listByPrefix(prefix: string): Promise<Map<string, string>>;
  watch(prefix: string): AsyncIterable<KVEvent>;
  close(): Promise<void>;
}

/** 服务发现接口（对齐 Go discovery.Discovery） */
export interface Discovery {
  register(info: AgentInfo): Promise<void>;
  unregister(agentId: string): Promise<void>;
  discover(agentId: string): Promise<AgentInfo | null>;
  listAgents(): Promise<AgentInfo[]>;
  heartbeat(agentId: string): Promise<void>;
  close(): Promise<void>;
}

/** 内存 KV 存储（用于测试和单节点模式） */
export class MemKVStore implements KVStore {
  private data = new Map<string, { value: string; expiresAt?: number }>();
  private watchers: Array<{ prefix: string; controller: AbortController; queue: KVEvent[] }> = [];

  async put(key: string, value: string, ttlMs?: number): Promise<void> {
    const expiresAt = ttlMs && ttlMs > 0 ? Date.now() + ttlMs : undefined;
    this.data.set(key, { value, expiresAt });
    this.notifyWatchers({ type: 'put', key, value });
  }

  async get(key: string): Promise<string> {
    const entry = this.data.get(key);
    if (!entry) throw new Error(`key not found: ${key}`);
    if (entry.expiresAt && Date.now() > entry.expiresAt) {
      this.data.delete(key);
      throw new Error(`key expired: ${key}`);
    }
    return entry.value;
  }

  async delete(key: string): Promise<void> {
    this.data.delete(key);
    this.notifyWatchers({ type: 'delete', key });
  }

  async listByPrefix(prefix: string): Promise<Map<string, string>> {
    const result = new Map<string, string>();
    const now = Date.now();
    for (const [key, entry] of this.data) {
      if (key.startsWith(prefix)) {
        if (!entry.expiresAt || now < entry.expiresAt) {
          result.set(key, entry.value);
        }
      }
    }
    return result;
  }

  async *watch(prefix: string): AsyncIterable<KVEvent> {
    const watcher = { prefix, controller: new AbortController(), queue: [] as KVEvent[] };
    this.watchers.push(watcher);
    try {
      while (!watcher.controller.signal.aborted) {
        if (watcher.queue.length > 0) {
          yield watcher.queue.shift()!;
        } else {
          await new Promise(resolve => setTimeout(resolve, 10));
        }
      }
    } finally {
      this.watchers = this.watchers.filter(w => w !== watcher);
    }
  }

  async close(): Promise<void> {
    for (const w of this.watchers) {
      w.controller.abort();
    }
    this.watchers = [];
    this.data.clear();
  }

  private notifyWatchers(event: KVEvent): void {
    for (const w of this.watchers) {
      if (event.key.startsWith(w.prefix)) {
        w.queue.push(event);
      }
    }
  }
}

const DISCOVERY_KEY_PREFIX = 'agentprimordia/discovery/';

/** 分布式发现配置 */
export interface DistributedDiscoveryConfig {
  nodeId: string;
  kvStore: KVStore;
  heartbeatIntervalMs?: number;
}

/** 基于 KV 存储的分布式服务发现（对齐 Go DistributedDiscovery） */
export class DistributedDiscovery implements Discovery {
  private readonly kv: KVStore;
  private readonly nodeId: string;
  private readonly heartbeatMs: number;
  private cache = new Map<string, AgentInfo>();
  private running = false;
  private heartbeatTimer?: ReturnType<typeof setInterval>;

  constructor(config: DistributedDiscoveryConfig) {
    this.kv = config.kvStore;
    this.nodeId = config.nodeId;
    this.heartbeatMs = config.heartbeatIntervalMs ?? 10000;
  }

  async start(): Promise<void> {
    if (this.running) return;
    this.running = true;
    // 初始同步
    await this.syncFromKV();
    // 心跳定时器
    this.heartbeatTimer = setInterval(() => {
      this.syncFromKV().catch(() => {});
    }, this.heartbeatMs);
  }

  async register(info: AgentInfo): Promise<void> {
    info.lastSeen = new Date();
    const key = DISCOVERY_KEY_PREFIX + info.id;
    const value = JSON.stringify(info);
    const ttl = this.heartbeatMs * 3;
    await this.kv.put(key, value, ttl);
    this.cache.set(info.id, info);
  }

  async unregister(agentId: string): Promise<void> {
    const key = DISCOVERY_KEY_PREFIX + agentId;
    await this.kv.delete(key);
    this.cache.delete(agentId);
  }

  async discover(agentId: string): Promise<AgentInfo | null> {
    // 先查缓存
    const cached = this.cache.get(agentId);
    if (cached) return { ...cached };

    // 从 KV 读取
    try {
      const key = DISCOVERY_KEY_PREFIX + agentId;
      const value = await this.kv.get(key);
      const info: AgentInfo = JSON.parse(value);
      this.cache.set(agentId, info);
      return info;
    } catch {
      return null;
    }
  }

  async listAgents(): Promise<AgentInfo[]> {
    const kvs = await this.kv.listByPrefix(DISCOVERY_KEY_PREFIX);
    const agents: AgentInfo[] = [];
    for (const [, value] of kvs) {
      try {
        agents.push(JSON.parse(value));
      } catch { /* skip malformed */ }
    }
    return agents;
  }

  async heartbeat(agentId: string): Promise<void> {
    const info = this.cache.get(agentId);
    if (info) {
      info.lastSeen = new Date();
      await this.register(info);
    }
  }

  async close(): Promise<void> {
    this.running = false;
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = undefined;
    }
    this.cache.clear();
  }

  private async syncFromKV(): Promise<void> {
    const kvs = await this.kv.listByPrefix(DISCOVERY_KEY_PREFIX);
    const newCache = new Map<string, AgentInfo>();
    for (const [key, value] of kvs) {
      try {
        const info: AgentInfo = JSON.parse(value);
        newCache.set(key.replace(DISCOVERY_KEY_PREFIX, ''), info);
      } catch { /* skip */ }
    }
    this.cache = newCache;
  }
}
