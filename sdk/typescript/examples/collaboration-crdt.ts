/**
 * CRDT 协作编辑示例 — 无冲突分布式协作（v3.2.0 新增持久化）
 *
 * 运行: npx tsx examples/collaboration-crdt.ts
 */
import { CRDTDocumentImpl, LamportClock, InMemoryCRDTPersistence, createSnapshot } from '../src/collaboration/crdt.js';

async function main() {
  console.log('=== AgentPrimordia TS SDK: CRDT Collaboration ===\n');

  // 创建两个协作客户端（模拟分布式环境）
  const docA = new CRDTDocumentImpl<{ title: string; content: string }>('client-A');
  const docB = new CRDTDocumentImpl<{ title: string; content: string }>('client-B');

  // 客户端 A 编辑
  console.log('--- Client A edits ---');
  docA.apply({ type: 'update', path: 'title', value: 'Collaborative Doc', clock: 1, clientID: 'client-A' });
  docA.apply({ type: 'update', path: 'content', value: 'Hello from A', clock: 2, clientID: 'client-A' });
  console.log(`A state: ${JSON.stringify(docA.getState())}`);

  // 客户端 B 并发编辑
  console.log('\n--- Client B edits (concurrent) ---');
  docB.apply({ type: 'update', path: 'content', value: 'Hello from B', clock: 1, clientID: 'client-B' });
  console.log(`B state: ${JSON.stringify(docB.getState())}`);

  // 合并（LWW: 时钟大者胜）
  console.log('\n--- Merge B into A ---');
  docA.merge(docB);
  console.log(`A after merge: ${JSON.stringify(docA.getState())}`);

  // Lamport Clock 演示
  console.log('\n--- Lamport Clock ---');
  const clock = new LamportClock('node-1');
  console.log(`Tick 1: ${clock.tick()}`);
  console.log(`Tick 2: ${clock.tick()}`);
  clock.receive(10); // 接收远端时钟
  console.log(`After receive(10): ${clock.tick()}`);

  // 持久化（v3.2.0 新增）
  console.log('\n--- Persistence (v3.2.0) ---');
  const persistence = new InMemoryCRDTPersistence();
  const snapshot = createSnapshot(docA, 'client-A');
  await persistence.save('doc-1', snapshot);
  console.log(`Saved snapshot: version=${snapshot.version}, ops=${snapshot.operations.length}`);

  const loaded = await persistence.load('doc-1');
  console.log(`Loaded: clientID=${loaded?.clientID}, state=${JSON.stringify(loaded?.state)}`);

  const docs = await persistence.list();
  console.log(`Stored documents: ${docs.join(', ')}`);

  console.log('\n--- Done ---');
}

main().catch(console.error);
