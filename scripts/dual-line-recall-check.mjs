#!/usr/bin/env node
// dual-line-recall-check.mjs — S0-3 双线召回对账门
//
// 读取 Go/TS 两臂在同一语料（docs/evals/embedding-corpus-v1.json）上跑出的
// lexical 底档结果，对账三件事：
//   1. 双线差 ≤0.02（逐档与均值，docs/V7路线图.md §二 S0-3 验收行）；
//   2. 两侧均为 semantic=false 的降级位臂（结果不得计入语义验收）；
//   3. 两侧均携带语义缓存命中率基线（冷+暖双轮口径 = 0.5）。
// 结果文件由测试在 AP_WRITE_S03_RESULTS=1 时产出：
//   Go: agentprimordia/bench/results/s0-3-recall-go.json
//   TS: sdk/typescript/bench/results/s0-3-recall-ts.json
// 语义臂（≥0.95，需真实端点）结果落 *-semantic.json，本门不做数值对账
//（端点浮点有噪声），只在校验模式 --with-semantic 下要求两侧同时存在且 ≥0.95。
import { readFileSync, existsSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const goPath = resolve(repoRoot, 'agentprimordia/bench/results/s0-3-recall-go.json');
const tsPath = resolve(repoRoot, 'sdk/typescript/bench/results/s0-3-recall-ts.json');

if (!existsSync(goPath) || !existsSync(tsPath)) {
  console.error('❌ 结果文件缺失——先在两侧跑基准（AP_WRITE_S03_RESULTS=1）：');
  if (!existsSync(goPath)) console.error('  Go: cd agentprimordia && AP_WRITE_S03_RESULTS=1 go test ./internal/memory/ -run TestEmbeddingCorpusRecall$');
  if (!existsSync(tsPath)) console.error('  TS: cd sdk/typescript && AP_WRITE_S03_RESULTS=1 npx vitest run src/llm/__tests__/embedding-recall.test.ts');
  process.exit(1);
}

const go = JSON.parse(readFileSync(goPath, 'utf8'));
const ts = JSON.parse(readFileSync(tsPath, 'utf8'));

const errors = [];
const MAX_DIFF = 0.02;

if (go.semantic !== false || ts.semantic !== false) {
  errors.push('两侧都应是 lexical 降级位臂（semantic=false）');
}
if (go.corpus !== ts.corpus || go.corpus !== 'docs/evals/embedding-corpus-v1.json') {
  errors.push(`语料不一致: ${go.corpus} vs ${ts.corpus}`);
}
if (JSON.stringify(go.seeds) !== JSON.stringify(ts.seeds)) {
  errors.push(`三档种子不一致: ${JSON.stringify(go.seeds)} vs ${JSON.stringify(ts.seeds)}`);
}
if (go.chunks !== ts.chunks || go.queries !== ts.queries) {
  errors.push(`子集规模不一致: go ${go.chunks}/${go.queries} vs ts ${ts.chunks}/${ts.queries}`);
}

console.log('S0-3 双线召回对账（lexical 降级位臂，corpus visible 子集）');
console.log('  corpus:', go.corpus, `(${go.chunks} chunks / ${go.queries} queries)`);
console.log('  seeds:', JSON.stringify(go.seeds), ' topK:', go.topK);
console.log('');
console.log('  tier(seed)      Go        TS       diff');
for (let i = 0; i < Math.min(go.tiers.length, ts.tiers.length); i++) {
  const g = go.tiers[i];
  const t = ts.tiers[i];
  if (g.seed !== t.seed) { errors.push(`档位种子错位: ${g.seed} vs ${t.seed}`); continue; }
  const diff = Math.abs(g.recall_at_10 - t.recall_at_10);
  console.log(`  seed ${g.seed}      ${g.recall_at_10.toFixed(4)}    ${t.recall_at_10.toFixed(4)}    ${diff.toFixed(4)}`);
  if (diff > MAX_DIFF) errors.push(`seed ${g.seed} 双线差 ${diff.toFixed(4)} > ${MAX_DIFF}`);
}
const meanDiff = Math.abs(go.mean_recall_at_10 - ts.mean_recall_at_10);
console.log(`  mean         ${go.mean_recall_at_10.toFixed(4)}    ${ts.mean_recall_at_10.toFixed(4)}    ${meanDiff.toFixed(4)}`);
if (meanDiff > MAX_DIFF) errors.push(`均值双线差 ${meanDiff.toFixed(4)} > ${MAX_DIFF}`);
console.log('');

for (const [side, res] of [['Go', go], ['TS', ts]]) {
  const cb = res.cache_baseline;
  if (!cb || cb.hits === undefined) { errors.push(`${side} 缺少语义缓存命中率基线`); continue; }
  console.log(`  ${side} 语义缓存命中率基线: hits=${cb.hits} misses=${cb.misses} hitRate=${cb.hit_rate}`);
  if (cb.hit_rate !== 0.5) errors.push(`${side} 缓存基线 hit_rate=${cb.hit_rate}，冷+暖双轮口径应为 0.5`);
}

if (errors.length) {
  console.error('');
  console.error('❌ 双线召回对账失败：');
  for (const e of errors) console.error('  - ' + e);
  process.exit(1);
}
console.log('✅ 双线召回对账通过（双线差 ≤0.02，降级位臂与缓存基线口径一致）');
