/**
 * 跨 Agent 共享记忆存储，与 Go 端 shared_store.go 对齐。
 *
 * 支持多个 Agent 共享记忆空间，每个 Agent 拥有私有存储，
 * 同时可以通过 Publish 发布记忆到共享空间。
 */

import type { Memory } from './store.js';
import type { MemoryEpisode } from '../types.js';
import { InMemoryStore } from './store.js';

// ===== SharedStore =====

/** 跨 Agent 共享记忆存储，与 Go 端 SharedStore 对齐。
 *
 * 每个 Agent 有独立的私有存储，同时可以通过 Publish
 * 发布记忆到共享空间供其他 Agent 搜索。
 *
 * 使用方式：
 *   const shared = new AgentSharedStore();
 *   shared.bind('agent1', agent1Memory);
 *   shared.bind('agent2', agent2Memory);
 *   await shared.publish('agent1', episode);
 *   const results = await shared.searchShared('agent2', '关键词');
 */
export class AgentSharedStore {
  /** agentID → 私有存储 */
  private bindings: Map<string, Memory> = new Map();
  /** 全局共享空间 */
  private shared: Memory;

  constructor(sharedStore?: Memory) {
    this.shared = sharedStore ?? new InMemoryStore();
  }

  /** 绑定 Agent 到其私有存储 */
  bind(agentID: string, store: Memory): void {
    this.bindings.set(agentID, store);
  }

  /** 解除绑定 */
  unbind(agentID: string): void {
    this.bindings.delete(agentID);
  }

  /** 发布共享记忆 */
  async publish(agentID: string, episode: MemoryEpisode): Promise<void> {
    if (!episode.metadata) {
      episode.metadata = {};
    }
    episode.metadata.published_by = agentID;
    await this.shared.add(episode);
  }

  /** 搜索其他 Agent 发布的共享记忆 */
  async searchShared(agentID: string, query: string): Promise<MemoryEpisode[]> {
    return this.shared.search(query, {});
  }

  /** 获取 Agent 的私有记忆 */
  getPrivate(agentID: string): Memory | null {
    return this.bindings.get(agentID) ?? null;
  }

  /** 获取所有已绑定的 Agent ID */
  getBoundAgents(): string[] {
    return [...this.bindings.keys()];
  }

  /** 获取共享记忆列表 */
  async listShared(opts?: { limit?: number; offset?: number }): Promise<MemoryEpisode[]> {
    return this.shared.list(opts);
  }

  /** 删除共享记忆 */
  async deleteShared(id: string): Promise<void> {
    await this.shared.delete(id);
  }

  /** 关闭共享存储 */
  close(): void {
    this.shared.close();
  }
}