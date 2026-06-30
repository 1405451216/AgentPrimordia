import { describe, it, expect, beforeEach } from 'vitest';
import { RAGStore, defaultFusionConfig } from '../../src/memory/rag.js';
import type { RAGFusionConfig } from '../../src/memory/rag.js';

describe('RAGStore — RRF Fusion', () => {
  let store: RAGStore;

  beforeEach(() => {
    store = new RAGStore(4, {
      fusion: { fusionMode: 'rrf', rrfK: 60, overFetchSize: 5 },
    });
  });

  describe('fusion config', () => {
    it('defaults to linear mode', () => {
      const s = new RAGStore(4);
      const cfg = s.getFusionConfig();
      expect(cfg.fusionMode).toBe('linear');
      expect(cfg.rrfK).toBe(60);
      expect(cfg.overFetchSize).toBe(5);
    });

    it('can switch to RRF mode via setFusionConfig', () => {
      const s = new RAGStore(4);
      s.setFusionConfig({ fusionMode: 'rrf' });
      expect(s.getFusionConfig().fusionMode).toBe('rrf');
    });

    it('can customize rrfK', () => {
      store.setFusionConfig({ fusionMode: 'rrf', rrfK: 30 });
      expect(store.getFusionConfig().rrfK).toBe(30);
    });

    it('defaultFusionConfig returns expected values', () => {
      const cfg = defaultFusionConfig();
      expect(cfg.fusionMode).toBe('linear');
      expect(cfg.ftsWeight).toBe(0.4);
      expect(cfg.vectorWeight).toBe(0.6);
      expect(cfg.rrfK).toBe(60);
      expect(cfg.overFetchSize).toBe(5);
    });
  });

  describe('RRF hybrid search', () => {
    beforeEach(async () => {
      // Add documents with embeddings
      await store.addDocument(
        { id: 'doc1', content: 'machine learning algorithms' },
        [1, 0, 0, 0],
      );
      await store.addDocument(
        { id: 'doc2', content: 'deep learning neural networks' },
        [0.9, 0.1, 0, 0],
      );
      await store.addDocument(
        { id: 'doc3', content: 'cooking recipes food' },
        [0, 0, 1, 0],
      );
      await store.addDocument(
        { id: 'doc4', content: 'learning algorithms optimization' },
        [0.8, 0.2, 0, 0],
      );
    });

    it('returns results sorted by RRF score', async () => {
      const results = await store.hybridSearch('learning algorithms', 3, [1, 0, 0, 0]);
      expect(results.length).toBeGreaterThan(0);
      // Results should be sorted descending by score
      for (let i = 1; i < results.length; i++) {
        expect((results[i - 1]!.score ?? 0) >= (results[i]!.score ?? 0)).toBe(true);
      }
    });

    it('combines FTS and vector results with RRF scoring', async () => {
      const results = await store.hybridSearch('learning', 4, [1, 0, 0, 0]);
      // Documents containing "learning" should rank higher
      const ids = results.map((r) => r.id);
      expect(ids).toContain('doc1');
      expect(ids).toContain('doc2');
      expect(ids).toContain('doc4');
    });

    it('includes sources field indicating retrieval channels', async () => {
      const results = await store.hybridSearch('learning algorithms', 3, [1, 0, 0, 0]);
      for (const r of results) {
        expect(r.sources).toBeDefined();
        expect(r.sources!.length).toBeGreaterThan(0);
      }
    });

    it('respects topK limit', async () => {
      const results = await store.hybridSearch('learning', 2, [1, 0, 0, 0]);
      expect(results.length).toBeLessThanOrEqual(2);
    });

    it('documents ranked by both channels score higher', async () => {
      const results = await store.hybridSearch('learning algorithms', 4, [1, 0, 0, 0]);
      // doc1 contains both "learning" and "algorithms" and has exact vector match
      // It should be the top result
      expect(results[0]!.id).toBe('doc1');
    });

    it('handles empty query gracefully', async () => {
      const results = await store.hybridSearch('', 5, [1, 0, 0, 0]);
      // Should not throw, may return empty or vector-only results
      expect(Array.isArray(results)).toBe(true);
    });

    it('works without vector embedding (FTS only)', async () => {
      const results = await store.hybridSearch('cooking', 3);
      expect(results.length).toBeGreaterThan(0);
      expect(results[0]!.id).toBe('doc3');
    });
  });

  describe('RRF vs Linear comparison', () => {
    beforeEach(async () => {
      await store.addDocument(
        { id: 'doc1', content: 'the quick brown fox' },
        [1, 0, 0, 0],
      );
      await store.addDocument(
        { id: 'doc2', content: 'quick fox running fast' },
        [0.9, 0.1, 0, 0],
      );
      await store.addDocument(
        { id: 'doc3', content: 'slow turtle crossing road' },
        [0, 0.5, 0.5, 0],
      );
    });

    it('RRF mode produces different scores than linear', async () => {
      // Get RRF results
      store.setFusionConfig({ fusionMode: 'rrf', rrfK: 60 });
      const rrfResults = await store.hybridSearch('quick fox', 3, [1, 0, 0, 0]);

      // Get linear results
      store.setFusionConfig({ fusionMode: 'linear', ftsWeight: 0.4, vectorWeight: 0.6 });
      const linearResults = await store.hybridSearch('quick fox', 3, [1, 0, 0, 0]);

      // Both should return results
      expect(rrfResults.length).toBeGreaterThan(0);
      expect(linearResults.length).toBeGreaterThan(0);

      // RRF scores are typically smaller (1/(60+rank) < 1)
      const rrfScore = rrfResults[0]!.score ?? 0;
      const linearScore = linearResults[0]!.score ?? 0;
      expect(rrfScore).toBeLessThan(linearScore);
    });
  });

  describe('over-fetch behavior', () => {
    beforeEach(async () => {
      for (let i = 0; i < 20; i++) {
        await store.addDocument(
          { id: `doc${i}`, content: `document number ${i} with keywords` },
          [i / 20, 0, 0, 0],
        );
      }
    });

    it('over-fetches more candidates than topK for better recall', async () => {
      const results = await store.hybridSearch('keywords', 5, [0.5, 0, 0, 0]);
      // Should return at most topK results
      expect(results.length).toBeLessThanOrEqual(5);
    });
  });
});
