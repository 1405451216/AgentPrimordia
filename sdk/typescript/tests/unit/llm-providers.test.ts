import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { MockProvider } from '../../src/llm/provider.js';
import {
  validateConfig,
  validateConfigOrThrow,
  configFromEnv,
  configFromEnvValidated,
  LLMConfigWatcher,
} from '../../src/llm/config.js';
import { OpenAIProvider, APIError } from '../../src/llm/openai.js';
import { AnthropicProvider } from '../../src/llm/anthropic.js';
import { GeminiProvider } from '../../src/llm/gemini.js';
import { OllamaProvider } from '../../src/llm/ollama.js';
import {
  DeepSeekProvider,
  QwenProvider,
  GLMProvider,
  MistralProvider,
  CohereProvider,
  AzureOpenAIProvider,
} from '../../src/llm/providers.js';
import type { CompletionRequest, ToolCallRequest } from '../../src/types.js';

// Helper: create a mock fetch response
function mockFetchResponse(body: unknown, ok = true, status = 200): Response {
  return {
    ok,
    status,
    text: async () => (typeof body === 'string' ? body : JSON.stringify(body)),
    json: async () => body,
    body: null,
  } as Response;
}

function mockStreamResponse(chunks: string[]): Response {
  const encoder = new TextEncoder();
  const stream = new ReadableStream({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(chunk));
      }
      controller.close();
    },
  });
  return {
    ok: true,
    status: 200,
    body: stream,
    text: async () => chunks.join(''),
    json: async () => JSON.parse(chunks.join('')),
  } as Response;
}

const sampleMessages = [
  { role: 'user' as const, content: 'Hello' },
];

const sampleTools = [
  {
    type: 'function' as const,
    function: {
      name: 'get_weather',
      description: 'Get weather',
      parameters: { type: 'object', properties: { city: { type: 'string' } } },
    },
  },
];

// ===== MockProvider Tests =====

describe('MockProvider', () => {
  it('should complete with default response', async () => {
    const provider = new MockProvider();
    const resp = await provider.complete({ messages: sampleMessages });
    expect(resp.content).toBe('mock response');
    expect(resp.role).toBe('assistant');
    expect(resp.usage.totalTokens).toBe(15);
  });

  it('should complete with custom response', async () => {
    const provider = new MockProvider({ response: 'custom reply' });
    const resp = await provider.complete({ messages: sampleMessages });
    expect(resp.content).toBe('custom reply');
  });

  it('should throw error when configured', async () => {
    const provider = new MockProvider({ error: true });
    await expect(provider.complete({ messages: sampleMessages })).rejects.toThrow('mock error');
  });

  it('should delay response when configured', async () => {
    const provider = new MockProvider({ delay: 50 });
    const start = Date.now();
    await provider.complete({ messages: sampleMessages });
    expect(Date.now() - start).toBeGreaterThanOrEqual(40);
  });

  it('should stream response word by word', async () => {
    const provider = new MockProvider({ response: 'hello world foo' });
    const chunks = [];
    for await (const chunk of provider.stream!({ messages: sampleMessages })) {
      chunks.push(chunk);
    }
    expect(chunks).toHaveLength(3);
    expect(chunks[0].content).toBe('hello ');
    expect(chunks[2].done).toBe(true);
  });

  it('should throw on stream when error configured', async () => {
    const provider = new MockProvider({ error: true });
    await expect(async () => {
      for await (const _ of provider.stream!({ messages: sampleMessages })) {
        // consume
      }
    }).rejects.toThrow('mock error');
  });

  it('should callTools', async () => {
    const toolCalls = [{ id: 'tc-1', name: 'get_weather', arguments: '{"city":"NYC"}' }];
    const provider = new MockProvider({ response: 'calling tool', toolCalls });
    const resp = await provider.callTools({ messages: sampleMessages, tools: sampleTools });
    expect(resp.content).toBe('calling tool');
    expect(resp.toolCalls).toEqual(toolCalls);
    expect(resp.usage.totalTokens).toBe(30);
  });

  it('should throw on callTools when error configured', async () => {
    const provider = new MockProvider({ error: true });
    await expect(
      provider.callTools({ messages: sampleMessages, tools: sampleTools })
    ).rejects.toThrow('mock error');
  });

  it('should return model info', () => {
    const provider = new MockProvider();
    const info = provider.info();
    expect(info.name).toBe('mock-model');
    expect(info.provider).toBe('mock');
    expect(info.maxContext).toBe(4096);
    expect(info.supportsTools).toBe(true);
    expect(info.supportsStreaming).toBe(true);
  });
});

// ===== Config Tests =====

describe('Config Validation', () => {
  it('should validate valid config', () => {
    const errs = validateConfig({ apiKey: 'key', model: 'gpt-4' });
    expect(errs).toHaveLength(0);
  });

  it('should report missing model', () => {
    const errs = validateConfig({ apiKey: 'key' });
    expect(errs).toContain('model is required');
  });

  it('should report invalid temperature', () => {
    const errs = validateConfig({ apiKey: 'key', model: 'gpt-4', temperature: -1 });
    expect(errs.some((e) => e.includes('temperature'))).toBe(true);
  });

  it('should report temperature > 3', () => {
    const errs = validateConfig({ apiKey: 'key', model: 'gpt-4', temperature: 4 });
    expect(errs.some((e) => e.includes('temperature'))).toBe(true);
  });

  it('should report negative maxTokens', () => {
    const errs = validateConfig({ apiKey: 'key', model: 'gpt-4', maxTokens: -1 });
    expect(errs.some((e) => e.includes('max_tokens'))).toBe(true);
  });

  it('should report invalid baseURL', () => {
    const errs = validateConfig({ apiKey: 'key', model: 'gpt-4', baseURL: 'ftp://bad' });
    expect(errs.some((e) => e.includes('base_url'))).toBe(true);
  });

  it('should accept valid baseURL', () => {
    const errs = validateConfig({ apiKey: 'key', model: 'gpt-4', baseURL: 'https://api.openai.com' });
    expect(errs).toHaveLength(0);
  });

  it('should throw on invalid config', () => {
    expect(() => validateConfigOrThrow({ apiKey: 'key' })).toThrow('config validation failed');
  });

  it('should not throw on valid config', () => {
    expect(() => validateConfigOrThrow({ apiKey: 'key', model: 'gpt-4' })).not.toThrow();
  });
});

describe('Config from Environment', () => {
  const origEnv = { ...process.env };

  afterEach(() => {
    process.env = { ...origEnv };
  });

  it('should load config from env', () => {
    process.env.AP_LLM_API_KEY = 'env-key';
    process.env.AP_LLM_BASE_URL = 'https://api.example.com';
    process.env.AP_LLM_MODEL = 'gpt-4';
    process.env.AP_LLM_TEMPERATURE = '0.5';
    process.env.AP_LLM_MAX_TOKENS = '1000';

    const cfg = configFromEnv();
    expect(cfg.apiKey).toBe('env-key');
    expect(cfg.baseURL).toBe('https://api.example.com');
    expect(cfg.model).toBe('gpt-4');
    expect(cfg.temperature).toBe(0.5);
    expect(cfg.maxTokens).toBe(1000);
  });

  it('should use custom prefix', () => {
    process.env.MY_LLM_API_KEY = 'custom-key';
    process.env.MY_LLM_MODEL = 'claude-3';
    const cfg = configFromEnv('MY_LLM');
    expect(cfg.apiKey).toBe('custom-key');
    expect(cfg.model).toBe('claude-3');
  });

  it('should handle missing env vars', () => {
    delete process.env.AP_LLM_API_KEY;
    delete process.env.AP_LLM_BASE_URL;
    delete process.env.AP_LLM_MODEL;
    delete process.env.AP_LLM_TEMPERATURE;
    delete process.env.AP_LLM_MAX_TOKENS;
    const cfg = configFromEnv();
    expect(cfg.apiKey).toBe('');
    expect(cfg.model).toBeUndefined();
    expect(cfg.temperature).toBeUndefined();
    expect(cfg.maxTokens).toBeUndefined();
  });

  it('should handle invalid temperature in env', () => {
    process.env.AP_LLM_TEMPERATURE = 'not-a-number';
    const cfg = configFromEnv();
    expect(cfg.temperature).toBeUndefined();
  });

  it('should handle invalid maxTokens in env', () => {
    process.env.AP_LLM_MAX_TOKENS = 'abc';
    const cfg = configFromEnv();
    expect(cfg.maxTokens).toBeUndefined();
  });

  it('should validate config from env', () => {
    process.env.AP_LLM_API_KEY = 'key';
    process.env.AP_LLM_MODEL = 'gpt-4';
    const cfg = configFromEnvValidated();
    expect(cfg.model).toBe('gpt-4');
  });

  it('should throw on invalid env config', () => {
    process.env.AP_LLM_API_KEY = 'key';
    delete process.env.AP_LLM_MODEL;
    expect(() => configFromEnvValidated()).toThrow('config validation failed');
  });
});

describe('LLMConfigWatcher', () => {
  const origEnv = { ...process.env };

  afterEach(() => {
    process.env = { ...origEnv };
  });

  it('should get current config', () => {
    process.env.AP_LLM_API_KEY = 'key';
    process.env.AP_LLM_MODEL = 'gpt-4';
    const watcher = new LLMConfigWatcher(() => {}, { interval: 100 });
    const cfg = watcher.getConfig();
    expect(cfg.model).toBe('gpt-4');
    watcher.stop();
  });

  it('should detect config changes', async () => {
    process.env.AP_LLM_API_KEY = 'key1';
    process.env.AP_LLM_MODEL = 'gpt-4';
    let changed = false;
    const watcher = new LLMConfigWatcher(
      () => { changed = true; },
      { interval: 50 }
    );
    watcher.start();

    // Change env
    process.env.AP_LLM_MODEL = 'gpt-5';
    await new Promise((r) => setTimeout(r, 200));
    expect(changed).toBe(true);
    watcher.stop();
  });

  it('should not fire when config unchanged', async () => {
    process.env.AP_LLM_API_KEY = 'key';
    process.env.AP_LLM_MODEL = 'gpt-4';
    let changed = false;
    const watcher = new LLMConfigWatcher(
      () => { changed = true; },
      { interval: 50 }
    );
    watcher.start();
    await new Promise((r) => setTimeout(r, 150));
    expect(changed).toBe(false);
    watcher.stop();
  });

  it('should not start twice', () => {
    const watcher = new LLMConfigWatcher(() => {}, { interval: 100 });
    watcher.start();
    watcher.start(); // Should be no-op
    watcher.stop();
  });
});

// ===== OpenAI Provider Tests =====

describe('OpenAIProvider', () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  afterEach(() => {
    if (fetchSpy) fetchSpy.mockRestore();
  });

  it('should construct with default config', () => {
    const provider = new OpenAIProvider({ apiKey: 'sk-test-key-1234' });
    const info = provider.info();
    expect(info.provider).toBe('openai');
    expect(info.name).toBe('gpt-4o-mini');
    expect(info.maxContext).toBe(128_000);
  });

  it('should construct with custom config', () => {
    const provider = new OpenAIProvider({
      apiKey: 'sk-test-key-1234',
      model: 'gpt-4',
      baseURL: 'https://custom.api.com/v1/',
      temperature: 0.7,
      maxTokens: 500,
    });
    const info = provider.info();
    expect(info.name).toBe('gpt-4');
  });

  it('should throw on missing API key', () => {
    expect(() => new OpenAIProvider({ apiKey: '' })).toThrow('API key is required');
  });

  it('should throw on short API key', () => {
    expect(() => new OpenAIProvider({ apiKey: 'short' })).toThrow('too short');
  });

  it('should throw on invalid temperature', () => {
    expect(() => new OpenAIProvider({ apiKey: 'sk-test-key-1234', temperature: 3 })).toThrow('temperature');
  });

  it('should complete a request', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        id: 'chatcmpl-1',
        choices: [{ message: { role: 'assistant', content: 'Hello there!' } }],
        usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
      })
    );

    const provider = new OpenAIProvider({ apiKey: 'sk-test-key-1234' });
    const resp = await provider.complete({ messages: sampleMessages });
    expect(resp.content).toBe('Hello there!');
    expect(resp.role).toBe('assistant');
    expect(resp.usage.totalTokens).toBe(15);
  });

  it('should handle API error with structured error', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        error: { message: 'Rate limited', code: 'rate_limit', type: 'rate_limit_error' },
      }, false, 429)
    );

    const provider = new OpenAIProvider({ apiKey: 'sk-test-key-1234' });
    await expect(provider.complete({ messages: sampleMessages })).rejects.toThrow('Rate limited');
  });

  it('should handle API error with plain text', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse('Internal Server Error', false, 500)
    );

    const provider = new OpenAIProvider({ apiKey: 'sk-test-key-1234' });
    await expect(provider.complete({ messages: sampleMessages })).rejects.toThrow('HTTP 500');
  });

  it('should handle empty choices', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({ id: '1', choices: [], usage: {} })
    );

    const provider = new OpenAIProvider({ apiKey: 'sk-test-key-1234' });
    await expect(provider.complete({ messages: sampleMessages })).rejects.toThrow('empty choices');
  });

  it('should handle error in response body', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({ error: { message: 'Bad request', code: 'bad', type: 'invalid' } })
    );

    const provider = new OpenAIProvider({ apiKey: 'sk-test-key-1234' });
    await expect(provider.complete({ messages: sampleMessages })).rejects.toThrow('Bad request');
  });

  it('should stream response', async () => {
    const sseChunks = [
      'data: {"choices":[{"delta":{"content":"Hello"}}]}\n',
      'data: {"choices":[{"delta":{"content":" world"}}]}\n',
      'data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}\n',
      'data: [DONE]\n',
    ];
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(mockStreamResponse(sseChunks));

    const provider = new OpenAIProvider({ apiKey: 'sk-test-key-1234' });
    const chunks = [];
    for await (const chunk of provider.stream({ messages: sampleMessages })) {
      chunks.push(chunk);
    }
    expect(chunks.length).toBeGreaterThanOrEqual(2);
    expect(chunks[0].content).toBe('Hello');
    expect(chunks[1].content).toBe(' world');
    expect(chunks[chunks.length - 1].done).toBe(true);
  });

  it('should handle stream error', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse('Server Error', false, 500)
    );

    const provider = new OpenAIProvider({ apiKey: 'sk-test-key-1234' });
    await expect(async () => {
      for await (const _ of provider.stream({ messages: sampleMessages })) {}
    }).rejects.toThrow('HTTP 500');
  });

  it('should call tools', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        choices: [{
          message: {
            content: 'Let me check',
            tool_calls: [{
              id: 'call_1',
              function: { name: 'get_weather', arguments: '{"city":"NYC"}' },
            }],
          },
        }],
        usage: { prompt_tokens: 20, completion_tokens: 10, total_tokens: 30 },
      })
    );

    const provider = new OpenAIProvider({ apiKey: 'sk-test-key-1234' });
    const resp = await provider.callTools({ messages: sampleMessages, tools: sampleTools });
    expect(resp.content).toBe('Let me check');
    expect(resp.toolCalls).toHaveLength(1);
    expect(resp.toolCalls[0].name).toBe('get_weather');
    expect(resp.toolCalls[0].arguments).toBe('{"city":"NYC"}');
  });

  it('should handle callTools with error response', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({ error: { message: 'Tool error', code: 'err', type: 'bad' } })
    );

    const provider = new OpenAIProvider({ apiKey: 'sk-test-key-1234' });
    await expect(
      provider.callTools({ messages: sampleMessages, tools: sampleTools })
    ).rejects.toThrow('Tool error');
  });

  it('should handle callTools with empty choices', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({ choices: [], usage: {} })
    );

    const provider = new OpenAIProvider({ apiKey: 'sk-test-key-1234' });
    await expect(
      provider.callTools({ messages: sampleMessages, tools: sampleTools })
    ).rejects.toThrow('empty choices');
  });

  it('should include temperature and maxTokens in request body', async () => {
    let capturedBody: any;
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (_url, opts) => {
      capturedBody = JSON.parse((opts as RequestInit).body as string);
      return mockFetchResponse({
        id: '1',
        choices: [{ message: { role: 'assistant', content: 'ok' } }],
        usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
      });
    });

    const provider = new OpenAIProvider({ apiKey: 'sk-test-key-1234', temperature: 0.8, maxTokens: 200 });
    await provider.complete({ messages: sampleMessages, temperature: 0.5, maxTokens: 100 });
    expect(capturedBody.temperature).toBe(0.5);
    expect(capturedBody.max_tokens).toBe(100);
  });
});

// ===== Anthropic Provider Tests =====

describe('AnthropicProvider', () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  afterEach(() => {
    if (fetchSpy) fetchSpy.mockRestore();
  });

  it('should construct with default config', () => {
    const provider = new AnthropicProvider({ apiKey: 'sk-ant-key-1234' });
    const info = provider.info();
    expect(info.provider).toBe('anthropic');
    expect(info.name).toBe('claude-sonnet-4-20250514');
    expect(info.maxContext).toBe(200_000);
  });

  it('should throw on missing API key', () => {
    expect(() => new AnthropicProvider({ apiKey: '' })).toThrow('API key is required');
  });

  it('should complete a request', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        id: 'msg-1',
        type: 'message',
        role: 'assistant',
        content: [{ type: 'text', text: 'Hello from Claude' }],
        usage: { input_tokens: 10, output_tokens: 5 },
      })
    );

    const provider = new AnthropicProvider({ apiKey: 'sk-ant-key-1234' });
    const resp = await provider.complete({ messages: sampleMessages });
    expect(resp.content).toBe('Hello from Claude');
    expect(resp.usage.totalTokens).toBe(15);
  });

  it('should handle system messages', async () => {
    let capturedBody: any;
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (_url, opts) => {
      capturedBody = JSON.parse((opts as RequestInit).body as string);
      return mockFetchResponse({
        id: 'msg-1',
        content: [{ type: 'text', text: 'ok' }],
        usage: { input_tokens: 10, output_tokens: 5 },
      });
    });

    const provider = new AnthropicProvider({ apiKey: 'sk-ant-key-1234' });
    await provider.complete({
      messages: [
        { role: 'system', content: 'You are helpful' },
        { role: 'user', content: 'Hi' },
      ],
    });
    expect(capturedBody.system).toBe('You are helpful');
  });

  it('should handle tool messages', async () => {
    let capturedBody: any;
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (_url, opts) => {
      capturedBody = JSON.parse((opts as RequestInit).body as string);
      return mockFetchResponse({
        id: 'msg-1',
        content: [{ type: 'text', text: 'ok' }],
        usage: { input_tokens: 10, output_tokens: 5 },
      });
    });

    const provider = new AnthropicProvider({ apiKey: 'sk-ant-key-1234' });
    await provider.complete({
      messages: [
        { role: 'user', content: 'What is the weather?' },
        { role: 'tool', content: 'Sunny', name: 'get_weather' },
      ],
    });
    // Tool messages are converted to user messages
    expect(capturedBody.messages).toHaveLength(2);
    expect(capturedBody.messages[1].role).toBe('user');
  });

  it('should handle API error', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({ error: { message: 'Claude error' } }, false, 400)
    );

    const provider = new AnthropicProvider({ apiKey: 'sk-ant-key-1234' });
    await expect(provider.complete({ messages: sampleMessages })).rejects.toThrow('Claude error');
  });

  it('should handle API error with plain text', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse('Server Error', false, 500)
    );

    const provider = new AnthropicProvider({ apiKey: 'sk-ant-key-1234' });
    await expect(provider.complete({ messages: sampleMessages })).rejects.toThrow('HTTP 500');
  });

  it('should stream response', async () => {
    const sseChunks = [
      'data: {"type":"content_block_delta","delta":{"text":"Hello"}}\n',
      'data: {"type":"content_block_delta","delta":{"text":" from Claude"}}\n',
      'data: {"type":"message_stop"}\n',
    ];
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(mockStreamResponse(sseChunks));

    const provider = new AnthropicProvider({ apiKey: 'sk-ant-key-1234' });
    const chunks = [];
    for await (const chunk of provider.stream({ messages: sampleMessages })) {
      chunks.push(chunk);
    }
    expect(chunks.length).toBeGreaterThanOrEqual(2);
    expect(chunks[0].content).toBe('Hello');
    expect(chunks[chunks.length - 1].done).toBe(true);
  });

  it('should call tools', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        id: 'msg-1',
        content: [
          { type: 'text', text: 'Checking weather' },
          { type: 'tool_use', id: 'tool-1', name: 'get_weather', input: { city: 'NYC' } },
        ],
        usage: { input_tokens: 20, output_tokens: 10 },
      })
    );

    const provider = new AnthropicProvider({ apiKey: 'sk-ant-key-1234' });
    const resp = await provider.callTools({ messages: sampleMessages, tools: sampleTools });
    expect(resp.content).toBe('Checking weather');
    expect(resp.toolCalls).toHaveLength(1);
    expect(resp.toolCalls[0].name).toBe('get_weather');
    expect(JSON.parse(resp.toolCalls[0].arguments)).toEqual({ city: 'NYC' });
  });

  it('should handle multiple system messages', async () => {
    let capturedBody: any;
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (_url, opts) => {
      capturedBody = JSON.parse((opts as RequestInit).body as string);
      return mockFetchResponse({
        id: 'msg-1',
        content: [{ type: 'text', text: 'ok' }],
        usage: { input_tokens: 10, output_tokens: 5 },
      });
    });

    const provider = new AnthropicProvider({ apiKey: 'sk-ant-key-1234' });
    await provider.complete({
      messages: [
        { role: 'system', content: 'Rule 1' },
        { role: 'system', content: 'Rule 2' },
        { role: 'user', content: 'Hi' },
      ],
    });
    expect(capturedBody.system).toBe('Rule 1\nRule 2');
  });
});

// ===== Gemini Provider Tests =====

describe('GeminiProvider', () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  afterEach(() => {
    if (fetchSpy) fetchSpy.mockRestore();
  });

  it('should construct with default config', () => {
    const provider = new GeminiProvider({ apiKey: 'gem-key-1234' });
    const info = provider.info();
    expect(info.provider).toBe('gemini');
    expect(info.name).toBe('gemini-2.0-flash');
    expect(info.maxContext).toBe(1_048_576);
  });

  it('should throw on missing API key', () => {
    expect(() => new GeminiProvider({ apiKey: '' })).toThrow('API key is required');
  });

  it('should complete a request', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        candidates: [{
          content: { parts: [{ text: 'Hello from Gemini' }], role: 'model' },
          finishReason: 'STOP',
        }],
        usageMetadata: { promptTokenCount: 10, candidatesTokenCount: 5, totalTokenCount: 15 },
      })
    );

    const provider = new GeminiProvider({ apiKey: 'gem-key-1234' });
    const resp = await provider.complete({ messages: sampleMessages });
    expect(resp.content).toBe('Hello from Gemini');
    expect(resp.usage.totalTokens).toBe(15);
  });

  it('should handle system messages', async () => {
    let capturedBody: any;
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (_url, opts) => {
      capturedBody = JSON.parse((opts as RequestInit).body as string);
      return mockFetchResponse({
        candidates: [{ content: { parts: [{ text: 'ok' }] } }],
        usageMetadata: { promptTokenCount: 5, candidatesTokenCount: 3, totalTokenCount: 8 },
      });
    });

    const provider = new GeminiProvider({ apiKey: 'gem-key-1234' });
    await provider.complete({
      messages: [
        { role: 'system', content: 'Be helpful' },
        { role: 'user', content: 'Hi' },
      ],
    });
    expect(capturedBody.systemInstruction).toBeDefined();
    expect(capturedBody.systemInstruction.parts[0].text).toBe('Be helpful');
  });

  it('should handle tool messages', async () => {
    let capturedBody: any;
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (_url, opts) => {
      capturedBody = JSON.parse((opts as RequestInit).body as string);
      return mockFetchResponse({
        candidates: [{ content: { parts: [{ text: 'ok' }] } }],
        usageMetadata: { promptTokenCount: 5, candidatesTokenCount: 3, totalTokenCount: 8 },
      });
    });

    const provider = new GeminiProvider({ apiKey: 'gem-key-1234' });
    await provider.complete({
      messages: [
        { role: 'user', content: 'Weather?' },
        { role: 'tool', content: 'Sunny', name: 'get_weather' },
      ],
    });
    expect(capturedBody.contents).toHaveLength(2);
    expect(capturedBody.contents[1].role).toBe('user');
  });

  it('should handle API error', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({ error: { message: 'Gemini error' } }, false, 400)
    );

    const provider = new GeminiProvider({ apiKey: 'gem-key-1234' });
    await expect(provider.complete({ messages: sampleMessages })).rejects.toThrow('Gemini error');
  });

  it('should handle API error with plain text', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse('Server Error', false, 500)
    );

    const provider = new GeminiProvider({ apiKey: 'gem-key-1234' });
    await expect(provider.complete({ messages: sampleMessages })).rejects.toThrow('HTTP 500');
  });

  it('should stream response', async () => {
    const sseChunks = [
      'data: {"candidates":[{"content":{"parts":[{"text":"Hello"}]}}]}\n',
      'data: {"candidates":[{"content":{"parts":[{"text":" from Gemini"}]}}]}\n',
    ];
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(mockStreamResponse(sseChunks));

    const provider = new GeminiProvider({ apiKey: 'gem-key-1234' });
    const chunks = [];
    for await (const chunk of provider.stream({ messages: sampleMessages })) {
      chunks.push(chunk);
    }
    expect(chunks.length).toBeGreaterThanOrEqual(2);
    expect(chunks[0].content).toBe('Hello');
    expect(chunks[chunks.length - 1].done).toBe(true);
  });

  it('should call tools', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        candidates: [{
          content: {
            parts: [
              { text: 'Checking weather' },
              { functionCall: { name: 'get_weather', args: { city: 'NYC' } } },
            ],
          },
        }],
        usageMetadata: { promptTokenCount: 20, candidatesTokenCount: 10, totalTokenCount: 30 },
      })
    );

    const provider = new GeminiProvider({ apiKey: 'gem-key-1234' });
    const resp = await provider.callTools({ messages: sampleMessages, tools: sampleTools });
    expect(resp.content).toBe('Checking weather');
    expect(resp.toolCalls).toHaveLength(1);
    expect(resp.toolCalls[0].name).toBe('get_weather');
  });
});

// ===== Ollama Provider Tests =====

describe('OllamaProvider', () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  afterEach(() => {
    if (fetchSpy) fetchSpy.mockRestore();
  });

  it('should construct with default config', () => {
    const provider = new OllamaProvider({});
    const info = provider.info();
    expect(info.provider).toBe('ollama');
    expect(info.name).toBe('llama3');
    expect(info.maxContext).toBe(8192);
  });

  it('should construct with custom config', () => {
    const provider = new OllamaProvider({ model: 'mistral', baseURL: 'http://my-ollama:11434' });
    expect(provider.info().name).toBe('mistral');
  });

  it('should complete a request', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        model: 'llama3',
        message: { role: 'assistant', content: 'Hello from Ollama' },
        done: true,
        prompt_eval_count: 10,
        eval_count: 5,
      })
    );

    const provider = new OllamaProvider({});
    const resp = await provider.complete({ messages: sampleMessages });
    expect(resp.content).toBe('Hello from Ollama');
    expect(resp.usage.promptTokens).toBe(10);
    expect(resp.usage.completionTokens).toBe(5);
  });

  it('should handle API error', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse('Model not found', false, 404)
    );

    const provider = new OllamaProvider({});
    await expect(provider.complete({ messages: sampleMessages })).rejects.toThrow('HTTP 404');
  });

  it('should stream response', async () => {
    const streamChunks = [
      JSON.stringify({ model: 'llama3', message: { role: 'assistant', content: 'Hello' }, done: false }) + '\n',
      JSON.stringify({ model: 'llama3', message: { role: 'assistant', content: ' from Ollama' }, done: false }) + '\n',
      JSON.stringify({ model: 'llama3', message: { role: 'assistant', content: '' }, done: true }) + '\n',
    ];
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(mockStreamResponse(streamChunks));

    const provider = new OllamaProvider({});
    const chunks = [];
    for await (const chunk of provider.stream({ messages: sampleMessages })) {
      chunks.push(chunk);
    }
    expect(chunks.length).toBeGreaterThanOrEqual(2);
    expect(chunks[0].content).toBe('Hello');
  });

  it('should call tools', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        model: 'llama3',
        message: {
          role: 'assistant',
          content: 'Let me check',
          tool_calls: [{ function: { name: 'get_weather', arguments: { city: 'NYC' } } }],
        },
        done: true,
        prompt_eval_count: 20,
        eval_count: 10,
      })
    );

    const provider = new OllamaProvider({});
    const resp = await provider.callTools({ messages: sampleMessages, tools: sampleTools });
    expect(resp.content).toBe('Let me check');
    expect(resp.toolCalls).toHaveLength(1);
    expect(resp.toolCalls[0].name).toBe('get_weather');
  });

  it('should include temperature and num_predict in options', async () => {
    let capturedBody: any;
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (_url, opts) => {
      capturedBody = JSON.parse((opts as RequestInit).body as string);
      return mockFetchResponse({
        model: 'llama3',
        message: { role: 'assistant', content: 'ok' },
        done: true,
        prompt_eval_count: 5,
        eval_count: 3,
      });
    });

    const provider = new OllamaProvider({ temperature: 0.7, maxTokens: 100 });
    await provider.complete({ messages: sampleMessages });
    expect(capturedBody.options.temperature).toBe(0.7);
    expect(capturedBody.options.num_predict).toBe(100);
  });
});

// ===== OpenAI-Compatible Provider Tests =====

describe('DeepSeekProvider', () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  afterEach(() => {
    if (fetchSpy) fetchSpy.mockRestore();
  });

  it('should construct with default config', () => {
    const provider = new DeepSeekProvider({ apiKey: 'ds-key-1234567890' });
    const info = provider.info();
    expect(info.provider).toBe('deepseek');
    expect(info.name).toBe('deepseek-chat');
    expect(info.maxContext).toBe(64_000);
  });

  it('should throw on missing API key', () => {
    expect(() => new DeepSeekProvider({ apiKey: '' })).toThrow('API key is required');
  });

  it('should complete a request', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        id: '1',
        choices: [{ message: { role: 'assistant', content: 'DeepSeek reply' } }],
        usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
      })
    );

    const provider = new DeepSeekProvider({ apiKey: 'ds-key-1234567890' });
    const resp = await provider.complete({ messages: sampleMessages });
    expect(resp.content).toBe('DeepSeek reply');
  });

  it('should call tools', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        choices: [{
          message: {
            content: 'ok',
            tool_calls: [{ id: 'tc1', function: { name: 'test', arguments: '{}' } }],
          },
        }],
        usage: { prompt_tokens: 5, completion_tokens: 3, total_tokens: 8 },
      })
    );

    const provider = new DeepSeekProvider({ apiKey: 'ds-key-1234567890' });
    const resp = await provider.callTools({ messages: sampleMessages, tools: sampleTools });
    expect(resp.toolCalls).toHaveLength(1);
  });

  it('should handle API error', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse('Error', false, 500)
    );

    const provider = new DeepSeekProvider({ apiKey: 'ds-key-1234567890' });
    await expect(provider.complete({ messages: sampleMessages })).rejects.toThrow('HTTP 500');
  });

  it('should stream response', async () => {
    const sseChunks = [
      'data: {"choices":[{"delta":{"content":"Hi"}}]}\n',
      'data: {"choices":[{"delta":{"content":" there"}}]}\n',
      'data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}\n',
      'data: [DONE]\n',
    ];
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(mockStreamResponse(sseChunks));

    const provider = new DeepSeekProvider({ apiKey: 'ds-key-1234567890' });
    const chunks = [];
    for await (const chunk of provider.stream({ messages: sampleMessages })) {
      chunks.push(chunk);
    }
    expect(chunks[0].content).toBe('Hi');
  });
});

describe('QwenProvider', () => {
  it('should construct with default config', () => {
    const provider = new QwenProvider({ apiKey: 'qwen-key-1234567890' });
    expect(provider.info().provider).toBe('qwen');
    expect(provider.info().name).toBe('qwen-plus');
    expect(provider.info().maxContext).toBe(128_000);
  });

  it('should complete a request', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        id: '1',
        choices: [{ message: { role: 'assistant', content: 'Qwen reply' } }],
        usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
      })
    );

    const provider = new QwenProvider({ apiKey: 'qwen-key-1234567890' });
    const resp = await provider.complete({ messages: sampleMessages });
    expect(resp.content).toBe('Qwen reply');
    fetchSpy.mockRestore();
  });
});

describe('GLMProvider', () => {
  it('should construct with default config', () => {
    const provider = new GLMProvider({ apiKey: 'glm-key-1234567890' });
    expect(provider.info().provider).toBe('glm');
    expect(provider.info().name).toBe('glm-4-flash');
    expect(provider.info().supportsTools).toBe(false);
  });

  it('should throw on callTools', async () => {
    const provider = new GLMProvider({ apiKey: 'glm-key-1234567890' });
    await expect(
      provider.callTools({ messages: sampleMessages, tools: sampleTools })
    ).rejects.toThrow('does not support tool calls');
  });

  it('should complete a request', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        id: '1',
        choices: [{ message: { role: 'assistant', content: 'GLM reply' } }],
        usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
      })
    );

    const provider = new GLMProvider({ apiKey: 'glm-key-1234567890' });
    const resp = await provider.complete({ messages: sampleMessages });
    expect(resp.content).toBe('GLM reply');
    fetchSpy.mockRestore();
  });
});

describe('MistralProvider', () => {
  it('should construct with default config', () => {
    const provider = new MistralProvider({ apiKey: 'mistral-key-123456' });
    expect(provider.info().provider).toBe('mistral');
    expect(provider.info().name).toBe('mistral-large-latest');
    expect(provider.info().maxContext).toBe(32_000);
  });

  it('should complete a request', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        id: '1',
        choices: [{ message: { role: 'assistant', content: 'Mistral reply' } }],
        usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
      })
    );

    const provider = new MistralProvider({ apiKey: 'mistral-key-123456' });
    const resp = await provider.complete({ messages: sampleMessages });
    expect(resp.content).toBe('Mistral reply');
    fetchSpy.mockRestore();
  });
});

describe('CohereProvider', () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  afterEach(() => {
    if (fetchSpy) fetchSpy.mockRestore();
  });

  it('should construct with default config', () => {
    const provider = new CohereProvider({ apiKey: 'cohere-key-1234567890' });
    const info = provider.info();
    expect(info.provider).toBe('cohere');
    expect(info.name).toBe('command-r-plus');
    expect(info.maxContext).toBe(128_000);
  });

  it('should throw on missing API key', () => {
    expect(() => new CohereProvider({ apiKey: '' })).toThrow('API key is required');
  });

  it('should complete a request', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        id: '1',
        message: { role: 'assistant', content: [{ text: 'Cohere reply' }] },
        usage: { billed_units: { input_tokens: 10, output_tokens: 5 } },
      })
    );

    const provider = new CohereProvider({ apiKey: 'cohere-key-1234567890' });
    const resp = await provider.complete({ messages: sampleMessages });
    expect(resp.content).toBe('Cohere reply');
    expect(resp.usage.totalTokens).toBe(15);
  });

  it('should handle API error', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse('Error', false, 500)
    );

    const provider = new CohereProvider({ apiKey: 'cohere-key-1234567890' });
    await expect(provider.complete({ messages: sampleMessages })).rejects.toThrow('Cohere API error');
  });

  it('should call tools', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        message: {
          content: [{ text: 'ok' }],
          tool_calls: [{ id: 'tc1', function: { name: 'test', arguments: '{}' } }],
        },
        usage: { billed_units: { input_tokens: 5, output_tokens: 3 } },
      })
    );

    const provider = new CohereProvider({ apiKey: 'cohere-key-1234567890' });
    const resp = await provider.callTools({ messages: sampleMessages, tools: sampleTools });
    expect(resp.toolCalls).toHaveLength(1);
  });

  it('should stream response', async () => {
    const streamChunks = [
      JSON.stringify({ type: 'content-delta', delta: { message: { content: { text: 'Hello' } } } }) + '\n',
      JSON.stringify({ type: 'message-end', delta: { usage: { billed_units: { input_tokens: 5, output_tokens: 3 } } } }) + '\n',
    ];
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(mockStreamResponse(streamChunks));

    const provider = new CohereProvider({ apiKey: 'cohere-key-1234567890' });
    const chunks = [];
    for await (const chunk of provider.stream({ messages: sampleMessages })) {
      chunks.push(chunk);
    }
    expect(chunks[0].content).toBe('Hello');
    expect(chunks[chunks.length - 1].done).toBe(true);
  });
});

describe('AzureOpenAIProvider', () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  afterEach(() => {
    if (fetchSpy) fetchSpy.mockRestore();
  });

  it('should construct with valid config', () => {
    const provider = new AzureOpenAIProvider({
      apiKey: 'azure-key-1234567890',
      resourceName: 'my-resource',
      deploymentName: 'my-deployment',
    });
    const info = provider.info();
    expect(info.provider).toBe('azure');
    expect(info.name).toBe('my-deployment');
  });

  it('should throw on missing API key', () => {
    expect(() => new AzureOpenAIProvider({
      apiKey: '',
      resourceName: 'res',
      deploymentName: 'dep',
    })).toThrow('API key is required');
  });

  it('should throw on missing resource name', () => {
    expect(() => new AzureOpenAIProvider({
      apiKey: 'key',
      resourceName: '',
      deploymentName: 'dep',
    })).toThrow('resource name is required');
  });

  it('should throw on missing deployment name', () => {
    expect(() => new AzureOpenAIProvider({
      apiKey: 'key',
      resourceName: 'res',
      deploymentName: '',
    })).toThrow('deployment name is required');
  });

  it('should complete a request', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        id: '1',
        choices: [{ message: { role: 'assistant', content: 'Azure reply' } }],
        usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
      })
    );

    const provider = new AzureOpenAIProvider({
      apiKey: 'azure-key-1234567890',
      resourceName: 'my-resource',
      deploymentName: 'my-deployment',
    });
    const resp = await provider.complete({ messages: sampleMessages });
    expect(resp.content).toBe('Azure reply');
  });

  it('should handle API error', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse('Error', false, 500)
    );

    const provider = new AzureOpenAIProvider({
      apiKey: 'azure-key-1234567890',
      resourceName: 'res',
      deploymentName: 'dep',
    });
    await expect(provider.complete({ messages: sampleMessages })).rejects.toThrow('Azure API error');
  });

  it('should stream response', async () => {
    const sseChunks = [
      'data: {"choices":[{"delta":{"content":"Hi"}}]}\n',
      'data: {"choices":[{"delta":{},"finish_reason":"stop"}]}\n',
      'data: [DONE]\n',
    ];
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(mockStreamResponse(sseChunks));

    const provider = new AzureOpenAIProvider({
      apiKey: 'azure-key-1234567890',
      resourceName: 'res',
      deploymentName: 'dep',
    });
    const chunks = [];
    for await (const chunk of provider.stream({ messages: sampleMessages })) {
      chunks.push(chunk);
    }
    expect(chunks[0].content).toBe('Hi');
  });

  it('should call tools', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({
        choices: [{
          message: {
            content: 'ok',
            tool_calls: [{ id: 'tc1', function: { name: 'test', arguments: '{}' } }],
          },
        }],
        usage: { prompt_tokens: 5, completion_tokens: 3, total_tokens: 8 },
      })
    );

    const provider = new AzureOpenAIProvider({
      apiKey: 'azure-key-1234567890',
      resourceName: 'res',
      deploymentName: 'dep',
    });
    const resp = await provider.callTools({ messages: sampleMessages, tools: sampleTools });
    expect(resp.toolCalls).toHaveLength(1);
  });

  it('should handle callTools with empty choices', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      mockFetchResponse({ choices: [], usage: {} })
    );

    const provider = new AzureOpenAIProvider({
      apiKey: 'azure-key-1234567890',
      resourceName: 'res',
      deploymentName: 'dep',
    });
    await expect(
      provider.callTools({ messages: sampleMessages, tools: sampleTools })
    ).rejects.toThrow('empty choices');
  });
});

// ===== APIError Tests =====

describe('APIError', () => {
  it('should create with all fields', () => {
    const err = new APIError('test message', 'err_code', 'err_type', 400);
    expect(err.message).toBe('test message');
    expect(err.code).toBe('err_code');
    expect(err.type).toBe('err_type');
    expect(err.status).toBe(400);
    expect(err.name).toBe('APIError');
  });
});
