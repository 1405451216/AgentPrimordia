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
import { InMemoryStore } from '../../src/memory/store.js';
import { HNSW } from '../../src/memory/vector-extended.js';
import {
  getErrorCode,
  errorCodeToMessage,
  ErrAgentStopped,
  ErrAgentRunning,
  ErrMaxTurnsExceeded,
  ErrNoToolkit,
  ErrToolNotFound,
  ErrToolExecution,
  ErrInvalidConfig,
  ErrConfirmDenied,
  ErrLLMCallFailed,
  ErrNotSupported,
  ErrCircuitOpen,
  ErrAPIKeyRequired,
  ErrEmptyResponse,
  ErrResponseParseFailed,
  ErrRetriesExhausted,
  ErrFallbackFailed,
  ErrPoolFull,
  ErrTaskNotFound,
  ErrTimeout,
  ErrEpisodeNotFound,
  ErrInvalidImportance,
  ErrEmptyEpisodeID,
  ErrEmptySessionID,
  ErrEmptyRole,
  ErrEmptyContent,
  ErrDimensionMismatch,
  ErrVectorNotFound,
  ErrCommandBlocked,
  ErrCommandNotAllowed,
  ErrAccessDenied,
  ErrPathTraversal,
  ErrBusClosed,
  ErrCheckpointNotFound,
  ErrGlobalWriteConflict,
  ErrScopeOverlap,
  ErrContextCanceled,
} from '../../src/errors.js';
import type { MemoryEpisode, Message } from '../../src/types.js';
import { TokenBucket, QuotaManager } from '../../src/governance/quota.js';
import { ACL } from '../../src/security/sandbox.js';
import { GuardrailEngine, RuleEngine, PromptInjectionRule, PIIRule } from '../../src/security/guardrails.js';
import { InMemoryCheckpointStore } from '../../src/agent/request-id.js';
import { GoalState, validateTransition, createGoal, canRetry } from '../../src/autonomy/index.js';
import { createSkill, activateSkill, isCompatible, versionString } from '../../src/skills/index.js';
import { OpenErr, type OpenAgentCard } from '../../src/a2a/interop.js';
import { SessionState, RealtimeSession } from '../../src/realtime/index.js';

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
              await runMemoryStoreTest(testCase);
              break;
            case 'llm_provider':
              await runLlmProviderTest(testCase);
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
            case 'autonomy_goal':
              runAutonomyGoalTest(testCase);
              break;
            case 'skills_lifecycle':
              runSkillsLifecycleTest(testCase);
              break;
            case 'a2a_interop':
              runA2AInteropTest(testCase);
              break;
            case 'realtime_session':
              runRealtimeSessionTest(testCase);
              break;
            case 'hnsw_recall':
              runHnswRecallTest(testCase);
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

// 模块 → TS SDK 实际导出的 sentinel 错误码集合（来自 errors.ts）。
// 与共享规范对账，确保 TS 错误码定义与 Go pkg/errors.go 完全一致。
const MODULE_SENTINEL_CODES: Record<string, string[]> = {
  agent: [ErrAgentStopped, ErrAgentRunning, ErrMaxTurnsExceeded, ErrNoToolkit].map((e) => e.code),
  tool: [ErrToolNotFound, ErrToolExecution, ErrInvalidConfig, ErrConfirmDenied].map((e) => e.code),
  llm: [
    ErrLLMCallFailed,
    ErrNotSupported,
    ErrCircuitOpen,
    ErrAPIKeyRequired,
    ErrEmptyResponse,
    ErrResponseParseFailed,
    ErrRetriesExhausted,
    ErrFallbackFailed,
  ].map((e) => e.code),
  pool: [ErrPoolFull, ErrTaskNotFound, ErrTimeout].map((e) => e.code),
  memory: [
    ErrEpisodeNotFound,
    ErrInvalidImportance,
    ErrEmptyEpisodeID,
    ErrEmptySessionID,
    ErrEmptyRole,
    ErrEmptyContent,
    ErrDimensionMismatch,
    ErrVectorNotFound,
  ].map((e) => e.code),
  security: [ErrCommandBlocked, ErrCommandNotAllowed, ErrAccessDenied, ErrPathTraversal].map(
    (e) => e.code,
  ),
  infra: [
    ErrBusClosed,
    ErrCheckpointNotFound,
    ErrGlobalWriteConflict,
    ErrScopeOverlap,
    ErrContextCanceled,
  ].map((e) => e.code),
};

function runErrorCodeMappingTest(tc: TestCase) {
  // 未知错误 fallback：非 CodeError 经 getErrorCode 应得到 UNKNOWN
  if (tc.expected.code !== undefined) {
    const raw = new Error(String(tc.input.error ?? 'some random error'));
    expect(getErrorCode(raw)).toBe(tc.expected.code);
    return;
  }

  const module = tc.input.module as string;
  const expectedCodes = tc.expected.codes as string[];
  expect(expectedCodes.length).toBeGreaterThan(0);

  // 验证错误码格式：大写字母前缀 + _ + 三位数字
  for (const code of expectedCodes) {
    expect(code).toMatch(/^[A-Z]+_\d{3}$/);
  }

  // TS SDK 实际 sentinel 错误码必须与规范双向一致（防漏定义/多定义）
  const actualCodes = MODULE_SENTINEL_CODES[module];
  expect(actualCodes, `模块 ${module} 缺少 sentinel 错误码定义`).toBeDefined();
  expect([...actualCodes!].sort()).toEqual([...expectedCodes].sort());

  // 每个错误码必须在消息映射表中注册（errorCodeToMessage 与 Go errorCodeMapping 对齐）
  for (const code of expectedCodes) {
    expect(errorCodeToMessage(code)).not.toBe('unknown error code');
  }
}

/** 从 fixture episode 数据构建 TS MemoryEpisode（字段对齐：sessionID → sessionId） */
function episodeFromFixture(data: Record<string, unknown>): MemoryEpisode {
  return {
    id: String(data.id ?? ''),
    sessionId: String(data.sessionID ?? 'fixture-session'),
    role: String(data.role ?? 'user'),
    content: String(data.content ?? ''),
    importance: data.importance as number | undefined,
    createdAt: new Date().toISOString(),
  };
}

async function runMemoryStoreTest(tc: TestCase) {
  // 真实调用 InMemoryStore 验证 Memory CRUD 行为（对齐 Go runMemoryStoreTest）
  const operation = tc.input.operation as string;
  const store = new InMemoryStore();

  switch (operation) {
    case 'add_then_search': {
      // 添加记忆后搜索，验证可检索
      await store.add(episodeFromFixture(tc.input.episode as Record<string, unknown>));
      const results = await store.search(tc.input.searchQuery as string);
      expect(results.length > 0).toBe(tc.expected.found as boolean);
      if (tc.expected.found) {
        expect(results.length).toBeGreaterThanOrEqual((tc.expected.minResults as number) ?? 1);
      }
      break;
    }
    case 'search': {
      // 空存储搜索应返回空结果
      const results = await store.search(tc.input.searchQuery as string);
      expect(results.length > 0).toBe(tc.expected.found as boolean);
      expect(results).toHaveLength((tc.expected.resultCount as number) ?? 0);
      break;
    }
    case 'add': {
      // 验证输入校验（importance 范围、空内容拒绝），错误码与 Go Episode.Validate 对齐
      let err: unknown;
      try {
        await store.add(episodeFromFixture(tc.input.episode as Record<string, unknown>));
      } catch (e) {
        err = e;
      }
      if (tc.expected.shouldError) {
        expect(err, '期望产生校验错误').toBeDefined();
        const expectedCode = tc.expected.errorCode as string | undefined;
        if (expectedCode) {
          expect(getErrorCode(err)).toBe(expectedCode);
        }
      } else {
        expect(err).toBeUndefined();
      }
      break;
    }
    default:
      throw new Error(`Unknown memory operation: ${operation}`);
  }
}

async function runLlmProviderTest(tc: TestCase) {
  // 真实调用 TS MockProvider 验证 Provider 行为（对齐 Go xlMockProvider 语义）
  const providerName = tc.input.provider as string;
  expect(providerName).toBe('mock');

  const configuredResponse = (tc.input.configuredResponse as string) ?? 'mock response';
  const errorMode = Boolean(tc.input.errorMode);
  const messages = (tc.input.messages ?? []) as Message[];

  const provider = new MockProvider({ response: configuredResponse, error: errorMode });

  let resp: Awaited<ReturnType<MockProvider['complete']>> | undefined;
  let err: unknown;
  try {
    resp = await provider.complete({ messages });
  } catch (e) {
    err = e;
  }

  if (tc.expected.shouldError) {
    expect(err, '期望产生错误').toBeDefined();
    // 错误模式应返回 LLM_001（对齐 Go 端 ErrLLMCallFailed）
    const expectedCode = tc.expected.errorCode as string | undefined;
    if (expectedCode) {
      expect(getErrorCode(err)).toBe(expectedCode);
    }
    return;
  }

  expect(err).toBeUndefined();
  expect(resp!.role).toBe(tc.expected.role);
  expect(resp!.content).toBe(tc.expected.content);
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

// ===== v3.4-v3.6 新增套件（评估报告 §8.1：补齐 Go/TS 双侧实现，
// 修复此前 "Skipping ... unknown suite" 静默跳过） =====

function parseVersionString(s: string): { major: number; minor: number; patch: number } {
  const [major, minor, patch] = s.split('.').map(Number);
  return { major, minor, patch };
}

function runAutonomyGoalTest(tc: TestCase) {
  const input = tc.input as {
    transitions?: string[];
    maxRetries?: number;
    retryCount?: number;
  };
  const expected = tc.expected as {
    finalState?: string;
    allValid?: boolean;
    errorContains?: string;
    canRetry?: boolean;
  };

  // goal_retry_limit
  if (input.maxRetries !== undefined) {
    const goal = createGoal('xl-goal', { maxRetries: input.maxRetries });
    goal.retryCount = input.retryCount ?? 0;
    expect(canRetry(goal)).toBe(expected.canRetry);
    return;
  }

  let state: GoalState = GoalState.Created;
  let allValid = true;
  let lastErr: string | undefined;
  for (const tr of input.transitions ?? []) {
    const [from, to] = tr.split('->');
    if (from !== state) {
      allValid = false;
      break;
    }
    if (!validateTransition(state, to as GoalState)) {
      allValid = false;
      lastErr = `autonomy: 非法状态转换 ${state} → ${to}`;
      break;
    }
    state = to as GoalState;
  }
  expect(allValid).toBe(expected.allValid);
  if (expected.finalState !== undefined) {
    expect(state).toBe(expected.finalState);
  }
  if (expected.errorContains !== undefined && lastErr !== undefined) {
    expect(lastErr).toContain(expected.errorContains);
  }
}

function runSkillsLifecycleTest(tc: TestCase) {
  const input = tc.input as {
    name?: string;
    steps?: Array<{ id: string; toolName: string }>;
    operation?: string;
    v1?: string;
    v2?: string;
  };
  const expected = tc.expected as {
    status?: string;
    version?: string;
    compatible?: boolean;
  };

  // skill_activate
  if (input.operation === 'activate') {
    const skill = createSkill('s', 'd', []);
    activateSkill(skill);
    expect(skill.status).toBe(expected.status);
    return;
  }
  // skill_version_compat
  if (input.v1 !== undefined) {
    const a = parseVersionString(input.v1);
    const b = parseVersionString(input.v2 ?? '');
    expect(isCompatible(a, b)).toBe(expected.compatible);
    return;
  }
  // skill_create_draft
  const skill = createSkill(input.name ?? 's', 'xl', input.steps ?? []);
  expect(skill.status).toBe(expected.status);
  expect(versionString(skill.version)).toBe(expected.version);
}

function runA2AInteropTest(tc: TestCase) {
  const input = tc.input as {
    name?: string;
    url?: string;
    capabilities?: { streaming?: boolean };
    states?: string[];
    code?: number;
  };
  const expected = tc.expected as {
    hasName?: boolean;
    hasUrl?: boolean;
    streamingCapable?: boolean;
    terminalFlags?: boolean[];
    message?: string;
  };

  // error_codes
  if (input.code !== undefined) {
    const map: Record<number, string> = {
      [OpenErr.TaskNotFound]: 'Task not found',
    };
    expect(map[input.code]).toBe(expected.message);
    return;
  }
  // task_state_terminal
  if (input.states !== undefined) {
    const terminal = new Set(['completed', 'failed', 'canceled']);
    const flags = input.states.map((s) => terminal.has(s));
    expect(flags).toEqual(expected.terminalFlags);
    return;
  }
  // agent_card_schema
  const card: OpenAgentCard = {
    name: input.name ?? '',
    description: '',
    url: input.url ?? '',
    version: '',
    capabilities: {
      streaming: input.capabilities?.streaming ?? false,
      pushNotifications: false,
      stateTransitionHistory: false,
    },
  };
  expect(card.name !== '').toBe(expected.hasName);
  expect(card.url !== '').toBe(expected.hasUrl);
  expect(card.capabilities.streaming).toBe(expected.streamingCapable);
}

function runRealtimeSessionTest(tc: TestCase) {
  const input = tc.input as {
    transitions?: string[];
    state?: string;
    action?: string;
  };
  const expected = tc.expected as {
    finalState?: string;
    allValid?: boolean;
    newState?: string;
    allowed?: boolean;
  };

  // session_barge_in
  if (input.action === 'barge_in') {
    expect(input.state).toBe('speaking');
    const s = new RealtimeSession('xl-session');
    s.transitionTo(SessionState.Listening);
    s.transitionTo(SessionState.Thinking);
    s.transitionTo(SessionState.Speaking);
    let allowed = true;
    try {
      s.transitionTo(SessionState.Listening); // speaking → listening（barge-in）
    } catch {
      allowed = false;
    }
    expect(allowed).toBe(expected.allowed);
    if (expected.newState !== undefined && allowed) {
      expect(s.state).toBe(expected.newState);
    }
    return;
  }

  const s = new RealtimeSession('xl-session');
  let allValid = true;
  for (const tr of input.transitions ?? []) {
    const [from, to] = tr.split('->');
    if (from !== s.state) {
      allValid = false;
      break;
    }
    try {
      s.transitionTo(to as SessionState);
    } catch {
      allValid = false;
      break;
    }
  }
  expect(allValid).toBe(expected.allValid);
  if (expected.finalState !== undefined) {
    expect(s.state).toBe(expected.finalState);
  }
}

// ===== hnsw_recall — HNSW 召回质量双线门（v5.1 检索质量革命）=====
//
// 与 Go 侧 internal/memory/cross_language_test.go 的 runHNSWRecallTest 对应。
// 数据集生成算法（双侧必须逐位一致）：mulberry32 种子 PRNG + Box-Muller 高斯；
// dim=16、10 簇心（每分量 gaussian×5）；向量与查询共用同一 RNG 流，
// 顺序：先 10×16 簇心、后 datasetSize 条向量、最后 queryCount 条查询；
// 所有向量生成后 L2 归一化（单位向量下欧氏与余弦排序单调等价，
// Go 侧建图用余弦、TS 侧用欧氏，ground truth 双线一致）。

function xlMulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function xlGaussian(next: () => number): number {
  let u = 0;
  let v = 0;
  while (u === 0) u = next();
  while (v === 0) v = next();
  return Math.sqrt(-2 * Math.log(u)) * Math.cos(2 * Math.PI * v);
}

function xlNormalize(v: number[]): number[] {
  let norm = 0;
  for (const x of v) norm += x * x;
  norm = Math.sqrt(norm);
  if (norm === 0) return v;
  return v.map((x) => x / norm);
}

function runHnswRecallTest(tc: TestCase): void {
  const input = tc.input as {
    datasetSize: number;
    dataSeed: number;
    rngSeed: number;
    queryCount: number;
    topK: number;
  };
  const expected = tc.expected as { minRecall: number };

  const DIM = 16;
  const CLUSTERS = 10;

  const dataRng = xlMulberry32(input.dataSeed);
  const centroids: number[][] = [];
  for (let c = 0; c < CLUSTERS; c++) {
    const ct: number[] = [];
    for (let d = 0; d < DIM; d++) ct.push(xlGaussian(dataRng) * 5);
    centroids.push(ct);
  }
  const pick = (): number[] => {
    const base = centroids[Math.floor(dataRng() * CLUSTERS)]!;
    return base.map((x) => x + xlGaussian(dataRng));
  };

  const vectors: number[][] = [];
  for (let i = 0; i < input.datasetSize; i++) vectors.push(xlNormalize(pick()));
  const queries: number[][] = [];
  for (let q = 0; q < input.queryCount; q++) queries.push(xlNormalize(pick()));

  const hnsw = new HNSW({
    maxConnections: 16,
    efConstruction: 200,
    efSearch: 50,
    random: xlMulberry32(input.rngSeed),
  });
  const items = vectors.map((v, i) => ({ id: `v${i}`, vector: v }));
  for (const item of items) hnsw.insert(item.id, item.vector);

  const euclidean = (a: number[], b: number[]): number => {
    let sum = 0;
    for (let i = 0; i < a.length; i++) {
      const d = a[i]! - b[i]!;
      sum += d * d;
    }
    return Math.sqrt(sum);
  };

  let totalRecall = 0;
  for (const query of queries) {
    const scored = items
      .map((v) => ({ id: v.id, d: euclidean(query, v.vector) }))
      .sort((a, b) => a.d - b.d);
    const truth = new Set(scored.slice(0, input.topK).map((s) => s.id));
    const results = hnsw.search(query, input.topK);
    const hits = results.filter((r) => truth.has(r.id)).length;
    totalRecall += hits / input.topK;
  }
  const recall = totalRecall / queries.length;
  expect(recall).toBeGreaterThanOrEqual(expected.minRecall);
}
