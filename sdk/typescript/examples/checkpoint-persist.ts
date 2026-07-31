/**
 * 检查点持久化示例 — Agent 状态保存与恢复
 *
 * 运行: npx tsx examples/checkpoint-persist.ts
 */
import { SQLiteCheckpointStore } from '../src/persist/sqlite-checkpoint.js';
import type { AgentState } from '../src/persist/sqlite-checkpoint.js';

async function main() {
  console.log('=== AgentPrimordia TS SDK: Checkpoint Persistence ===\n');

  // 创建检查点存储（内存模式，无需真实 SQLite）
  const store = new SQLiteCheckpointStore({ path: ':memory:' });

  // 保存 Agent 状态
  const state: AgentState = {
    agentId: 'agent-checkpoint-demo',
    turnCount: 5,
    messages: [
      { role: 'user', content: 'What is AgentPrimordia?' },
      { role: 'assistant', content: 'It is a production-grade AI agent framework.' },
    ],
    metadata: { model: 'gpt-4', session: 'demo-session' },
    timestamp: Date.now(),
  };

  console.log('Saving checkpoint...');
  await store.save(state);
  console.log(`Saved: agentId=${state.agentId}, turn=${state.turnCount}`);

  // 恢复状态
  console.log('\nRestoring checkpoint...');
  const restored = await store.load('agent-checkpoint-demo');
  if (restored) {
    console.log(`Restored: turn=${restored.turnCount}, messages=${restored.messages.length}`);
    console.log(`Last message: "${restored.messages[restored.messages.length - 1]?.content}"`);
  }

  // 覆盖保存
  console.log('\nOverwriting with new state...');
  await store.save({ ...state, turnCount: 10, timestamp: Date.now() });
  const updated = await store.load('agent-checkpoint-demo');
  console.log(`Updated turn: ${updated?.turnCount}`);

  // 加载不存在的检查点
  console.log('\nLoading non-existent checkpoint...');
  const missing = await store.load('non-existent-agent');
  console.log(`Result: ${missing === null ? 'null (not found)' : 'found'}`);

  console.log('\n--- Done ---');
}

main().catch(console.error);
