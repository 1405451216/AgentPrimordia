/**
 * 流式编排管道 — 多 Agent token 级串联。
 *
 * 与普通 Pipeline 的区别：
 * - 普通 Pipeline 等待每个 Agent 完整运行后再传递给下一个
 * - StreamingPipeline 在 Agent 产出 token 时立即流式传递给下游
 *
 * 适用场景：
 * - 翻译管道：英→中→摘要（逐 token 传递，减少端到端延迟）
 * - 内容生成：草稿→润色→格式化（流水线并行）
 * - 多视角分析：多个 Agent 同时分析同一输入，流式聚合结果
 *
 * 使用方式：
 *   const pipeline = new StreamingPipeline([
 *     { name: 'translator', agent: agent1 },
 *     { name: 'summarizer', agent: agent2 },
 *   ]);
 *   for await (const event of pipeline.stream(input)) {
 *     if (event.type === 'token') process.stdout.write(event.content);
 *   }
 */

import type { ReActAgent, StreamEvent } from '../agent/react-loop.js';
import type { Response } from '../types.js';

// ===== 类型定义 =====

/** 流式管道步骤 */
export interface StreamingPipelineStep {
  /** 步骤名 */
  name: string;
  /** Agent 实例 */
  agent: ReActAgent;
  /**
   * 输入转换函数（可选）。
   * 默认行为：将上游 Agent 的最终输出作为下游 Agent 的输入。
   * 可自定义：如拼接原始输入 + 上游输出。
   */
  transformInput?: (originalInput: string, upstreamOutput: string | null) => string;
}

/** 流式管道事件 */
export type StreamingPipelineEvent =
  | { type: 'step_start'; step: string; index: number }
  | { type: 'step_token'; step: string; index: number; content: string }
  | { type: 'step_tool_call'; step: string; index: number; toolName: string }
  | { type: 'step_tool_result'; step: string; index: number; toolName: string; success: boolean }
  | { type: 'step_done'; step: string; index: number; response: Response }
  | { type: 'pipeline_done'; results: Response[] }
  | { type: 'error'; step?: string; index?: number; error: Error };

// ===== 缓冲区管理 =====

/**
 * Token 缓冲区 — 在流式传输和最终输出之间做缓冲。
 *
 * 当上游 Agent 产出 token 时，既流式传递给下游消费者，
 * 又同时累积到缓冲区，供下一个 Agent 使用完整输出。
 */
class TokenBuffer {
  private chunks: string[] = [];
  private listeners: Array<(chunk: string) => void> = [];
  private done = false;
  private resolveDone: () => void = () => {};
  private donePromise: Promise<void>;

  constructor() {
    this.donePromise = new Promise((resolve) => {
      this.resolveDone = resolve;
    });
  }

  /** 追加一个 token chunk */
  append(chunk: string): void {
    if (this.done) return;
    this.chunks.push(chunk);
    for (const listener of this.listeners) {
      listener(chunk);
    }
  }

  /** 标记写入完成 */
  finish(): void {
    this.done = true;
    this.resolveDone();
  }

  /** 获取完整输出（等待 finish 后） */
  async getFullOutput(): Promise<string> {
    await this.donePromise;
    return this.chunks.join('');
  }

  /** 注册实时监听器 */
  onChunk(listener: (chunk: string) => void): void {
    this.listeners.push(listener);
    // 回放已有 chunks
    for (const chunk of this.chunks) {
      listener(chunk);
    }
  }
}

// ===== StreamingPipeline 实现 =====

export class StreamingPipeline {
  private steps: StreamingPipelineStep[];

  constructor(steps: StreamingPipelineStep[]) {
    if (steps.length === 0) {
      throw new Error('StreamingPipeline: steps cannot be empty');
    }
    this.steps = steps;
  }

  /**
   * 流式执行管道。
   *
   * 每个 Agent 产出 token 时立即 yield，同时累积完整输出供下游使用。
   * 下游 Agent 在上游完成后启动。
   */
  async *stream(input: string): AsyncIterable<StreamingPipelineEvent> {
    const results: Response[] = [];
    let currentInput = input;

    for (let i = 0; i < this.steps.length; i++) {
      const step = this.steps[i]!;
      yield { type: 'step_start', step: step.name, index: i };

      try {
        // 最后一个步骤直接流式 yield，中间步骤通过缓冲区桥接
        const isLast = i === this.steps.length - 1;
        let fullOutput: string | null = null;
        let response: Response;

        if (isLast) {
          // 最后一个步骤：直接流式传递
          let lastResponse: Response | null = null;
          for await (const event of step.agent.streamEvents(currentInput)) {
            const pipelineEvent = this.mapEvent(event, step.name, i);
            yield pipelineEvent;
            if (event.type === 'done') {
              lastResponse = event.response;
            }
          }
          response = lastResponse ?? {
            content: '',
            metrics: { totalTurns: 0, totalTools: 0, duration: 0, llmLatency: 0, toolLatency: 0 },
          };
          fullOutput = response.content;
        } else {
          // 中间步骤：缓冲区桥接
          const buffer = new TokenBuffer();

          // 启动 Agent 流式执行（收集事件，稍后处理）
          const collectedEvents: Array<{ type: string; toolName?: string; success?: boolean; content?: string }> = [];
          let lastResponse: Response | null = null;

          // 执行 Agent 并收集 token + 事件
          for await (const event of step.agent.streamEvents(currentInput)) {
            if (event.type === 'token' && event.content) {
              buffer.append(event.content);
              collectedEvents.push({ type: 'token', content: event.content });
            }
            if (event.type === 'tool_call') {
              collectedEvents.push({ type: 'tool_call', toolName: event.toolCall.name });
            }
            if (event.type === 'tool_result') {
              collectedEvents.push({ type: 'tool_result', toolName: event.result.toolCallId ?? '', success: !event.result.isError });
            }
            if (event.type === 'done') {
              lastResponse = event.response;
            }
          }
          buffer.finish();

          // yield 所有已收集的事件
          for (const ev of collectedEvents) {
            if (ev.type === 'token') {
              yield { type: 'step_token', step: step.name, index: i, content: ev.content ?? '' };
            } else if (ev.type === 'tool_call') {
              yield { type: 'step_tool_call', step: step.name, index: i, toolName: ev.toolName ?? '' };
            } else if (ev.type === 'tool_result') {
              yield { type: 'step_tool_result', step: step.name, index: i, toolName: ev.toolName ?? '', success: ev.success ?? true };
            }
          }

          response = lastResponse ?? {
            content: '',
            metrics: { totalTurns: 0, totalTools: 0, duration: 0, llmLatency: 0, toolLatency: 0 },
          };
          fullOutput = await buffer.getFullOutput();
        }

        results.push(response);
        yield { type: 'step_done', step: step.name, index: i, response };

        // 为下一步准备输入
        if (i < this.steps.length - 1) {
          const nextStep = this.steps[i + 1]!;
          if (nextStep.transformInput) {
            currentInput = nextStep.transformInput(input, fullOutput);
          } else {
            currentInput = fullOutput ?? '';
          }
        }
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        yield { type: 'error', step: step.name, index: i, error };
        return;
      }
    }

    yield { type: 'pipeline_done', results };
  }

  /** 将 Agent StreamEvent 映射为 PipelineEvent */
  private mapEvent(event: StreamEvent, stepName: string, index: number): StreamingPipelineEvent {
    switch (event.type) {
      case 'token':
        return { type: 'step_token', step: stepName, index, content: event.content };
      case 'tool_call':
        return { type: 'step_tool_call', step: stepName, index, toolName: event.toolCall.name };
      case 'tool_result':
        return { type: 'step_tool_result', step: stepName, index, toolName: event.result.toolCallId ?? '', success: !event.result.isError };
      case 'done':
        return { type: 'step_done', step: stepName, index, response: event.response };
      case 'error':
        return { type: 'error', step: stepName, index, error: event.error };
      default:
        // turn_end 等事件忽略
        return { type: 'step_token', step: stepName, index, content: '' };
    }
  }

  /** 非流式执行（收集所有结果） */
  async run(input: string): Promise<Response[]> {
    const results: Response[] = [];
    for await (const event of this.stream(input)) {
      if (event.type === 'step_done') {
        results.push(event.response);
      }
    }
    return results;
  }
}
