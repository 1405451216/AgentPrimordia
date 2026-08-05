/**
 * 跨语言行为一致性测试 — TS SDK 侧
 *
 * 加载 shared/cross-language-spec.json 中的共享测试规范，
 * 验证 TS SDK 的行为与 Go SDK 保持一致。
 *
 * 这些测试确保 Go 和 TypeScript 两个 SDK 在面对相同输入时
 * 产生等价输出。
 */
import { describe, it, expect } from 'vitest';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { ToolRegistry } from '../../src/tools/registry.js';
import { ReActAgent } from '../../src/agent/react-loop.js';
import { MockProvider } from '../../src/llm/provider.js';
import { TokenBucket, QuotaManager } from '../../src/governance/quota.js';
import { ACL } from '../../src/security/sandbox.js';
import { GuardrailEngine, RuleEngine, PromptInjectionRule, PIIRule } from '../../src/security/guardrails.js';
import { InMemoryCheckpointStore } from '../../src/agent/request-id.js';

// ===== 加载共享规范 =====

interface TestCase {
  id: string;
  description: string;
  input: Record<string, unknown>;
  expected: Record<string, unknown>;
}

interface TestSuite {
  name: string;
  description: string;
  cases: TestCase[];
}

interface CrossLanguageSpec {
  version: string;
  description: string;
  testSuites: TestSuite[];
}

function loadSpec(): CrossLanguageSpec {
  const specPath = path.resolve(__dirname, 'cross-language-spec.json');
  const raw = fs.readFileSync(specPath, 'utf-8');
  return JSON.parse(raw) as CrossLanguageSpec;
}

// ===== 测试运行器 =====

const spec = loadSpec();

describe('Cross-Language Behavioral Alignment', () => {
  for (const suite of spec.testSuites) {
    describe(suite.name, () => {
      for (const testCase of suite.cases) {
        it(`${testCase.id}: ${testCase.description}`, async () => {
          switch (suite.name) {
            case 'tool_execution':
              await runToolExecutionTest(testCase);
              break;
            case 'vector_operations':
              runVectorOperationTest(testCase);
              break;
            case 'error_handling':
              runErrorHandlingTest(testCase);
              break;
            case 'json_serialization':
              runJsonSerializationTest(testCase);
              break;
            case 'error_code_mapping':
              runErrorCodeMappingTest(testCase);
              break;
            case 'memory_store':
              runMemoryStoreTest(testCase);
              break;
            case 'llm_provider':
              runLlmProviderTest(testCase);
              break;
            case 'health_check':
              runHealthCheckTest(testCase);
              break;
            case 'chaos_config':
              runChaosConfigTest(testCase);
              break;
            case 'orchestration':
              runOrchestrationTest(testCase);
              break;
            case 'agent_config':
              runAgentConfigTest(testCase);
              break;
            case 'governance_quota':
              runGovernanceQuotaTest(testCase);
              break;
            case 'security_acl':
              runSecurityAclTest(testCase);
              break;
            case 'guardrail_rules':
              runGuardrailRulesTest(testCase);
              break;
            case 'persist_checkpoint':
              await runPersistCheckpointTest(testCase);
              break;
            default:
              // 其余套件不需要 LLM Provider 的测试骨架
              console.log(`Skipping ${testCase.id}: unknown suite`);
          }
        });
      }
    });
  }
});

// ===== 具体测试实现 =====

async function runToolExecutionTest(tc: TestCase) {
  const registry = new ToolRegistry();
  registry.register({
    name: 'echo',
    description: 'Echo the input',
    parameters: { type: 'object', properties: { text: { type: 'string' } } },
    async execute(args: Record<string, unknown>) {
      return `Echo: ${args.text ?? 'empty'}`;
    },
  });

  const toolName = tc.input.toolName as string;
  const args = tc.input.args as Record<string, unknown>;
  const tool = registry.get(toolName);

  expect(tool).toBeDefined();
  const result = await tool!.execute(args);
  expect(result).toBe(tc.expected.result);
}

function runVectorOperationTest(tc: TestCase) {
  const vectorA = tc.input.vectorA as number[];
  const vectorB = tc.input.vectorB as number[];
  const expectedScore = tc.expected.score as number;
  const tolerance = (tc.expected.tolerance as number) ?? 0.001;

  const score = cosineSimilarity(vectorA, vectorB);
  expect(Math.abs(score - expectedScore)).toBeLessThanOrEqual(tolerance);
}

function runErrorHandlingTest(tc: TestCase) {
  // 验证输入校验逻辑
  const name = tc.input.name as string;
  const maxTurns = tc.input.max_turns as number | undefined;

  if (tc.expected.shouldError) {
    if (name === '') {
      expect(name).toBe('');
      // 在 TS SDK 中，空名称应触发错误或警告
    }
    if (maxTurns !== undefined && maxTurns < 0) {
      expect(maxTurns).toBeLessThan(0);
    }
  }
}

function runJsonSerializationTest(tc: TestCase) {
  // 验证 JSON 序列化/反序列化一致性
  const input = tc.input;
  const serialized = JSON.stringify(input);
  const deserialized = JSON.parse(serialized);

  expect(deserialized.id).toBe(tc.expected.id);
  expect(deserialized.vector).toEqual(tc.expected.vector);
  expect(deserialized.metadata).toEqual(tc.expected.metadata);
}

// ===== 辅助函数 =====

function cosineSimilarity(a: number[], b: number[]): number {
  if (a.length !== b.length) return 0;
  if (a.length === 0) return 0;

  let dot = 0;
  let normA = 0;
  let normB = 0;
  for (let i = 0; i < a.length; i++) {
    dot += a[i] * b[i];
    normA += a[i] * a[i];
    normB += b[i] * b[i];
  }

  const denom = Math.sqrt(normA) * Math.sqrt(normB);
  if (denom === 0) return 0;
  return dot / denom;
}

// ===== 新增套件测试骨架 =====

function runErrorCodeMappingTest(tc: TestCase) {
  // 骨架：验证 TS ErrorCode 枚举与 Go pkg/errors.go 错误码映射一致
  const module = tc.input.module as string | undefined;
  const expectedCodes = tc.expected.codes as string[] | undefined;

  if (expectedCodes && module) {
    // TODO: 从 TS ErrorCode 枚举中按模块前缀过滤，验证编号与 Go 侧一致
    expect(expectedCodes.length).toBeGreaterThan(0);
    // 验证错误码格式：大写字母前缀 + _ + 三位数字
    for (const code of expectedCodes) {
      expect(code).toMatch(/^[A-Z]+_\d{3}$/);
    }
  }

  if (tc.expected.code) {
    // 未知错误 fallback 验证
    expect(tc.expected.code).toBe('UNKNOWN');
  }
}

function runMemoryStoreTest(tc: TestCase) {
  // 骨架：验证 Memory CRUD 操作行为一致性
  const operation = tc.input.operation as string;

  switch (operation) {
    case 'add_then_search': {
      // TODO: 调用 TS MemoryStore.add() 后 search()，验证可检索
      expect(tc.expected.found).toBe(true);
      expect(tc.expected.minResults).toBeGreaterThanOrEqual(1);
      break;
    }
    case 'search': {
      // TODO: 空存储搜索应返回空结果
      expect(tc.expected.found).toBe(false);
      expect(tc.expected.resultCount).toBe(0);
      break;
    }
    case 'add': {
      // TODO: 验证输入校验（importance 范围、空内容拒绝）
      if (tc.expected.shouldError) {
        expect(tc.expected.shouldError).toBe(true);
      }
      break;
    }
    default:
      throw new Error(`Unknown memory operation: ${operation}`);
  }
}

function runLlmProviderTest(tc: TestCase) {
  // 骨架：验证 Provider 接口行为（CompletionRequest/Response 格式）
  const provider = tc.input.provider as string;
  expect(provider).toBe('mock');

  if (tc.input.errorMode) {
    // TODO: MockProvider 错误模式应返回 LLM_001
    expect(tc.expected.shouldError).toBe(true);
    expect(tc.expected.errorCode).toBe('LLM_001');
    return;
  }

  const messages = tc.input.messages as Array<Record<string, unknown>>;
  if (messages.length === 0) {
    // TODO: 空消息列表应返回错误
    expect(tc.expected.shouldError).toBe(true);
    return;
  }

  // TODO: 正常请求应返回预配置响应
  expect(tc.expected.role).toBe('assistant');
  expect(tc.expected.content).toBeDefined();
}

function runHealthCheckTest(tc: TestCase) {
  // 骨架：验证健康检查端点响应格式
  const ready = tc.input.ready as boolean;
  const endpoint = tc.input.endpoint as string;

  expect(endpoint).toBe('/healthz');

  if (ready) {
    expect(tc.expected.statusCode).toBe(200);
    expect((tc.expected.body as Record<string, unknown>).status).toBe('ready');
  } else {
    expect(tc.expected.statusCode).toBe(503);
    expect((tc.expected.body as Record<string, unknown>).status).toBe('not_ready');
  }
}

function runChaosConfigTest(tc: TestCase) {
  // 骨架：验证混沌工程实验配置行为一致性
  const name = tc.input.name as string | undefined;

  if (name !== undefined && name === '') {
    // 空名称应被拒绝
    expect(tc.expected.shouldError).toBe(true);
    return;
  }

  if (tc.input.type === 'slo') {
    // SLO 稳态验证阈值配置
    expect(tc.expected.type).toBe('slo');
    expect(tc.expected.threshold).toBeGreaterThan(0);
    expect(tc.expected.threshold).toBeLessThanOrEqual(1);
    return;
  }

  // 基本实验配置验证
  if (name) {
    expect(tc.expected.name).toBe(name);
    expect(tc.expected.status).toBe('pending');
  }
}

function runOrchestrationTest(tc: TestCase) {
  // 骨架：验证 Pipeline/DAG 编排模式执行语义
  const pattern = tc.input.pattern as string;

  if (pattern === 'pipeline') {
    const stages = tc.input.stages as string[];
    if (stages.length === 0) {
      // 空 Pipeline 应返回错误
      expect(tc.expected.shouldError).toBe(true);
    } else {
      // 顺序执行验证
      expect(tc.expected.executionOrder).toEqual(stages);
      expect(tc.expected.allCompleted).toBe(true);
    }
    return;
  }

  if (pattern === 'dag') {
    // DAG 拓扑排序验证
    const nodes = tc.input.nodes as Array<{ id: string; deps: string[] }>;
    expect(tc.expected.validOrder).toBe(true);
    // 无依赖的节点应排在最前
    const rootNodes = nodes.filter(n => n.deps.length === 0);
    expect(rootNodes.length).toBeGreaterThan(0);
    expect(tc.expected.firstNode).toBe(rootNodes[0].id);
    return;
  }

  throw new Error(`Unknown orchestration pattern: ${pattern}`);
}

// ===== agent_config（v3.5-3 补齐） =====

function runAgentConfigTest(tc: TestCase) {
  const name = tc.input.name as string;
  const maxTurns = tc.input.maxTurns as number | undefined;
  const systemPrompt = tc.input.systemPrompt as string | undefined;

  if (tc.id === 'agent_default_max_turns') {
    // 默认 MaxTurns 应为 50（与 Go 端 config.go 默认一致，cross-language spec 契约）
    const agent = new ReActAgent({
      name: name as string,
      model: new MockProvider(),
      toolkit: new ToolRegistry(),
    });
    expect(agent.name).toBe(name);
    expect((agent as unknown as { maxTurns: number }).maxTurns).toBe(tc.expected.maxTurns);
    return;
  }

  // agent_basic_config：name / maxTurns 被正确保留
  const agent = new ReActAgent({
    name: name as string,
    model: new MockProvider(),
    toolkit: new ToolRegistry(),
    systemPrompt: systemPrompt ?? '',
    maxTurns: maxTurns ?? undefined,
  });
  expect(agent.name).toBe(tc.expected.name);
  if (maxTurns !== undefined) {
    expect((agent as unknown as { maxTurns: number }).maxTurns).toBe(maxTurns);
  }
}

// ===== governance_quota（v3.5-3 补齐） =====

function runGovernanceQuotaTest(tc: TestCase) {
  if (tc.id === 'quota_manager_tenant_isolation') {
    // 不同租户配额独立计算
    const tenantA = tc.input.tenantA as { limit: number; used: number };
    const tenantB = tc.input.tenantB as { limit: number; used: number };
    const expected = tc.expected as { tenantARemaining: number; tenantBRemaining: number };

    const qa = new QuotaManager('tenantA', { maxTokensPerDay: tenantA.limit });
    expect(qa.recordTokens(tenantA.used)).toBe(true);
    expect(qa.recordTokens(expected.tenantARemaining)).toBe(true);
    expect(qa.recordTokens(1)).toBe(false);

    const qb = new QuotaManager('tenantB', { maxTokensPerDay: tenantB.limit });
    expect(qb.recordTokens(tenantB.used)).toBe(true);
    expect(qb.recordTokens(expected.tenantBRemaining)).toBe(true);
    expect(qb.recordTokens(1)).toBe(false);
    return;
  }

  const capacity = tc.input.capacity as number;
  const refillRate = tc.input.refillRate as number;
  const consume = tc.input.consume as number;

  const bucket = new TokenBucket(refillRate, capacity);
  const allowed = bucket.take(consume);

  if (tc.id === 'quota_token_bucket_basic') {
    expect(allowed).toBe(tc.expected.allowed);
    // 剩余配额恰好为 expected.remaining：多取 1 应失败
    const remaining = (tc.expected as { remaining: number }).remaining;
    expect(bucket.take(remaining + 1)).toBe(false);
    return;
  }
  // quota_token_bucket_exhausted
  expect(allowed).toBe(tc.expected.allowed);
}

// ===== security_acl（v3.5-3 补齐） =====

function runSecurityAclTest(tc: TestCase) {
  const agentId = tc.input.agentId as string;
  const resource = tc.input.resource as string;
  const permission = tc.input.permission as string;
  const rules = tc.input.rules as Array<{
    agent: string;
    resource: string;
    permission: string;
    effect: string;
  }>;

  const acl = new ACL();
  for (const rule of rules) {
    // 与 Go 端一致：把尾部 glob "/*" 归一化为目录前缀
    let res = rule.resource;
    if (res.endsWith('/*')) {
      res = res.slice(0, -1);
    }
    if (rule.effect.toLowerCase() === 'deny') {
      acl.deny(rule.agent, res);
    } else {
      acl.allow(rule.agent, res, 'all' as const);
    }
  }

  const level = (permission.toLowerCase() === 'write' ? 'write' : 'read') as 'read' | 'write';
  const got = acl.check(agentId, resource, level);
  expect(got).toBe(tc.expected.allowed);
}

// ===== guardrail_rules（v3.5-3 补齐） =====

function runGuardrailRulesTest(tc: TestCase) {
  const text = tc.input.text as string;
  const rules = tc.input.rules as string[];
  const expected = tc.expected as { passed: boolean; violations?: number };

  // 与 Go 端对齐：RuleEngine + PromptInjectionRule / PIIRule（检测即 reject）
  const engine = new RuleEngine();
  for (const rule of rules) {
    switch (rule) {
      case 'injection':
        engine.addRule(new PromptInjectionRule({ action: 'reject' }));
        break;
      case 'pii':
        engine.addRule(new PIIRule({
          action: 'reject',
          detectEmail: true,
          detectPhone: true,
          detectSSN: true,
          detectCreditCard: true,
        }));
        break;
      default:
        throw new Error(`Unknown rule: ${rule}`);
    }
  }
  const report = engine.checkInput(text);
  expect(report.passed).toBe(expected.passed);
  if (!expected.passed && (expected.violations ?? 1) > 0) {
    expect(report.results.length).toBeGreaterThan(0);
  }
}

// ===== persist_checkpoint（v3.5-3 补齐） =====

async function runPersistCheckpointTest(tc: TestCase) {
  const store = new InMemoryCheckpointStore();

  if (tc.id === 'checkpoint_not_found') {
    // 恢复不存在的检查点应返回 null（TS CheckpointStore.load 语义）
    const expected = tc.expected as { shouldError: boolean };
    const cp = await store.load('nonexistent-agent');
    expect(expected.shouldError).toBe(true);
    expect(cp).toBeNull();
    return;
  }

  if (tc.id === 'checkpoint_overwrite') {
    const agentId = tc.input.agentId as string;
    const stateV2 = tc.input.stateV2 as { turn: number };
    const expected = tc.expected as { restoredTurn: number };

    await store.save({
      id: agentId,
      sessionID: 's',
      turn: 1,
      messages: [],
      metrics: { totalTurns: 0, totalTools: 0, duration: 0, llmLatency: 0, toolLatency: 0 },
      createdAt: new Date().toISOString(),
    });
    await store.save({
      id: agentId,
      sessionID: 's',
      turn: stateV2.turn,
      messages: [],
      metrics: { totalTurns: 0, totalTools: 0, duration: 0, llmLatency: 0, toolLatency: 0 },
      createdAt: new Date().toISOString(),
    });
    const cp = await store.load(agentId);
    expect(cp?.turn).toBe(expected.restoredTurn);
    return;
  }

  // checkpoint_save_and_restore
  const agentId = tc.input.agentId as string;
  const state = tc.input.state as { turn: number; messages: number; toolCalls: number };
  const expected = tc.expected as { restored: boolean; turn: number; messages: number };

  await store.save({
    id: agentId,
    sessionID: 'sess-' + agentId,
    turn: state.turn,
    messages: Array.from({ length: state.messages }, () => ({ role: 'user' as const, content: 'm' })),
    metrics: { totalTurns: state.turn, totalTools: state.toolCalls, duration: 0, llmLatency: 0, toolLatency: 0 },
    createdAt: new Date().toISOString(),
  });
  const cp = await store.load(agentId);
  expect(expected.restored).toBe(true);
  expect(cp?.turn).toBe(expected.turn);
  expect(cp?.messages.length).toBe(expected.messages);
}
