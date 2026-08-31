// embedding-recall.test.ts — S0-3 真实 corpus 双线召回基准（TS 臂）。
//
// 口径（与 Go 侧 agentprimordia/internal/memory/embedding_recall_test.go 逐位对齐）：
//   - 语料：docs/evals/embedding-corpus-v1.json（S0-2 题面台账注册，sha256 冻结）；
//     CI 回归只跑 visible 子集（holdout=false），holdout 子集留给 S0-3 终验；
//   - 嵌入：LexicalEmbedder（无 key 降级位，回归底档臂；语义臂见 describe('语义臂')）；
//     chunk 向量输入 = title + '\n' + text；
//   - 三档固定种子：seed ∈ {7, 8, 9}（v5.1 口径 7+N），种子只影响 HNSW 构建——
//     插入顺序（mulberry32 Fisher-Yates 洗牌）与层级生成随机源（HNSWConfig.random）；
//   - recall@10：|HNSW top-10 ∩ gold| / |gold|（gold ≤ 8）；
//   - 结果写入 sdk/typescript/bench/results/s0-3-recall-ts.json
//     （仅 AP_WRITE_S03_RESULTS=1 时落盘）；双线对账：node scripts/dual-line-recall-check.mjs。
import { describe, it, expect } from 'vitest';
import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { LexicalEmbedder, CachedEmbeddingProvider, createEmbeddingProviderFromEnv } from '../embedding.js';
import { HNSW } from '../../memory/vector-extended.js';
import type { EmbeddingProvider } from '../embedding.js';

const __dirname = dirname(fileURLToPath(import.meta.url));
// src/llm/__tests__ -> 仓库根（与 registry.test.ts 同款相对定位）
const REPO_ROOT = resolve(__dirname, '../../../../../');
const CORPUS_PATH = resolve(REPO_ROOT, 'docs/evals/embedding-corpus-v1.json');
const TS_RESULTS_DIR = resolve(__dirname, '../../../bench/results');
const FLOOR = 0.57; // lexical 底档阈值：与 Go 侧 s03LexicalFloor 同值同公式（实测 0.6254 - 0.05）

/** mulberry32 种子 PRNG（与 tests/shared/cross-language.test.ts#xlMulberry32 及 Go clMulberry32 一致）。 */
function mulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

interface CorpusItem {
  id: string;
  type: 'chunk' | 'query';
  source?: string;
  title: string;
  text: string;
  term?: string;
  gold?: string[];
  holdout: boolean;
}

const SEEDS = [7, 8, 9];

function loadVisibleCorpus(): { chunks: CorpusItem[]; queries: CorpusItem[] } {
  const items = JSON.parse(readFileSync(CORPUS_PATH, 'utf-8')) as CorpusItem[];
  const chunks: CorpusItem[] = [];
  const queries: CorpusItem[] = [];
  for (const it of items) {
    if (it.holdout) continue; // CI 回归口径：只跑 visible 子集
    if (it.type === 'chunk') chunks.push(it);
    else if (it.type === 'query') queries.push(it);
  }
  if (chunks.length === 0 || queries.length === 0) {
    throw new Error(`visible 子集为空: chunks=${chunks.length} queries=${queries.length}`);
  }
  return { chunks, queries };
}

/** chunk 嵌入输入约定（双线一致）：title + 换行 + 正文。 */
function chunkEmbedInput(c: CorpusItem): string {
  return c.title + '\n' + c.text;
}

/** mulberry32 Fisher-Yates：种子决定 chunk 插入顺序（构建顺序臂）。 */
function shuffledIndices(n: number, seed: number): number[] {
  const rng = mulberry32(seed);
  const idx = Array.from({ length: n }, (_, i) => i);
  for (let i = n - 1; i > 0; i--) {
    const j = Math.min(i, Math.floor(rng() * (i + 1)));
    [idx[i], idx[j]] = [idx[j]!, idx[i]!];
  }
  return idx;
}

/** 单臂三档种子召回（对齐 Go runS03Recall）。 */
async function runRecall(
  corpus: { chunks: CorpusItem[]; queries: CorpusItem[] },
  emb: EmbeddingProvider,
): Promise<number[]> {
  const chunkVecs = await emb.embeddings(corpus.chunks.map(chunkEmbedInput));
  const queryVecs = await emb.embeddings(corpus.queries.map((q) => q.text));

  const recalls: number[] = [];
  for (const seed of SEEDS) {
    const idx = new HNSW({
      maxConnections: 16,
      maxConnectionsLayer0: 32, // 对齐 Go：layer0 = MaxConnections*2
      efConstruction: 200,
      efSearch: 50,
      random: mulberry32(seed),
    });
    for (const i of shuffledIndices(corpus.chunks.length, seed)) {
      idx.insert(corpus.chunks[i]!.id, chunkVecs[i]!);
    }
    let total = 0;
    for (let qi = 0; qi < corpus.queries.length; qi++) {
      const q = corpus.queries[qi]!;
      const gold = new Set(q.gold ?? []);
      const results = idx.search(queryVecs[qi]!, 10);
      let hits = 0;
      for (const r of results) if (gold.has(r.id)) hits++;
      total += hits / (q.gold?.length ?? 1);
    }
    recalls.push(total / corpus.queries.length);
  }
  return recalls;
}

interface RecallResultJSON {
  arm: string;
  semantic: boolean;
  corpus: string;
  query_scope: string;
  seeds: number[];
  topK: number;
  chunks: number;
  queries: number;
  tiers: Array<{ seed: number; recall_at_10: number }>;
  mean_recall_at_10: number;
  cache_baseline: { hits: number; misses: number; hit_rate: number };
}

function writeResultJSON(name: string, res: RecallResultJSON): void {
  mkdirSync(TS_RESULTS_DIR, { recursive: true });
  writeFileSync(resolve(TS_RESULTS_DIR, name), JSON.stringify(res, null, 2) + '\n');
}

describe('Embedding corpus recall — lexical 臂（回归底档）', () => {
  it('三档固定种子 recall@10 ≥ 底档阈值 0.57（实测 0.6254 - 0.05）', async () => {
    const corpus = loadVisibleCorpus();
    const emb = new LexicalEmbedder();
    expect(emb.semantic()).toBe(false); // 降级位臂数字不进语义验收

    const recalls = await runRecall(corpus, emb);
    const mean = recalls.reduce((a, b) => a + b, 0) / recalls.length;
    recalls.forEach((r, i) => {
      console.log(`tier seed=${SEEDS[i]} recall@10 = ${r.toFixed(4)}`);
    });
    console.log(`lexical 臂 mean recall@10 = ${mean.toFixed(4)}（${corpus.chunks.length} chunks / ${corpus.queries.length} queries）`);

    expect(mean).toBeGreaterThanOrEqual(FLOOR);

    // 语义缓存命中率基线：冷跑 + 暖跑各一轮查询集 → 精确 0.5
    const cache = new CachedEmbeddingProvider(new LexicalEmbedder());
    await cache.embeddings(corpus.queries.map((q) => q.text));
    await cache.embeddings(corpus.queries.map((q) => q.text));
    const stats = cache.cacheStats();
    expect(stats.hits).toBe(corpus.queries.length);
    expect(stats.misses).toBe(corpus.queries.length);
    expect(stats.hitRate).toBe(0.5);

    if (process.env.AP_WRITE_S03_RESULTS === '1') {
      writeResultJSON('s0-3-recall-ts.json', {
        arm: 'lexical-fallback',
        semantic: false,
        corpus: 'docs/evals/embedding-corpus-v1.json',
        query_scope: 'visible-only',
        seeds: SEEDS,
        topK: 10,
        chunks: corpus.chunks.length,
        queries: corpus.queries.length,
        tiers: recalls.map((r, i) => ({ seed: SEEDS[i]!, recall_at_10: r })),
        mean_recall_at_10: mean,
        cache_baseline: { hits: stats.hits, misses: stats.misses, hit_rate: stats.hitRate },
      });
      expect(existsSync(resolve(TS_RESULTS_DIR, 's0-3-recall-ts.json'))).toBe(true);
    }
  });
});

describe('语义臂（需真实端点，S0-3 验收 ≥0.95）', () => {
  it('无 secrets 时降级豁免；端点就位时同一语料同一三档种子真跑', async (ctx) => {
    if (!process.env.AP_EMBEDDINGS_BASE_URL && !process.env.AP_EMBEDDINGS_PROVIDER) {
      console.warn('A1 运营依赖未就位 → 降级豁免（docs/V7路线图.md §九）：未设置 AP_EMBEDDINGS_BASE_URL/AP_EMBEDDINGS_PROVIDER，语义臂 recall@10 ≥0.95 待端点就位后真跑');
      ctx.skip();
      return;
    }
    const corpus = loadVisibleCorpus();
    const emb = createEmbeddingProviderFromEnv(process.env);
    expect(emb.semantic()).toBe(true); // 语义臂必须 Semantic()=true（降级位不得计入语义验收）
    console.log(`语义臂端点: model=${emb.model()} dimension=${emb.dimension()}`);

    const recalls = await runRecall(corpus, emb);
    const mean = recalls.reduce((a, b) => a + b, 0) / recalls.length;
    expect(mean).toBeGreaterThanOrEqual(0.95);

    if (process.env.AP_WRITE_S03_RESULTS === '1') {
      writeResultJSON('s0-3-recall-ts-semantic.json', {
        arm: 'semantic',
        semantic: true,
        corpus: 'docs/evals/embedding-corpus-v1.json',
        query_scope: 'visible-only',
        seeds: SEEDS,
        topK: 10,
        chunks: corpus.chunks.length,
        queries: corpus.queries.length,
        tiers: recalls.map((r, i) => ({ seed: SEEDS[i]!, recall_at_10: r })),
        mean_recall_at_10: mean,
        cache_baseline: { hits: 0, misses: 0, hit_rate: 0 },
      });
    }
  });
});
