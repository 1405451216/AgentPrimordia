/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// AgentPrimordia Studio — Vite 配置
//
// 开发模式：`npm run dev` 默认监听 5173，并将 /api 代理到 Studio 后端
// （`go run ./cmd/studio`，默认 :8090）。后端 /api/v1/* 端点由
// internal/studio 实现，默认返回 demo 数据，开箱即可演示。
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8090',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/test-setup.ts',
  },
});
