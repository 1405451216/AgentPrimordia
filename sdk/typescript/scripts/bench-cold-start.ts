/**
 * bench-cold-start.ts — 边缘冷启动基准测试脚本
 *
 * 在 Cloudflare Workers / Deno Deploy / Bun 环境中运行，
 * 量化 ColdStartOptimizer.analyze() 的真实冷启动时间。
 *
 * 用法：
 *   # Deno Deploy
 *   deno run --allow-all scripts/bench-cold-start.ts
 *
 *   # Bun
 *   bun run scripts/bench-cold-start.ts
 *
 *   # Cloudflare Workers (wrangler dev)
 *   # 将本文件作为 Worker 入口，访问 /bench 端点
 *
 * 产出：冷启动时间基线 + 优化前后对比
 */

import { ColdStartOptimizer } from '../src/edge/cold-start.js';

async function runBenchmark(): Promise<void> {
  const startTime = performance.now();

  const optimizer = new ColdStartOptimizer();
  const report = await optimizer.analyze();

  const totalMs = performance.now() - startTime;

  console.log('=== AgentPrimordia Edge Cold Start Benchmark ===');
  console.log(`Runtime: ${report.runtime}`);
  console.log(`Is Edge: ${report.isEdge}`);
  console.log(`Cold Start: ${report.coldStartMs.toFixed(2)}ms`);
  console.log(`Total (incl. analyze): ${totalMs.toFixed(2)}ms`);
  console.log(`Loaded Modules: ${report.loadedModules}`);
  console.log(`Memory: ${report.memoryUsageMB.toFixed(1)}MB`);
  console.log('');
  console.log('Suggestions:');
  for (const s of report.suggestions) {
    console.log(`  - [${s.priority}] ${s.description} (est. saving: ${s.estimatedSavingMs}ms)`);
  }
  console.log('');
  console.log(JSON.stringify({
    runtime: report.runtime,
    isEdge: report.isEdge,
    coldStartMs: report.coldStartMs,
    totalMs,
    loadedModules: report.loadedModules,
    memoryUsageMB: report.memoryUsageMB,
    timestamp: new Date().toISOString(),
  }, null, 2));
}

runBenchmark().catch(console.error);
