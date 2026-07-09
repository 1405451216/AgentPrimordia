/**
 * Prompt Hot Update — Prompt 平台化的「热更新」入口（T2-2 文档命名文件）。
 *
 * 复用已实现的 HotUpdateManager（hot-update.ts）作为底层，提供与文档一致的
 * PromptHotUpdateManager 别名，以及一处便捷工厂 createPromptHotUpdater()。
 *
 * 热更新能力：在不重启 Agent 的前提下切换生效的 Prompt 版本，并通过事件总线
 * 通知订阅者（便于 UI / 多实例同步）。单元测试见 prompt-platform-t2-2.test.ts。
 */

export {
  HotUpdateManager as PromptHotUpdateManager,
  type HotUpdateEvent,
  type FileWatcherSource,
  type PollingSource,
  type FileWatcherOptions,
} from './hot-update.js';

import { HotUpdateManager, type HotUpdateEvent } from './hot-update.js';
import { VersionedPromptRegistry } from './versioned-registry.js';

/** 便捷工厂：基于一个版本化注册表创建一个热更新管理器 */
export function createPromptHotUpdater(
  registry: VersionedPromptRegistry = new VersionedPromptRegistry(),
): HotUpdateManager {
  return new HotUpdateManager(registry);
}

/** 订阅热更新事件的类型别名（便于调用方使用） */
export type PromptHotUpdateListener = (event: HotUpdateEvent) => void;
