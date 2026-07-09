// Google Gemini Provider for TypeScript SDK
// API docs: https://ai.google.dev/api/rest/v1beta/models/generateContent

import type { ProviderConfig, CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, Chunk, ModelInfo, Message, ToolCall } from '../types.js';
import type { Provider } from './provider.js';
import { APIError } from './openai.js';

const DEFAULT_BASE_URL = 'https://generativelanguage.googleapis.com';
const DEFAULT_TIMEOUT = 120_000;
const USER_AGENT = 'AgentPrimordia-TS/1.0';

interface GeminiPart {
  text?: string;
  functionCall?: {
    name: string;
    args?: Record<string, unknown>;
  };
  functionResponse?: {
    name: string;
    response: { content: string };
  };
}

interface GeminiContent {
  role: 'user' | 'model';
  parts: GeminiPart[];
}

interface GeminiResponse {
  candidates: Array<{
    content: { parts: GeminiPart[]; role: string };
    finishReason?: string;
  }>;
  usageMetadata: {
    promptTokenCount: number;
    candidatesTokenCount: number;
    totalTokenCount: number;
  };
}

export class GeminiProvider implements Provider {
  private config: Required<ProviderConfig>;

  constructor(config: ProviderConfig) {
    if (!config.apiKey) {
      throw new Error('Gemini API key is required');
    }
    this.config = {
      apiKey: config.apiKey,
      baseURL: (config.baseURL ?? DEFAULT_BASE_URL).replace(/\/+$/, ''),
      model: config.model ?? 'gemini-2.0-flash',
      temperature: config.temperature ?? 0,
      maxTokens: config.maxTokens ?? 8192,
    };
  }

  async complete(req: CompletionRequest): Promise<CompletionResponse> {
    const { contents, systemInstruction } = this.buildContents(req.messages);
    const body: Record<string, unknown> = { contents };

    if (systemInstruction) {
      body.systemInstruction = { parts: [{ text: systemInstruction }] };
    }

    const generationConfig: Record<string, unknown> = {};
    const temp = req.temperature ?? this.config.temperature;
    if (temp && temp > 0) {
      generationConfig.temperature = temp;
    }
    const maxTok = req.maxTokens ?? this.config.maxTokens;
    if (maxTok > 0) {
      generationConfig.maxOutputTokens = maxTok;
    }
    if (Object.keys(generationConfig).length > 0) {
      body.generationConfig = generationConfig;
    }

    const model = req.model ?? this.config.model;
    const raw = await this.doRequest(model, ':generateContent', body);
    const resp = JSON.parse(raw) as GeminiResponse;

    const content = resp.candidates?.[0]?.content?.parts?.[0]?.text ?? '';

    return {
      id: `gemini-${resp.usageMetadata?.totalTokenCount ?? 0}`,
      content,
      role: 'assistant',
      usage: {
        promptTokens: resp.usageMetadata?.promptTokenCount ?? 0,
        completionTokens: resp.usageMetadata?.candidatesTokenCount ?? 0,
        totalTokens: resp.usageMetadata?.totalTokenCount ?? 0,
      },
    };
  }

  async *stream(req: CompletionRequest): AsyncIterable<Chunk> {
    const { contents, systemInstruction } = this.buildContents(req.messages);
    const body: Record<string, unknown> = { contents, stream: true };

    if (systemInstruction) {
      body.systemInstruction = { parts: [{ text: systemInstruction }] };
    }

    const temp = req.temperature ?? this.config.temperature;
    if (temp && temp > 0) {
      body.generationConfig = { temperature: temp };
    }

    const model = req.model ?? this.config.model;
    const url = `${this.config.baseURL}/v1beta/models/${model}:streamGenerateContent?alt=sse&key=${this.config.apiKey}`;

    const resp = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'User-Agent': USER_AGENT },
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
          const event = JSON.parse(data) as GeminiResponse;
          const text = event.candidates?.[0]?.content?.parts?.[0]?.text;
          if (text) {
            yield { content: text, done: false };
          }
        } catch {
          continue;
        }
      }
    }
    yield { content: '', done: true };
  }

  async callTools(req: ToolCallRequest): Promise<ToolCallResponse> {
    const { contents, systemInstruction } = this.buildContents(req.messages);
    const body: Record<string, unknown> = {
      contents,
      tools: [{
        functionDeclarations: req.tools.map((t) => ({
          name: t.function.name,
          description: t.function.description,
          parameters: t.function.parameters,
        })),
      }],
    };

    if (systemInstruction) {
      body.systemInstruction = { parts: [{ text: systemInstruction }] };
    }

    const model = req.model ?? this.config.model;
    const raw = await this.doRequest(model, ':generateContent', body);
    const resp = JSON.parse(raw) as GeminiResponse;

    const parts = resp.candidates?.[0]?.content?.parts ?? [];
    const textPart = parts.find((p) => p.text);
    const toolParts = parts.filter((p) => p.functionCall);

    const toolCalls: ToolCall[] = toolParts.map((p) => ({
      id: `call-${Math.random().toString(36).slice(2)}`,
      name: p.functionCall!.name,
      arguments: JSON.stringify(p.functionCall!.args ?? {}),
    }));

    return {
      content: textPart?.text ?? '',
      toolCalls,
      usage: {
        promptTokens: resp.usageMetadata?.promptTokenCount ?? 0,
        completionTokens: resp.usageMetadata?.candidatesTokenCount ?? 0,
        totalTokens: resp.usageMetadata?.totalTokenCount ?? 0,
      },
    };
  }

  info(): ModelInfo {
    return {
      name: this.config.model,
      provider: 'gemini',
      maxContext: 1_048_576,
      supportsTools: true,
      supportsStreaming: true,
    };
  }

  // ===== Internal helpers =====

  private buildContents(messages: Message[]): { contents: GeminiContent[]; systemInstruction: string } {
    let systemInstruction = '';
    const contents: GeminiContent[] = [];

    for (const msg of messages) {
      if (msg.role === 'system') {
        systemInstruction += (systemInstruction ? '\n' : '') + msg.content;
        continue;
      }
      if (msg.role === 'tool') {
        // Convert tool results to Gemini's functionResponse format
        contents.push({
          role: 'user',
          parts: [{ functionResponse: { name: msg.name ?? 'tool', response: { content: msg.content } } }],
        });
        continue;
      }
      if (msg.role === 'assistant' && msg.toolCalls && msg.toolCalls.length > 0) {
        // Convert assistant tool_calls to Gemini's functionCall parts
        const parts: GeminiPart[] = [];
        if (msg.content) {
          parts.push({ text: msg.content });
        }
        for (const tc of msg.toolCalls) {
          let args: Record<string, unknown> = {};
          try { args = JSON.parse(tc.arguments); } catch {}
          parts.push({ functionCall: { name: tc.name, args } });
        }
        contents.push({ role: 'model', parts });
        continue;
      }
      contents.push({
        role: msg.role === 'assistant' ? 'model' : 'user',
        parts: [{ text: msg.content }],
      });
    }

    return { contents, systemInstruction };
  }

  private async doRequest(model: string, action: string, body: Record<string, unknown>): Promise<string> {
    const url = `${this.config.baseURL}/v1beta/models/${model}${action}?key=${this.config.apiKey}`;
    const resp = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'User-Agent': USER_AGENT },
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
        return new APIError(parsed.error.message, parsed.error.code ?? '', parsed.error.status ?? '', status);
      }
    } catch {}
    return new APIError(`Gemini API returned HTTP ${status}: ${body}`, '', '', status);
  }
}
