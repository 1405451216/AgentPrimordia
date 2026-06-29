// context-compress.ts — 上下文窗口管理与 Token 估算
// 补充 stream-extended.ts 中的 CompressStrategy，提供 ContextWindow 和 Token 估算
// Mirrors Go internal/agent/context_compress.go

import type { Message } from '../types.js';
import { CompressStrategy } from './stream-extended.js';
import type { CompressConfig } from './stream-extended.js';

// ===== Token 估算 =====

/** 估算消息的 Token 数（简单启发式：1 Token ≈ 4 字符） */
export function estimateTokens(messages: Message[]): number {
  let total = 0;
  for (const m of messages) {
    total += m.content?.length ?? 0;
  }
  return Math.ceil(total / 4);
}

/** 估算单条消息的 Token 数 */
export function estimateTokenCount(text: string): number {
  return Math.ceil(text.length / 4);
}

// ===== 上下文窗口管理 =====

export interface ContextWindowConfig {
  /** 最大 Token 数 */
  maxTokens: number;
  /** 压缩策略（可选，复用 stream-extended 的 CompressStrategy） */
  compressStrategy?: CompressStrategy;
  /** 压缩配置（用于创建新 CompressStrategy） */
  compressConfig?: CompressConfig;
}

export class ContextWindow {
  private config: ContextWindowConfig;
  private strategy: CompressStrategy;

  constructor(config: ContextWindowConfig) {
    this.config = config;
    this.strategy = config.compressStrategy ?? new CompressStrategy(config.compressConfig ?? {});
  }

  /** 管理上下文窗口，在消息超出预算时自动压缩 */
  async manage(messages: Message[]): Promise<Message[]> {
    const tokens = estimateTokens(messages);
    if (tokens <= this.config.maxTokens) {
      return messages;
    }

    try {
      // 尝试 LLM 压缩
      return await this.strategy.compressWithLLM(messages);
    } catch {
      // 降级为简单截断
      return this.simpleTrim(messages);
    }
  }

  /** 简单截断：保留系统消息 + 从后向前保留最近消息 */
  simpleTrim(messages: Message[]): Message[] {
    const systemMsgs = messages.filter((m) => m.role === 'system');
    const others = messages.filter((m) => m.role !== 'system');

    const result: Message[] = [...systemMsgs];
    let tokenCount = estimateTokens(systemMsgs);

    // 从后向前添加，保留最近消息
    for (let i = others.length - 1; i >= 0; i--) {
      const msgTokens = estimateTokenCount(others[i].content);
      if (tokenCount + msgTokens <= this.config.maxTokens) {
        result.splice(systemMsgs.length, 0, others[i]);
        tokenCount += msgTokens;
      } else {
        break;
      }
    }

    return result;
  }

  /** 获取当前 Token 用量 */
  measure(messages: Message[]): { tokens: number; budget: number; usage: number } {
    const tokens = estimateTokens(messages);
    return {
      tokens,
      budget: this.config.maxTokens,
      usage: Math.round((tokens / this.config.maxTokens) * 100),
    };
  }
}