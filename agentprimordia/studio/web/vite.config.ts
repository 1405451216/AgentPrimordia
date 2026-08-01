/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// AgentPrimordia Studio — Vite 配置
//
// 开发模式：`npm run dev` 默认监听 5173，并将 /api 代理到本地管理后端
// （`go run ./cmd/admin`，默认 :8080）。后端尚未实现的 /api/v1/* 端点
// 会由页面以空态/错误态优雅降级展示。
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
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
