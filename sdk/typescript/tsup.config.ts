import { defineConfig } from 'tsup';

export default defineConfig({
  entry: ['src/index.ts'],
  // P2-1: 双格式构建 — ESM + CJS，兼容所有消费者
  format: ['esm', 'cjs'],
  dts: true,
  splitting: false,
  sourcemap: true,
  clean: true,
  treeshake: true,
  target: 'es2022',
  outDir: 'dist',
  // CJS 输出文件名使用 .cjs 扩展名避免与 ESM 冲突
  // tsup 会自动处理 .mjs/.cjs 扩展名
});
