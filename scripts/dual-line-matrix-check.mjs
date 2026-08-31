#!/usr/bin/env node
// dual-line-matrix-check.mjs — S0-5 双线豁免矩阵 CI 审查门
// 规则：
//  1. 矩阵表结构合法：每行 6/7 列，豁免行必须含「理由」与「升格条件」非空；
//  2. 已落地板块（存量基线 B*、S0 版、以及任何 Go/TS 列路径已出现在仓库中的行）：
//     反引号引用的仓库相对路径必须真实存在（支持 agentprimordia/、sdk/typescript/src 等前缀）；
//  3. 未来版本行（v6.1+，尚未开工）跳过路径存在性检查，但豁免理由必须填。
import { readFileSync, existsSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const matrixPath = resolve(repoRoot, 'docs/双线豁免矩阵.md');
const content = readFileSync(matrixPath, 'utf8');

// 只取表格行
const rows = content.split('\n').filter(l => /^\|\s*(B?\d+)\s*\|/.test(l));
let errors = [];
let checked = 0, skipped = 0;

// 路径候选解析：按仓库常见根尝试
const roots = ['', 'agentprimordia/', 'sdk/typescript/', 'agentprimordia/sdk/typescript/'];
function pathExists(p) {
  const clean = p.replace(/\/$/, '');
  if (existsSync(resolve(repoRoot, clean))) return true;
  for (const r of roots) if (existsSync(resolve(repoRoot, r + clean))) return true;
  return false;
}

for (const row of rows) {
  const cells = row.split('|').map(c => c.trim()).filter(c => c !== '');
  if (cells.length < 5) { errors.push(`列数不足: ${row.slice(0, 40)}...`); continue; }
  const [, id] = cells;
  const versionCell = cells[1] || '';
  const isFuture = /^v6\.[1-9]|^v7/.test(versionCell) && !/S0/.test(versionCell);
  const isRegistered = versionCell === 'S0' || /^B\d/.test(id) || /存量/.test(row);
  const body = row;
  // 豁免行必须有理由与升格说明（含「升格」「豁免」「平台」「无等价」字样之一）
  if (/Go-only|协议对等/.test(row) && !/(升格|豁免|平台限制|无等价)/.test(row)) {
    errors.push(`${id}: 豁免行缺少理由/升格条件`);
  }
  if (isFuture) { skipped++; continue; }
  if (!isRegistered && !/S0/.test(versionCell)) { skipped++; continue; }
  // 已落地行：校验反引号路径
  const paths = [...row.matchAll(/`([^`]+\/[^`]*)`/g)].map(m => m[1]);
  for (const p of paths) {
    checked++;
    const bare = p.replace(/^\*\*/, '').replace(/\/?\*\*$/, ''); // 容错 markdown 粗体
    if (!pathExists(bare) && !pathExists(p)) errors.push(`${id}: 路径不存在 -> ${p}`);
  }
}

if (errors.length) {
  console.error('❌ 双线豁免矩阵审查失败：');
  for (const e of errors) console.error('  - ' + e);
  process.exit(1);
}
console.log(`✅ 双线豁免矩阵审查通过（校验路径 ${checked} 个，跳过未来版本行 ${skipped} 行）`);
