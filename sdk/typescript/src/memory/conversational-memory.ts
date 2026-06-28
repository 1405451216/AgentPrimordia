/**
 * 对话记忆系统，与 Go 端 conversational_memory.go 对齐。
 *
 * 支持窗口管理、摘要压缩和元数据追踪。
 */

import type { SummaryExtractor, SummaryResult } from './summarizer.js';

// ===== 消息类型 =====

/** 对话消息，与 Go 端 Message 对齐 */
export interface ChatMessage {
  /** 角色：user, assistant, system, tool */
  role: string;
  /** 消息内容 */
  content: string;
  /** 时间戳 */
  timestamp: Date;
  /** 元数据 */
  metadata?: Record<string, string>;
  /** token 数量估算 */
  tokenCount?: number;
}

// ===== 摘要压缩器接口 =====

/** 摘要压缩器接口，与 Go 端 SummaryCompressor 对齐 */
export interface SummaryCompressor {
  /**
   * 压缩消息列表并生成摘要
   * @param messages - 待压缩的消息列表
   * @param existingSummary - 已有的摘要（用于增量更新）
   */
  compress(messages: ChatMessage[], existingSummary: string): Promise<string>;
}

// ===== 默认压缩器 =====

/** 默认压缩器实现 — 使用 SummaryExtractor 进行压缩 */
export class DefaultCompressor implements SummaryCompressor {
  private extractor: SummaryExtractor;

  constructor(extractor: SummaryExtractor) {
    this.extractor = extractor;
  }

  async compress(messages: ChatMessage[], existingSummary: string): Promise<string> {
    const content = messages
      .map((m) => `${m.role}: ${m.content}`)
      .join('\n');

    const result: SummaryResult = await this.extractor.extractSummary(
      existingSummary
        ? `已有摘要：${existingSummary}\n\n新消息：\n${content}`
        : content
    );

    const topics = result.topics ? `\n标签：${result.topics}` : '';
    return `${result.summary}${topics}`;
  }
}

// ===== ConversationalMemory 配置 =====

/** 对话记忆配置，与 Go 端 ConversationalMemoryConfig 对齐 */
export interface ChatMemoryConfig {
  /** 窗口最大消息数（默认 50） */
  maxMessages?: number;
  /** 触发摘要的消息数（默认 maxMessages * 80%） */
  summaryTrigger?: number;
  /** 自定义压缩器 */
  compressor?: SummaryCompressor;
  /** 初始摘要 */
  initialSummary?: string;
  /** 元数据 */
  metadata?: Record<string, string>;
}

// ===== ConversationalMemory =====

/** 对话记忆系统，与 Go 端 ConversationalMemory 对齐。
 *
 * 维护一个滑动窗口的消息列表，当消息数超过阈值时
 * 自动触发摘要压缩，将旧消息压缩为摘要以保持上下文。
 *
 * 使用方式：
 *   const memory = new ChatMemory({ maxMessages: 50 });
 *   memory.addMessage('user', '今天天气怎么样？');
 *   memory.addMessage('assistant', '今天晴天，25度...');
 *   const context = memory.getContext();
 */
export class ChatMemory {
  private messages: ChatMessage[] = [];
  private summary: string;
  private maxMessages: number;
  private summaryTrigger: number;
  private compressor: SummaryCompressor;
  private metadata: Record<string, string>;
  private lastUpdated: Date;
  private totalMessages: number = 0;

  constructor(config: ChatMemoryConfig = {}) {
    this.maxMessages = config.maxMessages ?? 50;
    this.summaryTrigger = config.summaryTrigger ?? Math.floor(this.maxMessages * 0.8);
    this.compressor = config.compressor ?? new DefaultCompressor({
      extractSummary: async () => ({ summary: '', topics: '' }),
    });
    this.summary = config.initialSummary ?? '';
    this.metadata = config.metadata ?? {};
    this.lastUpdated = new Date();
  }

  /** 添加消息到记忆中 */
  addMessage(role: string, content: string, metadata?: Record<string, string>): void {
    const msg: ChatMessage = {
      role,
      content,
      timestamp: new Date(),
      metadata,
      tokenCount: content.length,
    };

    this.messages.push(msg);
    this.totalMessages++;
    this.lastUpdated = new Date();

    // 检查是否需要压缩
    if (this.messages.length >= this.summaryTrigger) {
      this.compress().catch(() => {
        // 压缩失败时丢弃最旧的消息
        this.messages.shift();
      });
    }

    // 如果超出最大消息数，丢弃最旧的消息
    if (this.messages.length > this.maxMessages) {
      this.messages.shift();
    }
  }

  /** 获取当前消息列表 */
  getMessages(): ChatMessage[] {
    return [...this.messages];
  }

  /** 获取摘要 */
  getSummary(): string {
    return this.summary;
  }

  /** 获取完整上下文（摘要 + 消息列表） */
  getContext(): string {
    const parts: string[] = [];

    if (this.summary) {
      parts.push(`[对话摘要]\n${this.summary}`);
    }

    if (this.messages.length > 0) {
      parts.push(
        this.messages
          .map((m) => `${m.role}: ${m.content}`)
          .join('\n')
      );
    }

    return parts.join('\n\n');
  }

  /** 获取记忆统计 */
  getStats(): {
    messageCount: number;
    totalMessages: number;
    hasSummary: boolean;
    lastUpdated: Date;
    windowSize: number;
    maxMessages: number;
  } {
    return {
      messageCount: this.messages.length,
      totalMessages: this.totalMessages,
      hasSummary: this.summary.length > 0,
      lastUpdated: this.lastUpdated,
      windowSize: this.messages.length,
      maxMessages: this.maxMessages,
    };
  }

  /** 清空记忆 */
  clear(): void {
    this.messages = [];
    this.summary = '';
    this.lastUpdated = new Date();
  }

  /** 压缩记忆 */
  private async compress(): Promise<void> {
    if (this.messages.length <= this.summaryTrigger) return;

    // 保留最近的消息，压缩较旧的消息
    const keepCount = Math.floor(this.maxMessages * 0.3);
    const toCompress = this.messages.slice(0, this.messages.length - keepCount);
    this.messages = this.messages.slice(this.messages.length - keepCount);

    try {
      const newSummary = await this.compressor.compress(toCompress, this.summary);
      this.summary = newSummary;
    } catch {
      // 压缩失败，恢复消息
      this.messages = [...toCompress, ...this.messages];
    }
  }
}