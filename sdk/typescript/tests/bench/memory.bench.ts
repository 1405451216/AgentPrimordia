/**
 * Memory Store 性能基准测试
 * 测试 InMemoryStore 的 add/search/list 操作性能
 */
import { describe, bench } from 'vitest';
import { InMemoryStore } from '../../src/memory/store.js';
import type { MemoryEpisode } from '../../src/types.js';

function makeEpisode(i: number): MemoryEpisode {
  return {
    id: `ep-${i}`,
    sessionId: `session-${i % 10}`,
    role: i % 2 === 0 ? 'user' : 'assistant',
    content: `This is episode ${i} about programming in Go and TypeScript. Topic ${i % 5}.`,
    summary: `Summary of episode ${i}`,
    topics: `programming,go,typescript,topic-${i % 5}`,
    importance: Math.random(),
    createdAt: new Date(Date.now() - i * 1000).toISOString(),
  };
}

describe('InMemoryStore Add', () => {
  bench('add 100 episodes', () => {
    const store = new InMemoryStore();
    for (let i = 0; i < 100; i++) {
      store.add(makeEpisode(i));
    }
    store.close();
  });

  bench('add 1000 episodes', () => {
    const store = new InMemoryStore();
    for (let i = 0; i < 1000; i++) {
      store.add(makeEpisode(i));
    }
    store.close();
  });
});

describe('InMemoryStore Search', () => {
  // Pre-populate store
  const setupStore = (n: number): InMemoryStore => {
    const store = new InMemoryStore();
    for (let i = 0; i < n; i++) {
      store.add(makeEpisode(i));
    }
    return store;
  };

  const store100 = setupStore(100);
  const store1000 = setupStore(1000);

  bench('search in 100 episodes', () => {
    store100.search('programming');
  });

  bench('search in 1000 episodes', () => {
    store1000.search('programming');
  });

  bench('searchByTag in 100 episodes', () => {
    store100.searchByTag('go');
  });

  bench('multi-token search in 1000 episodes', () => {
    store1000.search('Go TypeScript');
  });
});

describe('InMemoryStore List & Stats', () => {
  const store = new InMemoryStore();
  for (let i = 0; i < 500; i++) {
    store.add(makeEpisode(i));
  }

  bench('list all', () => {
    store.list({ limit: 500 });
  });

  bench('list by session', () => {
    store.list({ sessionId: 'session-0', limit: 100 });
  });

  bench('count by session', () => {
    store.count('session-0');
  });

  bench('stats', () => {
    store.stats();
  });

  bench('getImportant', () => {
    store.getImportant(0.5, 50);
  });

  bench('getTimeline', () => {
    store.getTimeline(7);
  });
});

describe('InMemoryStore Delete', () => {
  bench('delete 100 episodes', () => {
    const store = new InMemoryStore();
    for (let i = 0; i < 100; i++) {
      store.add(makeEpisode(i));
    }
    for (let i = 0; i < 100; i++) {
      store.delete(`ep-${i}`);
    }
    store.close();
  });
});
