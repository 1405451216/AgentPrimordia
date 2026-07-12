export interface AgentMessage {
  id: string;
  from: string;
  to: string;
  type: 'request' | 'response' | 'broadcast';
  content: string;
  timestamp: Date;
  metadata?: Record<string, unknown>;
}

export type MessageHandler = (msg: AgentMessage) => Promise<AgentMessage | void>;

export class A2ABus {
  private agents: Map<string, MessageHandler> = new Map();
  private idCounter = 0;

  register(agentID: string, handler: MessageHandler): void {
    this.agents.set(agentID, handler);
  }

  unregister(agentID: string): void {
    this.agents.delete(agentID);
  }

  async send(from: string, to: string, content: string, metadata?: Record<string, unknown>): Promise<AgentMessage | void> {
    const handler = this.agents.get(to);
    if (!handler) throw new Error(`Agent ${to} not registered`);
    const msg: AgentMessage = {
      id: `msg-${++this.idCounter}`,
      from, to, type: 'request', content, timestamp: new Date(), metadata,
    };
    return handler(msg);
  }

  /**
   * broadcast 向所有其他 agent 并行发送消息（v2.0 #10 A2A Bus broadcast 并行化）。
   *
   * 优化：将原来串行的 for...await 改为并行 Promise.all，N 个 agent 的广播
   * 耗时从 O(N) 降至 O(1)。每个 agent 独立处理，单个 agent 异常不影响其他 agent。
   *
   * @param from 发送者 ID
   * @param content 消息内容
   * @param metadata 可选元数据
   * @param timeoutMs 每个 agent 的处理超时（默认 5000ms）
   */
  async broadcast(
    from: string,
    content: string,
    metadata?: Record<string, unknown>,
    timeoutMs = 5000,
  ): Promise<void> {
    const baseMsg: AgentMessage = {
      id: `msg-${++this.idCounter}`,
      from, to: '*', type: 'broadcast', content, timestamp: new Date(), metadata,
    };

    const promises: Promise<void>[] = [];
    for (const [id, handler] of this.agents) {
      if (id !== from) {
        promises.push(
          this.callWithTimeout(handler, { ...baseMsg, to: id }, timeoutMs),
        );
      }
    }
    await Promise.all(promises);
  }

  /**
   * callWithTimeout 调用 handler 并应用超时控制。
   * handler 异常会被吞掉，不传播给 broadcast 调用方。
   */
  private callWithTimeout(
    handler: MessageHandler,
    msg: AgentMessage,
    timeoutMs: number,
  ): Promise<void> {
    return Promise.race([
      handler(msg).then(() => {}).catch(() => {}),
      new Promise<void>((_, reject) => {
        const timer = setTimeout(() => reject(new Error('broadcast timeout')), timeoutMs);
        // 防止 timer 保持事件循环活跃（Node 环境下）
        timer.unref?.();
      }),
    ]).catch(() => {
      // 吞掉所有异常：超时或 handler 内部错误均不影响其他 agent
    });
  }

  listAgents(): string[] {
    return Array.from(this.agents.keys());
  }
}

