/**
 * DAG 编排示例 — 有向无环图工作流
 *
 * 运行: npx tsx examples/orchestration-dag.ts
 */
import { DAGOrchestrator } from '../src/orchestration/extended.js';
import type { Provider } from '../src/llm/provider.js';
import type { CompletionRequest, CompletionResponse, ToolCallRequest, ToolCallResponse, ModelInfo } from '../src/types.js';

function namedProvider(name: string): Provider {
  return {
    async complete(req: CompletionRequest): Promise<CompletionResponse> {
      const input = req.messages[req.messages.length - 1]?.content ?? '';
      return { id: `${name}-${Date.now()}`, content: `[${name}] ${input.slice(0, 30)}`, role: 'assistant', usage: { promptTokens: 5, completionTokens: 5, totalTokens: 10 } };
    },
    async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
      return { content: '', toolCalls: [], usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 } };
    },
    info(): ModelInfo {
      return { name, provider: 'demo', maxContext: 4096, supportsTools: false, supportsStreaming: false };
    },
  };
}

async function main() {
  console.log('=== AgentPrimordia TS SDK: DAG Orchestration ===\n');

  // 构建 DAG:
  //   A (数据采集) → B (分析)
  //   A (数据采集) → C (可视化)
  //   B + C → D (汇总报告)
  const dag = new DAGOrchestrator({
    nodes: [
      { id: 'A', name: 'DataCollector', provider: namedProvider('Collector') },
      { id: 'B', name: 'Analyzer', provider: namedProvider('Analyzer'), deps: ['A'] },
      { id: 'C', name: 'Visualizer', provider: namedProvider('Visualizer'), deps: ['A'] },
      { id: 'D', name: 'Reporter', provider: namedProvider('Reporter'), deps: ['B', 'C'] },
    ],
  });

  console.log('DAG Structure:');
  console.log('  A (Collector) → B (Analyzer)');
  console.log('  A (Collector) → C (Visualizer)');
  console.log('  B + C → D (Reporter)\n');

  console.log('Executing DAG...');
  const result = await dag.run('Collect and analyze Q3 sales data');

  console.log(`\nFinal output: ${result.content}`);
  console.log(`Execution order: ${result.executionOrder?.join(' → ') ?? 'A → B,C → D'}`);
  console.log(`Total nodes: ${result.stepsCompleted ?? 4}`);

  console.log('\n--- Done ---');
}

main().catch(console.error);
