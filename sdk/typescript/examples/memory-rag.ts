/**
 * 记忆与 RAG 示例 — 向量存储 + 检索增强生成
 *
 * 运行: npx tsx examples/memory-rag.ts
 */
import { VectorStore } from '../src/memory/vector.js';
import { InMemoryStore } from '../src/memory/store.js';

async function main() {
  console.log('=== AgentPrimordia TS SDK: Memory & RAG Example ===\n');

  // 1. 向量存储
  console.log('--- Vector Store ---');
  const vectors = new VectorStore(3);

  await vectors.add('doc-1', [1, 0, 0], { title: 'Go 并发编程' });
  await vectors.add('doc-2', [0, 1, 0], { title: 'TypeScript 类型系统' });
  await vectors.add('doc-3', [0.7, 0.7, 0], { title: 'Go + TS 双语言开发' });

  const results = await vectors.search([0.9, 0.1, 0], 2);
  console.log('搜索 [0.9, 0.1, 0] 最相似的文档:');
  for (const r of results) {
    console.log(`  - ${r.metadata?.title} (score: ${r.score.toFixed(3)})`);
  }

  // 2. 情景记忆
  console.log('\n--- Episodic Memory ---');
  const memory = new InMemoryStore();

  await memory.add({
    id: 'ep-1', sessionId: 's1', role: 'user',
    content: 'AgentPrimordia 支持哪些编排模式？',
  });
  await memory.add({
    id: 'ep-2', sessionId: 's1', role: 'assistant',
    content: '支持 Pipeline、Parallel、DAG、Handoff、GroupChat 五种编排模式。',
  });
  await memory.add({
    id: 'ep-3', sessionId: 's1', role: 'user',
    content: '如何使用 DAG 模式？',
  });

  const searchResults = await memory.search('编排模式');
  console.log(`搜索 "编排模式": 找到 ${searchResults.length} 条记忆`);
  for (const ep of searchResults) {
    console.log(`  [${ep.role}] ${ep.content.slice(0, 40)}...`);
  }

  // 3. RAG 流程模拟
  console.log('\n--- RAG Pipeline ---');
  const query = '双语言开发';
  const queryVec = [0.8, 0.6, 0]; // 模拟 embedding
  const relevantDocs = await vectors.search(queryVec, 1);

  const context = relevantDocs.map(d => d.metadata?.title).join(', ');
  const prompt = `基于以下上下文回答问题:\n上下文: ${context}\n问题: ${query}`;
  console.log(`RAG Prompt:\n${prompt}`);

  console.log('\n--- Done ---');
}

main().catch(console.error);
