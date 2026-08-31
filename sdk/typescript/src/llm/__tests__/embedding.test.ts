// embedding.test.ts — S0-3 语义原生化单测：三线实现 + 黄金值双线对齐 + 环境装配。
//
// 黄金值说明：FNV-1a 32 位对 ASCII/UTF-8 字节序是良定义的整数运算，
// 这里硬编码的 fnv/idx/sign 由独立脚本计算（与 Go 侧 embedding_test.go 同值），
// 任何一侧实现漂移（字节序、模运算、符号位）都会被立刻抓住。
import { describe, it, expect, vi } from 'vitest';
import {
  LexicalEmbedder,
  lexicalFnv1a32,
  lexicalTokenize,
  OpenAIEmbeddingProvider,
  OllamaEmbeddingProvider,
  CachedEmbeddingProvider,
  EmbeddingProviderAdapter,
  createEmbeddingProviderFromEnv,
} from '../embedding.js';
import type { EmbeddingConfig } from '../embedding.js';

/** 构造可注入的 fake fetch（对齐 Go 测试的 httptest.Client 注入口径）。 */
function fakeFetch(handler: (url: string, init?: RequestInit) => Response | Promise<Response>) {
  const fn = vi.fn(async (url: string | URL | Request, init?: RequestInit) =>
    handler(String(url), init));
  return fn as unknown as typeof fetch;
}

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200 });
}

describe('LexicalEmbedder — 无 key 降级位', () => {
  it('同输入两次嵌入结果逐位一致（确定性要求）', async () => {
    const e = new LexicalEmbedder();
    const texts = [
      'HNSW 索引与向量检索',
      'AgentPrimordia 是通用 Go Agent 开发框架',
      '',
      '   \t\n  ',
      'mixed 英文与中文 mixed 123 456',
    ];
    const first = await e.embeddings(texts);
    const second = await e.embeddings(texts);
    expect(first).toHaveLength(texts.length);
    for (let i = 0; i < texts.length; i++) {
      expect(first[i]).toHaveLength(256);
      expect(first[i]).toEqual(second[i]);
    }
  });

  it('FNV-1a 黄金值（独立计算，与 Go 侧同值）', () => {
    expect(lexicalFnv1a32('hnsw')).toBe(763821949); // idx=125 sign=-1
    expect(lexicalFnv1a32('索引')).toBe(1541071376); // idx=16 sign=+1
    expect(lexicalFnv1a32('memory')).toBe(2229924270); // idx=174 sign=-1
    expect(763821949 & 0xff).toBe(125);
    expect((763821949 >>> 8) & 1).toBe(1);
  });

  it('分词规则：CJK bigram / 单字 / 拉丁整词 / 分隔符', () => {
    expect(lexicalTokenize('向量检索')).toEqual(['向量', '量检', '检索']);
    expect(lexicalTokenize('网')).toEqual(['网']);
    expect(lexicalTokenize('HNSW')).toEqual(['hnsw']);
    expect(lexicalTokenize('ef_search 200')).toEqual(['ef_search', '200']);
    expect(lexicalTokenize('HNSW 索引')).toEqual(['hnsw', '索引']);
    expect(lexicalTokenize('Go 1.26')).toEqual(['go', '1', '26']);
    expect(lexicalTokenize('')).toEqual([]);
  });

  it('L2 归一化、符号黄金位与降级位元信息（semantic=false）', async () => {
    const e = new LexicalEmbedder();
    expect(e.semantic()).toBe(false); // 降级位不得计入语义验收数字
    expect(e.dimension()).toBe(256);
    expect(e.model()).toBe('lexical-fallback-v1');

    const [v] = await e.embeddings(['向量检索']);
    // 三个 bigram 全部 count=1：非零位 idx 108(-)/167(-)/161(+)
    const expectSign = new Map<number, number>([[108, -1], [167, -1], [161, 1]]);
    let normSq = 0;
    v.forEach((x, i) => {
      normSq += x * x;
      if (expectSign.has(i)) {
        expect(Math.sign(x)).toBe(expectSign.get(i));
      } else {
        expect(x).toBe(0);
      }
    });
    expect(Math.abs(normSq - 1)).toBeLessThan(1e-9);

    // 重复词的 sublinear TF：w=sqrt(2)，L2 归一化后该维度回到 1
    const [v2] = await e.embeddings(['检索 检索']);
    expect(v2[161]).toBeGreaterThan(0.999);
    expect(v2[161]).toBeLessThan(1.001);
  });

  it('空文本 → 全零向量（norm==0 分支）', async () => {
    const e = new LexicalEmbedder();
    const vecs = await e.embeddings(['', '   ']);
    for (const v of vecs) expect(v.every((x) => x === 0)).toBe(true);
  });
});

describe('OpenAIEmbeddingProvider — OpenAI 兼容实现', () => {
  const config = (fetchImpl: typeof fetch, extra: Partial<EmbeddingConfig> = {}): EmbeddingConfig => ({
    apiKey: 'test-key',
    baseURL: 'http://fake.local/v1',
    model: 'test-embed',
    dimensions: 2,
    fetchImpl,
    ...extra,
  });

  it('wire format：路径/Bearer 头/model/input/dimensions + 观测维度', async () => {
    let captured: { url: string; auth: string; body: EmbeddingWireCapture } | null = null;
    interface EmbeddingWireCapture { model: string; input: string[]; dimensions?: number }
    const fetchImpl = fakeFetch((url, init) => {
      captured = {
        url,
        auth: String(new Headers(init?.headers).get('Authorization')),
        body: JSON.parse(String(init?.body)) as EmbeddingWireCapture,
      };
      return jsonResponse({
        data: [{ index: 0, embedding: [0.1, 0.2] }, { index: 1, embedding: [0.3, 0.4] }],
      });
    });
    const p = new OpenAIEmbeddingProvider(config(fetchImpl));
    const vecs = await p.embeddings(['第一段', 'second']);

    expect(captured!.url).toBe('http://fake.local/v1/embeddings');
    expect(captured!.auth).toBe('Bearer test-key');
    expect(captured!.body.model).toBe('test-embed');
    expect(captured!.body.input).toEqual(['第一段', 'second']);
    expect(captured!.body.dimensions).toBe(2);
    expect(vecs[1]).toEqual([0.3, 0.4]);
    expect(p.dimension()).toBe(2);
    expect(p.semantic()).toBe(true);
  });

  it('index 未回填（恒 0）→ 按响应顺序对位', async () => {
    const fetchImpl = fakeFetch(() =>
      jsonResponse({ data: [{ embedding: [1] }, { embedding: [2] }] }));
    const p = new OpenAIEmbeddingProvider(config(fetchImpl, { dimensions: 0 }));
    const vecs = await p.embeddings(['a', 'b']);
    expect(vecs[0]).toEqual([1]);
    expect(vecs[1]).toEqual([2]);
  });

  it('API key 缺失 → 构造报错（对齐 ErrAPIKeyRequired）', () => {
    expect(() => new OpenAIEmbeddingProvider({})).toThrow(/API key is required/);
  });

  it('非 200 → APIError 携带状态码', async () => {
    const fetchImpl = fakeFetch(() => new Response('bad key', { status: 401 }));
    const p = new OpenAIEmbeddingProvider(config(fetchImpl));
    await expect(p.embeddings(['x'])).rejects.toMatchObject({ status: 401 });
  });

  it('响应体 error 字段 → 抛出带 message 的错误', async () => {
    const fetchImpl = fakeFetch(() =>
      jsonResponse({ error: { message: 'quota exceeded', type: 'insufficient_quota' } }));
    const p = new OpenAIEmbeddingProvider(config(fetchImpl));
    await expect(p.embeddings(['x'])).rejects.toThrow(/quota exceeded/);
  });
});

describe('OllamaEmbeddingProvider — ollama 原生实现', () => {
  it('/api/embed 批量路径 + 探测结果固化', async () => {
    const calls: string[] = [];
    const fetchImpl = fakeFetch((url, init) => {
      calls.push(url);
      const body = JSON.parse(String(init?.body)) as { model: string; input?: string[]; prompt?: string };
      if (url.endsWith('/api/embed')) {
        expect(body.model).toBe('nomic-embed-text');
        return jsonResponse({ embeddings: body.input!.map(() => [1, 0, 0]) });
      }
      throw new Error('不应走逐条路径: ' + url);
    });
    const p = new OllamaEmbeddingProvider({ baseURL: 'http://fake.local', fetchImpl });
    const vecs = await p.embeddings(['a', 'b']);
    expect(vecs).toEqual([[1, 0, 0], [1, 0, 0]]);
    expect(p.dimension()).toBe(3); // 观测维度
    await p.embeddings(['c']); // 复用批量路径（探测固化）
    expect(calls.filter((u) => u.endsWith('/api/embed'))).toHaveLength(2);
  });

  it('/api/embed 404 → 降级逐条 /api/embeddings', async () => {
    const perText: string[] = [];
    const fetchImpl = fakeFetch((url, init) => {
      if (url.endsWith('/api/embed')) return new Response('not found', { status: 404 });
      const body = JSON.parse(String(init?.body)) as { prompt: string };
      perText.push(body.prompt);
      return jsonResponse({ embedding: [body.prompt.length, 7] });
    });
    const p = new OllamaEmbeddingProvider({ baseURL: 'http://fake.local', fetchImpl });
    const vecs = await p.embeddings(['你好', 'hi']);
    expect(perText).toEqual(['你好', 'hi']);
    expect(vecs[0]).toEqual([2, 7]);
    expect(vecs[1]).toEqual([2, 7]);
    // 404 结论固化后直接走逐条（不再探测批量）
    await p.embeddings(['第三次']);
    expect(perText).toHaveLength(3);
  });
});

describe('createEmbeddingProviderFromEnv — 环境装配', () => {
  it('全部未设置 → 降级位（不配置即词面兜底）', () => {
    const p = createEmbeddingProviderFromEnv({});
    expect(p.semantic()).toBe(false);
    expect(p.model()).toBe('lexical-fallback-v1');
  });

  it('provider=ollama → 原生实现；openai 缺 key 报错；未知 provider 报错', () => {
    expect(createEmbeddingProviderFromEnv({ AP_EMBEDDINGS_PROVIDER: 'ollama' })).toBeInstanceOf(OllamaEmbeddingProvider);
    expect(() =>
      createEmbeddingProviderFromEnv({ AP_EMBEDDINGS_PROVIDER: 'openai', AP_EMBEDDINGS_MODEL: 'm' }),
    ).toThrow(/API key is required/);
    expect(() =>
      createEmbeddingProviderFromEnv({ AP_EMBEDDINGS_PROVIDER: 'nope', AP_EMBEDDINGS_MODEL: 'm', AP_EMBEDDINGS_API_KEY: 'k' }),
    ).toThrow(/AP_EMBEDDINGS_PROVIDER/);
  });
});

describe('EmbeddingProviderAdapter / CachedEmbeddingProvider', () => {
  it('适配器委托并透传声明语义性', async () => {
    const inner = new LexicalEmbedder();
    const a = new EmbeddingProviderAdapter(inner, 'fake-semantic', 256, true);
    const vecs = await a.embeddings(['x']);
    expect(vecs).toHaveLength(1);
    expect(vecs[0]).toHaveLength(256);
    expect(a.model()).toBe('fake-semantic');
    expect(a.dimension()).toBe(256);
    expect(a.semantic()).toBe(true); // 声明被透传——调用方仅对真实端点传 true
  });

  it('缓存：批内去重 + 二次调用全命中 + 命中率计数', async () => {
    const inner = new LexicalEmbedder();
    const spy = vi.spyOn(inner, 'embeddings');
    const p = new CachedEmbeddingProvider(inner, 16);

    const first = await p.embeddings(['a', 'b', 'c', 'c']);
    expect(spy).toHaveBeenCalledTimes(1);
    expect(spy.mock.calls[0]![0]).toEqual(['a', 'b', 'c']); // 批内去重
    expect(first[2]).toEqual(first[3]); // 重复文本同向量

    await p.embeddings(['c', 'b']); // 全命中，不触发远端
    expect(spy).toHaveBeenCalledTimes(1);

    const stats = p.cacheStats();
    expect(stats.hits).toBe(2);
    expect(stats.misses).toBe(4);
    expect(Math.abs(stats.hitRate - 1 / 3)).toBeLessThan(1e-9);
    expect(p.semantic()).toBe(false); // 降级位包缓存仍是降级位
  });
});
