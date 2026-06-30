/**
 * WebSocket 双向流传输 — A2A 通信的实时双向通道。
 *
 * 与 Go 端 TCP/HTTP Transport 不同，WebSocket 提供全双工通信，
 * 适合实时协作、流式 token 传输等场景。
 *
 * 这是 TS SDK 的独有能力 — 浏览器原生支持 WebSocket，
 * Go 端需要第三方库（gorilla/websocket）。
 *
 * 使用方式：
 *   const transport = new WebSocketTransport('ws://localhost:8080/a2a');
 *   await transport.connect();
 *   transport.onMessage((msg) => console.log(msg));
 *   await transport.send({ type: 'task', data: 'hello' });
 *   transport.close();
 */

import type { A2AMessage } from '../a2a/transport.js';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);

// ===== 类型定义 =====

export interface WebSocketTransportConfig {
  url: string;
  /** 重连间隔（毫秒），默认 3000 */
  reconnectInterval?: number;
  /** 最大重连次数，默认 5 */
  maxReconnects?: number;
  /** 心跳间隔（毫秒），0 表示不心跳，默认 30000 */
  heartbeatInterval?: number;
  /** 连接超时（毫秒），默认 10000 */
  connectTimeout?: number;
  /** 自定义 headers（仅 Node.js ws 库支持） */
  headers?: Record<string, string>;
  /** 自定义 WebSocket 构造器（用于测试） */
  wsConstructor?: new (url: string, protocols?: string[]) => WebSocket;
}

export type WSMessageHandler = (message: A2AMessage) => void;
export type WSConnectionHandler = () => void;
export type WSErrorHandler = (error: Error) => void;

// ===== WebSocket Transport =====

export class WebSocketTransport {
  private config: Required<Omit<WebSocketTransportConfig, 'headers' | 'wsConstructor'>> & Pick<WebSocketTransportConfig, 'headers' | 'wsConstructor'>;
  private ws: WebSocket | null = null;
  private messageHandlers: WSMessageHandler[] = [];
  private connectHandlers: WSConnectionHandler[] = [];
  private disconnectHandlers: WSConnectionHandler[] = [];
  private errorHandlers: WSErrorHandler[] = [];
  private reconnectCount = 0;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private manualClose = false;

  constructor(config: WebSocketTransportConfig) {
    this.config = {
      url: config.url,
      reconnectInterval: config.reconnectInterval ?? 3000,
      maxReconnects: config.maxReconnects ?? 5,
      heartbeatInterval: config.heartbeatInterval ?? 30000,
      connectTimeout: config.connectTimeout ?? 10000,
      headers: config.headers,
      wsConstructor: config.wsConstructor,
    };
  }

  /** 连接到 WebSocket 服务器 */
  async connect(): Promise<void> {
    this.manualClose = false;

    const WS = this.config.wsConstructor ?? this.getWebSocketConstructor();
    if (!WS) {
      throw new Error('WebSocket is not available in this runtime. Install "ws" package or use Node.js 22+.');
    }

    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        reject(new Error(`WebSocket connect timeout after ${this.config.connectTimeout}ms`));
      }, this.config.connectTimeout);

      try {
        this.ws = new WS(this.config.url);
        const ws = this.ws!;

        ws.onopen = () => {
          clearTimeout(timeout);
          this.reconnectCount = 0;
          this.startHeartbeat();
          for (const h of this.connectHandlers) h();
          resolve();
        };

        ws.onmessage = (event: MessageEvent) => {
          try {
            const data = typeof event.data === 'string' ? event.data : new TextDecoder().decode(event.data as ArrayBuffer);
            const message = JSON.parse(data) as A2AMessage;
            for (const h of this.messageHandlers) h(message);
          } catch {
            // Ignore malformed messages
          }
        };

        ws.onclose = () => {
          clearTimeout(timeout);
          this.stopHeartbeat();
          for (const h of this.disconnectHandlers) h();

          if (!this.manualClose && this.reconnectCount < this.config.maxReconnects) {
            this.reconnectCount++;
            setTimeout(() => this.connect().catch(() => {}), this.config.reconnectInterval);
          }
        };

        ws.onerror = () => {
          clearTimeout(timeout);
          const err = new Error('WebSocket connection error');
          for (const h of this.errorHandlers) h(err);
          if (this.reconnectCount === 0) reject(err);
        };
      } catch (err) {
        clearTimeout(timeout);
        reject(err instanceof Error ? err : new Error(String(err)));
      }
    });
  }

  /** 发送消息 */
  async send(message: A2AMessage): Promise<void> {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      throw new Error('WebSocket is not connected');
    }
    this.ws.send(JSON.stringify(message));
  }

  /** 关闭连接 */
  close(): void {
    this.manualClose = true;
    this.stopHeartbeat();
    this.ws?.close();
    this.ws = null;
  }

  // ===== 事件订阅 =====

  onMessage(handler: WSMessageHandler): void { this.messageHandlers.push(handler); }
  onConnect(handler: WSConnectionHandler): void { this.connectHandlers.push(handler); }
  onDisconnect(handler: WSConnectionHandler): void { this.disconnectHandlers.push(handler); }
  onError(handler: WSErrorHandler): void { this.errorHandlers.push(handler); }

  // ===== 内部方法 =====

  private getWebSocketConstructor(): new (url: string, protocols?: string[]) => WebSocket {
    // 检查全局 WebSocket（浏览器、Deno、Bun、Node 22+）
    if (typeof globalThis.WebSocket !== 'undefined') {
      return globalThis.WebSocket as new (url: string, protocols?: string[]) => WebSocket;
    }

    // Node.js < 22: 尝试加载 ws 包
    if (typeof process !== 'undefined' && process.versions?.node) {
      try {
        const ws = require('ws');
        const WsClass = ws.WebSocket ?? ws.default ?? ws;
        return WsClass as new (url: string, protocols?: string[]) => WebSocket;
      } catch {
        // fallthrough
      }
    }

    // Fallback: 返回一个会抛错的构造器
    throw new Error('WebSocket is not available. Install "ws" package or use Node.js 22+.');
  }

  private startHeartbeat(): void {
    if (this.config.heartbeatInterval <= 0) return;
    this.heartbeatTimer = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        try {
          this.ws.send(JSON.stringify({ type: 'heartbeat', data: '' }));
        } catch {
          // Ignore heartbeat errors
        }
      }
    }, this.config.heartbeatInterval);
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }

  /** 获取连接状态 */
  get connected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }
}

// ===== WebSocket A2A Server（Node.js 专用） =====

/**
 * WebSocket A2A 服务端 — 接收 Agent 连接并路由消息。
 * 仅在 Node.js 环境下可用（依赖 ws 包）。
 */
export class WebSocketA2AServer {
  private port: number;
  private server: { close(): void } | null = null;
  private clients: Map<string, WebSocket> = new Map();
  private messageHandlers: WSMessageHandler[] = [];

  constructor(port: number) {
    this.port = port;
  }

  /** 启动 WebSocket 服务端 */
  async start(): Promise<void> {
    if (typeof process === 'undefined' || !process.versions?.node) {
      throw new Error('WebSocketA2AServer is only available in Node.js');
    }

    const { WebSocketServer } = require('ws');

    return new Promise((resolve) => {
      this.server = new WebSocketServer({ port: this.port });
      (this.server as unknown as { on: (event: string, cb: (ws: WebSocket & { on: (event: string, cb: (data: Buffer) => void) => void }, req: { socket?: { remoteAddress?: string }; url: string }) => void) => void }).on('connection', (ws: WebSocket & { on: (event: string, cb: (data: Buffer) => void) => void }, req: { socket?: { remoteAddress?: string }; url: string }) => {
        const clientId = `${req.socket?.remoteAddress ?? 'unknown'}-${Date.now()}`;
        this.clients.set(clientId, ws);

        ws.on('message', (data: Buffer) => {
          try {
            const message = JSON.parse(data.toString()) as A2AMessage;
            for (const h of this.messageHandlers) h(message);

            // 广播给其他客户端（除了发送者）
            for (const [id, client] of this.clients) {
              if (id !== clientId && client.readyState === WebSocket.OPEN) {
                client.send(JSON.stringify(message));
              }
            }
          } catch {
            // Ignore malformed messages
          }
        });

        ws.on('close', () => {
          this.clients.delete(clientId);
        });
      });

      resolve();
    });
  }

  onMessage(handler: WSMessageHandler): void {
    this.messageHandlers.push(handler);
  }

  close(): void {
    for (const [, ws] of this.clients) {
      ws.close();
    }
    this.clients.clear();
    this.server?.close();
    this.server = null;
  }

  get clientCount(): number {
    return this.clients.size;
  }
}
