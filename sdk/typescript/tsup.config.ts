import { defineConfig } from 'tsup';

export default defineConfig([
  // Node.js 构建（默认）
  {
    entry: ['src/index.ts'],
    format: ['esm', 'cjs'],
    dts: true,
    splitting: false,
    sourcemap: true,
    clean: true,
    treeshake: true,
    target: 'es2022',
    outDir: 'dist',
    platform: 'node',
  },
  // Browser 构建（ESM only，排除 Node 专属模块）
  {
    entry: ['src/index.ts'],
    format: ['esm'],
    dts: false,
    splitting: false,
    sourcemap: true,
    clean: false,
    treeshake: true,
    target: 'es2022',
    outDir: 'dist/browser',
    platform: 'browser',
    external: ['better-sqlite3', 'node:*'],
    define: {
      'process.env.NODE_ENV': '"production"',
    },
  },
]);
