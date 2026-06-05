# Memory

## InMemoryStore

Episodic memory store for conversation history.

```typescript
import { InMemoryStore } from '@agentprimordia/sdk';

const store = new InMemoryStore();

// Add episodes
await store.add({
  id: 'ep-1',
  sessionId: 'session-1',
  role: 'user',
  content: 'Hello!',
  summary: 'User greeting',
  topics: ['greeting'],
  importance: 0.5,
  metadata: {},
  createdAt: new Date().toISOString(),
});

// Search episodes
const results = await store.search('greeting', { limit: 5 });

// Get stats
const stats = await store.stats();
console.log(`Total episodes: ${stats.totalEpisodes}`);
```

### Search Options

```typescript
const results = await store.search('query', {
  sessionId: 'session-1',    // Filter by session
  limit: 10,                 // Max results
  offset: 0,                 // Pagination offset
  roleFilter: 'user',        // Filter by role
});
```

### Timeline

```typescript
const timeline = await store.getTimeline(7); // Last 7 days
// Returns Map<date, episode[]>
```

## VectorStore

Cosine similarity vector search.

```typescript
import { VectorStore } from '@agentprimordia/sdk';

const vectors = new VectorStore(128); // 128-dimensional vectors

// Add vectors
vectors.add('doc-1', [0.1, 0.2, ...], { source: 'wikipedia' });

// Search
const results = vectors.search([0.1, 0.2, ...], 5); // Top 5
for (const result of results) {
  console.log(`${result.id}: score=${result.score}`);
}
```

### Configuration

| Option | Default | Description |
|--------|---------|-------------|
| dimensions | 16 | Vector dimensions |
