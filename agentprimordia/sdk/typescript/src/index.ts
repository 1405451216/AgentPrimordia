export interface CompletionRequest {
  prompt: string;
  maxTokens?: number;
}

export interface CompletionResponse {
  id: string;
  content: string;
  role: string;
  usage: Usage;
}

export interface Usage {
  promptTokens: number;
  completionTokens: number;
}

export interface ModelInfo {
  name: string;
  provider: string;
  maxContext: number;
  supportsTools: boolean;
}

export interface ToolCallRequest {
  toolName: string;
  args: Record<string, unknown>;
}

export interface ToolCallResponse {
  result: string;
  usage: Usage;
}

export interface ToolDefinition {
  name: string;
  description: string;
  parameters: Record<string, unknown>;
  execute: (args: Record<string, unknown>) => Promise<string>;
}

export interface LLMProvider {
  complete(req: CompletionRequest): Promise<CompletionResponse>;
  stream(req: CompletionRequest): AsyncIterable<Chunk>;
  callTools(req: ToolCallRequest): Promise<ToolCallResponse>;
  embeddings(texts: string[]): Promise<number[][]>;
  info(): ModelInfo;
}

export interface Chunk {
  content: string;
  done: boolean;
}

export class MockProvider implements LLMProvider {
  private response: string;

  constructor(response = "I'm a mock assistant. How can I help?") {
    this.response = response;
  }

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    return {
      id: "mock-1",
      content: this.response,
      role: "assistant",
      usage: { promptTokens: 10, completionTokens: 20 },
    };
  }

  async *stream(req: CompletionRequest): AsyncIterable<Chunk> {
    yield { content: this.response, done: true };
  }

  async callTools(req: ToolCallRequest): Promise<ToolCallResponse> {
    return {
      result: `mock result for ${req.toolName}`,
      usage: { promptTokens: 0, completionTokens: 0 },
    };
  }

  async embeddings(texts: string[]): Promise<number[][]> {
    return texts.map(() => new Array(128).fill(0));
  }

  info(): ModelInfo {
    return {
      name: "mock",
      provider: "mock",
      maxContext: 4096,
      supportsTools: true,
    };
  }
}

export interface ReActConfig {
  name: string;
  systemPrompt: string;
  model: LLMProvider;
  tools?: ToolDefinition[];
  maxTurns?: number;
}

export interface AgentResponse {
  content: string;
  turns: number;
  toolCalls: number;
}

export class ReActAgent {
  private name: string;
  private systemPrompt: string;
  private model: LLMProvider;
  private tools: ToolDefinition[];
  private maxTurns: number;

  constructor(config: ReActConfig) {
    this.name = config.name;
    this.systemPrompt = config.systemPrompt;
    this.model = config.model;
    this.tools = config.tools ?? [];
    this.maxTurns = config.maxTurns ?? 3;
  }

  async run(input: string): Promise<AgentResponse> {
    const response = await this.model.complete({
      prompt: `${this.systemPrompt}\nUser: ${input}`,
    });
    return {
      content: response.content,
      turns: 1,
      toolCalls: 0,
    };
  }
}

export interface PoolConfig {
  maxConcurrency: number;
  defaultAgent: Omit<ReActConfig, "model">;
}

export interface TaskConfig {
  id: string;
  title: string;
  prompt: string;
}

export interface TaskResult {
  taskID: string;
  task: TaskConfig;
  content: string;
  error: Error | null;
  duration: number;
}

export interface PoolStats {
  completedTasks: number;
  failedTasks: number;
  runningTasks: number;
}

export class Pool {
  private model: LLMProvider | null = null;
  private config: PoolConfig;

  constructor(config: PoolConfig) {
    this.config = config;
  }

  setModel(model: LLMProvider): void {
    this.model = model;
  }

  async dispatch(tasks: TaskConfig[]): Promise<TaskResult[]> {
    if (!this.model) {
      throw new Error("model not set");
    }
    const results: TaskResult[] = [];
    for (const task of tasks) {
      const start = Date.now();
      const agent = new ReActAgent({
        ...this.config.defaultAgent,
        model: this.model,
      });
      const resp = await agent.run(task.prompt);
      results.push({
        taskID: task.id,
        task,
        content: resp.content,
        error: null,
        duration: Date.now() - start,
      });
    }
    return results;
  }

  stats(): PoolStats {
    return { completedTasks: 0, failedTasks: 0, runningTasks: 0 };
  }

  close(): void {}
}

export const VERSION = "0.7.0";
