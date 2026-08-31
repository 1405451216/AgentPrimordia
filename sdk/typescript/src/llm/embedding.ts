// embedding.ts — S0-3 语义原生化：Embedding Provider 抽象与三线实现
//（OpenAI 兼容 / ollama 原生 / 无 key 词面降级位）——与 Go 侧
// agentprimordia/internal/llm/embedding.go 逐位对齐（docs/双线豁免矩阵.md #6）。
//
// 对齐纪律：LexicalEmbedder 的算法（分词、FNV-1a、维度、sqrt TF、L2 归一化、
// 浮点累加次序）与 Go 侧完全一致，任何一侧改动必须同步另一侧——
// 双线 recall@10 差 ≤0.02 门（scripts/dual-line-recall-check.mjs）依赖逐位一致。
// 刻意用 sqrt 而非 ln 做 sublinear TF：sqrt 为 IEEE 正确舍入，双线逐位一致。

import { APIError } from './openai.js';

/** EmbeddingProvider 语义嵌入抽象（对齐 Go internal/llm.EmbeddingProvider）。
 *
 * 与既有 Provider.embeddings? 的关系：后者是散落在聊天 Provider 上的可选能力；
 * 本接口补齐 dimension/model/semantic 元信息，是 memory 向量路径注入与
 * 双线召回基准的统一装配口径。
 * - dimension：向量维度；0 表示由服务端决定且尚未观测到真实维度。
 * - semantic：是否真实语义嵌入。false 表示无 key 降级位（词面伪嵌入），
 *   其输出只可用于兜底检索与回归底档基准，不得用于任何语义验收数字。
 */
export interface EmbeddingProvider {
  embeddings(texts: string[]): Promise<number[][]>;
  dimension(): number;
  model(): string;
  semantic(): boolean;
}

/** 语义嵌入 Provider 配置（OpenAI 兼容与 ollama 原生共用）。 */
export interface EmbeddingConfig {
  /** Bearer 令牌；ollama 原生端点可留空。 */
  apiKey?: string;
  /** 服务根地址：OpenAI 兼容默认 https://api.openai.com/v1（含 ollama 的 /v1）；
   *  ollama 原生默认 http://127.0.0.1:11434（回环 IP 直写，避免容器内解析差异）。 */
  baseURL?: string;
  /** 嵌入模型（如 text-embedding-3-small / bge-m3 / nomic-embed-text）。 */
  model?: string;
  /** 期望维度；0/缺省表示由服务端决定（OpenAI 兼容实现随请求携带 dimensions）。 */
  dimensions?: number;
  /** 单次请求超时毫秒；缺省 60s。 */
  timeoutMs?: number;
  /** 可注入 fetch 实现（测试用 httptest/fake server 对齐 Go 侧 Client 注入）。 */
  fetchImpl?: typeof fetch;
}

const DEFAULT_OPENAI_EMBEDDINGS_BASE_URL = 'https://api.openai.com/v1';
const DEFAULT_EMBEDDING_MODEL = 'text-embedding-3-small';
const OLLAMA_EMBEDDINGS_DEFAULT_BASE_URL = 'http://127.0.0.1:11434';
const DEFAULT_OLLAMA_EMBEDDING_MODEL = 'nomic-embed-text';
const DEFAULT_EMBEDDING_TIMEOUT_MS = 60_000;
const MAX_RESPONSE_CHARS = 10 * 1024 * 1024; // 与 Go maxResponseSize 对齐

// ===== OpenAI 兼容实现 =====

interface EmbeddingWireRequest {
  model: string;
  input: string[];
  dimensions?: number;
}

interface EmbeddingWireResponse {
  model?: string;
  data?: Array<{ index?: number; embedding?: number[] }>;
  error?: { message?: string; type?: string; code?: string };
}

/** OpenAI 兼容嵌入实现：POST {base}/embeddings，OpenAI wire format。
 *  对任何 OpenAI 兼容端点可用（含 ollama 暴露的 /v1/embeddings）。 */
export class OpenAIEmbeddingProvider implements EmbeddingProvider {
  private readonly apiKey: string;
  private readonly baseURL: string;
  private readonly modelName: string; // 命名避开接口方法 model()（TS 类字段与方法不能同名）
  private readonly dimensions: number;
  private readonly timeoutMs: number;
  private readonly fetchImpl: typeof fetch;
  /** 首次成功响应观测到的真实维度（未显式配置时供 dimension() 返回）。 */
  private observed = 0;

  constructor(config: EmbeddingConfig) {
    if (!config.apiKey) {
      // 对齐 Go ErrAPIKeyRequired / requireApiKey 语义
      throw new Error('API key is required');
    }
    this.apiKey = config.apiKey;
    this.baseURL = (config.baseURL ?? DEFAULT_OPENAI_EMBEDDINGS_BASE_URL).replace(/\/+$/, '');
    this.modelName = config.model ?? DEFAULT_EMBEDDING_MODEL;
    this.dimensions = config.dimensions ?? 0;
    this.timeoutMs = config.timeoutMs ?? DEFAULT_EMBEDDING_TIMEOUT_MS;
    this.fetchImpl = config.fetchImpl ?? globalThis.fetch.bind(globalThis);
  }

  async embeddings(texts: string[]): Promise<number[][]> {
    if (texts.length === 0) return [];
    const req: EmbeddingWireRequest = { model: this.modelName, input: texts };
    if (this.dimensions > 0) req.dimensions = this.dimensions;

    const raw = await this.doRequest('/embeddings', req);
    const resp = JSON.parse(raw) as EmbeddingWireResponse;
    if (resp.error?.message) {
      throw new APIError(resp.error.message, resp.error.code ?? '', resp.error.type ?? '', 200);
    }
    const data = resp.data ?? [];
    if (data.length !== texts.length) {
      throw new Error(`failed to parse LLM response: embeddings 返回条数 ${data.length} ≠ 请求条数 ${texts.length}`);
    }
    // index 容错：与 Go 侧同规则——index 集合构不成 [0,n) 排列时按响应顺序对位
    const wellFormed = data.every((d, i) => d.index === i);
    if (!wellFormed) {
      const seen = new Set<number>();
      let perm = data.length > 0;
      for (const d of data) {
        const idx = d.index ?? -1;
        if (idx < 0 || idx >= data.length || seen.has(idx)) { perm = false; break; }
        seen.add(idx);
      }
      if (!perm) data.forEach((d, i) => { d.index = i; });
    }
    const out: number[][] = new Array(texts.length);
    for (const d of data) out[d.index!] = d.embedding ?? [];
    if (this.observed === 0 && out[0] && out[0].length > 0) this.observed = out[0].length;
    return out;
  }

  dimension(): number {
    return this.dimensions > 0 ? this.dimensions : this.observed;
  }

  model(): string { return this.modelName; }

  semantic(): boolean { return true; }

  private async doRequest(path: string, body: unknown): Promise<string> {
    const resp = await this.fetchImpl(this.baseURL + path, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${this.apiKey}`,
        'User-Agent': 'AgentPrimordia-TS/1.0',
      },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(this.timeoutMs),
    });
    const text = await resp.text();
    if (text.length > MAX_RESPONSE_CHARS) {
      throw new Error(`embeddings response exceeds ${MAX_RESPONSE_CHARS} chars`);
    }
    if (!resp.ok) {
      throw new APIError(text.slice(0, 512) || `HTTP ${resp.status}`, String(resp.status), 'embeddings', resp.status);
    }
    return text;
  }
}

// ===== ollama 原生实现 =====

interface OllamaEmbedBatchResponse {
  embeddings?: number[][];
  error?: string;
}

/** ollama 原生嵌入实现：优先 POST {base}/api/embed 批量；端点不存在（404）时
 *  降级逐条 POST {base}/api/embeddings（与 Go 侧 OllamaProvider.Embeddings 逐条口径
 *  一致）。探测结果进程内固化。 */
export class OllamaEmbeddingProvider implements EmbeddingProvider {
  private readonly apiKey: string;
  private readonly baseURL: string;
  private readonly modelName: string; // 命名避开接口方法 model()
  private readonly dimensions: number;
  private readonly timeoutMs: number;
  private readonly fetchImpl: typeof fetch;
  private observed = 0;
  /** /api/embed 可用性：0 未探测、1 可用、2 不可用（回退逐条）——对齐 Go batchState。 */
  private batchState = 0;

  constructor(config: EmbeddingConfig) {
    this.apiKey = config.apiKey ?? ''; // ollama 无需 key
    this.baseURL = (config.baseURL ?? OLLAMA_EMBEDDINGS_DEFAULT_BASE_URL).replace(/\/+$/, '');
    this.modelName = config.model ?? DEFAULT_OLLAMA_EMBEDDING_MODEL;
    this.dimensions = config.dimensions ?? 0;
    this.timeoutMs = config.timeoutMs ?? DEFAULT_EMBEDDING_TIMEOUT_MS;
    this.fetchImpl = config.fetchImpl ?? globalThis.fetch.bind(globalThis);
  }

  async embeddings(texts: string[]): Promise<number[][]> {
    if (texts.length === 0) return [];

    if (this.batchState === 0) {
      try {
        return await this.embedBatch(texts);
      } catch (err) {
        if (isNotFound(err)) {
          this.batchState = 2; // 旧版 ollama 无批量端点 → 固化走逐条
        } else {
          throw err;
        }
      }
    }
    if (this.batchState === 1) {
      return await this.embedBatch(texts);
    }

    // 逐条回退（/api/embeddings，与既有 OllamaProvider.embeddings 口径一致）
    const out: number[][] = [];
    for (const text of texts) {
      const raw = await this.doRequest('/api/embeddings', { model: this.modelName, prompt: text });
      const one = JSON.parse(raw) as { embedding?: number[] };
      out.push(one.embedding ?? []);
    }
    this.recordObserved(out[0]);
    return out;
  }

  private async embedBatch(texts: string[]): Promise<number[][]> {
    const raw = await this.doRequest('/api/embed', { model: this.modelName, input: texts });
    const resp = JSON.parse(raw) as OllamaEmbedBatchResponse;
    if (resp.error) throw new Error(resp.error);
    const embeddings = resp.embeddings ?? [];
    if (embeddings.length !== texts.length) {
      throw new Error(`failed to parse LLM response: /api/embed 返回条数 ${embeddings.length} ≠ 请求条数 ${texts.length}`);
    }
    this.batchState = 1;
    this.recordObserved(embeddings[0]);
    return embeddings;
  }

  dimension(): number {
    return this.dimensions > 0 ? this.dimensions : this.observed;
  }

  model(): string { return this.modelName; }

  semantic(): boolean { return true; }

  private recordObserved(vec?: number[]): void {
    if (this.observed === 0 && vec && vec.length > 0) this.observed = vec.length;
  }

  private async doRequest(path: string, body: unknown): Promise<string> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'User-Agent': 'AgentPrimordia-TS/1.0',
    };
    if (this.apiKey) headers.Authorization = `Bearer ${this.apiKey}`;
    const resp = await this.fetchImpl(this.baseURL + path, {
      method: 'POST',
      headers,
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(this.timeoutMs),
    });
    const text = await resp.text();
    if (!resp.ok) {
      const err = new APIError(text.slice(0, 512) || `HTTP ${resp.status}`, String(resp.status), 'embeddings', resp.status) as APIError & { status: number };
      throw err;
    }
    return text;
  }
}

function isNotFound(err: unknown): boolean {
  return err instanceof APIError && (err as APIError & { status?: number }).status === 404;
}

// ===== 无 key 降级位（词面伪嵌入）=====

const LEXICAL_DIM = 256;
const LEXICAL_MODEL_NAME = 'lexical-fallback-v1';
const FNV_OFFSET_32 = 0x811c9dc5;
const FNV_PRIME_32 = 0x01000193;

/**
 * LexicalEmbedder 是「无 key 降级位」：完全本地、确定性的词面统计伪嵌入，
 * 不是语义嵌入（semantic() 恒 false）。用途边界：
 *  1. 未配置 AP_EMBEDDINGS_* 时 memory 向量路径的兜底；
 *  2. 双线召回基准的回归底档臂（阈值取自实测，见 bench/results/s0-3-recall-*.json）。
 *
 * 算法（与 Go 侧 internal/llm/embedding.go 的 LexicalEmbedder 逐位对齐，不得单侧改动）：
 *  1. 小写化后扫描码点：CJK 连续段产出字符 bigram（段长 1 时产出单字），
 *     拉丁字母/数字/下划线连续段产出整词 token；其余码点为分隔符；
 *  2. FNV-1a 32 位哈希（token 的 UTF-8 字节）映射到 256 维：idx = h & 0xFF，
 *     符号 = (h>>8)&1 ? -1 : +1（符号哈希降低碰撞偏置）；
 *  3. 词频权重取 sqrt（sublinear TF 阻尼）——刻意不用 ln：sqrt 为 IEEE 正确舍入、
 *     双线逐位一致；ln 依赖各平台 libm 舍入，存在跨语言 1ulp 级偏差风险；
 *  4. L2 归一化（sqrt 同上）；空文本/零向量返回全零向量。
 *
 * 注：小写化对 CJK/ASCII 语料双线一致；浮点累加按 token 首现顺序进行，
 * 保证双线求和次序一致（Map 迭代序 = 插入序，与 Go 的 order 切片一致）。
 */
export class LexicalEmbedder implements EmbeddingProvider {
  async embeddings(texts: string[]): Promise<number[][]> {
    return texts.map(lexicalEmbedOne);
  }

  dimension(): number { return LEXICAL_DIM; }

  model(): string { return LEXICAL_MODEL_NAME; }

  semantic(): boolean { return false; }
}

function lexicalIsCJK(cp: number): boolean {
  return (cp >= 0x4e00 && cp <= 0x9fff) || // CJK 统一表意文字
    (cp >= 0x3400 && cp <= 0x4dbf) ||     // CJK 扩展 A
    (cp >= 0xf900 && cp <= 0xfaff);       // CJK 兼容表意文字
}

/** 分词：CJK 字符 bigram + 拉丁整词（for..of 按码点迭代，与 Go range rune 一致）。 */
export function lexicalTokenize(s: string): string[] {
  const lower = s.toLowerCase();
  const tokens: string[] = [];
  let cjk: number[] = [];
  let latin: number[] = [];

  const flushLatin = (): void => {
    if (latin.length > 0) {
      tokens.push(String.fromCodePoint(...latin));
      latin = [];
    }
  };
  const flushCJK = (): void => {
    if (cjk.length === 1) {
      tokens.push(String.fromCodePoint(cjk[0]!));
    } else if (cjk.length > 1) {
      for (let i = 0; i + 1 < cjk.length; i++) {
        tokens.push(String.fromCodePoint(cjk[i]!, cjk[i + 1]!));
      }
    }
    cjk = [];
  };

  for (const ch of lower) {
    const cp = ch.codePointAt(0)!;
    if (lexicalIsCJK(cp)) {
      flushLatin();
      cjk.push(cp);
    } else if ((cp >= 0x61 && cp <= 0x7a) || (cp >= 0x30 && cp <= 0x39) || cp === 0x5f) {
      flushCJK();
      latin.push(cp);
    } else {
      flushLatin();
      flushCJK();
    }
  }
  flushLatin();
  flushCJK();
  return tokens;
}

/** UTF-8 字节编码（手动实现，避免依赖运行时 TextEncoder 差异；与 Go []byte(s) 一致）。 */
function utf8Bytes(s: string): number[] {
  const out: number[] = [];
  for (const ch of s) {
    const c = ch.codePointAt(0)!;
    if (c <= 0x7f) out.push(c);
    else if (c <= 0x7ff) out.push(0xc0 | (c >> 6), 0x80 | (c & 63));
    else if (c <= 0xffff) out.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 63), 0x80 | (c & 63));
    else out.push(0xf0 | (c >> 18), 0x80 | ((c >> 12) & 63), 0x80 | ((c >> 6) & 63), 0x80 | (c & 63));
  }
  return out;
}

/** FNV-1a 32 位哈希（Math.imul 提供 uint32 乘法环绕语义，与 Go uint32 一致）。 */
export function lexicalFnv1a32(s: string): number {
  let h = FNV_OFFSET_32;
  for (const b of utf8Bytes(s)) {
    h = (h ^ b) >>> 0;
    h = Math.imul(h, FNV_PRIME_32) >>> 0;
  }
  return h;
}

/** 单条词面伪嵌入（首现顺序累加 + L2 归一化）。 */
function lexicalEmbedOne(s: string): number[] {
  const counts = new Map<string, number>(); // Map 迭代序 = 首现顺序
  for (const tok of lexicalTokenize(s)) {
    counts.set(tok, (counts.get(tok) ?? 0) + 1);
  }
  const acc = new Array<number>(LEXICAL_DIM).fill(0);
  for (const [tok, n] of counts) {
    const h = lexicalFnv1a32(tok);
    const idx = h & 0xff;
    let w = Math.sqrt(n); // sublinear TF（sqrt 阻尼）
    if ((h >>> 8) & 1) w = -w;
    acc[idx] += w;
  }
  let norm = 0;
  for (const v of acc) norm += v * v;
  norm = Math.sqrt(norm);
  if (norm === 0) return acc; // 空文本 → 全零向量
  return acc.map((v) => v / norm);
}

// ===== 适配与环境装配 =====

/** 将既有「仅 embeddings 能力」的对象升格为 EmbeddingProvider（对齐 Go
 *  NewEmbeddingProviderAdapter）。元信息由调用方声明——semantic 传 true 即承诺
 *  该嵌入器连接真实语义端点，其结果方可计入语义验收数字。 */
export class EmbeddingProviderAdapter implements EmbeddingProvider {
  constructor(
    private readonly inner: { embeddings(texts: string[]): Promise<number[][]> },
    private readonly modelName: string,
    private readonly dim: number,
    private readonly isSemantic: boolean,
  ) {}

  embeddings(texts: string[]): Promise<number[][]> { return this.inner.embeddings(texts); }

  dimension(): number { return this.dim; }

  model(): string { return this.modelName; }

  semantic(): boolean { return this.isSemantic; }
}

/** 从环境变量装配语义嵌入 Provider（对齐 Go NewEmbeddingProviderFromEnv；命名沿用
 *  AP_LLM_* 前缀约定）：
 *  - AP_EMBEDDINGS_PROVIDER: openai（默认）| ollama
 *  - AP_EMBEDDINGS_API_KEY / AP_EMBEDDINGS_BASE_URL / AP_EMBEDDINGS_MODEL
 *  - AP_EMBEDDINGS_DIMENSIONS: 期望维度（可选）
 *  全部未设置时返回 LexicalEmbedder（无 key 降级位）——不配置即词面兜底，
 *  绝不把降级位伪装成语义臂。 */
export function createEmbeddingProviderFromEnv(
  env: Record<string, string | undefined> = process.env,
): EmbeddingProvider {
  const provider = (env.AP_EMBEDDINGS_PROVIDER ?? '').trim().toLowerCase();
  const baseURL = (env.AP_EMBEDDINGS_BASE_URL ?? '').trim();
  const model = (env.AP_EMBEDDINGS_MODEL ?? '').trim();
  const apiKey = env.AP_EMBEDDINGS_API_KEY ?? '';
  const dims = Number.parseInt(env.AP_EMBEDDINGS_DIMENSIONS ?? '', 10);

  if (!provider && !baseURL && !model && !apiKey && Number.isNaN(dims)) {
    return new LexicalEmbedder(); // 未配置 → 降级位兜底
  }
  const effective = provider || 'openai';
  const config: EmbeddingConfig = {
    apiKey,
    baseURL,
    model,
    dimensions: Number.isNaN(dims) ? 0 : dims,
  };
  if (effective === 'openai') return new OpenAIEmbeddingProvider(config);
  if (effective === 'ollama') return new OllamaEmbeddingProvider(config);
  throw new Error(`未知 AP_EMBEDDINGS_PROVIDER "${effective}"（支持 openai|ollama）`);
}

// ===== 语义缓存（S0-3 命中率基线）=====

/** 缓存观测快照（对齐 Go EmbeddingCacheStats）。 */
export interface EmbeddingCacheStats {
  hits: number;
  misses: number;
  evictions: number;
  entries: number;
  /** hits/(hits+misses)；尚无请求时为 0。 */
  hitRate: number;
}

const EMBEDDING_CACHE_KEY_SEP = '\x1f';

/** 嵌入结果 LRU 缓存：key = 模型名 + 分隔符 + 文本（对齐 Go EmbeddingCache）。 */
export class EmbeddingCache {
  private readonly maxEntries: number;
  private readonly entries = new Map<string, number[]>(); // Map 迭代序 = 插入序 → 触达时重插实现 LRU
  private hits = 0;
  private misses = 0;
  private evictions = 0;

  constructor(maxEntries = 1024) {
    this.maxEntries = maxEntries > 0 ? maxEntries : 1024;
  }

  get(model: string, text: string): number[] | undefined {
    const key = model + EMBEDDING_CACHE_KEY_SEP + text;
    const vec = this.entries.get(key);
    if (vec === undefined) {
      this.misses++;
      return undefined;
    }
    // 重插到队尾（Map 迭代序首端最旧 → delete+set 等价 MoveToFront）
    this.entries.delete(key);
    this.entries.set(key, vec);
    this.hits++;
    return [...vec];
  }

  put(model: string, text: string, vec: number[]): void {
    const key = model + EMBEDDING_CACHE_KEY_SEP + text;
    if (this.entries.has(key)) this.entries.delete(key);
    this.entries.set(key, [...vec]);
    while (this.entries.size > this.maxEntries) {
      const oldest = this.entries.keys().next().value;
      if (oldest === undefined) break;
      this.entries.delete(oldest);
      this.evictions++;
    }
  }

  stats(): EmbeddingCacheStats {
    const total = this.hits + this.misses;
    return {
      hits: this.hits,
      misses: this.misses,
      evictions: this.evictions,
      entries: this.entries.size,
      hitRate: total > 0 ? this.hits / total : 0,
    };
  }
}

/** EmbeddingProvider 装饰器：按条缓存嵌入结果。命中/未命中计数即 S0-3
 *  「语义缓存命中率基线」的观测量。批内重复文本去重，只远端调用一次。 */
export class CachedEmbeddingProvider implements EmbeddingProvider {
  private readonly cache: EmbeddingCache;

  constructor(private readonly inner: EmbeddingProvider, maxEntries = 1024) {
    this.cache = new EmbeddingCache(maxEntries);
  }

  async embeddings(texts: string[]): Promise<number[][]> {
    const out: number[][] = new Array(texts.length);
    const missing = new Map<string, number[]>();
    for (let i = 0; i < texts.length; i++) {
      const hit = this.cache.get(this.inner.model(), texts[i]!);
      if (hit !== undefined) {
        out[i] = hit;
      } else {
        const idxs = missing.get(texts[i]!) ?? [];
        idxs.push(i);
        missing.set(texts[i]!, idxs);
      }
    }
    if (missing.size > 0) {
      const uniqueTexts = [...missing.keys()];
      const vecs = await this.inner.embeddings(uniqueTexts);
      if (vecs.length !== uniqueTexts.length) {
        throw new Error(`failed to parse LLM response: embeddings 返回条数 ${vecs.length} ≠ 请求条数 ${uniqueTexts.length}`);
      }
      for (let j = 0; j < uniqueTexts.length; j++) {
        this.cache.put(this.inner.model(), uniqueTexts[j]!, vecs[j]!);
        for (const i of missing.get(uniqueTexts[j]!)!) out[i] = vecs[j]!;
      }
    }
    return out;
  }

  dimension(): number { return this.inner.dimension(); }

  model(): string { return this.inner.model(); }

  /** 缓存不改变语义性：降级位包上缓存仍是降级位。 */
  semantic(): boolean { return this.inner.semantic(); }

  cacheStats(): EmbeddingCacheStats { return this.cache.stats(); }
}