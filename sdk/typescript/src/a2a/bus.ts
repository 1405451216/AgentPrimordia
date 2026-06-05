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

  async broadcast(from: string, content: string, metadata?: Record<string, unknown>): Promise<void> {
    const msg: AgentMessage = {
      id: `msg-${++this.idCounter}`,
      from, to: '*', type: 'broadcast', content, timestamp: new Date(), metadata,
    };
    for (const [id, handler] of this.agents) {
      if (id !== from) {
        try { await handler({ ...msg, to: id }); } catch { /* swallow */ }
      }
    }
  }

  listAgents(): string[] {
    return Array.from(this.agents.keys());
  }
}
