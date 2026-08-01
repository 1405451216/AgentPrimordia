import { defineConfig } from 'vitest/config';
import { fileURLToPath } from 'node:url';

export default defineConfig({
  esbuild: {
    jsx: 'automatic',
  },
  resolve: {
    alias: {
      'better-sqlite3': fileURLToPath(new URL('./tests/mocks/better-sqlite3.js', import.meta.url)),
    },
  },
  test: {
    globals: true,
    environment: 'node',
    include: ['tests/**/*.test.ts', 'src/**/__tests__/**/*.test.ts'],
    benchmark: {
      include: ['tests/bench/**/*.bench.ts', 'tests/bench/**/*-bench.ts'],
      reporter: 'default',
    },
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      include: ['src/**/*.ts'],
      exclude: ['src/persist/sqlite-checkpoint.ts'],
      thresholds: {
        // 阈值按 CI 实际运行的 Node 20 校准（v8 覆盖率在不同 Node 版本间
        // 存在行/语句计数差异，Node 26 下为 75.22%，Node 20 下为 72.84%，
        // 以 CI 环境为准并留 ~3 个百分点的余量）
        lines: 70,
        functions: 70,
        branches: 70,
        statements: 70,
      },
    },
  },
});
