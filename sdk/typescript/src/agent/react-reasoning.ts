// react-reasoning.ts — LLM 推理阶段
// 包含同步推理和流式推理两种模式
// Mirrors Go internal/agent/react_reasoning.go

import type { Message, ToolCall, ToolDefinition, Usage, CompletionRequest } from '../types.js';
import type { Provider } from '../llm/provider.js';

// ===== 推理结果 =====

export interface Thought {
  /** 推理内容 */
  content: string;
  /** 工具调用列表 */
  toolCalls?: ToolCall[];
  /** Token 用量 */
  usage?: Usage;
}

// ===== 流式事件 =====

export type StreamEventType = 'token' | 'thought' | 'error' | 'tool_call' | 'done';

export interface StreamEvent {
  type: StreamEventType;
  content: string;
  toolCalls?: ToolCall[];
  usage?: Usage;
}

// ===== 推理配置 =====

export interface ReasoningConfig {
  /** LLM Provider */
  provider: Provider;
  /** 温度参数 */
  temperature?: number;
  /** 最大 Token 数 */
  maxTokens?: number;
  /** 最大重试次数 */
  maxRetries?: number;
  /** 流式回调 */
  onStream?: (event: StreamEvent) => void;
}

// ===== 推理引擎 =====

export class ReasoningEngine {
  private config: ReasoningConfig;

  constructor(config: ReasoningConfig) {
    this.config = {
      temperature: 0.7,
      maxTokens: 4096,
      maxRetries: 3,
      ...config,
    };
  }

  // ===== 同步推理 =====

  /** 非流式推理 */
  async reason(
    messages: Message[],
    toolDefs: ToolDefinition[],
  ): Promise<Thought> {
    const _startTime = Date.now();

    if (toolDefs.length > 0) {
      // 有工具定义时，先尝试工具调用
      const resp = await this.callWithRetry(messages, toolDefs);
      if (resp.toolCalls && resp.toolCalls.length > 0) {
        return {
          content: resp.content,
          toolCalls: resp.toolCalls,
          usage: resp.usage,
        };
      }

      // 工具调用返回空，回退到普通补全
      if (resp.content === '') {
        const completeResp = await this.completeWithRetry(messages);
        return {
          content: completeResp.content,
          usage: completeResp.usage,
        };
      }

      return {
        content: resp.content,
        usage: resp.usage,
      };
    }

    // 无工具定义，直接补全
    const resp = await this.completeWithRetry(messages);
    return {
      content: resp.content,
      usage: resp.usage,
    };
  }

  // ===== 流式推理 =====

  /** 流式推理 */
  async reasonStream(
    messages: Message[],
    toolDefs: ToolDefinition[],
  ): Promise<Thought> {
    const _startTime = Date.now();

    if (!this.config.provider.stream) {
      // 不支持流式，回退到同步推理
      return this.reason(messages, toolDefs);
    }

    try {
      // 流式补全
      const contentParts: string[] = [];
      const req: CompletionRequest = {
        messages: messages,
        temperature: this.config.temperature,
        maxTokens: this.config.maxTokens,
      };

      for await (const chunk of this.config.provider.stream(req)) {
        if (chunk.content) {
          contentParts.push(chunk.content);
          this.emit({ type: 'token', content: chunk.content });
        }
        if (chunk.done) break;
      }

      const content = contentParts.join('');

      // 流式完成后，如果有工具定义，尝试工具调用
      if (toolDefs.length > 0) {
        try {
          const resp = await this.callWithRetry(messages, toolDefs);
          if (resp.toolCalls && resp.toolCalls.length > 0) {
            this.emit({
              type: 'tool_call',
              content: resp.content,
              toolCalls: resp.toolCalls,
            });
            return {
              content: resp.content,
              toolCalls: resp.toolCalls,
              usage: resp.usage,
            };
          }
        } catch {
          // 工具调用失败，继续使用流式内容
        }
      }

      this.emit({ type: 'thought', content });
      return { content };

    } catch (err) {
      // 流式失败，回退到同步推理
      this.emit({
        type: 'error',
        content: `Stream failed: ${err instanceof Error ? err.message : String(err)}, falling back to sync`,
      });

      return this.reason(messages, toolDefs);
    }
  }

  // ===== 内部方法 =====

  /** 带重试的工具调用 */
  private async callWithRetry(
    messages: Message[],
    toolDefs: ToolDefinition[],
  ): Promise<{ content: string; toolCalls?: ToolCall[]; usage: Usage }> {
    let lastErr: Error | null = null;

    for (let attempt = 0; attempt < this.config.maxRetries!; attempt++) {
      try {
        const resp = await this.config.provider.callTools({
          messages: messages,
          tools: toolDefs,
          temperature: this.config.temperature,
          maxTokens: this.config.maxTokens,
        });
        return resp;
      } catch (err) {
        lastErr = err instanceof Error ? err : new Error(String(err));
        if (attempt < this.config.maxRetries! - 1) {
          await this.sleep(Math.pow(2, attempt) * 100); // 指数退避
        }
      }
    }

    throw lastErr ?? new Error('callTools failed after retries');
  }

  /** 带重试的普通补全 */
  private async completeWithRetry(
    messages: Message[],
  ): Promise<{ content: string; usage: Usage }> {
    let lastErr: Error | null = null;

    for (let attempt = 0; attempt < this.config.maxRetries!; attempt++) {
      try {
        const resp = await this.config.provider.complete({
          messages: messages,
          temperature: this.config.temperature,
          maxTokens: this.config.maxTokens,
        });
        return { content: resp.content, usage: resp.usage };
      } catch (err) {
        lastErr = err instanceof Error ? err : new Error(String(err));
        if (attempt < this.config.maxRetries! - 1) {
          await this.sleep(Math.pow(2, attempt) * 100);
        }
      }
    }

    throw lastErr ?? new Error('complete failed after retries');
  }

  /** 发送流式事件 */
  private emit(event: StreamEvent): void {
    this.config.onStream?.(event);
  }

  /** 延迟 */
  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}

// ===== 单轮推理（独立函数，不依赖完整 Agent） =====

/** 单轮非流式推理 */
export async function singleRoundReasoning(
  provider: Provider,
  messages: Message[],
  toolDefs: ToolDefinition[] = [],
  opts?: { temperature?: number; maxTokens?: number },
): Promise<Thought> {
  const engine = new ReasoningEngine({
    provider,
    temperature: opts?.temperature,
    maxTokens: opts?.maxTokens,
  });
  return engine.reason(messages, toolDefs);
}

/** 单轮流式推理 */
export async function singleRoundReasoningStream(
  provider: Provider,
  messages: Message[],
  toolDefs: ToolDefinition[] = [],
  opts?: {
    temperature?: number;
    maxTokens?: number;
    onStream?: (event: StreamEvent) => void;
  },
): Promise<Thought> {
  const engine = new ReasoningEngine({
    provider,
    temperature: opts?.temperature,
    maxTokens: opts?.maxTokens,
    onStream: opts?.onStream,
  });
  return engine.reasonStream(messages, toolDefs);
}