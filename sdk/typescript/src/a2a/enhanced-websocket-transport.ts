/**
 * 增强型 WebSocket 传输 - 指数退避重连、心跳保活、消息队列、自动恢复
 *
 * 基于 WebSocketTransport 增强，增加以下能力：
 * - 指数退避重连策略（可配置最大重连次数）
 * - Ping/Pong 心跳保活
 * - 断线期间消息缓存队列
 * - 重连后自动重发未确认消息
 * - 实现 A2ATransport 接口，可直接作为 A2A 传输层
 */

import type { A2AMessage } from './transport.js';
import type { WebSocketTransportConfig } from './websocket-transport.js';

// ===== 类型定义 =====

/** 增强型 WebSocket 配置（扩展自 WebSocketTransportConfig） */
export interface EnhancedWSOptions extends WebSocketTransportConfig {
  /** 是否启用自动重连，默认 true */
  reconnect?: boolean;
  /** 最大重连尝试次数，默认 10 */
  maxReconnectAttempts?: number;
  /** 基础重连间隔（毫秒），默认 1000。实际间隔 = interval * 2^attempt */
  reconnectInterval?: number;
  /** 心跳间隔（毫秒），默认 30000。0 表示禁用心跳 */
  heartbeatInterval?: number;
  /** 最大消息队列大小，默认 1000 */
  messageQueueSize?: number;
  /** 消息确认超时（毫秒），默认 30000 */
  ackTimeout?: number;
}

/** 队列中的消息 */
interface QueuedMessage {
  message: A2AMessage;
  /** 是否已发送但未确认 */
  pending: boolean;
  /** 发送时间戳 */
  sentAt: number;
  /** 重发次数 */
  retries: number;
}

/** 连接状态 */
export type ConnectionStatus = 'disconnected' | 'connecting' | 'connected' | 'reconnecting';

// ===== 增强型 WebSocket Transport =====

export class EnhancedWSTransport {
  private ws: WebSocket | null = null;
  private config: Required<Omit<EnhancedWSOptions, 'headers' | 'wsConstructor'>> & Pick<EnhancedWSOptions, 'headers' | 'wsConstructor'>;
  private status: ConnectionStatus = 'disconnected';
  private reconnectAttempts = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private messageQueue: QueuedMessage[] = [];
  private pendingAcks: Map<string, QueuedMessage> = new Map();
  private manualClose = false;
  private clock = 0; // 内部逻辑时钟，用于消息排序

  // 事件处理器
  private messageHandlers: Array<(msg: A2AMessage) => void> = [];
  private connectHandlers: Array<() => void> = [];
  private disconnectHandlers: Array<() => void> = [];
  private errorHandlers: Array<(err: Error) => void> = [];
  private statusChangeHandlers: Array<(status: ConnectionStatus) => void> = [];

  //用于测试的 WebSocket 构造器注入
  private wsConstructor?: new (url: string, protocols?: string[]) => WebSocket;

  constructor(config: EnhancedWSOptions) {
    this.config = {
      url: config.url,
      reconnectInterval: config.reconnectInterval ?? 1000,
      maxReconnects: config.maxReconnects ?? 5,
      heartbeatInterval: config.heartbeatInterval ?? 30000,
      connectTimeout: config.connectTimeout ?? 10000,
      headers: config.headers,
      reconnect: config.reconnect ?? true,
      maxReconnectAttempts: config.maxReconnectAttempts ?? 10,
      messageQueueSize: config.messageQueueSize ?? 1000,
      ackTimeout: config.ackTimeout ?? 30000,
    };
    this.wsConstructor = (config as any).wsConstructor;
  }

  // ===== A2ATransport 接口实现 =====

  /**
   * 启动传输层（连接到 WebSocket 服务器）
   */
  async start(): Promise<void> {
    await this.connect();
  }

  /**
   * 停止传输层
   */
  async stop(): Promise<void> {
    this.manualClose = true;
    this.clearTimers();
    this.ws?.close();
    this.ws = null;
    this.setStatus('disconnected');
  }

  /**
   * 发送消息（A2ATransport 接口）
   * @param _target - 目标 agent（WebSocket 点对点场景不使用）
   * @param message - 消息
   */
  async send(_target: string, message: A2AMessage): Promise<A2AMessage> {
    return this.enqueueAndSend(message);
  }

  /**
   * 注册消息处理器
   */
  onMessage(handler: (msg: A2AMessage) => Promise<A2AMessage> | A2AMessage): void {
    this.messageHandlers.push((msg: A2AMessage) => {
      const result = handler(msg);
      if (result instanceof Promise) {
        result.catch(err => {
          for (const h of this.errorHandlers) h(err instanceof Error ? err : new Error(String(err)));
        });
      }
    });
  }

  // ===== 核心方法 =====

  /**
   * 连接到 WebSocket 服务器
   */
  async connect(): Promise<void> {
    if (this.status === 'connected' || this.status === 'connecting') return;

    this.manualClose = false;
    this.setStatus('connecting');

    const WS = this.wsConstructor ?? this.getWebSocketConstructor();
    if (!WS) {
      throw new Error('WebSocket is not available in this runtime.');
    }

    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        this.setStatus('disconnected');
        reject(new Error(`WebSocket connect timeout after ${this.config.connectTimeout}ms`));
      }, this.config.connectTimeout);

      try {
        this.ws = new WS(this.config.url);
        const ws = this.ws!;

        ws.onopen = () => {
          clearTimeout(timeout);
          this.setStatus('connected');
          this.reconnectAttempts = 0;
          this.startHeartbeat();
          for (const h of this.connectHandlers) h();
          // 重连后自动恢复：重发未确认消息
          this.flushPendingMessages();
          resolve();
        };

        ws.onmessage = (event: MessageEvent) => {
          this.handleMessage(event);
        };

        ws.onclose = () => {
          clearTimeout(timeout);
          this.stopHeartbeat();
          this.setStatus('disconnected');
          for (const h of this.disconnectHandlers) h();

          if (!this.manualClose && this.config.reconnect) {
            this.scheduleReconnect();
          }
        };

        ws.onerror = () => {
          clearTimeout(timeout);
          const err = new Error('WebSocket connection error');
          for (const h of this.errorHandlers) h(err);
          if (this.status !== 'connected') {
            reject(err);
          }
        };
      } catch (err) {
        clearTimeout(timeout);
        this.setStatus('disconnected');
        reject(err instanceof Error ? err : new Error(String(err)));
      }
    });
  }

  /**
   * 发送消息（如果连接中），否则加入队列
   */
  async sendMessage(message: A2AMessage): Promise<void> {
    await this.enqueueAndSend(message);
  }

  /**
   * 注册连接事件处理器
   */
  onConnect(handler: () => void): void {
    this.connectHandlers.push(handler);
  }

  /**
   * 注册断开事件处理器
   */
  onDisconnect(handler: () => void): void {
    this.disconnectHandlers.push(handler);
  }

  /**
   * 注册错误事件处理器
   */
  onError(handler: (err: Error) => void): void {
    this.errorHandlers.push(handler);
  }

  /**
   * 注册状态变更处理器
   */
  onStatusChange(handler: (status: ConnectionStatus) => void): void {
    this.statusChangeHandlers.push(handler);
  }

  /**
   * 获取当前连接状态
   */
  get connected(): boolean {
    return this.status === 'connected' && this.ws?.readyState === WebSocket.OPEN;
  }

  /** 获取当前状态 */
  getConnectionStatus(): ConnectionStatus {
    return this.status;
  }

  /** 获取待处理消息数量 */
  getPendingCount(): number {
    return this.pendingAcks.size;
  }

  /** 获取队列大小 */
  getQueueSize(): number {
    return this.messageQueue.length;
  }

  /**
   * 关闭连接
   */
  close(): void {
    this.stop();
  }

  // ===== 内部实现 =====

  /**
   * 入队并发送消息
   */
  private async enqueueAndSend(message: A2AMessage): Promise<A2AMessage> {
    const queued: QueuedMessage = {
      message,
      pending: false,
      sentAt: 0,
      retries: 0,
    };

    if (this.connected) {
      this.doSend(queued);
    } else {
      // 断线期间缓存消息
      if (this.messageQueue.length >= this.config.messageQueueSize) {
        throw new Error('Message queue is full');
      }
      this.messageQueue.push(queued);
      // 尝试重连
      if (this.config.reconnect && this.status === 'disconnected' && !this.manualClose) {
        this.connect().catch(() => {});
      }
    }

    // 返回一个 A2AMessage（模拟发送成功）
    return message;
  }

  /**
   * 实际发送消息
   */
  private doSend(queued: QueuedMessage): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;

    queued.pending = true;
    queued.sentAt = Date.now();
    this.pendingAcks.set(queued.message.id, queued);

    this.ws.send(JSON.stringify(queued.message));

    // 设置确认超时
    setTimeout(() => {
      if (this.pendingAcks.has(queued.message.id)) {
        // 超时未确认，标记为需要重发
        queued.pending = false;
        this.pendingAcks.delete(queued.message.id);
        queued.retries++;
        if (queued.retries < 3) {
          if (this.connected) {
            this.doSend(queued);
          }
        } else {
          // 超过重试次数，通知错误
          for (const h of this.errorHandlers) {
            h(new Error(`Message ${queued.message.id} failed after 3 retries`));
          }
        }
      }
    }, this.config.ackTimeout);
  }

  /**
   * 处理收到的消息
   */
  private handleMessage(event: MessageEvent): void {
    try {
      const data = typeof event.data === 'string'
        ? event.data
        : new TextDecoder().decode(event.data as ArrayBuffer);
      const message = JSON.parse(data) as A2AMessage;

      // 处理 pong 响应
      if ((message as any).type === 'pong') {
        return; // 心跳响应，不做处理
      }

      // 处理 ack 确认
      if ((message as any).type === 'ack') {
        const ackId = (message as any).ackId;
        if (ackId && this.pendingAcks.has(ackId)) {
          this.pendingAcks.delete(ackId);
        }
        return;
      }

      // 通知消息处理器
      for (const h of this.messageHandlers) h(message);
    } catch {
      // 忽略格式错误的消息
    }
  }

  /**
   * 重连后重发未确认消息
   */
  private flushPendingMessages(): void {
    // 重发队列中的消息
    const queued = [...this.messageQueue];
    this.messageQueue = [];
    for (const item of queued) {
      if (this.connected) {
        this.doSend(item);
      } else {
        this.messageQueue.push(item);
      }
    }

    // 重发未确认消息
    for (const [id, item] of this.pendingAcks) {
      if (this.connected) {
        item.pending = false;
        this.doSend(item);
      }
    }
  }

  /**
   * 调度重连（指数退避）
   */
  private scheduleReconnect(): void {
    if (this.reconnectAttempts >= this.config.maxReconnectAttempts) {
      for (const h of this.errorHandlers) {
        h(new Error(`Max reconnect attempts (${this.config.maxReconnectAttempts}) reached`));
      }
      return;
    }

    this.setStatus('reconnecting');
    this.reconnectAttempts++;

    // 指数退避: interval * 2^(attempt-1)
    const delay = this.config.reconnectInterval * Math.pow(2, this.reconnectAttempts - 1);

    this.reconnectTimer = setTimeout(() => {
      if (!this.manualClose) {
        this.connect().catch(() => {});
      }
    }, delay);
  }

  /**
   * 启动心跳保活
   */
  private startHeartbeat(): void {
    if (this.config.heartbeatInterval <= 0) return;

    this.heartbeatTimer = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        try {
          this.ws.send(JSON.stringify({ type: 'ping', clock: ++this.clock }));
        } catch {
          // 心跳失败时由 onclose 触发重连
        }
      }
    }, this.config.heartbeatInterval);
  }

  /**
   * 停止心跳
   */
  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }

  /**
   * 清除所有定时器
   */
  private clearTimers(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.stopHeartbeat();
  }

  /**
   * 更新状态并通知
   */
  private setStatus(status: ConnectionStatus): void {
    this.status = status;
    for (const h of this.statusChangeHandlers) h(status);
  }

  /**
   * 获取 WebSocket 构造器
   */
  private getWebSocketConstructor(): new (url: string, protocols?: string[]) => WebSocket {
    if (typeof globalThis.WebSocket !== "undefined") {
      return globalThis.WebSocket as new (url: string, protocols?: string[]) => WebSocket;
    }
    throw new Error("WebSocket is not available in this runtime.");
  }
}