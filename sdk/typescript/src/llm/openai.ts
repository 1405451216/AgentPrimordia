import type { ProviderConfig, CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, Chunk, ModelInfo } from '../types.js';
import type { Provider } from './provider.js';

export class APIError extends Error {
  code: string;
  type: string;
  status: number;

  constructor(message: string, code: string, type: string, status: number) {
    super(message);
    this.name = 'APIError';
    this.code = code;
    this.type = type;
    this.status = status;
  }
}

const DEFAULT_BASE_URL = 'https://api.openai.com/v1';
const DEFAULT_TIMEOUT = 120_000;
const USER_AGENT = 'AgentPrimordia-TS/1.0';

export class OpenAIProvider implements Provider {
  private config: Required<ProviderConfig>;

  constructor(config: ProviderConfig) {
    if (!config.apiKey) {
      throw new Error('API key is required');
    }
    this.config = {
      apiKey: config.apiKey,
      baseURL: (config.baseURL ?? DEFAULT_BASE_URL).replace(/\/+$/, ''),
      model: config.model ?? 'gpt-4o-mini',
      temperature: config.temperature ?? 0,
      maxTokens: config.maxTokens ?? 0,
    };
  }

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    const body = this.buildBody(req);
    const raw = await this.doRequest('/chat/completions', body);
    return this.parseResponse(raw);
  }

  async *stream(req: CompletionRequest): AsyncIterable<Chunk> {
    const body = { ...this.buildBody(req), stream: true };
    const resp = await fetch(this.config.baseURL + '/chat/completions', {
      method: 'POST',
      headers: this.headers(),
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(DEFAULT_TIMEOUT),
    });

    if (!resp.ok) {
      const text = await resp.text();
      throw this.parseError(resp.status, text);
    }

    if (!resp.body) {
      throw new Error('Response body is null');
    }
    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    const STREAM_READ_TIMEOUT = 60_000;

    while (true) {
      const readPromise = reader.read();
      const timeoutPromise = new Promise<never>((_, reject) =>
        setTimeout(() => reject(new Error('Stream read timeout')), STREAM_READ_TIMEOUT)
      );
      const { done, value } = await Promise.race([readPromise, timeoutPromise]);
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop()!;

      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed.startsWith('data: ')) continue;
        const data = trimmed.slice(6);
        if (data === '[DONE]') {
          yield { content: '', done: true };
          return;
        }

        try {
          const parsed = JSON.parse(data);
          const choice = parsed.choices?.[0];
          if (!choice) continue;

          const chunk: Chunk = {
            content: choice.delta?.content ?? '',
            done: choice.finish_reason === 'stop',
          };
          if (chunk.done && parsed.usage) {
            chunk.usage = {
              promptTokens: parsed.usage.prompt_tokens,
              completionTokens: parsed.usage.completion_tokens,
              totalTokens: parsed.usage.total_tokens,
            };
          }
          yield chunk;
        } catch (err) {
          console.error(`[AgentPrimordia] Failed to parse stream chunk: ${data}`, err);
          continue;
        }
      }
    }
  }

  async callTools(req: ToolCallRequest): Promise<ToolCallResponse> {
    const body: Record<string, unknown> = {
      ...this.buildBody({ messages: req.messages }),
      tools: req.tools.map((t) => ({ type: t.type, function: t.function })),
    };

    const raw = await this.doRequest('/chat/completions', body);
    const parsed = JSON.parse(raw);

    if (parsed.error) {
      throw new APIError(parsed.error.message, parsed.error.code ?? '', parsed.error.type ?? '', 0);
    }

    const choice = parsed.choices?.[0];
    if (!choice) throw new Error('empty choices in response');

    const result: ToolCallResponse = {
      content: choice.message?.content ?? '',
      toolCalls: (choice.message?.tool_calls ?? []).map((tc: Record<string, unknown>) => ({
        id: String(tc.id ?? ''),
        name: String((tc.function as Record<string, unknown>)?.name ?? ''),
        arguments: String((tc.function as Record<string, unknown>)?.arguments ?? ''),
      })),
      usage: {
        promptTokens: parsed.usage?.prompt_tokens ?? 0,
        completionTokens: parsed.usage?.completion_tokens ?? 0,
        totalTokens: parsed.usage?.total_tokens ?? 0,
      },
    };

    return result;
  }

  info(): ModelInfo {
    return {
      name: this.config.model,
      provider: 'openai',
      maxContext: 128_000,
      supportsTools: true,
      supportsStreaming: true,
    };
  }

  private headers(): Record<string, string> {
    return {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${this.config.apiKey}`,
      'User-Agent': USER_AGENT,
    };
  }

  private buildBody(req: CompletionRequest): Record<string, unknown> {
    const body: Record<string, unknown> = {
      model: req.model ?? this.config.model,
      messages: req.messages.map((m) => ({ role: m.role, content: m.content })),
    };
    if (req.temperature ?? this.config.temperature) {
      body.temperature = req.temperature ?? this.config.temperature;
    }
    if (req.maxTokens ?? this.config.maxTokens) {
      body.max_tokens = req.maxTokens ?? this.config.maxTokens;
    }
    return body;
  }

  private async doRequest(path: string, body: Record<string, unknown>): Promise<string> {
    const resp = await fetch(this.config.baseURL + path, {
      method: 'POST',
      headers: this.headers(),
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(DEFAULT_TIMEOUT),
    });

    const text = await resp.text();
    if (!resp.ok) {
      throw this.parseError(resp.status, text);
    }
    return text;
  }

  private parseResponse(raw: string): CompletionResponse {
    const parsed = JSON.parse(raw);
    if (parsed.error) {
      throw new APIError(parsed.error.message, parsed.error.code ?? '', parsed.error.type ?? '', 0);
    }
    const choice = parsed.choices?.[0];
    if (!choice) throw new Error('empty choices in response');

    return {
      id: parsed.id,
      content: choice.message?.content ?? '',
      role: choice.message?.role ?? 'assistant',
      usage: {
        promptTokens: parsed.usage?.prompt_tokens ?? 0,
        completionTokens: parsed.usage?.completion_tokens ?? 0,
        totalTokens: parsed.usage?.total_tokens ?? 0,
      },
    };
  }

  private parseError(status: number, body: string): Error {
    try {
      const parsed = JSON.parse(body);
      if (parsed.error?.message) {
        return new APIError(parsed.error.message, parsed.error.code ?? '', parsed.error.type ?? '', status);
      }
    } catch {}
    return new APIError(`API returned HTTP ${status}: ${body}`, '', '', status);
  }
}
