#!/usr/bin/env node
/**
 * 包体积分析脚本
 *
 * 使用 esbuild API 分析 SDK 包体积，输出：
 * - 各入口点打包后的大小
 * - 模块依赖图
 * - 大型依赖警告
 *
 * 用法：node scripts/analyze-bundle.js
 */

const esbuild = require('esbuild');
const fs = require('fs');
const path = require('path');

const ROOT = path.resolve(__dirname, '..');
const SRC = path.join(ROOT, 'src');

// ===== 分析配置 =====

/** 入口点列表 */
const ENTRIES = [
  { name: 'main', path: path.join(SRC, 'index.ts') },
  { name: 'agent', path: path.join(SRC, 'agent', 'react-loop.ts') },
  { name: 'llm', path: path.join(SRC, 'llm', 'provider.ts') },
  { name: 'tools', path: path.join(SRC, 'tools', 'registry.ts') },
];

/** 警告阈值 (KB) */
const SIZE_WARNING_THRESHOLD = 50;

/**
 * 格式化字节数为可读字符串
 */
function formatSize(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

/**
 * 分析单个入口点
 */
async function analyzeEntry(entry) {
  try {
    const result = await esbuild.build({
      entryPoints: [entry.path],
      bundle: true,
      write: false,
      format: 'esm',
      platform: 'node',
      target: 'es2022',
      minify: true,
      metafile: true,
      external: ['react', 'react-dom', 'better-sqlite3'],
      logLevel: 'silent',
    });

    const outputs = result.metafile.outputs;
    let totalSize = 0;
    let largestModule = { name: '', size: 0 };

    for (const [file, info] of Object.entries(outputs)) {
      if (file.endsWith('.js') || file.endsWith('.mjs')) {
        totalSize += info.bytes;
        // 分析各输入模块
        for (const [input, inputInfo] of Object.entries(result.metafile.inputs)) {
          if (inputInfo.bytes > largestModule.size && input.includes('src/')) {
            largestModule = { name: input, size: inputInfo.bytes };
          }
        }
      }
    }

    return {
      name: entry.name,
      size: totalSize,
      formattedSize: formatSize(totalSize),
      warning: totalSize > SIZE_WARNING_THRESHOLD * 1024,
      largestModule,
    };
  } catch (err) {
    return {
      name: entry.name,
      size: 0,
      formattedSize: 'ERROR',
      warning: false,
      error: err.message,
    };
  }
}

/**
 * 运行全部分析
 */
async function run() {
  console.log('Bundle Analysis Report');
  console.log('=====================\n');

  const results = [];
  for (const entry of ENTRIES) {
    const result = await analyzeEntry(entry);
    results.push(result);
  }

  for (const r of results) {
    if (r.error) {
      console.log(`[${r.name}] ERROR: ${r.error}`);
    } else {
      const flag = r.warning ? ' [WARNING: Large bundle]' : '';
      console.log(`[${r.name}] ${r.formattedSize}${flag}`);
      if (r.largestModule.name) {
        console.log(`  Largest module: ${r.largestModule.name} (${formatSize(r.largestModule.size)})`);
      }
    }
  }

  // 输出 CSV 格式（方便 CI 解析）
  console.log('\n---CSV---');
  console.log('entry,size_bytes,has_warning');
  for (const r of results) {
    console.log(`${r.name},${r.size},${r.warning}`);
  }
}

run().catch(err => {
  console.error('Bundle analysis failed:', err);
  process.exit(1);
});