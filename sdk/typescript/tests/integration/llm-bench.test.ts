/**
 * TS 端真实 LLM 跑分集成测试（v3.7-2，建立双线可比基线）。
 *
 * 需要 OPENAI_API_KEY（或 LLM_BENCH_API_KEY）；未配置时跳过。
 * 默认仅跑 3 条用例控制成本；LLM_BENCH_FULL=1 时跑完整基准集。
 */
import { describe, it, expect } from 'vitest';
import { HARNESS_BENCHMARK_CASES } from '../../src/eval/benchmark-cases.js';
import { codeConstructScore } from '../../src/eval/benchmark-eval.js';
import { OpenAIProvider } from '../../src/llm/openai.js';
import type { CompletionRequest } from '../../src/types.js';

const apiKey = process.env.OPENAI_API_KEY ?? process.env.LLM_BENCH_API_KEY;
const describeOrSkip = apiKey ? describe : describe.skip;

const SYSTEM_PROMPT = '你是软件工程 Agent，直接给出可验证的输出（代码或结论），不要输出无关解释。';

describeOrSkip('TS Real LLM Benchmark (v3.7-2 baseline)', () => {
  it('should run real LLM on shared benchmark cases and produce comparable pass rate', async () => {
    const provider = new OpenAIProvider({
      apiKey: apiKey!,
      model: process.env.LLM_BENCH_MODEL ?? 'gpt-4o-mini',
    });

    let cases = HARNESS_BENCHMARK_CASES;
    if (process.env.LLM_BENCH_FULL !== '1') {
      cases = cases.slice(0, 3); // 控制成本
    }

    let passed = 0;
    const results: Array<{ case_id: string; passed: boolean; score: number }> = [];
    for (const c of cases) {
      const resp = await provider.complete({
        messages: [
          { role: 'system', content: SYSTEM_PROMPT },
          { role: 'user', content: c.input },
        ],
        temperature: 0,
        maxTokens: 1024,
      } as CompletionRequest);

      const score = codeConstructScore(c, resp.content);
      results.push({ case_id: c.id, passed: score.passed, score: score.score });
      if (score.passed) passed++;
    }

    const passRate = passed / cases.length;
    // 基线断言：真实 LLM 在基准集上有基本成功率（>0），并记录分数
    expect(passRate).toBeGreaterThan(0);
    expect(results.length).toBe(cases.length);

    // 输出基线摘要（供 CI 对比）
    // eslint-disable-next-line no-console
    console.log(`TS LLM 基线: total=${cases.length} passed=${passed} pass_rate=${passRate.toFixed(3)}`);
  }, 5 * 60 * 1000);
});
