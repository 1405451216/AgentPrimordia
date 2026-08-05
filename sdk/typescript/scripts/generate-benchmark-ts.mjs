// 从 Go 权威 JSON 生成 TS 基准集镜像。
// 权威来源: agentprimordia/internal/eval/benchmark_cases.json
// 用法: node scripts/generate-benchmark-ts.mjs
import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const jsonPath = join(here, '..', '..', '..', 'agentprimordia', 'internal', 'eval', 'benchmark_cases.json');
const outPath = join(here, '..', 'src', 'eval', 'benchmark-cases.ts');

const cases = JSON.parse(readFileSync(jsonPath, 'utf8'));
if (!Array.isArray(cases) || cases.length < 50) {
  throw new Error(`基准集用例数 = ${cases?.length}, 要求 ≥50`);
}

const header = `/**
 * 真实 harness 基准集（v3.5-1）— GENERATED 文件，请勿手工编辑。
 * 权威来源: agentprimordia/internal/eval/benchmark_cases.json
 * 重新生成: node scripts/generate-benchmark-ts.mjs
 */
import type { EvalCase } from './shared-cases.js';

/** 真实 harness 基准集（与 Go 端 HarnessBenchmarkCases() 严格一致） */
export const HARNESS_BENCHMARK_CASES: EvalCase[] = `;

// 逐条序列化为合法 TS 对象字面量
function toTS(c) {
  const fields = Object.entries(c).map(([k, v]) => {
    if (v === undefined) return null;
    if (typeof v === 'string') return `    ${k}: '${v.replace(/\\/g, '\\\\').replace(/'/g, "\\'")}'`;
    if (Array.isArray(v)) return `    ${k}: [${v.map((x) => `'${x}'`).join(', ')}]`;
    if (typeof v === 'number') return `    ${k}: ${v}`;
    if (typeof v === 'boolean') return `    ${k}: ${v}`;
    if (typeof v === 'object') {
      const inner = Object.entries(v).map(([ik, iv]) => `${ik}: '${iv}'`).join(', ');
      return `    ${k}: { ${inner} }`;
    }
    return null;
  }).filter(Boolean);
  return `  {\n${fields.join(',\n')}\n  }`;
}

const arr = cases.map((c) => toTS(c)).join(',\n');

writeFileSync(outPath, `${header}[\n${arr},\n];\n`, 'utf8');
console.log(`已生成 ${outPath}（${cases.length} 条）`);
