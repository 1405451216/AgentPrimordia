import { VectorStore } from './vector.js';
import type { Memory } from './store.js';
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
}

// ===== RAG Store — Hybrid Search (FTS + Vector) =====

export interface RAGStoreConfig {
  ftsWeight?: number;   // default: 0.4
  vectorWeight?: number; // default: 0.6
  topK?: number;         // default: 5
  minScore?: number;     // default: 0.3
  chunkSize?: number;    // default: 1000
  chunkOverlap?: number; // default: 200
}

export class RAGStore {
  private documents: Map<string, RAGDocument> = new Map();
  private vectorStore: VectorStore;
  private ftsIndex: Map<string, Set<string>> = new Map(); // word -> doc IDs
  private config: Required<RAGStoreConfig>;
  private splitter: TextSplitter;

  constructor(vectorDimensions: number = 384, config?: RAGStoreConfig) {
    this.vectorStore = new VectorStore(vectorDimensions);
    this.config = {
      ftsWeight: config?.ftsWeight ?? 0.4,
      vectorWeight: config?.vectorWeight ?? 0.6,
      topK: config?.topK ?? 5,
      minScore: config?.minScore ?? 0.3,
      chunkSize: config?.chunkSize ?? 1000,
      chunkOverlap: config?.chunkOverlap ?? 200,
    };
    this.splitter = new TextSplitter({
      chunkSize: this.config.chunkSize,
      chunkOverlap: this.config.chunkOverlap,
    });
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

  /** Hybrid search: combine FTS and vector search results. */
  async hybridSearch(query: string, topK?: number, queryEmbedding?: number[]): Promise<RAGDocument[]> {
    const k = topK ?? this.config.topK;

    // FTS search
    const ftsResults = this.ftsSearch(query, k * 2);
    const ftsMap = new Map<string, number>();
    for (const { id, score } of ftsResults) {
      ftsMap.set(id, score * this.config.ftsWeight);
    }

    // Vector search (if embedding available)
    const vectorMap = new Map<string, number>();
    if (queryEmbedding) {
      const vecResults = this.vectorStore.search(queryEmbedding, k * 2);
      for (const { id, score } of vecResults) {
        vectorMap.set(id, score * this.config.vectorWeight);
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
          merged.push({ ...doc, score: totalScore });
        }
      }
    }

    // Sort by score descending
    merged.sort((a, b) => (b.score ?? 0) - (a.score ?? 0));
    return merged.slice(0, k);
  }

  /** FTS-only search (keyword matching with TF-IDF-like scoring). */
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
        const doc = this.documents.get(docId);
        if (!doc) continue;

        // TF scoring (term frequency in document)
        const docWords = this.tokenize(doc.content);
        const tf = docWords.filter((w) => w === word).length / docWords.length;

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
    for (const word of words) {
      if (!this.ftsIndex.has(word)) {
        this.ftsIndex.set(word, new Set());
      }
      this.ftsIndex.get(word)!.add(docId);
    }
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

    // Remove from FTS index
    const words = this.tokenize(doc.content);
    for (const word of words) {
      this.ftsIndex.get(word)?.delete(id);
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
    } catch {}

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
