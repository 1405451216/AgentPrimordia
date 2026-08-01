/**
 * CRDT 同步服务器（V3.1 Phase 1 生产实现）。
 *
 * WebSocket 同步服务器，管理多客户端操作广播。
 * 替代 v3.0 中 AgentCRDTClient 无服务器可连接的状态。
 *
 * 核心能力：
 * - 多客户端 WebSocket 连接管理
 * - 操作广播：客户端提交的操作实时转发给其他客户端
 * - 操作历史：服务器端操作日志 + 新客户端快照恢复
 * - 心跳检测：自动清理断开的客户端
 *
 * 使用方式（Node.js）：
 *
 *   import { SyncServer } from '@agentprimordia/sdk/collaboration';
 *   const server = new SyncServer({ port: 8080, path: '/sync' });
 *   server.start();
 */

import type { Operation } from './crdt.js';

// ===== 类型定义 =====

/** 同步消息类型 */
export type SyncMessageType = 'operation' | 'snapshot' | 'heartbeat' | 'sync_request' | 'sync_response' | 'join' | 'leave' | 'error';

/** 同步消息 */
export interface SyncMessage {
  type: SyncMessageType;
  /** 发送者客户端 ID */
  from?: string;
  /** 操作（type=operation 时） */
  operation?: Operation;
  /** 操作批次（type=snapshot 时） */
  operations?: Operation[];
  /** 文档快照（type=snapshot 时） */
  snapshot?: unknown;
  /** 时间戳 */
  timestamp: number;
  /** 错误信息（type=error 时） */
  error?: string;
}

/** 客户端连接信息 */
export interface ClientInfo {
  id: string;
  connectedAt: number;
  lastHeartbeat: number;
  operationCount: number;
}

/** 同步服务器配置 */
export interface SyncServerConfig {
  /** 监听端口（默认 8080） */
  port?: number;
  /** WebSocket 路径（默认 /sync） */
  path?: string;
  /** 心跳超时（毫秒，默认 30000） */
  heartbeatTimeout?: number;
  /** 操作历史最大长度（默认 10000） */
  maxHistorySize?: number;
  /** 最大客户端数（默认 100） */
  maxClients?: number;
}

/** 服务器统计 */
export interface SyncServerStats {
  totalClients: number;
  activeClients: number;
  totalOperations: number;
  historySize: number;
  uptime: number;
}

// ===== WebSocket 抽象（兼容 Node.js ws 和浏览器环境） =====

interface WebSocketLike {
  send(data: string): void;
  close(): void;
  readyState: number;
  onmessage?: (event: { data: string }) => void;
  onclose?: () => void;
  onerror?: (error: unknown) => void;
}

interface WebSocketServerLike {
  on(event: string, handler: (...args: any[]) => void): void;
  close(): void;
  clients: Set<WebSocketLike>;
}

/**
 * CRDT 同步服务器。
 *
 * 管理多客户端的 CRDT 操作同步：
 * - 接收客户端操作并广播给其他客户端
 * - 维护操作历史，支持新客户端快照恢复
 * - 心跳检测，自动清理超时客户端
 */
export class SyncServer {
  private config: Required<SyncServerConfig>;
  private clients: Map<string, { ws: WebSocketLike; info: ClientInfo }> = new Map();
  private operationHistory: Operation[] = [];
  private documentState: Record<string, unknown> = {};
  private wss: WebSocketServerLike | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private startTime = 0;
  private running = false;

  constructor(config?: SyncServerConfig) {
    this.config = {
      port: config?.port ?? 8080,
      path: config?.path ?? '/sync',
      heartbeatTimeout: config?.heartbeatTimeout ?? 30000,
      maxHistorySize: config?.maxHistorySize ?? 10000,
      maxClients: config?.maxClients ?? 100,
    };
  }

  /**
   * 启动同步服务器。
   * 需要 Node.js 环境中的 'ws' 模块。
   */
  async start(): Promise<void> {
    if (this.running) {
      throw new Error('SyncServer is already running');
    }

    // 动态导入 ws（仅 Node.js 环境）
    let WebSocketServer: any;
    try {
      const ws = await import('ws');
      WebSocketServer = ws.WebSocketServer ?? ws.default?.WebSocketServer ?? ws.Server;
    } catch {
      throw new Error('SyncServer requires the "ws" package in Node.js environment');
    }

    this.wss = new WebSocketServer({
      port: this.config.port,
      path: this.config.path,
    }) as unknown as WebSocketServerLike;

    this.wss.on('connection', (ws: WebSocketLike, req?: any) => {
      this.handleConnection(ws, req);
    });

    // 启动心跳检测
    this.heartbeatTimer = setInterval(() => {
      this.checkHeartbeats();
    }, this.config.heartbeatTimeout / 2);

    this.startTime = Date.now();
    this.running = true;
  }

  /** 停止同步服务器 */
  stop(): void {
    if (!this.running) return;

    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }

    // 关闭所有客户端连接
    for (const [, client] of this.clients) {
      client.ws.close();
    }
    this.clients.clear();

    if (this.wss) {
      this.wss.close();
      this.wss = null;
    }

    this.running = false;
  }

  /** 是否正在运行 */
  isRunning(): boolean {
    return this.running;
  }

  /** 获取服务器统计 */
  getStats(): SyncServerStats {
    return {
      totalClients: this.clients.size,
      activeClients: this.getActiveClientCount(),
      totalOperations: this.operationHistory.length,
      historySize: this.operationHistory.length,
      uptime: this.running ? Date.now() - this.startTime : 0,
    };
  }

  /** 获取操作历史 */
  getHistory(limit?: number): Operation[] {
    if (limit && limit > 0) {
      return this.operationHistory.slice(-limit);
    }
    return [...this.operationHistory];
  }

  /** 获取当前文档状态 */
  getDocumentState(): Record<string, unknown> {
    return { ...this.documentState };
  }

  /** 获取已连接客户端列表 */
  getClients(): ClientInfo[] {
    return Array.from(this.clients.values()).map((c) => ({ ...c.info }));
  }

  // ===== 内部方法 =====

  /** 处理新连接 */
  private handleConnection(ws: WebSocketLike, _req?: any): void {
    if (this.clients.size >= this.config.maxClients) {
      const errorMsg: SyncMessage = {
        type: 'error',
        timestamp: Date.now(),
        error: 'Server is at maximum capacity',
      };
      ws.send(JSON.stringify(errorMsg));
      ws.close();
      return;
    }

    // 等待客户端发送 join 消息来确定 clientID
    let clientID = '';

    ws.onmessage = (event: { data: string }) => {
      try {
        const msg: SyncMessage = JSON.parse(event.data);
        this.handleMessage(ws, clientID, msg);

        // 如果是 join 消息，注册客户端
        if (msg.type === 'join' && msg.from) {
          clientID = msg.from;
          this.registerClient(clientID, ws);
        }
      } catch (_e) {
        const errorMsg: SyncMessage = {
          type: 'error',
          timestamp: Date.now(),
          error: 'Invalid message format',
        };
        ws.send(JSON.stringify(errorMsg));
      }
    };

    ws.onclose = () => {
      if (clientID) {
        this.unregisterClient(clientID);
      }
    };

    ws.onerror = () => {
      if (clientID) {
        this.unregisterClient(clientID);
      }
    };
  }

  /** 处理客户端消息 */
  private handleMessage(ws: WebSocketLike, clientID: string, msg: SyncMessage): void {
    switch (msg.type) {
      case 'operation':
        this.handleOperation(clientID, msg);
        break;

      case 'sync_request':
        this.handleSyncRequest(ws, clientID);
        break;

      case 'heartbeat':
        this.handleHeartbeat(clientID);
        break;

      case 'join':
        // 已在 handleConnection 中处理
        break;

      default:
        break;
    }
  }

  /** 处理操作消息：记录历史 + 广播 */
  private handleOperation(clientID: string, msg: SyncMessage): void {
    if (!msg.operation) return;

    const op = msg.operation;

    // 记录到历史
    this.operationHistory.push(op);
    if (this.operationHistory.length > this.config.maxHistorySize) {
      this.operationHistory = this.operationHistory.slice(-this.config.maxHistorySize);
    }

    // 更新文档状态
    if (op.type === 'update' || op.type === 'insert') {
      this.documentState[op.path] = op.value;
    } else if (op.type === 'delete') {
      delete this.documentState[op.path];
    }

    // 更新客户端统计
    const client = this.clients.get(clientID);
    if (client) {
      client.info.operationCount++;
    }

    // 广播给其他客户端
    this.broadcast(msg, clientID);
  }

  /** 处理同步请求：发送操作历史快照 */
  private handleSyncRequest(ws: WebSocketLike, _clientID: string): void {
    const response: SyncMessage = {
      type: 'snapshot',
      from: 'server',
      operations: [...this.operationHistory],
      snapshot: { ...this.documentState },
      timestamp: Date.now(),
    };
    ws.send(JSON.stringify(response));
  }

  /** 处理心跳 */
  private handleHeartbeat(clientID: string): void {
    const client = this.clients.get(clientID);
    if (client) {
      client.info.lastHeartbeat = Date.now();
    }
  }

  /** 注册客户端 */
  private registerClient(clientID: string, ws: WebSocketLike): void {
    const info: ClientInfo = {
      id: clientID,
      connectedAt: Date.now(),
      lastHeartbeat: Date.now(),
      operationCount: 0,
    };

    this.clients.set(clientID, { ws, info });

    // 通知其他客户端
    const joinMsg: SyncMessage = {
      type: 'join',
      from: clientID,
      timestamp: Date.now(),
    };
    this.broadcast(joinMsg, clientID);

    // 发送当前快照给新客户端
    this.handleSyncRequest(ws, clientID);
  }

  /** 注销客户端 */
  private unregisterClient(clientID: string): void {
    this.clients.delete(clientID);

    // 通知其他客户端
    const leaveMsg: SyncMessage = {
      type: 'leave',
      from: clientID,
      timestamp: Date.now(),
    };
    this.broadcast(leaveMsg, clientID);
  }

  /** 广播消息给所有客户端（排除指定客户端） */
  private broadcast(msg: SyncMessage, excludeClientID?: string): void {
    const data = JSON.stringify(msg);
    for (const [id, client] of this.clients) {
      if (id === excludeClientID) continue;
      try {
        if (client.ws.readyState === 1) {
          // OPEN
          client.ws.send(data);
        }
      } catch {
        // 发送失败，清理连接
        this.unregisterClient(id);
      }
    }
  }

  /** 检查心跳超时 */
  private checkHeartbeats(): void {
    const now = Date.now();
    const timeout = this.config.heartbeatTimeout;

    for (const [id, client] of this.clients) {
      if (now - client.info.lastHeartbeat > timeout) {
        client.ws.close();
        this.unregisterClient(id);
      }
    }
  }

  /** 获取活跃客户端数 */
  private getActiveClientCount(): number {
    const now = Date.now();
    let count = 0;
    for (const [, client] of this.clients) {
      if (now - client.info.lastHeartbeat < this.config.heartbeatTimeout) {
        count++;
      }
    }
    return count;
  }
}
