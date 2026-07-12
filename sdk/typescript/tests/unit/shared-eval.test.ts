/**
 * 共享 Eval 用例与执行器单元测试。
 *
 * 验证 Go ↔ TS 共享用例一致性。
 */
import { describe, it, expect, vi } from 'vitest';
import {
  SHARED_EVAL_CASES,
  runSharedEval,
  evaluateWithKeyword,
  evaluateWithAnyKeyword,
  CategoryChat,
  CategorySafety,
  MetricAccuracy,
} from '../../src/eval/shared-cases.js';
import type { EvalAgent, EvalCase } from '../../src/eval/shared-cases.js';

describe('SHARED_EVAL_CASES', () => {
  it('should contain at least 5 cases', () => {
    expect(SHARED_EVAL_CASES.length).toBeGreaterThanOrEqual(5);
  });

  it('should have all required fields', () => {
    for (const c of SHARED_EVAL_CASES) {
      expect(c.id).toBeTruthy();
      expect(c.name).toBeTruthy();
      expect(c.input).toBeTruthy();
      expect(c.expected).toBeTruthy();
      expect(c.category).toBeTruthy();
      expect(c.threshold).toBeGreaterThan(0);
      expect(c.threshold).toBeLessThanOrEqual(1);
    }
  });

  it('should have unique IDs', () => {
    const ids = SHARED_EVAL_CASES.map((c) => c.id);
    const unique = new Set(ids);
    expect(unique.size).toBe(ids.length);
  });

  it('should include greeting case', () => {
    const greeting = SHARED_EVAL_CASES.find((c) => c.id === 'greeting');
    expect(greeting).toBeDefined();
    expect(greeting!.category).toBe(CategoryChat);
  });

  it('should include safety_pii case', () => {
    const safety = SHARED_EVAL_CASES.find((c) => c.id === 'safety_pii');
    expect(safety).toBeDefined();
    expect(safety!.category).toBe(CategorySafety);
    expect(safety!.metrics).toContain(MetricAccuracy);
  });
});

describe('evaluateWithKeyword', () => {
  it('should pass when output contains keyword', () => {
    const result = evaluateWithKeyword('Hello there!', 'Hello');
    expect(result.passed).toBe(true);
    expect(result.score).toBe(1.0);
  });

  it('should fail when output does not contain keyword', () => {
    const result = evaluateWithKeyword('Goodbye!', 'Hello');
    expect(result.passed).toBe(false);
    expect(result.score).toBe(0);
  });

  it('should handle empty output', () => {
    const result = evaluateWithKeyword('', 'Hello');
    expect(result.passed).toBe(false);
    expect(result.score).toBe(0);
  });
});

describe('evaluateWithAnyKeyword', () => {
  it('should pass when any keyword matches', () => {
    const result = evaluateWithAnyKeyword('search results found', ['lookup', 'search']);
    expect(result.passed).toBe(true);
  });

  it('should fail when no keyword matches', () => {
    const result = evaluateWithAnyKeyword('no match here', ['search', 'find']);
    expect(result.passed).toBe(false);
  });
});

describe('runSharedEval', () => {
  function createMockAgent(responses: Record<string, string>): EvalAgent {
    return {
      run: vi.fn(async ({ input }: { input: string }) => {
        for (const [kw, resp] of Object.entries(responses)) {
          if (input.includes(kw)) {
            return { output: resp };
          }
        }
        return { output: 'default response' };
      }),
    };
  }

  it('should run all cases', async () => {
    const agent = createMockAgent({
      Hello: 'Hello there!',
      Search: 'web_search results',
      memory: 'memory_recall: saved',
      SSN: 'block: PII',
      Plan: 'decompose: steps',
    });

    const result = await runSharedEval(agent);
    expect(result.total).toBe(SHARED_EVAL_CASES.length);
    expect(result.passed).toBeGreaterThan(0);
    expect(result.pass_rate).toBeGreaterThan(0);
  });

  it('should handle agent errors', async () => {
    const agent: EvalAgent = {
      run: vi.fn(async () => {
        throw new Error('agent failure');
      }),
    };

    const result = await runSharedEval(agent, SHARED_EVAL_CASES.slice(0, 1));
    expect(result.failed).toBe(1);
    expect(result.results[0].error).toBe('agent failure');
  });

  it('should calculate pass_rate correctly', async () => {
    const agent = createMockAgent({
      Hello: 'Hello there!',
    });

    const result = await runSharedEval(agent);
    expect(result.pass_rate).toBe(result.passed / result.total);
  });
});

describe('cross-language compat', () => {
  it('should have same case IDs as Go shared cases', () => {
    const ids = SHARED_EVAL_CASES.map((c) => c.id).sort();
    expect(ids).toEqual([
      'greeting',
      'memory_recall',
      'planning_decompose',
      'safety_pii',
      'tool_search_web',
    ]);
  });

  it('should produce JSON compatible with Go convention', () => {
    const json = JSON.stringify(SHARED_EVAL_CASES);
    // 验证字段名使用与 Go 端一致的命名
    expect(json).toContain('"threshold"');
    expect(json).toContain('"category"');
    expect(json).toContain('"difficulty"');
  });
});
