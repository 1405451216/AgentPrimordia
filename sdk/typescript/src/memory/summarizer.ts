/**
 * 记忆摘要提取器，与 Go 端 summarizer.go 对齐。
 *
 * 使用 LLM 从内容中提取摘要和标签，支持重试和降级策略。
 */

import type { Provider } from '../llm/provider.js';

// ===== 摘要提取结果 =====

/** 摘要提取结果，与 Go 端 SummaryResult 对齐 */
export interface SummaryResult {
  /** 摘要文本（1-2 句话） */
  summary: string;
  /** 逗号分隔的标签 */
  topics: string;
}

// ===== 摘要提取器接口 =====

/** 摘要提取接口，与 Go 端 SummaryExtractor 对齐 */
export interface SummaryExtractor {
  /** 从内容中提取摘要和标签 */
  extractSummary(content: string): Promise<SummaryResult>;
}

// ===== LLM 摘要提取器 =====

/** 摘要提取器配置 */
export interface SummarizerConfig {
  /** LLM Provider */
  provider: Provider;
  /** 模型名称（如 flash/mini 版本以降低成本） */
  model?: string;
  /** 最大重试次数 */
  maxRetries?: number;
  /** 摘要最大长度 */
  maxSummaryLen?: number;
}

/** LLM 摘要提取器，与 Go 端 Summarizer 对齐。
 *
 * 使用 LLM 从内容中提取简短摘要和标签。
 * 支持自定义模型以降低摘要成本。
 *
 * 使用方式：
 *   const summarizer = new LLMSummarizer({ provider: llmProvider });
 *   const result = await summarizer.extractSummary('用户问：今天天气怎么样？\nAgent 回答：今天晴天，25度...');
 */
export class LLMSummarizer implements SummaryExtractor {
  private provider: Provider;
  private model: string;
  private maxRetries: number;
  private maxSummaryLen: number;

  constructor(config: SummarizerConfig) {
    this.provider = config.provider;
    this.model = config.model ?? '';
    this.maxRetries = config.maxRetries ?? 1;
    this.maxSummaryLen = config.maxSummaryLen ?? 500;
  }

  /** 设置模型 */
  withModel(model: string): LLMSummarizer {
    this.model = model;
    return this;
  }

  /** 从内容中提取摘要和标签 */
  async extractSummary(content: string): Promise<SummaryResult> {
    const prompt = `请为以下内容生成简短摘要（1-2句话）和标签。

内容：
${content}

请按以下格式输出：
第一行：摘要
第二行：topics: 标签1,标签2,标签3`;

    let _lastError: Error | null = null;

    for (let attempt = 0; attempt <= this.maxRetries; attempt++) {
      try {
        const response = await this.provider.complete({
          messages: [
            { role: 'system', content: '你是一个专业的摘要提取助手。请简洁准确地提取摘要和标签。' },
            { role: 'user', content: prompt },
          ],
          model: this.model || undefined,
          temperature: 0.1,
          maxTokens: this.maxSummaryLen,
        });

        return this.parseResponse(response.content);
      } catch (err: unknown) {
        _lastError = err instanceof Error ? err : new Error(String(err));
        if (attempt < this.maxRetries) {
          // 等待后重试
          await new Promise((resolve) => setTimeout(resolve, 500 * (attempt + 1)));
        }
      }
    }

    // 降级：返回简单的截断摘要
    const fallback = content.slice(0, this.maxSummaryLen);
    return {
      summary: fallback + (content.length > this.maxSummaryLen ? '...' : ''),
      topics: '',
    };
  }

  /** 解析 LLM 响应 */
  private parseResponse(text: string): SummaryResult {
    const lines = text.split('\n').filter((l) => l.trim());
    let summary = '';
    let topics = '';

    for (const line of lines) {
      const trimmed = line.trim();
      if (trimmed.toLowerCase().startsWith('topics:')) {
        topics = trimmed.slice(7).trim();
      } else if (!summary) {
        summary = trimmed;
      }
    }

    if (!summary && text) {
      summary = text.slice(0, this.maxSummaryLen);
    }

    return { summary, topics };
  }
}

// ===== 简单正则摘要提取器（无需 LLM） =====

/** 简单摘要提取器 — 使用正则规则提取关键信息，无需 LLM。
 *
 * 适用于不需要 LLM 的场景，或作为降级策略。
 */
export class SimpleSummarizer implements SummaryExtractor {
  private maxLen: number;

  constructor(maxLen: number = 200) {
    this.maxLen = maxLen;
  }

  async extractSummary(content: string): Promise<SummaryResult> {
    const sentences = content.split(/[。！？.!?\n]+/).filter((s) => s.trim().length > 5);
    const summary = sentences.slice(0, 3).join('。').slice(0, this.maxLen);

    // 提取常见关键词作为标签
    const keywords = this.extractKeywords(content);

    return {
      summary: summary || content.slice(0, this.maxLen),
      topics: keywords.join(','),
    };
  }

  private extractKeywords(text: string): string[] {
    // 简单的关键词提取：按频率取前 5 个非停用词
    const words = text.toLowerCase()
      .split(/[^\p{L}\p{N}]+/u)
      .filter((w) => w.length > 1 && !STOP_WORDS.has(w));

    const freq = new Map<string, number>();
    for (const w of words) {
      freq.set(w, (freq.get(w) ?? 0) + 1);
    }

    return [...freq.entries()]
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5)
      .map(([w]) => w);
  }
}

/** 常见中文停用词 */
const STOP_WORDS = new Set([
  '的', '了', '在', '是', '我', '有', '和', '就', '不', '人', '都', '一',
  '一个', '上', '也', '很', '到', '说', '要', '去', '你', '会', '着',
  '没有', '看', '好', '自己', '这', '他', '她', '它', '们', '那', '些',
  'the', 'a', 'an', 'is', 'are', 'was', 'were', 'be', 'been', 'being',
  'have', 'has', 'had', 'do', 'does', 'did', 'will', 'would', 'could',
  'should', 'may', 'might', 'can', 'shall', 'to', 'of', 'in', 'for',
  'on', 'with', 'at', 'by', 'from', 'as', 'into', 'through', 'during',
  'and', 'but', 'or', 'nor', 'not', 'so', 'yet', 'both', 'either',
  'neither', 'each', 'every', 'all', 'any', 'few', 'more', 'most',
  'other', 'some', 'such', 'no', 'only', 'own', 'same', 'than', 'too',
  'very', 'just', 'because', 'about', 'what', 'which', 'who', 'whom',
]);