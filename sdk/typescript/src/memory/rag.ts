import { VectorStore } from './vector.js';
import type { MemoryEpisode } from '../types.js';
import type { Provider } from '../llm/provider.js';
import { TextSplitter } from '../tools/builtin/index.js';

// ===== RAG Document =====

export interface RAGDocument {
  id: string;
  content: string;
  metadata?: Record<string, string>;
  embedding?: number[];
  score?: number;
  source?: string;
  /** 检索来源列表（"fts" 和/或 "vector"），与 Go 端 RAGResult.Sources 对齐 */
  sources?: string[];
}

// ===== 融合模式（与 Go 端 HybridFusionMode 对齐）=====

/** 混合检索融合模式
 * - linear: 线性加权融合（向后兼容默认）
 * - rrf: Reciprocal Rank Fusion，基于排名而非分数，鲁棒性更强
 */
export type FusionMode = 'linear' | 'rrf';

/** RAG 检索融合配置（与 Go 端 RAGFusionConfig 对齐）
 *
 * 字段可通过构造函数或 setFusionConfig 调整。
 */
export interface RAGFusionConfig {
  /** 融合模式 */
  fusionMode?: FusionMode;
  /** FTS 通道权重（仅 linear 模式生效，默认 0.4） */
  ftsWeight?: number;
  /** 向量通道权重（仅 linear 模式生效，默认 0.6） */
  vectorWeight?: number;
  /** RRF 平滑常数（仅 rrf 模式生效，默认 60）
   * 经验值来自原始 RRF 论文（Cormack et al., 2009）
   */
  rrfK?: number;
  /** 单通道预取数量，用于增加融合召回率（默认 5）
   * 最终 fetchK = min(topK + overFetchSize, 2 * topK)
   */
  overFetchSize?: number;
}

/** 默认融合配置 */
export function defaultFusionConfig(): Required<RAGFusionConfig> {
  return {
    fusionMode: 'linear',
    ftsWeight: 0.4,
    vectorWeight: 0.6,
    rrfK: 60,
    overFetchSize: 5,
  };
}

// ===== RAG Store — Hybrid Search (FTS + Vector) =====

export interface RAGStoreConfig {
  ftsWeight?: number;   // default: 0.4
  vectorWeight?: number; // default: 0.6
  topK?: number;         // default: 5
  minScore?: number;     // default: 0.3
  chunkSize?: number;    // default: 1000
  chunkOverlap?: number; // default: 200
  /** 融合配置（与 Go 端 RAGFusionConfig 对齐） */
  fusion?: RAGFusionConfig;
}

export class RAGStore {
  private documents: Map<string, RAGDocument> = new Map();
  private vectorStore: VectorStore;
  private ftsIndex: Map<string, Set<string>> = new Map(); // word -> doc IDs
  /** 预计算的文档 TF 缓存：docId -> word -> count，避免每次搜索重新 tokenize */
  private docTFCache: Map<string, Map<string, number>> = new Map();
  private config: Required<RAGStoreConfig>;
  private fusionConfig: Required<RAGFusionConfig>;
  private splitter: TextSplitter;

  constructor(vectorDimensions: number = 384, config?: RAGStoreConfig) {
    this.vectorStore = new VectorStore(vectorDimensions);
    const defaults = defaultFusionConfig();
    this.fusionConfig = {
      fusionMode: config?.fusion?.fusionMode ?? defaults.fusionMode,
      ftsWeight: config?.fusion?.ftsWeight ?? config?.ftsWeight ?? defaults.ftsWeight,
      vectorWeight: config?.fusion?.vectorWeight ?? config?.vectorWeight ?? defaults.vectorWeight,
      rrfK: config?.fusion?.rrfK ?? defaults.rrfK,
      overFetchSize: config?.fusion?.overFetchSize ?? defaults.overFetchSize,
    };
    this.config = {
      ftsWeight: this.fusionConfig.ftsWeight,
      vectorWeight: this.fusionConfig.vectorWeight,
      topK: config?.topK ?? 5,
      minScore: config?.minScore ?? 0.3,
      chunkSize: config?.chunkSize ?? 1000,
      chunkOverlap: config?.chunkOverlap ?? 200,
      fusion: this.fusionConfig,
    };
    this.splitter = new TextSplitter({
      chunkSize: this.config.chunkSize,
      chunkOverlap: this.config.chunkOverlap,
    });
  }

  /** 动态调整 RAG 检索融合配置（与 Go 端 SetFusionConfig 对齐）
   *
   * 典型用法：根据 A/B 测试结果调整融合权重，或在 QPS 下降时切换到 RRF 模式
   */
  setFusionConfig(cfg: RAGFusionConfig): void {
    const defaults = defaultFusionConfig();
    this.fusionConfig = {
      fusionMode: cfg.fusionMode ?? defaults.fusionMode,
      ftsWeight: cfg.ftsWeight ?? defaults.ftsWeight,
      vectorWeight: cfg.vectorWeight ?? defaults.vectorWeight,
      rrfK: cfg.rrfK ?? defaults.rrfK,
      overFetchSize: cfg.overFetchSize ?? defaults.overFetchSize,
    };
  }

  /** 获取当前融合配置 */
  getFusionConfig(): Required<RAGFusionConfig> {
    return { ...this.fusionConfig };
  }

  /** Add a document to the RAG store (with optional embedding). */
  async addDocument(doc: RAGDocument, embedding?: number[]): Promise<void> {
    // Split into chunks if document is large
    const chunks = this.splitter.split(doc.content);

    for (let i = 0; i < chunks.length; i++) {
      const chunkId = chunks.length > 1 ? `${doc.id}_chunk_${i}` : doc.id;
      const chunkDoc: RAGDocument = {
        id: chunkId,
        content: chunks[i],
        metadata: { ...doc.metadata, parentDoc: doc.id, chunkIndex: String(i) },
        source: doc.source ?? doc.id,
      };

      this.documents.set(chunkId, chunkDoc);

      // Add to vector store if embedding provided
      if (embedding) {
        this.vectorStore.add(chunkId, embedding, chunkDoc.metadata);
      }

      // Add to FTS index
      this.indexFTS(chunkId, chunks[i]);
    }
  }

  /** Add multiple documents with embeddings from a provider. */
  async addDocuments(docs: RAGDocument[], embedFn: (text: string) => Promise<number[]>): Promise<void> {
    for (const doc of docs) {
      const embedding = await embedFn(doc.content);
      await this.addDocument(doc, embedding);
    }
  }

  /** 计算预取数量（over-fetch 以提升融合召回率） */
  private computeFetchK(topK: number): number {
    const fetchK = topK + this.fusionConfig.overFetchSize;
    return Math.min(fetchK, 2 * topK);
  }

  /** Hybrid search: combine FTS and vector search results.
   *
   * 融合策略由 fusionConfig.fusionMode 决定（与 Go 端对齐）：
   * - linear: 线性加权融合（向后兼容）
   * - rrf: Reciprocal Rank Fusion（推荐用于生产）
   */
  async hybridSearch(query: string, topK?: number, queryEmbedding?: number[]): Promise<RAGDocument[]> {
    const k = topK ?? this.config.topK;
    const fetchK = this.computeFetchK(k);

    // FTS search (over-fetch)
    const ftsResults = this.ftsSearch(query, fetchK);

    if (this.fusionConfig.fusionMode === 'rrf') {
      return this.hybridSearchRRF(query, ftsResults, k, queryEmbedding);
    }
    return this.hybridSearchLinear(ftsResults, k, queryEmbedding);
  }

  /** 线性加权融合（与 Go 端 hybridSearchLinear 对齐）
   * FTS 通道和向量通道分别计算分数，按权重相加。
   */
  private hybridSearchLinear(
    ftsResults: { id: string; score: number }[],
    topK: number,
    queryEmbedding?: number[],
  ): RAGDocument[] {
    const fetchK = this.computeFetchK(topK);
    const ftsMap = new Map<string, number>();
    for (const { id, score } of ftsResults) {
      ftsMap.set(id, score * this.fusionConfig.ftsWeight);
    }

    // Vector search (if embedding available)
    const vectorMap = new Map<string, number>();
    if (queryEmbedding) {
      const vecResults = this.vectorStore.search(queryEmbedding, fetchK);
      for (const { id, score } of vecResults) {
        vectorMap.set(id, score * this.fusionConfig.vectorWeight);
      }
    }

    // Merge results
    const allIds = new Set([...ftsMap.keys(), ...vectorMap.keys()]);
    const merged: RAGDocument[] = [];

    for (const id of allIds) {
      const ftsScore = ftsMap.get(id) ?? 0;
      const vecScore = vectorMap.get(id) ?? 0;
      const totalScore = ftsScore + vecScore;

      if (totalScore >= this.config.minScore) {
        const doc = this.documents.get(id);
        if (doc) {
          const sources: string[] = [];
          if (ftsMap.has(id)) sources.push('fts');
          if (vectorMap.has(id)) sources.push('vector');
          merged.push({ ...doc, score: totalScore, sources });
        }
      }
    }

    // Sort by score descending
    merged.sort((a, b) => (b.score ?? 0) - (a.score ?? 0));
    return merged.slice(0, topK);
  }

  /** Reciprocal Rank Fusion 融合算法（与 Go 端 hybridSearchRRF 对齐）
   *
   * 公式：RRF_score(d) = Σ 1 / (k + rank_i(d))
   * 其中 rank_i(d) 是文档 d 在第 i 个通道中的排名（1-based），k 为平滑常数（默认 60）
   *
   * 优势：基于排名而非分数，不受通道量纲影响，对长尾 query 鲁棒性更强。
   * 论文：Cormack, G. V., Clarke, C. L., & Buettcher, S. (2009).
   * "Reciprocal rank fusion outperforms condorcet and individual rank learning methods."
   */
  private hybridSearchRRF(
    query: string,
    ftsResults: { id: string; score: number }[],
    topK: number,
    queryEmbedding?: number[],
  ): RAGDocument[] {
    const k = this.fusionConfig.rrfK > 0 ? this.fusionConfig.rrfK : 60;
    const fetchK = this.computeFetchK(topK);

    // 记录每个文档在各通道的排名（1-based），用于独立累加 RRF 分数
    const ftsRanks = new Map<string, number>();
    const vecRanks = new Map<string, number>();
    const episodes = new Map<string, RAGDocument>();
    const sourcesMap = new Map<string, string[]>();

    // FTS 通道排名
    for (let i = 0; i < ftsResults.length; i++) {
      const id = ftsResults[i]!.id;
      const rank = i + 1;
      if (!ftsRanks.has(id) || rank < ftsRanks.get(id)!) {
        ftsRanks.set(id, rank);
      }
      const doc = this.documents.get(id);
      if (doc) {
        episodes.set(id, doc);
        const sources = sourcesMap.get(id) ?? [];
        if (!sources.includes('fts')) sources.push('fts');
        sourcesMap.set(id, sources);
      }
    }

    // 向量通道排名
    if (queryEmbedding) {
      const vecResults = this.vectorStore.search(queryEmbedding, fetchK);
      for (let i = 0; i < vecResults.length; i++) {
        const id = vecResults[i]!.id;
        const rank = i + 1;
        if (!vecRanks.has(id) || rank < vecRanks.get(id)!) {
          vecRanks.set(id, rank);
        }
        if (!episodes.has(id)) {
          const doc = this.documents.get(id);
          if (doc) episodes.set(id, doc);
        }
        const sources = sourcesMap.get(id) ?? [];
        if (!sources.includes('vector')) sources.push('vector');
        sourcesMap.set(id, sources);
      }
    }

    // 计算 RRF 分数并构造结果
    // 公式：RRF_score(d) = Σ 1 / (k + rank_i(d))，对每个命中通道独立累加
    const results: RAGDocument[] = [];
    for (const [id, doc] of episodes) {
      let rrfScore = 0;
      const ftsRank = ftsRanks.get(id);
      if (ftsRank !== undefined) {
        rrfScore += 1.0 / (k + ftsRank);
      }
      const vecRank = vecRanks.get(id);
      if (vecRank !== undefined) {
        rrfScore += 1.0 / (k + vecRank);
      }
      results.push({ ...doc, score: rrfScore, sources: sourcesMap.get(id) ?? [] });
    }

    // Sort by score descending
    results.sort((a, b) => (b.score ?? 0) - (a.score ?? 0));
    return results.slice(0, topK);
  }

  /** FTS-only search (keyword matching with TF-IDF-like scoring).
   *
   * 使用预计算的 docTFCache 避免 O(N×M) 重复 tokenize，性能提升显著。
   */
  private ftsSearch(query: string, topK: number): { id: string; score: number }[] {
    const queryWords = this.tokenize(query);
    if (queryWords.length === 0) return [];

    const scores: Map<string, number> = new Map();
    const totalDocs = this.documents.size;

    for (const word of queryWords) {
      const docIds = this.ftsIndex.get(word);
      if (!docIds || docIds.size === 0) continue;

      // IDF scoring
      const idf = Math.log((totalDocs + 1) / (docIds.size + 1)) + 1;

      for (const docId of docIds) {
        // 使用预计算的 TF 缓存，避免每次搜索重新 tokenize
        const tfMap = this.docTFCache.get(docId);
        if (!tfMap) continue;

        const tf = (tfMap.get(word) ?? 0) / (tfMap.get('__total__') ?? 1);

        const currentScore = scores.get(docId) ?? 0;
        scores.set(docId, currentScore + tf * idf);
      }
    }

    const results = Array.from(scores.entries())
      .map(([id, score]) => ({ id, score }))
      .sort((a, b) => b.score - a.score)
      .slice(0, topK);

    return results;
  }

  private indexFTS(docId: string, content: string): void {
    const words = this.tokenize(content);
    const tfMap = new Map<string, number>();
    let total = 0;

    for (const word of words) {
      // 更新 FTS 倒排索引
      if (!this.ftsIndex.has(word)) {
        this.ftsIndex.set(word, new Set());
      }
      this.ftsIndex.get(word)!.add(docId);

      // 预计算 TF（词频统计）
      tfMap.set(word, (tfMap.get(word) ?? 0) + 1);
      total++;
    }
    tfMap.set('__total__', total);
    this.docTFCache.set(docId, tfMap);
  }

  private tokenize(text: string): string[] {
    return text
      .toLowerCase()
      .replace(/[^\w\s\u4e00-\u9fff]/g, ' ')
      .split(/\s+/)
      .filter((w) => w.length > 1);
  }

  /** Get document by ID. */
  getDocument(id: string): RAGDocument | undefined {
    return this.documents.get(id);
  }

  /** List all documents. */
  listDocuments(): RAGDocument[] {
    return Array.from(this.documents.values());
  }

  /** Delete a document. */
  deleteDocument(id: string): boolean {
    const doc = this.documents.get(id);
    if (!doc) return false;

    // Remove from FTS index and TF cache
    const tfMap = this.docTFCache.get(id);
    if (tfMap) {
      for (const word of tfMap.keys()) {
        if (word === '__total__') continue;
        this.ftsIndex.get(word)?.delete(id);
      }
      this.docTFCache.delete(id);
    }

    // Remove from vector store
    this.vectorStore.delete(id);

    // Remove from documents
    this.documents.delete(id);
    return true;
  }

  /** Clear all documents. */
  clear(): void {
    this.documents.clear();
    this.ftsIndex.clear();
    this.docTFCache.clear();
    this.vectorStore = new VectorStore(this.vectorStore.dimensions());
  }

  /** Get store statistics. */
  stats(): { totalDocuments: number; totalChunks: number; vectorCount: number; vocabularySize: number } {
    return {
      totalDocuments: new Set(Array.from(this.documents.values()).map((d) => d.metadata?.parentDoc ?? d.id)).size,
      totalChunks: this.documents.size,
      vectorCount: this.vectorStore.count(),
      vocabularySize: this.ftsIndex.size,
    };
  }
}

// ===== RAG Pipeline =====

export interface RAGPipelineConfig {
  ragStore: RAGStore;
  embedFn: (text: string) => Promise<number[]>;
  topK?: number;
  minScore?: number;
}

export class RAGPipeline {
  private config: RAGPipelineConfig;

  constructor(config: RAGPipelineConfig) {
    this.config = config;
  }

  /** Index a document into the RAG pipeline. */
  async index(content: string, id?: string, metadata?: Record<string, string>): Promise<void> {
    const docId = id ?? `doc-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const embedding = await this.config.embedFn(content);
    await this.config.ragStore.addDocument({ id: docId, content, metadata, embedding });
  }

  /** Index multiple documents. */
  async indexBatch(documents: { content: string; id?: string; metadata?: Record<string, string> }[]): Promise<void> {
    for (const doc of documents) {
      await this.index(doc.content, doc.id, doc.metadata);
    }
  }

  /** Query the RAG pipeline for relevant context. */
  async query(query: string): Promise<RAGDocument[]> {
    const embedding = await this.config.embedFn(query);
    return this.config.ragStore.hybridSearch(query, this.config.topK ?? 5, embedding);
  }

  /** Format RAG results for prompt injection. */
  formatContext(docs: RAGDocument[]): string {
    if (docs.length === 0) return '';
    let result = '=== Relevant Knowledge ===\n';
    for (let i = 0; i < docs.length; i++) {
      const doc = docs[i];
      result += `[${i + 1} | score: ${(doc.score ?? 0).toFixed(2)}${doc.source ? ` | ${doc.source}` : ''}]\n${doc.content}\n\n`;
    }
    result += '=== End Knowledge ===\n';
    return result;
  }
}

// ===== RAG Reranker =====

export interface RerankOptions {
  topK?: number;
  model?: string;
  deduplicate?: boolean;
}

export class RAGReranker {
  private provider?: Provider;

  constructor(provider?: Provider) {
    this.provider = provider;
  }

  /** Rerank documents by relevance to the query. */
  async rerank(query: string, docs: RAGDocument[], opts?: RerankOptions): Promise<RAGDocument[]> {
    const topK = opts?.topK ?? docs.length;

    // If no provider, use simple keyword overlap scoring
    if (!this.provider) {
      return this.simpleRerank(query, docs, topK, opts?.deduplicate ?? true);
    }

    // Use LLM for reranking
    return this.llmRerank(query, docs, topK);
  }

  private simpleRerank(query: string, docs: RAGDocument[], topK: number, deduplicate: boolean): RAGDocument[] {
    const queryWords = new Set(query.toLowerCase().split(/\s+/).filter((w) => w.length > 1));
    const scored = docs.map((doc) => {
      const docWords = doc.content.toLowerCase().split(/\s+/);
      const overlap = docWords.filter((w) => queryWords.has(w)).length;
      const overlapScore = overlap / Math.max(queryWords.size, 1);
      const finalScore = (doc.score ?? 0) * 0.5 + overlapScore * 0.5;
      return { ...doc, score: finalScore };
    });

    let result = scored.sort((a, b) => (b.score ?? 0) - (a.score ?? 0));

    if (deduplicate) {
      const seen = new Set<string>();
      result = result.filter((d) => {
        const key = d.content.slice(0, 100);
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
      });
    }

    return result.slice(0, topK);
  }

  private async llmRerank(query: string, docs: RAGDocument[], topK: number): Promise<RAGDocument[]> {
    if (!this.provider) return docs.slice(0, topK);

    const prompt = `Rank the following documents by relevance to the query. Return only the document numbers in order of relevance, comma-separated.\n\nQuery: ${query}\n\n`;

    const docList = docs.map((d, i) => `Doc ${i + 1}: ${d.content.slice(0, 200)}...`).join('\n\n');

    try {
      const resp = await this.provider.complete({
        messages: [
          { role: 'system', content: 'You are a document relevance ranker. Return only comma-separated numbers.' },
          { role: 'user', content: prompt + docList },
        ],
        temperature: 0,
      });

      // Parse ranking from response
      const numbers = resp.content.match(/\d+/g);
      if (!numbers) return docs.slice(0, topK);

      const ranked: RAGDocument[] = [];
      for (const num of numbers) {
        const idx = parseInt(num) - 1;
        if (idx >= 0 && idx < docs.length && !ranked.includes(docs[idx])) {
          ranked.push(docs[idx]);
        }
      }

      // Add any unranked docs
      for (const doc of docs) {
        if (!ranked.includes(doc)) ranked.push(doc);
      }

      return ranked.slice(0, topK);
    } catch {
      return docs.slice(0, topK);
    }
  }
}

// ===== P4-B1: MMR (Maximal Marginal Relevance) Reranker =====
// 与 Go 端 MMRReranker 对齐。
// 在相关性和多样性之间取得平衡：选择与查询相关但与已选结果不太相似的结果。
// 适用于需要避免重复内容的 RAG 场景。
//
// 使用方式：
//   const reranker = new MMRReranker({ lambda: 0.7 });
//   const reranked = await reranker.rerank(query, docs, { topK: 5 });

export interface MMRConfig {
  /** 相关性权重 [0, 1]，越高越偏向相关性；默认 0.7 */
  lambda?: number;
}

export interface MMRRerankOptions {
  /** 返回的最大文档数 */
  topK?: number;
  /** 是否去重，默认 true */
  deduplicate?: boolean;
}

export class MMRReranker {
  private lambda: number;

  constructor(config?: MMRConfig) {
    const lambda = config?.lambda ?? 0.7;
    // Clamp to [0, 1]
    this.lambda = Math.max(0, Math.min(1, lambda));
  }

  /** 返回重排序器名称 */
  get name(): string {
    return 'mmr';
  }

  /** 执行 MMR 重排序 */
  async rerank(
    query: string,
    docs: RAGDocument[],
    opts?: MMRRerankOptions,
  ): Promise<RAGDocument[]> {
    const topK = opts?.topK ?? docs.length;
    const deduplicate = opts?.deduplicate ?? true;

    if (docs.length <= 1) return docs.slice(0, topK);

    // 去重
    let candidates = docs;
    if (deduplicate) {
      const seen = new Set<string>();
      candidates = docs.filter((d) => {
        const key = d.content.slice(0, 100);
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
      });
    }

    if (candidates.length <= 1) return candidates.slice(0, topK);

    // 计算查询与文档的相关性分数
    const queryWords = new Set(query.toLowerCase().split(/\s+/).filter((w) => w.length > 1));
    const relevanceScores = candidates.map((doc) => {
      const docWords = doc.content.toLowerCase().split(/\s+/);
      const overlap = docWords.filter((w) => queryWords.has(w)).length;
      const overlapScore = overlap / Math.max(queryWords.size, 1);
      // 结合原有分数（如向量相似度分数）
      return (doc.score ?? 0) * 0.5 + overlapScore * 0.5;
    });

    // MMR 选择过程
    const selected: number[] = [];
    const remaining = Array.from({ length: candidates.length }, (_, i) => i);

    // 第一个选择相关性最高的
    let bestIdx = 0;
    let bestScore = -Infinity;
    for (let i = 0; i < remaining.length; i++) {
      const idx = remaining[i];
      if (relevanceScores[idx] > bestScore) {
        bestScore = relevanceScores[idx];
        bestIdx = i;
      }
    }
    selected.push(remaining[bestIdx]);
    remaining.splice(bestIdx, 1);

    // 逐步选择：最大化 lambda * relevance - (1-lambda) * max_similarity
    while (selected.length < topK && remaining.length > 0) {
      let bestMMRIdx = 0;
      let bestMMRScore = -Infinity;

      for (let i = 0; i < remaining.length; i++) {
        const idx = remaining[i];
        // 计算与已选文档的最大相似度
        let maxSim = 0;
        for (const selIdx of selected) {
          const sim = jaccardSimilarity(
            candidates[idx].content,
            candidates[selIdx].content,
          );
          if (sim > maxSim) maxSim = sim;
        }
        // MMR 分数 = lambda * relevance - (1-lambda) * max_similarity
        const mmrScore = this.lambda * relevanceScores[idx] - (1 - this.lambda) * maxSim;
        if (mmrScore > bestMMRScore) {
          bestMMRScore = mmrScore;
          bestMMRIdx = i;
        }
      }

      selected.push(remaining[bestMMRIdx]);
      remaining.splice(bestMMRIdx, 1);
    }

    return selected.map((idx) => ({
      ...candidates[idx],
      score: relevanceScores[idx],
    }));
  }
}

/** Jaccard 相似度：交集大小 / 并集大小 */
function jaccardSimilarity(a: string, b: string): number {
  const setA = new Set(a.toLowerCase().split(/\s+/).filter((w) => w.length > 1));
  const setB = new Set(b.toLowerCase().split(/\s+/).filter((w) => w.length > 1));
  if (setA.size === 0 && setB.size === 0) return 1;
  let intersection = 0;
  for (const w of setA) {
    if (setB.has(w)) intersection++;
  }
  const union = setA.size + setB.size - intersection;
  return union > 0 ? intersection / union : 0;
}


// ===== Auto Summarizer =====

export interface SummarizerConfig {
  provider: Provider;
  model?: string;
  maxSummaryLength?: number;
  language?: string;
}

export class Summarizer {
  private config: SummarizerConfig;

  constructor(config: SummarizerConfig) {
    this.config = config;
  }

  /** Generate a summary of the given text. */
  async summarize(text: string, opts?: { maxLength?: number; focus?: string }): Promise<{ summary: string; topics: string[] }> {
    const maxLen = opts?.maxLength ?? this.config.maxSummaryLength ?? 200;
    const focus = opts?.focus ? ` Focus on: ${opts.focus}.` : '';

    const resp = await this.config.provider.complete({
      messages: [
        {
          role: 'system',
          content: `You are a summarization assistant. Summarize the following text in at most ${maxLen} characters.${focus} Also extract 3-5 topic tags. Return JSON: {"summary": "...", "topics": ["tag1", "tag2"]}`,
        },
        { role: 'user', content: text },
      ],
      model: this.config.model,
      temperature: 0,
    });

    try {
      const jsonMatch = resp.content.match(/\{[\s\S]*\}/);
      if (jsonMatch) {
        const parsed = JSON.parse(jsonMatch[0]);
        return {
          summary: parsed.summary ?? resp.content,
          topics: parsed.topics ?? [],
        };
      }
    } catch { /* JSON parse failed, fall through to raw response */ }

    return { summary: resp.content, topics: [] };
  }

  /** Summarize a conversation history. */
  async summarizeConversation(messages: { role: string; content: string }[]): Promise<{ summary: string; topics: string[] }> {
    const conversation = messages.map((m) => `${m.role}: ${m.content}`).join('\n');
    return this.summarize(conversation);
  }
}

// ===== Memory Compressor =====

export class MemoryCompressor {
  private summarizer: Summarizer;

  constructor(summarizer: Summarizer) {
    this.summarizer = summarizer;
  }

  /** Compress memory episodes by summarizing old ones. */
  async compress(episodes: MemoryEpisode[], opts?: { keepRecent?: number }): Promise<{
    kept: MemoryEpisode[];
    summarized: { summary: string; topics: string[]; episodeCount: number };
  }> {
    const keepRecent = opts?.keepRecent ?? 10;

    if (episodes.length <= keepRecent) {
      return { kept: episodes, summarized: { summary: '', topics: [], episodeCount: 0 } };
    }

    const toSummarize = episodes.slice(0, episodes.length - keepRecent);
    const toKeep = episodes.slice(episodes.length - keepRecent);

    const text = toSummarize.map((e) => `${e.role}: ${e.content}`).join('\n');
    const { summary, topics } = await this.summarizer.summarize(text);

    return {
      kept: toKeep,
      summarized: { summary, topics, episodeCount: toSummarize.length },
    };
  }
}
