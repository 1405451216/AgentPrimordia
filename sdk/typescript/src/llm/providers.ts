import type { ProviderConfig, CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, Chunk, ModelInfo } from '../types.js';
import type { Provider } from './provider.js';
import { APIError, OpenAIProvider } from './openai.js';

/**
 * Base class for OpenAI-compatible providers.
 * Most providers (DeepSeek, Qwen, GLM, etc.) use OpenAI-compatible API format.
 */
export abstract class OpenAICompatibleProvider implements Provider {
  protected config: Required<ProviderConfig>;
  protected abstract readonly defaultBaseURL: string;
  protected abstract readonly providerName: string;

  constructor(config: ProviderConfig, defaultModel: string, baseURL?: string) {
    if (!config.apiKey?.trim()) throw new Error('API key is required');
    this.config = {
      apiKey: config.apiKey,
      baseURL: (config.baseURL ?? baseURL ?? '').replace(/\/+$/, ''),
      model: config.model ?? defaultModel,
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
      signal: AbortSignal.timeout(120_000),
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
        if (data === '[DONE]') { yield { content: '', done: true }; return; }
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
        } catch { continue; }
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
    if (parsed.error) throw this.parseError(0, raw);
    const choice = parsed.choices?.[0];
    if (!choice) throw new Error('empty choices in response');

    return {
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
  }

  info(): ModelInfo {
    return {
      name: this.config.model,
      provider: this.providerName,
      maxContext: this.getMaxContext(),
      supportsTools: this.supportsTools(),
      supportsStreaming: true,
    };
  }

  protected abstract getMaxContext(): number;
  protected supportsTools(): boolean { return true; }

  protected headers(): Record<string, string> {
    return {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${this.config.apiKey}`,
      'User-Agent': 'AgentPrimordia-TS/1.0',
    };
  }

  protected buildBody(req: CompletionRequest): Record<string, unknown> {
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

  protected async doRequest(path: string, body: Record<string, unknown>): Promise<string> {
    const resp = await fetch(this.config.baseURL + path, {
      method: 'POST',
      headers: this.headers(),
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(120_000),
    });
    const text = await resp.text();
    if (!resp.ok) throw this.parseError(resp.status, text);
    return text;
  }

  protected parseResponse(raw: string): CompletionResponse {
    const parsed = JSON.parse(raw);
    if (parsed.error) throw this.parseError(0, raw);
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

  protected parseError(status: number, body: string): Error {
    try {
      const parsed = JSON.parse(body);
      if (parsed.error?.message) {
        return new APIError(parsed.error.message, parsed.error.code ?? '', parsed.error.type ?? '', status);
      }
    } catch {}
    return new APIError(`API returned HTTP ${status}: ${body}`, '', '', status);
  }
}

// ===== DeepSeek Provider =====

export class DeepSeekProvider extends OpenAICompatibleProvider {
  protected readonly defaultBaseURL = 'https://api.deepseek.com/v1';
  protected readonly providerName = 'deepseek';

  constructor(config: ProviderConfig) {
    super(config, 'deepseek-chat', 'https://api.deepseek.com/v1');
  }

  protected getMaxContext(): number { return 64_000; }
}

// ===== Qwen (通义千问 / DashScope) Provider =====

export class QwenProvider extends OpenAICompatibleProvider {
  protected readonly defaultBaseURL = 'https://dashscope.aliyuncs.com/compatible-mode/v1';
  protected readonly providerName = 'qwen';

  constructor(config: ProviderConfig) {
    super(config, 'qwen-plus', 'https://dashscope.aliyuncs.com/compatible-mode/v1');
  }

  protected getMaxContext(): number { return 128_000; }
}

// ===== GLM (智谱) Provider =====

export class GLMProvider extends OpenAICompatibleProvider {
  protected readonly defaultBaseURL = 'https://open.bigmodel.cn/api/paas/v4';
  protected readonly providerName = 'glm';

  constructor(config: ProviderConfig) {
    super(config, 'glm-4-flash', 'https://open.bigmodel.cn/api/paas/v4');
  }

  protected getMaxContext(): number { return 128_000; }
  // Note: GLM's OpenAI-compatible layer has limited tool_calls support
  protected supportsTools(): boolean { return false; }

  async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
    throw new Error('GLM provider does not support tool calls via OpenAI-compatible API. Use OpenAI/Anthropic/Gemini/Qwen instead.');
  }
}

// ===== Mistral Provider =====

export class MistralProvider extends OpenAICompatibleProvider {
  protected readonly defaultBaseURL = 'https://api.mistral.ai/v1';
  protected readonly providerName = 'mistral';

  constructor(config: ProviderConfig) {
    super(config, 'mistral-large-latest', 'https://api.mistral.ai/v1');
  }

  protected getMaxContext(): number { return 32_000; }
}

// ===== Cohere Provider =====

export class CohereProvider implements Provider {
  private config: Required<ProviderConfig>;

  constructor(config: ProviderConfig) {
    if (!config.apiKey?.trim()) throw new Error('Cohere: API key is required');
    this.config = {
      apiKey: config.apiKey,
      baseURL: (config.baseURL ?? 'https://api.cohere.com/v2').replace(/\/+$/, ''),
      model: config.model ?? 'command-r-plus',
      temperature: config.temperature ?? 0,
      maxTokens: config.maxTokens ?? 0,
    };
  }

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    const body = this.buildBody(req);
    const raw = await this.doRequest('/chat', body);
    return this.parseResponse(raw);
  }

  async *stream(req: CompletionRequest): AsyncIterable<Chunk> {
    const body = { ...this.buildBody(req), stream: true };
    const resp = await fetch(this.config.baseURL + '/chat', {
      method: 'POST',
      headers: this.headers(),
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(120_000),
    });
    if (!resp.ok) throw new APIError(`Cohere API error: ${resp.status}`, '', '', resp.status);
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
          const parsed = JSON.parse(trimmed);
          if (parsed.type === 'content-delta' && parsed.delta?.message?.content?.text) {
            yield { content: parsed.delta.message.content.text, done: false };
          }
          if (parsed.type === 'message-end') {
            yield { content: '', done: true, usage: parsed.delta?.usage ? {
              promptTokens: parsed.delta.usage.billed_units?.input_tokens ?? 0,
              completionTokens: parsed.delta.usage.billed_units?.output_tokens ?? 0,
              totalTokens: (parsed.delta.usage.billed_units?.input_tokens ?? 0) + (parsed.delta.usage.billed_units?.output_tokens ?? 0),
            } : undefined };
          }
        } catch { continue; }
      }
    }
  }

  async callTools(req: ToolCallRequest): Promise<ToolCallResponse> {
    const body: Record<string, unknown> = {
      ...this.buildBody({ messages: req.messages }),
      tools: req.tools.map((t) => ({ type: 'function', function: t.function })),
    };
    const raw = await this.doRequest('/chat', body);
    const parsed = JSON.parse(raw);
    const msg = parsed.message ?? {};

    return {
      content: msg.content?.[0]?.text ?? '',
      toolCalls: (msg.tool_calls ?? []).map((tc: Record<string, unknown>) => ({
        id: String(tc.id ?? ''),
        name: String((tc.function as Record<string, unknown>)?.name ?? ''),
        arguments: String((tc.function as Record<string, unknown>)?.arguments ?? ''),
      })),
      usage: {
        promptTokens: parsed.usage?.billed_units?.input_tokens ?? 0,
        completionTokens: parsed.usage?.billed_units?.output_tokens ?? 0,
        totalTokens: (parsed.usage?.billed_units?.input_tokens ?? 0) + (parsed.usage?.billed_units?.output_tokens ?? 0),
      },
    };
  }

  info(): ModelInfo {
    return { name: this.config.model, provider: 'cohere', maxContext: 128_000, supportsTools: true, supportsStreaming: true };
  }

  private headers(): Record<string, string> {
    return { 'Content-Type': 'application/json', Authorization: `Bearer ${this.config.apiKey}` };
  }

  private buildBody(req: CompletionRequest): Record<string, unknown> {
    const messages = req.messages.map((m) => ({ role: m.role, content: m.content }));
    const body: Record<string, unknown> = { model: req.model ?? this.config.model, messages };
    if (req.temperature ?? this.config.temperature) body.temperature = req.temperature ?? this.config.temperature;
    if (req.maxTokens ?? this.config.maxTokens) body.max_tokens = req.maxTokens ?? this.config.maxTokens;
    return body;
  }

  private async doRequest(path: string, body: Record<string, unknown>): Promise<string> {
    const resp = await fetch(this.config.baseURL + path, {
      method: 'POST', headers: this.headers(), body: JSON.stringify(body),
      signal: AbortSignal.timeout(120_000),
    });
    const text = await resp.text();
    if (!resp.ok) throw new APIError(`Cohere API error: ${resp.status}: ${text}`, '', '', resp.status);
    return text;
  }

  private parseResponse(raw: string): CompletionResponse {
    const parsed = JSON.parse(raw);
    const msg = parsed.message ?? {};
    return {
      id: parsed.id ?? '',
      content: msg.content?.[0]?.text ?? '',
      role: msg.role ?? 'assistant',
      usage: {
        promptTokens: parsed.usage?.billed_units?.input_tokens ?? 0,
        completionTokens: parsed.usage?.billed_units?.output_tokens ?? 0,
        totalTokens: (parsed.usage?.billed_units?.input_tokens ?? 0) + (parsed.usage?.billed_units?.output_tokens ?? 0),
      },
    };
  }
}

// ===== Azure OpenAI Provider =====

export interface AzureConfig {
  apiKey: string;
  resourceName: string;   // Azure resource name
  deploymentName: string;  // Deployment name
  apiVersion?: string;     // Default: 2024-02-15-preview
  temperature?: number;
  maxTokens?: number;
}

export class AzureOpenAIProvider implements Provider {
  private config: AzureConfig;
  private apiVersion: string;

  constructor(config: AzureConfig) {
    if (!config.apiKey?.trim()) throw new Error('Azure: API key is required');
    if (!config.resourceName?.trim()) throw new Error('Azure: resource name is required');
    if (!config.deploymentName?.trim()) throw new Error('Azure: deployment name is required');
    this.config = config;
    this.apiVersion = config.apiVersion ?? '2024-02-15-preview';
  }

  private get baseURL(): string {
    return `https://${this.config.resourceName}.openai.azure.com/openai/deployments/${this.config.deploymentName}`;
  }

  private headers(): Record<string, string> {
    return { 'Content-Type': 'application/json', 'api-key': this.config.apiKey };
  }

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    const body = this.buildBody(req);
    const url = `${this.baseURL}/chat/completions?api-version=${this.apiVersion}`;
    const resp = await fetch(url, { method: 'POST', headers: this.headers(), body: JSON.stringify(body), signal: AbortSignal.timeout(120_000) });
    const text = await resp.text();
    if (!resp.ok) throw new APIError(`Azure API error: ${resp.status}: ${text}`, '', '', resp.status);
    return this.parseResponse(text);
  }

  async *stream(req: CompletionRequest): AsyncIterable<Chunk> {
    const body = { ...this.buildBody(req), stream: true };
    const url = `${this.baseURL}/chat/completions?api-version=${this.apiVersion}`;
    const resp = await fetch(url, { method: 'POST', headers: this.headers(), body: JSON.stringify(body), signal: AbortSignal.timeout(120_000) });
    if (!resp.ok) throw new APIError(`Azure API error: ${resp.status}`, '', '', resp.status);
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
        if (data === '[DONE]') { yield { content: '', done: true }; return; }
        try {
          const parsed = JSON.parse(data);
          const choice = parsed.choices?.[0];
          if (!choice) continue;
          yield { content: choice.delta?.content ?? '', done: choice.finish_reason === 'stop' };
        } catch { continue; }
      }
    }
  }

  async callTools(req: ToolCallRequest): Promise<ToolCallResponse> {
    const body: Record<string, unknown> = {
      ...this.buildBody({ messages: req.messages }),
      tools: req.tools.map((t) => ({ type: t.type, function: t.function })),
    };
    const url = `${this.baseURL}/chat/completions?api-version=${this.apiVersion}`;
    const resp = await fetch(url, { method: 'POST', headers: this.headers(), body: JSON.stringify(body), signal: AbortSignal.timeout(120_000) });
    const text = await resp.text();
    if (!resp.ok) throw new APIError(`Azure API error: ${resp.status}: ${text}`, '', '', resp.status);
    const parsed = JSON.parse(text);
    const choice = parsed.choices?.[0];
    if (!choice) throw new Error('empty choices');
    return {
      content: choice.message?.content ?? '',
      toolCalls: (choice.message?.tool_calls ?? []).map((tc: Record<string, unknown>) => ({
        id: String(tc.id ?? ''),
        name: String((tc.function as Record<string, unknown>)?.name ?? ''),
        arguments: String((tc.function as Record<string, unknown>)?.arguments ?? ''),
      })),
      usage: { promptTokens: parsed.usage?.prompt_tokens ?? 0, completionTokens: parsed.usage?.completion_tokens ?? 0, totalTokens: parsed.usage?.total_tokens ?? 0 },
    };
  }

  info(): ModelInfo {
    return { name: this.config.deploymentName, provider: 'azure', maxContext: 128_000, supportsTools: true, supportsStreaming: true };
  }

  private buildBody(req: CompletionRequest): Record<string, unknown> {
    const body: Record<string, unknown> = {
      messages: req.messages.map((m) => ({ role: m.role, content: m.content })),
    };
    if (req.temperature ?? this.config.temperature) body.temperature = req.temperature ?? this.config.temperature;
    if (req.maxTokens ?? this.config.maxTokens) body.max_tokens = req.maxTokens ?? this.config.maxTokens;
    return body;
  }

  private parseResponse(raw: string): CompletionResponse {
    const parsed = JSON.parse(raw);
    const choice = parsed.choices?.[0];
    return {
      id: parsed.id,
      content: choice?.message?.content ?? '',
      role: choice?.message?.role ?? 'assistant',
      usage: { promptTokens: parsed.usage?.prompt_tokens ?? 0, completionTokens: parsed.usage?.completion_tokens ?? 0, totalTokens: parsed.usage?.total_tokens ?? 0 },
    };
  }
}
