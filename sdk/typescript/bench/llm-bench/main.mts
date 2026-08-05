/**
 * AgentPrimordia TS 端真实 LLM 跑分工具（v3.7-2）。
 *
 * 与 Go 端 bench/llm-bench 对齐：对同一套 harness 基准集
 * （HARNESS_BENCHMARK_CASES，60 条真实编码任务）跑真实 LLM Provider，
 * 产出含 成功率/成本/耗时/恢复率 的 JSON 报告，供双线分数对比。
 *
 * 用法：
 *   OPENAI_API_KEY=sk-xxx npx tsx bench/llm-bench/main.mts --model gpt-4o-mini
 */
import { HARNESS_BENCHMARK_CASES } from '../../src/eval/benchmark-cases.js';
import { codeConstructScore } from '../../src/eval/benchmark-eval.js';
import { OpenAIProvider } from '../../src/llm/openai.js';
import type { Provider } from '../../src/llm/provider.js';
import type { CompletionRequest } from '../../src/types.js';
import { writeFileSync } from 'node:fs';

interface BenchCaseResult {
  case_id: string;
  phase?: string;
  lang?: string;
  passed: boolean;
  score: number;
  duration_ms: number;
  attempts: number;
  recovered?: boolean;
  error?: string;
}

interface BenchReport {
  version: string;
  model: string;
  provider: string;
  total: number;
  passed: number;
  failed: number;
  pass_rate: number;
  cost_usd: number;
  latency_ms: number;
  avg_latency_ms: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  recovery_rate: number;
  threshold: number;
  generated: string;
  cases: BenchCaseResult[];
}

function parseArgs(argv: string[]) {
  const args: Record<string, string | boolean> = {};
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a.startsWith('--')) {
      const eq = a.indexOf('=');
      if (eq > 0) {
        args[a.slice(2, eq)] = a.slice(eq + 1);
      } else if (i + 1 < argv.length && !argv[i + 1].startsWith('--')) {
        args[a.slice(2)] = argv[++i];
      } else {
        args[a.slice(2)] = true;
      }
    }
  }
  return args;
}

const args = parseArgs(process.argv);
const model = (args.model as string) ?? 'gpt-4o-mini';
const version = (args.version as string) ?? 'dev';
const threshold = parseFloat((args.threshold as string) ?? '0.8');
const retries = parseInt((args.retries as string) ?? '1', 10);
const out = (args.out as string) ?? 'ts-llm-bench-report.json';
const limit = parseInt((args.limit as string) ?? '0', 10);

const apiKey = process.env.OPENAI_API_KEY ?? process.env.LLM_BENCH_API_KEY;
if (!apiKey) {
  console.error('ERROR: OPENAI_API_KEY 未配置');
  process.exit(2);
}

const provider: Provider = new OpenAIProvider({ apiKey, model });

const cases = HARNESS_BENCHMARK_CASES.slice(0, limit > 0 ? Math.min(limit, HARNESS_BENCHMARK_CASES.length) : HARNESS_BENCHMARK_CASES.length);

const systemPrompt = '你是软件工程 Agent，直接给出可验证的输出（代码或结论），不要输出无关解释。';

async function runCase(c: (typeof cases)[number]): Promise<BenchCaseResult> {
  const start = Date.now();
  let output = '';
  let error: string | undefined;
  try {
    const req: CompletionRequest = {
      messages: [
        { role: 'system', content: systemPrompt },
        { role: 'user', content: c.input },
      ],
      temperature: 0,
      maxTokens: 1024,
    };
    const resp = await provider.complete(req);
    output = resp.content;
  } catch (err) {
    error = err instanceof Error ? err.message : String(err);
  }
  const duration = Date.now() - start;
  const score = codeConstructScore(c, output);
  return { case_id: c.id, phase: c.harness_phase, lang: c.lang, passed: score.passed && !error, score: score.score, duration_ms: duration, attempts: 1, error };
}

async function main() {
  const report: BenchReport = {
    version, model, provider: 'openai', total: cases.length, passed: 0, failed: 0,
    pass_rate: 0, cost_usd: 0, latency_ms: 0, avg_latency_ms: 0,
    prompt_tokens: 0, completion_tokens: 0, total_tokens: 0, recovery_rate: 1,
    threshold, generated: new Date().toISOString(), cases: [],
  };

  const startAll = Date.now();
  for (const c of cases) {
    let result = await runCase(c);
    // 失败重试（恢复率）
    if (!result.passed && result.attempts <= retries) {
      for (let a = 1; a <= retries && !result.passed; a++) {
        result.attempts++;
        const retry = await runCase(c);
        if (retry.passed) {
          result = { ...retry, attempts: result.attempts, recovered: true };
        }
      }
    }
    if (result.passed) report.passed++; else report.failed++;
    report.cases.push(result);
  }
  report.latency_ms = Date.now() - startAll;
  report.avg_latency_ms = report.total > 0 ? Math.round(report.latency_ms / report.total) : 0;
  report.pass_rate = report.total > 0 ? report.passed / report.total : 0;
  const recovered = report.cases.filter((c) => c.recovered).length;
  report.recovery_rate = report.failed > 0 || recovered > 0 ? recovered / (report.failed + recovered) : 1;

  // 汇总 token 用量（TS provider 不暴露 usage 时的兜底：无）
  for (const c of report.cases) {
    // usage 由 provider.complete 返回但 TS OpenAIProvider 未暴露字段；此处预留
  }

  const payload = JSON.stringify(report, null, 2);
  writeFileSync(out, payload, 'utf8');

  console.log(`==> TS LLM 跑分完成`);
  console.log(`    Model:    ${model}`);
  console.log(`    Total:    ${report.total}  Passed: ${report.passed}  Failed: ${report.failed}`);
  console.log(`    PassRate: ${report.pass_rate.toFixed(4)}`);
  console.log(`    Recovery: ${report.recovery_rate.toFixed(4)}`);
  console.log(`    Latency:  ${report.latency_ms}ms total / ${report.avg_latency_ms}ms avg`);
  console.log(`    报告已写入 ${out}`);
  process.exit(report.pass_rate >= threshold ? 0 : 1);
}

void main();
