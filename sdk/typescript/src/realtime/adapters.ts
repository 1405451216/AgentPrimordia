// Realtime adapters — 真实 ASR/TTS HTTP 适配器（v4.1 双语言对齐）
//
// 客户端形状与 Go internal/agent/realtime 的 OpenAIASR / OpenAITTS 对齐：
//   - OpenAIASR: POST multipart（file + model）→ {"text": "..."}
//   - OpenAITTS: POST JSON {model, input, voice} → 音频字节
//
// 端点兼容 OpenAI Whisper / Audio Speech API，亦兼容本地
// faster-whisper / Piper 等 OpenAI 兼容服务（免 Key）。

export interface ASRAdapter {
  transcribe(audio: Uint8Array): Promise<string>;
  readonly name: string;
}

export interface TTSAdapter {
  synthesize(text: string): Promise<Uint8Array>;
  readonly name: string;
}

export interface OpenAIASROptions {
  /** 转写模型名（默认 whisper-1；本地服务可忽略） */
  model?: string;
  /** 请求超时（默认 60s） */
  timeoutMs?: number;
}

/** OpenAI Whisper 兼容 ASR 适配器（multipart 音频 → 文本）。 */
export class OpenAIASR implements ASRAdapter {
  readonly name = 'openai-asr';
  private readonly url: string;
  private readonly apiKey: string;
  private readonly model: string;
  private readonly timeoutMs: number;

  constructor(url: string, apiKey = '', opts: OpenAIASROptions = {}) {
    this.url = url;
    this.apiKey = apiKey;
    this.model = opts.model ?? 'whisper-1';
    this.timeoutMs = opts.timeoutMs ?? 60_000;
  }

  async transcribe(audio: Uint8Array): Promise<string> {
    if (audio.length === 0) throw new Error('realtime: 空音频数据');

    const form = new FormData();
    form.set('model', this.model);
    form.set('file', new Blob([audio as BlobPart]), 'audio.bin');

    const headers: Record<string, string> = {};
    if (this.apiKey) headers['Authorization'] = `Bearer ${this.apiKey}`;

    const res = await fetchWithTimeout(this.url, {
      method: 'POST',
      headers,
      body: form,
      timeoutMs: this.timeoutMs,
    });
    if (!res.ok) {
      throw new Error(`realtime: asr endpoint ${res.status}: ${await res.text()}`);
    }
    const data = (await res.json()) as { text?: string };
    return data.text ?? '';
  }
}

export interface OpenAITTSOptions {
  /** 合成模型名（默认 tts-1；本地服务可忽略） */
  model?: string;
  /** 发音人（默认 alloy） */
  voice?: string;
  /** 请求超时（默认 60s） */
  timeoutMs?: number;
}

/** OpenAI TTS 兼容 TTS 适配器（文本 → 音频字节）。 */
export class OpenAITTS implements TTSAdapter {
  readonly name = 'openai-tts';
  private readonly url: string;
  private readonly apiKey: string;
  private readonly model: string;
  private readonly voice: string;
  private readonly timeoutMs: number;

  constructor(url: string, apiKey = '', opts: OpenAITTSOptions = {}) {
    this.url = url;
    this.apiKey = apiKey;
    this.model = opts.model ?? 'tts-1';
    this.voice = opts.voice ?? 'alloy';
    this.timeoutMs = opts.timeoutMs ?? 60_000;
  }

  async synthesize(text: string): Promise<Uint8Array> {
    if (!text) throw new Error('realtime: 空文本');

    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (this.apiKey) headers['Authorization'] = `Bearer ${this.apiKey}`;

    const res = await fetchWithTimeout(this.url, {
      method: 'POST',
      headers,
      body: JSON.stringify({ model: this.model, input: text, voice: this.voice }),
      timeoutMs: this.timeoutMs,
    });
    if (!res.ok) {
      throw new Error(`realtime: tts endpoint ${res.status}: ${await res.text()}`);
    }
    return new Uint8Array(await res.arrayBuffer());
  }
}

async function fetchWithTimeout(
  input: string,
  init: RequestInit & { timeoutMs: number },
): Promise<Response> {
  const { timeoutMs, ...rest } = init;
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), timeoutMs);
  try {
    return await fetch(input, { ...rest, signal: ctrl.signal });
  } finally {
    clearTimeout(timer);
  }
}
