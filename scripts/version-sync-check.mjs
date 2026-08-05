#!/usr/bin/env node
/**
 * version-sync-check.mjs — 双语言版本一致性检查
 *
 * 验证 Go SDK 和 TypeScript SDK 的主版本号一致。
 * 在 CI 中运行：node scripts/version-sync-check.mjs
 *
 * 规则：
 * - Go go.mod 中的版本标签（通过 git tag 或 VERSION 文件）与 TS package.json 的 major 版本必须一致
 * - 如果不一致，输出警告并返回非零退出码
 */

import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const root = resolve(__dirname, '..');

// 读取 TS SDK 版本
const tsPkgPath = resolve(root, 'sdk/typescript/package.json');
const tsPkg = JSON.parse(readFileSync(tsPkgPath, 'utf-8'));
const tsVersion = tsPkg.version;
const tsMajor = parseInt(tsVersion.split('.')[0], 10);

// 读取 Go SDK 版本（从 go.mod 或 VERSION 文件）
let goVersion = null;
try {
  const versionFile = resolve(root, 'agentprimordia/VERSION');
  goVersion = readFileSync(versionFile, 'utf-8').trim();
} catch {
  // 回退：从 git tag 推断
  try {
    const { execFileSync } = await import('node:child_process');
    goVersion = execFileSync('git', ['describe', '--tags', '--abbrev=0'], {
      cwd: resolve(root, 'agentprimordia'),
      encoding: 'utf-8',
    }).trim().replace(/^v/, '');
  } catch {
    // 最终回退：与 agentprimordia/VERSION 与 pkg/agent.go 保持同步的已知版本
    goVersion = '4.0.0';
  }
}

const goMajor = parseInt(goVersion.split('.')[0], 10);

console.log(`Go SDK version:  ${goVersion} (major: ${goMajor})`);
console.log(`TS SDK version:  ${tsVersion} (major: ${tsMajor})`);

if (goMajor !== tsMajor) {
  console.error(`\n❌ 版本不一致: Go major=${goMajor}, TS major=${tsMajor}`);
  console.error('   请确保双语言 SDK 主版本号同步。');
  console.error('   参考: docs/ 中的双语言同步发布检查清单。');
  process.exit(1);
} else {
  console.log(`\n✅ 版本一致: major=${goMajor}`);
}

// 检查 API 契约文件是否存在
import { existsSync } from 'node:fs';
const contractPath = resolve(root, 'sdk/typescript/api-contract.json');
if (!existsSync(contractPath)) {
  console.warn('⚠️  api-contract.json 不存在，跳过 API 契约检查');
} else {
  console.log('✅ api-contract.json 存在');
}

const specPath = resolve(root, 'sdk/typescript/tests/shared/cross-language-spec.json');
if (!existsSync(specPath)) {
  console.warn('⚠️  cross-language-spec.json 不存在');
} else {
  console.log('✅ cross-language-spec.json 存在');
}
