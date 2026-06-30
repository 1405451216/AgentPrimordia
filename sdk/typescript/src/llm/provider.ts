import type {
  CompletionRequest,
  CompletionResponse,
  ToolCallRequest,
  ToolCallResponse,
  Chunk,
  ModelInfo,
  ToolCall,
} from '../types.js';

/** LLM Provider 接口，与 Go 端 llm.Provider 对齐。
 *
 * 核心方法：
 * - complete: 非流式补全
 * - stream: 流式补全（可选）
 * - callTools: 工具调用（Function Calling）
 * - embeddings: 文本向量化（可选）
 * - info: 返回模型信息
 */
export interface Provider {
  complete(req: CompletionRequest): Promise<CompletionResponse>;
  stream?(req: CompletionRequest): AsyncIterable<Chunk>;
  callTools(req: ToolCallRequest): Promise<ToolCallResponse>;
  embeddings?(texts: string[]): Promise<number[][]>;
  info(): ModelInfo;
}

/** Mock Provider，用于测试和开发。
 *
 * 支持配置：
 * - response: 模拟返回内容
 * - toolCalls: 模拟工具调用
 * - error: 是否模拟错误
 * - delay: 模拟延迟（毫秒）
 */
export class MockProvider implements Provider {
  private response: string;
  private toolCalls: ToolCall[];
  private shouldError: boolean;
  private delay: number;

  constructor(opts?: { response?: string; toolCalls?: ToolCall[]; error?: boolean; delay?: number }) {
    this.response = opts?.response ?? 'mock response';
    this.toolCalls = opts?.toolCalls ?? [];
    this.shouldError = opts?.error ?? false;
    this.delay = opts?.delay ?? 0;
  }

  async complete(_req: CompletionRequest): Promise<CompletionResponse> {
    if (this.delay > 0) {
      await new Promise((r) => setTimeout(r, this.delay));
    }
    if (this.shouldError) {
      throw new Error('mock error');
    }
    return {
      id: 'mock-' + Math.random().toString(36).slice(2),
      content: this.response,
      role: 'assistant',
      usage: { promptTokens: 10, completionTokens: 5, totalTokens: 15 },
    };
  }

  async *stream(_req: CompletionRequest): AsyncIterable<Chunk> {
    if (this.shouldError) {
      throw new Error('mock error');
    }
    const words = this.response.split(' ');
    for (let i = 0; i < words.length; i++) {
      yield { content: words[i] + ' ', done: i === words.length - 1 };
    }
  }

  async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
    if (this.shouldError) {
      throw new Error('mock error');
    }
    return {
      content: this.response,
      toolCalls: this.toolCalls,
      usage: { promptTokens: 20, completionTokens: 10, totalTokens: 30 },
    };
  }

  info(): ModelInfo {
    return {
      name: 'mock-model',
      provider: 'mock',
      maxContext: 4096,
      supportsTools: true,
      supportsStreaming: true,
    };
  }
}
