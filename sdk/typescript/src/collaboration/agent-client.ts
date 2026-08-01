/**
 * AgentCRDTClient — Agent 作为 CRDT 客户端参与人机协作编辑。
 *
 * v3.0 方向7：人机协作编辑（Agent 作为 CRDT 客户端）
 *
 * 核心设计：
 * - Agent 和人类用户都是 CRDT 文档的客户端
 * - Agent 生成的编辑以 CRDT 操作形式提交
 * - 通过 WebSocket 实时同步操作
 * - 使用 Lamport clock 解决并发冲突
 * - Agent 编辑和人工编辑自动合并，保证最终一致性
 *
 * 使用方式：
 *
 *   const client = new AgentCRDTClient({
 *     clientID: 'agent-1',
 *     document: new CRDTDocumentImpl(initialState),
 *     syncEndpoint: 'ws://localhost:8080/sync',
 *   });
 *   await client.connect();
 *   client.edit('content', 'Agent generated text');
 */

import {
  CRDTDocumentImpl,
  type Operation,
  LamportClock,
} from './crdt.js';

// ===== Agent CRDT 客户端 =====

export interface AgentCRDTClientConfig<T extends object> {
  /** 客户端 ID（Agent ID） */
  clientID: string;
  /** CRDT 文档 */
  document: CRDTDocumentImpl<T>;
  /** WebSocket 同步端点 */
  syncEndpoint?: string;
  /** 是否启用操作日志 */
  enableOperationLog?: boolean;
  /** 操作缓冲区大小（发送前累积的操作数） */
  operationBufferSize?: number;
  /** 自动重连间隔（毫秒），默认 5000 */
  reconnectInterval?: number;
}

export type EditType = 'set' | 'insert' | 'delete';

export interface AgentEdit {
  path: string;
  value?: unknown;
  type: EditType;
  timestamp: number;
  /** 来源：agent 或 human */
  source: 'agent' | 'human';
}

export interface SyncMessage {
  type: 'operation' | 'snapshot' | 'heartbeat' | 'sync_request';
  operations?: Operation[];
  state?: unknown;
  clientID: string;
  clock: number;
}

export type ConnectionState = 'disconnected' | 'connecting' | 'connected' | 'reconnecting';

export class AgentCRDTClient<T extends object> {
  private config: AgentCRDTClientConfig<T>;
  private document: CRDTDocumentImpl<T>;
  private clock: LamportClock;
  private ws: WebSocket | null = null;
  private state: ConnectionState = 'disconnected';
  private operationBuffer: Operation[] = [];
  private operationLog: Operation[] = [];
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private listeners: Set<(edit: AgentEdit) => void> = new Set();
  private stateListeners: Set<(state: ConnectionState) => void> = new Set();

  constructor(config: AgentCRDTClientConfig<T>) {
    this.config = {
      enableOperationLog: true,
      operationBufferSize: 10,
      reconnectInterval: 5000,
      ...config,
    };
    this.document = config.document;
    this.clock = new LamportClock(config.clientID);
  }

  /**
   * 连接到同步服务器。
   */
  async connect(): Promise<void> {
    if (!this.config.syncEndpoint) {
      // 离线模式，不连接
      this.setState('connected');
      return;
    }

    this.setState('connecting');

    try {
      const ws = new WebSocket(this.config.syncEndpoint);
      this.ws = ws;

      ws.onopen = () => {
        this.setState('connected');
        // 发送同步请求
        this.send({
          type: 'sync_request',
          clientID: this.config.clientID,
          clock: this.clock.value,
        });
      };

      ws.onmessage = (event) => {
        try {
          const msg: SyncMessage = JSON.parse(event.data);
          this.handleSyncMessage(msg);
        } catch {
          // 忽略解析错误
        }
      };

      ws.onclose = () => {
        this.setState('disconnected');
        this.scheduleReconnect();
      };

      ws.onerror = () => {
        this.setState('reconnecting');
      };
    } catch {
      this.setState('disconnected');
      this.scheduleReconnect();
    }
  }

  /**
   * 断开连接。
   */
  disconnect(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.setState('disconnected');
  }

  /**
   * Agent 编辑文档（set）。
   */
  edit(path: string, value: unknown): AgentEdit {
    const op = this.document.set(path, value);
    this.recordOperation(op, 'agent');

    const edit: AgentEdit = {
      path,
      value,
      type: 'set',
      timestamp: Date.now(),
      source: 'agent',
    };

    this.notifyListeners(edit);
    return edit;
  }

  /**
   * Agent 插入内容。
   */
  insert(path: string, value: unknown): AgentEdit {
    const op = this.document.insert(path, value);
    this.recordOperation(op, 'agent');

    const edit: AgentEdit = {
      path,
      value,
      type: 'insert',
      timestamp: Date.now(),
      source: 'agent',
    };

    this.notifyListeners(edit);
    return edit;
  }

  /**
   * Agent 删除内容。
   */
  delete(path: string): AgentEdit {
    const op = this.document.delete(path);
    this.recordOperation(op, 'agent');

    const edit: AgentEdit = {
      path,
      type: 'delete',
      timestamp: Date.now(),
      source: 'agent',
    };

    this.notifyListeners(edit);
    return edit;
  }

  /**
   * 应用来自远程的操作（人类用户或其他 Agent）。
   */
  applyRemoteOperation(op: Operation): void {
    this.clock.update(op.clock);
    this.document.apply(op);

    if (this.config.enableOperationLog) {
      this.operationLog.push(op);
    }

    const edit: AgentEdit = {
      path: op.path,
      value: op.value,
      type: op.type === 'insert' ? 'insert' : op.type === 'delete' ? 'delete' : 'set',
      timestamp: Date.now(),
      source: 'human',
    };

    this.notifyListeners(edit);
  }

  /**
   * 批量应用远程操作。
   */
  applyRemoteOperations(ops: Operation[]): void {
    for (const op of ops) {
      this.applyRemoteOperation(op);
    }
  }

  /**
   * 获取当前文档状态。
   */
  getState(): T {
    return this.document.getState();
  }

  /**
   * 获取文档中指定路径的值。
   */
  get<K>(path: string): K | undefined {
    return this.document.get<K>(path);
  }

  /**
   * 获取所有本地操作（用于同步）。
   */
  getOperations(): Operation[] {
    return this.document.getOperations();
  }

  /**
   * 获取操作日志。
   */
  getOperationLog(): Operation[] {
    return [...this.operationLog];
  }

  /**
   * 获取当前 Lamport clock 值。
   */
  getClock(): number {
    return this.clock.value;
  }

  /**
   * 获取连接状态。
   */
  getConnectionState(): ConnectionState {
    return this.state;
  }

  /**
   * 添加编辑监听器。
   */
  onEdit(listener: (edit: AgentEdit) => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  /**
   * 添加连接状态监听器。
   */
  onStateChange(listener: (state: ConnectionState) => void): () => void {
    this.stateListeners.add(listener);
    return () => this.stateListeners.delete(listener);
  }

  // ===== 内部方法 =====

  private recordOperation(op: Operation, _source: 'agent' | 'human'): void {
    if (this.config.enableOperationLog) {
      this.operationLog.push(op);
    }

    // 缓冲操作，批量发送
    this.operationBuffer.push(op);
    if (this.operationBuffer.length >= (this.config.operationBufferSize ?? 10)) {
      this.flushOperations();
    } else if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      // 即使未满缓冲区，也立即发送（低延迟模式）
      this.flushOperations();
    }
  }

  private flushOperations(): void {
    if (this.operationBuffer.length === 0) return;
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;

    const ops = [...this.operationBuffer];
    this.operationBuffer = [];

    this.send({
      type: 'operation',
      operations: ops,
      clientID: this.config.clientID,
      clock: this.clock.value,
    });
  }

  private handleSyncMessage(msg: SyncMessage): void {
    switch (msg.type) {
      case 'operation':
        if (msg.operations) {
          this.applyRemoteOperations(msg.operations);
        }
        break;
      case 'snapshot':
        if (msg.state) {
          // 从快照恢复
          this.document = new CRDTDocumentImpl(this.config.clientID, msg.state as T);
        }
        break;
      case 'heartbeat':
        // 心跳，更新时钟
        this.clock.update(msg.clock);
        break;
      case 'sync_request':
        // 远程请求同步，发送本地操作
        this.flushOperations();
        break;
    }
  }

  private send(msg: SyncMessage): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;
    this.ws.send(JSON.stringify(msg));
  }

  private setState(state: ConnectionState): void {
    if (this.state === state) return;
    this.state = state;
    this.stateListeners.forEach(listener => listener(state));
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer) return;
    this.setState('reconnecting');
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, this.config.reconnectInterval);
  }

  private notifyListeners(edit: AgentEdit): void {
    this.listeners.forEach(listener => listener(edit));
  }
}

// ===== 冲突解决策略 =====

export type ConflictResolution = 'agent_wins' | 'human_wins' | 'merge' | 'latest';

/**
 * 冲突解决器：解决 Agent 生成内容与人工编辑的并发冲突。
 */
export class ConflictResolver {
  private strategy: ConflictResolution;

  constructor(strategy: ConflictResolution = 'latest') {
    this.strategy = strategy;
  }

  /**
   * 解决冲突。
   */
  resolve(agentOp: Operation, humanOp: Operation): Operation {
    switch (this.strategy) {
      case 'agent_wins':
        return agentOp;
      case 'human_wins':
        return humanOp;
      case 'merge':
        // 合并策略：如果两个操作都是 update 且路径不同，可以合并
        if (agentOp.path !== humanOp.path) {
          // 路径不同，不冲突
          return agentOp; // 返回 agent 操作，human 操作也会被应用
        }
        // 路径相同，按策略决定
        return agentOp.clock > humanOp.clock ? agentOp : humanOp;
      case 'latest':
      default:
        // 最新 clock 胜出
        if (agentOp.clock > humanOp.clock) return agentOp;
        if (agentOp.clock < humanOp.clock) return humanOp;
        // clock 相同，clientID 大的胜出
        return agentOp.clientID > humanOp.clientID ? agentOp : humanOp;
    }
  }

  /**
   * 批量解决冲突。
   */
  resolveBatch(agentOps: Operation[], humanOps: Operation[]): Operation[] {
    const result: Operation[] = [];
    const used = new Set<number>();

    for (const agentOp of agentOps) {
      let hasConflict = false;
      for (let i = 0; i < humanOps.length; i++) {
        if (used.has(i)) continue;
        const humanOp = humanOps[i];
        if (agentOp.path === humanOp.path) {
          result.push(this.resolve(agentOp, humanOp));
          used.add(i);
          hasConflict = true;
          break;
        }
      }
      if (!hasConflict) {
        result.push(agentOp);
      }
    }

    // 添加未处理的 human 操作
    for (let i = 0; i < humanOps.length; i++) {
      if (!used.has(i)) {
        result.push(humanOps[i]);
      }
    }

    return result;
  }
}
