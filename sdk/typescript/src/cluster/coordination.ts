/**
 * coordination.ts — 集群协调基础
 *
 * 轻量实现，复杂逻辑通过 A2A 委托 Go 后端。
 * 对齐 Go 端 internal/agent/cluster/manager.go 的核心接口。
 * Stability: Experimental
 */

import type { AgentInfo, Discovery, NodeInfo, NodeRole } from './discovery.js';

/** 集群消息 */
export interface ClusterMessage {
  id: string;
  from: string;
  to: string;
  type: string;
  content: string;
  metadata?: Record<string, string>;
  timestamp: number;
}

/** 集群消息回复 */
export interface ClusterReply {
  id: string;
  inReplyTo: string;
  from: string;
  content: string;
  isError: boolean;
  timestamp: number;
}

/** 远程节点接口 */
export interface RemoteNode {
  readonly id: string;
  readonly address: string;
  sendMessage(msg: ClusterMessage): Promise<ClusterReply>;
  healthCheck(): Promise<boolean>;
  close(): Promise<void>;
}

/** 集群协调器配置 */
export interface CoordinationConfig {
  nodeId: string;
  discovery: Discovery;
  /** 消息处理器（按消息类型注册） */
  handlers?: Map<string, (msg: ClusterMessage) => Promise<ClusterReply>>;
}

/** 集群协调器（轻量实现） */
export class ClusterCoordinator {
  private readonly nodeId: string;
  private readonly discovery: Discovery;
  private readonly handlers = new Map<string, (msg: ClusterMessage) => Promise<ClusterReply>>();
  private readonly remoteNodes = new Map<string, RemoteNode>();
  private running = false;

  constructor(config: CoordinationConfig) {
    this.nodeId = config.nodeId;
    this.discovery = config.discovery;
    if (config.handlers) {
      for (const [type, handler] of config.handlers) {
        this.handlers.set(type, handler);
      }
    }
  }

  /** 注册消息处理器 */
  onMessage(type: string, handler: (msg: ClusterMessage) => Promise<ClusterReply>): void {
    this.handlers.set(type, handler);
  }

  /** 注册远程节点 */
  addRemoteNode(node: RemoteNode): void {
    this.remoteNodes.set(node.id, node);
  }

  /** 发送消息到指定节点 */
  async sendToNode(nodeId: string, msg: ClusterMessage): Promise<ClusterReply> {
    const node = this.remoteNodes.get(nodeId);
    if (!node) {
      return {
        id: `err_${Date.now()}`,
        inReplyTo: msg.id,
        from: this.nodeId,
        content: `node not found: ${nodeId}`,
        isError: true,
        timestamp: Date.now(),
      };
    }
    return node.sendMessage(msg);
  }

  /** 广播消息到所有已知节点 */
  async broadcast(msg: ClusterMessage): Promise<ClusterReply[]> {
    const replies: ClusterReply[] = [];
    const promises = Array.from(this.remoteNodes.values()).map(async (node) => {
      try {
        return await node.sendMessage(msg);
      } catch (err) {
        return {
          id: `err_${Date.now()}`,
          inReplyTo: msg.id,
          from: node.id,
          content: err instanceof Error ? err.message : String(err),
          isError: true,
          timestamp: Date.now(),
        };
      }
    });
    replies.push(...await Promise.all(promises));
    return replies;
  }

  /** 处理入站消息 */
  async handleIncoming(msg: ClusterMessage): Promise<ClusterReply> {
    const handler = this.handlers.get(msg.type);
    if (!handler) {
      return {
        id: `reply_${msg.id}`,
        inReplyTo: msg.id,
        from: this.nodeId,
        content: `no handler for message type: ${msg.type}`,
        isError: true,
        timestamp: Date.now(),
      };
    }
    return handler(msg);
  }

  /** 获取集群成员列表 */
  async getMembers(): Promise<AgentInfo[]> {
    return this.discovery.listAgents();
  }

  /** 检查所有远程节点健康状态 */
  async healthCheckAll(): Promise<Map<string, boolean>> {
    const results = new Map<string, boolean>();
    const checks = Array.from(this.remoteNodes.entries()).map(async ([id, node]) => {
      const healthy = await node.healthCheck();
      results.set(id, healthy);
    });
    await Promise.all(checks);
    return results;
  }

  get isRunning(): boolean { return this.running; }
  get localNodeId(): string { return this.nodeId; }
}

// ===== 对齐 Go cluster.ClusterConfig =====

/** 集群配置（对齐 Go cluster.ClusterConfig） */
export interface ClusterConfig {
  /** 当前节点 ID */
  nodeId: string;
  /** 监听地址 */
  listenAddr: string;
  /** 服务发现接口 */
  discovery?: Discovery;
  /** 心跳间隔（毫秒） */
  heartbeatIntervalMs?: number;
  /** 心跳超时（毫秒） */
  heartbeatTimeoutMs?: number;
  /** 选举超时（毫秒） */
  electionTimeoutMs?: number;
}

/** 填充集群配置默认值（对齐 Go ClusterConfigWithDefaults） */
export function clusterConfigWithDefaults(cfg: ClusterConfig): Required<ClusterConfig> {
  return {
    nodeId: cfg.nodeId,
    listenAddr: cfg.listenAddr,
    discovery: cfg.discovery ?? (undefined as unknown as Discovery),
    heartbeatIntervalMs: cfg.heartbeatIntervalMs ?? 5000,
    heartbeatTimeoutMs: cfg.heartbeatTimeoutMs ?? 15000,
    electionTimeoutMs: cfg.electionTimeoutMs ?? 10000,
  };
}

// ===== 对齐 Go cluster.ConsistentHash =====

/**
 * 一致性哈希环（对齐 Go cluster.ConsistentHash）
 *
 * 用于将 Agent/任务分片到不同节点，支持虚拟节点以平衡负载。
 */
export class ConsistentHash {
  private readonly replicas: number;
  private readonly ring: number[] = [];
  private readonly hashMap = new Map<number, string>();
  private readonly nodeSet = new Set<string>();

  constructor(replicas = 32) {
    this.replicas = replicas > 0 ? replicas : 32;
  }

  /** 添加节点到哈希环 */
  addNode(nodeId: string): void {
    if (this.nodeSet.has(nodeId)) return;
    this.nodeSet.add(nodeId);
    for (let i = 0; i < this.replicas; i++) {
      const h = this.hash(`${nodeId}#${i}`);
      this.ring.push(h);
      this.hashMap.set(h, nodeId);
    }
    this.ring.sort((a, b) => a - b);
  }

  /** 从哈希环移除节点 */
  removeNode(nodeId: string): void {
    if (!this.nodeSet.has(nodeId)) return;
    this.nodeSet.delete(nodeId);
    const toRemove: number[] = [];
    for (const [h, nid] of this.hashMap) {
      if (nid === nodeId) toRemove.push(h);
    }
    for (const h of toRemove) {
      this.hashMap.delete(h);
      const idx = this.ring.indexOf(h);
      if (idx >= 0) this.ring.splice(idx, 1);
    }
  }

  /** 获取负责指定 key 的节点 */
  getNode(key: string): string | null {
    if (this.ring.length === 0) return null;
    const h = this.hash(key);
    let lo = 0;
    let hi = this.ring.length;
    while (lo < hi) {
      const mid = (lo + hi) >>> 1;
      if (this.ring[mid] < h) lo = mid + 1;
      else hi = mid;
    }
    if (lo >= this.ring.length) lo = 0;
    return this.hashMap.get(this.ring[lo]) ?? null;
  }

  /** 获取负责指定 key 的前 N 个不同节点（用于副本） */
  getNodes(key: string, count: number): string[] {
    if (this.ring.length === 0 || count <= 0) return [];
    const h = this.hash(key);
    let lo = 0;
    let hi = this.ring.length;
    while (lo < hi) {
      const mid = (lo + hi) >>> 1;
      if (this.ring[mid] < h) lo = mid + 1;
      else hi = mid;
    }
    const seen = new Set<string>();
    const result: string[] = [];
    for (let i = 0; i < this.ring.length && result.length < count; i++) {
      const pos = (lo + i) % this.ring.length;
      const nid = this.hashMap.get(this.ring[pos]);
      if (nid && !seen.has(nid)) {
        seen.add(nid);
        result.push(nid);
      }
    }
    return result;
  }

  /** 获取环上所有节点（去重） */
  getNodesList(): string[] {
    return [...this.nodeSet];
  }

  /** 返回环上的虚拟节点数 */
  get ringSize(): number {
    return this.ring.length;
  }

  /** FNV-1a 32-bit 哈希 */
  private hash(s: string): number {
    let h = 0x811c9dc5;
    for (let i = 0; i < s.length; i++) {
      h ^= s.charCodeAt(i);
      h = (h * 0x01000193) >>> 0;
    }
    return h;
  }
}

// ===== 对齐 Go cluster.ClusterManager =====

/** 集群管理器（轻量 TS 实现，复杂逻辑通过 A2A 委托 Go 后端） */
export class ClusterManager {
  private readonly config: Required<ClusterConfig>;
  private readonly localNode: NodeInfo;
  private readonly nodes = new Map<string, NodeInfo>();
  private readonly hashRing: ConsistentHash;
  private leaderId = '';
  private role: NodeRole = 'follower';
  private _running = false;

  constructor(cfg: ClusterConfig) {
    this.config = clusterConfigWithDefaults(cfg);
    const now = new Date();
    this.localNode = {
      id: cfg.nodeId,
      address: cfg.listenAddr,
      role: 'follower',
      status: 'online',
      joinTime: now,
      lastSeen: now,
    };
    this.hashRing = new ConsistentHash(64);
    this.hashRing.addNode(cfg.nodeId);
  }

  /** 启动集群管理器 */
  async start(): Promise<void> {
    if (this._running) return;
    this._running = true;
    if (this.config.discovery) {
      await this.config.discovery.register({
        id: this.config.nodeId,
        name: this.config.nodeId,
        address: this.config.listenAddr,
        lastSeen: new Date(),
        metadata: { role: 'follower', status: 'online' },
      });
    }
  }

  /** 停止集群管理器 */
  async stop(): Promise<void> {
    if (!this._running) return;
    this._running = false;
    if (this.config.discovery) {
      await this.config.discovery.unregister(this.config.nodeId);
    }
    this.hashRing.removeNode(this.config.nodeId);
  }

  /** 获取本地节点信息 */
  getLocalNode(): NodeInfo { return { ...this.localNode }; }

  /** 列出所有已知节点 */
  listNodes(): NodeInfo[] {
    return [this.localNode, ...this.nodes.values()];
  }

  /** 获取指定节点信息 */
  getNode(nodeId: string): NodeInfo | null {
    if (nodeId === this.config.nodeId) return { ...this.localNode };
    return this.nodes.get(nodeId) ?? null;
  }

  /** 获取当前领导者 ID */
  getLeader(): string { return this.leaderId; }

  /** 判断当前节点是否为领导者 */
  isLeader(): boolean { return this.leaderId === this.config.nodeId; }

  /** 获取当前节点角色 */
  getRole(): NodeRole { return this.role; }

  /** 获取一致性哈希环 */
  getHashRing(): ConsistentHash { return this.hashRing; }

  /** 是否正在运行 */
  get running(): boolean { return this._running; }
}
