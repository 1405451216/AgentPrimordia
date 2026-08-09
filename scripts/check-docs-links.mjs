#!/usr/bin/env node
// check-docs-links.mjs — v4.4-4 文档死链门
//
// 扫描仓库 Markdown 文档中的相对链接（[text](./path) / [text](path)），
// 断言目标文件/锚点存在；发现死链退出码 1（docs-build CI 门）。
//
// 用法：
//   node scripts/check-docs-links.mjs [--root .]
import { readFileSync, existsSync, statSync } from 'node:fs';
import { resolve, dirname, join } from 'node:path';

const args = {};
for (let i = 2; i < process.argv.length; i++) {
  if (process.argv[i] === '--root') args.root = process.argv[++i];
}
const ROOT = resolve(args.root ?? '.');
const SKIP_PREFIX = ['http://', 'https://', 'mailto:', '#', 'skill://', 'rule://'];
const SKIP_DIRS = new Set(['node_modules', '.git', 'dist', '.claude', '.zcode', '.qoder', '.aelacli', '.pdf-build', '.promo-build']);

/** 递归收集 *.md 文件 */
function walk(dir, out) {
  for (const name of readFileSync ? listDir(dir) : []) {
    const p = join(dir, name);
    const st = statSync(p);
    if (st.isDirectory()) {
      if (SKIP_DIRS.has(name)) continue;
      walk(p, out);
    } else if (name.endsWith('.md')) {
      out.push(p);
    }
  }
  return out;
}

import { readdirSync } from 'node:fs';
function listDir(dir) {
  return readdirSync(dir);
}

const files = walk(ROOT, []);
const LINK_RE = /\[[^\]]*\]\(([^)]+)\)/g;
let broken = 0;
let checked = 0;

for (const file of files) {
  const content = readFileSync(file, 'utf8');
  const base = dirname(file);
  for (const m of content.matchAll(LINK_RE)) {
    const raw = m[1].trim();
    if (SKIP_PREFIX.some((p) => raw.startsWith(p))) continue;
    if (raw.includes('://')) continue;
    // 站点根路径（vitepress /api/ 等）非文件链接
    if (raw.startsWith('/')) continue;
    // 去掉锚点部分
    const target = raw.split('#')[0];
    if (!target) continue;
    const abs = resolve(base, target);
    checked++;
    if (!existsSync(abs)) {
      console.error(`死链: ${file.replace(ROOT, '.')} → ${raw}`);
      broken++;
    }
  }
}

if (broken > 0) {
  console.error(`\n文档死链门: ${broken} 条死链（共检查 ${checked} 条相对链接）`);
  process.exit(1);
}
console.log(`文档死链门: 通过（${checked} 条相对链接全部可达）`);
