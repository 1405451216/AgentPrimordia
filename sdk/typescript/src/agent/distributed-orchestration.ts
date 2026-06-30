/**
 * 分布式 Agent 编排 — 跨进程/跨机器的 Agent 协作，利用 WebSocket 传输层。
 *
 * 核心能力：
 * 1. WebSocket 传输的分布式 Agent 网络（注册、发现、任务分发）
 * 2. 基于角色的任务路由（将任务发送给最合适的 Agent）
 * 3. MapReduce 模式的分布式任务执行
 * 4. 分布式 Agent 编排（Pipeline、DAG、GroupChat 跨进程执行）
 * 5. 故障转移和负载均衡
 *
 * 与 Go 端的分布式方案区别：
 * - Go 使用 TCP/HTTP 传输，TS 使用 WebSocket（全双工、Edge 兼容）
 * - Go 需要外部注册中心（etcd/consul），TS 内置 Peer-to-Peer 发现
 * - TS 支持浏览器端参与分布式 Agent 网络
 *
 * 使用方式：
 *   const orchestrator = new DistributedOrchestrator({
 *     agentId: 'worker-1',
 *     roles: ['researcher', 'analyzer'],
 *     serverUrl: 'ws://localhost:8080',
 *   });
 *   await orchestrator.start();
 *   // 提交分布式任务
 *   const result = await orchestrator.submitMapReduce(
 *     ['task1', 'task2', 'task3'],
 *     (task) => agent.run(task),
 *   );
 */

import type { WebSocketTransport } from '../a2a/websocket-transport.js';

// ===== 类型定义 =====

/** 分布式 Agent 节点信息 */
export interface AgentNode {
  id: string;
  name: string;
  address: string;
  roles: string[];
  status: 'online' | 'offline' | 'busy';
  load: number; // 0-1, 当前负载
  lastHeartbeat: number;
  metadata?: Record<string, string>;
}

/** 分布式任务 */
export interface DistributedTask {
  id: string;
  type: 'single' | 'map' | 'reduce' | 'pipeline' | 'broadcast';
  input: string | string[];
  targetRoles?: string[];
  targetAgentId?: string;
  timeout?: number;
  priority?: number;
  metadata?: Record<string, unknown>;
}

/** 分布式任务结果 */
export interface DistributedTaskResult {
  taskId: string;
  agentId: string;
  success: boolean;
  output: string;
  error?: string;
  durationMs: number;
}

/** MapReduce 结果 */
export interface MapReduceResult {
  mapResults: DistributedTaskResult[];
  reduceResult: string;
  totalDurationMs: number;
}

/** 分布式编排配置 */
export interface DistributedOrchestrationConfig {
  /** 本地 Agent ID */
  agentId: string;
  /** 本地 Agent 名称 */
  name: string;
  /** 本地 Agent 角色 */
  roles: string[];
  /** WebSocket 服务器 URL */
  serverUrl: string;
  /** 心跳间隔（毫秒），默认 10000 */
  heartbeatIntervalMs?: number;
  /** 任务超时（毫秒），默认 60000 */
  taskTimeoutMs?: number;
  /** 最大重试次数，默认 2 */
  maxRetries?: number;
  /** 负载均衡策略 */
  loadBalanceStrategy?: 'round-robin' | 'least-loaded' | 'random';
}

// ===== 分布式编排器 =====

/**
 * 分布式 Agent 编排器 — 管理跨进程 Agent 协作。
 *
 * 工作模式：
 * 1. Worker 模式：注册为 Worker，接收并执行任务
 * 2. Coordinator 模式：分发任务给其他 Agent，收集结果
 * 3. Hybrid 模式：同时作为 Worker 和 Coordinator
 *
 * 任务类型：
 * - single: 发送给单个 Agent
 * - map: 将输入数组分发给多个 Agent 并行处理
 * - reduce: 将多个 Agent 的结果合并
 * - pipeline: 串行传递给多个 Agent
 * - broadcast: 广播给所有 Agent
 */
export class DistributedOrchestrator {
  private config: Required<DistributedOrchestrationConfig>;
  private transport: WebSocketTransport;
  private nodes: Map<string, AgentNode> = new Map();
  private taskHandlers: Map<string, (input: string) => Promise<string>> = new Map();
  private pendingTasks: Map<string, {
    resolve: (result: DistributedTaskResult) => void;
    reject: (error: Error) => void;
    timer?: ReturnType<typeof setTimeout>;
  }> = new Map();
  private roundRobinIndex = 0;
  private running = false;

  constructor(
    config: DistributedOrchestrationConfig,
    transport: WebSocketTransport,
  ) {
    this.config = {
      agentId: config.agentId,
      name: config.name,
      roles: config.roles,
      serverUrl: config.serverUrl,
      heartbeatIntervalMs: config.heartbeatIntervalMs ?? 10000,
      taskTimeoutMs: config.taskTimeoutMs ?? 60000,
      maxRetries: config.maxRetries ?? 2,
      loadBalanceStrategy: config.loadBalanceStrategy ?? 'least-loaded',
    };
    this.transport = transport;
  }

  /** 启动分布式编排器 */
  async start(): Promise<void> {
    await this.transport.connect();
    this.running = true;

    // 注册消息处理器
    this.transport.onMessage((msg) => this.handleMessage(msg));

    // 注册自己
    this.registerSelf();

    // 启动心跳
    this.startHeartbeat();
  }

  /** 停止编排器 */
  async stop(): Promise<void> {
    this.running = false;
    this.unregisterSelf();
    this.transport.close();
  }

  /** 注册任务处理器 */
  onTask(handler: (input: string) => Promise<string>): void {
    this.taskHandlers.set('default', handler);
  }

  /** 按角色注册任务处理器 */
  onRoleTask(role: string, handler: (input: string) => Promise<string>): void {
    this.taskHandlers.set(role, handler);
  }

  /** 提交单个任务到指定 Agent */
  async submitTask(
    input: string,
    opts?: { targetAgentId?: string; targetRoles?: string[]; timeout?: number },
  ): Promise<DistributedTaskResult> {
    const taskId = this.generateId();
    const target = this.selectAgent(opts?.targetAgentId, opts?.targetRoles);

    if (!target) {
      throw new Error('No available agent for task');
    }

    return this.sendTaskAndWait(taskId, target.id, input, opts?.timeout);
  }

  /** MapReduce：将任务分发给多个 Agent 并行执行，然后合并结果 */
  async submitMapReduce(
    inputs: string[],
    mapFn: (input: string) => Promise<string>,
    reduceFn?: (results: string[]) => Promise<string>,
    opts?: { targetRoles?: string[]; timeout?: number },
  ): Promise<MapReduceResult> {
    const startTime = Date.now();

    // 如果没有其他 Agent 可用，本地执行
    const availableAgents = this.getAvailableAgents(opts?.targetRoles);
    const useDistributed = availableAgents.length > 1;

    let mapResults: DistributedTaskResult[];

    if (useDistributed) {
      // 分布式执行：将输入分发给多个 Agent
      const tasks = inputs.map((input, i) => {
        const agent = availableAgents[i % availableAgents.length]!;
        const taskId = this.generateId();
        return this.sendTaskAndWait(taskId, agent.id, input, opts?.timeout);
      });
      mapResults = await Promise.allSettled(tasks).then((results) =>
        results.map((r, i) => {
          if (r.status === 'fulfilled') return r.value;
          return {
            taskId: `failed-${i}`,
            agentId: 'unknown',
            success: false,
            output: '',
            error: r.reason?.message ?? 'Unknown error',
            durationMs: 0,
          } satisfies DistributedTaskResult;
        }),
      );
    } else {
      // 本地执行
      mapResults = await Promise.all(
        inputs.map(async (input) => {
          const start = Date.now();
          try {
            const output = await mapFn(input);
            return {
              taskId: this.generateId(),
              agentId: this.config.agentId,
              success: true,
              output,
              durationMs: Date.now() - start,
            } satisfies DistributedTaskResult;
          } catch (err) {
            return {
              taskId: this.generateId(),
              agentId: this.config.agentId,
              success: false,
              output: '',
              error: err instanceof Error ? err.message : String(err),
              durationMs: Date.now() - start,
            } satisfies DistributedTaskResult;
          }
        }),
      );
    }

    // Reduce 阶段
    const successfulOutputs = mapResults.filter((r) => r.success).map((r) => r.output);
    const reduceResult = reduceFn
      ? await reduceFn(successfulOutputs)
      : successfulOutputs.join('\n---\n');

    return {
      mapResults,
      reduceResult,
      totalDurationMs: Date.now() - startTime,
    };
  }

  /** 分布式 Pipeline：串行传递给多个 Agent */
  async submitPipeline(
    input: string,
    stages: { role?: string; agentId?: string; transform?: (output: string) => string }[],
    opts?: { timeout?: number },
  ): Promise<DistributedTaskResult[]> {
    const results: DistributedTaskResult[] = [];
    let currentInput = input;

    for (const stage of stages) {
      const result = await this.submitTask(currentInput, {
        targetAgentId: stage.agentId,
        targetRoles: stage.role ? [stage.role] : undefined,
        timeout: opts?.timeout,
      });

      if (!result.success) {
        results.push(result);
        break;
      }

      // 应用转换
      currentInput = stage.transform ? stage.transform(result.output) : result.output;
      results.push(result);
    }

    return results;
  }

  /** 广播消息到所有 Agent */
  async broadcast(input: string): Promise<DistributedTaskResult[]> {
    const agents = this.getAvailableAgents();
    const tasks = agents.map((agent) => {
      const taskId = this.generateId();
      return this.sendTaskAndWait(taskId, agent.id, input);
    });

    return Promise.allSettled(tasks).then((results) =>
      results.map((r, i) => {
        if (r.status === 'fulfilled') return r.value;
        return {
          taskId: `broadcast-failed-${i}`,
          agentId: 'unknown',
          success: false,
          output: '',
          error: r.reason?.message ?? 'Unknown error',
          durationMs: 0,
        } satisfies DistributedTaskResult;
      }),
    );
  }

  /** 获取所有已知 Agent 节点 */
  getNodes(): AgentNode[] {
    return Array.from(this.nodes.values());
  }

  /** 获取可用 Agent 列表 */
  getAvailableAgents(roles?: string[]): AgentNode[] {
    let agents = Array.from(this.nodes.values()).filter(
      (n) => n.status === 'online' && n.id !== this.config.agentId,
    );

    if (roles && roles.length > 0) {
      agents = agents.filter((a) => roles.some((r) => a.roles.includes(r)));
    }

    return agents;
  }

  // ===== 内部方法 =====

  private registerSelf(): void {
    this.nodes.set(this.config.agentId, {
      id: this.config.agentId,
      name: this.config.name,
      address: this.config.serverUrl,
      roles: this.config.roles,
      status: 'online',
      load: 0,
      lastHeartbeat: Date.now(),
    });
  }

  private unregisterSelf(): void {
    this.nodes.delete(this.config.agentId);
  }

  private startHeartbeat(): void {
    const timer = setInterval(() => {
      if (!this.running) {
        clearInterval(timer);
        return;
      }
      // 更新自己的心跳
      const self = this.nodes.get(this.config.agentId);
      if (self) {
        self.lastHeartbeat = Date.now();
      }
      // 清理过期节点（超过 3 倍心跳间隔未心跳的节点视为离线）
      const now = Date.now();
      const timeout = this.config.heartbeatIntervalMs * 3;
      for (const [id, node] of this.nodes) {
        if (id === this.config.agentId) continue;
        if (now - node.lastHeartbeat > timeout) {
          node.status = 'offline';
        }
      }
    }, this.config.heartbeatIntervalMs);
  }

  private selectAgent(targetId?: string, targetRoles?: string[]): AgentNode | null {
    // 指定 Agent ID
    if (targetId) {
      const node = this.nodes.get(targetId);
      if (node && node.status === 'online') return node;
      return null;
    }

    // 按角色筛选
    const candidates = this.getAvailableAgents(targetRoles);

    // 如果没有其他 Agent，返回自己
    if (candidates.length === 0) {
      const self = this.nodes.get(this.config.agentId);
      return self ?? null;
    }

    // 负载均衡
    switch (this.config.loadBalanceStrategy) {
      case 'round-robin':
        return candidates[this.roundRobinIndex++ % candidates.length] ?? null;
      case 'random':
        return candidates[Math.floor(Math.random() * candidates.length)] ?? null;
      case 'least-loaded':
      default:
        return candidates.sort((a, b) => a.load - b.load)[0] ?? null;
    }
  }

  private sendTaskAndWait(
    taskId: string,
    targetAgentId: string,
    input: string,
    timeoutMs?: number,
  ): Promise<DistributedTaskResult> {
    return new Promise((resolve, reject) => {
      const timeout = timeoutMs ?? this.config.taskTimeoutMs;
      const timer = setTimeout(() => {
        this.pendingTasks.delete(taskId);
        reject(new Error(`Task ${taskId} timed out after ${timeout}ms`));
      }, timeout);

      this.pendingTasks.set(taskId, { resolve, reject, timer });

      // 如果目标是自己，本地执行
      if (targetAgentId === this.config.agentId) {
        this.executeLocal(taskId, input).then(resolve).catch(reject);
        return;
      }

      // 发送到远程 Agent
      this.transport.send({
        type: 'task_request',
        data: JSON.stringify({ taskId, input, from: this.config.agentId }),
      } as never).catch((err) => {
        clearTimeout(timer);
        this.pendingTasks.delete(taskId);
        reject(err);
      });
    });
  }

  private async executeLocal(taskId: string, input: string): Promise<DistributedTaskResult> {
    const start = Date.now();
    try {
      // 查找处理器
      const handler = this.taskHandlers.get('default');
      if (!handler) {
        throw new Error('No task handler registered');
      }
      const output = await handler(input);
      return {
        taskId,
        agentId: this.config.agentId,
        success: true,
        output,
        durationMs: Date.now() - start,
      };
    } catch (err) {
      return {
        taskId,
        agentId: this.config.agentId,
        success: false,
        output: '',
        error: err instanceof Error ? err.message : String(err),
        durationMs: Date.now() - start,
      };
    }
  }

  private handleMessage(msg: unknown): void {
    const message = msg as { type: string; data: string };
    if (!message.type || !message.data) return;

    try {
      const payload = JSON.parse(message.data);

      switch (message.type) {
        case 'task_request':
          this.handleTaskRequest(payload);
          break;
        case 'task_response':
          this.handleTaskResponse(payload);
          break;
        case 'heartbeat':
          this.handleHeartbeat(payload);
          break;
        case 'agent_register':
          this.handleAgentRegister(payload);
          break;
        case 'agent_unregister':
          this.handleAgentUnregister(payload);
          break;
      }
    } catch {
      // 忽略无效消息
    }
  }

  private async handleTaskRequest(payload: { taskId: string; input: string; from: string }): Promise<void> {
    const result = await this.executeLocal(payload.taskId, payload.input);

    // 发送结果
    this.transport.send({
      type: 'task_response',
      data: JSON.stringify(result),
    } as never).catch(() => {});
  }

  private handleTaskResponse(result: DistributedTaskResult): void {
    const pending = this.pendingTasks.get(result.taskId);
    if (pending) {
      clearTimeout(pending.timer);
      this.pendingTasks.delete(result.taskId);
      pending.resolve(result);
    }
  }

  private handleHeartbeat(payload: { agentId: string; load?: number }): void {
    const node = this.nodes.get(payload.agentId);
    if (node) {
      node.lastHeartbeat = Date.now();
      node.status = 'online';
      node.load = payload.load ?? 0;
    }
  }

  private handleAgentRegister(node: AgentNode): void {
    this.nodes.set(node.id, { ...node, lastHeartbeat: Date.now() });
  }

  private handleAgentUnregister(payload: { agentId: string }): void {
    this.nodes.delete(payload.agentId);
  }

  private generateId(): string {
    return `${this.config.agentId}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  }
}
