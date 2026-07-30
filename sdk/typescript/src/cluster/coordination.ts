/**
 * coordination.ts — 集群协调基础
 *
 * 轻量实现，复杂逻辑通过 A2A 委托 Go 后端。
 * 对齐 Go 端 internal/agent/cluster/manager.go 的核心接口。
 * Stability: Experimental
 */

import type { AgentInfo, Discovery } from './discovery.js';

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
