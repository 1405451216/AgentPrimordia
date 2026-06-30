/**
 * 多模态深度融合 — Vision+Audio Agent 协作。
 *
 * 在现有 MultimodalProvider 基础上构建多 Agent 协作框架：
 * - VisionAgent: 专注图像理解和视觉推理
 * - AudioAgent: 专注音频理解和语音分析
 * - FusionAgent: 融合多模态信息，产出统一理解
 *
 * 核心流程：
 * 1. 输入分流：图像 → VisionAgent，音频 → AudioAgent，文本 → TextAgent
 * 2. 并行推理：各模态 Agent 并行处理
 * 3. 融合决策：FusionAgent 汇总各模态结果，产出最终响应
 *
 * 使用方式：
 *   const fusion = new MultimodalFusion({
 *     vision: visionProvider,
 *     audio: audioProvider,
 *     text: textProvider,
 *   });
 *   const result = await fusion.process({
 *     text: '描述这张图片中的声音',
 *     imageUrl: 'https://...',
 *     audioData: base64Audio,
 *   });
 */

import type { Provider } from '../llm/provider.js';
import type { MultimodalProvider, MultimodalRequest, MultimodalContent } from '../llm/multimodal.js';

// ===== 类型定义 =====

/** 多模态输入 */
export interface MultimodalInput {
  /** 文本输入 */
  text?: string;
  /** 图像 URL */
  imageUrl?: string;
  /** 图像 Base64 */
  imageB64?: string;
  /** 图像 MIME 类型 */
  imageMimeType?: string;
  /** 音频 Base64 */
  audioData?: string;
  /** 音频 MIME 类型 */
  audioMimeType?: string;
  /** 视频数据 */
  videoData?: string;
  /** 视频 MIME 类型 */
  videoMimeType?: string;
}

/** 单模态推理结果 */
export interface ModalityResult {
  /** 模态类型 */
  modality: 'vision' | 'audio' | 'text' | 'video';
  /** 推理结果文本 */
  content: string;
  /** 置信度 [0, 1] */
  confidence: number;
  /** 推理耗时（毫秒） */
  duration: number;
  /** Token 使用量 */
  tokensUsed?: number;
  /** 错误信息（如果失败） */
  error?: string;
}

/** 融合结果 */
export interface FusionResult {
  /** 最终融合响应 */
  content: string;
  /** 各模态推理结果 */
  modalityResults: ModalityResult[];
  /** 总耗时（毫秒） */
  totalDuration: number;
  /** 融合策略 */
  strategy: string;
  /** 总 Token 使用量 */
  totalTokens: number;
}

/** 多模态融合配置 */
export interface MultimodalFusionConfig {
  /** 视觉理解 Provider */
  vision?: MultimodalProvider;
  /** 音频理解 Provider */
  audio?: MultimodalProvider;
  /** 文本理解 Provider（必填） */
  text: Provider;
  /** 融合 Provider（可选，默认使用 text） */
  fusion?: Provider;
  /** 是否并行推理（默认 true） */
  parallel?: boolean;
  /** 融合策略 */
  strategy?: 'weighted' | 'concat' | 'hierarchical';
  /** 各模态权重（weighted 策略使用） */
  weights?: {
    vision?: number;
    audio?: number;
    text?: number;
  };
  /** 系统提示词前缀 */
  systemPromptPrefix?: string;
}

// ===== 多模态融合器 =====

/**
 * 多模态深度融合器。
 *
 * 将输入分发到各模态 Agent 并行处理，
 * 然后融合结果产出统一理解。
 */
export class MultimodalFusion {
  private config: Required<Omit<MultimodalFusionConfig, 'vision' | 'audio' | 'fusion'>> &
    Pick<MultimodalFusionConfig, 'vision' | 'audio' | 'fusion'>;

  constructor(config: MultimodalFusionConfig) {
    this.config = {
      text: config.text,
      vision: config.vision,
      audio: config.audio,
      fusion: config.fusion,
      parallel: config.parallel ?? true,
      strategy: config.strategy ?? 'hierarchical',
      weights: config.weights ?? { vision: 0.3, audio: 0.3, text: 0.4 },
      systemPromptPrefix: config.systemPromptPrefix ?? '',
    };
  }

  /**
   * 处理多模态输入。
   *
   * 根据输入中包含的模态类型，分发给对应的 Provider 处理，
   * 然后融合各模态结果。
   */
  async process(input: MultimodalInput): Promise<FusionResult> {
    const startTime = Date.now();
    const results: ModalityResult[] = [];

    // 构建各模态任务
    const tasks: Array<Promise<ModalityResult>> = [];

    // 文本任务
    if (input.text) {
      tasks.push(this.processText(input.text));
    }

    // 视觉任务
    if (input.imageUrl || input.imageB64) {
      tasks.push(this.processVision(input));
    }

    // 音频任务
    if (input.audioData) {
      tasks.push(this.processAudio(input));
    }

    // 视频任务（复用视觉 Provider）
    if (input.videoData && this.config.vision) {
      tasks.push(this.processVideo(input));
    }

    // 并行或串行执行
    if (this.config.parallel) {
      const taskResults = await Promise.allSettled(tasks);
      for (const result of taskResults) {
        if (result.status === 'fulfilled') {
          results.push(result.value);
        } else {
          results.push({
            modality: 'text',
            content: '',
            confidence: 0,
            duration: 0,
            error: result.reason instanceof Error ? result.reason.message : String(result.reason),
          });
        }
      }
    } else {
      for (const task of tasks) {
        try {
          results.push(await task);
        } catch (err) {
          results.push({
            modality: 'text',
            content: '',
            confidence: 0,
            duration: 0,
            error: err instanceof Error ? err.message : String(err),
          });
        }
      }
    }

    // 融合结果
    const fusionContent = await this.fuse(results, input.text);
    const totalDuration = Date.now() - startTime;
    const totalTokens = results.reduce((s, r) => s + (r.tokensUsed ?? 0), 0);

    return {
      content: fusionContent,
      modalityResults: results,
      totalDuration,
      strategy: this.config.strategy,
      totalTokens,
    };
  }

  // ===== 单模态处理 =====

  /** 处理文本输入 */
  private async processText(text: string): Promise<ModalityResult> {
    const start = Date.now();
    try {
      const resp = await this.config.text.complete({
        messages: [
          { role: 'system', content: `${this.config.systemPromptPrefix}You are a text understanding assistant.` },
          { role: 'user', content: text },
        ],
      });

      return {
        modality: 'text',
        content: resp.content,
        confidence: 0.9,
        duration: Date.now() - start,
        tokensUsed: resp.usage.totalTokens,
      };
    } catch (err) {
      return {
        modality: 'text',
        content: '',
        confidence: 0,
        duration: Date.now() - start,
        error: err instanceof Error ? err.message : String(err),
      };
    }
  }

  /** 处理视觉输入 */
  private async processVision(input: MultimodalInput): Promise<ModalityResult> {
    if (!this.config.vision) {
      return {
        modality: 'vision',
        content: '',
        confidence: 0,
        duration: 0,
        error: 'No vision provider configured',
      };
    }

    const start = Date.now();
    try {
      const content: MultimodalContent[] = [];
      if (input.imageUrl) {
        content.push({ type: 'image_url', imageUrl: input.imageUrl });
      }
      if (input.imageB64) {
        content.push({ type: 'image_b64', imageB64: input.imageB64, mimeType: input.imageMimeType ?? 'image/png' });
      }
      content.push({ type: 'text', text: input.text ?? 'Describe this image in detail.' });

      const req: MultimodalRequest = {
        messages: [
          {
            role: 'user',
            content,
          },
        ],
      };

      const resp = await this.config.vision.completeMultimodal(req);

      return {
        modality: 'vision',
        content: resp.content,
        confidence: 0.85,
        duration: Date.now() - start,
        tokensUsed: resp.usage.totalTokens,
      };
    } catch (err) {
      return {
        modality: 'vision',
        content: '',
        confidence: 0,
        duration: Date.now() - start,
        error: err instanceof Error ? err.message : String(err),
      };
    }
  }

  /** 处理音频输入 */
  private async processAudio(input: MultimodalInput): Promise<ModalityResult> {
    if (!this.config.audio) {
      return {
        modality: 'audio',
        content: '',
        confidence: 0,
        duration: 0,
        error: 'No audio provider configured',
      };
    }

    const start = Date.now();
    try {
      const content: MultimodalContent[] = [
        { type: 'audio', audioData: input.audioData!, mimeType: input.audioMimeType ?? 'audio/wav' },
        { type: 'text', text: input.text ?? 'Transcribe and analyze this audio.' },
      ];

      const req: MultimodalRequest = {
        messages: [{ role: 'user', content }],
      };

      const resp = await this.config.audio.completeMultimodal(req);

      return {
        modality: 'audio',
        content: resp.content,
        confidence: 0.8,
        duration: Date.now() - start,
        tokensUsed: resp.usage.totalTokens,
      };
    } catch (err) {
      return {
        modality: 'audio',
        content: '',
        confidence: 0,
        duration: Date.now() - start,
        error: err instanceof Error ? err.message : String(err),
      };
    }
  }

  /** 处理视频输入 */
  private async processVideo(input: MultimodalInput): Promise<ModalityResult> {
    if (!this.config.vision) {
      return {
        modality: 'video',
        content: '',
        confidence: 0,
        duration: 0,
        error: 'No video provider configured',
      };
    }

    const start = Date.now();
    try {
      const content: MultimodalContent[] = [
        { type: 'video', videoData: input.videoData!, mimeType: input.videoMimeType ?? 'video/mp4' },
        { type: 'text', text: input.text ?? 'Analyze this video.' },
      ];

      const req: MultimodalRequest = {
        messages: [{ role: 'user', content }],
      };

      const resp = await this.config.vision.completeMultimodal(req);

      return {
        modality: 'video',
        content: resp.content,
        confidence: 0.75,
        duration: Date.now() - start,
        tokensUsed: resp.usage.totalTokens,
      };
    } catch (err) {
      return {
        modality: 'video',
        content: '',
        confidence: 0,
        duration: Date.now() - start,
        error: err instanceof Error ? err.message : String(err),
      };
    }
  }

  // ===== 融合策略 =====

  /** 融合各模态结果 */
  private async fuse(results: ModalityResult[], originalText?: string): Promise<string> {
    // 过滤掉失败的结果
    const validResults = results.filter((r) => !r.error && r.content);

    if (validResults.length === 0) {
      return 'Unable to process input: all modalities failed.';
    }

    if (validResults.length === 1) {
      return validResults[0]!.content;
    }

    const fusionProvider = this.config.fusion ?? this.config.text;

    if (this.config.strategy === 'concat') {
      // 简单拼接
      return validResults
        .map((r) => `[${r.modality.toUpperCase()}]\n${r.content}`)
        .join('\n\n---\n\n');
    }

    if (this.config.strategy === 'weighted') {
      // 加权融合
      const weights = this.config.weights;
      const parts = validResults.map((r) => {
        const weight = (weights as Record<string, number | undefined>)[r.modality] ?? 0.33;
        return `[${r.modality} (weight: ${weight})]\n${r.content}`;
      });
      return parts.join('\n\n');
    }

    // hierarchical: 使用 LLM 进行层级融合
    const modalitySummaries = validResults
      .map((r) => `## ${r.modality.toUpperCase()} Analysis (confidence: ${(r.confidence * 100).toFixed(0)}%)\n${r.content}`)
      .join('\n\n');

    const fusionPrompt = originalText
      ? `Original question: ${originalText}\n\nHere are analyses from different modalities:\n\n${modalitySummaries}\n\nPlease synthesize these analyses into a coherent, comprehensive response that addresses the original question.`
      : `Here are analyses from different modalities:\n\n${modalitySummaries}\n\nPlease synthesize these analyses into a coherent, comprehensive response.`;

    try {
      const resp = await fusionProvider.complete({
        messages: [
          {
            role: 'system',
            content: `${this.config.systemPromptPrefix}You are a multimodal fusion assistant. Your task is to synthesize information from multiple modalities (vision, audio, text) into a unified understanding. Consider the confidence of each modality and resolve any conflicts.`,
          },
          { role: 'user', content: fusionPrompt },
        ],
      });
      return resp.content;
    } catch {
      // 融合失败时退回到拼接模式
      return validResults
        .map((r) => `[${r.modality.toUpperCase()}]\n${r.content}`)
        .join('\n\n---\n\n');
    }
  }
}

// ===== 多模态 Agent 工厂 =====

/**
 * 创建多模态融合 Agent。
 *
 * 便捷工厂函数，封装常用的配置组合。
 */
export function createMultimodalAgent(config: MultimodalFusionConfig): MultimodalFusion {
  return new MultimodalFusion(config);
}
