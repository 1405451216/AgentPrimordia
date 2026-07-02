/** 事件类型联合。
 *
 * 包含 SDK 内置事件类型，同时通过 `(string & {})` 交叉类型
 * 支持下游应用（如 AELA）发布自定义事件（如 'model.suggestion'、
 * 'guardrail.blocked'、'context.tokens'），无需修改 SDK 源码。
 *
 * `(string & {})` 是 TypeScript 常见模式：它允许任意字符串赋值，
 * 同时保留已知字面量的自动补全和类型检查。
 */
export type EventType =
  | 'agent.start'
  | 'agent.stop'
  | 'agent.error'
  | 'turn.start'
  | 'turn.end'
  | 'tool.call'
  | 'tool.result'
  | 'llm.call'
  | 'llm.response'
  | 'pool.dispatch'
  | 'pool.complete'
  | (string & {});

export interface Event {
  id: string;
  type: EventType;
  source: string;
  timestamp: Date;
  payload?: unknown;
}

export class Bus {
  private subscribers: Map<EventType, { id: string; ch: (event: Event) => void }[]> = new Map();
  private wildcard: { id: string; ch: (event: Event) => void }[] = [];
  private idCounter = 0;
  private closed = false;

  subscribe(eventType: EventType, handler: (event: Event) => void): string {
    const id = `sub-${++this.idCounter}`;
    const entry = { id, ch: handler };
    if (!this.subscribers.has(eventType)) {
      this.subscribers.set(eventType, []);
    }
    this.subscribers.get(eventType)!.push(entry);
    return id;
  }

  subscribeAll(handler: (event: Event) => void): string {
    const id = `sub-${++this.idCounter}`;
    this.wildcard.push({ id, ch: handler });
    return id;
  }

  unsubscribe(id: string): void {
    for (const [, subs] of this.subscribers) {
      const idx = subs.findIndex((s) => s.id === id);
      if (idx >= 0) { subs.splice(idx, 1); return; }
    }
    const wIdx = this.wildcard.findIndex((s) => s.id === id);
    if (wIdx >= 0) this.wildcard.splice(wIdx, 1);
  }

  publish(event: Event): void {
    if (this.closed) return;
    if (!event.timestamp) event.timestamp = new Date();

    const subs = this.subscribers.get(event.type as EventType) ?? [];
    for (const sub of subs) {
      try { sub.ch(event); } catch (err) { console.error(`[AgentPrimordia] Event subscriber ${sub.id} failed:`, err); }
    }
    for (const sub of this.wildcard) {
      try { sub.ch(event); } catch (err) { console.error(`[AgentPrimordia] Wildcard subscriber ${sub.id} failed:`, err); }
    }
  }

  close(): void {
    this.closed = true;
    this.subscribers.clear();
    this.wildcard = [];
  }

  subscriberCount(eventType: EventType): number {
    return (this.subscribers.get(eventType)?.length ?? 0) + this.wildcard.length;
  }
}
