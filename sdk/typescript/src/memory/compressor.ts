/**
 * 记忆压缩器，与 Go 端 compressor.go 对齐。
 *
 * 当记忆条目超过阈值时，自动压缩旧条目为摘要，减少存储开销。
 */

import type { Memory } from './store.js';
import type { MemoryEpisode } from '../types.js';
import type { SummaryExtractor, SummaryResult } from './summarizer.js';

// ===== 压缩器配置 =====

/** 压缩器配置，与 Go 端 CompressorConfig 对齐 */
export interface CompressorConfig {
  /** 保留最近的 N 条不压缩（默认 20） */
  windowSize?: number;
  /** 最少条目数才触发压缩（默认 10） */
  minEpisodes?: number;
  /** 摘要提取器 */
  summarizer: CompressSummarizer;
  /** 超过此时间（毫秒）的条目可压缩（默认 24h） */
  ttl?: number;
}

// ===== 压缩摘要器接口 =====

/** 压缩摘要结果，与 Go 端 CompressorSummary 对齐 */
export interface CompressorSummary {
  /** 摘要文本 */
  text: string;
  /** 标签列表 */
  tags: string[];
}

/** 压缩摘要接口，与 Go 端 CompressSummarizer 对齐 */
export interface CompressSummarizer {
  /** 从多个记忆片段中提取摘要 */
  summarize(episodes: MemoryEpisode[]): Promise<CompressorSummary>;
}

// ===== LLM 压缩摘要器 =====

/** 基于 LLM 的压缩摘要器 */
export class LLMCompressSummarizer implements CompressSummarizer {
  private extractor: SummaryExtractor;

  constructor(extractor: SummaryExtractor) {
    this.extractor = extractor;
  }

  async summarize(episodes: MemoryEpisode[]): Promise<CompressorSummary> {
    const content = episodes
      .map((ep) => {
        const parts: string[] = [];
        if (ep.summary) parts.push(`[摘要]${ep.summary}`);
        if (ep.content) parts.push(`[内容]${ep.content}`);
        if (ep.topics) parts.push(`[标签]${ep.topics}`);
        return parts.join(' ');
      })
      .join('\n');

    const result: SummaryResult = await this.extractor.extractSummary(content);
    return {
      text: result.summary,
      tags: result.topics ? result.topics.split(',').map((t) => t.trim()) : [],
    };
  }
}

// ===== Compressor =====

/** 记忆压缩器，与 Go 端 Compressor 对齐。
 *
 * 当记忆条目超过阈值时，自动压缩旧条目为摘要。
 *
 * 使用方式：
 *   const compressor = new Compressor({
 *     windowSize: 20,
 *     minEpisodes: 10,
 *     summarizer: new LLMCompressSummarizer(new LLMSummarizer({ provider })),
 *   });
 *   await compressor.compress(memoryStore);
 */
export class Compressor {
  private windowSize: number;
  private minEpisodes: number;
  private summarizer: CompressSummarizer;
  private ttl: number;

  constructor(config: CompressorConfig) {
    this.windowSize = config.windowSize ?? 20;
    this.minEpisodes = config.minEpisodes ?? 10;
    this.summarizer = config.summarizer;
    this.ttl = config.ttl ?? 24 * 60 * 60 * 1000; // 24h
  }

  /** 压缩旧记忆 */
  async compress(store: Memory): Promise<CompressorSummary | null> {
    const episodes = await store.list({});
    if (episodes.length < this.minEpisodes) {
      return null; // 条目太少，不压缩
    }

    // 按时间排序，分离可压缩和保留的
    const sorted = episodes.sort(
      (a, b) => (a.createdAt ?? '').localeCompare(b.createdAt ?? '')
    );

    const cutoff = sorted.length - this.windowSize;
    if (cutoff <= 0) return null;

    const toCompress = sorted.slice(0, cutoff);
    if (toCompress.length < 2) return null;

    // 生成摘要
    const summary = await this.summarizer.summarize(toCompress);

    // 删除旧条目
    for (const ep of toCompress) {
      await store.delete(ep.id);
    }

    // 添加压缩摘要作为新条目
    const compressedEpisode: MemoryEpisode = {
      id: `compressed_${Date.now()}`,
      sessionId: 'compressed',
      role: 'system',
      content: summary.text,
      summary: summary.text,
      topics: summary.tags.join(','),
      createdAt: new Date().toISOString(),
      importance: 0.5,
      metadata: {
        compressed: 'true',
        originalCount: String(toCompress.length),
      },
    };

    await store.add(compressedEpisode);
    return summary;
  }
}