/**
 * PrivacyRouter — 隐私优先混合推理路由层。
 *
 * v3.0 方向6：隐私优先混合推理（PII → 本地 WebGPU）
 *
 * 核心逻辑：
 * 1. 检测输入中的 PII（个人身份信息）
 * 2. 如果包含 PII 且本地 WebGPU 可用 → 路由到本地推理
 * 3. 如果不包含 PII → 路由到远程 API（成本更低、质量更高）
 * 4. 如果包含 PII 但本地不可用 → 脱敏后发送远程
 *
 * 支持的 PII 类型：
 * - 邮箱、电话、身份证号、银行卡号、IP 地址
 */

import type { Message, CompletionResponse, ModelInfo } from '../types.js';
import { WebGPUProvider } from './webgpu-provider.js';

// ===== PII 检测 =====

export type PIIType =
  | 'email'
  | 'phone'
  | 'id_card'
  | 'bank_card'
  | 'ip_address'
  | 'ssn';

export interface PIIDetection {
  type: PIIType;
  value: string;
  start: number;
  end: number;
}

/** PII 正则模式 */
const PII_PATTERNS: Record<PIIType, RegExp> = {
  email: /[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/g,
  phone: /(?:\+?86)?1[3-9]\d{9}/g,
  id_card: /\d{17}[\dXx]/g,
  bank_card: /\b\d{16,19}\b/g,
  ip_address: /\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b/g,
  ssn: /\b\d{3}-\d{2}-\d{4}\b/g,
};

/**
 * 检测文本中的 PII。
 */
export function detectPII(text: string): PIIDetection[] {
  const results: PIIDetection[] = [];

  for (const [type, pattern] of Object.entries(PII_PATTERNS)) {
    const regex = new RegExp(pattern.source, pattern.flags);
    let match;
    while ((match = regex.exec(text)) !== null) {
      results.push({
        type: type as PIIType,
        value: match[0],
        start: match.index,
        end: match.index + match[0].length,
      });
    }
  }

  // 按位置排序
  results.sort((a, b) => a.start - b.start);
  return results;
}

/**
 * 检测文本是否包含 PII。
 */
export function hasPII(text: string): boolean {
  return detectPII(text).length > 0;
}

// ===== PII 脱敏 =====

/**
 * 脱敏 PII（将敏感信息替换为占位符）。
 */
export function redactPII(text: string): { redacted: string; detections: PIIDetection[] } {
  const detections = detectPII(text);
  if (detections.length === 0) {
    return { redacted: text, detections };
  }

  // 从后往前替换，避免偏移
  let result = text;
  for (let i = detections.length - 1; i >= 0; i--) {
    const d = detections[i];
    const placeholder = getPlaceholder(d.type);
    result = result.slice(0, d.start) + placeholder + result.slice(d.end);
  }

  return { redacted: result, detections };
}

function getPlaceholder(type: PIIType): string {
  switch (type) {
    case 'email': return '[EMAIL]';
    case 'phone': return '[PHONE]';
    case 'id_card': return '[ID_CARD]';
    case 'bank_card': return '[BANK_CARD]';
    case 'ip_address': return '[IP]';
    case 'ssn': return '[SSN]';
    default: return '[REDACTED]';
  }
}

// ===== 隐私优先路由 =====

export type RoutingDecision = 'local' | 'remote' | 'redacted_remote';

export interface RoutingResult {
  decision: RoutingDecision;
  reason: string;
  piiDetected: PIIDetection[];
  /** 处理后的输入（如果是 redacted_remote，则为脱敏后的输入） */
  processedInput: string;
}

export interface PrivacyRouterConfig {
  /** 是否启用本地推理 */
  enableLocal: boolean;
  /** 是否允许脱敏后发送远程 */
  allowRedactedRemote: boolean;
  /** 本地推理 Provider（WebGPU） */
  localProvider?: WebGPUProvider;
  /** 远程 Provider */
  remoteProvider?: any;
  /** 默认本地模型 */
  localModel?: string;
  /** 是否在路由决策时输出日志 */
  enableLogging?: boolean;
}

/**
 * PrivacyRouter 隐私优先混合推理路由器。
 *
 * 路由策略：
 * 1. 输入包含 PII + 本地可用 → 本地推理（数据不出域）
 * 2. 输入包含 PII + 本部不可用 + 允许脱敏 → 脱敏后远程
 * 3. 输入包含 PII + 本地不可用 + 不允许脱敏 → 拒绝
 * 4. 输入不包含 PII → 直接远程
 */
export class PrivacyRouter {
  private config: PrivacyRouterConfig;
  private localAvailable = false;

  constructor(config: PrivacyRouterConfig) {
    this.config = {
      ...config,
      enableLocal: config.enableLocal ?? true,
      allowRedactedRemote: config.allowRedactedRemote ?? true,
    };
  }

  /**
   * 初始化路由器（探测本地推理可用性）。
   */
  async initialize(): Promise<boolean> {
    if (!this.config.enableLocal || !this.config.localProvider) {
      this.localAvailable = false;
      return false;
    }

    try {
      const caps = await this.config.localProvider.initialize();
      this.localAvailable = caps.supported;
      return this.localAvailable;
    } catch {
      this.localAvailable = false;
      return false;
    }
  }

  /**
   * 路由决策。
   */
  route(input: string): RoutingResult {
    const detections = detectPII(input);

    if (detections.length === 0) {
      // 无 PII，直接远程
      return {
        decision: 'remote',
        reason: 'No PII detected, routing to remote',
        piiDetected: [],
        processedInput: input,
      };
    }

    // 检测到 PII
    if (this.config.enableLocal && this.localAvailable) {
      // 本地推理可用，数据不出域
      return {
        decision: 'local',
        reason: `PII detected (${detections.length} items), routing to local WebGPU`,
        piiDetected: detections,
        processedInput: input,
      };
    }

    // 本地不可用
    if (this.config.allowRedactedRemote) {
      // 脱敏后发送远程
      const { redacted } = redactPII(input);
      return {
        decision: 'redacted_remote',
        reason: `PII detected but local unavailable, redacted ${detections.length} items`,
        piiDetected: detections,
        processedInput: redacted,
      };
    }

    // 不允许脱敏，拒绝请求
    return {
      decision: 'local',
      reason: 'PII detected, local unavailable, redaction not allowed',
      piiDetected: detections,
      processedInput: input,
    };
  }

  /**
   * 执行推理（根据路由决策选择 Provider）。
   */
  async complete(messages: Message[]): Promise<CompletionResponse> {
    // 拼接消息文本用于 PII 检测
    const input = messages.map(m => typeof m.content === 'string' ? m.content : '').join(' ');

    const routing = this.route(input);

    if (routing.decision === 'remote') {
      // 直接远程
      if (!this.config.remoteProvider) {
        throw new Error('Remote provider not configured');
      }
      return this.config.remoteProvider.complete(messages);
    }

    if (routing.decision === 'local') {
      // 本地推理
      if (!this.config.localProvider) {
        throw new Error('Local provider not configured');
      }

      // 确保模型已加载
      if (!this.config.localProvider.isModelLoaded()) {
        const model = this.config.localModel ?? 'automatic/llama-3-8b';
        await this.config.localProvider.loadModel(model);
      }

      const response = await this.config.localProvider.chat(messages);
      return {
        id: `local-${Date.now()}`,
        content: response.content,
        role: 'assistant' as const,
        usage: response.usage,
      } as CompletionResponse;
    }

    // redacted_remote：脱敏后发送远程
    const redactedMessages = messages.map(m => {
      if (typeof m.content === 'string') {
        const { redacted } = redactPII(m.content);
        return { ...m, content: redacted };
      }
      return m;
    });

    if (!this.config.remoteProvider) {
      throw new Error('Remote provider not configured');
    }
    return this.config.remoteProvider.complete(redactedMessages);
  }

  /**
   * 流式推理（根据路由决策选择 Provider）。
   */
  async *stream(messages: Message[]): AsyncGenerator<{ content: string; done: boolean }> {
    const input = messages.map(m => typeof m.content === 'string' ? m.content : '').join(' ');
    const routing = this.route(input);

    if (routing.decision === 'remote') {
      if (!this.config.remoteProvider?.stream) {
        throw new Error('Remote provider does not support streaming');
      }
      yield* this.config.remoteProvider.stream(messages);
      return;
    }

    if (routing.decision === 'local') {
      if (!this.config.localProvider) {
        throw new Error('Local provider not configured');
      }
      if (!this.config.localProvider.isModelLoaded()) {
        const model = this.config.localModel ?? 'automatic/llama-3-8b';
        await this.config.localProvider.loadModel(model);
      }
      yield* this.config.localProvider.stream(messages);
      return;
    }

    // redacted_remote
    const redactedMessages = messages.map(m => {
      if (typeof m.content === 'string') {
        const { redacted } = redactPII(m.content);
        return { ...m, content: redacted };
      }
      return m;
    });

    if (!this.config.remoteProvider?.stream) {
      throw new Error('Remote provider does not support streaming');
    }
    yield* this.config.remoteProvider.stream(redactedMessages);
  }

  /**
   * 获取路由统计信息。
   */
  getStats(): {
    localAvailable: boolean;
    enableLocal: boolean;
    allowRedactedRemote: boolean;
  } {
    return {
      localAvailable: this.localAvailable,
      enableLocal: this.config.enableLocal,
      allowRedactedRemote: this.config.allowRedactedRemote,
    };
  }
}
