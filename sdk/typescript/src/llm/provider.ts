import type {
  CompletionRequest,
  CompletionResponse,
  ToolCallRequest,
  ToolCallResponse,
  Chunk,
  ModelInfo,
  ProviderConfig,
} from '../types.js';

export interface Provider {
  complete(req: CompletionRequest): Promise<CompletionResponse>;
  stream?(req: CompletionRequest): AsyncIterable<Chunk>;
  callTools(req: ToolCallRequest): Promise<ToolCallResponse>;
  embeddings?(texts: string[]): Promise<number[][]>;
  info(): ModelInfo;
}

export class MockProvider implements Provider {
  private response: string;
  private toolCalls: import('../types.js').ToolCall[];
  private shouldError: boolean;
  private delay: number;

  constructor(opts?: { response?: string; toolCalls?: import('../types.js').ToolCall[]; error?: boolean; delay?: number }) {
    this.response = opts?.response ?? 'mock response';
    this.toolCalls = opts?.toolCalls ?? [];
    this.shouldError = opts?.error ?? false;
    this.delay = opts?.delay ?? 0;
  }

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
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

  async *stream(req: CompletionRequest): AsyncIterable<Chunk> {
    if (this.shouldError) {
      throw new Error('mock error');
    }
    const words = this.response.split(' ');
    for (let i = 0; i < words.length; i++) {
      yield { content: words[i] + ' ', done: i === words.length - 1 };
    }
  }

  async callTools(req: ToolCallRequest): Promise<ToolCallResponse> {
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
