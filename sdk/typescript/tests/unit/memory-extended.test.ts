import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { VectorStore } from '../../src/memory/vector.js';
import {
  RAGStore,
  RAGPipeline,
  RAGReranker,
  Summarizer,
  MemoryCompressor,
  type RAGDocument,
} from '../../src/memory/rag.js';
import {
  LLMSummarizer,
  SimpleSummarizer,
} from '../../src/memory/summarizer.js';
import {
  Compressor,
  LLMCompressSummarizer,
} from '../../src/memory/compressor.js';
import {
  HNSW,
  ConversationalMemory,
  SharedStore,
  MilvusProvider,
  QdrantProvider,
} from '../../src/memory/vector-extended.js';
import {
  EnhancedRAGPipeline,
  SimpleTextLoader,
  createSplitter,
  availableStrategies,
  registerSplitter,
} from '../../src/memory/rag-pipeline.js';
import { InMemoryStore } from '../../src/memory/store.js';
import { MockProvider } from '../../src/llm/provider.js';
import type { MemoryEpisode } from '../../src/types.js';

// ===== VectorStore Tests =====

describe('VectorStore', () => {
  it('should add and search vectors', () => {
    const store = new VectorStore(3);
    store.add('a', [1, 0, 0]);
    store.add('b', [0, 1, 0]);
    store.add('c', [0, 0, 1]);

    const results = store.search([1, 0, 0], 2);
    expect(results).toHaveLength(2);
    expect(results[0].id).toBe('a');
    expect(results[0].score).toBeGreaterThan(0.99);
  });

  it('should return empty for empty store', () => {
    const store = new VectorStore(3);
    const results = store.search([1, 0, 0], 5);
    expect(results).toHaveLength(0);
  });

  it('should throw on dimension mismatch', () => {
    const store = new VectorStore(3);
    expect(() => store.add('a', [1, 0])).toThrow('dimension mismatch');
  });

  it('should throw on non-finite values', () => {
    const store = new VectorStore(3);
    expect(() => store.add('a', [1, NaN, 0])).toThrow('non-finite');
  });

  it('should throw on query dimension mismatch', () => {
    const store = new VectorStore(3);
    store.add('a', [1, 0, 0]);
    expect(() => store.search([1, 0], 5)).toThrow('dimension mismatch');
  });

  it('should throw on non-finite query values', () => {
    const store = new VectorStore(3);
    store.add('a', [1, 0, 0]);
    expect(() => store.search([1, Infinity, 0], 5)).toThrow('non-finite');
  });

  it('should get vector by id', () => {
    const store = new VectorStore(3);
    store.add('a', [1, 0, 0], { source: 'test' });
    const result = store.get('a');
    expect(result).toBeDefined();
    expect(result!.vector).toEqual([1, 0, 0]);
    expect(result!.metadata).toEqual({ source: 'test' });
  });

  it('should return undefined for missing id', () => {
    const store = new VectorStore(3);
    expect(store.get('missing')).toBeUndefined();
  });

  it('should delete vector', () => {
    const store = new VectorStore(3);
    store.add('a', [1, 0, 0]);
    store.add('b', [0, 1, 0]);
    expect(store.delete('a')).toBe(true);
    expect(store.get('a')).toBeUndefined();
    expect(store.count()).toBe(1);
  });

  it('should return false for deleting missing id', () => {
    const store = new VectorStore(3);
    expect(store.delete('missing')).toBe(false);
  });

  it('should count vectors', () => {
    const store = new VectorStore(3);
    expect(store.count()).toBe(0);
    store.add('a', [1, 0, 0]);
    store.add('b', [0, 1, 0]);
    expect(store.count()).toBe(2);
  });

  it('should return dimensions', () => {
    const store = new VectorStore(128);
    expect(store.dimensions()).toBe(128);
  });

  it('should handle re-adding existing id', () => {
    const store = new VectorStore(3);
    store.add('a', [1, 0, 0]);
    store.add('a', [0, 1, 0]);
    expect(store.count()).toBe(1);
    const result = store.get('a');
    expect(result!.vector).toEqual([0, 1, 0]);
  });

  it('should handle many vectors (brute force path)', () => {
    const store = new VectorStore(3);
    for (let i = 0; i < 50; i++) {
      store.add(`v${i}`, [Math.random(), Math.random(), Math.random()]);
    }
    const results = store.search([0.5, 0.5, 0.5], 5);
    expect(results).toHaveLength(5);
    // Results should be sorted by score descending
    for (let i = 1; i < results.length; i++) {
      expect(results[i - 1].score).toBeGreaterThanOrEqual(results[i].score);
    }
  });

  it('should use HNSW search for larger datasets', () => {
    const store = new VectorStore(3, { M: 8, efConstruction: 50, efSearch: 20 });
    for (let i = 0; i < 150; i++) {
      store.add(`v${i}`, [
        Math.sin(i * 0.1),
        Math.cos(i * 0.1),
        Math.sin(i * 0.2),
      ]);
    }
    const results = store.search([0.5, 0.5, 0.5], 5);
    expect(results.length).toBeGreaterThan(0);
    expect(results.length).toBeLessThanOrEqual(5);
  });

  it('should handle delete of entry point', () => {
    const store = new VectorStore(3);
    store.add('a', [1, 0, 0]);
    store.add('b', [0, 1, 0]);
    store.add('c', [0, 0, 1]);
    // Delete first node (entry point)
    store.delete('a');
    // Search should still work
    const results = store.search([0, 1, 0], 2);
    expect(results.length).toBeGreaterThan(0);
    expect(results[0].id).toBe('b');
  });
});

// ===== RAGStore Tests =====

describe('RAGStore', () => {
  it('should add and get documents', async () => {
    const store = new RAGStore(3);
    await store.addDocument({ id: 'doc1', content: 'Hello world test' });
    const doc = store.getDocument('doc1');
    expect(doc).toBeDefined();
    expect(doc!.content).toBe('Hello world test');
  });

  it('should list documents', async () => {
    const store = new RAGStore(3);
    await store.addDocument({ id: 'd1', content: 'doc one' });
    await store.addDocument({ id: 'd2', content: 'doc two' });
    const docs = store.listDocuments();
    expect(docs).toHaveLength(2);
  });

  it('should delete documents', async () => {
    const store = new RAGStore(3);
    await store.addDocument({ id: 'd1', content: 'doc one' });
    expect(store.deleteDocument('d1')).toBe(true);
    expect(store.getDocument('d1')).toBeUndefined();
    expect(store.deleteDocument('missing')).toBe(false);
  });

  it('should clear all', async () => {
    const store = new RAGStore(3);
    await store.addDocument({ id: 'd1', content: 'doc one' });
    await store.addDocument({ id: 'd2', content: 'doc two' });
    store.clear();
    expect(store.listDocuments()).toHaveLength(0);
    expect(store.stats().totalDocuments).toBe(0);
  });

  it('should return stats', async () => {
    const store = new RAGStore(3);
    await store.addDocument({ id: 'd1', content: 'hello world' });
    await store.addDocument({ id: 'd2', content: 'test data' });
    const stats = store.stats();
    expect(stats.totalDocuments).toBe(2);
    expect(stats.totalChunks).toBe(2);
    expect(stats.vocabularySize).toBeGreaterThan(0);
  });

  it('should do hybrid search with FTS only', async () => {
    const store = new RAGStore(3, { minScore: 0 });
    await store.addDocument({ id: 'd1', content: 'The quick brown fox' });
    await store.addDocument({ id: 'd2', content: 'The lazy dog sleeps' });
    const results = await store.hybridSearch('quick fox');
    expect(results.length).toBeGreaterThan(0);
    expect(results[0].id).toBe('d1');
  });

  it('should do hybrid search with vector', async () => {
    const store = new RAGStore(3, { minScore: 0 });
    await store.addDocument({ id: 'd1', content: 'hello world' }, [1, 0, 0]);
    await store.addDocument({ id: 'd2', content: 'goodbye world' }, [0, 1, 0]);
    const results = await store.hybridSearch('hello', 5, [1, 0, 0]);
    expect(results.length).toBeGreaterThan(0);
    expect(results[0].id).toBe('d1');
  });

  it('should add documents with embedFn', async () => {
    const store = new RAGStore(3);
    const embedFn = vi.fn().mockResolvedValue([1, 0, 0]);
    await store.addDocuments(
      [{ id: 'd1', content: 'test' }, { id: 'd2', content: 'data' }],
      embedFn
    );
    expect(embedFn).toHaveBeenCalledTimes(2);
    expect(store.stats().vectorCount).toBe(2);
  });

  it('should split large documents into chunks', async () => {
    const store = new RAGStore(3, { chunkSize: 10, chunkOverlap: 2 });
    const longText = 'a'.repeat(50);
    await store.addDocument({ id: 'big', content: longText });
    const docs = store.listDocuments();
    expect(docs.length).toBeGreaterThan(1);
    const stats = store.stats();
    expect(stats.totalChunks).toBeGreaterThan(1);
  });

  it('should return empty results for empty query', async () => {
    const store = new RAGStore(3);
    await store.addDocument({ id: 'd1', content: 'test' });
    const results = await store.hybridSearch('');
    expect(results).toHaveLength(0);
  });
});

// ===== RAGPipeline Tests =====

describe('RAGPipeline', () => {
  it('should index and query documents', async () => {
    const ragStore = new RAGStore(3, { minScore: 0 });
    const embedFn = vi.fn().mockImplementation(async (text: string) => {
      if (text.includes('hello')) return [1, 0, 0];
      return [0, 1, 0];
    });
    const pipeline = new RAGPipeline({ ragStore, embedFn });

    await pipeline.index('hello world', 'doc1');
    await pipeline.index('goodbye world', 'doc2');

    const results = await pipeline.query('hello');
    expect(results.length).toBeGreaterThan(0);
  });

  it('should index batch', async () => {
    const ragStore = new RAGStore(3, { minScore: 0 });
    const embedFn = vi.fn().mockResolvedValue([1, 0, 0]);
    const pipeline = new RAGPipeline({ ragStore, embedFn });

    await pipeline.indexBatch([
      { content: 'doc one', id: 'd1' },
      { content: 'doc two', id: 'd2' },
    ]);

    expect(ragStore.listDocuments().length).toBeGreaterThanOrEqual(2);
  });

  it('should format context', () => {
    const ragStore = new RAGStore(3);
    const pipeline = new RAGPipeline({ ragStore, embedFn: async () => [0, 0, 0] });
    const docs: RAGDocument[] = [
      { id: '1', content: 'first doc', score: 0.9, source: 'test' },
      { id: '2', content: 'second doc', score: 0.8 },
    ];
    const formatted = pipeline.formatContext(docs);
    expect(formatted).toContain('Relevant Knowledge');
    expect(formatted).toContain('first doc');
    expect(formatted).toContain('second doc');
    expect(formatted).toContain('End Knowledge');
  });

  it('should format empty context', () => {
    const ragStore = new RAGStore(3);
    const pipeline = new RAGPipeline({ ragStore, embedFn: async () => [0, 0, 0] });
    expect(pipeline.formatContext([])).toBe('');
  });
});

// ===== RAGReranker Tests =====

describe('RAGReranker', () => {
  it('should rerank without provider (simple)', async () => {
    const reranker = new RAGReranker();
    const docs: RAGDocument[] = [
      { id: '1', content: 'hello world test', score: 0.5 },
      { id: '2', content: 'goodbye world', score: 0.8 },
      { id: '3', content: 'hello there', score: 0.3 },
    ];
    const results = await reranker.rerank('hello test', docs);
    expect(results.length).toBeGreaterThan(0);
    // Doc 1 has more overlap with query
    expect(results[0].id).toBe('1');
  });

  it('should deduplicate documents', async () => {
    const reranker = new RAGReranker();
    const docs: RAGDocument[] = [
      { id: '1', content: 'same content here', score: 0.5 },
      { id: '2', content: 'same content here', score: 0.5 },
      { id: '3', content: 'different content', score: 0.3 },
    ];
    const results = await reranker.rerank('content', docs, { deduplicate: true });
    // Should have fewer results due to dedup
    expect(results.length).toBeLessThan(docs.length);
  });

  it('should limit to topK', async () => {
    const reranker = new RAGReranker();
    const docs: RAGDocument[] = [
      { id: '1', content: 'hello', score: 0.5 },
      { id: '2', content: 'world', score: 0.3 },
      { id: '3', content: 'test', score: 0.1 },
    ];
    const results = await reranker.rerank('hello', docs, { topK: 2 });
    expect(results.length).toBeLessThanOrEqual(2);
  });

  it('should rerank with LLM provider', async () => {
    const provider = new MockProvider({ response: '2, 1, 3' });
    const reranker = new RAGReranker(provider);
    const docs: RAGDocument[] = [
      { id: '1', content: 'first document', score: 0.5 },
      { id: '2', content: 'second document', score: 0.3 },
      { id: '3', content: 'third document', score: 0.1 },
    ];
    const results = await reranker.rerank('query', docs);
    expect(results.length).toBeGreaterThan(0);
    // LLM said 2, 1, 3
    expect(results[0].id).toBe('2');
  });

  it('should handle LLM rerank with no numbers in response', async () => {
    const provider = new MockProvider({ response: 'no numbers here' });
    const reranker = new RAGReranker(provider);
    const docs: RAGDocument[] = [
      { id: '1', content: 'first', score: 0.5 },
      { id: '2', content: 'second', score: 0.3 },
    ];
    const results = await reranker.rerank('query', docs);
    expect(results.length).toBeGreaterThan(0);
  });

  it('should handle LLM rerank error', async () => {
    const provider = new MockProvider({ error: true });
    const reranker = new RAGReranker(provider);
    const docs: RAGDocument[] = [
      { id: '1', content: 'first', score: 0.5 },
    ];
    const results = await reranker.rerank('query', docs);
    expect(results.length).toBeGreaterThan(0);
  });
});

// ===== Summarizer (rag.ts) Tests =====

describe('Summarizer (rag.ts)', () => {
  it('should summarize text', async () => {
    const provider = new MockProvider({
      response: '{"summary":"This is a summary","topics":["ai","test"]}',
    });
    const summarizer = new Summarizer({ provider, model: 'gpt-4' });
    const result = await summarizer.summarize('Long text to summarize');
    expect(result.summary).toBe('This is a summary');
    expect(result.topics).toContain('ai');
  });

  it('should handle non-JSON response', async () => {
    const provider = new MockProvider({ response: 'Just a plain text summary' });
    const summarizer = new Summarizer({ provider });
    const result = await summarizer.summarize('Long text');
    expect(result.summary).toBe('Just a plain text summary');
    expect(result.topics).toEqual([]);
  });

  it('should summarize with focus', async () => {
    let capturedMessages: any;
    const provider = {
      complete: vi.fn().mockImplementation(async (req: any) => {
        capturedMessages = req.messages;
        return { id: '1', content: '{"summary":"focused","topics":[]}', role: 'assistant', usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 } };
      }),
      callTools: vi.fn(),
      info: () => ({ name: 'mock', provider: 'mock', maxContext: 4096, supportsTools: true, supportsStreaming: true }),
    };
    const summarizer = new Summarizer({ provider: provider as any });
    await summarizer.summarize('text', { focus: 'key points' });
    expect(capturedMessages[0].content).toContain('Focus on: key points');
  });

  it('should summarize conversation', async () => {
    const provider = new MockProvider({
      response: '{"summary":"Conversation summary","topics":["chat"]}',
    });
    const summarizer = new Summarizer({ provider });
    const result = await summarizer.summarizeConversation([
      { role: 'user', content: 'Hello' },
      { role: 'assistant', content: 'Hi there' },
    ]);
    expect(result.summary).toBe('Conversation summary');
  });
});

// ===== MemoryCompressor Tests =====

describe('MemoryCompressor', () => {
  it('should not compress when below keepRecent', async () => {
    const provider = new MockProvider({ response: '{"summary":"summary","topics":[]}' });
    const summarizer = new Summarizer({ provider });
    const compressor = new MemoryCompressor(summarizer);

    const episodes: MemoryEpisode[] = Array.from({ length: 5 }, (_, i) => ({
      id: `ep-${i}`,
      sessionId: 's1',
      role: 'user',
      content: `message ${i}`,
      createdAt: new Date(Date.now() + i).toISOString(),
    }));

    const result = await compressor.compress(episodes, { keepRecent: 10 });
    expect(result.kept).toHaveLength(5);
    expect(result.summarized.episodeCount).toBe(0);
  });

  it('should compress old episodes', async () => {
    const provider = new MockProvider({ response: '{"summary":"compressed","topics":["old"]}' });
    const summarizer = new Summarizer({ provider });
    const compressor = new MemoryCompressor(summarizer);

    const episodes: MemoryEpisode[] = Array.from({ length: 15 }, (_, i) => ({
      id: `ep-${i}`,
      sessionId: 's1',
      role: 'user',
      content: `message ${i}`,
      createdAt: new Date(Date.now() + i * 1000).toISOString(),
    }));

    const result = await compressor.compress(episodes, { keepRecent: 5 });
    expect(result.kept).toHaveLength(5);
    expect(result.summarized.episodeCount).toBe(10);
    expect(result.summarized.summary).toBe('compressed');
  });
});

// ===== LLMSummarizer Tests =====

describe('LLMSummarizer', () => {
  it('should extract summary from LLM response', async () => {
    const provider = new MockProvider({
      response: 'This is the summary line\ntopics: ai, test, summary',
    });
    const summarizer = new LLMSummarizer({ provider, model: 'gpt-4' });
    const result = await summarizer.extractSummary('Some content to summarize');
    expect(result.summary).toBe('This is the summary line');
    expect(result.topics).toBe('ai, test, summary');
  });

  it('should fallback on LLM error', async () => {
    const provider = new MockProvider({ error: true });
    const summarizer = new LLMSummarizer({ provider, maxRetries: 1 });
    const result = await summarizer.extractSummary('Some content here');
    expect(result.summary).toContain('Some content here');
    expect(result.topics).toBe('');
  });

  it('should handle empty response', async () => {
    const provider = new MockProvider({ response: '' });
    const summarizer = new LLMSummarizer({ provider });
    const result = await summarizer.extractSummary('test content');
    expect(result.summary).toBe('');
  });

  it('should set model with withModel', () => {
    const provider = new MockProvider();
    const summarizer = new LLMSummarizer({ provider });
    const returned = summarizer.withModel('gpt-4-mini');
    expect(returned).toBe(summarizer);
  });

  it('should handle topics on first line', async () => {
    const provider = new MockProvider({
      response: 'topics: tag1, tag2\nSummary text here',
    });
    const summarizer = new LLMSummarizer({ provider });
    const result = await summarizer.extractSummary('content');
    expect(result.topics).toBe('tag1, tag2');
    expect(result.summary).toBe('Summary text here');
  });
});

// ===== SimpleSummarizer Tests =====

describe('SimpleSummarizer', () => {
  it('should extract summary without LLM', async () => {
    const summarizer = new SimpleSummarizer(200);
    const result = await summarizer.extractSummary(
      'This is the first sentence. This is the second sentence. And a third one here.'
    );
    expect(result.summary).toBeTruthy();
    expect(result.summary.length).toBeLessThanOrEqual(200);
  });

  it('should extract keywords as topics', async () => {
    const summarizer = new SimpleSummarizer(500);
    const result = await summarizer.extractSummary(
      'machine learning is great. machine learning rocks. python is awesome for machine learning'
    );
    expect(result.topics).toBeTruthy();
    const topics = result.topics.split(',');
    expect(topics.length).toBeGreaterThan(0);
  });

  it('should handle short content', async () => {
    const summarizer = new SimpleSummarizer(100);
    const result = await summarizer.extractSummary('Short text');
    expect(result.summary).toBeTruthy();
  });
});

// ===== Compressor Tests =====

describe('Compressor', () => {
  function createMockStore(episodes: MemoryEpisode[]) {
    const store = new InMemoryStore();
    for (const ep of episodes) {
      store.add(ep);
    }
    return store;
  }

  it('should not compress when below minEpisodes', async () => {
    const mockSummarizer = {
      summarize: vi.fn().mockResolvedValue({ text: 'summary', tags: ['tag'] }),
    };
    const compressor = new Compressor({
      summarizer: mockSummarizer as any,
      windowSize: 5,
      minEpisodes: 10,
    });

    const episodes: MemoryEpisode[] = Array.from({ length: 5 }, (_, i) => ({
      id: `ep-${i}`,
      sessionId: 's1',
      role: 'user',
      content: `message ${i}`,
      createdAt: new Date(Date.now() + i * 1000).toISOString(),
    }));

    const store = createMockStore(episodes);
    const result = await compressor.compress(store);
    expect(result).toBeNull();
  });

  it('should compress old episodes', async () => {
    const mockSummarizer = {
      summarize: vi.fn().mockResolvedValue({ text: 'compressed summary', tags: ['old', 'chat'] }),
    };
    const compressor = new Compressor({
      summarizer: mockSummarizer as any,
      windowSize: 5,
      minEpisodes: 2,
    });

    const episodes: MemoryEpisode[] = Array.from({ length: 15 }, (_, i) => ({
      id: `ep-${i}`,
      sessionId: 's1',
      role: 'user',
      content: `message ${i}`,
      createdAt: new Date(Date.now() + i * 1000).toISOString(),
    }));

    const store = createMockStore(episodes);
    const result = await compressor.compress(store);
    expect(result).not.toBeNull();
    expect(result!.text).toBe('compressed summary');
    expect(result!.tags).toEqual(['old', 'chat']);
  });

  it('should not compress when cutoff is too small', async () => {
    const mockSummarizer = {
      summarize: vi.fn().mockResolvedValue({ text: 'summary', tags: [] }),
    };
    const compressor = new Compressor({
      summarizer: mockSummarizer as any,
      windowSize: 12,
      minEpisodes: 5,
    });

    const episodes: MemoryEpisode[] = Array.from({ length: 15 }, (_, i) => ({
      id: `ep-${i}`,
      sessionId: 's1',
      role: 'user',
      content: `message ${i}`,
      createdAt: new Date(Date.now() + i * 1000).toISOString(),
    }));

    const store = createMockStore(episodes);
    // store.list({}) returns max 10 episodes, cutoff = 10 - 12 = -2 <= 0
    const result = await compressor.compress(store);
    expect(result).toBeNull();
  });
});

// ===== LLMCompressSummarizer Tests =====

describe('LLMCompressSummarizer', () => {
  it('should summarize episodes', async () => {
    const mockExtractor = {
      extractSummary: vi.fn().mockResolvedValue({
        summary: 'episode summary',
        topics: 'tag1,tag2',
      }),
    };
    const summarizer = new LLMCompressSummarizer(mockExtractor as any);
    const episodes: MemoryEpisode[] = [
      { id: '1', sessionId: 's', role: 'user', content: 'hello', createdAt: '' },
      { id: '2', sessionId: 's', role: 'assistant', content: 'hi', summary: 'greeting', topics: 'chat', createdAt: '' },
    ];
    const result = await summarizer.summarize(episodes);
    expect(result.text).toBe('episode summary');
    expect(result.tags).toEqual(['tag1', 'tag2']);
  });
});

// ===== HNSW Tests =====

describe('HNSW', () => {
  it('should insert and search', () => {
    const hnsw = new HNSW({ maxConnections: 4 });
    hnsw.insert('a', [1, 0, 0]);
    hnsw.insert('b', [0, 1, 0]);
    hnsw.insert('c', [0, 0, 1]);
    const results = hnsw.search([1, 0, 0], 2);
    expect(results.length).toBeGreaterThan(0);
    expect(results[0].id).toBe('a');
  });

  it('should return empty for empty index', () => {
    const hnsw = new HNSW();
    expect(hnsw.search([1, 0], 5)).toHaveLength(0);
  });

  it('should remove nodes', () => {
    const hnsw = new HNSW();
    hnsw.insert('a', [1, 0]);
    hnsw.insert('b', [0, 1]);
    expect(hnsw.remove('a')).toBe(true);
    expect(hnsw.size()).toBe(1);
    expect(hnsw.remove('missing')).toBe(false);
  });

  it('should return size', () => {
    const hnsw = new HNSW();
    expect(hnsw.size()).toBe(0);
    hnsw.insert('a', [1, 0]);
    expect(hnsw.size()).toBe(1);
  });

  it('should handle many insertions', () => {
    const hnsw = new HNSW({ maxConnections: 8 });
    for (let i = 0; i < 50; i++) {
      hnsw.insert(`v${i}`, [Math.sin(i), Math.cos(i)]);
    }
    const results = hnsw.search([0.5, 0.5], 5);
    expect(results.length).toBeGreaterThan(0);
  });
});

// ===== ConversationalMemory Tests =====

describe('ConversationalMemory', () => {
  it('should add and get episodes', async () => {
    const mem = new ConversationalMemory();
    const ep: MemoryEpisode = {
      id: 'ep1',
      sessionId: 's1',
      role: 'user',
      content: 'Hello world',
      createdAt: new Date().toISOString(),
    };
    await mem.add(ep);
    const result = await mem.get('ep1');
    expect(result).not.toBeNull();
    expect(result!.content).toBe('Hello world');
  });

  it('should search episodes', async () => {
    const mem = new ConversationalMemory({ autoSummarize: false });
    await mem.add({ id: '1', sessionId: 's1', role: 'user', content: 'hello world', createdAt: new Date().toISOString() });
    await mem.add({ id: '2', sessionId: 's1', role: 'assistant', content: 'goodbye world', createdAt: new Date().toISOString() });
    const results = await mem.search('hello');
    expect(results).toHaveLength(1);
    expect(results[0].content).toBe('hello world');
  });

  it('should filter by sessionId', async () => {
    const mem = new ConversationalMemory({ autoSummarize: false });
    await mem.add({ id: '1', sessionId: 's1', role: 'user', content: 'hello', createdAt: new Date().toISOString() });
    await mem.add({ id: '2', sessionId: 's2', role: 'user', content: 'hello', createdAt: new Date().toISOString() });
    const results = await mem.search('hello', { sessionId: 's1' });
    expect(results).toHaveLength(1);
  });

  it('should delete episodes', async () => {
    const mem = new ConversationalMemory();
    await mem.add({ id: '1', sessionId: 's1', role: 'user', content: 'hello', createdAt: new Date().toISOString() });
    await mem.delete('1');
    expect(await mem.get('1')).toBeNull();
  });

  it('should count by session', async () => {
    const mem = new ConversationalMemory({ autoSummarize: false });
    await mem.add({ id: '1', sessionId: 's1', role: 'user', content: 'a', createdAt: new Date().toISOString() });
    await mem.add({ id: '2', sessionId: 's1', role: 'user', content: 'b', createdAt: new Date().toISOString() });
    await mem.add({ id: '3', sessionId: 's2', role: 'user', content: 'c', createdAt: new Date().toISOString() });
    expect(await mem.count('s1')).toBe(2);
    expect(await mem.count('s2')).toBe(1);
  });

  it('should list episodes', async () => {
    const mem = new ConversationalMemory({ autoSummarize: false });
    await mem.add({ id: '1', sessionId: 's1', role: 'user', content: 'a', createdAt: '2024-01-01T00:00:00Z' });
    await mem.add({ id: '2', sessionId: 's1', role: 'user', content: 'b', createdAt: '2024-01-02T00:00:00Z' });
    const results = await mem.list({ sessionId: 's1' });
    expect(results).toHaveLength(2);
  });

  it('should update summary', async () => {
    const mem = new ConversationalMemory();
    await mem.add({ id: '1', sessionId: 's1', role: 'user', content: 'hello', createdAt: new Date().toISOString() });
    await mem.updateSummary('1', 'greeting', 'chat,hello');
    const ep = await mem.get('1');
    expect(ep!.summary).toBe('greeting');
    expect(ep!.topics).toBe('chat,hello');
  });

  it('should set importance', async () => {
    const mem = new ConversationalMemory();
    await mem.add({ id: '1', sessionId: 's1', role: 'user', content: 'hello', createdAt: new Date().toISOString() });
    await mem.setImportance('1', 0.8);
    const ep = await mem.get('1');
    expect(ep!.importance).toBe(0.8);
  });

  it('should search by tag', async () => {
    const mem = new ConversationalMemory({ autoSummarize: false });
    await mem.add({ id: '1', sessionId: 's1', role: 'user', content: 'hello', topics: 'chat,greeting', createdAt: new Date().toISOString() });
    await mem.add({ id: '2', sessionId: 's1', role: 'user', content: 'bye', topics: 'farewell', createdAt: new Date().toISOString() });
    const results = await mem.searchByTag('chat');
    expect(results).toHaveLength(1);
    expect(results[0].id).toBe('1');
  });

  it('should get important episodes', async () => {
    const mem = new ConversationalMemory({ autoSummarize: false });
    await mem.add({ id: '1', sessionId: 's1', role: 'user', content: 'low', importance: 0.2, createdAt: new Date().toISOString() });
    await mem.add({ id: '2', sessionId: 's1', role: 'user', content: 'high', importance: 0.9, createdAt: new Date().toISOString() });
    const results = await mem.getImportant(0.5, 10);
    expect(results).toHaveLength(1);
    expect(results[0].id).toBe('2');
  });

  it('should get timeline', async () => {
    const mem = new ConversationalMemory({ autoSummarize: false });
    const now = new Date().toISOString();
    await mem.add({ id: '1', sessionId: 's1', role: 'user', content: 'today', createdAt: now });
    const timeline = await mem.getTimeline(1);
    const todayKey = now.slice(0, 10);
    expect(timeline[todayKey]).toBeDefined();
    expect(timeline[todayKey].length).toBeGreaterThan(0);
  });

  it('should cleanup expired', async () => {
    const mem = new ConversationalMemory({ autoSummarize: false });
    const old = new Date(Date.now() - 10 * 86400000).toISOString();
    await mem.add({ id: '1', sessionId: 's1', role: 'user', content: 'old', createdAt: old });
    await mem.add({ id: '2', sessionId: 's1', role: 'user', content: 'new', createdAt: new Date().toISOString() });
    const deleted = await mem.cleanupExpired(5);
    expect(deleted).toBe(1);
    expect(await mem.get('1')).toBeNull();
  });

  it('should return stats', async () => {
    const mem = new ConversationalMemory({ autoSummarize: false });
    await mem.add({ id: '1', sessionId: 's1', role: 'user', content: 'a', createdAt: new Date().toISOString() });
    await mem.add({ id: '2', sessionId: 's2', role: 'user', content: 'b', createdAt: new Date().toISOString() });
    const stats = await mem.stats();
    expect(stats.totalEpisodes).toBe(2);
    expect(stats.totalSessions).toBe(2);
  });

  it('should close', () => {
    const mem = new ConversationalMemory();
    mem.close();
    // After close, should be empty
    expect(mem.stats()).resolves.toHaveProperty('totalEpisodes', 0);
  });

  it('should get history', async () => {
    const mem = new ConversationalMemory({ autoSummarize: false, maxTurns: 3 });
    for (let i = 0; i < 5; i++) {
      await mem.add({ id: `ep${i}`, sessionId: 's1', role: 'user', content: `msg ${i}`, createdAt: new Date(Date.now() + i).toISOString() });
    }
    const history = mem.getHistory('s1', 3);
    expect(history).toHaveLength(3);
  });

  it('should auto-summarize when threshold exceeded', async () => {
    const mem = new ConversationalMemory({ autoSummarize: true, maxTurns: 3, summaryThreshold: 5 });
    for (let i = 0; i < 7; i++) {
      await mem.add({ id: `ep${i}`, sessionId: 's1', role: 'user', content: `message ${i}`, createdAt: new Date(Date.now() + i).toISOString() });
    }
    // Should have summarized old messages
    const history = mem.getHistory('s1', 100);
    // The first few messages should be replaced by a summary
    expect(history.some((e) => e.id.startsWith('summary-'))).toBe(true);
  });
});

// ===== SharedStore Tests =====

describe('SharedStore', () => {
  it('should set and get values', () => {
    const store = new SharedStore();
    store.set('key1', 'value1');
    expect(store.get('key1')?.value).toBe('value1');
  });

  it('should delete values', () => {
    const store = new SharedStore();
    store.set('key1', 'value1');
    expect(store.delete('key1')).toBe(true);
    expect(store.get('key1')).toBeUndefined();
    expect(store.delete('missing')).toBe(false);
  });

  it('should check existence', () => {
    const store = new SharedStore();
    store.set('key1', 'value1');
    expect(store.has('key1')).toBe(true);
    expect(store.has('missing')).toBe(false);
  });

  it('should list keys', () => {
    const store = new SharedStore();
    store.set('a', 1);
    store.set('b', 2);
    expect(store.keys()).toEqual(['a', 'b']);
  });

  it('should watch for changes', () => {
    const store = new SharedStore();
    const callback = vi.fn();
    const unwatch = store.watch('key1', callback);
    store.set('key1', 'value1');
    expect(callback).toHaveBeenCalledWith(expect.objectContaining({ value: 'value1' }));
    store.delete('key1');
    expect(callback).toHaveBeenCalledWith(null);
    unwatch();
    store.set('key1', 'value2');
    expect(callback).toHaveBeenCalledTimes(2); // Not called after unwatch
  });

  it('should acquire and release lock', async () => {
    const store = new SharedStore();
    const release = await store.lock('resource1', 1000);
    expect(release).not.toBeNull();
    release!();
  });

  it('should return null on lock timeout', async () => {
    const store = new SharedStore();
    const release1 = await store.lock('resource1', 100);
    expect(release1).not.toBeNull();
    // Try to acquire again with short timeout
    const release2 = await store.lock('resource1', 50);
    expect(release2).toBeNull();
    release1!();
  });

  it('should support TTL', () => {
    return new Promise<void>((done) => {
      const store = new SharedStore();
      store.set('temp', 'value', { ttlMs: 50 });
      expect(store.has('temp')).toBe(true);
      setTimeout(() => {
        expect(store.has('temp')).toBe(false);
        done();
      }, 100);
    });
  });
});

// ===== EnhancedRAGPipeline Tests =====

describe('EnhancedRAGPipeline', () => {
  it('should ingest text with default splitter', async () => {
    const ragStore = new RAGStore(3);
    const pipeline = new EnhancedRAGPipeline({ ragStore });
    const result = await pipeline.ingest('This is a test document');
    expect(result.ingested).toBeGreaterThan(0);
    expect(result.failed).toBe(0);
    expect(result.source).toBe('This is a test document');
  });

  it('should use custom loader', async () => {
    const ragStore = new RAGStore(3);
    const loader = {
      load: vi.fn().mockResolvedValue([
        { id: 'doc1', content: 'loaded content', source: 'custom' },
      ]),
    };
    const pipeline = new EnhancedRAGPipeline({ ragStore, loader: loader as any });
    const result = await pipeline.ingest('source');
    expect(result.ingested).toBeGreaterThan(0);
    expect(loader.load).toHaveBeenCalledWith('source');
  });

  it('should support different split strategies', () => {
    const strategies = availableStrategies();
    expect(strategies).toContain('character');
    expect(strategies).toContain('recursive');
    expect(strategies).toContain('line');
    expect(strategies).toContain('sentence');
  });

  it('should create splitter by strategy', () => {
    const splitter = createSplitter('character', { chunkSize: 100, chunkOverlap: 20 });
    const chunks = splitter.split('a'.repeat(250));
    expect(chunks.length).toBeGreaterThan(1);
  });

  it('should throw on unknown strategy', () => {
    expect(() => createSplitter('unknown' as any)).toThrow('未知切分策略');
  });

  it('should register custom splitter', () => {
    registerSplitter('code' as any, (cfg) => ({
      split: (text: string) => text.split('\n'),
    }));
    const splitter = createSplitter('code' as any);
    expect(splitter.split('line1\nline2')).toHaveLength(2);
  });

  it('should handle line splitter', () => {
    const splitter = createSplitter('line', { chunkSize: 2 });
    const text = 'line1\nline2\nline3\nline4';
    const chunks = splitter.split(text);
    expect(chunks.length).toBe(2);
  });

  it('should handle sentence splitter', () => {
    const splitter = createSplitter('sentence', { chunkSize: 50 });
    const text = 'First sentence. Second sentence! Third one?';
    const chunks = splitter.split(text);
    expect(chunks.length).toBeGreaterThan(0);
  });

  it('should handle recursive splitter', () => {
    const splitter = createSplitter('recursive', { chunkSize: 20 });
    const text = 'Paragraph one.\n\nParagraph two.\n\nParagraph three.';
    const chunks = splitter.split(text);
    expect(chunks.length).toBeGreaterThan(0);
  });

  it('should handle failed chunks in ingest', async () => {
    const ragStore = new RAGStore(3);
    // Mock addDocument to throw
    ragStore.addDocument = vi.fn().mockRejectedValue(new Error('storage error'));
    const pipeline = new EnhancedRAGPipeline({ ragStore });
    const result = await pipeline.ingest('test content');
    expect(result.failed).toBeGreaterThan(0);
    expect(result.ingested).toBe(0);
  });
});

// ===== MilvusProvider Tests =====

describe('MilvusProvider', () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  afterEach(() => {
    if (fetchSpy) fetchSpy.mockRestore();
  });

  it('should insert vectors', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true } as Response);
    const provider = new MilvusProvider({
      endpoint: 'http://localhost:19530',
      collection: 'test',
      dimension: 3,
    });
    await provider.insert([
      { id: '1', vector: [1, 0, 0], metadata: { label: 'a' } },
    ]);
    expect(fetchSpy).toHaveBeenCalled();
  });

  it('should search vectors', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      json: async () => ({ results: [{ id: '1', distance: 0.5 }] }),
    } as Response);
    const provider = new MilvusProvider({
      endpoint: 'http://localhost:19530',
      collection: 'test',
      dimension: 3,
      apiKey: 'key',
    });
    const results = await provider.search([1, 0, 0], 5);
    expect(results).toHaveLength(1);
    expect(results[0].id).toBe('1');
  });

  it('should delete vectors', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true } as Response);
    const provider = new MilvusProvider({
      endpoint: 'http://localhost:19530',
      collection: 'test',
      dimension: 3,
    });
    await provider.delete(['1', '2']);
    expect(fetchSpy).toHaveBeenCalled();
  });
});

// ===== QdrantProvider Tests =====

describe('QdrantProvider', () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  afterEach(() => {
    if (fetchSpy) fetchSpy.mockRestore();
  });

  it('should insert vectors', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true } as Response);
    const provider = new QdrantProvider({
      endpoint: 'http://localhost:6333',
      collection: 'test',
      dimension: 3,
      apiKey: 'key',
    });
    await provider.insert([
      { id: '1', vector: [1, 0, 0], metadata: { label: 'a' } },
    ]);
    expect(fetchSpy).toHaveBeenCalled();
  });

  it('should search vectors', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      json: async () => ({ result: [{ id: '1', score: 0.95 }] }),
    } as Response);
    const provider = new QdrantProvider({
      endpoint: 'http://localhost:6333',
      collection: 'test',
      dimension: 3,
    });
    const results = await provider.search([1, 0, 0], 5);
    expect(results).toHaveLength(1);
    expect(results[0].score).toBe(0.95);
  });

  it('should delete vectors', async () => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true } as Response);
    const provider = new QdrantProvider({
      endpoint: 'http://localhost:6333',
      collection: 'test',
      dimension: 3,
    });
    await provider.delete(['1', '2']);
    expect(fetchSpy).toHaveBeenCalled();
  });
});

// ===== InMemoryStore Additional Tests =====

describe('InMemoryStore Extended', () => {
  it('should validate episode on add', async () => {
    const store = new InMemoryStore();
    await expect(store.add({ id: '', sessionId: 's', role: 'user', content: 'test', createdAt: '' })).rejects.toThrow('ID is required');
    await expect(store.add({ id: '1', sessionId: 's', role: 'user', content: '', createdAt: '' })).rejects.toThrow('content is required');
  });

  it('should validate importance range', async () => {
    const store = new InMemoryStore();
    await store.add({ id: '1', sessionId: 's', role: 'user', content: 'test', createdAt: '' });
    await expect(store.setImportance('1', -0.1)).rejects.toThrow('Importance');
    await expect(store.setImportance('1', 1.1)).rejects.toThrow('Importance');
  });

  it('should throw on updateSummary for missing episode', async () => {
    const store = new InMemoryStore();
    await expect(store.updateSummary('missing', 'sum', 'tags')).rejects.toThrow('not found');
  });

  it('should throw on setImportance for missing episode', async () => {
    const store = new InMemoryStore();
    await expect(store.setImportance('missing', 0.5)).rejects.toThrow('not found');
  });

  it('should update existing episode (re-index)', async () => {
    const store = new InMemoryStore();
    await store.add({ id: '1', sessionId: 's', role: 'user', content: 'hello world', createdAt: '' });
    // Re-add with different content
    await store.add({ id: '1', sessionId: 's', role: 'user', content: 'goodbye world', createdAt: '' });
    // Search for old content should not find it
    const oldResults = await store.search('hello');
    expect(oldResults).toHaveLength(0);
    // Search for new content should find it
    const newResults = await store.search('goodbye');
    expect(newResults).toHaveLength(1);
  });
});
