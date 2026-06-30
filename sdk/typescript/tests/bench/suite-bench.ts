/**
 * 性能基准套件 — 对齐 Go SDK 的 bench/suite/ 目录结构。
 *
 * Go 端基准场景：
 * - latency_test.go: BenchmarkLatency, BenchmarkConcurrent, BenchmarkFirstTokenLatency, BenchmarkMemoryLatency, BenchmarkVectorSearch
 * - tool_calling_test.go: BenchmarkToolCalling, BenchmarkAgentRun, BenchmarkMemoryStore
 *
 * 本文件复刻所有 Go 基准场景，使用 vitest bench API。
 *
 * 运行方式：npx vitest bench --reporter=verbose
 */

import { describe, bench } from 'vitest';
import { ReActAgent, HookManager } from '../../src/agent/react-loop.js';
import { MockProvider } from '../../src/llm/provider.js';
import { ToolRegistry } from '../../src/tools/registry.js';
import { InMemoryStore } from '../../src/memory/store.js';
import { VectorStore } from '../../src/memory/vector.js';
import { ObjectPool } from '../../src/jsonutil/pool.js';
import { detectRuntime } from '../../src/edge/runtime.js';
import type { ToolCall, ToolResult } from '../../src/types.js';

// ===== Mock 工具 =====

class MockTool {
  name = 'mock_tool';
  async execute(tc: ToolCall): Promise<ToolResult> {
    return { toolCallId: tc.id, content: `result: ${tc.arguments}`, isError: false };
  }
}

// ===== 对齐 Go: BenchmarkLatency =====
// 基准：Agent 延迟（单轮非流式）

describe('BenchmarkLatency (Go: BenchmarkLatency)', () => {
  const provider = new MockProvider({ response: 'hello', delay: 0 });
  const toolkit = new ToolRegistry();

  bench('1-turn latency (no tools)', async () => {
    const agent = new ReActAgent({
      name: 'LatencyAgent',
      model: provider,
      toolkit,
      maxTurns: 1,
      systemPrompt: '你是助手',
    });
    await agent.run('hello');
  });
});

// ===== 对齐 Go: BenchmarkConcurrent =====
// 基准：并发 Agent 吞吐量

describe('BenchmarkConcurrent (Go: BenchmarkConcurrent)', () => {
  const provider = new MockProvider({ response: 'concurrent response', delay: 0 });
  const toolkit = new ToolRegistry();

  bench('10 concurrent agents', async () => {
    const tasks = Array.from({ length: 10 }, (_, i) => {
      const agent = new ReActAgent({
        name: `Agent-${i}`,
        model: provider,
        toolkit,
        maxTurns: 1,
        systemPrompt: '你是助手',
      });
      return agent.run(`task-${i}`);
    });
    await Promise.all(tasks);
  });

  bench('50 concurrent agents', async () => {
    const tasks = Array.from({ length: 50 }, (_, i) => {
      const agent = new ReActAgent({
        name: `Agent-${i}`,
        model: provider,
        toolkit,
        maxTurns: 1,
        systemPrompt: '你是助手',
      });
      return agent.run(`task-${i}`);
    });
    await Promise.all(tasks);
  });
});

// ===== 对齐 Go: BenchmarkFirstTokenLatency =====
// 基准：首 Token 延迟（流式）

describe('BenchmarkFirstTokenLatency (Go: BenchmarkFirstTokenLatency)', () => {
  const provider = new MockProvider({ response: 'streaming response', delay: 0 });
  const toolkit = new ToolRegistry();

  bench('first event latency (streaming)', async () => {
    const agent = new ReActAgent({
      name: 'StreamAgent',
      model: provider,
      toolkit,
      maxTurns: 1,
      systemPrompt: '你是助手',
    });
    const iter = agent.streamEvents('hello');
    await iter.next(); // 等待第一个事件
    // 不消费剩余事件（模拟 first-token 延迟测量）
  });
});

// ===== 对齐 Go: BenchmarkMemoryLatency =====
// 基准：记忆操作延迟

describe('BenchmarkMemoryLatency (Go: BenchmarkMemoryLatency)', () => {
  const memory = new InMemoryStore();

  // 预填充数据
  for (let i = 0; i < 1000; i++) {
    memory.add({
      id: `pre-${i}`,
      sessionId: 'bench',
      role: 'user',
      content: `预填充记忆条目 ${i}，包含一些常见关键词如文件、搜索、分析`,
      createdAt: new Date().toISOString(),
    });
  }

  bench('Search 1K memories', async () => {
    await memory.search('文件搜索');
  });

  bench('Add memory', async () => {
    await memory.add({
      id: `bench-${Date.now()}`,
      sessionId: 'bench',
      role: 'user',
      content: 'benchmark test episode',
      createdAt: new Date().toISOString(),
    });
  });
});

// ===== 对齐 Go: BenchmarkVectorSearch =====
// 基准：向量搜索延迟

describe('BenchmarkVectorSearch (Go: BenchmarkVectorSearch)', () => {
  const dim = 128;
  const store = new VectorStore(dim);

  // 预填充向量
  for (let i = 0; i < 1000; i++) {
    const vec = Array.from({ length: dim }, (_, j) => (i * dim + j) / 100000.0);
    store.add(`vec-${i}`, vec);
  }

  const query = Array.from({ length: dim }, () => 0.5);

  bench('Vector search 1K vectors (k=10)', () => {
    store.search(query, 10);
  });

  bench('Vector add', () => {
    const vec = Array.from({ length: dim }, () => Math.random());
    store.add(`bench-vec-${Date.now()}`, vec);
  });
});

// ===== 对齐 Go: BenchmarkToolCalling =====
// 基准：工具调用准确率

describe('BenchmarkToolCalling (Go: BenchmarkToolCalling)', () => {
  const provider = new MockProvider({
    response: '使用工具',
    toolCalls: [{ id: 'tc-1', name: 'mock_tool', arguments: '{"input":"test"}' }],
    delay: 0,
  });
  const toolkit = new ToolRegistry();
  toolkit.register(new MockTool());

  bench('Agent with 1 tool call (2 turns)', async () => {
    const agent = new ReActAgent({
      name: 'ToolAgent',
      model: provider,
      toolkit,
      maxTurns: 3,
      systemPrompt: '你是一个助手，使用工具完成任务。',
    });
    await agent.run('读取文件');
  });
});

// ===== 对齐 Go: BenchmarkAgentRun =====
// 基准：单次 Agent 运行吞吐量

describe('BenchmarkAgentRun (Go: BenchmarkAgentRun)', () => {
  const provider = new MockProvider({ response: 'throughput response', delay: 0 });
  const toolkit = new ToolRegistry();

  bench('Agent run throughput (3 turns max)', async () => {
    const agent = new ReActAgent({
      name: 'ThroughputAgent',
      model: provider,
      toolkit,
      maxTurns: 3,
      systemPrompt: '你是助手',
    });
    await agent.run('hello');
  });
});

// ===== 对齐 Go: BenchmarkMemoryStore =====
// 基准：记忆存储写入和搜索

describe('BenchmarkMemoryStore (Go: BenchmarkMemoryStore)', () => {
  bench('Memory Add', async () => {
    const mem = new InMemoryStore();
    for (let i = 0; i < 100; i++) {
      await mem.add({
        id: `bench-${i}`,
        sessionId: 'bench',
        role: 'user',
        content: 'benchmark test episode',
        createdAt: new Date().toISOString(),
      });
    }
  });

  bench('Memory Search', async () => {
    const mem = new InMemoryStore();
    for (let i = 0; i < 100; i++) {
      await mem.add({
        id: `bench-${i}`,
        sessionId: 'bench',
        role: 'user',
        content: `benchmark test episode ${i}`,
        createdAt: new Date().toISOString(),
      });
    }
    await mem.search('benchmark');
  });
});

// ===== TS 独有基准 =====

describe('TS-Specific Benchmarks', () => {
  // ObjectPool 性能（TS 独有优化）
  bench('ObjectPool get/put (64 pool size)', () => {
    const pool = new ObjectPool(
      () => ({ id: 0, data: '' }),
      (obj) => { obj.id = 0; obj.data = ''; },
      64,
    );
    const obj = pool.get();
    obj.id = 1;
    obj.data = 'test';
    pool.put(obj);
  });

  // HookManager 有/无订阅者
  const hooksWithSub = new HookManager();
  hooksWithSub.register('before_turn', () => {});

  const hooksNoSub = new HookManager();

  bench('fireHook with subscriber', async () => {
    await hooksWithSub.fireHook('before_turn', {
      agentID: 'test',
      sessionID: 's1',
      turn: 0,
    });
  });

  bench('fireHook no subscriber (skip)', async () => {
    await hooksNoSub.fireHook('before_turn', {
      agentID: 'test',
      sessionID: 's1',
      turn: 0,
    });
  });

  // Runtime 检测
  bench('detectRuntime (cached)', () => {
    detectRuntime();
  });
});

// ===== P4 新功能基准 =====

describe('P4 Feature Benchmarks', () => {
  // 并行工具执行 vs 串行
  bench('Serial tool execution (3 tools)', async () => {
    const providerWithTools = new MockProvider({
      response: '使用工具',
      toolCalls: [
        { id: 'tc-1', name: 'mock_tool', arguments: '{"a":1}' },
        { id: 'tc-2', name: 'mock_tool', arguments: '{"a":2}' },
        { id: 'tc-3', name: 'mock_tool', arguments: '{"a":3}' },
      ],
      delay: 0,
    });
    const tk = new ToolRegistry();
    tk.register(new MockTool());

    const agent = new ReActAgent({
      name: 'SerialAgent',
      model: providerWithTools,
      toolkit: tk,
      maxTurns: 3,
    });
    await agent.run('test');
  });

  bench('Parallel tool execution (3 tools)', async () => {
    const providerWithTools = new MockProvider({
      response: '使用工具',
      toolCalls: [
        { id: 'tc-1', name: 'mock_tool', arguments: '{"a":1}' },
        { id: 'tc-2', name: 'mock_tool', arguments: '{"a":2}' },
        { id: 'tc-3', name: 'mock_tool', arguments: '{"a":3}' },
      ],
      delay: 0,
    });
    const tk = new ToolRegistry();
    tk.register(new MockTool());

    const agent = new ReActAgent({
      name: 'ParallelAgent',
      model: providerWithTools,
      toolkit: tk,
      maxTurns: 3,
      parallelToolExecution: true,
    });
    await agent.run('test');
  });
});
