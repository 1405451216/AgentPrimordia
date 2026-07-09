/**
 * Bun Edge Agent（T3-1）。
 *
 * 使用 Bun 内置 SQLite 作为状态存储。非 Bun 环境自动降级为内存存储，可单测。
 */

import type { Provider } from '../llm/provider.js';
import type { ReActAgent } from '../agent/react-loop.js';
import { buildEdgeAgent, MemoryEdgeStorage, BunSQLiteStorage, type EdgeStorage } from './edge-storage.js';

export interface BunAgentOptions {
  name?: string;
  provider: Provider;
  storage?: EdgeStorage;
  maxTurns?: number;
  systemPrompt?: string;
}

/** Bun 上的轻量 Agent */
export class BunEdgeAgent {
  readonly storage: EdgeStorage;
  private agent: ReActAgent;

  constructor(opts: BunAgentOptions) {
    this.storage = opts.storage ?? new BunSQLiteStorage();
    this.agent = buildEdgeAgent({
      name: opts.name ?? 'bun-agent',
      provider: opts.provider,
      maxTurns: opts.maxTurns,
      systemPrompt: opts.systemPrompt,
    });
  }

  async run(input: string): Promise<string> {
    const resp = await this.agent.run(input);
    await this.storage.set('last:input', input);
    await this.storage.set('last:output', resp.content);
    return resp.content;
  }

  getAgent(): ReActAgent {
    return this.agent;
  }
}
