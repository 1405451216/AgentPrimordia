#!/usr/bin/env node
// dual-bench-compare.mjs — v4.2-5 双线真实 LLM 基准对照表生成
//
// 输入：Go 侧 bench/llm-bench 报告 + TS 侧 sdk/typescript/bench/llm-bench 报告
// （同一模型、同一基准集跑分产出），输出：
//   - dual-bench-comparison.json  结构化对照（逐指标 + 逐用例）
//   - dual-bench-comparison.md    人类可读对照表
//
// 可比化契约：
//   - 两侧必须报告相同用例集（case_id 集合一致），否则退出码 1
//   - 逐用例标记两侧判定分歧（一侧通过一侧失败）→ 对照表标 ⚠
//   - 指标对照不设门禁（真实 LLM 数值由发布流程人工复核），退出码只反映可比性
//
// 用法：
//   node scripts/dual-bench-compare.mjs \
//     --go-report go-llm-bench-report.json \
//     --ts-report ts-llm-bench-report.json \
//     --out bench/results/dual
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';

function parseArgs(argv) {
  const args = {};
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a.startsWith('--')) {
      const key = a.slice(2);
      args[key] = argv[i + 1] && !argv[i + 1].startsWith('--') ? argv[i + 1] : true;
      if (args[key] !== true) i++;
    }
  }
  return args;
}

const args = parseArgs(process.argv);
if (!args['go-report'] || !args['ts-report']) {
  console.error('用法: node scripts/dual-bench-compare.mjs --go-report <json> --ts-report <json> [--out <dir>]');
  process.exit(2);
}

/** 归一化报告字段（Go 与 TS 形状一致）。 */
function loadReport(path) {
  const raw = JSON.parse(readFileSync(path, 'utf8'));
  return {
    version: raw.version ?? 'dev',
    model: raw.model ?? '?',
    provider: raw.provider ?? '?',
    total: raw.total ?? 0,
    passed: raw.passed ?? 0,
    failed: raw.failed ?? 0,
    pass_rate: raw.pass_rate ?? 0,
    cost_usd: raw.cost_usd ?? 0,
    avg_latency_ms: raw.avg_latency_ms ?? 0,
    recovery_rate: raw.recovery_rate ?? 0,
    prompt_tokens: raw.prompt_tokens ?? 0,
    completion_tokens: raw.completion_tokens ?? 0,
    total_tokens: raw.total_tokens ?? 0,
    generated: raw.generated ?? '',
    cases: raw.cases ?? [],
  };
}

const go = loadReport(args['go-report']);
const ts = loadReport(args['ts-report']);

// ---- 可比化契约：用例集一致 ----
const goIDs = new Set(go.cases.map((c) => c.case_id));
const tsIDs = new Set(ts.cases.map((c) => c.case_id));
const onlyGo = [...goIDs].filter((id) => !tsIDs.has(id));
const onlyTS = [...tsIDs].filter((id) => !goIDs.has(id));
if (onlyGo.length > 0 || onlyTS.length > 0) {
  console.error(`可比化失败：用例集不一致（Go 独有 ${onlyGo.length}，TS 独有 ${onlyTS.length}）`);
  if (onlyGo.length) console.error('  Go 独有: ' + onlyGo.slice(0, 5).join(', '));
  if (onlyTS.length) console.error('  TS 独有: ' + onlyTS.slice(0, 5).join(', '));
  process.exit(1);
}

// ---- 逐用例对照 ----
const goByID = new Map(go.cases.map((c) => [c.case_id, c]));
const tsByID = new Map(ts.cases.map((c) => [c.case_id, c]));
const perCase = go.cases.map((gc) => {
  const tc = tsByID.get(gc.case_id);
  const gPass = Boolean(gc.passed);
  const tPass = Boolean(tc?.passed);
  return {
    case_id: gc.case_id,
    phase: gc.phase ?? '',
    lang: gc.lang ?? '',
    go: { passed: gPass, score: gc.score ?? 0, duration_ms: gc.duration_ms ?? 0 },
    ts: { passed: tPass, score: tc?.score ?? 0, duration_ms: tc?.duration_ms ?? 0 },
    divergent: gPass !== tPass,
  };
});
const divergences = perCase.filter((c) => c.divergent);

// ---- 指标对照表 ----
const metrics = [
  ['模型', go.model, ts.model],
  ['用例数', go.total, ts.total],
  ['通过数', go.passed, ts.passed],
  ['通过率', go.pass_rate.toFixed(4), ts.pass_rate.toFixed(4)],
  ['成本(USD)', go.cost_usd.toFixed(6), ts.cost_usd.toFixed(6)],
  ['平均延迟(ms)', go.avg_latency_ms, ts.avg_latency_ms],
  ['恢复率', go.recovery_rate.toFixed(4), ts.recovery_rate.toFixed(4)],
  ['Prompt tokens', go.prompt_tokens, ts.prompt_tokens],
  ['Completion tokens', go.completion_tokens, ts.completion_tokens],
];

// ---- 输出 JSON ----
const outDir = args.out ? resolve(args.out) : resolve('bench/results/dual');
mkdirSync(outDir, { recursive: true });
const result = {
  generated: new Date().toISOString(),
  go: { version: go.version, model: go.model, provider: go.provider, pass_rate: go.pass_rate, cost_usd: go.cost_usd, avg_latency_ms: go.avg_latency_ms, recovery_rate: go.recovery_rate },
  ts: { version: ts.version, model: ts.model, provider: ts.provider, pass_rate: ts.pass_rate, cost_usd: ts.cost_usd, avg_latency_ms: ts.avg_latency_ms, recovery_rate: ts.recovery_rate },
  comparable: { case_set_identical: true, total: perCase.length, divergent: divergences.length },
  metrics,
  per_case: perCase,
};
writeFileSync(resolve(outDir, 'dual-bench-comparison.json'), JSON.stringify(result, null, 2));

// ---- 输出 Markdown ----
const lines = [];
lines.push('# 双线真实 LLM 基准对照表（Go / TS）');
lines.push('');
lines.push(`> 生成时间：${result.generated} ｜ 用例集可比：✅ ${perCase.length} 条一致 ｜ 判定分歧：${divergences.length} 条`);
lines.push('');
lines.push('## 指标对照');
lines.push('');
lines.push('| 指标 | Go | TS |');
lines.push('|------|----|----|');
for (const [name, g, t] of metrics) lines.push(`| ${name} | ${g} | ${t} |`);
lines.push('');
lines.push('## 逐用例判定分歧');
lines.push('');
if (divergences.length === 0) {
  lines.push('（无分歧：两侧判定一致）');
} else {
  lines.push('| case_id | Go | TS |');
  lines.push('|---------|----|----|');
  for (const c of divergences) lines.push(`| ${c.case_id} | ${c.go.passed ? '✅' : '❌'} | ${c.ts.passed ? '✅' : '❌'} |`);
}
lines.push('');
writeFileSync(resolve(outDir, 'dual-bench-comparison.md'), lines.join('\n'));

console.log(`双线对照表已生成: ${outDir}`);
console.log(`  指标: 通过率 Go=${(go.pass_rate * 100).toFixed(1)}% TS=${(ts.pass_rate * 100).toFixed(1)}% | 成本 Go=$${go.cost_usd.toFixed(4)} TS=$${ts.cost_usd.toFixed(4)} | 延迟 Go=${go.avg_latency_ms}ms TS=${ts.avg_latency_ms}ms`);
console.log(`  可比性: 用例集 ${perCase.length} 条一致，判定分歧 ${divergences.length} 条`);
if (divergences.length > 0) {
  console.warn(`⚠ ${divergences.length} 条用例两侧判定分歧（见 per_case.divergent）`);
}
