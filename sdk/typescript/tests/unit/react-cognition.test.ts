/**
 * ReActAgent 认知能力（Planner/Reflector）接入测试
 *
 * 覆盖：
 * - Planner 分解 >1 子任务 → 按依赖拓扑顺序执行，聚合指标，返回末位结论
 * - Planner 分解 ≤1 子任务 → 降级为普通 ReAct 循环
 * - Planner 失败 → 回退普通循环
 * - Reflector 完成路径批评：severity ≥ 阈值触发改写，低于阈值不改写
 */
import { describe, it, expect } from 'vitest';
import { ReActAgent } from '../../src/agent/react-loop.js';
import { MockProvider } from '../../src/llm/provider.js';
import { ToolRegistry } from '../../src/tools/registry.js';
import { LLMPlanner } from '../../src/agent/planning.js';
import { LLMReflector } from '../../src/agent/reflection.js';
import type { Tool, ToolCall, CompletionRequest, ToolCallRequest, CompletionResponse, ToolCallResponse } from '../../src/types.js';

// ===== 测试工具 =====

class WriteTool implements Tool {
  name = 'write_tool';
  description = 'Write a file';
  parameters = {
    type: 'object' as const,
    properties: { content: { type: 'string' } },
    required: ['content'],
  };
  written: string[] = [];
  async execute(args: Record<string, unknown>): Promise<string> {
    this.written.push(String(args.content));
    return 'written';
  }
}

// ===== 脚本化 Provider：Complete 与 CallTools 双队列，时序确定 =====

class ScriptedProvider extends MockProvider {
  completeQueue: string[] = [];
  toolCallQueue: { content: string; toolCalls: ToolCall[] }[] = [];

  withResponse(content: string): this {
    this.completeQueue.push(content);
    return this;
  }

  withToolResponse(toolCalls: ToolCall[], content = ''): this {
    this.toolCallQueue.push({ content, toolCalls });
    return this;
  }

  override async complete(_req: CompletionRequest): Promise<CompletionResponse> {
    const content = this.completeQueue.shift() ?? 'default complete';
    return {
      id: 'scripted',
      content,
      role: 'assistant',
      usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 },
    };
  }

  override async callTools(_req: ToolCallRequest): Promise<ToolCallResponse> {
    const next = this.toolCallQueue.shift() ?? { content: 'fallback done', toolCalls: [] };
    return {
      content: next.content,
      toolCalls: next.toolCalls,
      usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2 },
    };
  }
}

// 两个子任务的根计划
const twoTaskPlan = JSON.stringify([
  { id: '1', description: '编写内容', depends_on: [] },
  { id: '2', description: '总结输出', depends_on: ['1'] },
]);

// 单子任务计划 → 引擎应降级为普通循环
const singleTaskPlan = JSON.stringify([{ id: '1', description: '直接执行', depends_on: [] }]);

const lowCritique = JSON.stringify({ issues: [], severity: 'low', corrections: [] });
const highCritique = JSON.stringify({
  issues: [{ description: '结论不完整', severity: 'high' }],
  severity: 'high',
  corrections: [{ original: 'done', corrected: 'done-v2', reason: '补充版本号' }],
});

function buildAgent(provider: ScriptedProvider, opts: {
  planner?: LLMPlanner;
  reflector?: LLMReflector;
  threshold?: 'low' | 'medium' | 'high' | 'critical';
}) {
  const registry = new ToolRegistry();
  registry.register(new WriteTool());
  return new ReActAgent({
    name: 'cognition-bot',
    model: provider,
    toolkit: registry,
    maxTurns: 6,
    planner: opts.planner,
    reflector: opts.reflector,
    reflectionSeverityThreshold: opts.threshold,
  });
}

describe('ReActAgent Planner 接入', () => {
  it('多子任务计划按依赖顺序执行并聚合指标', async () => {
    const provider = new ScriptedProvider();
    provider.withResponse(twoTaskPlan); // 根计划分解
    // 子任务1：一次工具调用 + 空 toolCalls 收尾
    provider
      .withToolResponse([{ id: 'c1', name: 'write_tool', arguments: JSON.stringify({ content: 'hello' }) }])
      .withToolResponse([], '子任务1完成');
    // 子任务2：直接收尾
    provider.withToolResponse([], '最终结论：全部完成');

    const agent = buildAgent(provider, { planner: new LLMPlanner(provider) });
    const resp = await agent.run('创建并总结');

    expect(resp.content).toBe('最终结论：全部完成');
    expect(resp.metrics.totalTools).toBe(1);
    // 子任务1 两轮 + 子任务2 一轮
    expect(resp.metrics.totalTurns).toBe(3);
  });

  it('单子任务计划降级为普通循环', async () => {
    const provider = new ScriptedProvider();
    provider.withResponse(singleTaskPlan); // 分解仅 1 个子任务 → 不走计划分支
    provider.withToolResponse([], '直接结论');

    const agent = buildAgent(provider, { planner: new LLMPlanner(provider) });
    const resp = await agent.run('简单任务');

    expect(resp.content).toBe('直接结论');
    expect(resp.metrics.totalTurns).toBe(1);
  });

  it('Planner 失败时回退普通循环', async () => {
    const provider = new ScriptedProvider();
    provider.withResponse('这不是 JSON'); // 解析失败 → 空子任务列表
    provider.withToolResponse([], '普通循环结论');

    const agent = buildAgent(provider, { planner: new LLMPlanner(provider) });
    const resp = await agent.run('任务');

    expect(resp.content).toBe('普通循环结论');
  });
});

describe('ReActAgent Reflector 接入', () => {
  it('severity 低于阈值时不改写输出', async () => {
    const provider = new ScriptedProvider();
    provider.withToolResponse([], '原始结论');
    provider.withResponse(lowCritique); // 批评：low

    const agent = buildAgent(provider, {
      reflector: new LLMReflector(provider),
      threshold: 'high',
    });
    const resp = await agent.run('任务');

    expect(resp.content).toBe('原始结论');
  });

  it('severity 达到阈值时调用 improve 改写输出', async () => {
    const provider = new ScriptedProvider();
    provider.withToolResponse([], '原始结论');
    provider.withResponse(highCritique); // 批评：high
    provider.withResponse('改写后的结论'); // improve 应答

    const agent = buildAgent(provider, {
      reflector: new LLMReflector(provider),
      threshold: 'high',
    });
    const resp = await agent.run('任务');

    expect(resp.content).toBe('改写后的结论');
  });

  it('计划路径下每个子任务完成都会批评', async () => {
    const provider = new ScriptedProvider();
    provider.withResponse(twoTaskPlan);
    provider.withToolResponse([], '子任务1完成').withResponse(lowCritique);
    provider.withToolResponse([], '子任务2完成').withResponse(lowCritique);

    const agent = buildAgent(provider, {
      planner: new LLMPlanner(provider),
      reflector: new LLMReflector(provider),
      threshold: 'high',
    });
    const resp = await agent.run('任务');

    expect(resp.content).toBe('子任务2完成');
    // 批评队列应被两个子任务各消费一次
    expect(provider.completeQueue.length).toBe(0);
  });
});
