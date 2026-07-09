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
    include: ['tests/**/*.test.ts'],
    benchmark: {
      include: ['tests/bench/**/*.bench.ts'],
      reporter: 'default',
    },
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      include: ['src/**/*.ts'],
      exclude: ['src/persist/sqlite-checkpoint.ts'],
      thresholds: {
        lines: 75,
        functions: 70,
        branches: 70,
        statements: 75,
      },
    },
  },
});
