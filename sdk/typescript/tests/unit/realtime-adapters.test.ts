import { describe, it, expect, vi, afterEach, type Mock } from 'vitest';
import { OpenAIASR, OpenAITTS } from '../../src/realtime/adapters.js';

// 与 Go internal/agent/realtime/{asr,tts}_test.go 行为对齐的客户端形状测试。

function mockFetchJson(body: unknown, status = 200): Mock {
  return vi.fn().mockResolvedValue(new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  }));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('OpenAIASR (v4.1, mirrors Go asr.go)', () => {
  it('transcribes via multipart with model + auth', async () => {
    const fetchMock = mockFetchJson({ text: '你好，世界' });
    vi.stubGlobal('fetch', fetchMock);

    const asr = new OpenAIASR('https://api.example.com/v1/audio/transcriptions', 'sk-test', { model: 'whisper-1' });
    const text = await asr.transcribe(new Uint8Array([1, 2, 3]));

    expect(text).toBe('你好，世界');
    expect(asr.name).toBe('openai-asr');
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('https://api.example.com/v1/audio/transcriptions');
    expect((init!.headers as Record<string, string>)['Authorization']).toBe('Bearer sk-test');
    const form = init!.body as FormData;
    expect(form.get('model')).toBe('whisper-1');
    expect(form.get('file')).toBeInstanceOf(Blob);
  });

  it('omits Authorization when keyless (local faster-whisper)', async () => {
    const fetchMock = mockFetchJson({ text: 'ok' });
    vi.stubGlobal('fetch', fetchMock);

    const asr = new OpenAIASR('http://127.0.0.1:9000/v1/audio/transcriptions');
    await asr.transcribe(new Uint8Array([1]));
    const init = fetchMock.mock.calls[0][1];
    expect((init!.headers as Record<string, string>)['Authorization']).toBeUndefined();
  });

  it('rejects empty audio', async () => {
    vi.stubGlobal('fetch', mockFetchJson({ text: 'x' }));
    const asr = new OpenAIASR('http://localhost');
    await expect(asr.transcribe(new Uint8Array(0))).rejects.toThrow('空音频');
  });

  it('surfaces non-2xx status + body', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('invalid api key', { status: 401 })));
    const asr = new OpenAIASR('http://localhost', 'bad');
    await expect(asr.transcribe(new Uint8Array([1]))).rejects.toThrow(/401/);
  });
});

describe('OpenAITTS (v4.1, mirrors Go tts.go)', () => {
  it('synthesizes JSON request to audio bytes with voice', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(new Uint8Array([1, 2, 3]), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    const tts = new OpenAITTS('https://api.example.com/v1/audio/speech', 'sk-test', { voice: 'nova' });
    const audio = await tts.synthesize('你好，世界');

    expect(Array.from(audio)).toEqual([1, 2, 3]);
    expect(tts.name).toBe('openai-tts');
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('https://api.example.com/v1/audio/speech');
    const body = JSON.parse(String(init!.body));
    expect(body.input).toBe('你好，世界');
    expect(body.voice).toBe('nova');
  });

  it('rejects empty text', async () => {
    vi.stubGlobal('fetch', vi.fn());
    const tts = new OpenAITTS('http://localhost');
    await expect(tts.synthesize('')).rejects.toThrow('空文本');
  });

  it('surfaces non-2xx status + body', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('insufficient quota', { status: 402 })));
    const tts = new OpenAITTS('http://localhost', 'k');
    await expect(tts.synthesize('hi')).rejects.toThrow(/402/);
  });
});
