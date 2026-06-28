import type { Provider } from '../llm/provider.js';

// ===== Eval Types =====

export interface EvalToolCall {
  name: string;
  args: string;
}

export interface EvalResponse {
  content: string;
  toolCalls: EvalToolCall[];
}

export interface EvalInput {
  task: string;
  agentOutput: EvalResponse;
  expected: string;
  metadata?: Record<string, unknown>;
}

export interface CriterionResult {
  name: string;
  score: number;
  passed: boolean;
  reason: string;
}

export interface EvalResult {
  score: number;
  passed: boolean;
  criteria: CriterionResult[];
}

export interface EvalCase {
  task: string;
  input: string;
  expected: string;
  metadata?: Record<string, unknown>;
}

export interface CaseResult {
  case: EvalCase;
  score: number;
  passed: boolean;
  error?: Error;
}

export interface EvalSuiteResult {
  total: number;
  passed: number;
  failed: number;
  passRate: number;
  results: CaseResult[];
}

// ===== Evaluator Interface =====

export interface Evaluator {
  evaluate(input: EvalInput): Promise<EvalResult>;
}

// ===== Exact Match Evaluator =====

export class ExactMatchEvaluator implements Evaluator {
  private caseInsensitive: boolean;
  private normalizeWhitespace: boolean;

  constructor(opts?: { caseInsensitive?: boolean; normalizeWhitespace?: boolean }) {
    this.caseInsensitive = opts?.caseInsensitive ?? false;
    this.normalizeWhitespace = opts?.normalizeWhitespace ?? true;
  }

  async evaluate(input: EvalInput): Promise<EvalResult> {
    let actual = input.agentOutput.content;
    let expected = input.expected;

    if (this.normalizeWhitespace) {
      actual = actual.replace(/\s+/g, ' ').trim();
      expected = expected.replace(/\s+/g, ' ').trim();
    }

    if (this.caseInsensitive) {
      actual = actual.toLowerCase();
      expected = expected.toLowerCase();
    }

    const passed = actual === expected;
    return {
      score: passed ? 1.0 : 0.0,
      passed,
      criteria: [{
        name: 'exact_match',
        score: passed ? 1.0 : 0.0,
        passed,
        reason: passed ? 'Exact match' : 'Output does not match expected',
      }],
    };
  }
}

// ===== Contains Evaluator =====

export class ContainsEvaluator implements Evaluator {
  private caseInsensitive: boolean;

  constructor(opts?: { caseInsensitive?: boolean }) {
    this.caseInsensitive = opts?.caseInsensitive ?? false;
  }

  async evaluate(input: EvalInput): Promise<EvalResult> {
    let actual = input.agentOutput.content;
    let expected = input.expected;

    if (this.caseInsensitive) {
      actual = actual.toLowerCase();
      expected = expected.toLowerCase();
    }

    const passed = actual.includes(expected);
    return {
      score: passed ? 1.0 : 0.0,
      passed,
      criteria: [{
        name: 'contains',
        score: passed ? 1.0 : 0.0,
        passed,
        reason: passed ? 'Output contains expected text' : 'Output does not contain expected text',
      }],
    };
  }
}

// ===== Regex Evaluator =====

export class RegexEvaluator implements Evaluator {
  private pattern: RegExp;

  constructor(pattern: string | RegExp, flags?: string) {
    this.pattern = typeof pattern === 'string' ? new RegExp(pattern, flags) : pattern;
  }

  async evaluate(input: EvalInput): Promise<EvalResult> {
    const match = input.agentOutput.content.match(this.pattern);
    const passed = !!match;
    return {
      score: passed ? 1.0 : 0.0,
      passed,
      criteria: [{
        name: 'regex_match',
        score: passed ? 1.0 : 0.0,
        passed,
        reason: passed ? 'Output matches pattern' : 'Output does not match pattern',
      }],
    };
  }
}

// ===== LLM Evaluator =====

export class LLMEvaluator implements Evaluator {
  private provider: Provider;
  private model?: string;
  private criteria: string[];

  constructor(provider: Provider, criteria: string[], model?: string) {
    this.provider = provider;
    this.criteria = criteria;
    this.model = model;
  }

  async evaluate(input: EvalInput): Promise<EvalResult> {
    const criteriaList = this.criteria.map((c, i) => `${i + 1}. ${c}`).join('\n');
    const prompt = `Evaluate the following agent response against the expected result.

Task: ${input.task}
Agent Output: ${input.agentOutput.content}
Expected: ${input.expected}

Evaluate against these criteria:
${criteriaList}

Return JSON with:
- score: float 0-1
- passed: boolean
- criteria: array of {name, score, passed, reason}

Return ONLY valid JSON.`;

    const resp = await this.provider.complete({
      messages: [{ role: 'user', content: prompt }],
      model: this.model,
      temperature: 0,
    });

    return this.parseResult(resp.content);
  }

  private parseResult(text: string): EvalResult {
    try {
      const json = JSON.parse(text);
      return {
        score: json.score ?? 0,
        passed: json.passed ?? false,
        criteria: (json.criteria ?? []).map((c: Record<string, unknown>) => ({
          name: String(c.name ?? ''),
          score: Number(c.score ?? 0),
          passed: Boolean(c.passed ?? false),
          reason: String(c.reason ?? ''),
        })),
      };
    } catch {
      // Fallback: check if expected is contained
      return {
        score: 0,
        passed: false,
        criteria: [{ name: 'parse_error', score: 0, passed: false, reason: 'Could not parse LLM evaluation' }],
      };
    }
  }
}

// ===== Composite Evaluator =====

export class CompositeEvaluator implements Evaluator {
  private evaluators: { evaluator: Evaluator; weight: number }[];

  constructor(evaluators: { evaluator: Evaluator; weight: number }[]) {
    this.evaluators = evaluators;
    const totalWeight = evaluators.reduce((sum, e) => sum + e.weight, 0);
    if (totalWeight !== 1.0) {
      // Normalize weights
      const factor = 1.0 / totalWeight;
      this.evaluators = evaluators.map(e => ({ ...e, weight: e.weight * factor }));
    }
  }

  async evaluate(input: EvalInput): Promise<EvalResult> {
    const results = await Promise.all(
      this.evaluators.map(async ({ evaluator, weight }) => {
        const result = await evaluator.evaluate(input);
        return { result, weight };
      })
    );

    const totalScore = results.reduce((sum, { result, weight }) => sum + result.score * weight, 0);
    const allPassed = results.every(r => r.result.passed);
    const allCriteria = results.flatMap(r => r.result.criteria);

    return {
      score: totalScore,
      passed: allPassed,
      criteria: allCriteria,
    };
  }
}

// ===== Eval Suite =====

export interface EvalSuiteConfig {
  evaluator: Evaluator;
  cases: EvalCase[];
  agentRun?: (input: string) => Promise<EvalResponse>;
}

export class EvalSuite {
  private config: EvalSuiteConfig;

  constructor(config: EvalSuiteConfig) {
    this.config = config;
  }

  async run(): Promise<EvalSuiteResult> {
    const results: CaseResult[] = [];

    for (const testCase of this.config.cases) {
      try {
        let agentOutput: EvalResponse;
        if (this.config.agentRun) {
          agentOutput = await this.config.agentRun(testCase.input);
        } else {
          agentOutput = { content: '', toolCalls: [] };
        }

        const evalResult = await this.config.evaluator.evaluate({
          task: testCase.task,
          agentOutput,
          expected: testCase.expected,
          metadata: testCase.metadata,
        });

        results.push({
          case: testCase,
          score: evalResult.score,
          passed: evalResult.passed,
        });
      } catch (err) {
        results.push({
          case: testCase,
          score: 0,
          passed: false,
          error: err instanceof Error ? err : new Error(String(err)),
        });
      }
    }

    const passed = results.filter(r => r.passed).length;
    const failed = results.length - passed;

    return {
      total: results.length,
      passed,
      failed,
      passRate: results.length > 0 ? passed / results.length : 0,
      results,
    };
  }
}
