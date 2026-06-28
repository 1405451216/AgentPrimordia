import type { Provider } from './provider.js';
import type { CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, Chunk, ModelInfo, Usage } from '../types.js';
import { OpenAIProvider } from './openai.js';

// ===== Multimodal Types =====

export type MultimodalCapability = 'text' | 'vision' | 'audio' | 'video';

export interface MultimodalContent {
  type: 'text' | 'image_url' | 'image_b64' | 'audio' | 'video';
  text?: string;
  imageUrl?: string;
  imageB64?: string;
  mimeType?: string;
  audioData?: string;
  videoData?: string;
}

export interface MultimodalMessage {
  role: string;
  content: MultimodalContent[];
}

export interface MultimodalRequest {
  messages: MultimodalMessage[];
  model?: string;
  temperature?: number;
  maxTokens?: number;
}

export interface MultimodalResponse {
  id: string;
  content: string;
  role: string;
  usage: Usage;
}

// ===== Multimodal Provider Interface =====

export interface MultimodalProvider extends Provider {
  capabilities: MultimodalCapability[];
  completeMultimodal(req: MultimodalRequest): Promise<MultimodalResponse>;
}

// ===== Multimodal Adapter =====
// Wraps standard providers to add multimodal support

export class MultimodalAdapter implements MultimodalProvider {
  private inner: Provider;
  private _capabilities: MultimodalCapability[];

  constructor(inner: Provider, capabilities: MultimodalCapability[] = ['text', 'vision']) {
    this.inner = inner;
    this._capabilities = capabilities;
  }

  get capabilities(): MultimodalCapability[] { return this._capabilities; }

  async completeMultimodal(req: MultimodalRequest): Promise<MultimodalResponse> {
    // Convert multimodal messages to standard format
    const messages = req.messages.map((m) => {
      const textParts = m.content.filter((c) => c.type === 'text').map((c) => c.text).join('');
      return { role: m.role as 'system' | 'user' | 'assistant' | 'tool', content: textParts };
    });

    const resp = await this.inner.complete({
      messages,
      model: req.model,
      temperature: req.temperature,
      maxTokens: req.maxTokens,
    });

    return {
      id: resp.id,
      content: resp.content,
      role: resp.role,
      usage: resp.usage,
    };
  }

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    return this.inner.complete(req);
  }

  async *stream(req: CompletionRequest): AsyncIterable<Chunk> {
    if (this.inner.stream) {
      yield* this.inner.stream(req);
    } else {
      const resp = await this.inner.complete(req);
      yield { content: resp.content, done: true, usage: resp.usage };
    }
  }

  async callTools(req: ToolCallRequest): Promise<ToolCallResponse> {
    return this.inner.callTools(req);
  }

  info(): ModelInfo {
    return this.inner.info();
  }
}

// ===== Content Part Builders =====

export function textContent(text: string): MultimodalContent {
  return { type: 'text', text };
}

export function imageUrlContent(url: string): MultimodalContent {
  return { type: 'image_url', imageUrl: url };
}

export function imageB64Content(b64: string, mimeType: string = 'image/png'): MultimodalContent {
  return { type: 'image_b64', imageB64: b64, mimeType };
}

export function audioContent(data: string, mimeType: string = 'audio/wav'): MultimodalContent {
  return { type: 'audio', audioData: data, mimeType };
}

export function videoContent(data: string, mimeType: string = 'video/mp4'): MultimodalContent {
  return { type: 'video', videoData: data, mimeType };
}

// ===== OpenAI Multimodal Provider =====
// Full multimodal support for OpenAI GPT-4o models

export class OpenAIMultimodalProvider extends MultimodalAdapter {
  private _openaiConfig: { apiKey: string; baseURL?: string; model?: string };

  constructor(config: { apiKey: string; baseURL?: string; model?: string }) {
    super(new OpenAIProvider(config), ['text', 'vision']);
    this._openaiConfig = config;
  }

  async completeMultimodal(req: MultimodalRequest): Promise<MultimodalResponse> {
    const baseURL = (this._openaiConfig.baseURL ?? 'https://api.openai.com/v1').replace(/\/+$/, '');
    const model = req.model ?? this._openaiConfig.model ?? 'gpt-4o';

    // Build OpenAI multimodal message format
    const messages = req.messages.map((m) => {
      const content = m.content.map((c) => {
        if (c.type === 'text') return { type: 'text', text: c.text };
        if (c.type === 'image_url') return { type: 'image_url', image_url: { url: c.imageUrl } };
        if (c.type === 'image_b64') return { type: 'image_url', image_url: { url: `data:${c.mimeType};base64,${c.imageB64}` } };
        return { type: 'text', text: '' };
      });
      return { role: m.role, content };
    });

    const resp = await fetch(`${baseURL}/chat/completions`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${this._openaiConfig.apiKey}`,
      },
      body: JSON.stringify({
        model,
        messages,
        temperature: req.temperature ?? 0,
        ...(req.maxTokens ? { max_tokens: req.maxTokens } : {}),
      }),
      signal: AbortSignal.timeout(120_000),
    });

    const text = await resp.text();
    if (!resp.ok) throw new Error(`OpenAI multimodal error: ${resp.status}: ${text}`);
    const parsed = JSON.parse(text);
    const choice = parsed.choices?.[0];

    return {
      id: parsed.id,
      content: choice?.message?.content ?? '',
      role: choice?.message?.role ?? 'assistant',
      usage: {
        promptTokens: parsed.usage?.prompt_tokens ?? 0,
        completionTokens: parsed.usage?.completion_tokens ?? 0,
        totalTokens: parsed.usage?.total_tokens ?? 0,
      },
    };
  }
}
