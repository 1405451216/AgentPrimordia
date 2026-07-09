// Ollama Provider for TypeScript SDK
// API docs: https://github.com/ollama/ollama/blob/main/docs/api.md
// Ollama runs locally and doesn't require an API key.

import type { ProviderConfig, CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, Chunk, ModelInfo, Message, ToolCall } from '../types.js';
import type { Provider } from './provider.js';

const DEFAULT_BASE_URL = 'http://localhost:11434';
const DEFAULT_TIMEOUT = 300_000; // Local models can be slower
const USER_AGENT = 'AgentPrimordia-TS/1.0';

interface OllamaMessage {
  role: string;
  content: string;
  tool_calls?: Array<{ id: string; type: 'function'; function: { name: string; arguments: string } }>;
  tool_call_id?: string;
  name?: string;
}

interface OllamaTool {
  type: string;
  function: {
    name: string;
    description: string;
    parameters: Record<string, unknown>;
  };
}

interface OllamaChatResponse {
  model: string;
  message: {
    role: string;
    content: string;
    tool_calls?: Array<{
      function: { name: string; arguments: Record<string, unknown> };
    }>;
  };
  done: boolean;
  total_duration?: number;
  prompt_eval_count?: number;
  eval_count?: number;
}

export class OllamaProvider implements Provider {
  private config: Required<Pick<ProviderConfig, 'baseURL' | 'model' | 'temperature' | 'maxTokens'>> & { apiKey: string };

  constructor(config: ProviderConfig) {
    this.config = {
      apiKey: config.apiKey ?? '', // Ollama doesn't need an API key
      baseURL: (config.baseURL ?? DEFAULT_BASE_URL).replace(/\/+$/, ''),
      model: config.model ?? 'llama3',
      temperature: config.temperature ?? 0,
      maxTokens: config.maxTokens ?? 0,
    };
  }

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    const body: Record<string, unknown> = {
      model: req.model ?? this.config.model,
      messages: this.buildMessages(req.messages),
      stream: false,
    };

    const options: Record<string, unknown> = {};
    const temp = req.temperature ?? this.config.temperature;
    if (temp && temp > 0) {
      options.temperature = temp;
    }
    const maxTok = req.maxTokens ?? this.config.maxTokens;
    if (maxTok > 0) {
      options.num_predict = maxTok;
    }
    if (Object.keys(options).length > 0) {
      body.options = options;
    }

    const raw = await this.doRequest('/api/chat', body);
    const resp = JSON.parse(raw) as OllamaChatResponse;

    return {
      id: `ollama-${Date.now()}`,
      content: resp.message.content,
      role: 'assistant',
      usage: {
        promptTokens: resp.prompt_eval_count ?? 0,
        completionTokens: resp.eval_count ?? 0,
        totalTokens: (resp.prompt_eval_count ?? 0) + (resp.eval_count ?? 0),
      },
    };
  }

  async *stream(req: CompletionRequest): AsyncIterable<Chunk> {
    const body: Record<string, unknown> = {
      model: req.model ?? this.config.model,
      messages: this.buildMessages(req.messages),
      stream: true,
    };

    const options: Record<string, unknown> = {};
    const temp = req.temperature ?? this.config.temperature;
    if (temp && temp > 0) {
      options.temperature = temp;
    }
    if (Object.keys(options).length > 0) {
      body.options = options;
    }

    const resp = await fetch(this.config.baseURL + '/api/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'User-Agent': USER_AGENT },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(DEFAULT_TIMEOUT),
    });

    if (!resp.ok) {
      const text = await resp.text();
      throw new Error(`Ollama API returned HTTP ${resp.status}: ${text}`);
    }

    if (!resp.body) throw new Error('Response body is null');

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop()!;

      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed) continue;

        try {
          const chunk = JSON.parse(trimmed) as OllamaChatResponse;
          if (chunk.message?.content) {
            yield { content: chunk.message.content, done: false };
          }
          if (chunk.done) {
            yield { content: '', done: true };
            return;
          }
        } catch {
          continue;
        }
      }
    }
    yield { content: '', done: true };
  }

  async callTools(req: ToolCallRequest): Promise<ToolCallResponse> {
    const tools: OllamaTool[] = req.tools.map((t) => ({
      type: t.type,
      function: {
        name: t.function.name,
        description: t.function.description,
        parameters: t.function.parameters,
      },
    }));

    const body: Record<string, unknown> = {
      model: req.model ?? this.config.model,
      messages: this.buildMessages(req.messages),
      tools,
      stream: false,
    };

    const raw = await this.doRequest('/api/chat', body);
    const resp = JSON.parse(raw) as OllamaChatResponse;

    const toolCalls: ToolCall[] = (resp.message.tool_calls ?? []).map((tc) => ({
      id: `call-${Math.random().toString(36).slice(2)}`,
      name: tc.function.name,
      arguments: JSON.stringify(tc.function.arguments ?? {}),
    }));

    return {
      content: resp.message.content,
      toolCalls,
      usage: {
        promptTokens: resp.prompt_eval_count ?? 0,
        completionTokens: resp.eval_count ?? 0,
        totalTokens: (resp.prompt_eval_count ?? 0) + (resp.eval_count ?? 0),
      },
    };
  }

  info(): ModelInfo {
    return {
      name: this.config.model,
      provider: 'ollama',
      maxContext: 8192,
      supportsTools: true,
      supportsStreaming: true,
    };
  }

  // ===== Internal helpers =====

  private buildMessages(messages: Message[]): OllamaMessage[] {
    return messages.map((msg) => {
      const m: OllamaMessage = { role: msg.role, content: msg.content };
      if (msg.role === 'assistant' && msg.toolCalls && msg.toolCalls.length > 0) {
        m.tool_calls = msg.toolCalls.map((tc) => ({ id: tc.id, type: 'function', function: { name: tc.name, arguments: tc.arguments } }));
      }
      if (msg.role === 'tool' && msg.toolCallId) {
        m.tool_call_id = msg.toolCallId;
      }
      if (msg.name) {
        m.name = msg.name;
      }
      return m;
    });
  }

  private async doRequest(path: string, body: Record<string, unknown>): Promise<string> {
    const resp = await fetch(this.config.baseURL + path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'User-Agent': USER_AGENT },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(DEFAULT_TIMEOUT),
    });

    const text = await resp.text();
    if (!resp.ok) {
      throw new Error(`Ollama API returned HTTP ${resp.status}: ${text}`);
    }
    return text;
  }
}
