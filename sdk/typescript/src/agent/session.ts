import type { ReActAgent } from './react-loop.js';
import type { Response, Message } from '../types.js';
import type { Memory } from '../memory/store.js';

export interface SessionOption {
  id?: string;
  maxHistory?: number;
}

/**
 * Session maintains multi-turn conversation context.
 * Automatically appends history to memory and tracks conversation state.
 */
export class Session {
  readonly id: string;
  private agent: ReActAgent;
  private memory?: Memory;
  private history: Message[] = [];
  private maxHistory: number;
  private counter = 0;

  constructor(agent: ReActAgent, memory?: Memory, opts?: SessionOption) {
    this.agent = agent;
    this.memory = memory;
    this.id = opts?.id ?? `sess-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    this.maxHistory = opts?.maxHistory ?? 50;
  }

  /**
   * Ask the agent a question. Previous conversation history is automatically
   * included as context.
   */
  async ask(input: string): Promise<Response> {
    // Build context from history
    const contextParts: string[] = [];
    if (this.history.length > 0) {
      const recent = this.history.slice(-this.maxHistory);
      for (const msg of recent) {
        const prefix = msg.role === 'user' ? 'User' : msg.role === 'assistant' ? 'Assistant' : 'System';
        contextParts.push(`${prefix}: ${msg.content}`);
      }
    }

    const fullInput = contextParts.length > 0
      ? `Previous conversation:\n${contextParts.join('\n')}\n\nCurrent question: ${input}`
      : input;

    const response = await this.agent.run(fullInput);

    // Save to history
    this.history.push({ role: 'user', content: input });
    this.history.push({ role: 'assistant', content: response.content });

    // Trim history
    if (this.history.length > this.maxHistory * 2) {
      this.history = this.history.slice(-this.maxHistory * 2);
    }

    // Save to memory if available
    if (this.memory) {
      const episodeId = `ep-${this.id}-${++this.counter}`;
      await this.memory.add({
        id: episodeId,
        sessionId: this.id,
        role: 'user',
        content: input,
        createdAt: new Date().toISOString(),
      });
      await this.memory.add({
        id: `ep-${this.id}-${++this.counter}`,
        sessionId: this.id,
        role: 'assistant',
        content: response.content,
        createdAt: new Date().toISOString(),
      });
    }

    return response;
  }

  /** Stream the agent's response with conversation context. */
  async *askStream(input: string): AsyncIterable<string> {
    const contextParts: string[] = [];
    if (this.history.length > 0) {
      const recent = this.history.slice(-this.maxHistory);
      for (const msg of recent) {
        const prefix = msg.role === 'user' ? 'User' : msg.role === 'assistant' ? 'Assistant' : 'System';
        contextParts.push(`${prefix}: ${msg.content}`);
      }
    }

    const fullInput = contextParts.length > 0
      ? `Previous conversation:\n${contextParts.join('\n')}\n\nCurrent question: ${input}`
      : input;

    let fullContent = '';
    for await (const chunk of this.agent.stream(fullInput)) {
      fullContent += chunk;
      yield chunk;
    }

    this.history.push({ role: 'user', content: input });
    this.history.push({ role: 'assistant', content: fullContent });

    if (this.history.length > this.maxHistory * 2) {
      this.history = this.history.slice(-this.maxHistory * 2);
    }
  }

  /** Get conversation history. */
  getHistory(): Message[] {
    return [...this.history];
  }

  /** Clear conversation history. */
  clear(): void {
    this.history = [];
  }

  /** Get the number of messages in history. */
  get length(): number {
    return this.history.length;
  }
}
