// Anthropic Claude Provider for TypeScript SDK
// API docs: https://docs.anthropic.com/en/api/messages

import type { ProviderConfig, CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, Chunk, ModelInfo, Message, ToolCall } from '../types.js';
import type { Provider } from './provider.js';
import { APIError } from './openai.js';

const DEFAULT_BASE_URL = 'https://api.anthropic.com';
const DEFAULT_TIMEOUT = 120_000;
const API_VERSION = '2023-06-01';
const USER_AGENT = 'AgentPrimordia-TS/1.0';

interface AnthropicContentBlock {
  type: 'text' | 'tool_use';
  text?: string;
  id?: string;
  name?: string;
  input?: unknown;
}

interface AnthropicMessage {
  role: 'user' | 'assistant';
  content: string | AnthropicContentBlock[];
}

interface AnthropicResponse {
  id: string;
  type: string;
  role: string;
  content: AnthropicContentBlock[];
  model: string;
  stop_reason?: string;
  usage: {
    input_tokens: number;
    output_tokens: number;
  };
}

export class AnthropicProvider implements Provider {
  private config: Required<ProviderConfig>;

  constructor(config: ProviderConfig) {
    if (!config.apiKey) {
      throw new Error('Anthropic API key is required');
    }
    this.config = {
      apiKey: config.apiKey,
      baseURL: (config.baseURL ?? DEFAULT_BASE_URL).replace(/\/+$/, ''),
      model: config.model ?? 'claude-sonnet-4-20250514',
      temperature: config.temperature ?? 0,
      maxTokens: config.maxTokens ?? 4096,
    };
  }

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    const { messages, system } = this.convertMessages(req.messages);
    const body: Record<string, unknown> = {
      model: req.model ?? this.config.model,
      messages,
      max_tokens: req.maxTokens ?? this.config.maxTokens,
    };
    if (system) {
      body.system = system;
    }
    const temp = req.temperature ?? this.config.temperature;
    if (temp) {
      body.temperature = temp;
    }

    const raw = await this.doRequest('/v1/messages', body);
    const resp = JSON.parse(raw) as AnthropicResponse;

    const content = resp.content.find((b) => b.type === 'text')?.text ?? '';

    return {
      id: resp.id,
      content,
      role: 'assistant',
      usage: {
        promptTokens: resp.usage.input_tokens,
        completionTokens: resp.usage.output_tokens,
        totalTokens: resp.usage.input_tokens + resp.usage.output_tokens,
      },
    };
  }

  async *stream(req: CompletionRequest): AsyncIterable<Chunk> {
    const { messages, system } = this.convertMessages(req.messages);
    const body: Record<string, unknown> = {
      model: req.model ?? this.config.model,
      messages,
      max_tokens: req.maxTokens ?? this.config.maxTokens,
      stream: true,
    };
    if (system) {
      body.system = system;
    }
    const temp = req.temperature ?? this.config.temperature;
    if (temp) {
      body.temperature = temp;
    }

    const resp = await fetch(this.config.baseURL + '/v1/messages', {
      method: 'POST',
      headers: this.headers(),
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(DEFAULT_TIMEOUT),
    });

    if (!resp.ok) {
      const text = await resp.text();
      throw this.parseError(resp.status, text);
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
        if (!trimmed.startsWith('data: ')) continue;
        const data = trimmed.slice(6);

        try {
          const event = JSON.parse(data);
          if (event.type === 'content_block_delta' && event.delta?.text) {
            yield { content: event.delta.text, done: false };
          } else if (event.type === 'message_stop') {
            yield { content: '', done: true };
            return;
          }
        } catch {
          continue;
        }
      }
    }
  }

  async callTools(req: ToolCallRequest): Promise<ToolCallResponse> {
    const { messages, system } = this.convertMessages(req.messages);
    const body: Record<string, unknown> = {
      model: req.model ?? this.config.model,
      messages,
      max_tokens: req.maxTokens ?? this.config.maxTokens,
      tools: req.tools.map((t) => ({
        name: t.function.name,
        description: t.function.description,
        input_schema: t.function.parameters,
      })),
    };
    if (system) {
      body.system = system;
    }

    const raw = await this.doRequest('/v1/messages', body);
    const resp = JSON.parse(raw) as AnthropicResponse;

    const textBlock = resp.content.find((b) => b.type === 'text');
    const toolUseBlocks = resp.content.filter((b) => b.type === 'tool_use');

    const toolCalls: ToolCall[] = toolUseBlocks.map((b) => ({
      id: b.id ?? '',
      name: b.name ?? '',
      arguments: JSON.stringify(b.input ?? {}),
    }));

    return {
      content: textBlock?.text ?? '',
      toolCalls,
      usage: {
        promptTokens: resp.usage.input_tokens,
        completionTokens: resp.usage.output_tokens,
        totalTokens: resp.usage.input_tokens + resp.usage.output_tokens,
      },
    };
  }

  info(): ModelInfo {
    return {
      name: this.config.model,
      provider: 'anthropic',
      maxContext: 200_000,
      supportsTools: true,
      supportsStreaming: true,
    };
  }

  // ===== Internal helpers =====

  private headers(): Record<string, string> {
    return {
      'Content-Type': 'application/json',
      'x-api-key': this.config.apiKey,
      'anthropic-version': API_VERSION,
      'User-Agent': USER_AGENT,
    };
  }

  private convertMessages(messages: Message[]): { messages: AnthropicMessage[]; system: string } {
    let system = '';
    const converted: AnthropicMessage[] = [];

    for (const msg of messages) {
      if (msg.role === 'system') {
        system += (system ? '\n' : '') + msg.content;
        continue;
      }
      if (msg.role === 'tool') {
        // Convert tool results to user messages with tool_result content
        converted.push({
          role: 'user',
          content: [{
            type: 'text',
            text: `[Tool Result: ${msg.name}]\n${msg.content}`,
          }],
        });
        continue;
      }
      converted.push({
        role: msg.role as 'user' | 'assistant',
        content: msg.content,
      });
    }

    return { messages: converted, system };
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

  private parseError(status: number, body: string): Error {
    try {
      const parsed = JSON.parse(body);
      if (parsed.error?.message) {
        return new APIError(parsed.error.message, parsed.error.code ?? '', parsed.error.type ?? '', status);
      }
    } catch {}
    return new APIError(`Anthropic API returned HTTP ${status}: ${body}`, '', '', status);
  }
}
